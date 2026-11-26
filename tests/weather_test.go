package tests

import (
	"context"
	"errors"
	"mobilitycontrolroom/internal/apperrors"
	"mobilitycontrolroom/internal/config"
	"mobilitycontrolroom/internal/repository"
	"mobilitycontrolroom/internal/weather"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type forecastScope []repository.Area

func (a forecastScope) ListAreas(context.Context) ([]repository.Area, error) { return a, nil }

type forecastStore struct {
	snapshot repository.ForecastSnapshot
	writes   int
}

func (s *forecastStore) LoadLatest(context.Context) (repository.ForecastSnapshot, error) {
	return s.snapshot, nil
}
func (s *forecastStore) ReplaceSnapshot(_ context.Context, snapshot repository.ForecastSnapshot) error {
	s.writes++
	s.snapshot = snapshot
	return nil
}

func TestExternalAPIFailureKeepsLastForecast(t *testing.T) {
	for _, present := range []bool{false, true} {
		if _, err := config.Parse(func(key string) (string, bool) { return "", present && key == "DATABASE_URL" }); err == nil || err.Error() != "DATABASE_URL is required" {
			t.Fatalf("missing or empty DATABASE_URL must fail explicitly, got %v", err)
		}
	}
	config, err := config.Parse(func(key string) (string, bool) {
		if key == "DATABASE_URL" {
			return "postgres://localhost/test", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	config.RetryDelay = time.Millisecond
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	config.ForecastURL = server.URL
	client, err := weather.NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 25, 10, 20, 0, 0, time.UTC)
	scope := forecastScope{{ID: 7, Name: "Astoria", Latitude: 40.7615, Longitude: -73.9239}}
	store := &forecastStore{snapshot: repository.ForecastSnapshot{Version: "11111111-1111-4111-8111-111111111111", FetchedAt: now}}
	for i := 0; i < 30; i++ {
		store.snapshot.Hours = append(store.snapshot.Hours, repository.ForecastHour{AreaID: 7, HourStart: now.Truncate(time.Hour).Add(time.Duration(i) * time.Hour), RainMM: 1, PrecipitationProbability: 90})
	}
	service := weather.NewService(scope, store, client, config, func() time.Time { return now })
	if err := service.RestoreSavedForecast(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.FetchAndPublishForecast(context.Background()); err == nil {
		t.Fatal("provider outage was accepted")
	}
	if calls.Load() != int32(config.MaxAttempts) || store.writes != 0 {
		t.Fatalf("outage must use bounded retries without replacing data: calls=%d writes=%d", calls.Load(), store.writes)
	}
	view, err := service.CurrentForecast(now.Add(3 * time.Hour))
	if err != nil || view.Snapshot == nil || view.Version != store.snapshot.Version || view.Freshness != "stale" {
		t.Fatalf("last usable forecast was lost: %+v %v", view, err)
	}
	expired, err := service.CurrentForecast(now.Add(6*time.Hour + time.Second))
	if !errors.Is(err, apperrors.ErrForecastExpired) || expired.Snapshot != nil {
		t.Fatal("expired forecast remained usable during outage")
	}
}
