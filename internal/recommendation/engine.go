// Package recommendation implements the pure hourly scoring policy.
package recommendation

import (
	"fmt"
	"mobilitycontrolroom/internal/appErrors"
	"sort"
	"time"
)

// It starts at the next full UTC hour, even when now is exactly on the hour.
func NextFullHourWindow(now time.Time, duration time.Duration) Window {
	start := now.UTC().Truncate(time.Hour).Add(time.Hour)
	return Window{start, start.Add(duration), int64(duration / time.Second)}
}

// It explains why the estimate may be wrong and what it cannot tell us.
func recommendationLimitations() []string {
	return []string{
		"This recommendation may be inaccurate if the rain forecast is wrong or weather differs within a zone. Check current local conditions before acting.",
		"Historical trips from 2022–2025 may not reflect today's travel patterns. Holidays, major events, road closures, pricing and service disruptions are not included in the estimate.",
		"Completed trips do not tell us how many riders are waiting or how many drivers are available. Check actual driver availability before moving drivers; this estimate does not specify a driver count.",
		"Rainy-hour samples can be limited. Sample counts and variation describe past observations, not a guarantee of accuracy. The comparison does not prove that rain caused the change in trips.",
		"The forecast probability covers any precipitation, so it may not match the chance of rain above our rainfall threshold.",
		"Only the selected zones are compared. A more urgent issue elsewhere will not appear here. Seasonal averages can also change abruptly at season boundaries.",
	}
}

// Evaluates each zone and ranks those with complete planning-window evidence.
// It has no storage, network or cache side effects.
func EvaluatePlanningWindow(now time.Time, options Options, inputs []AreaInput) (Result, error) {
	// 1. Validate settings and prepare the planning window.
	if err := options.Validate(); err != nil {
		return Result{}, err
	}
	result := newPlanningWindowResult(now, options, len(inputs))
	if len(inputs) == 0 {
		result.Status = "unavailable"
		result.DecisionReason = ReasonNoOperationalAreas
		result.Message = "No operational areas are available."
		return result, nil
	}

	// 2. Estimate every hour and sum the evidence for each zone.
	seen := make(map[int]bool)
	for _, input := range inputs {
		if input.Area.ID <= 0 || seen[input.Area.ID] {
			return Result{}, fmt.Errorf("invalid or duplicate recommendation area")
		}
		seen[input.Area.ID] = true
		result.Areas = append(result.Areas, estimateAreaWindow(input, result.Window, options))
	}

	// 3. Sort for consistent output, then choose an action among assessable zones.
	sort.Slice(result.Areas, func(i, j int) bool {
		return result.Areas[i].Area.ID < result.Areas[j].Area.ID
	})
	selectScopeDecision(&result)
	return result, nil
}

// Sets the window, expiry time and minimum extra trips needed to act.
// For example, 50 extra trips per hour means a threshold of 100 for two hours.
func newPlanningWindowResult(now time.Time, options Options, areaCount int) Result {
	window := NextFullHourWindow(now, options.Window)
	return Result{
		Status:                   "no_intervention",
		MinimumHistoricalSamples: options.MinSamples,
		RainfallThresholdMM:      options.RainThreshold,
		Window:                   window,
		GeneratedAt:              now.UTC(),
		ValidUntil:               window.Start,
		Coverage:                 Coverage{Total: areaCount},
		Areas:                    make([]AreaEvidence, 0, areaCount),
		Threshold:                options.AdditionalPerHour * options.Window.Hours(),
		Warnings:                 recommendationLimitations(),
	}
}

// Adds up one zone's hours. Totals stay unknown unless every hour is assessable.
func estimateAreaWindow(input AreaInput, window Window, options Options) AreaEvidence {
	area := AreaEvidence{
		Area: input.Area, Reasons: []string{},
		Hourly: make([]HourEvidence, 0, int(options.Window/time.Hour)),
	}
	baseline, additional := 0.0, 0.0

	for index := 0; index < int(options.Window/time.Hour); index++ {
		at := window.Start.Add(time.Duration(index) * time.Hour)
		var hour HourInput
		if index < len(input.Hours) {
			hour = input.Hours[index]
		}

		evidence := estimateHourTrips(input.Area.ID, at, hour, options)
		area.Hourly = append(area.Hourly, evidence)
		if evidence.Reason != "" {
			area.Reasons = append(area.Reasons, evidence.Reason)
			continue
		}

		// Keep negative extra trips too: rain can be associated with fewer trips.
		baseline += *evidence.Baseline
		additional += *evidence.Additional
	}

	area.Assessable = len(area.Reasons) == 0
	if area.Assessable {
		area.Baseline = floatPointer(baseline)
		area.Additional = floatPointer(additional)
		area.Estimated = floatPointer(baseline + additional)
		if baseline > 0 {
			area.RelativeUplift = floatPointer(100 * additional / baseline)
		}
	}

	return area
}

// Compare complete zones and explain the decision in one pass over their evidence.
// Areas are already sorted by ID, so keeping the first equal total breaks ties.
func selectScopeDecision(result *Result) {
	var winner *AreaEvidence
	missingForecast, rainForecast := false, false

	for _, area := range result.Areas {
		if !area.Assessable {
			for _, reason := range area.Reasons {
				missingForecast = missingForecast || reason == "missing_forecast" || reason == "invalid_forecast"
			}
			continue
		}
		result.Coverage.Assessed++
		if winner == nil || *area.Additional > *winner.Additional {
			winner = &area
		}
		for _, hour := range area.Hourly {
			rainForecast = rainForecast || hour.Forecast != nil && hour.Forecast.RainMM >= result.RainfallThresholdMM
		}
	}

	if result.Coverage.Assessed < result.Coverage.Total {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Only %d of %d zones can be assessed. %d zones could not be assessed because forecast or historical evidence is incomplete or weather is unsupported. A higher-impact zone may be among them.", result.Coverage.Assessed, result.Coverage.Total, result.Coverage.Total-result.Coverage.Assessed))
	}

	if winner == nil {
		result.Status = "insufficient_evidence"
		result.DecisionReason = ReasonIncompleteEvidence
		result.Message = "No monitored zone has sufficient forecast and historical evidence for the full planning window. Review the hourly evidence before deciding."
		if missingForecast {
			result.Status = "unavailable"
			result.DecisionReason = ReasonForecastUnavailable
			result.Message = appErrors.ForecastUnavailable.Message
			result.ErrorCode = appErrors.ForecastUnavailable.Code
		}
		return
	}

	result.HighestModeledAdditional = cloneFloatPointer(winner.Additional)

	if *winner.Additional < result.Threshold {
		result.DecisionReason = ReasonNoQualifyingRain
		result.Message = fmt.Sprintf("No rain at or above %g mm/hour is forecast in the assessed zones for this planning window. No additional rain-related completed trips are projected in those zones.", result.RainfallThresholdMM)
		if rainForecast {
			result.DecisionReason = ReasonBelowThreshold
			result.Message = fmt.Sprintf("Rain is forecast, but no assessed zone reaches the intervention threshold of %g additional completed trips over this planning window.", result.Threshold)
		}
		return
	}
	result.Primary = winner
	result.Status = "action_required"
	result.DecisionReason = ReasonHighestImpact
	result.Message = "Review driver availability in " + winner.Area.Name + ". Rain is forecast in this zone, which has the highest modeled additional completed trips among the assessed zones and meets the intervention threshold."
}
