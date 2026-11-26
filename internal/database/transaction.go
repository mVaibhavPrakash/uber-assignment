package database

import (
	"context"

	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Begin starts a transaction and names the operation if the database call fails.
func Begin(ctx context.Context, pool *pgxpool.Pool, operation string) (pgx.Tx, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin %s: %w", operation, err)
	}
	return tx, nil
}
