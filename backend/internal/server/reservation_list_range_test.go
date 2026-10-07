package server

import (
	"testing"
	"time"
)

// An undated reservation list used to cap at start+30d, hiding every booking
// made 31..max_advance_days out (guests can book up to max_advance_days, 45
// for some businesses) from all dashboard views until it drifted into range.
func TestDefaultReservationListEndHonorsMaxAdvanceDays(t *testing.T) {
	start := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)

	// Bookable horizon longer than 30d → the default window must cover it
	// (+1 day so the boundary day itself is fully included).
	if got, want := defaultReservationListEnd(start, 45), start.AddDate(0, 0, 46); !got.Equal(want) {
		t.Fatalf("45-day horizon: got %s, want %s", got, want)
	}

	// Short horizons keep the 30-day floor so "past + recent" views behave.
	if got, want := defaultReservationListEnd(start, 7), start.AddDate(0, 0, 30); !got.Equal(want) {
		t.Fatalf("7-day horizon: got %s, want %s", got, want)
	}

	if got, want := defaultReservationListEnd(start, 0), start.AddDate(0, 0, 30); !got.Equal(want) {
		t.Fatalf("zero horizon: got %s, want %s", got, want)
	}
}
