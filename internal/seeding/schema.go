package seeding

import (
	"context"
	"fmt"

	"mobilitycontrolroom/database/migrations"

	"github.com/jackc/pgx/v5"
)

// Prevents concurrent setup, then creates or checks the tables.
func prepareSeedSchema(ctx context.Context, tx pgx.Tx) error {
	if err := lockSeed(ctx, tx); err != nil {
		return err
	}
	tables, err := countSeedTables(ctx, tx)
	if err != nil {
		return err
	}
	switch tables {
	case 0:
		return applySeedMigration(ctx, tx)
	case 3:
		return requireEmptySeedTables(ctx, tx)
	default:
		return fmt.Errorf("partial schema found; expected zero or three assignment tables")
	}
}

// lockSeed allows only one seed transaction to initialize the tables at a time.
func lockSeed(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(73192027)")
	if err != nil {
		return fmt.Errorf("lock seed: %w", err)
	}
	return nil
}

// countSeedTables checks whether the assignment schema exists in this database.
func countSeedTables(ctx context.Context, tx pgx.Tx) (int, error) {
	const query = `
 SELECT COUNT(*)
 FROM information_schema.tables
 WHERE table_schema = current_schema()
   AND table_name IN (
     'community_areas', 'historical_observations', 'weather_forecasts'
   )`
	var count int
	if err := tx.QueryRow(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("inspect schema: %w", err)
	}
	return count, nil
}

// applySeedMigration creates all assignment tables in the seed transaction.
func applySeedMigration(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, migrations.Up); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}
	return nil
}

// requireEmptySeedTables stops setup from adding a second dataset to existing data.
func requireEmptySeedTables(ctx context.Context, tx pgx.Tx) error {
	const query = `
 SELECT EXISTS (SELECT 1 FROM community_areas)
     OR EXISTS (SELECT 1 FROM historical_observations)
     OR EXISTS (SELECT 1 FROM weather_forecasts)`
	var occupied bool
	if err := tx.QueryRow(ctx, query).Scan(&occupied); err != nil {
		return fmt.Errorf("inspect seed state: %w", err)
	}
	if occupied {
		return fmt.Errorf("database already contains data; use explicit -reset only for the disposable assignment database")
	}
	return nil
}
