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

	defer rows.Close()
	areas := make([]repository.Area, 0)

	for rows.Next() {
		var area repository.Area
		if err = rows.Scan(&area.ID, &area.Name, &area.Latitude, &area.Longitude); err != nil {
			return nil, fmt.Errorf("scan area: %w", err)
		}
		areas = append(areas, area)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read areas: %w", err)
	}

	return areas, nil
}

func validQuery(q repository.HistoricalQuery) error {
	if q.MinRainfallMM <= 0 || math.IsNaN(q.MinRainfallMM) || math.IsInf(q.MinRainfallMM, 0) {
		return fmt.Errorf("rainfall threshold must be finite and positive")
	}
	return nil
}

func (r *Repository) AggregateHourly(ctx context.Context, q repository.HistoricalQuery) ([]repository.HourlyStats, error) {
	if err := validQuery(q); err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, historicalSQL, q.MinRainfallMM)
	if err != nil {
		return nil, fmt.Errorf("aggregate historical hours: %w", err)
	}

	defer rows.Close()
	statistics := make([]repository.HourlyStats, 0)

	for rows.Next() {
		var s repository.HourlyStats
		if err = rows.Scan(&s.AreaID, &s.Season, &s.DayOfWeek, &s.HourOfDay, &s.DryMean, &s.DrySamples, &s.RainMean, &s.RainSamples, &s.RainStdDev, &s.AbsoluteUplift, &s.PercentageUplift); err != nil {
			return nil, fmt.Errorf("scan historical group: %w", err)
		}
		statistics = append(statistics, s)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read historical groups: %w", err)
	}

	return statistics, nil
}

// ExplainHistorical is an adapter-specific inspection tool, not a business-layer
// repository method. It executes the identical SQL used by AggregateHourly.
func (r *Repository) ExplainHistorical(ctx context.Context, q repository.HistoricalQuery) ([]string, error) {
	if err := validQuery(q); err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+historicalSQL, q.MinRainfallMM)
	if err != nil {
		return nil, fmt.Errorf("explain historical query: %w", err)
	}

	defer rows.Close()
	plan := []string{}

	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			return nil, fmt.Errorf("scan historical plan: %w", err)
		}
		plan = append(plan, line)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read historical plan: %w", err)
	}

	return plan, nil
}
