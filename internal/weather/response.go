package weather

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mobilitycontrolroom/internal/repository"
	"time"
)

type providerResponse struct {
	LocationID *int `json:"location_id"`
	UTCOffset  *int `json:"utc_offset_seconds"`
	Units      struct {
		Time        string `json:"time"`
		Rain        string `json:"rain"`
		Snow        string `json:"snowfall"`
		Probability string `json:"precipitation_probability"`
	} `json:"hourly_units"`
	Hourly struct {
		Time        []*int64   `json:"time"`
		Rain        []*float64 `json:"rain"`
		Snow        []*float64 `json:"snowfall"`
		Probability []*int     `json:"precipitation_probability"`
	} `json:"hourly"`
}

// Matches each location and hourly weather value to our zone and time.
func parseForecastHours(raw []byte, areas []repository.Area, start time.Time, forecastHours int) ([]repository.ForecastHour, error) {
	// Step 1: Read either one location object or a list of location objects.
	responses, err := decodeProviderLocations(raw)
	if err != nil {
		return nil, err
	}
	if len(responses) != len(areas) {
		return nil, fmt.Errorf("forecast response count %d differs from requested locations %d", len(responses), len(areas))
	}

	// Step 2: Match locations in request order and collect their hourly values.
	rows := make([]repository.ForecastHour, 0, len(areas)*forecastHours)
	for i, response := range responses {
		if err := response.validateLocationOrderAndUnits(i); err != nil {
			return nil, err
		}
		hours, err := response.alignForecastHours(areas[i].ID, start, forecastHours)
		if err != nil {
			return nil, err
		}
		rows = append(rows, hours...)
	}
	return rows, nil
}

// Handles the API's single-location and multiple-location formats.
func decodeProviderLocations(raw []byte) ([]providerResponse, error) {
	var err error
	raw = bytes.TrimSpace(raw)
	var responses []providerResponse
	if len(raw) > 0 && raw[0] == '[' {
		err = json.Unmarshal(raw, &responses)
	} else {
		var response providerResponse
		err = json.Unmarshal(raw, &response)
		responses = []providerResponse{response}
	}
	if err != nil {
		return nil, fmt.Errorf("invalid forecast JSON: %w", err)
	}
	return responses, nil
}

// Checks the location order and the units requested from the API.
func (response providerResponse) validateLocationOrderAndUnits(index int) error {
	if response.LocationID != nil && *response.LocationID != index {
		return fmt.Errorf("forecast location order mismatch")
	}
	if response.UTCOffset == nil || *response.UTCOffset != 0 || response.Units.Time != "unixtime" || response.Units.Rain != "mm" || response.Units.Snow != "cm" || response.Units.Probability != "%" {
		return fmt.Errorf("unexpected forecast timezone or units")
	}
	return nil
}

// Assigns each weather value to the hour it describes for one zone.
func (response providerResponse) alignForecastHours(areaID int, start time.Time, forecastHours int) ([]repository.ForecastHour, error) {
	h := response.Hourly
	n := len(h.Time)
	if n == 0 || len(h.Rain) != n || len(h.Snow) != n || len(h.Probability) != n {
		return nil, fmt.Errorf("incomplete forecast parallel arrays")
	}
	positions, err := indexContiguousHourlyTimestamps(h.Time)
	if err != nil {
		return nil, err
	}
	rows := make([]repository.ForecastHour, 0, forecastHours)
	// The API labels these values at the end of the hour. For example,
	// values labelled 11:00 describe the interval starting at 10:00.
	for hour := range forecastHours {
		at := start.Add(time.Duration(hour) * time.Hour)
		index, ok := positions[at.Add(time.Hour).Unix()]
		if !ok || h.Rain[index] == nil || h.Snow[index] == nil || h.Probability[index] == nil {
			return nil, fmt.Errorf("missing mandatory forecast hour for area %d", areaID)
		}
		rows = append(rows, repository.ForecastHour{AreaID: areaID, HourStart: at, RainMM: *h.Rain[index], SnowfallCM: *h.Snow[index], PrecipitationProbability: *h.Probability[index]})
	}
	return rows, nil
}

// Checks that timestamps are complete, hourly and in order,
// then records where each timestamp appears in the response arrays.
func indexContiguousHourlyTimestamps(times []*int64) (map[int64]int, error) {
	positions := make(map[int64]int, len(times))
	for i, at := range times {
		if at == nil || *at%3600 != 0 || i > 0 && *at-*times[i-1] != 3600 {
			return nil, fmt.Errorf("forecast timestamps missing, unaligned or unordered")
		}
		positions[*at] = i
	}
	return positions, nil
}
