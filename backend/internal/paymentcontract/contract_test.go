package paymentcontract

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeSignatureAdapter struct {
	name         string
	missingErr   error
	invalidErr   error
	validErr     error
	observedCase []string
}

func (a *fakeSignatureAdapter) Name() string { return a.name }

func (a *fakeSignatureAdapter) VerifySignatureCase(_ *testing.T, testCase SignatureCase) error {
	a.observedCase = append(a.observedCase, string(testCase))
	switch testCase {
	case SignatureMissing:
		return a.missingErr
	case SignatureInvalid:
		return a.invalidErr
	case SignatureValid:
		return a.validErr
	default:
		return errors.New("unexpected signature case")
	}
}

func (a *fakeSignatureAdapter) VerifySecretRotation(_ *testing.T) SecretRotationObservation {
	return SecretRotationObservation{
		PreviousConfigured: nil,
		PreviousRetired:    errors.New("retired"),
	}
}

func (a *fakeSignatureAdapter) VerifyBehaviorCase(_ *testing.T, testCase CaseSpec) error {
	a.observedCase = append(a.observedCase, testCase.ID)
	return nil
}

func (a *fakeSignatureAdapter) ObserveKillSwitch(_ *testing.T, testCase KillSwitchCase) bool {
	a.observedCase = append(a.observedCase, "kill_switch/"+string(testCase))
	return testCase == KillSwitchReconciliationAllowed
}

func TestRunContractExecutesAssertionsOwnedBySharedRunner(t *testing.T) {
	adapter := &fakeSignatureAdapter{
		name:       "fakepay",
		missingErr: errors.New("missing"),
		invalidErr: errors.New("invalid"),
	}

	RunContract(t, []Adapter{adapter})

	require.Equal(t, []string{
		"missing", "invalid", "valid",
		"replay/duplicate_event", "replay/replayed_payment", "replay/delayed_completion", "replay/out_of_order",
		"binding/wrong_business", "binding/wrong_bill", "binding/wrong_provider_account",
		"amount_currency/wrong_amount", "amount_currency/wrong_currency",
		"under_overpayment/underpayment", "under_overpayment/overpayment",
		"lifecycle/refund", "lifecycle/reversal", "lifecycle/dispute", "lifecycle/cancellation", "lifecycle/expiration", "lifecycle/provider_timeout", "lifecycle/local_timeout_after_provider_success",
		"reconciliation/mismatch_repaired",
		"kill_switch/new_attempt_blocked",
		"kill_switch/reconciliation_allowed",
	}, adapter.observedCase)
}
