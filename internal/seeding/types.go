// Seed input types mirror the prepared JSON; cleanup belongs to data preparation.
package seeding

import (
	"mobilitycontrolroom/internal/repository"
	"time"
)

type Area = repository.Area

type Trip struct {
	AreaID    int       `json:"area_id"`
	HourStart time.Time `json:"hour_start"`
	TripCount int       `json:"trip_count"`
}

type Weather struct {
	AreaID               int       `json:"area_id"`
	HourStart            time.Time `json:"hour_start"`
	RainMM               float64   `json:"rain_mm"`
	SnowfallCM           float64   `json:"snowfall_cm"`
	TemperatureC         *float64  `json:"temperature_c"`
	ApparentTemperatureC *float64  `json:"apparent_temperature_c"`
	PrecipitationMM      *float64  `json:"precipitation_mm"`
	WindSpeedKMH         *float64  `json:"wind_speed_kmh"`
}
