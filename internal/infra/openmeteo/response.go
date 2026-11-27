package openmeteo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"mobilitycontrolroom/internal/domain"
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
func parseForecastHours(ctx context.Context, raw []byte, areas []domain.Area, start time.Time, forecastHours int) ([]domain.ForecastHour, error) {
	responses, err := decodeProviderLocations(raw)
	if err != nil {
		return nil, err
	}
	if len(responses) != len(areas) {
		return nil, fmt.Errorf("forecast response count %d differs from requested locations %d", len(responses), len(areas))
	}

	var rows []domain.ForecastHour
	for i, response := range responses {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := response.validateLocationOrderAndUnits(i); err != nil {
			return nil, fmt.Errorf("forecast area %d: %w", areas[i].ID, err)
		}
		rows, err = response.appendForecastHours(rows, areas[i].ID, start, forecastHours)
		if err != nil {
			return nil, fmt.Errorf("forecast area %d: %w", areas[i].ID, err)
		}
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
func (response providerResponse) appendForecastHours(rows []domain.ForecastHour, areaID int, start time.Time, forecastHours int) ([]domain.ForecastHour, error) {
	h := response.Hourly
	n := len(h.Time)
	if n < forecastHours || len(h.Rain) != n || len(h.Snow) != n || len(h.Probability) != n {
		return nil, fmt.Errorf("incomplete forecast parallel arrays")
	}

	// The API labels values at the end of the hour. For example, 11:00
	// describes the interval starting at 10:00.
	firstEnd := start.Add(time.Hour).Unix()
	firstIndex := -1
	for i, at := range h.Time {
		if at == nil || *at%3600 != 0 || (i > 0 && (*at <= *h.Time[i-1] || *at-*h.Time[i-1] != 3600)) {
			return nil, fmt.Errorf("forecast timestamps missing, unaligned or unordered")
		}
		if *at == firstEnd {
			firstIndex = i
		}
	}
	if firstIndex < 0 || forecastHours > n-firstIndex {
		return nil, fmt.Errorf("missing mandatory forecast hours")
	}
	for hour := range forecastHours {
		index := firstIndex + hour
		if h.Rain[index] == nil || h.Snow[index] == nil || h.Probability[index] == nil {
			return nil, fmt.Errorf("missing mandatory forecast hour")
		}
		rows = append(rows, domain.ForecastHour{
			AreaID: areaID, HourStart: start.Add(time.Duration(hour) * time.Hour),
			RainMM: *h.Rain[index], SnowfallCM: *h.Snow[index], PrecipitationProbability: *h.Probability[index],
		})
	}
	return rows, nil
}
