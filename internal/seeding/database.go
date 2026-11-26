package seeding

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type seedWriter struct{ tx pgx.Tx }

// Inserts the zone list before observations reference it.
func (w seedWriter) CopyAreas(ctx context.Context, input []Area) error {
	columns := []string{"id", "name", "latitude", "longitude"}
	_, err := w.tx.CopyFrom(ctx, pgx.Identifier{"community_areas"}, columns, pgx.CopyFromSlice(len(input), func(i int) ([]any, error) {
		a := input[i]
		return []any{a.ID, a.Name, a.Latitude, a.Longitude}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy areas: %w", err)
	}
	return nil
}

// Inserts one prepared batch using PostgreSQL COPY.
func (writer seedWriter) CopyObservations(ctx context.Context, input []Observation) error {
	columns := []string{
		"area_id", "hour_start", "local_date",
		"day_of_week", "season", "hour_of_day", "trip_count",
		"rain_mm", "snowfall_cm", "temperature_c", "apparent_temperature_c",
		"precipitation_mm", "wind_speed_kmh",
	}
	_, err := writer.tx.CopyFrom(ctx, pgx.Identifier{"historical_observations"}, columns, pgx.CopyFromSlice(len(input), func(i int) ([]any, error) {
		row := input[i]
		weather := row.Weather
		return []any{
			row.AreaID, row.HourStart.UTC(), row.LocalDate,
			row.DayOfWeek, row.Season, row.HourOfDay, row.TripCount,
			weather.RainMM, weather.SnowfallCM, weather.TemperatureC, weather.ApparentTemperatureC,
			weather.PrecipitationMM, weather.WindSpeedKMH,
		}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy observations: %w", err)
	}
	return nil
}
