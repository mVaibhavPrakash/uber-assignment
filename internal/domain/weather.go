package domain

import (
	"errors"
	"time"
)

var (
	// ErrNoSnapshot indicates that no forecast has been published.
	ErrNoSnapshot = errors.New("no forecast snapshot")
	// ErrForecastExpired indicates that the snapshot is outside its allowed age.
	ErrForecastExpired = errors.New("forecast expired")
	// ErrForecastCoverage indicates that the snapshot cannot support the planning window.
	ErrForecastCoverage = errors.New("forecast does not cover the current recommendation window")
)

// ForecastHour holds the weather for a zone and one UTC hour interval.
type ForecastHour struct {
	AreaID                   int
	HourStart                time.Time
	RainMM                   float64
	SnowfallCM               float64
	PrecipitationProbability int
}

// ForecastSnapshot is a complete, versioned set of hourly forecasts.
type ForecastSnapshot struct {
	Version   string
	FetchedAt time.Time
	Hours     []ForecastHour
}

// ForecastStatus describes a snapshot without loading its hourly forecasts.
type ForecastStatus struct {
	Freshness string
	Version   string
	FetchedAt time.Time
}

// ForecastView includes the forecast hours when the snapshot is usable.
type ForecastView struct {
	ForecastStatus
	Snapshot *ForecastSnapshot
}
