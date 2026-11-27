package httpApi

import (
	"math"
	"slices"
	"time"

	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/domain/recommendation"
)

type areaResponse struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type forecastHourResponse struct {
	AreaID                   int       `json:"area_id"`
	HourStart                time.Time `json:"hour_start"`
	RainMM                   float64   `json:"rain_mm"`
	SnowfallCM               float64   `json:"snowfall_cm"`
	PrecipitationProbability int       `json:"precipitation_probability"`
}

type windowResponse struct {
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	DurationSeconds int64     `json:"duration_seconds"`
}

type coverageResponse struct {
	Assessed int `json:"assessed"`
	Total    int `json:"total"`
}

type hourResponse struct {
	HourStart                  time.Time             `json:"hour_start"`
	DayOfWeek                  int                   `json:"day_of_week"`
	LocalHour                  int                   `json:"local_hour"`
	Forecast                   *forecastHourResponse `json:"forecast"`
	DrySamples                 int64                 `json:"dry_samples"`
	RainSamples                int64                 `json:"rain_samples"`
	HistoricalPercentageUplift *float64              `json:"historical_percentage_uplift"`
	Reason                     string                `json:"reason,omitempty"`
	DryMean                    *int64                `json:"dry_mean"`
	RainMean                   *int64                `json:"rain_mean"`
	HistoricalAbsoluteUplift   *int64                `json:"historical_absolute_uplift"`
	RainStdDev                 *int64                `json:"rain_stddev"`
	Baseline                   *int64                `json:"baseline_trips"`
	Estimated                  *int64                `json:"estimated_trips"`
	Additional                 *int64                `json:"additional_trips"`
}

type areaEvidenceResponse struct {
	Area            areaResponse   `json:"area"`
	Assessable      bool           `json:"assessable"`
	RelativeUplift  *float64       `json:"relative_uplift_percent"`
	Reasons         []string       `json:"reasons"`
	Hourly          []hourResponse `json:"hourly,omitempty"`
	Baseline        *int64         `json:"baseline_trips"`
	Estimated       *int64         `json:"estimated_trips"`
	Additional      *int64         `json:"additional_trips"`
	AdditionalExact *float64       `json:"additional_trips_exact"`
}

type recommendationResponse struct {
	Status                   string                        `json:"status"`
	DecisionReason           recommendation.DecisionReason `json:"decision_reason"`
	Message                  string                        `json:"message"`
	MinimumHistoricalSamples int64                         `json:"minimum_historical_samples"`
	RainfallThresholdMM      float64                       `json:"rainfall_threshold_mm"`
	Freshness                string                        `json:"freshness"`
	Window                   windowResponse                `json:"window"`
	GeneratedAt              time.Time                     `json:"generated_at"`
	ValidUntil               time.Time                     `json:"valid_until"`
	ForecastVersion          string                        `json:"forecast_version"`
	ForecastFetchedAt        *time.Time                    `json:"forecast_fetched_at"`
	Coverage                 coverageResponse              `json:"coverage"`
	Primary                  *areaEvidenceResponse         `json:"primary_recommendation"`
	Areas                    []areaEvidenceResponse        `json:"areas"`
	Warnings                 []string                      `json:"warnings"`
	ErrorCode                string                        `json:"error_code,omitempty"`
	Threshold                *int64                        `json:"threshold_additional_trips"`
	HighestModeledAdditional *int64                        `json:"highest_modeled_additional_trips"`
	ThresholdExact           float64                       `json:"threshold_additional_trips_exact"`
	RequestID                string                        `json:"request_id,omitempty"`
}

func summarize(result recommendation.Result) recommendationResponse {
	response := recommendationResponse{
		Status: result.Status, DecisionReason: result.DecisionReason, Message: result.Message,
		MinimumHistoricalSamples: result.MinimumHistoricalSamples, RainfallThresholdMM: result.RainfallThresholdMM,
		Freshness: result.Freshness, Window: windowResponse(result.Window),
		GeneratedAt: result.GeneratedAt, ValidUntil: result.ValidUntil,
		ForecastVersion: result.ForecastVersion, ForecastFetchedAt: result.ForecastFetchedAt,
		Coverage: coverageResponse(result.Coverage), Warnings: warningsWithForecastFreshness(result),
		Areas:     make([]areaEvidenceResponse, 0, len(result.Areas)),
		Threshold: roundTrips(&result.Threshold), ThresholdExact: result.Threshold,
		HighestModeledAdditional: roundTrips(result.HighestModeledAdditional),
	}
	for _, area := range result.Areas {
		response.Areas = append(response.Areas, presentArea(area, false))
	}
	if result.Primary != nil {
		primary := presentArea(*result.Primary, false)
		response.Primary = &primary
	}
	return response
}

func presentAreas(areas []domain.Area) []areaResponse {
	if areas == nil {
		return nil
	}
	response := make([]areaResponse, len(areas))
	for i, area := range areas {
		response[i] = areaResponse(area)
	}
	return response
}

func presentArea(area recommendation.AreaEvidence, includeHourly bool) areaEvidenceResponse {
	response := areaEvidenceResponse{
		Area: areaResponse(area.Area), Assessable: area.Assessable, Reasons: area.Reasons, RelativeUplift: area.RelativeUplift,
		Baseline: roundTrips(area.Baseline), Estimated: roundTrips(area.Estimated),
		Additional: roundTrips(area.Additional), AdditionalExact: area.Additional,
	}
	if includeHourly {
		response.Hourly = make([]hourResponse, 0, len(area.Hourly))
		for _, hour := range area.Hourly {
			response.Hourly = append(response.Hourly, presentHour(hour))
		}
	}
	return response
}

func presentHour(hour recommendation.HourEvidence) hourResponse {
	response := hourResponse{
		HourStart: hour.HourStart, DayOfWeek: hour.DayOfWeek, LocalHour: hour.LocalHour,
		DrySamples: hour.DrySamples, RainSamples: hour.RainSamples, Reason: hour.Reason,
		HistoricalPercentageUplift: hour.HistoricalPercentageUplift,
		DryMean:                    roundTrips(hour.DryMean), RainMean: roundTrips(hour.RainMean),
		HistoricalAbsoluteUplift: roundTrips(hour.HistoricalAbsoluteUplift), RainStdDev: roundTrips(hour.RainStdDev),
		Baseline: roundTrips(hour.Baseline), Estimated: roundTrips(hour.Estimated), Additional: roundTrips(hour.Additional),
	}
	if hour.Forecast != nil {
		response.Forecast = (*forecastHourResponse)(hour.Forecast)
	}
	return response
}

// Round only for presentation so the decision keeps the original precision.
func roundTrips(value *float64) *int64 {
	if value == nil {
		return nil
	}
	rounded := int64(math.Round(*value))
	return &rounded
}

func warningsWithForecastFreshness(result recommendation.Result) []string {
	if result.Freshness != "stale" {
		return result.Warnings
	}
	return append(slices.Clone(result.Warnings), "The weather forecast is older than the normal refresh interval. Conditions may have changed; check current weather before acting.")
}
