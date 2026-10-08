package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMetricsTokens_FiltersPlaceholderValue(t *testing.T) {
	result := parseMetricsTokens("", "replace_with_metrics_bearer_token")
	assert.Empty(t, result, "placeholder METRICS_TOKEN must be filtered to empty")
}

func TestParseMetricsTokens_FiltersPlaceholderInMultiList(t *testing.T) {
	result := parseMetricsTokens("real-token-abc,replace_with_metrics_bearer_token,", "")
	assert.Equal(t, []string{"real-token-abc"}, result)
}

func TestParseMetricsTokens_PreservesRealToken(t *testing.T) {
	result := parseMetricsTokens("", "abc123def456ghi789jkl012mno345pqr678stu")
	assert.Equal(t, []string{"abc123def456ghi789jkl012mno345pqr678stu"}, result)
}

func TestParseMetricsTokens_FiltersPlaceholderPrefixVariants(t *testing.T) {
	result := parseMetricsTokens("changeme_metrics_token", "")
	assert.Empty(t, result)
	result = parseMetricsTokens("replace_with_something", "")
	assert.Empty(t, result)
}

func TestParseMetricsTokens_EmptyReturnsEmpty(t *testing.T) {
	result := parseMetricsTokens("", "")
	assert.Empty(t, result)
	result = parseMetricsTokens(",", "")
	assert.Empty(t, result)
}

func TestIsPlaceholderMetricsToken_CatchesKnownPlaceholders(t *testing.T) {
	assert.True(t, isPlaceholderMetricsToken("replace_with_metrics_bearer_token"))
	assert.True(t, isPlaceholderMetricsToken("changeme_metrics_token"))
	assert.True(t, isPlaceholderMetricsToken("replace_with_anything"))
	assert.False(t, isPlaceholderMetricsToken("abc123def456ghi789jkl012mno345pqr678stu"))
	assert.False(t, isPlaceholderMetricsToken(""))
}
