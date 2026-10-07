package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// labor_high carries pct (fraction) + amount (dollars); the digest email must
// render a real localized line, never the snake_case "Action needed" fallback.
func TestFormatInsightText_LaborHigh_EN(t *testing.T) {
	out := formatInsightText(DigestInsight{
		Type:   "labor_high",
		Params: map[string]interface{}{"pct": 0.42, "amount": 5300.0},
	}, "en")
	assert.Contains(t, out, "42%")
	assert.Contains(t, out, "$5300")
	assert.Contains(t, out, "Labor")
	assert.NotContains(t, out, "labor_high", "must not leak the raw insight type")
	assert.NotContains(t, out, "0 items", "must not render the count fallback")
}

func TestFormatInsightText_LaborHigh_ES(t *testing.T) {
	out := formatInsightText(DigestInsight{
		Type:   "labor_high",
		Params: map[string]interface{}{"pct": 0.40, "amount": 4000.0},
	}, "es")
	assert.Contains(t, out, "40%")
	assert.Contains(t, out, "$4000")
	assert.Contains(t, out, "costo laboral")
	assert.NotContains(t, out, "labor_high")
}

// es_ar falls back to Spanish (digest email is en/es-only by design).
func TestFormatInsightText_LaborHigh_EsArUsesSpanish(t *testing.T) {
	out := formatInsightText(DigestInsight{
		Type:   "labor_high",
		Params: map[string]interface{}{"pct": 0.38, "amount": 3000.0},
	}, "es_ar")
	assert.Contains(t, out, "costo laboral")
}
