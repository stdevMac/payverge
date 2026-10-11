package services

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/services/director_tools"

	"github.com/stretchr/testify/assert"
)

// pillLocale must collapse region/family Spanish (es-AR, es_ar) to the base
// "es" the tool HumanLabel switches understand, so Argentine operators see the
// Spanish loading pill instead of the English default.
func TestPillLocaleCollapsesToBaseSubtag(t *testing.T) {
	cases := map[string]string{
		"es-AR": "es",
		"es_ar": "es",
		"ES-AR": "es",
		"es":    "es",
		"en":    "en",
		"fr":    "fr",
		"ar":    "ar",
		"":      "",
	}
	for in, want := range cases {
		assert.Equalf(t, want, pillLocale(in), "pillLocale(%q)", in)
	}
}

// End-to-end: a canonical es-AR locale must now yield the same Spanish pill
// label a plain "es" does — and crucially NOT the English default.
func TestPillLocaleYieldsSpanishToolLabelForArgentine(t *testing.T) {
	tool := &director_tools.SlowDaypartsTool{}
	assert.Equal(t, tool.HumanLabel("es"), tool.HumanLabel(pillLocale("es-AR")))
	assert.NotEqual(t, tool.HumanLabel("en"), tool.HumanLabel(pillLocale("es-AR")))
}
