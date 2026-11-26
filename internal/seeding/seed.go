// Package seeding validates prepared JSON and initializes the database.
package seeding

import (
	"context"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Reads the prepared JSON and inserts all records in one database transaction.
func Seed(ctx context.Context, pool *pgxpool.Pool, dir string) (Summary, error) {
	header, err := SeedHeader(dir)
	if err != nil {
		return Summary{}, err
	}
	var summary Summary
	err = seedTransaction(ctx, pool, func(writer seedWriter) error {
		if err := writer.CopyAreas(ctx, header.Areas); err != nil {
			return err
		}
		var err error
		summary, err = copyObservations(ctx, writer, filepath.Join(dir, "seed.json"))
		return err
	})
	return summary, err
}
