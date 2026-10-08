package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestServiceDayStart_MidnightDefault(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 11, 18, 27, 0, 0, loc)
	start := ServiceDayStart(now, loc, 0)
	assert.Equal(t, time.Date(2026, 8, 11, 0, 0, 0, 0, loc), start)
}

func TestServiceDayStart_FourAMCutoff(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// 1:30 AM still belongs to the previous service day when cutoff is 04:00.
	early := time.Date(2026, 8, 12, 1, 30, 0, 0, loc)
	start := ServiceDayStart(early, loc, 240)
	assert.Equal(t, time.Date(2026, 8, 11, 4, 0, 0, 0, loc), start)

	// After cutoff, same calendar morning is the new service day.
	after := time.Date(2026, 8, 12, 4, 15, 0, 0, loc)
	start = ServiceDayStart(after, loc, 240)
	assert.Equal(t, time.Date(2026, 8, 12, 4, 0, 0, 0, loc), start)
}

func TestClampServiceDayStartMinute(t *testing.T) {
	assert.Equal(t, 0, ClampServiceDayStartMinute(-1))
	assert.Equal(t, 0, ClampServiceDayStartMinute(1440))
	assert.Equal(t, 240, ClampServiceDayStartMinute(240))
}
