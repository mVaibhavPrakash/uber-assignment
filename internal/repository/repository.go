// Package repository defines storage-independent domain contracts.
package repository

import (
	"context"

	"time"
)

type Area struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type HistoricalQuery struct{ MinRainfallMM float64 }

// Nil means unknown; a pointer to zero is an observed zero mean.
type HourlyStats struct {
	AreaID, Season, DayOfWeek, HourOfDay int
	// Dry fields retain their API names but represent all rain below the configured threshold.
	DryMean, RainMean       *float64
	DrySamples, RainSamples int64
	// Historical comparisons are signed. RainStdDev is sample dispersion of
	// rainy trip counts, not uncertainty of the mean or a causal estimate.
	AbsoluteUplift, PercentageUplift, RainStdDev *float64
}

type ForecastHour struct {
	AreaID                   int       `json:"area_id"`
	HourStart                time.Time `json:"hour_start"`
	RainMM                   float64   `json:"rain_mm"`
	SnowfallCM               float64   `json:"snowfall_cm"`
	PrecipitationProbability int       `json:"precipitation_probability"`
}

// Implementations return caller-owned slices. ReplaceSnapshot publishes all rows
// atomically; failure must preserve the prior committed version.
type ForecastSnapshot struct {
	Version   string         `json:"version"`
	FetchedAt time.Time      `json:"fetched_at"`
	Hours     []ForecastHour `json:"hours"`
}

type AreaRepository interface {
	ListAreas(context.Context) ([]Area, error)
}
type HistoricalRepository interface {
	AggregateHourly(context.Context, HistoricalQuery) ([]HourlyStats, error)
}
type ForecastRepository interface {
	LoadLatest(context.Context) (ForecastSnapshot, error)
	ReplaceSnapshot(context.Context, ForecastSnapshot) error
}
