// Package config reads and validates server environment settings.
package config

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"mobilitycontrolroom/internal/domain/recommendation"
)

// Config holds the validated settings used to wire the application.
type Config struct {
	ListenAddr, DatabaseURL    string
	ForecastURL                url.URL
	Scoring                    recommendation.Options
	ForecastHours, MaxAttempts int

	RecommendationRefresh, WeatherRefresh, SchedulerCheck, WeatherMaxStaleness time.Duration
	HTTPAttemptTimeout, ExternalRequestTimeout, RetryDelay                     time.Duration
	WeatherJobTimeout, HistoricalQueryTimeout, RecommendationJobTimeout        time.Duration
	DBTimeout, ShutdownTimeout, FrontendPollInterval                           time.Duration
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return Parse(os.LookupEnv)
}

// Parse accepts an environment lookup so configuration can be validated in isolation.
func Parse(lookup func(string) (string, bool)) (Config, error) {
	const datasetTimezone = "America/New_York"
	var c Config
	p := environmentParser{lookup: lookup}
	c.ListenAddr = p.get("LISTEN_ADDR", ":8080")
	c.DatabaseURL = p.get("DATABASE_URL", "")
	endpoint := p.get("FORECAST_API_URL", "https://api.open-meteo.com/v1/forecast")
	timezone := p.get("CITY_TIMEZONE", datasetTimezone)
	c.Scoring = recommendation.Options{
		Window:            p.duration("RECOMMENDATION_WINDOW", "2h"),
		MinSamples:        int64(p.integer("MIN_HISTORICAL_SAMPLES", "5")),
		RainThreshold:     p.number("MIN_RAINFALL_MM", "0.5"),
		AdditionalPerHour: p.number("MIN_ADDITIONAL_TRIPS_PER_HOUR", "50"),
	}
	c.ForecastHours = p.integer("WEATHER_FORECAST_HOURS", "30")
	c.MaxAttempts = p.integer("MAX_ATTEMPTS", "3")

	c.RecommendationRefresh = p.duration("RECOMMENDATION_REFRESH_INTERVAL", "1h")
	c.WeatherRefresh = p.duration("WEATHER_REFRESH_INTERVAL", "3h")
	c.SchedulerCheck = p.duration("SCHEDULER_CHECK_INTERVAL", "15m")
	c.WeatherMaxStaleness = p.duration("WEATHER_MAX_STALENESS", "6h")
	c.HTTPAttemptTimeout = p.duration("HTTP_ATTEMPT_TIMEOUT", "10s")
	c.ExternalRequestTimeout = p.duration("EXTERNAL_REQUEST_TIMEOUT", "20s")
	c.RetryDelay = p.duration("RETRY_DELAY", "1s")
	c.WeatherJobTimeout = p.duration("WEATHER_JOB_TIMEOUT", "2m")
	c.HistoricalQueryTimeout = p.duration("HISTORICAL_QUERY_TIMEOUT", "60s")
	c.RecommendationJobTimeout = p.duration("RECOMMENDATION_JOB_TIMEOUT", "30s")
	c.DBTimeout = p.duration("DB_TIMEOUT", "15s")
	c.ShutdownTimeout = p.duration("SHUTDOWN_TIMEOUT", "15s")
	c.FrontendPollInterval = p.duration("FRONTEND_POLL_INTERVAL", "60s")
	if p.err != nil {
		return c, p.err
	}
	if timezone != datasetTimezone {
		return c, fmt.Errorf("CITY_TIMEZONE must be %s to match the bundled historical data", datasetTimezone)
	}

	var err error
	c.Scoring.Location, err = time.LoadLocation(datasetTimezone)
	if err != nil {
		return c, fmt.Errorf("CITY_TIMEZONE: %w", err)
	}
	if err := c.Scoring.Validate(); err != nil {
		return c, err
	}
	// The API presents this threshold as a signed whole-trip count.
	if math.Round(c.Scoring.AdditionalPerHour*c.Scoring.Window.Hours()) >= 1<<63 {
		return c, fmt.Errorf("MIN_ADDITIONAL_TRIPS_PER_HOUR produces a threshold outside the supported trip range")
	}
	forecastURL, err := url.Parse(endpoint)
	if err != nil || forecastURL.Hostname() == "" || (forecastURL.Scheme != "http" && forecastURL.Scheme != "https") {
		return c, fmt.Errorf("FORECAST_API_URL must be an absolute HTTP or HTTPS URL")
	}
	c.ForecastURL = *forecastURL

	// The forecast starts in the current hour and planning starts in the next hour.
	const maxHours = int64(1<<63-1) / int64(time.Hour)
	if int64(c.ForecastHours) <= int64(c.Scoring.Window/time.Hour) || int64(c.ForecastHours) > maxHours {
		return c, fmt.Errorf("WEATHER_FORECAST_HOURS must cover the current hour and the recommendation window")
	}
	if c.MaxAttempts <= 0 {
		return c, fmt.Errorf("MAX_ATTEMPTS must be positive")
	}
	return c, nil
}

type environmentParser struct {
	lookup func(string) (string, bool)
	err    error
}

func (p *environmentParser) invalid(key, message string) {
	if p.err == nil {
		p.err = fmt.Errorf("%s %s", key, message)
	}
}

func (p *environmentParser) get(key, fallback string) string {
	value, ok := p.lookup(key)
	if !ok {
		value = fallback
	}
	if strings.TrimSpace(value) == "" {
		p.invalid(key, "is required")
	}
	return value
}

func (p *environmentParser) duration(key, fallback string) time.Duration {
	value, err := time.ParseDuration(p.get(key, fallback))
	switch {
	case err != nil:
		p.invalid(key, "must be a duration")
	case key == "FRONTEND_POLL_INTERVAL":
		// Kept for compatibility with existing environments. The server doesn't use it.
	case key == "RETRY_DELAY":
		if value < 0 {
			p.invalid(key, "must be nonnegative")
		}
	case value <= 0:
		p.invalid(key, "must be positive")
	}
	return value
}

func (p *environmentParser) integer(key, fallback string) int {
	value, err := strconv.Atoi(p.get(key, fallback))
	if err != nil {
		p.invalid(key, "must be an integer")
	}
	return value
}

func (p *environmentParser) number(key, fallback string) float64 {
	value, err := strconv.ParseFloat(p.get(key, fallback), 64)
	if err != nil {
		p.invalid(key, "must be a number")
	}
	return value
}
