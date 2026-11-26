// Package scheduler runs bounded weather and recommendation jobs sequentially.
package scheduler

import (
	"context"
	"mobilitycontrolroom/internal/apperrors"

	"log/slog"
	"mobilitycontrolroom/internal/config"
	"sync"
	"time"
)

type Weather interface {
	FetchAndPublishForecast(context.Context) error
}

type Recommendations interface {
	ComputeAndPublish(context.Context, time.Time) error
	NeedsRecompute(time.Time) bool
}

type JobState struct {
	LastAttempt, LastSuccess time.Time
	LastError                string
}

type State struct {
	Weather, Recommendation JobState
}

type Scheduler struct {
	weather         Weather
	recommendations Recommendations
	config          config.Config
	clock           func() time.Time
	tick            sync.Mutex
	stateMu         sync.RWMutex
	state           State
}

func New(w Weather, r Recommendations, c config.Config, clock func() time.Time) *Scheduler {
	if clock == nil {
		clock = time.Now
	}
	return &Scheduler{weather: w, recommendations: r, config: c, clock: clock}
}

func (s *Scheduler) State() State {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.state
}

func (s *Scheduler) recordJobOutcome(weatherJob bool, attempt, success time.Time, err error) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	job := &s.state.Recommendation
	if weatherJob {
		job = &s.state.Weather
	}
	job.LastAttempt = attempt
	if err == nil {
		job.LastSuccess = success
		job.LastError = ""
	} else {
		job.LastError = "refresh_failed"
	}
}

func (s *Scheduler) Tick(ctx context.Context) error {
	if !s.tick.TryLock() {
		return apperrors.ErrSchedulerBusy
	}
	defer s.tick.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	state := s.State()
	now := s.clock()
	refreshed := false
	if state.Weather.LastAttempt.IsZero() || now.Sub(state.Weather.LastAttempt) >= s.config.WeatherRefresh {
		refreshed = s.runWeatherRefreshJob(ctx, now)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	now = s.clock()
	if refreshed || state.Recommendation.LastAttempt.IsZero() || now.Sub(state.Recommendation.LastAttempt) >= s.config.RecommendationRefresh || s.recommendations.NeedsRecompute(now) {
		s.runRecommendationJob(ctx, now)
	}
	return nil
}

func (s *Scheduler) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		_ = s.Tick(ctx)
		now := s.clock()
		delay := s.config.SchedulerCheck
		untilHour := now.UTC().Truncate(time.Hour).Add(time.Hour).Sub(now)
		if untilHour < delay {
			delay = untilHour
		}
		if delay <= 0 {
			delay = time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// Runs one bounded weather job and records its outcome.
func (s *Scheduler) runWeatherRefreshJob(ctx context.Context, now time.Time) bool {
	job, cancel := context.WithTimeout(ctx, s.config.WeatherJobTimeout)
	defer cancel()
	err := s.weather.FetchAndPublishForecast(job)
	s.recordJobOutcome(true, now, s.clock(), err)
	if err != nil {
		slog.Error("weather job failed", "op", "weather_refresh", "error", err)
	}
	return err == nil
}

// Runs one bounded scoring job and records its outcome.
func (s *Scheduler) runRecommendationJob(ctx context.Context, now time.Time) {
	job, cancel := context.WithTimeout(ctx, s.config.RecommendationJobTimeout)
	defer cancel()
	err := s.recommendations.ComputeAndPublish(job, now)
	s.recordJobOutcome(false, now, s.clock(), err)
	if err != nil {
		slog.Error("recommendation job failed", "op", "recommendation_refresh", "error", err)
	}
}
