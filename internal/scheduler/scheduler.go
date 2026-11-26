// Package scheduler runs bounded weather and recommendation jobs sequentially.
package scheduler

import (
	"context"
	"log/slog"
	"mobilitycontrolroom/internal/config"
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
	// Only Run reads and updates job state.
	state State
}

func New(w Weather, r Recommendations, c config.Config, clock func() time.Time) *Scheduler {
	if clock == nil {
		clock = time.Now
	}
	return &Scheduler{weather: w, recommendations: r, config: c, clock: clock}
}

func (s *Scheduler) recordJobOutcome(weatherJob bool, attempt, success time.Time, err error) {
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

func (s *Scheduler) tick(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	state := s.state
	now := s.clock()
	refreshed := false

	// 1. Fetch and save new weather when due. Always attempt it after startup.
	if state.Weather.LastAttempt.IsZero() || now.Sub(state.Weather.LastAttempt) >= s.config.GetWeatherRefresh() {
		refreshed = s.runWeatherRefreshJob(ctx, now)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// 2. Update the recommendation after a weather refresh or when its result is due.
	now = s.clock()
	if refreshed || state.Recommendation.LastAttempt.IsZero() || now.Sub(state.Recommendation.LastAttempt) >= s.config.GetRecommendationRefresh() || s.recommendations.NeedsRecompute(now) {
		s.runRecommendationJob(ctx, now)
	}
	return nil
}

// Run is called once and runs each job to completion before checking again.
func (s *Scheduler) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		_ = s.tick(ctx)
		now := s.clock()
		delay := s.config.GetSchedulerCheck()

		// At 10:53, waiting 15 minutes would mean checking again at 11:08.
		// The recommendation expires at 11:00, so the UI would have no valid
		// recommendation for 8 minutes. Wait 7 minutes and update at 11:00 instead.
		untilHour := now.UTC().Truncate(time.Hour).Add(time.Hour).Sub(now)
		if untilHour < delay {
			delay = untilHour
		}
		if delay <= 0 {
			// Always wait a little so we don't keep checking without a pause.
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
	job, cancel := context.WithTimeout(ctx, s.config.GetWeatherJobTimeout())
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
	job, cancel := context.WithTimeout(ctx, s.config.GetRecommendationJobTimeout())
	defer cancel()
	err := s.recommendations.ComputeAndPublish(job, now)
	s.recordJobOutcome(false, now, s.clock(), err)
	if err != nil {
		slog.Error("recommendation job failed", "op", "recommendation_refresh", "error", err)
	}
}
