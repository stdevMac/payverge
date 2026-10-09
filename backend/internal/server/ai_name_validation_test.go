package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAndValidateAiName(t *testing.T) {
	t.Run("trims whitespace", func(t *testing.T) {
		got, err := NormalizeAndValidateAiName("  Sage  ")
		require.NoError(t, err)
		assert.Equal(t, "Sage", got)
	})

	t.Run("empty after trim is allowed", func(t *testing.T) {
		got, err := NormalizeAndValidateAiName("   ")
		require.NoError(t, err)
		assert.Equal(t, "", got)
	})

	t.Run("rejects control characters", func(t *testing.T) {
		_, err := NormalizeAndValidateAiName("Sage\x00Evil")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "control")
	})

	t.Run("rejects overlong names", func(t *testing.T) {
		long := strings.Repeat("a", MaxAiNameLen+1)
		_, err := NormalizeAndValidateAiName(long)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at most")
	})

	t.Run("accepts max length", func(t *testing.T) {
		exact := strings.Repeat("á", MaxAiNameLen) // rune-aware
		got, err := NormalizeAndValidateAiName(exact)
		require.NoError(t, err)
		assert.Equal(t, exact, got)
	})
}
