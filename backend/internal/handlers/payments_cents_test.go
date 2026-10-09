package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCentsToDollarsMatchesWireContract(t *testing.T) {
	cases := []struct {
		cents int64
		want  float64
	}{
		{0, 0},
		{4550, 45.50},
		{1, 0.01},
		{100000, 1000.0},
		{-250, -2.50},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, centsToDollars(tc.cents), "centsToDollars(%d)", tc.cents)
		// Equivalence with the open-coded form the handlers are being migrated off of.
		assert.Equal(t, float64(tc.cents)/100.0, centsToDollars(tc.cents))
	}
}
