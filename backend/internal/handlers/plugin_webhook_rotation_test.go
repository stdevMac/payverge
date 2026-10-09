package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyWebhookSignatureWithSecretsAcceptsPrevious(t *testing.T) {
	plugin := &signatureSelectivePaymentPlugin{acceptedSecret: "previous"}
	require.NoError(t, verifyWebhookSignatureWithSecrets(plugin, []byte(`{"id":"evt"}`), "sig", []string{"current", "previous"}))
}

func TestVerifyWebhookSignatureWithSecretsRejectsEmptyAndInvalid(t *testing.T) {
	plugin := &signatureSelectivePaymentPlugin{acceptedSecret: "expected"}
	require.Error(t, verifyWebhookSignatureWithSecrets(plugin, []byte(`{}`), "sig", nil))
	require.Error(t, verifyWebhookSignatureWithSecrets(plugin, []byte(`{}`), "sig", []string{"wrong"}))
}

type signatureSelectivePaymentPlugin struct {
	testPaymentPlugin
	acceptedSecret string
}

func (p *signatureSelectivePaymentPlugin) VerifyWebhookSignature(_ []byte, _ string, secret string) bool {
	return secret == p.acceptedSecret
}
