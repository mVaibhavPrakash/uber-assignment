package tests

import (
	"context"
	"fmt"
	"math"
	"mobilitycontrolroom/database/migrations"
	"mobilitycontrolroom/internal/history"
	"mobilitycontrolroom/internal/repository"
	"mobilitycontrolroom/internal/repository/postgres"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHistoricalRainAndDryEvidence(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for the PostgreSQL business-evidence test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("business_evidence_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, migrations.Up); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO community_areas VALUES(7,'Astoria',40.7615,-73.9239);
		INSERT INTO historical_observations(area_id,hour_start,local_date,day_of_week,hour_of_day,trip_count,rain_mm,snowfall_cm) VALUES
		(7,'2022-12-03 15:00Z','2022-12-03',6,10,1100,0,0),
		(7,'2023-04-08 14:00Z','2023-04-08',6,10,1300,0,0),
		(7,'2023-01-07 15:00Z','2023-01-07',6,10,1500,1,0),
		(7,'2023-08-05 14:00Z','2023-08-05',6,10,1900,0.5,0),
		(7,'2022-12-17 15:00Z','2022-12-17',6,10,9000,1,1),
		(7,'2022-12-24 15:00Z','2022-12-24',6,10,1200,0.1,0),
		(7,'2022-12-31 15:00Z','2022-12-31',6,10,1200,0.4,0),
		(7,'2023-04-10 14:00Z','2023-04-10',1,10,600,0,0),
		(7,'2023-04-08 15:00Z','2023-04-08',6,11,900,0,0);`)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	adapter := postgres.New(pool)
	evidence, err := history.Load(ctx, adapter, adapter, repository.HistoricalQuery{MinRainfallMM: 0.5}, loc)
	if err != nil {
		t.Fatal(err)
	}
	if groups := evidence.Coverage(2).Groups; groups != 3 {
		t.Fatalf("expected three day and hour groups across all seasons, got %d", groups)
	}
	winter := time.Date(2023, time.February, 11, 10, 0, 0, 0, loc)
	summer := time.Date(2023, time.August, 12, 10, 0, 0, 0, loc)
	for _, at := range []time.Time{winter.UTC(), summer.UTC()} {
		stats, ok := evidence.LookupComparableHour(7, at)
		if !ok {
			t.Fatalf("no pooled history for %s", at)
		}
		assertHistoricalComparison(t, stats)
	}
	for _, check := range []struct {
		at   time.Time
		mean float64
	}{{summer.AddDate(0, 0, 2), 600}, {summer.Add(time.Hour), 900}} {
		stats, ok := evidence.LookupComparableHour(7, check.at.UTC())
		if !ok || stats.DrySamples != 1 || stats.DryMean == nil || *stats.DryMean != check.mean || stats.RainSamples != 0 {
			t.Fatalf("wrong day or hour selected for %s: %+v", check.at, stats)
		}
	}
}

func assertHistoricalComparison(t *testing.T, stats repository.HourlyStats) {
	t.Helper()
	if stats.DrySamples != 4 || stats.RainSamples != 2 {
		t.Fatalf("baseline must include light rain; qualifying rain stays separate and snow excluded: %+v", stats)
	}
	if stats.DryMean == nil || *stats.DryMean != 1200 || stats.RainMean == nil || *stats.RainMean != 1700 || stats.AbsoluteUplift == nil || *stats.AbsoluteUplift != 500 {
		t.Fatalf("incorrect completed-trip comparison: %+v", stats)
	}
	if stats.PercentageUplift == nil || math.Abs(*stats.PercentageUplift-41.6666667) > 1e-6 || stats.RainStdDev == nil || math.Abs(*stats.RainStdDev-282.8427125) > 1e-6 {
		t.Fatalf("incorrect explanatory evidence: %+v", stats)
	}
}
