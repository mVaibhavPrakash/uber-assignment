package tests

import (
	"testing"
	"time"

	"mobilitycontrolroom/internal/recommendation"
	"mobilitycontrolroom/internal/repository"
)

func TestRecommendationSelectsAnActionableZoneWithPartialCoverage(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// The next two hours are Wednesday noon (rain) and 1 PM (dry).
	now := time.Date(2026, time.November, 25, 11, 20, 0, 0, ny)
	noon := time.Date(2026, time.November, 25, 12, 0, 0, 0, ny)
	onePM := noon.Add(time.Hour)
	options := recommendation.Options{
		Window: 2 * time.Hour, Location: ny, MinSamples: 5,
		RainThreshold: 0.5, AdditionalPerHour: 50,
	}
	zone := func(id int, rainyMean float64, rainySamples int64) recommendation.AreaInput {
		dryMean := 1200.0
		return recommendation.AreaInput{
			Area: repository.Area{ID: id},
			Hours: []recommendation.HourInput{
				{
					At: noon,
					Forecast: &repository.ForecastHour{
						AreaID: id, HourStart: noon, RainMM: 0.5, PrecipitationProbability: 90,
					},
					History: &repository.HourlyStats{
						AreaID: id, DayOfWeek: 3, HourOfDay: 12,
						DryMean: &dryMean, RainMean: &rainyMean, DrySamples: 5, RainSamples: rainySamples,
					},
				},
				{At: onePM, Forecast: &repository.ForecastHour{AreaID: id, HourStart: onePM, RainMM: 0}},
			},
		}
	}
	// Zone 8 has a larger observed increase, but only four rainy samples.
	result, err := recommendation.EvaluatePlanningWindow(now, options, []recommendation.AreaInput{
		zone(7, 1700, 5), zone(8, 2400, 4),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "action_required" || result.DecisionReason != recommendation.ReasonHighestImpact ||
		result.Primary == nil || result.Primary.Area.ID != 7 {
		t.Fatalf("expected action in zone 7, got %s (%s), primary %+v", result.Status, result.DecisionReason, result.Primary)
	}
	if result.Coverage != (recommendation.Coverage{Assessed: 1, Total: 2}) {
		t.Fatalf("expected 1/2 zones assessed, got %+v", result.Coverage)
	}
	// (1700 - 1200) × 90% = 450 extra trips at noon. The dry hour adds zero without a baseline.
	winner := result.Primary
	if winner.Additional == nil || *winner.Additional != 450 || winner.Baseline != nil {
		t.Fatalf("expected 450 extra trips and unknown typical trips, got %+v", winner)
	}
}
