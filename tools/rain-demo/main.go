// A local UI harness using fictional inputs and the production scoring engine.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"
	_ "time/tzdata"

	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/domain/recommendation"
	"mobilitycontrolroom/internal/transport/httpApi"
)

func main() {
	address := flag.String("listen", "127.0.0.1:8081", "local sample API address")
	flag.Parse()
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		log.Fatal(err)
	}
	service := rainScenario{options: recommendation.Options{
		Window: 2 * time.Hour, MinSamples: 5, RainThreshold: 0.5,
		AdditionalPerHour: 50, Location: location,
	}}
	server := &http.Server{Addr: *address, Handler: httpApi.Handler(service, time.Now), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("FICTIONAL SAMPLE DATA: rain scenario API at http://%s; no database or weather provider is used", *address)
	log.Fatal(server.ListenAndServe())
}

type rainScenario struct{ options recommendation.Options }

func (rainScenario) Areas() []domain.Area {
	return []domain.Area{{ID: 7, Name: "Astoria (fictional sample data)"}}
}

func (s rainScenario) CurrentRecommendation(now time.Time) recommendation.Result {
	window := recommendation.NextFullHourWindow(now, s.options.Window)
	input := recommendation.AreaInput{Area: s.Areas()[0]}
	for index, dry := range []float64{1200, 1400} {
		at := window.Start.Add(time.Duration(index) * time.Hour)
		day, hour := domain.LocalWeekdayAndHour(at, s.options.Location)
		rain, rainMM := 1700.0, 1.0
		if index == 1 {
			rainMM = 0
		}
		input.Hours = append(input.Hours, recommendation.HourInput{
			At:       at,
			Forecast: &domain.ForecastHour{AreaID: 7, HourStart: at, RainMM: rainMM, PrecipitationProbability: 90},
			History: &domain.HourlyStats{
				AreaID: 7, DayOfWeek: day, HourOfDay: hour,
				DryMean: &dry, RainMean: &rain, DrySamples: 5, RainSamples: 5,
			},
		})
	}
	result, err := recommendation.EvaluatePlanningWindow(context.Background(), now, s.options, []recommendation.AreaInput{input})
	if err != nil {
		panic(err) // Fixed sample options must remain valid; the API recovers safely.
	}
	result.Freshness = "fresh"
	result.ForecastVersion = "fictional-rain-sample"
	result.ForecastFetchedAt = &now
	result.Message = "FICTIONAL SAMPLE DATA. " + result.Message
	return result
}
