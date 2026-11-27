package history

import (
	"context"
	"fmt"

	"mobilitycontrolroom/internal/domain"
)

type historicalKey struct {
	AreaID, DayOfWeek, HourOfDay int
}

type historicalSnapshot map[historicalKey]domain.HourlyStats

// Coverage counts historical groups with enough dry and rainy observations.
type Coverage struct {
	Groups, DrySupportedGroups, RainSupportedGroups int
}

// newHistorical indexes trusted database aggregates. The maximum size is
// bounded by the seeded area's weekday/hour combinations. Caller-owned
// slices and nullable pointers are copied both on insertion and lookup.
func newHistorical(ctx context.Context, areas []domain.Area, rows []domain.HourlyStats) (historicalSnapshot, error) {
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
	if len(rows) > len(known)*7*24 {
		return nil, fmt.Errorf("historical snapshot exceeds calendar bound")
	}
	snapshot := make(historicalSnapshot, len(rows))
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := historicalKey{AreaID: row.AreaID, DayOfWeek: row.DayOfWeek, HourOfDay: row.HourOfDay}
		if !known[row.AreaID] || row.DayOfWeek < 1 || row.DayOfWeek > 7 || row.HourOfDay < 0 || row.HourOfDay > 23 {
			return nil, fmt.Errorf("invalid historical calendar key %+v", key)
		}
		if _, exists := snapshot[key]; exists {
			return nil, fmt.Errorf("duplicate historical calendar key %+v", key)
		}
		snapshot[key] = cloneHourlyStats(row)
	}
	return snapshot, nil
}

func cloneHourlyStats(row domain.HourlyStats) domain.HourlyStats {
	row.DryMean = cloneFloatPointer(row.DryMean)
	row.RainMean = cloneFloatPointer(row.RainMean)
	row.AbsoluteUplift = cloneFloatPointer(row.AbsoluteUplift)
	row.PercentageUplift = cloneFloatPointer(row.PercentageUplift)
	row.RainStdDev = cloneFloatPointer(row.RainStdDev)
	return row
}

// Copy an existing number so changing it cannot change the cached value.
func cloneFloatPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyData := *value
	return &copyData
}

func (s historicalSnapshot) get(key historicalKey) (domain.HourlyStats, bool) {
	row, ok := s[key]
	return cloneHourlyStats(row), ok
}

func (s historicalSnapshot) coverage(minSamples int64) Coverage {
	if minSamples < 1 {
		minSamples = 1
	}
	result := Coverage{Groups: len(s)}
	for _, row := range s {
		if row.DrySamples >= minSamples {
			result.DrySupportedGroups++
		}
		if row.DrySamples >= minSamples && row.RainSamples >= minSamples {
			result.RainSupportedGroups++
		}
	}
	return result
}
