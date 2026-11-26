// Package weather fetches and publishes complete live forecast snapshots.
package weather

import (
	"context"

	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mobilitycontrolroom/internal/config"
	"mobilitycontrolroom/internal/repository"

	"github.com/google/uuid"
)

type Client struct {
	config config.Config
	http   *http.Client
}

// NewClient sets up the endpoint caller. It does not store data or track API quotas.
func NewClient(c config.Config, h *http.Client) (*Client, error) {
	if h == nil {
		return nil, fmt.Errorf("weather HTTP client required")
	}
	return &Client{config: c, http: h}, nil
}

// Requests the seeded zones together and returns a forecast snapshot.
// The weather service handles database storage and keeping the previous forecast on failure.
func (c *Client) Fetch(ctx context.Context, areas []repository.Area, now time.Time) (repository.ForecastSnapshot, error) {
	if len(areas) == 0 {
		return repository.ForecastSnapshot{}, fmt.Errorf("no seeded weather areas")
	}

	latitudes, longitudes := make([]string, len(areas)), make([]string, len(areas))
	for i, a := range areas {
		latitudes[i] = strconv.FormatFloat(a.Latitude, 'f', -1, 64)
		longitudes[i] = strconv.FormatFloat(a.Longitude, 'f', -1, 64)
	}

	start := now.UTC().Truncate(time.Hour)
	params := url.Values{"latitude": {strings.Join(latitudes, ",")}, "longitude": {strings.Join(longitudes, ",")}, "hourly": {"rain,snowfall,precipitation_probability"}, "timezone": {"GMT"}, "timeformat": {"unixtime"}, "precipitation_unit": {"mm"}, "start_hour": {start.Format("2006-01-02T15:04")}, "end_hour": {start.Add(time.Duration(c.config.GetForecastHours()) * time.Hour).Format("2006-01-02T15:04")}}
	raw, err := c.requestWithRetries(ctx, params)
	if err != nil {
		return repository.ForecastSnapshot{}, err
	}

	hours, err := parseForecastHours(raw, areas, start, c.config.GetForecastHours())
	if err != nil {
		return repository.ForecastSnapshot{}, err
	}

	return repository.ForecastSnapshot{Version: uuid.NewString(), FetchedAt: now.UTC().Truncate(time.Microsecond), Hours: hours}, nil
}
