package aicontract

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stretchr/testify/require"
)

func TestScriptedProvider_EnforcesFeatureAndZDR(t *testing.T) {
	zdr := true
	p := NewScriptedProvider([]ProviderStep{{
		Feature:      "waiter",
		PrivacyClass: "customer_sensitive",
		RequireZDR:   &zdr,
		ResponseText: "ok",
		ServedModel:  "google/gemini-2.5-flash",
	}})
	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "waiter",
		Model:    "google/gemini-2.5-flash",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "hi"}},
	})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Text)
	require.Equal(t, "google/gemini-2.5-flash", resp.Model)
	require.Equal(t, 0, p.Remaining())
	require.True(t, p.LastRequests[0].RequiresZDR())
}

func TestScriptedProvider_FailsOnExtraCall(t *testing.T) {
	p := NewScriptedProvider(nil)
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "script exhausted")
}

func TestScriptedProvider_FeatureMismatch(t *testing.T) {
	p := NewScriptedProvider([]ProviderStep{{Feature: "director", ResponseText: "x"}})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "waiter",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "feature")
}
