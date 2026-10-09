package services

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWizardResponseSchema_Shape(t *testing.T) {
	s := wizardResponseSchema()
	require.NotNil(t, s)
	assert.Equal(t, llm.TypeObject, s.Type)

	require.Contains(t, s.Properties, "message")
	assert.Equal(t, llm.TypeString, s.Properties["message"].Type)
	require.Equal(t, 800, *s.Properties["message"].MaxLength)

	require.Contains(t, s.Properties, "is_complete")
	assert.Equal(t, llm.TypeBoolean, s.Properties["is_complete"].Type)

	require.Contains(t, s.Properties, "extracted_config")
	ec := s.Properties["extracted_config"]
	require.NotNil(t, ec)
	assert.Equal(t, llm.TypeObject, ec.Type)

	require.Contains(t, s.Properties, "suggested_options")
	so := s.Properties["suggested_options"]
	require.NotNil(t, so)
	assert.Equal(t, llm.TypeArray, so.Type)
	require.NotNil(t, so.Items)
	assert.Equal(t, llm.TypeString, so.Items.Type)
	require.Equal(t, 4, *so.MaxItems)
	require.Equal(t, 80, *so.Items.MaxLength)

	for _, key := range []string{"business_type", "cuisine", "price_range", "categories", "items_per_category"} {
		require.Equal(t, 240, *ec.Properties[key].MaxLength, key)
	}
	require.Equal(t, 400, *ec.Properties["signature_dishes"].MaxLength)

	assert.Contains(t, s.Required, "message")
	assert.Contains(t, s.Required, "is_complete")
}

func TestWizardParse_ConformantAndLegacyOutputs(t *testing.T) {
	conformant := `{"message":"How many tacos?","is_complete":false,"extracted_config":{"cuisine":"Mexican"},"suggested_options":["3","5"]}`
	fenced := "```json\n" + conformant + "\n```"
	preamble := "Here you go:\n" + conformant

	for name, raw := range map[string]string{"conformant": conformant, "fenced": fenced, "preamble": preamble} {
		t.Run(name, func(t *testing.T) {
			var wr WizardResponse
			cleaned := cleanJSONResponse(raw)
			if err := unmarshalWizardResponse(cleaned, &wr); err != nil {
				t.Fatalf("%s failed to parse: %v (cleaned=%q)", name, err, cleaned)
			}
			if wr.Message != "How many tacos?" {
				t.Fatalf("%s: message mismatch: %q", name, wr.Message)
			}
			if wr.ExtractedConfig["cuisine"] != "Mexican" {
				t.Fatalf("%s: extracted_config not parsed: %#v", name, wr.ExtractedConfig)
			}
		})
	}
}
