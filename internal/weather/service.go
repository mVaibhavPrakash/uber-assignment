package weather

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"mobilitycontrolroom/internal/appErrors"
	"mobilitycontrolroom/internal/cache"
	"mobilitycontrolroom/internal/config"
	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/repository"
)

type Provider interface {
	Fetch(context.Context, []repository.Area, time.Time) (repository.ForecastSnapshot, error)
}
type Service struct {
	areas     repository.AreaRepository
	forecasts repository.ForecastRepository
	provider  Provider
	store     cache.ForecastStore
	config    config.Config
	now       func() time.Time
	refresh   sync.Mutex
}

type View struct {
	Freshness string                       `json:"freshness"`
	Version   string                       `json:"version,omitempty"`
	FetchedAt time.Time                    `json:"fetched_at"`
	Snapshot  *repository.ForecastSnapshot `json:"-"`
}

func NewService(areas repository.AreaRepository, forecasts repository.ForecastRepository, provider Provider, c config.Config, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{areas: areas, forecasts: forecasts, provider: provider, config: c, now: clock}
}

func (s *Service) RestoreSavedForecast(ctx context.Context) error {
	areas, err := s.areas.ListAreas(ctx)
	if err != nil {
		return fmt.Errorf("restore forecast scope: %w", err)
	}
	_, err = s.loadSavedForecast(ctx, areas)
	if errors.Is(err, appErrors.ErrNoSnapshot) {
		s.store.Clear()
		return nil
	}
	return err
}

func (s *Service) FetchAndPublishForecast(ctx context.Context) error {
	if !s.refresh.TryLock() {
		return appErrors.ErrWeatherRefreshBusy
	}
	defer s.refresh.Unlock()
	areas, err := s.areas.ListAreas(ctx)
	if err != nil {
		return fmt.Errorf("refresh forecast scope: %w", err)
	}
	snapshot, err := s.provider.Fetch(ctx, areas, s.now())
	if err != nil {
		return err
	}
	snapshot.FetchedAt = s.now().UTC().Truncate(time.Microsecond)
	if err = domain.ValidateForecast(snapshot, areas); err != nil {
		return err
	}
	if err = s.forecasts.ReplaceSnapshot(ctx, snapshot); err != nil {
		if errors.Is(err, appErrors.ErrCommitAmbiguous) {
			return s.reconcileForecastCommit(ctx, areas, snapshot.Version, err)
		}
		return err
	}
	s.store.Set(snapshot)
	return nil
}

func (s *Service) CurrentForecast(now time.Time) (View, error) {
	snapshot, ok := s.store.Get()
	if !ok {
		return View{Freshness: "unavailable"}, appErrors.ErrNoSnapshot
	}

	view := View{Freshness: "fresh", Version: snapshot.Version, FetchedAt: snapshot.FetchedAt}

	age := now.Sub(snapshot.FetchedAt)
	if age < 0 || age > s.config.GetWeatherMaxStaleness() {
		view.Freshness = "unavailable"
		return view, appErrors.ErrForecastExpired
	}

	start := domain.NextFullHour(now)
	end := start.Add(s.config.GetWindow())
	first, last := coveredHourRange(snapshot.Hours)
	if first.After(start) || last.Before(end.Add(-time.Hour)) {
		view.Freshness = "unavailable"
		return view, appErrors.ErrForecastCoverage
	}

	if age >= s.config.GetWeatherRefresh() {
		view.Freshness = "stale"
	}
	view.Snapshot = &snapshot

	return view, nil
}

// Finds the first and last hours covered by a complete snapshot.
func coveredHourRange(hours []repository.ForecastHour) (time.Time, time.Time) {
	first, last := hours[0].HourStart, hours[0].HourStart
	for _, h := range hours {
		if h.HourStart.Before(first) {
			first = h.HourStart
		}
		if h.HourStart.After(last) {
			last = h.HourStart
		}
	}
	return first, last
}

// Startup and commit recovery both publish the version PostgreSQL actually holds.
func (s *Service) loadSavedForecast(ctx context.Context, areas []repository.Area) (string, error) {
	snapshot, err := s.forecasts.LoadLatest(ctx)
	if err != nil {
		return "", fmt.Errorf("load saved forecast: %w", err)
	}
	if err := domain.ValidateForecast(snapshot, areas); err != nil {
		return "", err
	}
	s.store.Set(snapshot)
	return snapshot.Version, nil
}

func (s *Service) reconcileForecastCommit(ctx context.Context, areas []repository.Area, attemptedVersion string, commitErr error) error {
	// The write may have committed even if its context expired. Give the read its own deadline.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.config.GetDBTimeout())
	defer cancel()
	savedVersion, err := s.loadSavedForecast(ctx, areas)
	if err != nil {
		s.store.Clear()
		return errors.Join(commitErr, fmt.Errorf("reconcile forecast commit: %w", err))
	}
	if savedVersion == attemptedVersion {
		return nil
	}
	return commitErr
}
