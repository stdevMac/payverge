package database

import "time"

// ClampServiceDayStartMinute keeps a service-day cutoff in [0, 1439].
func ClampServiceDayStartMinute(minute int) int {
	if minute < 0 || minute >= 24*60 {
		return 0
	}
	return minute
}

// ServiceDayStart returns the start instant of the service day containing t
// in loc. When startMinute is 0 this is local midnight; when startMinute is
// 240 (04:00), a 01:30 close still belongs to the previous service day.
func ServiceDayStart(t time.Time, loc *time.Location, startMinute int) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	startMinute = ClampServiceDayStartMinute(startMinute)
	local := t.In(loc)
	hour := startMinute / 60
	min := startMinute % 60
	start := time.Date(local.Year(), local.Month(), local.Day(), hour, min, 0, 0, loc)
	if local.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	return start
}
