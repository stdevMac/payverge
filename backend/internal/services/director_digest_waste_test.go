package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// waste_high carries amount/ingredient (not count/item_names); the digest email
// must render a real localized line, never the snake_case "Action needed" fallback.
func TestFormatInsightText_WasteHigh_EN(t *testing.T) {
	out := formatInsightText(DigestInsight{
		Type:   "waste_high",
		Params: map[string]interface{}{"amount": 847.0, "ingredient": "Salmon"},
	}, "en")
	assert.Contains(t, out, "$847")
	assert.Contains(t, out, "Salmon")
	assert.NotContains(t, out, "waste_high", "must not leak the raw insight type")
	assert.NotContains(t, out, "0 items", "must not render the count fallback")
}

func TestFormatInsightText_WasteHigh_ES(t *testing.T) {
	out := formatInsightText(DigestInsight{
		Type:   "waste_high",
		Params: map[string]interface{}{"amount": 500.0, "ingredient": "Salmón"},
	}, "es")
	assert.Contains(t, out, "$500")
	assert.Contains(t, out, "Salmón")
	assert.Contains(t, out, "pérdida")
	assert.NotContains(t, out, "waste_high")
}

// es_ar falls back to Spanish (digest email is en/es-only by design).
func TestFormatInsightText_WasteHigh_EsArUsesSpanish(t *testing.T) {
	out := formatInsightText(DigestInsight{
		Type:   "waste_high",
		Params: map[string]interface{}{"amount": 300.0, "ingredient": "Carne"},
	}, "es_ar")
	assert.Contains(t, out, "pérdida")
}

func TestFormatInsightText_WasteHigh_EscapesIngredient(t *testing.T) {
	out := formatInsightText(DigestInsight{
		Type:   "waste_high",
		Params: map[string]interface{}{"amount": 100.0, "ingredient": "<script>x</script>"},
	}, "en")
	assert.NotContains(t, out, "<script>", "ingredient must be HTML-escaped")
	assert.True(t, strings.Contains(out, "&lt;script&gt;"))
}
