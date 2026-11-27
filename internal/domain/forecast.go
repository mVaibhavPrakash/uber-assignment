package domain

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"
)

// ValidWeatherValues checks the hourly measurements used to estimate rain impact.
func ValidWeatherValues(h ForecastHour) bool {
	return h.RainMM >= 0 && h.RainMM <= math.MaxFloat64 &&
		h.SnowfallCM >= 0 && h.SnowfallCM <= math.MaxFloat64 &&
		h.PrecipitationProbability >= 0 && h.PrecipitationProbability <= 100
}

// ValidateForecast requires a complete rectangular scope and contiguous UTC
// hours so every zone can be compared over the same planning window.
func ValidateForecast(s ForecastSnapshot, areas []Area) error {
	if !isForecastVersion(s.Version) {
		return fmt.Errorf("forecast version must be a canonical nonzero UUID")
	}
	if s.FetchedAt.IsZero() || len(s.Hours) == 0 || len(areas) == 0 {
		return fmt.Errorf("forecast metadata/scope is empty")
	}
	known, err := validateAndIndexForecastAreas(areas)
	if err != nil {
		return err
	}
	periods, err := validateRowsAndCollectCoverage(s.Hours, known)
	if err != nil {
		return err
	}
	return validateForecastCoverage(periods, areas)
}

// validateForecastCoverage requires the same complete hourly range for every zone.
func validateForecastCoverage(periods map[int]forecastPeriod, areas []Area) error {
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

// Identifies the zones a forecast is allowed to contain.
func validateAndIndexForecastAreas(areas []Area) (map[int]bool, error) {
	known := map[int]bool{}
	for _, a := range areas {
		if a.ID <= 0 || known[a.ID] {
			return nil, fmt.Errorf("invalid forecast scope")
		}
		known[a.ID] = true
	}
	return known, nil
}

// Checks rows and counts unique hours for each zone.
func validateRowsAndCollectCoverage(hours []ForecastHour, known map[int]bool) (map[int]forecastPeriod, error) {
	periods := map[int]forecastPeriod{}
	seen := map[struct {
		area int
		hour int64
	}]bool{}
	for _, h := range hours {
		wholeHour := !h.HourStart.IsZero() && h.HourStart.Equal(h.HourStart.Truncate(time.Hour))
		if !known[h.AreaID] || !wholeHour || !ValidWeatherValues(h) {
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

func isForecastVersion(version string) bool {
	if len(version) != 36 || version[8] != '-' || version[13] != '-' || version[18] != '-' || version[23] != '-' {
		return false
	}
	digits := strings.ReplaceAll(version, "-", "")
	if len(digits) != 32 || digits != strings.ToLower(digits) || digits == "00000000000000000000000000000000" {
		return false
	}
	var decoded [16]byte
	_, err := hex.Decode(decoded[:], []byte(digits))
	return err == nil
}
