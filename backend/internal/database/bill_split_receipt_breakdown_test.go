package database

import "testing"

// assertReceiptBalances pins the receipt invariant: the breakdown and the
// claimed lines add up to exactly what the share was charged.
func assertReceiptBalances(t *testing.T, r *BillSplitShareReceipt) {
	t.Helper()
	if r.SubtotalCents < 0 || r.TaxCents < 0 || r.ServiceFeeCents < 0 {
		t.Fatalf("negative breakdown: %+v", r)
	}
	if got := r.SubtotalCents + r.TaxCents + r.ServiceFeeCents; got != r.AmountCents {
		t.Fatalf("subtotal %d + tax %d + service %d = %d, want amount %d",
			r.SubtotalCents, r.TaxCents, r.ServiceFeeCents, got, r.AmountCents)
	}
	if len(r.Items) == 0 {
		return
	}
	var lines int64
	for _, line := range r.Items {
		if line.SubtotalCents < 0 {
			t.Fatalf("negative line: %+v", line)
		}
		lines += line.SubtotalCents
	}
	if lines != r.SubtotalCents {
		t.Fatalf("lines sum to %d, want subtotal %d", lines, r.SubtotalCents)
	}
}

func TestBillSplitShareReceipt_101CentSplitBalances(t *testing.T) {
	bill := Bill{BillNumber: "B101", Subtotal: 85, TaxAmount: 9, ServiceFeeAmount: 7, TotalAmount: 101}
	items := []BillItem{
		{ID: "a", Name: "Coffee", Price: 0.43, Quantity: 1, Subtotal: 0.43},
		{ID: "b", Name: "Cookie", Price: 0.42, Quantity: 1, Subtotal: 0.42},
	}

	for _, mode := range []BillSplitMode{BillSplitModeEqual, BillSplitModeItems} {
		t.Run(string(mode), func(t *testing.T) {
			var charged int64
			for i, amount := range []int64{50, 51} {
				share := BillSplitShare{ID: uint(i + 1), Mode: mode, AmountCents: amount}
				if mode == BillSplitModeItems {
					share.ClaimedItemIDs = []string{"a", "b"}
					share.ClaimedFractions = map[string]string{"a": "1/2", "b": "1/2"}
				}
				r, err := buildBillSplitShareReceipt(bill, items, share)
				if err != nil {
					t.Fatal(err)
				}
				assertReceiptBalances(t, r)
				charged += r.AmountCents
			}
			if charged != bill.TotalAmount {
				t.Fatalf("shares charged %d, want %d", charged, bill.TotalAmount)
			}
		})
	}
}

// A share charged less than its claimed lines plus tax (custom amount, or a
// discount applied to the bill) must still balance instead of overstating.
func TestBillSplitShareReceipt_ItemsChargeBelowLinesStillBalances(t *testing.T) {
	bill := Bill{BillNumber: "B2", Subtotal: 1000, TaxAmount: 100, ServiceFeeAmount: 0, TotalAmount: 1100}
	items := []BillItem{
		{ID: "a", Name: "Steak", Price: 6, Quantity: 1, Subtotal: 6},
		{ID: "b", Name: "Wine", Price: 4, Quantity: 1, Subtotal: 4},
	}
	share := BillSplitShare{ID: 1, Mode: BillSplitModeItems, AmountCents: 7,
		ClaimedItemIDs: []string{"a", "b"}}
	r, err := buildBillSplitShareReceipt(bill, items, share)
	if err != nil {
		t.Fatal(err)
	}
	assertReceiptBalances(t, r)
}
