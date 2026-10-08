package database

import "testing"

// TestClampUpcomingReservationLimit guards the GetUpcomingReservations limit
// bound: a hand-edited ?limit must never produce an unbounded scan (limit<=0
// previously skipped the LIMIT clause entirely) nor an absurd cap. Same
// default-and-cap convention as the inventory movement list.
func TestClampUpcomingReservationLimit(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero defaults (was unbounded)", 0, defaultUpcomingReservationLimit},
		{"negative defaults", -5, defaultUpcomingReservationLimit},
		{"small passes through", 10, 10},
		{"at max passes through", maxUpcomingReservationLimit, maxUpcomingReservationLimit},
		{"just over max clamps", maxUpcomingReservationLimit + 1, maxUpcomingReservationLimit},
		{"huge clamps", 10_000_000, maxUpcomingReservationLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampUpcomingReservationLimit(tc.in); got != tc.want {
				t.Fatalf("clampUpcomingReservationLimit(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
