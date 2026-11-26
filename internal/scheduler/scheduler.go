// Package scheduler runs bounded weather and recommendation jobs sequentially.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"mobilitycontrolroom/internal/config"
	"mobilitycontrolroom/internal/domain"
)

type Weather interface {
	FetchAndPublishForecast(context.Context) error
}

type Recommendations interface {
	ComputeAndPublish(context.Context, time.Time) error
	NeedsRecompute(time.Time) bool
}

type Scheduler struct {
	weather                                       Weather
	recommendations                               Recommendations
	config                                        config.Config
	clock                                         func() time.Time
	lastWeatherAttempt, lastRecommendationAttempt time.Time
}

func New(w Weather, r Recommendations, c config.Config, clock func() time.Time) *Scheduler {
	if clock == nil {
		clock = time.Now
	}
	return &Scheduler{weather: w, recommendations: r, config: c, clock: clock}
}

// Run is called once and runs each job to completion before checking again.
func (s *Scheduler) Run(ctx context.Context) {
	for ctx.Err() == nil {
		s.tick(ctx)
		now := s.clock()
		delay := s.config.GetSchedulerCheck()

		// At 10:53, waiting 15 minutes would mean checking again at 11:08.
		// The recommendation expires at 11:00, so the UI would have no valid
		// recommendation for 8 minutes. Wait 7 minutes and update at 11:00 instead.
		delay = min(delay, domain.NextFullHour(now).Sub(now))
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

func (s *Scheduler) tick(ctx context.Context) {
	now := s.clock()
	refreshed := false
	if jobDue(s.lastWeatherAttempt, now, s.config.GetWeatherRefresh()) {
		refreshed = s.runWeatherRefreshJob(ctx, now)
	}
	if ctx.Err() != nil {
		return
	}

	// A slow weather request can cross into a new planning window.
	now = s.clock()
	if refreshed || jobDue(s.lastRecommendationAttempt, now, s.config.GetRecommendationRefresh()) || s.recommendations.NeedsRecompute(now) {
		s.runRecommendationJob(ctx, now)
	}
}

func jobDue(lastAttempt, now time.Time, interval time.Duration) bool {
	return lastAttempt.IsZero() || now.Sub(lastAttempt) >= interval
}

func (s *Scheduler) runWeatherRefreshJob(ctx context.Context, now time.Time) bool {
	job, cancel := context.WithTimeout(ctx, s.config.GetWeatherJobTimeout())
	defer cancel()
	err := s.weather.FetchAndPublishForecast(job)
	s.lastWeatherAttempt = now
	if err != nil {
		slog.Error("weather job failed", "op", "weather_refresh", "error", err)
	}
	return err == nil
}

func (s *Scheduler) runRecommendationJob(ctx context.Context, now time.Time) {
	job, cancel := context.WithTimeout(ctx, s.config.GetRecommendationJobTimeout())
	defer cancel()
	err := s.recommendations.ComputeAndPublish(job, now)
	s.lastRecommendationAttempt = now
	if err != nil {
		slog.Error("recommendation job failed", "op", "recommendation_refresh", "error", err)
	}
}
