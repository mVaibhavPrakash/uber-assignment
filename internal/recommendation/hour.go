package recommendation

import (
	"math"
	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/repository"
	"time"
)

// Estimates trips for one zone and hour using its forecast and past trips.
func estimateHourTrips(areaID int, at time.Time, input HourInput, options Options) HourEvidence {
	// Step 1: Find the local season, day and hour used to compare historical trips.
	_, season, day, hour := domain.LocalCalendarFields(at, options.Location)
	evidence := HourEvidence{HourStart: at, Season: season, DayOfWeek: day, LocalHour: hour}
	copyHistoricalEvidence(&evidence, input.History)

	// Step 2: Check that the forecast belongs to this zone and hour and has valid values.
	if input.Forecast == nil || !input.At.Equal(at) || !input.Forecast.HourStart.Equal(at) || input.Forecast.AreaID != areaID {
		evidence.Reason = "missing_forecast"
		return evidence
	}
	if !hasValidWeatherValues(*input.Forecast) {
		evidence.Reason = "invalid_forecast"
		return evidence
	}

	// Step 3: Check whether the weather and historical sample counts support an estimate.
	forecast := *input.Forecast
	evidence.Forecast = &forecast
	evidence.Reason = unassessableHourReason(areaID, evidence, input.History, options)
	if evidence.Reason != "" {
		return evidence
	}

	// Step 4: For rain, multiply (rainy mean - dry mean) by rain probability.
	// Example: (1700 - 1200) * 90% = 450 extra trips. Below-threshold hours add zero.
	additional := 0.0
	if forecast.RainMM >= options.RainThreshold {
		additional = (*evidence.RainMean - *evidence.DryMean) * float64(forecast.PrecipitationProbability) / 100
	}

	// Step 5: Return the below-threshold baseline, extra trips and their combined estimate.
	evidence.Baseline = floatPointer(*evidence.DryMean)
	evidence.Additional = floatPointer(additional)
	evidence.Estimated = floatPointer(*evidence.DryMean + additional)

	return evidence
}

// Explains why an hour cannot be scored, or returns an empty string.
// Snow remains excluded from the historical comparison.
func unassessableHourReason(areaID int, hour HourEvidence, history *repository.HourlyStats, options Options) string {
	switch {
	case hour.Forecast.SnowfallCM > 0:
		return "snow_not_supported"
	case history == nil || history.AreaID != areaID || history.Season != hour.Season || history.DayOfWeek != hour.DayOfWeek || history.HourOfDay != hour.LocalHour:
		return "missing_history"
	case hour.DryMean == nil || hour.DrySamples < options.MinSamples:
		return "insufficient_dry_history"
	case hour.Forecast.RainMM >= options.RainThreshold && (hour.RainMean == nil || hour.RainSamples < options.MinSamples):
		return "insufficient_rain_history"
	default:
		return ""
	}
}

// Adds the historical numbers shown in hourly insights, keeping missing values null.
func copyHistoricalEvidence(hour *HourEvidence, history *repository.HourlyStats) {
	if history == nil {
		return
	}
	hour.DryMean = cloneFloatPointer(history.DryMean)
	hour.RainMean = cloneFloatPointer(history.RainMean)
	hour.DrySamples = history.DrySamples
	hour.RainSamples = history.RainSamples
	hour.HistoricalAbsoluteUplift = cloneFloatPointer(history.AbsoluteUplift)
	hour.HistoricalPercentageUplift = cloneFloatPointer(history.PercentageUplift)
	hour.RainStdDev = cloneFloatPointer(history.RainStdDev)
}

// Requires valid rain/snow amounts and a probability from 0 to 100.
func hasValidWeatherValues(h repository.ForecastHour) bool {
	return h.RainMM >= 0 && h.SnowfallCM >= 0 && !math.IsNaN(h.RainMM) && !math.IsNaN(h.SnowfallCM) && !math.IsInf(h.RainMM, 0) && !math.IsInf(h.SnowfallCM, 0) && h.PrecipitationProbability >= 0 && h.PrecipitationProbability <= 100
}

// A nil number means unknown; a pointer to zero means a known zero.
func floatPointer(value float64) *float64 {
	return &value
}

// Own historical values while constructing a result, before it is published.
func cloneFloatPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return floatPointer(*value)
}
