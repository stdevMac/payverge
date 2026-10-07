package paymentcontract

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type SecretRotationObservation struct {
	PreviousConfigured error
	PreviousRetired    error
}

// Adapter translates shared cases into each provider's real verification and
// activation schemes. The shared runner owns the pass/fail assertions.
type Adapter interface {
	Name() string
	VerifySignatureCase(t *testing.T, testCase SignatureCase) error
	VerifyBehaviorCase(t *testing.T, testCase CaseSpec) error
	VerifySecretRotation(t *testing.T) SecretRotationObservation
	ObserveKillSwitch(t *testing.T, testCase KillSwitchCase) bool
}

func RunContract(t *testing.T, adapters []Adapter) {
	t.Helper()
	for _, adapter := range adapters {
		adapter := adapter
		t.Run(adapter.Name(), func(t *testing.T) {
			t.Run("signature", func(t *testing.T) {
				for _, testCase := range SignatureCases {
					testCase := testCase
					t.Run(string(testCase), func(t *testing.T) {
						err := adapter.VerifySignatureCase(t, testCase)
						switch testCase {
						case SignatureMissing, SignatureInvalid:
							require.Error(t, err, "%s must reject the %s signature case", adapter.Name(), testCase)
						case SignatureValid:
							require.NoError(t, err, "%s must accept its valid signature case", adapter.Name())
						default:
							t.Fatalf("unknown shared signature case %q", testCase)
						}
					})
				}
			})
			for _, check := range RequiredChecks {
				if check == "signature" || check == "secret_rotation" || check == "kill_switch" {
					continue
				}
				check := check
				t.Run(check, func(t *testing.T) {
					for _, testCase := range BehaviorCases {
						if testCase.Check != check {
							continue
						}
						testCase := testCase
						t.Run(testCase.Name, func(t *testing.T) {
							require.NoError(t, adapter.VerifyBehaviorCase(t, testCase), "%s must satisfy %s", adapter.Name(), testCase.ID)
						})
					}
				})
			}
			t.Run("secret_rotation", func(t *testing.T) {
				t.Run("previous_accepted_then_retired", func(t *testing.T) {
					observation := adapter.VerifySecretRotation(t)
					require.NoError(t, observation.PreviousConfigured, "%s must accept the previous secret while configured", adapter.Name())
					require.Error(t, observation.PreviousRetired, "%s must reject the previous secret once retired", adapter.Name())
				})
			})
			t.Run("kill_switch", func(t *testing.T) {
				for _, testCase := range KillSwitchCases {
					testCase := testCase
					t.Run(string(testCase), func(t *testing.T) {
						allowed := adapter.ObserveKillSwitch(t, testCase)
						switch testCase {
						case KillSwitchNewAttemptBlocked:
							require.False(t, allowed, "%s kill switch must block new attempts", adapter.Name())
						case KillSwitchReconciliationAllowed:
							require.True(t, allowed, "%s kill switch must preserve reconciliation", adapter.Name())
						default:
							t.Fatalf("unknown shared kill-switch case %q", testCase)
						}
					})
				}
			})
		})
	}
}
