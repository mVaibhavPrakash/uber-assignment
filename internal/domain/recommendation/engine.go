// Package recommendation implements the pure hourly scoring policy.
package recommendation

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"mobilitycontrolroom/internal/domain"
)

// NextFullHourWindow starts at the next full UTC hour, even when now is exactly on the hour.
func NextFullHourWindow(now time.Time, duration time.Duration) Window {
	start := domain.NextFullHour(now)
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
		"Only the selected zones are compared. A more urgent issue elsewhere will not appear here. Historical averages pool all seasons, so they may miss seasonal travel patterns.",
	}
}

// EvaluatePlanningWindow evaluates each zone and ranks those with complete planning-window evidence.
// It has no storage, network or cache side effects.
func EvaluatePlanningWindow(ctx context.Context, now time.Time, options Options, inputs []AreaInput) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
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

	seen := make(map[int]bool)
	for _, input := range inputs {
		if input.Area.ID <= 0 || seen[input.Area.ID] {
			return Result{}, fmt.Errorf("invalid or duplicate recommendation area")
		}
		seen[input.Area.ID] = true
		area, err := estimateAreaWindow(ctx, input, result.Window, options)
		if err != nil {
			return Result{}, err
		}
		result.Areas = append(result.Areas, area)
	}

	slices.SortFunc(result.Areas, func(a, b AreaEvidence) int { return cmp.Compare(a.Area.ID, b.Area.ID) })
	chooseRecommendation(&result)
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

// Rain impact and trip volume can have different coverage within the same zone.
func estimateAreaWindow(ctx context.Context, input AreaInput, window Window, options Options) (AreaEvidence, error) {
	area := AreaEvidence{
		Area: input.Area, Reasons: []string{},
		Hourly: make([]HourEvidence, 0, int(options.Window/time.Hour)),
	}
	baseline, additional := 0.0, 0.0
	hasFullBaseline := true

	for index := range int(options.Window / time.Hour) {
		if err := ctx.Err(); err != nil {
			return AreaEvidence{}, err
		}
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
		additional += *evidence.Additional
		if evidence.Baseline == nil {
			hasFullBaseline = false
		} else {
			baseline += *evidence.Baseline
		}
	}

	area.Assessable = len(area.Reasons) == 0
	if !area.Assessable {
		return area, nil
	}
	area.Additional = &additional
	if hasFullBaseline {
		area.Baseline = &baseline
		area.Estimated = floatPointer(baseline + additional)
		if baseline > 0 {
			area.RelativeUplift = floatPointer(100 * additional / baseline)
		}
	}

	return area, nil
}

type zoneComparison struct {
	assessed       int
	highestImpact  *AreaEvidence
	hasForecast    bool
	qualifyingRain bool
}

// Areas are sorted by ID, so keeping the first equal estimate breaks ties.
func compareZones(areas []AreaEvidence, rainThreshold float64) zoneComparison {
	var comparison zoneComparison
	for _, area := range areas {
		for _, hour := range area.Hourly {
			if hour.Forecast != nil {
				comparison.hasForecast = true
				comparison.qualifyingRain = comparison.qualifyingRain || hour.Forecast.RainMM >= rainThreshold
			}
		}
		if !area.Assessable {
			continue
		}
		comparison.assessed++
		if comparison.highestImpact == nil || *area.Additional > *comparison.highestImpact.Additional {
			comparison.highestImpact = &area
		}
	}
	return comparison
}

func chooseRecommendation(result *Result) {
	zones := compareZones(result.Areas, result.RainfallThresholdMM)
	result.Coverage.Assessed = zones.assessed
	partial := zones.assessed < result.Coverage.Total
	if partial {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Only %d of %d zones can be assessed. %d zones could not be assessed because forecast or historical evidence is incomplete or weather is unsupported. A higher-impact zone may be among them.", result.Coverage.Assessed, result.Coverage.Total, result.Coverage.Total-result.Coverage.Assessed))
	}

	if !zones.hasForecast {
		result.Status = "unavailable"
		result.DecisionReason = ReasonForecastUnavailable
		result.Message = "Complete forecast coverage is unavailable."
		return
	}
	winner := zones.highestImpact
	if winner != nil {
		result.HighestModeledAdditional = cloneFloatPointer(winner.Additional)
	}

	switch {
	case winner != nil && *winner.Additional >= result.Threshold:
		result.Primary = winner
		result.Status = "action_required"
		result.DecisionReason = ReasonHighestImpact
		result.Message = "Review driver availability in " + winner.Area.Name + ". Rain is forecast in this zone, which has the highest modeled additional completed trips among the assessed zones and meets the intervention threshold."
	case partial:
		result.Status = "insufficient_evidence"
		result.DecisionReason = ReasonIncompleteEvidence
		result.Message = "No assessed zone reaches the intervention threshold. Some zones could not be assessed for the full planning window."
		if zones.assessed == 0 {
			result.Message = "No monitored zone has enough evidence to assess rain impact for the full planning window. Review the hourly evidence before deciding."
		}
	case !zones.qualifyingRain:
		result.DecisionReason = ReasonNoSignificantRainfall
		result.Message = fmt.Sprintf("Forecast rainfall stays below %g mm/hour in all monitored zones during this planning window. No additional rain-related trips are projected.", result.RainfallThresholdMM)
	default:
		result.DecisionReason = ReasonBelowThreshold
		result.Message = fmt.Sprintf("Rain at or above %g mm/hour is forecast, but no zone reaches the intervention threshold of %g additional completed trips over this planning window.", result.RainfallThresholdMM, result.Threshold)
	}
}

// UnavailableResult describes the current window when usable evidence is missing.
func UnavailableResult(now time.Time, options Options, areaCount int, version string) Result {
	result := newPlanningWindowResult(now, options, areaCount)
	result.Status = "unavailable"
	result.DecisionReason = ReasonRecommendationUnavailable
	result.Message = "A current, complete forecast and recommendation are not available."
	result.Freshness = "unavailable"
	result.ValidUntil = now.UTC()
	result.ForecastVersion = version
	return result
}
