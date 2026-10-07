package services

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/require"
)

func TestWizardStructuredFailureLogContainsMetadataNotPayload(t *testing.T) {
	secret := `owner@example.com Bearer sk_live_abcdefghijklmnop dish="Family Recipe"`
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })

	logWizardStructuredFailure(context.Background(), &llm.Response{
		Text: secret, Model: "test/model", ProviderRequestID: "gen-safe", FinishReason: "length",
		Usage: llm.Usage{CompletionTokens: 4096},
	}, errors.New("unexpected EOF"))

	got := buf.String()
	require.Contains(t, got, "provider_request_id=gen-safe")
	require.Contains(t, got, "finish_reason=length")
	require.Contains(t, got, "response_bytes=")
	require.Contains(t, got, "response_sha256=")
	require.NotContains(t, got, "owner@example.com")
	require.NotContains(t, got, "sk_live")
	require.NotContains(t, got, "Family Recipe")
}

func TestNoRawAILogPayloads(t *testing.T) {
	for _, name := range []string{"menu_ai_service.go", "ai.go", "../server/menu_enhancements.go"} {
		raw, err := os.ReadFile(name)
		require.NoError(t, err)
		for _, forbidden := range []string{
			"Raw response: %s",
			"Raw: %s",
			"response text: %q",
			"Successfully cropped and uploaded image for '%s'",
			"Generating image for: %s",
		} {
			require.NotContains(t, string(raw), forbidden, "%s logs raw AI output", name)
		}
	}
}
