package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/infra/openmeteo"
	"mobilitycontrolroom/internal/usecase/weather"
)

type forecastScope []domain.Area

func (a forecastScope) ListAreas(context.Context) ([]domain.Area, error) { return a, nil }

type forecastStore struct {
	snapshot domain.ForecastSnapshot
	writes   int
}

func (s *forecastStore) LoadLatest(context.Context) (domain.ForecastSnapshot, error) {
	return s.snapshot, nil
}

func (s *forecastStore) ReplaceSnapshot(_ context.Context, snapshot domain.ForecastSnapshot) error {
	s.writes++
	s.snapshot = snapshot
	return nil
}

func TestExternalAPIFailureKeepsLastForecast(t *testing.T) {
	const attempts = 3
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client, err := openmeteo.NewClient(*endpoint, server.Client(), openmeteo.Options{
		ForecastHours: 30, MaxAttempts: attempts, AttemptTimeout: time.Second,
		RequestTimeout: 3 * time.Second, RetryDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 25, 10, 20, 0, 0, time.UTC)
	scope := forecastScope{{ID: 7, Name: "Astoria", Latitude: 40.7615, Longitude: -73.9239}}
	store := &forecastStore{snapshot: domain.ForecastSnapshot{Version: "11111111-1111-4111-8111-111111111111", FetchedAt: now}}
	for i := 0; i < 30; i++ {
		store.snapshot.Hours = append(store.snapshot.Hours, domain.ForecastHour{AreaID: 7, HourStart: now.Truncate(time.Hour).Add(time.Duration(i) * time.Hour), RainMM: 1, PrecipitationProbability: 90})
	}
	service := weather.NewService(scope, store, client, weather.Options{
		Window: 2 * time.Hour, RefreshInterval: 3 * time.Hour, MaxStaleness: 6 * time.Hour,
	}, func() time.Time { return now })
	if err := service.RestoreSavedForecast(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := service.FetchAndPublishForecast(t.Context()); err == nil {
		t.Fatal("provider outage was accepted")
	}
	if calls.Load() != attempts || store.writes != 0 {
		t.Fatalf("outage must use bounded retries without replacing data: calls=%d writes=%d", calls.Load(), store.writes)
	}
	view, err := service.CurrentForecast(now.Add(3 * time.Hour))
	if err != nil || view.Snapshot == nil || view.Version != store.snapshot.Version || view.Freshness != "stale" {
		t.Fatalf("last usable forecast was lost: %+v %v", view, err)
	}
	expired, err := service.CurrentForecast(now.Add(6*time.Hour + time.Second))
	if !errors.Is(err, domain.ErrForecastExpired) || expired.Snapshot != nil {
		t.Fatal("expired forecast remained usable during outage")
	}
}
