package server

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// CLAIM-AI-002 / review-before-publish: ProcessMenuExtraction must not write
// live menu categories. Import is a separate authenticated operator action
// (FE MenuReviewEditor → ImportExtractedMenu).
func TestProcessMenuExtraction_DoesNotAutoImportToLiveMenu(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(".", "ai_menu_handlers.go"))
	require.NoError(t, err)
	// Extract ProcessMenuExtraction body (through next top-level func).
	re := regexp.MustCompile(`(?s)func ProcessMenuExtraction\(c \*gin\.Context\) \{.*?\nfunc `)
	m := re.Find(src)
	require.NotEmpty(t, m, "ProcessMenuExtraction not found")
	body := string(m)
	require.NotContains(t, body, "ImportExtractedMenu")
	require.NotContains(t, body, "AppendMenuCategories")
	require.Contains(t, body, "EnqueueMenuExtraction")
}

// CLAIM-AI-004: regenerate image returns a URL for the operator to apply; it
// must not mutate the published menu row in the same request.
func TestRegenerateMenuItemImage_ReturnsURLWithoutMenuMutation(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(".", "ai_menu_handlers.go"))
	require.NoError(t, err)
	re := regexp.MustCompile(`(?s)func RegenerateMenuItemImage\(c \*gin\.Context\) \{.*?\nfunc `)
	m := re.Find(src)
	require.NotEmpty(t, m)
	body := string(m)
	require.NotContains(t, body, "AppendMenuCategories")
	require.NotContains(t, body, "UpdateMenu(")
	require.Contains(t, body, `"url"`)
}
