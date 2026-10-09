package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ──────────────────────────────────────────────────
// verifyPaymentWebhookSignature
// ──────────────────────────────────────────────────

// ──────────────────────────────────────────────────
// parsePaymentWebhookBillID
// ──────────────────────────────────────────────────

// ──────────────────────────────────────────────────
// normalizeContextUint
// ──────────────────────────────────────────────────

func TestNormalizeContextUint_Uint(t *testing.T) {
	val, ok := normalizeContextUint(uint(42))
	assert.True(t, ok)
	assert.Equal(t, uint(42), val)
}

func TestNormalizeContextUint_Uint64(t *testing.T) {
	val, ok := normalizeContextUint(uint64(100))
	assert.True(t, ok)
	assert.Equal(t, uint(100), val)
}

func TestNormalizeContextUint_Int(t *testing.T) {
	val, ok := normalizeContextUint(int(5))
	assert.True(t, ok)
	assert.Equal(t, uint(5), val)
}

func TestNormalizeContextUint_NegativeInt(t *testing.T) {
	_, ok := normalizeContextUint(int(-1))
	assert.False(t, ok)
}

func TestNormalizeContextUint_Int64(t *testing.T) {
	val, ok := normalizeContextUint(int64(200))
	assert.True(t, ok)
	assert.Equal(t, uint(200), val)
}

func TestNormalizeContextUint_NegativeInt64(t *testing.T) {
	_, ok := normalizeContextUint(int64(-10))
	assert.False(t, ok)
}

func TestNormalizeContextUint_Float64(t *testing.T) {
	val, ok := normalizeContextUint(float64(3.0))
	assert.True(t, ok)
	assert.Equal(t, uint(3), val)
}

func TestNormalizeContextUint_NegativeFloat64(t *testing.T) {
	_, ok := normalizeContextUint(float64(-1.5))
	assert.False(t, ok)
}

func TestNormalizeContextUint_Float64Truncates(t *testing.T) {
	// float64(3.9) becomes uint(3) — truncation, not rounding
	val, ok := normalizeContextUint(float64(3.9))
	assert.True(t, ok)
	assert.Equal(t, uint(3), val)
}

func TestNormalizeContextUint_String(t *testing.T) {
	val, ok := normalizeContextUint("42")
	assert.True(t, ok)
	assert.Equal(t, uint(42), val)
}

func TestNormalizeContextUint_StringWithWhitespace(t *testing.T) {
	val, ok := normalizeContextUint("  77  ")
	assert.True(t, ok)
	assert.Equal(t, uint(77), val)
}

func TestNormalizeContextUint_InvalidString(t *testing.T) {
	_, ok := normalizeContextUint("not-a-number")
	assert.False(t, ok)
}

func TestNormalizeContextUint_EmptyString(t *testing.T) {
	_, ok := normalizeContextUint("")
	assert.False(t, ok)
}

func TestNormalizeContextUint_NilValue(t *testing.T) {
	_, ok := normalizeContextUint(nil)
	assert.False(t, ok)
}

func TestNormalizeContextUint_Bool(t *testing.T) {
	_, ok := normalizeContextUint(true)
	assert.False(t, ok)
}

func TestNormalizeContextUint_ZeroValues(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  uint
		ok    bool
	}{
		{"uint(0)", uint(0), 0, true},
		{"int(0)", int(0), 0, true},
		{"float64(0)", float64(0.0), 0, true},
		{"string 0", "0", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := normalizeContextUint(tt.input)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, val)
			}
		})
	}
}

// ──────────────────────────────────────────────────
// getStringFromContext (via table-driven tests)
// ──────────────────────────────────────────────────

// Note: getStringFromContext requires a gin.Context, which is harder to unit test
// without spinning up a gin router. We test normalizeContextUint thoroughly instead.
