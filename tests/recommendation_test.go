package tests

import (
	"encoding/json"
	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/recommendation"
	"mobilitycontrolroom/internal/repository"
	"strings"
	"testing"
	"time"
)

func recommendationInput(t *testing.T) (time.Time, recommendation.Options, []recommendation.AreaInput) {
	t.Helper()
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 25, 16, 20, 0, 0, time.UTC)
	options := recommendation.Options{Window: 2 * time.Hour, MinSamples: 5, RainThreshold: 0.5, AdditionalPerHour: 50, Location: location}
	area := recommendation.AreaInput{Area: repository.Area{ID: 7, Name: "Astoria"}}
	window := recommendation.PlanningWindow(now, options.Window)
	for at := window.Start; at.Before(window.End); at = at.Add(time.Hour) {
		_, season, day, hour := domain.Calendar(at, location)
		dry, rain := 1200.0, 1700.0
		area.Hours = append(area.Hours, recommendation.HourInput{At: at, Forecast: &repository.ForecastHour{AreaID: 7, HourStart: at, RainMM: 1, PrecipitationProbability: 90}, History: &repository.HourlyStats{AreaID: 7, Season: season, DayOfWeek: day, HourOfDay: hour, DryMean: &dry, RainMean: &rain, DrySamples: 5, RainSamples: 5}})
	}
	area.Hours[1].Forecast.RainMM = 0
	dry := 1400.0
	area.Hours[1].History.DryMean = &dry
	return now, options, []recommendation.AreaInput{area}
}

func TestRecommendationMessages(t *testing.T) {
	t.Run("whole_trips_in_json_without_changing_scoring", func(t *testing.T) {
		now, options, inputs := recommendationInput(t)
		*inputs[0].Hours[0].History.DryMean = 1200.25
		result, err := recommendation.Score(now, options, inputs)
		if err != nil {
			t.Fatal(err)
		}
		before := *result.Primary.Additional
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			Primary struct {
				Baseline   int64 `json:"baseline_trips"`
				Additional int64 `json:"additional_trips"`
				Hourly     []struct {
					Baseline   int64  `json:"baseline_trips"`
					Additional int64  `json:"additional_trips"`
					StdDev     *int64 `json:"rain_stddev"`
				} `json:"hourly"`
			} `json:"primary_recommendation"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatal(err)
		}
		if response.Primary.Baseline != 2600 || response.Primary.Additional != 450 || response.Primary.Hourly[0].Baseline != 1200 || response.Primary.Hourly[0].Additional != 450 || response.Primary.Hourly[0].StdDev != nil {
			t.Fatalf("unexpected whole-trip response: %s", data)
		}
		if before != 449.775 || *result.Primary.Additional != before {
			t.Fatal("JSON rounding changed the scoring result")
		}
		negative := -1.6
		missing, err := json.Marshal(recommendation.AreaEvidence{Additional: &negative})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(missing), `"baseline_trips":null`) || !strings.Contains(string(missing), `"additional_trips":-2`) {
			t.Fatalf("null or negative rounding lost: %s", missing)
		}
	})
	cases := []struct {
		name    string
		prepare func(*recommendation.Options, []recommendation.AreaInput) []recommendation.AreaInput
		status  string
		reason  recommendation.DecisionReason
		message string
	}{
		{name: "action", status: "action_required", reason: recommendation.ReasonHighestImpact, message: "highest modeled additional completed trips"},
		{name: "rain_below_threshold", prepare: func(options *recommendation.Options, inputs []recommendation.AreaInput) []recommendation.AreaInput {
			options.AdditionalPerHour = 300
			return inputs
		}, status: "no_intervention", reason: recommendation.ReasonBelowThreshold, message: "threshold of 600"},
		{name: "dry_forecast_does_not_require_rain_history", prepare: func(_ *recommendation.Options, inputs []recommendation.AreaInput) []recommendation.AreaInput {
			for _, hour := range inputs[0].Hours {
				hour.Forecast.RainMM = 0
				hour.History.RainMean = nil
				hour.History.RainSamples = 0
			}
			return inputs
		}, status: "no_intervention", reason: recommendation.ReasonNoQualifyingRain, message: "No rain at or above 0.5 mm/hour"},
		{name: "zero_impact_does_not_mean_no_rain", prepare: func(_ *recommendation.Options, inputs []recommendation.AreaInput) []recommendation.AreaInput {
			inputs[0].Hours[0].History.RainMean = inputs[0].Hours[0].History.DryMean
			return inputs
		}, status: "no_intervention", reason: recommendation.ReasonBelowThreshold, message: "Rain is forecast"},
		{name: "insufficient_rain_history_is_not_normal_operations", prepare: func(_ *recommendation.Options, inputs []recommendation.AreaInput) []recommendation.AreaInput {
			inputs[0].Hours[0].History.RainSamples = 4
			return inputs
		}, status: "insufficient_evidence", reason: recommendation.ReasonIncompleteEvidence, message: "do not support an estimate for every monitored zone"},
		{name: "missing_forecast_is_not_normal_operations", prepare: func(_ *recommendation.Options, inputs []recommendation.AreaInput) []recommendation.AreaInput {
			inputs[0].Hours[0].Forecast = nil
			return inputs
		}, status: "unavailable", reason: recommendation.ReasonForecastUnavailable, message: "forecast coverage is unavailable"},
		{name: "effective_configuration_is_returned", prepare: func(options *recommendation.Options, inputs []recommendation.AreaInput) []recommendation.AreaInput {
			options.MinSamples = 7
			options.RainThreshold = 2
			for _, hour := range inputs[0].Hours {
				hour.Forecast.RainMM = 0
				hour.History.DrySamples = 7
			}
			return inputs
		}, status: "no_intervention", reason: recommendation.ReasonNoQualifyingRain, message: "No rain at or above 2 mm/hour"},
		{name: "reason_checks_every_zone", prepare: func(options *recommendation.Options, inputs []recommendation.AreaInput) []recommendation.AreaInput {
			options.AdditionalPerHour = 300
			_, _, other := recommendationInput(t)
			other[0].Area.ID = 14
			for _, hour := range other[0].Hours {
				hour.Forecast.AreaID = 14
				hour.History.AreaID = 14
			}
			for _, hour := range inputs[0].Hours {
				hour.Forecast.RainMM = 0
			}
			return append(inputs, other[0])
		}, status: "no_intervention", reason: recommendation.ReasonBelowThreshold, message: "Rain is forecast"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			now, options, inputs := recommendationInput(t)
			if test.prepare != nil {
				inputs = test.prepare(&options, inputs)
			}
			result, err := recommendation.Score(now, options, inputs)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != test.status || result.DecisionReason != test.reason {
				t.Fatalf("unexpected decision: %s / %s", result.Status, result.DecisionReason)
			}
			if !strings.Contains(result.Message, test.message) {
				t.Fatalf("unclear explanation: %s", result.Message)
			}
			if result.MinimumHistoricalSamples != options.MinSamples || result.RainfallThresholdMM != options.RainThreshold {
				t.Fatal("response does not reflect configured evidence requirements")
			}
			if result.Status == "action_required" {
				if result.Primary == nil || result.Primary.Area.ID != 7 {
					t.Fatalf("wrong action: %+v", result.Primary)
				}
				if *result.Primary.Additional != 450 || *result.Primary.Baseline != 2600 || *result.Primary.Estimated != 3050 {
					t.Fatalf("scoring changed: %+v", result.Primary)
				}
				if !strings.Contains(result.Message, "Review driver availability in Astoria") {
					t.Fatal("action message is not specific to the winning zone")
				}
			} else if result.Primary != nil {
				t.Fatal("unexpected action without complete eligible evidence")
			}
			if result.Status == "insufficient_evidence" || result.Status == "unavailable" {
				if result.HighestModeledAdditional != nil {
					t.Fatal("missing evidence presented as measured impact")
				}
			}
			if !strings.Contains(strings.Join(result.Warnings, " "), "may be inaccurate") {
				t.Fatal("recommendation does not explain uncertainty")
			}
		})
	}
}
