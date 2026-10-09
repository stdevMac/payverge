package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// L2-34: drive the shipped normalizeAndValidateCounterPrefix used by
// UpdateCounterSettings — not a reimplemented gin stub.

func TestNormalizeAndValidateCounterPrefix_RejectsWhitespaceOnly(t *testing.T) {
	_, err := normalizeAndValidateCounterPrefix("   ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "counter_prefix is required")

	_, err = normalizeAndValidateCounterPrefix("\t\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "counter_prefix is required")

	_, err = normalizeAndValidateCounterPrefix("")
	require.Error(t, err)
}

func TestNormalizeAndValidateCounterPrefix_TrimsThenAcceptsWithinMax(t *testing.T) {
	// Untrimmed length is 7; after trim "BAR" (3) must pass max=5.
	got, err := normalizeAndValidateCounterPrefix("  BAR  ")
	require.NoError(t, err)
	assert.Equal(t, "BAR", got)

	got, err = normalizeAndValidateCounterPrefix("ABCDE")
	require.NoError(t, err)
	assert.Equal(t, "ABCDE", got)
}

func TestNormalizeAndValidateCounterPrefix_RejectsTooLongAfterTrim(t *testing.T) {
	_, err := normalizeAndValidateCounterPrefix("TOOLONG")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 5")

	// Spaces outside a 6-char core still fail after trim.
	_, err = normalizeAndValidateCounterPrefix("  ABCDEF  ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 5")
}
