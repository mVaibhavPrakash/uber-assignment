package recommendation

import (
	"context"
	"fmt"
	"mobilitycontrolroom/internal/apperrors"
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
type Service struct {
	history HistoricalEvidence
	weather ForecastEvidence
	options Options
	maxAge  time.Duration
	result  atomic.Pointer[Result]
}

// Connects historical evidence, weather and the configured scoring rules.
func NewService(h HistoricalEvidence, w ForecastEvidence, c config.Config) (*Service, error) {
	o := Options{Window: c.Window, MinSamples: int64(c.MinHistoricalSamples), RainThreshold: c.MinRainfallMM, AdditionalPerHour: c.MinAdditionalTripsPerHour, Location: c.Location}
	if err := o.Validate(); err != nil {
		return nil, err
	}
	return &Service{history: h, weather: w, options: o, maxAge: c.WeatherMaxStaleness}, nil
}

// Returns the zones loaded from the database.
func (s *Service) Areas() []repository.Area {
	return s.history.Areas()
}

// Builds and saves one recommendation result for the current planning window.
func (s *Service) ComputeAndPublish(ctx context.Context, now time.Time) error {
	// Step 1: Stop if cancelled, then get the latest usable weather.
	if err := ctx.Err(); err != nil {
		return err
	}
	view, err := s.weather.CurrentForecast(now)
	if err != nil {
		result := s.newUnavailableResult(now, view)
		s.result.Store(&result)
		return fmt.Errorf("recommendation forecast: %w", err)
	}
	// Step 2: Pair forecast hours with the historical statistics for each zone.
	inputs, err := s.joinForecastAndHistory(ctx, view.Snapshot.Hours, NextFullHourWindow(now, s.options.Window))
	if err != nil {
		return err
	}
	// Step 3: Calculate the estimates and choose an action or a no-action message.
	result, err := EvaluatePlanningWindow(now, s.options, inputs)
	if err != nil {
		return err
	}
	// Step 4: Record which forecast was used. Expire the result when the window
	// changes or the forecast gets too old, whichever comes first.
	result.ForecastVersion = view.Version
	fetched := view.FetchedAt
	result.ForecastFetchedAt = &fetched
	result.Freshness = view.Freshness
	expiry := fetched.Add(s.maxAge)
	if expiry.Before(result.ValidUntil) {
		result.ValidUntil = expiry
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// Step 5: Publish the finished result in one operation for API readers.
	s.result.Store(&result)
	return nil
}

// Returns the saved result only while its weather, window and expiry still match.
// Nested slices and pointers are shared and must never be modified by readers.
// Only the small result value is copied to report current freshness.
func (s *Service) CurrentRecommendation(now time.Time) Result {
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
func (s *Service) NeedsRecompute(now time.Time) bool {
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
func (s *Service) matchesForecastWindowAndExpiry(result *Result, version string, now time.Time) bool {
	return result != nil && result.ForecastVersion == version &&
		result.Window.Start.Equal(NextFullHourWindow(now, s.options.Window).Start) && now.Before(result.ValidUntil)
}

type forecastKey struct {
	area int
	hour int64
}

// Joins forecast hours with matching historical evidence in memory.
func (s *Service) joinForecastAndHistory(ctx context.Context, hours []repository.ForecastHour, window Window) ([]AreaInput, error) {
	// Step 1: Index forecasts by zone and hour so each lookup is quick.
	scope := s.history.Areas()
	forecast := map[forecastKey]repository.ForecastHour{}
	for _, h := range hours {
		forecast[forecastKey{h.AreaID, h.HourStart.Unix()}] = h
	}
	// Step 2: Pair each forecast hour with its matching historical statistics.
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
func (s *Service) newUnavailableResult(now time.Time, view weather.View) Result {
	result := newPlanningWindowResult(now, s.options, len(s.history.Areas()))
	result.Status = "unavailable"
	result.DecisionReason = ReasonRecommendationUnavailable
	result.Message = apperrors.RecommendationUnavailable.Message
	result.ErrorCode = apperrors.RecommendationUnavailable.Code
	result.Freshness = "unavailable"
	result.ValidUntil = now.UTC()
	result.ForecastVersion = view.Version
	return result
}
