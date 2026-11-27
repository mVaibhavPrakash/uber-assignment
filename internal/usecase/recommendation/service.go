// Package recommendation joins evidence, runs scoring and publishes the current decision.
package recommendation

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/domain/recommendation"
)

// HistoricalEvidence supplies the zones and their comparable hourly statistics.
type HistoricalEvidence interface {
	Areas() []domain.Area
	LookupComparableHour(int, time.Time) (domain.HourlyStats, bool)
}

// ForecastEvidence supplies usable weather and a cheap status check.
type ForecastEvidence interface {
	CurrentForecast(time.Time) (domain.ForecastView, error)
	ForecastStatus(time.Time) (domain.ForecastStatus, error)
}

// Manager computes and publishes the recommendation for the current planning window.
type Manager struct {
	history   HistoricalEvidence
	weather   ForecastEvidence
	options   recommendation.Options
	maxAge    time.Duration
	areaCount int
	update    sync.Mutex
	result    atomic.Pointer[recommendation.Result]
}

// NewManager connects historical evidence and weather to the scoring policy.
func NewManager(h HistoricalEvidence, w ForecastEvidence, options recommendation.Options, maxAge time.Duration) (*Manager, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	return &Manager{history: h, weather: w, options: options, maxAge: maxAge, areaCount: len(h.Areas())}, nil
}

// Areas returns the configured zones.
func (s *Manager) Areas() []domain.Area {
	return s.history.Areas()
}

// ComputeAndPublish runs scoring on a single version of the forecast.
func (s *Manager) ComputeAndPublish(ctx context.Context, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.update.TryLock() {
		return errors.New("recommendation computation already running")
	}
	defer s.update.Unlock()

	view, err := s.weather.CurrentForecast(now)
	if err == nil && view.Snapshot == nil {
		err = domain.ErrNoSnapshot
	}
	if err != nil {
		if ctx.Err() == nil {
			result := recommendation.UnavailableResult(now, s.options, s.areaCount, view.Version)
			s.result.Store(&result)
		}
		return fmt.Errorf("recommendation forecast: %w", err)
	}

	window := recommendation.NextFullHourWindow(now, s.options.Window)
	inputs, err := s.joinForecastAndHistory(ctx, view.Snapshot.Hours, window)
	if err != nil {
		return fmt.Errorf("match recommendation evidence: %w", err)
	}
	result, err := recommendation.EvaluatePlanningWindow(ctx, now, s.options, inputs)
	if err != nil {
		return fmt.Errorf("score recommendation: %w", err)
	}

	result.ForecastVersion = view.Version
	result.ForecastFetchedAt = &view.FetchedAt
	result.Freshness = view.Freshness
	if expiry := view.FetchedAt.Add(s.maxAge); expiry.Before(result.ValidUntil) {
		result.ValidUntil = expiry
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.result.Store(&result)
	return nil
}

// CurrentRecommendation returns the published decision while it is still valid.
// Its nested slices and pointers are shared and must be treated as read-only.
func (s *Manager) CurrentRecommendation(now time.Time) recommendation.Result {
	status, err := s.weather.ForecastStatus(now)
	cached := s.result.Load()
	if err != nil || !matchesForecastWindowAndExpiry(cached, status.Version, now) {
		return recommendation.UnavailableResult(now, s.options, s.areaCount, status.Version)
	}
	result := *cached
	result.Freshness = status.Freshness
	return result
}

// NeedsRecompute reports whether new evidence or a new planning window requires scoring.
func (s *Manager) NeedsRecompute(now time.Time) bool {
	status, err := s.weather.ForecastStatus(now)
	cached := s.result.Load()
	if cached == nil {
		return true
	}
	if err != nil {
		return cached.Status != "unavailable"
	}
	return !matchesForecastWindowAndExpiry(cached, status.Version, now)
}

func matchesForecastWindowAndExpiry(result *recommendation.Result, version string, now time.Time) bool {
	return result != nil && result.ForecastVersion == version &&
		result.Window.Start.Equal(domain.NextFullHour(now)) && now.Before(result.ValidUntil)
}

type forecastKey struct {
	area int
	hour int64
}

func (s *Manager) joinForecastAndHistory(ctx context.Context, hours []domain.ForecastHour, window recommendation.Window) ([]recommendation.AreaInput, error) {
	scope := s.history.Areas()
	forecast := make(map[forecastKey]domain.ForecastHour)
	for _, h := range hours {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !h.HourStart.Before(window.Start) && h.HourStart.Before(window.End) {
			forecast[forecastKey{h.AreaID, h.HourStart.Unix()}] = h
		}
	}

	// Leave missing inputs empty so scoring can explain unavailable evidence.
	inputs := make([]recommendation.AreaInput, 0, len(scope))
	for _, area := range scope {
		input := recommendation.AreaInput{Area: area, Hours: make([]recommendation.HourInput, 0, int(s.options.Window/time.Hour))}
		for at := window.Start; at.Before(window.End); at = at.Add(time.Hour) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			hour := recommendation.HourInput{At: at}
			if f, ok := forecast[forecastKey{area.ID, at.Unix()}]; ok {
				hour.Forecast = &f
			}
			if stats, ok := s.history.LookupComparableHour(area.ID, at); ok {
				hour.History = &stats
			}
			input.Hours = append(input.Hours, hour)
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}
