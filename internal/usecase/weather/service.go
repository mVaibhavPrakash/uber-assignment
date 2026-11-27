// Package weather restores, refreshes and publishes complete forecast snapshots.
package weather

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"mobilitycontrolroom/internal/domain"
)

var (
	// ErrCommitAmbiguous tells the service to reload after an uncertain write.
	ErrCommitAmbiguous = errors.New("forecast commit outcome unknown")
	// ErrRefreshBusy indicates that another job is publishing a forecast.
	ErrRefreshBusy = errors.New("forecast refresh already running")
)

// Areas supplies the zones that a forecast must cover.
type Areas interface {
	ListAreas(context.Context) ([]domain.Area, error)
}

// Store reads and atomically replaces the persisted forecast. An uncertain
// replacement returns ErrCommitAmbiguous. LoadLatest returns caller-owned data.
type Store interface {
	LoadLatest(context.Context) (domain.ForecastSnapshot, error)
	ReplaceSnapshot(context.Context, domain.ForecastSnapshot) error
}

// Provider fetches weather for all requested zones.
type Provider interface {
	Fetch(context.Context, []domain.Area, time.Time) (domain.ForecastSnapshot, error)
}

// Options controls forecast validity and the time allowed for commit recovery.
type Options struct {
	Window, RefreshInterval, MaxStaleness, RecoveryTimeout time.Duration
}

// Service publishes an immutable snapshot after it has been saved.
type Service struct {
	areas    Areas
	store    Store
	provider Provider
	options  Options
	now      func() time.Time
	update   sync.Mutex
	current  atomic.Pointer[cachedForecast]
}

type cachedForecast struct {
	snapshot    domain.ForecastSnapshot
	first, last time.Time
}

// NewService wires the forecast use case to its data sources.
func NewService(areas Areas, store Store, provider Provider, options Options, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{areas: areas, store: store, provider: provider, options: options, now: clock}
}

// RestoreSavedForecast loads the forecast written by an earlier run.
func (s *Service) RestoreSavedForecast(ctx context.Context) error {
	if !s.update.TryLock() {
		return ErrRefreshBusy
	}
	defer s.update.Unlock()

	areas, err := s.areas.ListAreas(ctx)
	if err != nil {
		return fmt.Errorf("restore forecast scope: %w", err)
	}
	_, err = s.loadSavedForecast(ctx, areas)
	if errors.Is(err, domain.ErrNoSnapshot) {
		s.current.Store(nil)
		return nil
	}
	return err
}

// FetchAndPublishForecast replaces the snapshot only after a successful save.
func (s *Service) FetchAndPublishForecast(ctx context.Context) error {
	if !s.update.TryLock() {
		return ErrRefreshBusy
	}
	defer s.update.Unlock()

	areas, err := s.areas.ListAreas(ctx)
	if err != nil {
		return fmt.Errorf("refresh forecast scope: %w", err)
	}
	snapshot, err := s.provider.Fetch(ctx, areas, s.now())
	if err != nil {
		return fmt.Errorf("fetch forecast: %w", err)
	}
	snapshot.FetchedAt = s.now().UTC().Truncate(time.Microsecond)
	if err := domain.ValidateForecast(snapshot, areas); err != nil {
		return fmt.Errorf("validate fetched forecast: %w", err)
	}
	if err := s.store.ReplaceSnapshot(ctx, snapshot); err != nil {
		wrapped := fmt.Errorf("save forecast: %w", err)
		if errors.Is(err, ErrCommitAmbiguous) {
			return s.reconcileForecastCommit(ctx, areas, snapshot.Version, wrapped)
		}
		return wrapped
	}
	s.publish(snapshot)
	return nil
}

// ForecastStatus checks freshness and coverage without copying the forecast.
func (s *Service) ForecastStatus(now time.Time) (domain.ForecastStatus, error) {
	status, _, err := s.usableForecast(now)
	return status, err
}

// CurrentForecast returns a caller-owned copy of the usable forecast.
func (s *Service) CurrentForecast(now time.Time) (domain.ForecastView, error) {
	status, cached, err := s.usableForecast(now)
	view := domain.ForecastView{ForecastStatus: status}
	if err == nil {
		snapshot := cached.snapshot
		snapshot.Hours = slices.Clone(snapshot.Hours)
		view.Snapshot = &snapshot
	}
	return view, err
}

func (s *Service) usableForecast(now time.Time) (domain.ForecastStatus, *cachedForecast, error) {
	status := domain.ForecastStatus{Freshness: "unavailable"}
	cached := s.current.Load()
	if cached == nil {
		return status, nil, domain.ErrNoSnapshot
	}
	status.Version = cached.snapshot.Version
	status.FetchedAt = cached.snapshot.FetchedAt
	age := now.Sub(status.FetchedAt)
	if age < 0 || age > s.options.MaxStaleness {
		return status, nil, domain.ErrForecastExpired
	}

	start := domain.NextFullHour(now)
	last := start.Add(s.options.Window - time.Hour)
	if cached.first.After(start) || cached.last.Before(last) {
		return status, nil, domain.ErrForecastCoverage
	}

	status.Freshness = "fresh"
	if age >= s.options.RefreshInterval {
		status.Freshness = "stale"
	}
	return status, cached, nil
}

func (s *Service) publish(snapshot domain.ForecastSnapshot) {
	snapshot.Hours = slices.Clone(snapshot.Hours)
	cached := &cachedForecast{snapshot: snapshot, first: snapshot.Hours[0].HourStart, last: snapshot.Hours[0].HourStart}
	for _, h := range snapshot.Hours[1:] {
		if h.HourStart.Before(cached.first) {
			cached.first = h.HourStart
		}
		if h.HourStart.After(cached.last) {
			cached.last = h.HourStart
		}
	}
	s.current.Store(cached)
}

func (s *Service) loadSavedForecast(ctx context.Context, areas []domain.Area) (string, error) {
	snapshot, err := s.store.LoadLatest(ctx)
	if err != nil {
		return "", fmt.Errorf("load saved forecast: %w", err)
	}
	if err := domain.ValidateForecast(snapshot, areas); err != nil {
		return "", fmt.Errorf("validate saved forecast: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.publish(snapshot)
	return snapshot.Version, nil
}

func (s *Service) reconcileForecastCommit(ctx context.Context, areas []domain.Area, attemptedVersion string, commitErr error) error {
	// An expired write may still have committed. Give the reconciliation read its own deadline.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.options.RecoveryTimeout)
	defer cancel()
	savedVersion, err := s.loadSavedForecast(ctx, areas)
	if err != nil {
		s.current.Store(nil)
		return errors.Join(commitErr, fmt.Errorf("reconcile forecast commit: %w", err))
	}
	if savedVersion == attemptedVersion {
		return nil
	}
	return commitErr
}
