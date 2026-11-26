package domain

import (
	"fmt"
	"github.com/google/uuid"
	"math"
	"mobilitycontrolroom/internal/repository"
	"time"
)

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// ValidateForecast requires a complete rectangular scope and contiguous UTC
// hours. It validates every write independently of database constraints.
func ValidateForecast(s repository.ForecastSnapshot, areas []repository.Area) error {
	version, err := uuid.Parse(s.Version)
	if err != nil || version.String() != s.Version || version == uuid.Nil {
		return fmt.Errorf("forecast version must be a canonical nonzero UUID")
	}
	if s.FetchedAt.IsZero() || len(s.Hours) == 0 || len(areas) == 0 {
		return fmt.Errorf("forecast metadata/scope is empty")
	}
	known, err := forecastAreaIDs(areas)
	if err != nil {
		return err
	}
	periods, err := collectForecastPeriods(s.Hours, known)
	if err != nil {
		return err
	}
	return validateForecastCoverage(periods, areas)
}

// validateForecastCoverage requires the same complete hourly range for every zone.
func validateForecastCoverage(periods map[int]forecastPeriod, areas []repository.Area) error {
	var reference forecastPeriod
	for i, a := range areas {
		b := periods[a.ID]
		if b.count == 0 || b.last.Sub(b.first)/time.Hour+1 != time.Duration(b.count) {
			return fmt.Errorf("incomplete forecast hours for area %d", a.ID)
		}
		if i == 0 {
			reference = b
		} else if !b.first.Equal(reference.first) || !b.last.Equal(reference.last) || b.count != reference.count {
			return fmt.Errorf("forecast area coverage differs")
		}
	}
	return nil
}

type forecastPeriod struct {
	first, last time.Time
	count       int
}

// forecastAreaIDs identifies the zones a forecast is allowed to contain.
func forecastAreaIDs(areas []repository.Area) (map[int]bool, error) {
	known := map[int]bool{}
	for _, a := range areas {
		if a.ID <= 0 || known[a.ID] {
			return nil, fmt.Errorf("invalid forecast scope")
		}
		known[a.ID] = true
	}
	return known, nil
}

// collectForecastPeriods checks rows and counts unique hours for each zone.
func collectForecastPeriods(hours []repository.ForecastHour, known map[int]bool) (map[int]forecastPeriod, error) {
	periods := map[int]forecastPeriod{}
	seen := map[struct {
		area int
		hour int64
	}]bool{}
	for _, h := range hours {
		if !known[h.AreaID] || h.HourStart.IsZero() || !h.HourStart.Equal(h.HourStart.Truncate(time.Hour)) || !finite(h.RainMM) || !finite(h.SnowfallCM) || h.RainMM < 0 || h.SnowfallCM < 0 || h.RainMM > math.MaxFloat32 || h.SnowfallCM > math.MaxFloat32 || h.PrecipitationProbability < 0 || h.PrecipitationProbability > 100 {
			return nil, fmt.Errorf("invalid forecast row for area %d", h.AreaID)
		}
		key := struct {
			area int
			hour int64
		}{h.AreaID, h.HourStart.Unix()}
		if seen[key] {
			return nil, fmt.Errorf("duplicate forecast area/hour")
		}
		seen[key] = true
		b := periods[h.AreaID]
		if b.count == 0 || h.HourStart.Before(b.first) {
			b.first = h.HourStart
		}
		if b.count == 0 || h.HourStart.After(b.last) {
			b.last = h.HourStart
		}
		b.count++
		periods[h.AreaID] = b
	}
	return periods, nil
}
