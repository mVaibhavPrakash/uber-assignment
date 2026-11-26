package recommendation

import (
	"math"
	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/repository"
	"time"
)

// Estimates trips for one zone and hour using its forecast and past trips.
func scoreHour(areaID int, at time.Time, input HourInput, options Options) HourEvidence {
	// Step 1: Find the local season, day and hour used to compare historical trips.
	_, season, day, hour := domain.Calendar(at, options.Location)
	evidence := HourEvidence{HourStart: at, Season: season, DayOfWeek: day, LocalHour: hour}
	copyHistory(&evidence, input.History)
	// Step 2: Check that the forecast belongs to this zone and hour and has valid values.
	if input.Forecast == nil || !input.At.Equal(at) || !input.Forecast.HourStart.Equal(at) || input.Forecast.AreaID != areaID {
		evidence.Reason = "missing_forecast"
		return evidence
	}
	if !validForecast(*input.Forecast) {
		evidence.Reason = "invalid_forecast"
		return evidence
	}
	// Step 3: Check whether the weather and historical sample counts support an estimate.
	forecast := *input.Forecast
	evidence.Forecast = &forecast
	evidence.Reason = assessmentReason(areaID, evidence, input.History, options)
	if evidence.Reason != "" {
		return evidence
	}
	// Step 4: For rain, multiply (rainy mean - dry mean) by rain probability.
	// Example: (1700 - 1200) * 90% = 450 extra trips. Dry hours add zero.
	additional := 0.0
	if forecast.RainMM >= options.RainThreshold {
		additional = (*evidence.RainMean - *evidence.DryMean) * float64(forecast.PrecipitationProbability) / 100
	}
	// Step 5: Return the dry baseline, extra trips and their combined estimate.
	evidence.Baseline = number(*evidence.DryMean)
	evidence.Additional = number(additional)
	evidence.Estimated = number(*evidence.DryMean + additional)
	return evidence
}

// Explains why an hour cannot be scored, or returns an empty string.
// Check snow and light rain first because our historical comparison excludes them.
func assessmentReason(areaID int, hour HourEvidence, history *repository.HourlyStats, options Options) string {
	switch {
	case hour.Forecast.SnowfallCM > 0:
		return "snow_not_supported"
	case hour.Forecast.RainMM > 0 && hour.Forecast.RainMM < options.RainThreshold:
		return "trace_rain_not_supported"
	case history == nil || history.AreaID != areaID || history.Season != hour.Season || history.DayOfWeek != hour.DayOfWeek || history.HourOfDay != hour.LocalHour:
		return "missing_history"
	case !usableMean(hour.DryMean, hour.DrySamples, options.MinSamples):
		return "insufficient_dry_history"
	case hour.Forecast.RainMM >= options.RainThreshold && !usableMean(hour.RainMean, hour.RainSamples, options.MinSamples):
		return "insufficient_rain_history"
	default:
		return ""
	}
}

// Adds the historical numbers shown in hourly insights, keeping missing values null.
func copyHistory(hour *HourEvidence, history *repository.HourlyStats) {
	if history == nil {
		return
	}
	hour.DryMean = copyNumber(history.DryMean)
	hour.RainMean = copyNumber(history.RainMean)
	hour.DrySamples = history.DrySamples
	hour.RainSamples = history.RainSamples
	hour.HistoricalAbsoluteUplift = copyNumber(history.AbsoluteUplift)
	hour.HistoricalPercentageUplift = copyNumber(history.PercentageUplift)
	hour.RainStdDev = copyNumber(history.RainStdDev)
}

// Requires a historical average and enough observations to use it.
func usableMean(mean *float64, n, minM int64) bool {
	return mean != nil && n >= minM
}

// Requires valid rain/snow amounts and a probability from 0 to 100.
func validForecast(h repository.ForecastHour) bool {
	return h.RainMM >= 0 && h.SnowfallCM >= 0 && !math.IsNaN(h.RainMM) && !math.IsNaN(h.SnowfallCM) && !math.IsInf(h.RainMM, 0) && !math.IsInf(h.SnowfallCM, 0) && h.PrecipitationProbability >= 0 && h.PrecipitationProbability <= 100
}
