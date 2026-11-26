package seeding

import (
	"encoding/json"
	"fmt"
	"time"
)

// Rejects missing/null mandatory fields rather than turning them
// into valid-looking zero values. Optional weather measurements remain nullable.
func (o *Observation) UnmarshalJSON(data []byte) error {
	var wire struct {
		AreaID    *int       `json:"area_id"`
		HourStart *time.Time `json:"hour_start"`
		TripCount *int       `json:"trip_count"`
		Weather   *Weather   `json:"weather"`
		LocalDate *time.Time `json:"local_date"`
		Season    *int       `json:"season"`
		DayOfWeek *int       `json:"day_of_week"`
		HourOfDay *int       `json:"hour_of_day"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.AreaID == nil || wire.HourStart == nil || wire.TripCount == nil || wire.Weather == nil || wire.LocalDate == nil || wire.Season == nil || wire.DayOfWeek == nil || wire.HourOfDay == nil {
		return fmt.Errorf("missing mandatory observation field")
	}
	*o = Observation{Trip: Trip{*wire.AreaID, *wire.HourStart, *wire.TripCount}, Weather: *wire.Weather, LocalDate: *wire.LocalDate, Season: *wire.Season, DayOfWeek: *wire.DayOfWeek, HourOfDay: *wire.HourOfDay}
	return nil
}

func validateObservation(o Observation) error {
	if err := ValidateTrip(o.Trip); err != nil {
		return err
	}
	if err := ValidateWeather(o.Weather); err != nil {
		return err
	}
	if o.AreaID != o.Weather.AreaID || !o.HourStart.Equal(o.Weather.HourStart) {
		return fmt.Errorf("observation weather key mismatch")
	}
	if o.LocalDate.IsZero() || o.Season < 1 || o.Season > 4 || o.DayOfWeek < 1 || o.DayOfWeek > 7 || o.HourOfDay < 0 || o.HourOfDay > 23 {
		return fmt.Errorf("invalid observation calendar")
	}
	return nil
}
