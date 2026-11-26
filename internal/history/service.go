// Package history loads immutable historical evidence through domain-oriented
// repository contracts. It has no database driver or SQL dependency.
package history

import (
	"context"
	"fmt"
	"math"
	"mobilitycontrolroom/internal/cache"
	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/repository"
	"time"
)

type Service struct {
	areas      []repository.Area
	statistics *cache.HistoricalSnapshot
	location   *time.Location
}

func Load(ctx context.Context, areas repository.AreaRepository, historical repository.HistoricalRepository, q repository.HistoricalQuery, loc *time.Location) (*Service, error) {
	if loc == nil {
		return nil, fmt.Errorf("historical lookup requires a timezone")
	}

	if q.MinRainfallMM <= 0 || math.IsNaN(q.MinRainfallMM) || math.IsInf(q.MinRainfallMM, 0) {
		return nil, fmt.Errorf("historical rainfall threshold must be finite and positive")
	}

	scope, err := areas.ListAreas(ctx)
	if err != nil {
		return nil, fmt.Errorf("load historical scope: %w", err)
	}
	rows, err := historical.AggregateHourly(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("load historical statistics: %w", err)
	}
	snapshot, err := cache.NewHistorical(scope, rows)
	if err != nil {
		return nil, fmt.Errorf("build historical snapshot: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return &Service{append([]repository.Area(nil), scope...), snapshot, loc}, nil
}

func (s *Service) Areas() []repository.Area {
	return append([]repository.Area(nil), s.areas...)
}

func (s *Service) LookupComparableHour(areaID int, at time.Time) (repository.HourlyStats, bool) {
	day, hour := domain.LocalWeekdayAndHour(at, s.location)
	return s.statistics.Get(cache.HistoricalKey{AreaID: areaID, DayOfWeek: day, HourOfDay: hour})
}

func (s *Service) Coverage(minSamples int64) cache.HistoricalCoverage {
	return s.statistics.Coverage(minSamples)
}
