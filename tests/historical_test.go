package tests

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"mobilitycontrolroom/database/migrations"
	"mobilitycontrolroom/internal/repository"
	"mobilitycontrolroom/internal/repository/postgres"
	"os"
	"testing"
	"time"
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
 INSERT INTO historical_observations(area_id,hour_start,local_date,day_of_week,season,hour_of_day,trip_count,rain_mm,snowfall_cm) VALUES
 (7,'2022-12-03 15:00Z','2022-12-03',6,1,10,1100,0,0),
 (7,'2022-12-10 15:00Z','2022-12-10',6,1,10,1300,0,0),
 (7,'2023-01-07 15:00Z','2023-01-07',6,1,10,1500,1,0),
 (7,'2023-02-04 15:00Z','2023-02-04',6,1,10,1900,0.5,0),
 (7,'2022-12-17 15:00Z','2022-12-17',6,1,10,9000,1,1),
 (7,'2022-12-24 15:00Z','2022-12-24',6,1,10,8000,0.1,0);`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := postgres.New(pool).AggregateHourly(ctx, repository.HistoricalQuery{MinRainfallMM: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one comparable group, got %d", len(rows))
	}
	stats := rows[0]
	if stats.DrySamples != 2 || stats.RainSamples != 2 {
		t.Fatalf("snow or trace rain entered comparison: %+v", stats)
	}
	if stats.DryMean == nil || *stats.DryMean != 1200 || stats.RainMean == nil || *stats.RainMean != 1700 || stats.AbsoluteUplift == nil || *stats.AbsoluteUplift != 500 {
		t.Fatalf("incorrect completed-trip comparison: %+v", stats)
	}
	if stats.PercentageUplift == nil || math.Abs(*stats.PercentageUplift-41.6666667) > 1e-6 || stats.RainStdDev == nil || math.Abs(*stats.RainStdDev-282.8427125) > 1e-6 {
		t.Fatalf("incorrect explanatory evidence: %+v", stats)
	}
}
