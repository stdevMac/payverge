package handlers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifiedWebhookPayloadEvidenceStoresHashOnly(t *testing.T) {
	payload := []byte(`{"id":"evt_123","card":"4242424242424242","email":"guest@example.com"}`)
	evidence := verifiedWebhookPayloadEvidence(payload)

	require.True(t, strings.HasPrefix(evidence, "sha256:"))
	require.Len(t, evidence, len("sha256:")+64)
	require.NotContains(t, evidence, "evt_123")
	require.NotContains(t, evidence, "4242")
	require.NotContains(t, evidence, "guest@example.com")
	require.Equal(t, evidence, verifiedWebhookPayloadEvidence(payload))
	require.NotEqual(t, evidence, verifiedWebhookPayloadEvidence(append(payload, ' ')))
}
