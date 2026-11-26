package recommendation

import (
	"fmt"
	"math"
	"mobilitycontrolroom/internal/repository"
	"time"
)

// Options holds the window, sample requirements and thresholds used to decide whether to act.
type Options struct {
	Window                           time.Duration
	MinSamples                       int64
	RainThreshold, AdditionalPerHour float64
	Location                         *time.Location
}

// Validate checks that the scoring settings are usable before calculations begin.
func (o Options) Validate() error {
	if o.Window <= 0 || o.Window%time.Hour != 0 || o.MinSamples <= 0 || !positive(o.RainThreshold) || !positive(o.AdditionalPerHour) || o.Location == nil {
		return fmt.Errorf("invalid recommendation options: positive whole-hour window, samples, finite thresholds and timezone required")
	}
	return nil
}

// positive rejects zero, negative numbers and values that cannot be used in calculations.
func positive(v float64) bool {
	return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}

// HourInput pairs one forecast hour with comparable historical trips. Missing evidence is nil.
type HourInput struct {
	At       time.Time
	Forecast *repository.ForecastHour
	History  *repository.HourlyStats
}

// AreaInput contains all planning-window hours for one zone.
type AreaInput struct {
	Area  repository.Area
	Hours []HourInput
}

// Window describes the future hours covered by the recommendation.
type Window struct {
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	DurationSeconds int64     `json:"duration_seconds"`
}

// Coverage reports how many zones have enough evidence for every hour in the window.
type Coverage struct {
	Assessed int `json:"assessed"`
	Total    int `json:"total"`
}

// HourEvidence explains the inputs and estimated trips for one hour, or why it could not be scored.
type HourEvidence struct {
	HourStart                  time.Time                `json:"hour_start"`
	Season                     int                      `json:"season"`
	DayOfWeek                  int                      `json:"day_of_week"`
	LocalHour                  int                      `json:"local_hour"`
	Forecast                   *repository.ForecastHour `json:"forecast"`
	DryMean                    *float64                 `json:"dry_mean"`
	RainMean                   *float64                 `json:"rain_mean"`
	DrySamples                 int64                    `json:"dry_samples"`
	RainSamples                int64                    `json:"rain_samples"`
	HistoricalAbsoluteUplift   *float64                 `json:"historical_absolute_uplift"`
	HistoricalPercentageUplift *float64                 `json:"historical_percentage_uplift"`
	RainStdDev                 *float64                 `json:"rain_stddev"`
	Baseline                   *float64                 `json:"baseline_trips"`
	Estimated                  *float64                 `json:"estimated_trips"`
	Additional                 *float64                 `json:"additional_trips"`
	Reason                     string                   `json:"reason,omitempty"`
}

// AreaEvidence contains a zone's hourly evidence and totals for the full window.
type AreaEvidence struct {
	Area           repository.Area `json:"area"`
	Assessable     bool            `json:"assessable"`
	Baseline       *float64        `json:"baseline_trips"`
	Estimated      *float64        `json:"estimated_trips"`
	Additional     *float64        `json:"additional_trips"`
	RelativeUplift *float64        `json:"relative_uplift_percent"`
	Reasons        []string        `json:"reasons"`
	Hourly         []HourEvidence  `json:"hourly,omitempty"`
}

// DecisionReason explains the existing decision without changing its eligibility rules.
type DecisionReason string

const (
	ReasonNoQualifyingRain          DecisionReason = "no_qualifying_rain"
	ReasonBelowThreshold            DecisionReason = "below_intervention_threshold"
	ReasonHighestImpact             DecisionReason = "highest_expected_impact"
	ReasonIncompleteEvidence        DecisionReason = "incomplete_evidence"
	ReasonForecastUnavailable       DecisionReason = "forecast_unavailable"
	ReasonRecommendationUnavailable DecisionReason = "recommendation_unavailable"
	ReasonNoOperationalAreas        DecisionReason = "no_operational_areas"
)

// Result is the recommendation shown to users, together with evidence and expiry details.
type Result struct {
	Status                   string         `json:"status"`
	DecisionReason           DecisionReason `json:"decision_reason"`
	Message                  string         `json:"message"`
	MinimumHistoricalSamples int64          `json:"minimum_historical_samples"`
	RainfallThresholdMM      float64        `json:"rainfall_threshold_mm"`
	Freshness                string         `json:"freshness"`
	Window                   Window         `json:"window"`
	GeneratedAt              time.Time      `json:"generated_at"`
	ValidUntil               time.Time      `json:"valid_until"`
	ForecastVersion          string         `json:"forecast_version"`
	ForecastFetchedAt        *time.Time     `json:"forecast_fetched_at"`
	Coverage                 Coverage       `json:"coverage"`
	Primary                  *AreaEvidence  `json:"primary_recommendation"`
	Areas                    []AreaEvidence `json:"areas"`
	Threshold                float64        `json:"threshold_additional_trips"`
	HighestModeledAdditional *float64       `json:"highest_modeled_additional_trips"`
	Warnings                 []string       `json:"warnings"`
	ErrorCode                string         `json:"error_code,omitempty"`
}
