package recommendation

import (
	"time"

	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/repository"
)

// Estimates trips for one zone and hour using its forecast and past trips.
func estimateHourTrips(areaID int, at time.Time, input HourInput, options Options) HourEvidence {
	// Match history by the forecast's local ISO day and hour.
	day, hour := domain.LocalWeekdayAndHour(at, options.Location)
	evidence := HourEvidence{HourStart: at, DayOfWeek: day, LocalHour: hour}
	copyHistoricalEvidence(&evidence, input.History)

	if input.Forecast == nil || !input.At.Equal(at) || !input.Forecast.HourStart.Equal(at) || input.Forecast.AreaID != areaID {
		evidence.Reason = "missing_forecast"
		return evidence
	}
	if !domain.ValidWeatherValues(*input.Forecast) {
		evidence.Reason = "invalid_forecast"
		return evidence
	}

	forecast := *input.Forecast
	evidence.Forecast = &forecast
	if forecast.SnowfallCM > 0 {
		evidence.Reason = "snow_not_supported"
		return evidence
	}

	baselineIssue := dryHistoryIssue(areaID, evidence, input.History, options.MinSamples)
	switch {
	case forecast.RainMM < options.RainThreshold:
		// Below the rain threshold we know the extra trips even if typical trips are unknown.
		evidence.Additional = floatPointer(0)
		if baselineIssue != "" {
			return evidence
		}
	case baselineIssue != "":
		evidence.Reason = baselineIssue
		return evidence
	case evidence.RainMean == nil || evidence.RainSamples < options.MinSamples:
		evidence.Reason = "insufficient_rain_history"
		return evidence
	default:
		uplift := *evidence.RainMean - *evidence.DryMean
		evidence.Additional = floatPointer(uplift * float64(forecast.PrecipitationProbability) / 100)
	}

	evidence.Baseline = cloneFloatPointer(evidence.DryMean)
	evidence.Estimated = floatPointer(*evidence.DryMean + *evidence.Additional)
	return evidence
}

func dryHistoryIssue(areaID int, hour HourEvidence, history *repository.HourlyStats, minSamples int64) string {
	switch {
	case history == nil || history.AreaID != areaID || history.DayOfWeek != hour.DayOfWeek || history.HourOfDay != hour.LocalHour:
		return "missing_history"
	case hour.DryMean == nil || hour.DrySamples < minSamples:
		return "insufficient_dry_history"
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
