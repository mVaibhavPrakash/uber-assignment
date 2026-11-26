package api

import "mobilitycontrolroom/internal/recommendation"

// Build a summary without changing cached areas, pointers or hourly evidence.
// Only area headers are copied; the historical and hourly data stays read-only.
func omitHourlyEvidence(result recommendation.Result) recommendation.Result {
	areas := make([]recommendation.AreaEvidence, len(result.Areas))
	for i, area := range result.Areas {
		area.Hourly = nil
		areas[i] = area
	}
	result.Areas = areas
	if result.Primary != nil {
		primary := *result.Primary
		primary.Hourly = nil
		result.Primary = &primary
	}
	result.Warnings = warningsWithForecastFreshness(result)
	return result
}

// Fresh results reuse read-only warnings. Stale responses get their own list,
// so repeated requests cannot append warnings to the cached recommendation.
func warningsWithForecastFreshness(result recommendation.Result) []string {
	if result.Freshness != "stale" {
		return result.Warnings
	}
	warnings := make([]string, 0, len(result.Warnings)+1)
	warnings = append(warnings, result.Warnings...)
	return append(warnings, "The weather forecast is older than the normal refresh interval. Conditions may have changed; check current weather before acting.")
}
