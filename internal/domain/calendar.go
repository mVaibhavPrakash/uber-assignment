// Package domain contains storage-independent calendar rules shared by offline
// ingestion and runtime historical lookup.
package domain

import "time"

// Calendar returns the local date (encoded as a UTC date), meteorological
// season (winter=1), ISO weekday (Monday=1) and local hour.
func Calendar(at time.Time, loc *time.Location) (date time.Time, season, day, hour int) {
	local := at.In(loc)
	date = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	season = int(local.Month())%12/3 + 1
	day = int(local.Weekday())
	if day == 0 {
		day = 7
	}
	hour = local.Hour()
	return
}
