package paymentcontract

type SignatureCase string
type KillSwitchCase string

type CaseSpec struct {
	Check string
	Name  string
	ID    string
}

const (
	SignatureMissing SignatureCase = "missing"
	SignatureInvalid SignatureCase = "invalid"
	SignatureValid   SignatureCase = "valid"
)

var SignatureCases = []SignatureCase{
	SignatureMissing,
	SignatureInvalid,
	SignatureValid,
}

const (
	KillSwitchNewAttemptBlocked     KillSwitchCase = "new_attempt_blocked"
	KillSwitchReconciliationAllowed KillSwitchCase = "reconciliation_allowed"
)

var KillSwitchCases = []KillSwitchCase{
	KillSwitchNewAttemptBlocked,
	KillSwitchReconciliationAllowed,
}

var ExecutedChecks = []string{
	"signature",
	"replay",
	"binding",
	"amount_currency",
	"under_overpayment",
	"lifecycle",
	"secret_rotation",
	"reconciliation",
	"kill_switch",
}

var ExecutableCases = []CaseSpec{
	{Check: "signature", Name: "missing", ID: "signature/missing"},
	{Check: "signature", Name: "invalid", ID: "signature/invalid"},
	{Check: "signature", Name: "valid", ID: "signature/valid"},
	{Check: "replay", Name: "duplicate_event", ID: "replay/duplicate_event"},
	{Check: "replay", Name: "replayed_payment", ID: "replay/replayed_payment"},
	{Check: "replay", Name: "delayed_completion", ID: "replay/delayed_completion"},
	{Check: "replay", Name: "out_of_order", ID: "replay/out_of_order"},
	{Check: "binding", Name: "wrong_business", ID: "binding/wrong_business"},
	{Check: "binding", Name: "wrong_bill", ID: "binding/wrong_bill"},
	{Check: "binding", Name: "wrong_provider_account", ID: "binding/wrong_provider_account"},
	{Check: "amount_currency", Name: "wrong_amount", ID: "amount_currency/wrong_amount"},
	{Check: "amount_currency", Name: "wrong_currency", ID: "amount_currency/wrong_currency"},
	{Check: "under_overpayment", Name: "underpayment", ID: "under_overpayment/underpayment"},
	{Check: "under_overpayment", Name: "overpayment", ID: "under_overpayment/overpayment"},
	{Check: "lifecycle", Name: "refund", ID: "lifecycle/refund"},
	{Check: "lifecycle", Name: "reversal", ID: "lifecycle/reversal"},
	{Check: "lifecycle", Name: "dispute", ID: "lifecycle/dispute"},
	{Check: "lifecycle", Name: "cancellation", ID: "lifecycle/cancellation"},
	{Check: "lifecycle", Name: "expiration", ID: "lifecycle/expiration"},
	{Check: "lifecycle", Name: "provider_timeout", ID: "lifecycle/provider_timeout"},
	{Check: "lifecycle", Name: "local_timeout_after_provider_success", ID: "lifecycle/local_timeout_after_provider_success"},
	{Check: "secret_rotation", Name: "previous_accepted_then_retired", ID: "secret_rotation/previous_accepted_then_retired"},
	{Check: "reconciliation", Name: "mismatch_repaired", ID: "reconciliation/mismatch_repaired"},
	{Check: "kill_switch", Name: "new_attempt_blocked", ID: "kill_switch/new_attempt_blocked"},
	{Check: "kill_switch", Name: "reconciliation_allowed", ID: "kill_switch/reconciliation_allowed"},
}

var BehaviorCases = append(
	append([]CaseSpec(nil), ExecutableCases[3:21]...),
	ExecutableCases[22],
)

var ProductionProviders = []string{
	"stripe",
	"paypal",
	"mercadopago",
}

var RequiredChecks = []string{
	"signature",
	"replay",
	"binding",
	"amount_currency",
	"under_overpayment",
	"lifecycle",
	"secret_rotation",
	"reconciliation",
	"kill_switch",
}
