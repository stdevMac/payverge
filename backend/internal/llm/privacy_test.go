package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivacyPolicy_ProductionFeaturesRequireZDR(t *testing.T) {
	sensitive := []string{
		"waiter", "waiter_whatsapp", "ops_assistant", "director",
		"guardrail", "wizard", "extraction", "image", "marketing",
	}
	for _, f := range sensitive {
		p, ok := PrivacyPolicyForFeature(f)
		require.True(t, ok, f)
		assert.True(t, p.RequireZDR, f)
		assert.NotEqual(t, PrivacyPublic, p.Class, f)
	}
}

func TestNewGenerateRequest_UnknownFeature(t *testing.T) {
	_, err := NewGenerateRequest("not_a_real_feature")
	require.ErrorIs(t, err, ErrPrivacyPolicy)
}

func TestNewGenerateRequest_SetsClass(t *testing.T) {
	req, err := NewGenerateRequest("ops_assistant")
	require.NoError(t, err)
	assert.Equal(t, PrivacyBusinessConfidential, req.PrivacyClass)
	assert.Equal(t, "ops_assistant", req.Feature)
}

func TestApplyFeaturePrivacy_FillsClass(t *testing.T) {
	req := &GenerateRequest{Feature: "director"}
	require.NoError(t, ApplyFeaturePrivacy(req))
	assert.Equal(t, PrivacyBusinessConfidential, req.PrivacyClass)
	assert.True(t, req.RequiresZDR())
}

func TestApplyFeaturePrivacy_RejectsWeakerClass(t *testing.T) {
	req := &GenerateRequest{Feature: "waiter", PrivacyClass: PrivacyPublic}
	err := ApplyFeaturePrivacy(req)
	require.Error(t, err)
}

func TestValidatePrivacy_RejectsEmpty(t *testing.T) {
	err := (GenerateRequest{}).ValidatePrivacy()
	require.Error(t, err)
}

func TestValidatePrivacy_AcceptsKnown(t *testing.T) {
	for _, c := range []PrivacyClass{PrivacyPublic, PrivacyCustomerSensitive, PrivacyBusinessConfidential} {
		require.NoError(t, (GenerateRequest{PrivacyClass: c}).ValidatePrivacy())
	}
}
