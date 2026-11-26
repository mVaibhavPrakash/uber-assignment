package weather

import (
	"context"
	"errors"
	"fmt"
	"mobilitycontrolroom/internal/apperrors"

	"mobilitycontrolroom/internal/cache"
	"mobilitycontrolroom/internal/config"
	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/repository"
	"sync"
	"time"
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

func (s *Service) Restore(ctx context.Context) error {
	areas, err := s.areas.ListAreas(ctx)
	if err != nil {
		return fmt.Errorf("restore forecast scope: %w", err)
	}
	snapshot, err := s.forecasts.LoadLatest(ctx)
	if errors.Is(err, apperrors.ErrNoSnapshot) {
		s.store.Clear()
		return nil
	}
	if err != nil {
		return fmt.Errorf("restore forecast snapshot: %w", err)
	}
	if err = domain.ValidateForecast(snapshot, areas); err != nil {
		return err
	}
	s.store.Set(snapshot)
	return nil
}

func (s *Service) Refresh(ctx context.Context) error {
	if !s.refresh.TryLock() {
		return apperrors.ErrWeatherRefreshBusy
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
		if errors.Is(err, apperrors.ErrCommitAmbiguous) {
			return apperrors.ReconcileCommit(ctx, s.config.DBTimeout, snapshot.Version, err,
				func(ctx context.Context) (string, error) { return s.reloadForecast(ctx, areas) }, s.store.Clear)
		}
		return err
	}
	s.store.Set(snapshot)
	return nil
}

func (s *Service) Current(now time.Time) (View, error) {
	snapshot, ok := s.store.Get()
	if !ok {
		return View{Freshness: "unavailable"}, apperrors.ErrNoSnapshot
	}
	view := View{Freshness: "fresh", Version: snapshot.Version, FetchedAt: snapshot.FetchedAt}
	age := now.Sub(snapshot.FetchedAt)
	if age < 0 || age > s.config.WeatherMaxStaleness {
		return View{Freshness: "unavailable", Version: snapshot.Version, FetchedAt: snapshot.FetchedAt}, apperrors.ErrForecastExpired
	}
	start := now.UTC().Truncate(time.Hour).Add(time.Hour)
	end := start.Add(s.config.Window)
	first, last := forecastBounds(snapshot.Hours)
	if first.After(start) || last.Before(end.Add(-time.Hour)) {
		view.Freshness = "unavailable"
		return view, apperrors.ErrForecastCoverage
	}
	if age >= s.config.WeatherRefresh {
		view.Freshness = "stale"
	}
	view.Snapshot = &snapshot
	return view, nil
}

// Finds the first and last hours covered by a complete snapshot.
func forecastBounds(hours []repository.ForecastHour) (time.Time, time.Time) {
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

// Reads and publishes the saved snapshot after an uncertain commit.
func (s *Service) reloadForecast(ctx context.Context, areas []repository.Area) (string, error) {
	snapshot, err := s.forecasts.LoadLatest(ctx)
	if err != nil {
		return "", err
	}
	if err := domain.ValidateForecast(snapshot, areas); err != nil {
		return "", err
	}
	s.store.Set(snapshot)
	return snapshot.Version, nil
}
