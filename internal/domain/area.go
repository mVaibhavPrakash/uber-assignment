package domain

// Identifies a monitored zone and its forecast coordinates.
type Area struct {
	ID        int
	Name      string
	Latitude  float64
	Longitude float64
}

// Compares completed trips by rainfall for one local calendar hour.
// A nil estimate means unknown. RainStdDev describes sample dispersion.
type HourlyStats struct {
	AreaID, DayOfWeek, HourOfDay                 int
	DryMean, RainMean                            *float64
	DrySamples, RainSamples                      int64
	AbsoluteUplift, PercentageUplift, RainStdDev *float64
}
