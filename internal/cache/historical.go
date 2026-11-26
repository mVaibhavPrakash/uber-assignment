// Package cache contains immutable typed runtime snapshots.
package cache

import (
	"fmt"
	"mobilitycontrolroom/internal/repository"
)

type HistoricalKey struct {
	AreaID, Season, DayOfWeek, HourOfDay int
}

type HistoricalSnapshot struct {
	statistics map[HistoricalKey]repository.HourlyStats
}

type HistoricalCoverage struct {
	Groups, DrySupportedGroups, RainSupportedGroups int
}

// NewHistorical indexes trusted database aggregates. The maximum size is
// bounded by the seeded area's seasonal/day/hour combinations. Caller-owned
// slices and nullable pointers are copied both on insertion and lookup.
func NewHistorical(areas []repository.Area, rows []repository.HourlyStats) (*HistoricalSnapshot, error) {
	known := make(map[int]bool, len(areas))
	for _, area := range areas {
		if area.ID <= 0 || known[area.ID] {
			return nil, fmt.Errorf("invalid or duplicate area %d", area.ID)
		}
		known[area.ID] = true
	}
	if len(known) == 0 {
		return nil, fmt.Errorf("cannot build history without seeded areas")
	}
	if len(rows) > len(known)*4*7*24 {
		return nil, fmt.Errorf("historical snapshot exceeds calendar bound")
	}
	snapshot := &HistoricalSnapshot{statistics: make(map[HistoricalKey]repository.HourlyStats, len(rows))}
	for _, row := range rows {
		key := HistoricalKey{row.AreaID, row.Season, row.DayOfWeek, row.HourOfDay}
		if !known[row.AreaID] || row.Season < 1 || row.Season > 4 || row.DayOfWeek < 1 || row.DayOfWeek > 7 || row.HourOfDay < 0 || row.HourOfDay > 23 {
			return nil, fmt.Errorf("invalid historical calendar key %+v", key)
		}
		if _, exists := snapshot.statistics[key]; exists {
			return nil, fmt.Errorf("duplicate historical calendar key %+v", key)
		}
		snapshot.statistics[key] = copyStats(row)
	}
	return snapshot, nil
}

func copyStats(row repository.HourlyStats) repository.HourlyStats {
	row.DryMean = copyFloat(row.DryMean)
	row.RainMean = copyFloat(row.RainMean)
	row.AbsoluteUplift = copyFloat(row.AbsoluteUplift)
	row.PercentageUplift = copyFloat(row.PercentageUplift)
	row.RainStdDev = copyFloat(row.RainStdDev)
	return row
}

func copyFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyData := *value
	return &copyData
}

func (s *HistoricalSnapshot) Get(key HistoricalKey) (repository.HourlyStats, bool) {
	row, ok := s.statistics[key]
	if !ok {
		return repository.HourlyStats{}, false
	}
	return copyStats(row), true
}

func (s *HistoricalSnapshot) Len() int {
	return len(s.statistics)
}

func (s *HistoricalSnapshot) Coverage(minSamples int64) HistoricalCoverage {
	if minSamples < 1 {
		minSamples = 1
	}
	result := HistoricalCoverage{Groups: len(s.statistics)}
	for _, row := range s.statistics {
		if row.DrySamples >= minSamples {
			result.DrySupportedGroups++
		}
		if row.DrySamples >= minSamples && row.RainSamples >= minSamples {
			result.RainSupportedGroups++
		}
	}
	return result
}
