package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildEnhanceImagePrompt(t *testing.T) {
	p := buildEnhanceImagePrompt("00000000", "Cheeseburger", "Beef, cheddar, brioche bun", nil)
	require.Contains(t, p, "Cheeseburger")
	require.Contains(t, p, "Beef, cheddar, brioche bun")
	// Must instruct the model to preserve the dish, not invent a new one.
	require.Contains(t, strings.ToLower(p), "keep the same dish")
	require.Contains(t, strings.ToLower(p), "do not invent a different dish")
}
