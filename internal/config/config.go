package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

// Config holds settings loaded and validated by Load or Parse.
// Services receive copies, so setters do not reconfigure already-created services.
type Config struct {
	listenAddr  string
	databaseURL string
	forecastURL string
	location    *time.Location

	window                    time.Duration
	minHistoricalSamples      int
	minRainfallMM             float64
	minAdditionalTripsPerHour float64

	forecastHours         int
	maxAttempts           int
	retryDelay            time.Duration
	recommendationRefresh time.Duration
	weatherRefresh        time.Duration
	schedulerCheck        time.Duration
	weatherMaxStaleness   time.Duration
	frontendPollInterval  time.Duration

	httpAttemptTimeout       time.Duration
	externalRequestTimeout   time.Duration
	weatherJobTimeout        time.Duration
	historicalQueryTimeout   time.Duration
	recommendationJobTimeout time.Duration
	dbTimeout                time.Duration
	shutdownTimeout          time.Duration
}

// Setters update this config value; Load and Parse validate environment settings.
func (c Config) GetListenAddr() string                            { return c.listenAddr }
func (c *Config) SetListenAddr(value string)                      { c.listenAddr = value }
func (c Config) GetDatabaseURL() string                           { return c.databaseURL }
func (c *Config) SetDatabaseURL(value string)                     { c.databaseURL = value }
func (c Config) GetForecastURL() string                           { return c.forecastURL }
func (c *Config) SetForecastURL(value string)                     { c.forecastURL = value }
func (c Config) GetLocation() *time.Location                      { return c.location }
func (c *Config) SetLocation(value *time.Location)                { c.location = value }
func (c Config) GetMinHistoricalSamples() int                     { return c.minHistoricalSamples }
func (c *Config) SetMinHistoricalSamples(value int)               { c.minHistoricalSamples = value }
func (c Config) GetForecastHours() int                            { return c.forecastHours }
func (c *Config) SetForecastHours(value int)                      { c.forecastHours = value }
func (c Config) GetMaxAttempts() int                              { return c.maxAttempts }
func (c *Config) SetMaxAttempts(value int)                        { c.maxAttempts = value }
func (c Config) GetMinRainfallMM() float64                        { return c.minRainfallMM }
func (c *Config) SetMinRainfallMM(value float64)                  { c.minRainfallMM = value }
func (c Config) GetMinAdditionalTripsPerHour() float64            { return c.minAdditionalTripsPerHour }
func (c *Config) SetMinAdditionalTripsPerHour(value float64)      { c.minAdditionalTripsPerHour = value }
func (c Config) GetWindow() time.Duration                         { return c.window }
func (c *Config) SetWindow(value time.Duration)                   { c.window = value }
func (c Config) GetRecommendationRefresh() time.Duration          { return c.recommendationRefresh }
func (c *Config) SetRecommendationRefresh(value time.Duration)    { c.recommendationRefresh = value }
func (c Config) GetWeatherRefresh() time.Duration                 { return c.weatherRefresh }
func (c *Config) SetWeatherRefresh(value time.Duration)           { c.weatherRefresh = value }
func (c Config) GetSchedulerCheck() time.Duration                 { return c.schedulerCheck }
func (c *Config) SetSchedulerCheck(value time.Duration)           { c.schedulerCheck = value }
func (c Config) GetWeatherMaxStaleness() time.Duration            { return c.weatherMaxStaleness }
func (c *Config) SetWeatherMaxStaleness(value time.Duration)      { c.weatherMaxStaleness = value }
func (c Config) GetHTTPAttemptTimeout() time.Duration             { return c.httpAttemptTimeout }
func (c *Config) SetHTTPAttemptTimeout(value time.Duration)       { c.httpAttemptTimeout = value }
func (c Config) GetExternalRequestTimeout() time.Duration         { return c.externalRequestTimeout }
func (c *Config) SetExternalRequestTimeout(value time.Duration)   { c.externalRequestTimeout = value }
func (c Config) GetRetryDelay() time.Duration                     { return c.retryDelay }
func (c *Config) SetRetryDelay(value time.Duration)               { c.retryDelay = value }
func (c Config) GetWeatherJobTimeout() time.Duration              { return c.weatherJobTimeout }
func (c *Config) SetWeatherJobTimeout(value time.Duration)        { c.weatherJobTimeout = value }
func (c Config) GetHistoricalQueryTimeout() time.Duration         { return c.historicalQueryTimeout }
func (c *Config) SetHistoricalQueryTimeout(value time.Duration)   { c.historicalQueryTimeout = value }
func (c Config) GetRecommendationJobTimeout() time.Duration       { return c.recommendationJobTimeout }
func (c *Config) SetRecommendationJobTimeout(value time.Duration) { c.recommendationJobTimeout = value }
func (c Config) GetDBTimeout() time.Duration                      { return c.dbTimeout }
func (c *Config) SetDBTimeout(value time.Duration)                { c.dbTimeout = value }
func (c Config) GetShutdownTimeout() time.Duration                { return c.shutdownTimeout }
func (c *Config) SetShutdownTimeout(value time.Duration)          { c.shutdownTimeout = value }
func (c Config) GetFrontendPollInterval() time.Duration           { return c.frontendPollInterval }
func (c *Config) SetFrontendPollInterval(value time.Duration)     { c.frontendPollInterval = value }

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
	return c, nil
}

type environmentParser struct {
	lookup func(string) (string, bool)
	err    error
}

func (p *environmentParser) get(key, fallback string) string {
	value, ok := p.lookup(key)
	if !ok {
		value = fallback
	}
	if strings.TrimSpace(value) == "" {
		p.err = fmt.Errorf("%s is required", key)
	}
	return value
}

func (p *environmentParser) duration(key, fallback string) time.Duration {
	value, err := time.ParseDuration(p.get(key, fallback))
	if err != nil {
		p.err = fmt.Errorf("%s must be a duration", key)
	}
	return value
}

func (p *environmentParser) integer(key, fallback string) int {
	value, err := strconv.Atoi(p.get(key, fallback))
	if err != nil {
		p.err = fmt.Errorf("%s must be an integer", key)
	}
	return value
}

func (p *environmentParser) number(key, fallback string) float64 {
	value, err := strconv.ParseFloat(p.get(key, fallback), 64)
	if err != nil {
		p.err = fmt.Errorf("%s must be a number", key)
	}
	return value
}

func (p *environmentParser) loadConnections(c *Config) error {
	c.listenAddr = p.get("LISTEN_ADDR", ":8080")
	c.databaseURL = p.get("DATABASE_URL", "")
	c.forecastURL = p.get("FORECAST_API_URL", "https://api.open-meteo.com/v1/forecast")
	timezone := p.get("CITY_TIMEZONE", "America/New_York")
	if p.err != nil {
		return p.err
	}
	var err error
	c.location, err = time.LoadLocation(timezone)
	if err != nil {
		return fmt.Errorf("CITY_TIMEZONE: %w", err)
	}
	return nil
}

func (p *environmentParser) loadScoringAndWeather(c *Config) error {
	c.minHistoricalSamples = p.integer("MIN_HISTORICAL_SAMPLES", "5")
	c.minRainfallMM = p.number("MIN_RAINFALL_MM", "0.5")
	c.minAdditionalTripsPerHour = p.number("MIN_ADDITIONAL_TRIPS_PER_HOUR", "50")
	c.forecastHours = p.integer("WEATHER_FORECAST_HOURS", "30")
	c.maxAttempts = p.integer("MAX_ATTEMPTS", "3")
	return p.err
}

func (p *environmentParser) loadDurations(c *Config) error {
	c.window = p.duration("RECOMMENDATION_WINDOW", "2h")
	c.recommendationRefresh = p.duration("RECOMMENDATION_REFRESH_INTERVAL", "1h")
	c.weatherRefresh = p.duration("WEATHER_REFRESH_INTERVAL", "3h")
	c.schedulerCheck = p.duration("SCHEDULER_CHECK_INTERVAL", "15m")
	c.weatherMaxStaleness = p.duration("WEATHER_MAX_STALENESS", "6h")
	c.httpAttemptTimeout = p.duration("HTTP_ATTEMPT_TIMEOUT", "10s")
	c.externalRequestTimeout = p.duration("EXTERNAL_REQUEST_TIMEOUT", "20s")
	c.retryDelay = p.duration("RETRY_DELAY", "1s")
	c.weatherJobTimeout = p.duration("WEATHER_JOB_TIMEOUT", "2m")
	c.historicalQueryTimeout = p.duration("HISTORICAL_QUERY_TIMEOUT", "60s")
	c.recommendationJobTimeout = p.duration("RECOMMENDATION_JOB_TIMEOUT", "30s")
	c.dbTimeout = p.duration("DB_TIMEOUT", "15s")
	c.shutdownTimeout = p.duration("SHUTDOWN_TIMEOUT", "15s")
	c.frontendPollInterval = p.duration("FRONTEND_POLL_INTERVAL", "60s")
	return p.err
}
