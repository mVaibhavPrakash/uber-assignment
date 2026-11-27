// Package openmeteo fetches hourly forecasts from the Open-Meteo API.
package openmeteo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mobilitycontrolroom/internal/domain"

	"github.com/google/uuid"
)

// Options sets the requested hourly coverage and bounds HTTP retries.
type Options struct {
	ForecastHours, MaxAttempts                 int
	AttemptTimeout, RequestTimeout, RetryDelay time.Duration
}

// Client implements the Open-Meteo forecast provider.
type Client struct {
	endpoint url.URL
	options  Options
	http     *http.Client
}

// NewClient creates a provider using the given endpoint and HTTP client.
func NewClient(endpoint url.URL, client *http.Client, options Options) (*Client, error) {
	if client == nil {
		return nil, fmt.Errorf("weather HTTP client required")
	}
	if options.ForecastHours <= 0 || int64(options.ForecastHours) > int64(1<<63-1)/int64(time.Hour) ||
		options.MaxAttempts <= 0 || options.AttemptTimeout <= 0 || options.RequestTimeout <= 0 || options.RetryDelay < 0 {
		return nil, fmt.Errorf("invalid weather client options")
	}
	return &Client{endpoint: endpoint, options: options, http: client}, nil
}

// Fetch requests the monitored zones together and aligns their forecast hours.
func (c *Client) Fetch(ctx context.Context, areas []domain.Area, now time.Time) (domain.ForecastSnapshot, error) {
	if len(areas) == 0 {
		return domain.ForecastSnapshot{}, fmt.Errorf("no seeded weather areas")
	}

	latitudes, longitudes := make([]string, len(areas)), make([]string, len(areas))
	for i, a := range areas {
		latitudes[i] = strconv.FormatFloat(a.Latitude, 'f', -1, 64)
		longitudes[i] = strconv.FormatFloat(a.Longitude, 'f', -1, 64)
	}

	start := now.UTC().Truncate(time.Hour)
	end := start.Add(time.Duration(c.options.ForecastHours) * time.Hour)
	params := url.Values{
		"latitude":           {strings.Join(latitudes, ",")},
		"longitude":          {strings.Join(longitudes, ",")},
		"hourly":             {"rain,snowfall,precipitation_probability"},
		"timezone":           {"GMT"},
		"timeformat":         {"unixtime"},
		"precipitation_unit": {"mm"},
		"start_hour":         {start.Format("2006-01-02T15:04")},
		"end_hour":           {end.Format("2006-01-02T15:04")},
	}
	raw, err := c.requestWithRetries(ctx, params)
	if err != nil {
		return domain.ForecastSnapshot{}, err
	}

	hours, err := parseForecastHours(ctx, raw, areas, start, c.options.ForecastHours)
	if err != nil {
		return domain.ForecastSnapshot{}, err
	}

	return domain.ForecastSnapshot{Version: uuid.NewString(), FetchedAt: now.UTC().Truncate(time.Microsecond), Hours: hours}, nil
}
