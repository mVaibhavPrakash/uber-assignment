package tests

import (
	"context"
	"fmt"
	"math"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mobilitycontrolroom/database/migrations"
	"mobilitycontrolroom/internal/domain"
	scoring "mobilitycontrolroom/internal/domain/recommendation"
	"mobilitycontrolroom/internal/repository/postgres"
	"mobilitycontrolroom/internal/usecase/history"
	"mobilitycontrolroom/internal/usecase/recommendation"
	"mobilitycontrolroom/internal/usecase/weather"
)

func openTestDB(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for the PostgreSQL integration test")
	}
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("business_evidence_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, migrations.Up); err != nil {
		t.Fatal(err)
	}
	return pool
}

type fixedForecast domain.ForecastSnapshot

func (f fixedForecast) Fetch(context.Context, []domain.Area, time.Time) (domain.ForecastSnapshot, error) {
	return domain.ForecastSnapshot(f), nil
}

func TestForecastRoundTripPreservesRainfallThreshold(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	pool := openTestDB(t, ctx)
	repo := postgres.New(pool)
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.January, 28, 11, 20, 0, 0, location).UTC()
	const threshold = 0.5
	rainfall := []float64{math.Nextafter(threshold, 0), threshold, math.Nextafter(threshold, 1)}
	snapshot := domain.ForecastSnapshot{Version: "11111111-1111-4111-8111-111111111111"}

	if _, err := pool.Exec(ctx, `INSERT INTO community_areas VALUES (7, 'Astoria', 40.7615, -73.9239)`); err != nil {
		t.Fatal(err)
	}
	for i, rain := range rainfall {
		at := domain.NextFullHour(now).Add(time.Duration(i) * time.Hour)
		snapshot.Hours = append(snapshot.Hours, domain.ForecastHour{
			AreaID: 7, HourStart: at, RainMM: rain, PrecipitationProbability: 100,
		})
		for sample, observedRain := range rainfall {
			observedAt := at.In(location).AddDate(0, 0, -7*(sample+1))
			day, hour := domain.LocalWeekdayAndHour(observedAt, location)
			_, err := pool.Exec(ctx, `INSERT INTO historical_observations
				(area_id, hour_start, local_date, day_of_week, hour_of_day, trip_count, rain_mm, snowfall_cm)
				VALUES (7, $1, $2::text::date, $3, $4, $5, $6, 0)`,
				observedAt, observedAt.Format(time.DateOnly), day, hour, 100+sample*200, observedRain)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	evidence, err := history.Load(ctx, repo, repo, threshold, location)
	if err != nil {
		t.Fatal(err)
	}
	settings := scoring.Options{Window: 3 * time.Hour, MinSamples: 1, RainThreshold: threshold, AdditionalPerHour: 50, Location: location}
	weatherSettings := weather.Options{Window: settings.Window, RefreshInterval: 3 * time.Hour, MaxStaleness: 6 * time.Hour}
	clock := func() time.Time { return now }
	live := weather.NewService(repo, repo, fixedForecast(snapshot), weatherSettings, clock)
	if err := live.FetchAndPublishForecast(ctx); err != nil {
		t.Fatal(err)
	}
	restored := weather.NewService(repo, repo, nil, weatherSettings, clock)
	if err := restored.RestoreSavedForecast(ctx); err != nil {
		t.Fatal(err)
	}

	score := func(forecasts *weather.Service) scoring.Result {
		t.Helper()
		manager, err := recommendation.NewManager(evidence, forecasts, settings, weatherSettings.MaxStaleness)
		if err != nil {
			t.Fatal(err)
		}
		if err := manager.ComputeAndPublish(ctx, now); err != nil {
			t.Fatal(err)
		}
		return manager.CurrentRecommendation(now)
	}
	liveResult, restoredResult := score(live), score(restored)
	if liveResult.Status != "action_required" || liveResult.Primary == nil || len(liveResult.Primary.Hourly) != len(rainfall) {
		t.Fatalf("expected an actionable three-hour recommendation, got %+v", liveResult)
	}
	if len(restoredResult.Areas) != 1 || len(restoredResult.Areas[0].Hourly) != len(rainfall) {
		t.Fatalf("restored recommendation is missing hourly evidence: %+v", restoredResult)
	}
	for i, want := range []float64{0, 300, 300} {
		hour := liveResult.Primary.Hourly[i]
		if hour.Forecast == nil || hour.Forecast.RainMM != rainfall[i] ||
			hour.DrySamples != 1 || hour.RainSamples != 2 || hour.Additional == nil || *hour.Additional != want {
			t.Fatalf("rainfall %.17g: want 1 dry sample, 2 rainy samples and %g additional trips, got %+v", rainfall[i], want, hour)
		}
		restoredHour := restoredResult.Areas[0].Hourly[i]
		if restoredHour.Forecast == nil || restoredHour.Forecast.RainMM != rainfall[i] {
			t.Fatalf("rainfall %.17g did not survive PostgreSQL round trip: %+v", rainfall[i], restoredHour.Forecast)
		}
	}
	if !reflect.DeepEqual(liveResult, restoredResult) {
		t.Fatal("recommendation changed after restoring the same forecast")
	}
}
