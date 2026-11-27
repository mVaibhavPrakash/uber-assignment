package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open owns connection setup; the caller must Close the returned pool after
// stopping handlers and workers. The startup schema check also verifies connectivity.
func Open(ctx context.Context, dsn string, timeout time.Duration) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	cfg.MaxConns = 4
	cfg.ConnConfig.ConnectTimeout = timeout
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	const weatherSchema = `
		SELECT count(*) = 4
		FROM pg_attribute
		WHERE attrelid IN (to_regclass('weather_forecasts'), to_regclass('historical_observations'))
		  AND attname IN ('rain_mm', 'snowfall_cm')
		  AND atttypid = 'double precision'::regtype AND NOT attisdropped`
	var ready bool
	if err = pool.QueryRow(checkCtx, weatherSchema).Scan(&ready); err != nil {
		pool.Close()
		return nil, fmt.Errorf("check weather schema: %w", err)
	}
	if !ready {
		pool.Close()
		return nil, fmt.Errorf("weather schema requires double precision rain and snowfall columns; run setup for a fresh database or explicitly reset and reseed a disposable development database")
	}
	return pool, nil
}
