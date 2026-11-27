// Package history loads and indexes historical trip evidence.
package history

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"mobilitycontrolroom/internal/domain"
)

// Areas supplies the monitored zones used to validate historical evidence.
type Areas interface {
	ListAreas(context.Context) ([]domain.Area, error)
}

// Statistics supplies trip aggregates split by a rainfall threshold in millimeters.
type Statistics interface {
	AggregateHourly(context.Context, float64) ([]domain.HourlyStats, error)
}

// Service provides read-only historical evidence for monitored zones.
type Service struct {
	areas      []domain.Area
	statistics historicalSnapshot
	location   *time.Location
}

// Load reads and indexes historical evidence for the configured zones.
func Load(ctx context.Context, areas Areas, historical Statistics, minRainfallMM float64, loc *time.Location) (*Service, error) {
	if loc == nil {
		return nil, fmt.Errorf("historical lookup requires a timezone")
	}
	if minRainfallMM <= 0 || math.IsNaN(minRainfallMM) || math.IsInf(minRainfallMM, 0) {
		return nil, fmt.Errorf("historical rainfall threshold must be finite and positive")
	}
	scope, err := areas.ListAreas(ctx)
	if err != nil {
		return nil, fmt.Errorf("load historical scope: %w", err)
	}
	rows, err := historical.AggregateHourly(ctx, minRainfallMM)
	if err != nil {
		return nil, fmt.Errorf("load historical statistics: %w", err)
	}
	snapshot, err := newHistorical(ctx, scope, rows)
	if err != nil {
		return nil, fmt.Errorf("build historical evidence: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Service{slices.Clone(scope), snapshot, loc}, nil
}

// Areas returns a caller-owned copy of the monitored zones.
func (s *Service) Areas() []domain.Area {
	return slices.Clone(s.areas)
}

// LookupComparableHour finds historical evidence for the local weekday and hour.
func (s *Service) LookupComparableHour(areaID int, at time.Time) (domain.HourlyStats, bool) {
	day, hour := domain.LocalWeekdayAndHour(at, s.location)
	return s.statistics.get(historicalKey{areaID, day, hour})
}

// Coverage summarizes historical support at the given sample threshold.
func (s *Service) Coverage(minSamples int64) Coverage {
	return s.statistics.coverage(minSamples)
}
