// Package domain contains calendar and forecast rules.
package domain

import "time"

// NextFullHour is the UTC boundary shared by planning, forecast coverage and scheduling.
func NextFullHour(now time.Time) time.Time {
	return now.UTC().Truncate(time.Hour).Add(time.Hour)
}

// LocalWeekdayAndHour returns the ISO day of week (Monday=1) and local hour.
func LocalWeekdayAndHour(at time.Time, loc *time.Location) (day, hour int) {
	local := at.In(loc)
	day = int(local.Weekday())
	if day == 0 {
		day = 7
	}
	return day, local.Hour()
}
