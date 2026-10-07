package handlers

import "testing"

// TestCentsToMicrounits verifies the integer-cents -> USDC micro-units
// conversion used to verify crypto settlements against the exact amount that is
// credited to the bill (1 cent = 10_000 micro-units, no float rounding).
func TestCentsToMicrounits(t *testing.T) {
	cases := []struct {
		cents int64
		want  int64
	}{
		{0, 0},
		{1, 10_000},
		{1234, 12_340_000},  // $12.34
		{100, 1_000_000},    // $1.00 == 1 USDC
		{99_99, 99_990_000}, // $99.99
	}
	for _, tc := range cases {
		if got := centsToMicrounits(tc.cents); got != tc.want {
			t.Errorf("centsToMicrounits(%d) = %d; want %d", tc.cents, got, tc.want)
		}
	}
}
