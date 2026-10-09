package database

import "testing"

// TestApplyBillTotalsSubtractsLoyaltyDiscount guards the core money bug: a loyalty
// discount that is stored on the bill (LoyaltyDiscountCents) must be subtracted
// from TotalAmount, because every downstream consumer (guest "amount remaining",
// the plugin charge amount, the "is this bill fully paid?" check, printers,
// accounting) reads TotalAmount as the single source of truth for what is owed.
// If the recompute path (item void/qty-update, order approval) rebuilds the total
// from gross components only, it silently erases an applied redemption — the guest
// burns points and still pays full price.
func TestApplyBillTotalsSubtractsLoyaltyDiscount(t *testing.T) {
	business := &Business{TaxRate: 8, ServiceFeeRate: 5}
	items := []BillItem{
		{Subtotal: 15.00},
		{Subtotal: 5.00},
	}
	// gross: subtotal 2000¢, tax 8% = 160¢, fee 5% = 100¢ -> 2260¢
	bill := &Bill{LoyaltyDiscountCents: 500}

	applyBillTotals(bill, items, business)

	if bill.Subtotal != 2000 {
		t.Fatalf("subtotal = %d, want 2000", bill.Subtotal)
	}
	if bill.TaxAmount != 160 {
		t.Fatalf("tax = %d, want 160", bill.TaxAmount)
	}
	if bill.ServiceFeeAmount != 100 {
		t.Fatalf("service fee = %d, want 100", bill.ServiceFeeAmount)
	}
	// total must be net of the loyalty discount: 2260 - 500 = 1760.
	if bill.TotalAmount != 1760 {
		t.Fatalf("total = %d, want 1760 (gross 2260 - discount 500)", bill.TotalAmount)
	}
}

// TestApplyBillTotalsWithoutDiscountUnchanged confirms the no-discount path keeps
// the gross total exactly as before (no regression for the overwhelmingly common
// bill that has no redemption applied).
func TestApplyBillTotalsWithoutDiscountUnchanged(t *testing.T) {
	business := &Business{TaxRate: 10, ServiceFeeRate: 0}
	items := []BillItem{{Subtotal: 30.00}}
	bill := &Bill{} // LoyaltyDiscountCents == 0

	applyBillTotals(bill, items, business)

	// 3000¢ + 10% tax (300¢) = 3300¢, no discount.
	if bill.TotalAmount != 3300 {
		t.Fatalf("total = %d, want 3300", bill.TotalAmount)
	}
}

// TestNetBillTotalCentsFloorsAtZero proves a discount larger than the gross total
// can never make the bill owe a negative amount — the guest owes 0, not a credit.
func TestNetBillTotalCentsFloorsAtZero(t *testing.T) {
	cases := []struct {
		gross, discount, want int64
	}{
		{2260, 500, 1760}, // normal
		{500, 500, 0},     // exact
		{300, 500, 0},     // discount exceeds total -> floored, never negative
		{1000, 0, 1000},   // no discount
		{1000, -50, 1000}, // negative/garbage discount ignored
	}
	for _, c := range cases {
		if got := NetBillTotalCents(c.gross, c.discount); got != c.want {
			t.Fatalf("NetBillTotalCents(%d, %d) = %d, want %d", c.gross, c.discount, got, c.want)
		}
	}
}
