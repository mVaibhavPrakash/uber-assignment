package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"mobilitycontrolroom/internal/api"
	"mobilitycontrolroom/internal/apperrors"
	"mobilitycontrolroom/internal/config"
	"mobilitycontrolroom/internal/recommendation"
	"mobilitycontrolroom/internal/repository"
	"mobilitycontrolroom/internal/weather"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type recommendationHistory []recommendation.AreaInput

func (h recommendationHistory) Areas() []repository.Area { return []repository.Area{h[0].Area} }
func (h recommendationHistory) LookupComparableHour(id int, at time.Time) (repository.HourlyStats, bool) {
	for _, hour := range h[0].Hours {
		if id == h[0].Area.ID && hour.At.Equal(at) {
			return *hour.History, true
		}
	}
	return repository.HourlyStats{}, false
}

type recommendationWeather struct{ view atomic.Pointer[weather.View] }

func (w *recommendationWeather) CurrentForecast(time.Time) (weather.View, error) {
	if view := w.view.Load(); view != nil {
		return *view, nil
	}
	return weather.View{}, apperrors.ErrNoSnapshot
}

// Runs under the existing recommendation test, keeping three top-level tests.
func checkRecommendationCache(t *testing.T) {
	now, _, inputs := recommendationInput(t)
	cfg, err := config.Parse(func(key string) (string, bool) {
		if key == "DATABASE_URL" {
			return "postgres://localhost/test", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	view := weather.View{Version: "first", Freshness: "fresh", FetchedAt: now, Snapshot: &repository.ForecastSnapshot{}}
	for _, hour := range inputs[0].Hours {
		view.Snapshot.Hours = append(view.Snapshot.Hours, *hour.Forecast)
	}
	forecast := &recommendationWeather{}
	forecast.view.Store(&view)
	service, err := recommendation.NewService(recommendationHistory(inputs), forecast, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ComputeAndPublish(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	handler := api.Handler(service, func() time.Time { return now })
	read := func(path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		return response
	}
	before := service.CurrentRecommendation(now)
	for i := 0; i < 3; i++ {
		var summary recommendation.Result
		if err := json.Unmarshal(read("/api/recommendations").Body.Bytes(), &summary); err != nil {
			t.Fatal(err)
		}
		if summary.Primary == nil || len(summary.Primary.Hourly) != 0 || len(summary.Areas[0].Hourly) != 0 {
			t.Fatal("summary must omit hourly details")
		}
		var insights struct{ Evidence recommendation.AreaEvidence }
		if err := json.Unmarshal(read("/api/areas/7/insights").Body.Bytes(), &insights); err != nil || len(insights.Evidence.Hourly) != 2 {
			t.Fatalf("summary changed cached insights: %v", err)
		}
	}
	after := service.CurrentRecommendation(now)
	if &before.Areas[0].Hourly[0] != &after.Areas[0].Hourly[0] || len(after.Primary.Hourly) != 2 {
		t.Fatal("reads must reuse read-only evidence without removing or deep-copying it")
	}
	stale := view
	stale.Freshness = "stale"
	forecast.view.Store(&stale)
	for _, path := range []string{"/api/recommendations", "/api/areas/7/insights", "/api/recommendations"} {
		var response struct {
			Warnings  []string
			Freshness string
		}
		if err := json.Unmarshal(read(path).Body.Bytes(), &response); err != nil || response.Freshness != "stale" || len(response.Warnings) != len(before.Warnings)+1 {
			t.Fatalf("stale response warnings are incorrect: %v", err)
		}
	}
	if len(service.CurrentRecommendation(now).Warnings) != len(before.Warnings) || before.Freshness != "fresh" {
		t.Fatal("response metadata changed the cached result")
	}
	forecast.view.Store(&view)
	var workers sync.WaitGroup
	failures := make(chan error, 5)
	for worker := 0; worker < 5; worker++ {
		workers.Add(1)
		go func(writer bool) {
			defer workers.Done()
			for i := 0; i < 20; i++ {
				if writer {
					if err := service.ComputeAndPublish(context.Background(), now); err != nil {
						failures <- err
						return
					}
				} else if response := read("/api/areas/7/insights"); response.Code != 200 {
					failures <- fmt.Errorf("concurrent insights returned %d", response.Code)
					return
				} else if response := read("/api/recommendations"); response.Code != 200 {
					failures <- fmt.Errorf("concurrent summary returned %d", response.Code)
					return
				}
			}
		}(worker == 0)
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if len(service.CurrentRecommendation(now).Areas[0].Hourly) != 2 {
		t.Fatal("concurrent reads changed cached evidence")
	}
	changed := view
	changed.Version = "second"
	forecast.view.Store(&changed)
	if !service.NeedsRecompute(now) || service.CurrentRecommendation(now).Status != "unavailable" || read("/api/recommendations").Code != 503 {
		t.Fatal("mismatched forecast version exposed an old action")
	}
	if err := service.ComputeAndPublish(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if service.CurrentRecommendation(now).ForecastVersion != "second" || before.ForecastVersion != "first" {
		t.Fatal("refresh modified a previously published result")
	}
	if service.CurrentRecommendation(before.ValidUntil).Status != "unavailable" || !service.NeedsRecompute(before.ValidUntil) {
		t.Fatal("expired result remained usable")
	}
	forecast.view.Store(nil)
	if service.CurrentRecommendation(now).Status != "unavailable" || !service.NeedsRecompute(now) {
		t.Fatal("missing weather exposed a cached action")
	}
	if err := service.ComputeAndPublish(context.Background(), now); err == nil || service.NeedsRecompute(now) {
		t.Fatal("unavailable state was not recorded")
	}
}
