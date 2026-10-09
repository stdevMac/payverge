package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"time"
)

// PaymentRequestIdempotencyKey creates a provider-safe, non-sensitive retry key
// from server-authoritative payment binding fields. The digest avoids leaking
// internal bill/business identifiers into provider logs.
func PaymentRequestIdempotencyKey(provider, operation string, binding ...interface{}) string {
	encoded, _ := json.Marshal(append([]interface{}{provider, operation}, binding...))
	digest := sha256.Sum256(encoded)
	return "pv_" + hex.EncodeToString(digest[:16])
}

// PaymentProviderContract is the release-gating manifest for every payment
// provider that may be exposed by the production plugin catalog. It makes the
// money-safety obligations reviewable in one place and gives startup a shared,
// fail-closed activation policy.
type PaymentProviderContract struct {
	Name                              string
	ActivationEnv                     string
	AttemptTimeout                    time.Duration
	ReconciliationTimeout             time.Duration
	SignatureVerification             bool
	ReplayProtection                  bool
	ProviderObjectBinding             bool
	AmountCurrencyBinding             bool
	BusinessBillBinding               bool
	UnderOverpaymentPolicy            bool
	LifecycleCoverage                 bool
	SecretRotation                    bool
	Reconciliation                    bool
	ReconciliationAllowedWhenDisabled bool
	HashOnlyWebhookEvidence           bool
	// ExternalSandbox names the provider's own test environment (Stripe test
	// mode, PayPal sandbox, Mercado Pago test users) that an operator proves
	// the integration against, with their own test-merchant credentials,
	// before setting ActivationEnv=true in production. Hermetic tests in this
	// repository are not enough on their own: a provider without an external
	// sandbox can never be production-enabled.
	ExternalSandboxAuthority bool
	ExternalSandbox          string
}

var productionPaymentProviderContracts = []PaymentProviderContract{
	contract("stripe", "PAYMENT_PROVIDER_STRIPE_ENABLED", "stripe-test-mode"),
	contract("paypal", "PAYMENT_PROVIDER_PAYPAL_ENABLED", "paypal-sandbox"),
	contract("mercadopago", "PAYMENT_PROVIDER_MERCADOPAGO_ENABLED", "mercadopago-sandbox"),
}

func contract(name, activationEnv, externalSandbox string) PaymentProviderContract {
	return PaymentProviderContract{
		Name:                              name,
		ActivationEnv:                     activationEnv,
		AttemptTimeout:                    30 * time.Second,
		ReconciliationTimeout:             10 * time.Second,
		SignatureVerification:             true,
		ReplayProtection:                  true,
		ProviderObjectBinding:             true,
		AmountCurrencyBinding:             true,
		BusinessBillBinding:               true,
		UnderOverpaymentPolicy:            true,
		LifecycleCoverage:                 true,
		SecretRotation:                    true,
		Reconciliation:                    true,
		ReconciliationAllowedWhenDisabled: true,
		HashOnlyWebhookEvidence:           true,
		ExternalSandboxAuthority:          externalSandbox != "",
		ExternalSandbox:                   externalSandbox,
	}
}

// ProductionPaymentProviderContracts returns a copy so callers cannot mutate
// the release contract at runtime.
func ProductionPaymentProviderContracts() []PaymentProviderContract {
	result := make([]PaymentProviderContract, len(productionPaymentProviderContracts))
	copy(result, productionPaymentProviderContracts)
	return result
}

func ProductionPaymentProviderContract(name string) (PaymentProviderContract, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, candidate := range productionPaymentProviderContracts {
		if candidate.Name == name {
			return candidate, true
		}
	}
	return PaymentProviderContract{}, false
}

// PaymentProviderStartsEnabled controls new payment attempts/catalog exposure.
// Production is fail-closed: a provider must have an external sandbox and be
// explicitly enabled once the operator has verified it there. Development defaults to enabled for the three
// established PSPs, while still honoring an explicit false kill switch.
func PaymentProviderStartsEnabled(name string, production bool) bool {
	providerContract, ok := ProductionPaymentProviderContract(name)
	if !ok {
		return false
	}
	if production && !providerContract.ExternalSandboxAuthority {
		return false
	}
	raw := strings.TrimSpace(os.Getenv(providerContract.ActivationEnv))
	if raw == "" {
		return !production
	}
	return strings.EqualFold(raw, "true")
}

// PaymentProviderReconciliationAllowed intentionally ignores the activation
// env: a kill switch must stop new money movement without hiding already-started
// attempts from status repair.
func PaymentProviderReconciliationAllowed(name string) bool {
	providerContract, ok := ProductionPaymentProviderContract(name)
	return ok && providerContract.Reconciliation && providerContract.ReconciliationAllowedWhenDisabled
}
