package config

import (
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"
	_ "time/tzdata"
)

type Config struct {
	ListenAddr, DatabaseURL                                                                                               string
	ForecastURL                                                                                                           string
	Location                                                                                                              *time.Location
	MinHistoricalSamples, ForecastHours, MaxAttempts                                                                      int
	MinRainfallMM, MinAdditionalTripsPerHour                                                                              float64
	Window, RecommendationRefresh, WeatherRefresh, SchedulerCheck, WeatherMaxStaleness                                    time.Duration
	HTTPAttemptTimeout, ExternalRequestTimeout, RetryDelay                                                                time.Duration
	WeatherJobTimeout, HistoricalQueryTimeout, RecommendationJobTimeout, DBTimeout, ShutdownTimeout, FrontendPollInterval time.Duration
}

func Load() (Config, error) { return Parse(os.LookupEnv) }

// Parse accepts an environment lookup so callers can validate isolated configs.
func Parse(lookup func(string) (string, bool)) (Config, error) {
	var c Config
	p := environmentParser{lookup: lookup}
	if err := p.loadConnections(&c); err != nil {
		return c, err
	}
	if err := p.loadScoringAndWeather(&c); err != nil {
		return c, err
	}
	if err := p.loadDurations(&c); err != nil {
		return c, err
	}
	return c, c.validate()
}

type environmentParser struct {
	lookup func(string) (string, bool)
	err    error
}

func (p *environmentParser) get(key, fallback string) string {
	if value, ok := p.lookup(key); ok {
		return value
	}
	return fallback
}

func (p *environmentParser) duration(key, fallback string) time.Duration {
	value, err := time.ParseDuration(p.get(key, fallback))
	if err != nil || value <= 0 {
		p.err = fmt.Errorf("%s must be a positive duration", key)
	}
	return value
}

func (p *environmentParser) integer(key, fallback string) int {
	value, err := strconv.Atoi(p.get(key, fallback))
	if err != nil || value <= 0 {
		p.err = fmt.Errorf("%s must be a positive integer", key)
	}
	return value
}

func (p *environmentParser) floatPointer(key, fallback string) float64 {
	value, err := strconv.ParseFloat(p.get(key, fallback), 64)
	if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		p.err = fmt.Errorf("%s must be finite and positive", key)
	}
	return value
}

func (p *environmentParser) loadConnections(c *Config) error {
	c.ListenAddr = p.get("LISTEN_ADDR", ":8080")
	if _, _, err := net.SplitHostPort(c.ListenAddr); err != nil {
		return fmt.Errorf("invalid LISTEN_ADDR: %w", err)
	}
	c.DatabaseURL = p.get("DATABASE_URL", "")
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return fmt.Errorf("DATABASE_URL must be a PostgreSQL URL")
	}
	if err := p.loadForecastURL(c); err != nil {
		return err
	}
	return p.loadTimezone(c)
}

func (p *environmentParser) loadForecastURL(c *Config) error {
	c.ForecastURL = p.get("FORECAST_API_URL", "https://api.open-meteo.com/v1/forecast")
	u, err := url.Parse(c.ForecastURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("FORECAST_API_URL must be an HTTP(S) URL")
	}
	return nil
}

func (p *environmentParser) loadTimezone(c *Config) error {
	var err error
	c.Location, err = time.LoadLocation(p.get("CITY_TIMEZONE", "America/New_York"))
	if err != nil {
		return fmt.Errorf("CITY_TIMEZONE: %w", err)
	}
	return nil
}

func (p *environmentParser) loadScoringAndWeather(c *Config) error {
	c.MinHistoricalSamples = p.integer("MIN_HISTORICAL_SAMPLES", "5")
	c.MinRainfallMM = p.floatPointer("MIN_RAINFALL_MM", "0.5")
	c.MinAdditionalTripsPerHour = p.floatPointer("MIN_ADDITIONAL_TRIPS_PER_HOUR", "50")
	c.ForecastHours = p.integer("WEATHER_FORECAST_HOURS", "30")
	if c.ForecastHours > 383 {
		return fmt.Errorf("WEATHER_FORECAST_HOURS must leave room for the extra boundary hour within the provider's 384-hour limit")
	}
	c.MaxAttempts = p.integer("MAX_ATTEMPTS", "3")
	return p.err
}

func (p *environmentParser) loadDurations(c *Config) error {
	for _, item := range []struct {
		key, fallback string
		target        *time.Duration
	}{
		{"RECOMMENDATION_WINDOW", "2h", &c.Window},
		{"RECOMMENDATION_REFRESH_INTERVAL", "1h", &c.RecommendationRefresh},
		{"WEATHER_REFRESH_INTERVAL", "3h", &c.WeatherRefresh},
		{"SCHEDULER_CHECK_INTERVAL", "15m", &c.SchedulerCheck},
		{"WEATHER_MAX_STALENESS", "6h", &c.WeatherMaxStaleness},
		{"HTTP_ATTEMPT_TIMEOUT", "10s", &c.HTTPAttemptTimeout},
		{"EXTERNAL_REQUEST_TIMEOUT", "20s", &c.ExternalRequestTimeout},
		{"RETRY_DELAY", "1s", &c.RetryDelay},
		{"WEATHER_JOB_TIMEOUT", "2m", &c.WeatherJobTimeout},
		{"HISTORICAL_QUERY_TIMEOUT", "60s", &c.HistoricalQueryTimeout},
		{"RECOMMENDATION_JOB_TIMEOUT", "30s", &c.RecommendationJobTimeout},
		{"DB_TIMEOUT", "15s", &c.DBTimeout},
		{"SHUTDOWN_TIMEOUT", "15s", &c.ShutdownTimeout},
		{"FRONTEND_POLL_INTERVAL", "60s", &c.FrontendPollInterval},
	} {
		*item.target = p.duration(item.key, item.fallback)
	}
	return p.err
}

func (c Config) validate() error {
	if c.Window%time.Hour != 0 {
		return fmt.Errorf("RECOMMENDATION_WINDOW must be whole hours")
	}
	// Compare without converting arbitrary user integers into overflowing durations.
	if float64(c.ForecastHours) < c.Window.Hours()+c.WeatherMaxStaleness.Hours()+2 {
		return fmt.Errorf("WEATHER_FORECAST_HOURS must cover window + max staleness + 2h")
	}
	if c.HTTPAttemptTimeout > c.ExternalRequestTimeout {
		return fmt.Errorf("HTTP_ATTEMPT_TIMEOUT must not exceed EXTERNAL_REQUEST_TIMEOUT")
	}
	if c.WeatherRefresh > c.WeatherMaxStaleness {
		return fmt.Errorf("WEATHER_REFRESH_INTERVAL must not exceed WEATHER_MAX_STALENESS")
	}
	return nil
}
