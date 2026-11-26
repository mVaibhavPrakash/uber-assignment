package recommendation

import (
	"encoding/json"
	"math"
)

// Round only when building JSON. Ranking and thresholds keep their full precision.
// Missing estimates stay null rather than looking like zero trips.
func wholeTrips(value *float64) *int64 {
	if value == nil {
		return nil
	}
	rounded := int64(math.Round(*value))
	return &rounded
}

// MarshalJSON presents hourly trip quantities as whole trips; percentages and weather retain decimals.
func (hour HourEvidence) MarshalJSON() ([]byte, error) {
	type fields HourEvidence
	return json.Marshal(struct {
		fields
		DryMean    *int64 `json:"dry_mean"`
		RainMean   *int64 `json:"rain_mean"`
		Uplift     *int64 `json:"historical_absolute_uplift"`
		StdDev     *int64 `json:"rain_stddev"`
		Baseline   *int64 `json:"baseline_trips"`
		Estimated  *int64 `json:"estimated_trips"`
		Additional *int64 `json:"additional_trips"`
	}{fields(hour), wholeTrips(hour.DryMean), wholeTrips(hour.RainMean),
		wholeTrips(hour.HistoricalAbsoluteUplift), wholeTrips(hour.RainStdDev),
		wholeTrips(hour.Baseline), wholeTrips(hour.Estimated), wholeTrips(hour.Additional)})
}

// MarshalJSON rounds window totals independently of their hourly values to avoid accumulating rounding errors.
func (area AreaEvidence) MarshalJSON() ([]byte, error) {
	type fields AreaEvidence
	return json.Marshal(struct {
		fields
		Baseline   *int64 `json:"baseline_trips"`
		Estimated  *int64 `json:"estimated_trips"`
		Additional *int64 `json:"additional_trips"`
	}{fields(area), wholeTrips(area.Baseline), wholeTrips(area.Estimated), wholeTrips(area.Additional)})
}

// MarshalJSON keeps recommendation-level trip quantities consistent with area and hourly responses.
func (result Result) MarshalJSON() ([]byte, error) {
	type fields Result
	return json.Marshal(struct {
		fields
		Threshold *int64 `json:"threshold_additional_trips"`
		Highest   *int64 `json:"highest_modeled_additional_trips"`
	}{fields(result), wholeTrips(&result.Threshold), wholeTrips(result.HighestModeledAdditional)})
}
