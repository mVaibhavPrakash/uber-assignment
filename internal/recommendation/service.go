// Manager manages recommendation computation; the scheduler calls ComputeAndPublish,
// while HTTP handlers call CurrentRecommendation to read the saved result.
package recommendation

import (
	"context"
	"fmt"
	"mobilitycontrolroom/internal/appErrors"
	"mobilitycontrolroom/internal/config"
	"mobilitycontrolroom/internal/repository"
	"mobilitycontrolroom/internal/weather"
	"sync/atomic"
	"time"
)

// Supplies the zones and historical statistics loaded from PostgreSQL.
type HistoricalEvidence interface {
	Areas() []repository.Area
	LookupComparableHour(int, time.Time) (repository.HourlyStats, bool)
}

// Supplies the latest usable weather snapshot.
type ForecastEvidence interface {
	CurrentForecast(time.Time) (weather.View, error)
}

// Prepares scoring inputs and atomically publishes a read-only result for API requests.
type Manager struct {
	history HistoricalEvidence
	weather ForecastEvidence
	options Options
	maxAge  time.Duration
	result  atomic.Pointer[Result]
}

// Connects historical evidence, weather and the configured scoring rules.
func NewManager(h HistoricalEvidence, w ForecastEvidence, c config.Config) (*Manager, error) {
	o := Options{Window: c.GetWindow(), MinSamples: int64(c.GetMinHistoricalSamples()), RainThreshold: c.GetMinRainfallMM(), AdditionalPerHour: c.GetMinAdditionalTripsPerHour(), Location: c.GetLocation()}
	if err := o.Validate(); err != nil {
		return nil, err
	}
	return &Manager{history: h, weather: w, options: o, maxAge: c.GetWeatherMaxStaleness()}, nil
}

// Returns the zones loaded from the database.
func (s *Manager) Areas() []repository.Area {
	return s.history.Areas()
}

// ComputeAndPublish runs the four steps of a recommendation update in order.
// Weather fetching is a separate scheduler job; this method reads cached inputs.
func (s *Manager) ComputeAndPublish(ctx context.Context, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// 1. Read usable weather from the forecast cache.
	view, err := s.weather.CurrentForecast(now)
	if err != nil {
		result := s.newUnavailableResult(now, view)
		s.result.Store(&result)
		return fmt.Errorf("recommendation forecast: %w", err)
	}

	// 2. Match each forecast hour with its cached historical trip statistics.
	inputs, err := s.joinForecastAndHistory(ctx, view.Snapshot.Hours, NextFullHourWindow(now, s.options.Window))
	if err != nil {
		return err
	}

	// 3. Estimate trips, compare assessable zones and choose the recommendation.
	result, err := EvaluatePlanningWindow(now, s.options, inputs)
	if err != nil {
		return err
	}

	// 4. Set forecast validity and publish the finished result to the cache.
	return s.publishRecommendation(ctx, result, view)
}

func (s *Manager) publishRecommendation(ctx context.Context, result Result, view weather.View) error {
	result.ForecastVersion = view.Version
	result.ForecastFetchedAt = &view.FetchedAt
	result.Freshness = view.Freshness
	expiry := view.FetchedAt.Add(s.maxAge)
	if expiry.Before(result.ValidUntil) {
		result.ValidUntil = expiry
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.result.Store(&result)
	return nil
}

// Returns the saved result only while its weather, window and expiry still match.
// Nested slices and pointers are shared and must never be modified by readers.
// Only the small result value is copied to report current freshness.
func (s *Manager) CurrentRecommendation(now time.Time) Result {
	view, err := s.weather.CurrentForecast(now)
	cached := s.result.Load()
	if err != nil || !s.matchesForecastWindowAndExpiry(cached, view.Version, now) {
		return s.newUnavailableResult(now, view)
	}
	result := *cached
	result.Freshness = view.Freshness
	return result
}

// Tells the scheduler when weather, the planning window or expiry changed.
func (s *Manager) NeedsRecompute(now time.Time) bool {
	view, err := s.weather.CurrentForecast(now)
	cached := s.result.Load()
	if cached == nil {
		return true
	}
	if err != nil {
		return cached.Status != "unavailable"
	}
	return !s.matchesForecastWindowAndExpiry(cached, view.Version, now)
}

// One validity rule is shared by API reads and the scheduler.
func (s *Manager) matchesForecastWindowAndExpiry(result *Result, version string, now time.Time) bool {
	return result != nil && result.ForecastVersion == version &&
		result.Window.Start.Equal(NextFullHourWindow(now, s.options.Window).Start) && now.Before(result.ValidUntil)
}

type forecastKey struct {
	area int
	hour int64
}

// Joins forecast hours with matching historical evidence in memory.
func (s *Manager) joinForecastAndHistory(ctx context.Context, hours []repository.ForecastHour, window Window) ([]AreaInput, error) {
	scope := s.history.Areas()
	forecast := map[forecastKey]repository.ForecastHour{}
	for _, h := range hours {
		forecast[forecastKey{h.AreaID, h.HourStart.Unix()}] = h
	}

	// Leave missing inputs empty so scoring can explain the missing evidence.

	inputs := make([]AreaInput, 0, len(scope))
	for _, area := range scope {
		input := AreaInput{Area: area}
		for at := window.Start; at.Before(window.End); at = at.Add(time.Hour) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			hour := HourInput{At: at}
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

// Creates a response with no action when a current result cannot be used.
func (s *Manager) newUnavailableResult(now time.Time, view weather.View) Result {
	result := newPlanningWindowResult(now, s.options, len(s.history.Areas()))
	result.Status = "unavailable"
	result.DecisionReason = ReasonRecommendationUnavailable
	result.Message = appErrors.RecommendationUnavailable.Message
	result.ErrorCode = appErrors.RecommendationUnavailable.Code
	result.Freshness = "unavailable"
	result.ValidUntil = now.UTC()
	result.ForecastVersion = view.Version
	return result
}
