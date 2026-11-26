// Package postgres implements the runtime repository contracts using PostgreSQL.
package postgres

import (
	"context"
	_ "embed"
	"fmt"
	"math"

	"mobilitycontrolroom/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed historical.sql
var historicalSQL string

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool} }

var _ repository.AreaRepository = (*Repository)(nil)
var _ repository.HistoricalRepository = (*Repository)(nil)

func (r *Repository) ListAreas(ctx context.Context) ([]repository.Area, error) {
	return listAreas(ctx, r.pool)
}

// areaReader lets the same query run on a pool or inside a locked transaction.
type areaReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// listAreas is the single place that reads zone details from PostgreSQL.
func listAreas(ctx context.Context, reader areaReader) ([]repository.Area, error) {
	const query = `
		SELECT id, name, latitude, longitude
		FROM community_areas
		ORDER BY id`
	rows, err := reader.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list areas: %w", err)
	}

	return collectRows(rows, "area", "areas", func(area *repository.Area) error {
		return rows.Scan(&area.ID, &area.Name, &area.Latitude, &area.Longitude)
	})
}

func validateHistoricalQuery(q repository.HistoricalQuery) error {
	if q.MinRainfallMM <= 0 || math.IsNaN(q.MinRainfallMM) || math.IsInf(q.MinRainfallMM, 0) {
		return fmt.Errorf("rainfall threshold must be finite and positive")
	}
	return nil
}

func (r *Repository) AggregateHourly(ctx context.Context, q repository.HistoricalQuery) ([]repository.HourlyStats, error) {
	if err := validateHistoricalQuery(q); err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, historicalSQL, q.MinRainfallMM)
	if err != nil {
		return nil, fmt.Errorf("aggregate historical hours: %w", err)
	}

	return collectRows(rows, "historical group", "historical groups", func(s *repository.HourlyStats) error {
		return rows.Scan(&s.AreaID, &s.DayOfWeek, &s.HourOfDay, &s.DryMean, &s.DrySamples, &s.RainMean, &s.RainSamples, &s.RainStdDev, &s.AbsoluteUplift, &s.PercentageUplift)
	})
}

// Keep row cleanup and scan/read errors consistent across repository queries.
func collectRows[T any](rows pgx.Rows, scanLabel, readLabel string, scan func(*T) error) ([]T, error) {
	defer rows.Close()
	values := make([]T, 0)
	for rows.Next() {
		var value T
		if err := scan(&value); err != nil {
			return nil, fmt.Errorf("scan %s: %w", scanLabel, err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", readLabel, err)
	}
	return values, nil
}
