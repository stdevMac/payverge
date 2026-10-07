package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePaymentAmountMagnitude(t *testing.T) {
	const ceiling int64 = 100_000_000
	cases := []struct {
		name      string
		amount    int64
		tip       int64
		wantError bool
	}{
		{"at ceiling ok", ceiling, 0, false},
		{"under ceiling ok", 4000, 500, false},
		{"amount one over rejected", ceiling + 1, 0, true},
		{"tip over rejected", 4000, ceiling + 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePaymentAmountMagnitude(tc.amount, tc.tip, ceiling)
			if tc.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "exceeds the maximum allowed payment amount")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestResolveMaxPaymentAmountCents_FallsBackOnBadEnv(t *testing.T) {
	for _, bad := range []string{"", "   ", "abc", "0", "-500"} {
		t.Setenv("MAX_PAYMENT_AMOUNT_CENTS", bad)
		assert.Equal(t, defaultMaxPaymentAmountCents, resolveMaxPaymentAmountCents())
	}
	t.Setenv("MAX_PAYMENT_AMOUNT_CENTS", "3000")
	assert.Equal(t, int64(3000), resolveMaxPaymentAmountCents())
}

func BenchmarkValidatePaymentAmountMagnitude(b *testing.B) {
	const ceiling int64 = 100_000_000
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// In-range path: the common case the production hot path takes.
		if err := validatePaymentAmountMagnitude(4000, 500, ceiling); err != nil {
			b.Fatal(err)
		}
	}
}
