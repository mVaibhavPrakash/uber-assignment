package recommendation

import (
	"fmt"
	"math"
	"time"

	"mobilitycontrolroom/internal/domain"
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
	threshold := o.AdditionalPerHour * o.Window.Hours()
	if o.Window <= 0 || o.Window%time.Hour != 0 || o.MinSamples <= 0 || !isFinitePositive(o.RainThreshold) || !isFinitePositive(threshold) || o.Location == nil {
		return fmt.Errorf("invalid recommendation options: positive whole-hour window, samples, finite thresholds and a timezone required")
	}
	return nil
}

func isFinitePositive(v float64) bool {
	return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}

// HourInput pairs one forecast hour with comparable historical trips. Missing evidence is nil.
type HourInput struct {
	At       time.Time
	Forecast *domain.ForecastHour
	History  *domain.HourlyStats
}

// AreaInput contains all planning-window hours for one zone.
type AreaInput struct {
	Area  domain.Area
	Hours []HourInput
}

// Window describes the future hours covered by the recommendation.
type Window struct {
	Start           time.Time
	End             time.Time
	DurationSeconds int64
}

// Coverage reports how many zones have enough evidence for every hour in the window.
type Coverage struct {
	Assessed int
	Total    int
}

// HourEvidence explains the inputs and estimated trips for one hour, or why it could not be scored.
type HourEvidence struct {
	HourStart                  time.Time
	DayOfWeek                  int
	LocalHour                  int
	Forecast                   *domain.ForecastHour
	DryMean                    *float64
	RainMean                   *float64
	DrySamples                 int64
	RainSamples                int64
	HistoricalAbsoluteUplift   *float64
	HistoricalPercentageUplift *float64
	RainStdDev                 *float64
	Baseline                   *float64
	Estimated                  *float64
	Additional                 *float64
	Reason                     string
}

// AreaEvidence contains a zone's hourly evidence and totals for the full window.
type AreaEvidence struct {
	Area           domain.Area
	Assessable     bool
	Baseline       *float64
	Estimated      *float64
	Additional     *float64
	RelativeUplift *float64
	Reasons        []string
	Hourly         []HourEvidence
}

// DecisionReason identifies the scoring outcome.
type DecisionReason string

// Reasons describe the inputs that led to the decision.
const (
	ReasonNoSignificantRainfall     DecisionReason = "no_significant_rainfall"
	ReasonBelowThreshold            DecisionReason = "below_intervention_threshold"
	ReasonHighestImpact             DecisionReason = "highest_expected_impact"
	ReasonIncompleteEvidence        DecisionReason = "incomplete_evidence"
	ReasonForecastUnavailable       DecisionReason = "forecast_unavailable"
	ReasonRecommendationUnavailable DecisionReason = "recommendation_unavailable"
	ReasonNoOperationalAreas        DecisionReason = "no_operational_areas"
)

// Result is the recommendation shown to users, together with evidence and expiry details.
type Result struct {
	Status                   string
	DecisionReason           DecisionReason
	Message                  string
	MinimumHistoricalSamples int64
	RainfallThresholdMM      float64
	Freshness                string
	Window                   Window
	GeneratedAt              time.Time
	ValidUntil               time.Time
	ForecastVersion          string
	ForecastFetchedAt        *time.Time
	Coverage                 Coverage
	Primary                  *AreaEvidence
	Areas                    []AreaEvidence
	Threshold                float64
	HighestModeledAdditional *float64
	Warnings                 []string
}
