// Package recommendation implements the pure hourly scoring policy.
package recommendation

import (
	"fmt"
	"mobilitycontrolroom/internal/apperrors"
	"sort"
	"time"
)

// PlanningWindow starts at the next full UTC hour, even when now is exactly on the hour.
func PlanningWindow(now time.Time, duration time.Duration) Window {
	start := now.UTC().Truncate(time.Hour).Add(time.Hour)
	return Window{start, start.Add(duration), int64(duration / time.Second)}
}

// limitations explains why the estimate may be wrong and what it cannot tell us.
func limitations() []string {
	return []string{
		"This recommendation may be inaccurate if the rain forecast is wrong or weather differs within a zone. Check current local conditions before acting.",
		"Historical trips from 2022–2025 may not reflect today's travel patterns. Holidays, major events, road closures, pricing and service disruptions are not included in the estimate.",
		"Completed trips do not tell us how many riders are waiting or how many drivers are available. Check actual driver availability before moving drivers; this estimate does not specify a driver count.",
		"Rainy-hour samples can be limited. Sample counts and variation describe past observations, not a guarantee of accuracy. The comparison does not prove that rain caused the change in trips.",
		"The forecast probability covers any precipitation, so it may not match the chance of rain above our rainfall threshold.",
		"Only the selected zones are compared. A more urgent issue elsewhere will not appear here. Seasonal averages can also change abruptly at season boundaries.",
	}
}

// Score evaluates each zone, then chooses an action only when the entire scope
// has enough evidence. It has no storage, network or cache side effects.
func Score(now time.Time, options Options, inputs []AreaInput) (Result, error) {
	// Step 1: Check the scoring settings and create the planning window.
	if err := options.Validate(); err != nil {
		return Result{}, err
	}
	result := newResult(now, options, len(inputs))
	if len(inputs) == 0 {
		result.Status = "unavailable"
		result.DecisionReason = ReasonNoOperationalAreas
		result.Message = "No operational areas are available."
		return result, nil
	}
	// Step 2: Score each zone once and count zones with complete evidence.
	seen := make(map[int]bool)
	missingForecast := false
	for _, input := range inputs {
		if input.Area.ID <= 0 || seen[input.Area.ID] {
			return Result{}, fmt.Errorf("invalid or duplicate recommendation area")
		}
		seen[input.Area.ID] = true
		area, missing := scoreArea(input, result.Window, options)
		missingForecast = missingForecast || missing
		if area.Assessable {
			result.Coverage.Assessed++
		}
		result.Areas = append(result.Areas, area)
	}
	// Step 3: Keep the response order consistent, then decide whether to act.
	sort.Slice(result.Areas, func(i, j int) bool {
		return result.Areas[i].Area.ID < result.Areas[j].Area.ID
	})
	chooseAction(&result, missingForecast)
	return result, nil
}

// Sets the window, expiry time and minimum extra trips needed to act.
// For example, 50 extra trips per hour means a threshold of 100 for two hours.
func newResult(now time.Time, options Options, areaCount int) Result {
	return Result{
		Status:                   "no_intervention",
		MinimumHistoricalSamples: options.MinSamples,
		RainfallThresholdMM:      options.RainThreshold,
		Window:                   PlanningWindow(now, options.Window),
		GeneratedAt:              now.UTC(),
		ValidUntil:               now.UTC().Truncate(time.Hour).Add(time.Hour),
		Coverage:                 Coverage{Total: areaCount},
		Areas:                    make([]AreaEvidence, 0, areaCount),
		Threshold:                options.AdditionalPerHour * options.Window.Hours(),
		Warnings:                 limitations(),
	}
}

// Adds up one zone's hourly estimates. The boolean reports missing or invalid weather.
func scoreArea(input AreaInput, window Window, options Options) (AreaEvidence, bool) {
	// Step 1: Start with no trips counted and assume every hour can be assessed.
	area := AreaEvidence{
		Area: input.Area, Assessable: true, Reasons: []string{},
		Hourly: make([]HourEvidence, 0, int(options.Window/time.Hour)),
	}
	baseline, additional := 0.0, 0.0
	missingForecast := false
	// Step 2: Assess every hour. Keep the reason whenever an hour cannot be scored.
	for index := 0; index < int(options.Window/time.Hour); index++ {
		at := window.Start.Add(time.Duration(index) * time.Hour)
		var hour HourInput
		if index < len(input.Hours) {
			hour = input.Hours[index]
		}
		evidence := scoreHour(input.Area.ID, at, hour, options)
		area.Hourly = append(area.Hourly, evidence)
		if evidence.Reason != "" {
			area.Assessable = false
			area.Reasons = append(area.Reasons, evidence.Reason)
			missingForecast = missingForecast || evidence.Reason == "missing_forecast" || evidence.Reason == "invalid_forecast"
			continue
		}
		// Keep negative extra trips too: rain can be associated with fewer trips.
		baseline += *evidence.Baseline
		additional += *evidence.Additional
	}
	// Step 3: Show totals only if every hour was assessed. A partial sum could mislead.
	if area.Assessable {
		area.Baseline = number(baseline)
		area.Additional = number(additional)
		area.Estimated = number(baseline + additional)
		if baseline > 0 {
			area.RelativeUplift = number(100 * additional / baseline)
		}
	}
	return area, missingForecast
}

// Sets the status and message the user sees.
func chooseAction(result *Result, missingForecast bool) {
	// Step 1: Missing weather means we cannot make a current recommendation.
	if missingForecast {
		result.Status = "unavailable"
		result.DecisionReason = ReasonForecastUnavailable
		result.Message = apperrors.ForecastUnavailable.Message
		result.ErrorCode = apperrors.ForecastUnavailable.Code
		return
	}
	// Step 2: Require enough evidence for every zone before choosing a winner.
	if result.Coverage.Assessed != result.Coverage.Total {
		result.Status = "insufficient_evidence"
		result.DecisionReason = ReasonIncompleteEvidence
		result.Message = "The forecast and historical evidence do not support an estimate for every monitored zone. Review the hourly evidence before deciding."
		return
	}
	// Step 3: Find the highest extra-trip estimate and compare it with the threshold.
	winner := bestArea(result.Areas)
	result.HighestModeledAdditional = copyNumber(winner.Additional)
	if *winner.Additional < result.Threshold {
		explainNoIntervention(result)
		return
	}
	// Step 4: Recommend reviewing driver availability in the winning zone.
	result.Primary = &winner
	result.Status = "action_required"
	result.DecisionReason = ReasonHighestImpact
	result.Message = "Review driver availability in " + winner.Area.Name + ". Rain is forecast in this zone, which has the highest modeled additional completed trips among the monitored zones and meets the intervention threshold."
}

// Checks all monitored forecast hours, not just the selected
// zone. Zero extra trips alone does not mean that no rain is forecast.
func explainNoIntervention(result *Result) {
	result.DecisionReason = ReasonBelowThreshold
	result.Message = fmt.Sprintf("Rain is forecast, but no monitored zone reaches the intervention threshold of %g additional completed trips over this planning window.", result.Threshold)
	for _, area := range result.Areas {
		for _, hour := range area.Hourly {
			if hour.Forecast != nil && hour.Forecast.RainMM >= result.RainfallThresholdMM {
				return
			}
		}
	}
	result.DecisionReason = ReasonNoQualifyingRain
	result.Message = fmt.Sprintf("No rain at or above %g mm/hour is forecast in the monitored zones for this planning window. No additional rain-related completed trips are projected.", result.RainfallThresholdMM)
}

// Higher absolute additional trips win; equal estimates use the smaller zone ID.
func bestArea(areas []AreaEvidence) AreaEvidence {
	best := areas[0]
	for _, area := range areas[1:] {
		if *area.Additional > *best.Additional || (*area.Additional == *best.Additional && area.Area.ID < best.Area.ID) {
			best = area
		}
	}
	return best
}
