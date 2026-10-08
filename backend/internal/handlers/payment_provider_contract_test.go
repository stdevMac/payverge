package handlers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/paymentcontract"
	"github.com/stdevmac/payverge/backend/internal/plugins"
)

type providerContractAdapter struct {
	name     string
	verify   func(*testing.T, paymentcontract.SignatureCase) error
	behavior func(*testing.T, paymentcontract.CaseSpec) error
	rotation func(*testing.T) paymentcontract.SecretRotationObservation
}

func (a providerContractAdapter) VerifyBehaviorCase(t *testing.T, testCase paymentcontract.CaseSpec) error {
	t.Helper()
	if a.behavior == nil {
		return fmt.Errorf("%s has no executable behavior adapter for %s", a.name, testCase.ID)
	}
	return a.behavior(t, testCase)
}

func (a providerContractAdapter) Name() string { return a.name }

func (a providerContractAdapter) VerifySignatureCase(t *testing.T, testCase paymentcontract.SignatureCase) error {
	t.Helper()
	return a.verify(t, testCase)
}

func (a providerContractAdapter) VerifySecretRotation(t *testing.T) paymentcontract.SecretRotationObservation {
	t.Helper()
	return a.rotation(t)
}

func (a providerContractAdapter) ObserveKillSwitch(t *testing.T, testCase paymentcontract.KillSwitchCase) bool {
	t.Helper()
	providerContract, ok := plugins.ProductionPaymentProviderContract(a.name)
	require.True(t, ok)
	t.Setenv(providerContract.ActivationEnv, "false")
	switch testCase {
	case paymentcontract.KillSwitchNewAttemptBlocked:
		return plugins.PaymentProviderStartsEnabled(a.name, true)
	case paymentcontract.KillSwitchReconciliationAllowed:
		return plugins.PaymentProviderReconciliationAllowed(a.name)
	default:
		t.Fatalf("unknown kill-switch case %q", testCase)
		return false
	}
}

func TestProductionPaymentProviderContract(t *testing.T) {
	adapters := []paymentcontract.Adapter{
		stripeContractAdapter(),
		payPalContractAdapter(),
		mercadoPagoContractAdapter(),
	}

	configured := plugins.ProductionPaymentProviderContracts()
	require.Len(t, configured, len(adapters))
	for i := range configured {
		require.Equal(t, configured[i].Name, adapters[i].Name(), "production provider contract and executable adapter order must match")
	}

	paymentcontract.RunContract(t, adapters)
}
