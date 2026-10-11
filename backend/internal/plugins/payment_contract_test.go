package plugins

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProductionPaymentProviderContractMatrix(t *testing.T) {
	want := []string{"stripe", "paypal", "mercadopago"}
	contracts := ProductionPaymentProviderContracts()
	require.Len(t, contracts, len(want))

	for _, name := range want {
		contract, ok := ProductionPaymentProviderContract(name)
		require.Truef(t, ok, "missing production payment contract for %s", name)
		require.Equal(t, name, contract.Name)
		require.NotEmpty(t, contract.ActivationEnv)
		require.True(t, contract.SignatureVerification)
		require.True(t, contract.ReplayProtection)
		require.True(t, contract.ProviderObjectBinding)
		require.True(t, contract.AmountCurrencyBinding)
		require.True(t, contract.BusinessBillBinding)
		require.True(t, contract.UnderOverpaymentPolicy)
		require.True(t, contract.LifecycleCoverage)
		require.True(t, contract.SecretRotation)
		require.True(t, contract.Reconciliation)
		require.True(t, contract.ReconciliationAllowedWhenDisabled)
		require.True(t, contract.HashOnlyWebhookEvidence)
		require.Greater(t, contract.AttemptTimeout, contract.ReconciliationTimeout,
			"new money attempts may wait longer than recovery polling")
		require.GreaterOrEqual(t, contract.ReconciliationTimeout, 5*time.Second)
	}
}

func TestPaymentRequestIdempotencyKeyIsStableAndOpaque(t *testing.T) {
	first := PaymentRequestIdempotencyKey("paypal", "refund", uint(7), "capture-secret", int64(1200))
	second := PaymentRequestIdempotencyKey("paypal", "refund", uint(7), "capture-secret", int64(1200))
	require.Equal(t, first, second)
	require.NotContains(t, first, "capture-secret")
	require.NotEqual(t, first, PaymentRequestIdempotencyKey("paypal", "refund", uint(7), "capture-secret", int64(1300)))
}

func TestEveryProviderDeclaresItsOwnExternalSandbox(t *testing.T) {
	// Production enablement requires the operator to prove the integration in
	// the provider's own sandbox; hermetic proof in this repository is not
	// enough. Every shipped provider must therefore name a sandbox of its own.
	for _, providerContract := range ProductionPaymentProviderContracts() {
		require.Truef(t, providerContract.ExternalSandboxAuthority,
			"%s must not be production-enabled on hermetic proof alone", providerContract.Name)
		require.Truef(t, strings.HasPrefix(providerContract.ExternalSandbox, providerContract.Name+"-"),
			"%s must name its own provider sandbox, got %q", providerContract.Name, providerContract.ExternalSandbox)
	}
}

func TestProductionRefusesProviderWithoutExternalSandbox(t *testing.T) {
	original := productionPaymentProviderContracts
	t.Cleanup(func() { productionPaymentProviderContracts = original })
	productionPaymentProviderContracts = []PaymentProviderContract{
		contract("stripe", "PAYMENT_PROVIDER_STRIPE_ENABLED", ""),
	}

	t.Setenv("PAYMENT_PROVIDER_STRIPE_ENABLED", "true")
	require.False(t, PaymentProviderStartsEnabled("stripe", true),
		"the activation env must not override a missing external sandbox in production")
	require.True(t, PaymentProviderStartsEnabled("stripe", false),
		"development stays usable without external sandbox proof")
	require.True(t, PaymentProviderReconciliationAllowed("stripe"),
		"repair of already-started attempts must not depend on the sandbox gate")
}

func TestPaymentProviderProductionKillSwitch(t *testing.T) {
	contract, ok := ProductionPaymentProviderContract("stripe")
	require.True(t, ok)

	t.Setenv(contract.ActivationEnv, "")
	require.False(t, PaymentProviderStartsEnabled("stripe", true), "production must fail closed")
	require.True(t, PaymentProviderStartsEnabled("stripe", false), "development stays usable by default")

	t.Setenv(contract.ActivationEnv, "true")
	require.True(t, PaymentProviderStartsEnabled("stripe", true))

	t.Setenv(contract.ActivationEnv, "false")
	require.False(t, PaymentProviderStartsEnabled("stripe", true))
	require.True(t, PaymentProviderReconciliationAllowed("stripe"), "kill switch must not suppress repair")
}

func TestUnknownPaymentProviderFailsClosed(t *testing.T) {
	require.False(t, PaymentProviderStartsEnabled("future-provider", true))
	require.False(t, PaymentProviderReconciliationAllowed("future-provider"))
}
