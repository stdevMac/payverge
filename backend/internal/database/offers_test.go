package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func scheduleIntPtr(value int) *int { return &value }

func TestOfferActiveAt(t *testing.T) {
	local := func(weekday time.Weekday, hour, minute int) time.Time {
		base := time.Date(2026, time.July, 19, hour, minute, 0, 0, time.UTC) // Sunday.
		return base.AddDate(0, 0, int(weekday))
	}

	tests := []struct {
		name       string
		mask       int16
		start, end *int
		now        time.Time
		want       bool
	}{
		{name: "legacy zero mask is all day every day", mask: 0, now: local(time.Saturday, 12, 0), want: true},
		{name: "saturday excluded", mask: 1 << uint(time.Monday), now: local(time.Saturday, 12, 0), want: false},
		{name: "same day starts inclusively", mask: 1 << uint(time.Monday), start: scheduleIntPtr(600), end: scheduleIntPtr(720), now: local(time.Monday, 10, 0), want: true},
		{name: "same day end is exclusive", mask: 1 << uint(time.Monday), start: scheduleIntPtr(600), end: scheduleIntPtr(720), now: local(time.Monday, 12, 0), want: false},
		{name: "friday overnight at saturday one", mask: 1 << uint(time.Friday), start: scheduleIntPtr(1320), end: scheduleIntPtr(120), now: local(time.Saturday, 1, 0), want: true},
		{name: "overnight uses start day mask", mask: 1 << uint(time.Saturday), start: scheduleIntPtr(1320), end: scheduleIntPtr(120), now: local(time.Saturday, 1, 0), want: false},
		{name: "before overnight start is inactive", mask: 1 << uint(time.Friday), start: scheduleIntPtr(1320), end: scheduleIntPtr(120), now: local(time.Friday, 21, 59), want: false},
		// #78: weekday lunch 11:00–15:00 is exclusive at the end.
		{name: "weekday lunch at noon", mask: 62, start: scheduleIntPtr(11 * 60), end: scheduleIntPtr(15 * 60), now: local(time.Wednesday, 12, 0), want: true},
		{name: "weekday lunch at 15:14 is over", mask: 62, start: scheduleIntPtr(11 * 60), end: scheduleIntPtr(15 * 60), now: local(time.Wednesday, 15, 14), want: false},
		{name: "nil minutes is all day on the weekday", mask: 62, now: local(time.Wednesday, 15, 14), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer := Offer{WeekdayMask: tt.mask, StartMinute: tt.start, EndMinute: tt.end}
			assert.Equal(t, tt.want, OfferActiveAt(offer, tt.now))
		})
	}
}

func TestOfferScheduleReferenceUsesBusinessTimezone(t *testing.T) {
	at := time.Date(2026, time.July, 20, 1, 30, 0, 0, time.UTC)
	reference := offerScheduleReference(at, "America/New_York")

	assert.Equal(t, time.Sunday, reference.Weekday())
	assert.Equal(t, 21, reference.Hour())
	assert.Equal(t, 30, reference.Minute())
	assert.True(t, OfferActiveAt(Offer{
		WeekdayMask: 1 << uint(time.Sunday),
		StartMinute: scheduleIntPtr(21 * 60),
		EndMinute:   scheduleIntPtr(22 * 60),
	}, reference))
}

func TestOfferScheduleReferenceHandlesDSTTransition(t *testing.T) {
	// New York jumps from 01:59 EST to 03:00 EDT on 2026-03-08.
	at := time.Date(2026, time.March, 8, 7, 30, 0, 0, time.UTC)
	reference := offerScheduleReference(at, "America/New_York")
	_, offset := reference.Zone()

	assert.Equal(t, 3, reference.Hour())
	assert.Equal(t, 30, reference.Minute())
	assert.Equal(t, -4*60*60, offset)
	assert.True(t, OfferActiveAt(Offer{
		WeekdayMask: 1 << uint(time.Sunday),
		StartMinute: scheduleIntPtr(3 * 60),
		EndMinute:   scheduleIntPtr(4 * 60),
	}, reference))
}

func TestOfferScheduleReferenceInvalidTimezoneFallsBackToUTC(t *testing.T) {
	callerLocation := time.FixedZone("caller", -4*60*60)
	at := time.Date(2026, time.July, 19, 23, 30, 0, 0, callerLocation)
	reference := offerScheduleReference(at, "Not/A_Real_Timezone")

	assert.Same(t, time.UTC, reference.Location())
	assert.Equal(t, time.Monday, reference.Weekday())
	assert.Equal(t, 3, reference.Hour())
	assert.Equal(t, 30, reference.Minute())
	assert.True(t, OfferActiveAt(Offer{
		WeekdayMask: 1 << uint(time.Monday),
		StartMinute: scheduleIntPtr(3 * 60),
		EndMinute:   scheduleIntPtr(4 * 60),
	}, reference))
}
