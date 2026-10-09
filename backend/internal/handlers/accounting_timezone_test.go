package handlers

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeParseDateRangeContext builds a gin.Context with start/end query params for
// parseDateRange testing.
func makeParseDateRangeContext(start, end string) *gin.Context {
	req := httptest.NewRequest("GET", "/test?start="+start+"&end="+end, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c
}

// TestParseDateRange_TimezonePDT verifies that parsing 2026-04-15 in
// America/Los_Angeles produces a half-open window [midnight PDT that day,
// midnight PDT next day) expressed in UTC. PDT (DST) is UTC-7, so midnight PDT
// == 07:00 UTC.
func TestParseDateRange_TimezonePDT(t *testing.T) {
	gin.SetMode(gin.TestMode)

	business := &database.Business{Timezone: "America/Los_Angeles"}
	c := makeParseDateRangeContext("2026-04-15", "2026-04-15")

	start, end, ok := parseDateRange(c, business)
	require.True(t, ok, "parseDateRange should succeed for valid input")

	expectedStart := time.Date(2026, time.April, 15, 7, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, time.April, 16, 7, 0, 0, 0, time.UTC)

	assert.True(t, start.Equal(expectedStart), "start = %s, want %s", start, expectedStart)
	assert.True(t, end.Equal(expectedEnd), "end = %s, want %s", end, expectedEnd)
}

// TestParseDateRange_TimezoneInvalidFallsBackToUTC ensures a bogus timezone
// string on the business does not crash and falls back to UTC.
func TestParseDateRange_TimezoneInvalidFallsBackToUTC(t *testing.T) {
	gin.SetMode(gin.TestMode)

	business := &database.Business{Timezone: "Not/A_Real_Zone"}
	c := makeParseDateRangeContext("2026-04-15", "2026-04-15")

	start, end, ok := parseDateRange(c, business)
	require.True(t, ok, "parseDateRange should not crash on invalid timezone")

	expectedStart := time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, time.April, 16, 0, 0, 0, 0, time.UTC)

	assert.True(t, start.Equal(expectedStart), "start = %s, want %s", start, expectedStart)
	assert.True(t, end.Equal(expectedEnd), "end = %s, want %s", end, expectedEnd)
}

// TestParseDateRange_EmptyTimezoneFallsBackToUTC verifies that an unset
// timezone field falls back to UTC (same as the reservation handler pattern).
func TestParseDateRange_EmptyTimezoneFallsBackToUTC(t *testing.T) {
	gin.SetMode(gin.TestMode)

	business := &database.Business{Timezone: ""}
	c := makeParseDateRangeContext("2026-04-15", "2026-04-15")

	start, end, ok := parseDateRange(c, business)
	require.True(t, ok)

	expectedStart := time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, time.April, 16, 0, 0, 0, 0, time.UTC)

	assert.True(t, start.Equal(expectedStart))
	assert.True(t, end.Equal(expectedEnd))
}
