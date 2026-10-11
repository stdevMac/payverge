package plugins_test

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/plugins/paypal"
	"github.com/stdevmac/payverge/backend/internal/plugins/stripe"

	"github.com/stretchr/testify/require"
)

func productionPaymentProviders() map[string]plugins.PaymentPlugin {
	return map[string]plugins.PaymentPlugin{
		"stripe":      stripe.NewStripePlugin(nil),
		"paypal":      paypal.NewPayPalPlugin(nil),
		"mercadopago": mercadopago.NewMercadoPagoPlugin(nil),
	}
}

// TestProductionPaymentProviders_FailClosedBehavior executes the common
// PaymentPlugin boundary against every provider in the production contract.
// The manifest booleans remain useful documentation, but cannot substitute for
// proving that malformed config, unsigned callbacks, and invalid attempts do
// not produce a successful payment result.
func TestProductionPaymentProviders_FailClosedBehavior(t *testing.T) {
	providers := productionPaymentProviders()
	require.Len(t, providers, len(plugins.ProductionPaymentProviderContracts()))

	seenEndpoints := map[string]string{}
	for _, providerContract := range plugins.ProductionPaymentProviderContracts() {
		provider, ok := providers[providerContract.Name]
		require.Truef(t, ok, "production contract %s must have an executable provider", providerContract.Name)

		t.Run(providerContract.Name, func(t *testing.T) {
			require.Equal(t, providerContract.Name, provider.GetName())
			require.Equal(t, "payment", provider.GetCategory())
			require.Error(t, provider.ValidateConfig(map[string]interface{}{"enabled": true}), "an enabled provider without credentials must fail closed")

			endpoint := provider.GetWebhookEndpoint()
			require.NotEmpty(t, endpoint)
			require.NotContains(t, endpoint, "?")
			if previous, duplicate := seenEndpoints[endpoint]; duplicate {
				t.Fatalf("webhook endpoint %s is shared by %s and %s", endpoint, previous, provider.GetName())
			}
			seenEndpoints[endpoint] = provider.GetName()

			payload := []byte(`{"event":"forged"}`)
			if requestVerifier, ok := provider.(plugins.WebhookRequestVerifier); ok {
				require.Error(t, requestVerifier.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
					Payload: payload,
					Config:  map[string]interface{}{},
				}), "missing provider signature context must fail closed")
			} else {
				require.False(t, provider.VerifyWebhookSignature(payload, "", ""), "missing signature and secret must fail closed")
			}

			response, err := provider.HandleWebhook(91, []byte(`{"`), map[string]string{})
			if err == nil {
				require.NotNil(t, response)
				require.False(t, response.Success, "malformed unsigned webhook must not report success")
			}

			paymentID, err := provider.ProcessPayment(91, 0, "USD", nil)
			require.Error(t, err, "zero-value payment attempt must not succeed")
			require.Empty(t, paymentID)
		})
	}
}
