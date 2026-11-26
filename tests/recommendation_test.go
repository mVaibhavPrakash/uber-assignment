package tests

import (
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
	window := recommendation.NextFullHourWindow(now, options.Window)
	for at := window.Start; at.Before(window.End); at = at.Add(time.Hour) {
		_, season, day, hour := domain.LocalCalendarFields(at, location)
		dry, rain := 1200.0, 1700.0
		area.Hours = append(area.Hours, recommendation.HourInput{At: at, Forecast: &repository.ForecastHour{AreaID: 7, HourStart: at, RainMM: 1, PrecipitationProbability: 90}, History: &repository.HourlyStats{AreaID: 7, Season: season, DayOfWeek: day, HourOfDay: hour, DryMean: &dry, RainMean: &rain, DrySamples: 5, RainSamples: 5}})
	}
	area.Hours[1].Forecast.RainMM = 0
	dry := 1400.0
	area.Hours[1].History.DryMean = &dry
	return now, options, []recommendation.AreaInput{area}
}

// A zone with insufficient samples must not veto or outrank a complete zone.
func TestRecommendationRanksAssessedZones(t *testing.T) {
	now, options, inputs := recommendationInput(t)
	inputs[0].Hours[0].Forecast.RainMM = options.RainThreshold
	_, _, excluded := recommendationInput(t)
	excluded[0].Area.ID = 1
	for _, hour := range excluded[0].Hours {
		hour.Forecast.AreaID, hour.History.AreaID = 1, 1
		*hour.History.RainMean = 10000
	}
	excluded[0].Hours[0].History.RainSamples = 4
	result, err := recommendation.EvaluatePlanningWindow(now, options, append(inputs, excluded...))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "action_required" || result.Primary == nil || result.Primary.Area.ID != 7 {
		t.Fatalf("eligible zone was blocked or wrong zone selected: %+v", result)
	}
	if *result.Primary.Additional != 450 || *result.Primary.Baseline != 2600 || *result.Primary.Estimated != 3050 || result.Threshold != 100 {
		t.Fatalf("incorrect recommendation calculation: %+v", result.Primary)
	}
	if result.Coverage.Assessed != 1 || result.Coverage.Total != 2 || result.Areas[0].Additional != nil {
		t.Fatal("incomplete evidence was counted as an assessed estimate")
	}
	if !strings.Contains(strings.Join(result.Warnings, " "), "Only 1 of 2 zones") {
		t.Fatal("partial-coverage recommendation lacks a warning")
	}
}

// Light rain is valid baseline weather and needs no rainy-hour history.
func TestLightRainDoesNotTriggerIntervention(t *testing.T) {
	now, options, inputs := recommendationInput(t)
	for _, hour := range inputs[0].Hours {
		hour.Forecast.RainMM = 0.4
		hour.History.RainMean = nil
		hour.History.RainSamples = 0
	}
	result, err := recommendation.EvaluatePlanningWindow(now, options, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "no_intervention" || result.DecisionReason != recommendation.ReasonNoQualifyingRain || result.Primary != nil {
		t.Fatalf("light rain incorrectly triggered an action or became unsupported: %+v", result)
	}
	area := result.Areas[0]
	if !area.Assessable || *area.Additional != 0 || *area.Baseline != 2600 || *area.Estimated != 2600 {
		t.Fatalf("incorrect below-threshold trip estimate: %+v", area)
	}
}
