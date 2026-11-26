package seeding

import (
	"context"
	"fmt"
	"mobilitycontrolroom/internal/apperrors"

	"mobilitycontrolroom/internal/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Applies the schema to an empty database and COPYs prepared observations in
// one transaction. JSON validation errors and COPY failures rollback
// areas, observations and any newly created schema together.
func seedTransaction(ctx context.Context, pool *pgxpool.Pool, write func(seedWriter) error) (err error) {
	tx, err := database.Begin(ctx, pool, "seed")
	if err != nil {
		return err
	}
	defer apperrors.Rollback(tx, "seed", &err)
	// Step 1: Prepare an empty schema without changing existing data.
	if err = prepareSeedSchema(ctx, tx); err != nil {
		return err
	}
	// Step 2: Copy validated seed records through this transaction.
	if err = write(seedWriter{tx: tx}); err != nil {
		return err
	}
	// Step 3: Commit both tables together, then update database statistics.
	if err = apperrors.CommitSeed(ctx, tx); err != nil {
		return err
	}
	return analyzeSeedTables(ctx, pool)
}

// analyzeSeedTables updates query-planning statistics after the data is committed.
func analyzeSeedTables(ctx context.Context, pool *pgxpool.Pool) error {
	const command = `ANALYZE community_areas, historical_observations, weather_forecasts`
	if _, err := pool.Exec(ctx, command); err != nil {
		return fmt.Errorf("seed committed but ANALYZE failed: %w", err)
	}
	return nil
}
