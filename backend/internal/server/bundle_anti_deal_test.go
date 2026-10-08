package server

import "testing"

// L3-18: anti-deal guard — bundle price at or above component sum is rejected.
func TestRejectAntiDealBundlePrice(t *testing.T) {
	t.Parallel()

	if err := rejectAntiDealBundlePrice(15, 20); err != nil {
		t.Fatalf("valid deal (15 < 20) must pass: %v", err)
	}
	if err := rejectAntiDealBundlePrice(20, 20); err == nil {
		t.Fatal("price == regularTotal must be rejected as anti-deal")
	}
	if err := rejectAntiDealBundlePrice(25, 20); err == nil {
		t.Fatal("price > regularTotal must be rejected as anti-deal")
	}
	// No component total → do not hard-fail (unknown/free components).
	if err := rejectAntiDealBundlePrice(10, 0); err != nil {
		t.Fatalf("zero regularTotal must not reject: %v", err)
	}
}
