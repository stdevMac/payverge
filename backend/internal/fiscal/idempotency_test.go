package fiscal

import "testing"

// TestIssueIdempotencyKeyIsPerBill locks F-DUPISSUE: the issue idempotency key
// must be keyed on the BILL only, not on the payment/alt-payment dimension.
// The factura always covers the full bill total, so the auto-issue path
// (HandleBillPaid, with a paymentID) and the manual path (IssueReceipt, nil/nil)
// must collapse to ONE job — otherwise a bill gets two full-bill facturas.
func TestIssueIdempotencyKeyIsPerBill(t *testing.T) {
	const businessID, billID = uint(7), uint(42)

	got := buildIdempotencyKey(businessID, billID, ActionIssueReceipt)
	want := "business:7:bill:42:action:issue_receipt:split:none"
	if got != want {
		t.Fatalf("buildIdempotencyKey = %q, want %q", got, want)
	}

	// Same bill → same key regardless of which path enqueued it.
	if a, b := buildIdempotencyKey(businessID, billID, ActionIssueReceipt),
		buildIdempotencyKey(businessID, billID, ActionIssueReceipt); a != b {
		t.Fatalf("same-bill keys must match: %q vs %q", a, b)
	}

	// Different bills must still get distinct keys.
	if buildIdempotencyKey(businessID, billID, ActionIssueReceipt) ==
		buildIdempotencyKey(businessID, billID+1, ActionIssueReceipt) {
		t.Fatal("distinct bills must produce distinct idempotency keys")
	}
}
