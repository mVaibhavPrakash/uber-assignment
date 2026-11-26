// Seed input types describe the prepared JSON loaded during database initialization.
package seeding

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"mobilitycontrolroom/internal/repository"
)

type Area = repository.Area

type Trip struct {
	AreaID    int       `json:"area_id"`
	HourStart time.Time `json:"hour_start"`
	TripCount int       `json:"trip_count"`
}

func (t *Trip) UnmarshalJSON(b []byte) error {
	type wire struct {
		AreaID    *int       `json:"area_id"`
		HourStart *time.Time `json:"hour_start"`
		TripCount *int       `json:"trip_count"`
	}
	var w wire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	if w.AreaID == nil || w.HourStart == nil || w.TripCount == nil {
		return fmt.Errorf("missing mandatory trip field")
	}
	*t = Trip{*w.AreaID, w.HourStart.UTC(), *w.TripCount}
	return nil
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

func (w *Weather) UnmarshalJSON(b []byte) error {
	type plain Weather
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	var required struct {
		AreaID     *int       `json:"area_id"`
		HourStart  *time.Time `json:"hour_start"`
		RainMM     *float64   `json:"rain_mm"`
		SnowfallCM *float64   `json:"snowfall_cm"`
	}
	if err := json.Unmarshal(b, &required); err != nil {
		return err
	}
	if required.AreaID == nil || required.HourStart == nil || required.RainMM == nil || required.SnowfallCM == nil {
		return fmt.Errorf("missing mandatory weather field")
	}
	*w = Weather(p)
	w.HourStart = w.HourStart.UTC()
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func ValidateArea(a Area) error {
	if a.ID < 1 || a.ID > math.MaxInt32 || a.Name == "" || !finite(a.Latitude) || !finite(a.Longitude) || a.Latitude < -90 || a.Latitude > 90 || a.Longitude < -180 || a.Longitude > 180 || a.Latitude == 0 && a.Longitude == 0 {
		return fmt.Errorf("invalid operational area %d", a.ID)
	}
	return nil
}

func ValidateTrip(t Trip) error {
	if t.AreaID < 1 || t.AreaID > math.MaxInt32 || t.HourStart.IsZero() || !t.HourStart.Equal(t.HourStart.Truncate(time.Hour)) || t.TripCount < 0 || int64(t.TripCount) > math.MaxInt32 {
		return fmt.Errorf("invalid trip")
	}
	return nil
}

func ValidateWeather(w Weather) error {
	if w.AreaID < 1 || w.AreaID > math.MaxInt32 || w.HourStart.IsZero() || !w.HourStart.Equal(w.HourStart.Truncate(time.Hour)) || !finite(w.RainMM) || !finite(w.SnowfallCM) || w.RainMM < 0 || w.SnowfallCM < 0 || w.RainMM > math.MaxFloat32 || w.SnowfallCM > math.MaxFloat32 {
		return fmt.Errorf("invalid weather")
	}
	for _, p := range []*float64{w.TemperatureC, w.ApparentTemperatureC, w.PrecipitationMM, w.WindSpeedKMH} {
		if p != nil && (!finite(*p) || math.Abs(*p) > math.MaxFloat32) {
			return fmt.Errorf("nonfinite optional weather")
		}
	}
	if w.PrecipitationMM != nil && *w.PrecipitationMM < 0 || w.WindSpeedKMH != nil && *w.WindSpeedKMH < 0 {
		return fmt.Errorf("negative precipitation or wind")
	}
	return nil
}
