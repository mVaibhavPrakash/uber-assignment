// Command ui-fixtures generates illustrative inputs for browser checks and
// screenshots. These files are never loaded by the production server or UI.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/recommendation"
	"mobilitycontrolroom/internal/repository"
)

type fixture struct {
	Description    string                 `json:"description"`
	Clock          time.Time              `json:"clock"`
	HTTPStatus     int                    `json:"http_status"`
	Recommendation recommendation.Result  `json:"recommendation"`
	Insights       map[int]map[string]any `json:"insights"`
}

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() error {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		return err
	}
	now := time.Date(2026, 11, 25, 16, 20, 0, 0, time.UTC)
	options := recommendation.Options{Window: 2 * time.Hour, MinSamples: 5, RainThreshold: 0.5, AdditionalPerHour: 50, Location: location}
	out := filepath.Join(".local", "ui-fixtures")
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	for _, state := range []string{"normal", "action", "insufficient", "stale", "unavailable"} {
		inputs := scenarioInputs(now, options)
		switch state {
		case "normal":
			for _, area := range inputs {
				for _, hour := range area.Hours {
					hour.Forecast.RainMM = 0
					hour.History.RainSamples = 2
				}
			}
		case "insufficient":
			inputs[1].Hours[0].History.RainSamples = 2
		case "unavailable":
			inputs[1].Hours[0].Forecast = nil
		}
		result, err := recommendation.Score(now, options, inputs)
		if err != nil {
			return err
		}
		result.ForecastVersion = "11111111-1111-4111-8111-111111111111"
		fetched := now.Add(-15 * time.Minute)
		result.ForecastFetchedAt = &fetched
		result.Freshness = "fresh"
		status := 200
		if state == "stale" {
			fetched = now.Add(-4 * time.Hour)
			result.Freshness = "stale"
			result.Warnings = append(result.Warnings, "The weather forecast is older than the normal refresh interval. Check current weather before acting.")
		}
		if state == "unavailable" {
			status = 503
			result.Freshness = "unavailable"
			result.ForecastFetchedAt = nil
			result.ValidUntil = now
		}
		bundle := fixture{Description: "Illustrative development scenario — simulated weather and historical inputs, not live observations.", Clock: now, HTTPStatus: status, Insights: map[int]map[string]any{}}
		for _, area := range result.Areas {
			bundle.Insights[area.Area.ID] = map[string]any{
				"forecast_version": result.ForecastVersion, "window": result.Window,
				"forecast_fetched_at": result.ForecastFetchedAt, "generated_at": result.GeneratedAt,
				"valid_until": result.ValidUntil, "freshness": result.Freshness,
				"evidence": area, "warnings": result.Warnings,
			}
		}
		// Match the summary endpoint: hourly records belong only in insights.
		result.Areas = append([]recommendation.AreaEvidence(nil), result.Areas...)
		for i := range result.Areas {
			result.Areas[i].Hourly = nil
		}
		if result.Primary != nil {
			primary := *result.Primary
			primary.Hourly = nil
			result.Primary = &primary
		}
		bundle.Recommendation = result
		data, err := json.MarshalIndent(bundle, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, state+".json"), data, 0644); err != nil {
			return err
		}
	}
	fmt.Println("Wrote five illustrative browser fixtures to", out)
	return nil
}

func scenarioInputs(now time.Time, options recommendation.Options) []recommendation.AreaInput {
	areas := []repository.Area{{ID: 7, Name: "Astoria (Queens)"}, {ID: 14, Name: "Bay Ridge (Brooklyn)"}}
	window := recommendation.PlanningWindow(now, options.Window)
	var inputs []recommendation.AreaInput
	for _, area := range areas {
		input := recommendation.AreaInput{Area: area}
		for i := 0; i < 2; i++ {
			at := window.Start.Add(time.Duration(i) * time.Hour)
			_, season, day, hour := domain.Calendar(at, options.Location)
			dry, rain, spread := 800.0+float64(i)*100, 850.0+float64(i)*100, 100.0
			forecast := repository.ForecastHour{AreaID: area.ID, HourStart: at}
			if area.ID == 14 {
				dry, rain, spread = 1200+float64(i)*200, 1700+float64(i)*200, 180
				if i == 0 {
					forecast.RainMM = 1
					forecast.PrecipitationProbability = 90
				}
			}
			difference := rain - dry
			percentage := difference / dry * 100
			stats := repository.HourlyStats{AreaID: area.ID, Season: season, DayOfWeek: day, HourOfDay: hour, DryMean: &dry, RainMean: &rain, DrySamples: 35, RainSamples: 12, AbsoluteUplift: &difference, PercentageUplift: &percentage, RainStdDev: &spread}
			input.Hours = append(input.Hours, recommendation.HourInput{At: at, Forecast: &forecast, History: &stats})
		}
		inputs = append(inputs, input)
	}
	return inputs
}
