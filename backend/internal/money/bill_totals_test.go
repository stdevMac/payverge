package money

import "testing"

func TestBillTotalsCentsUseDeterministicMinorUnitRounding(t *testing.T) {

	subtotal, tax, serviceFee, total := int64(882), int64(78), int64(64), int64(1024)

	gotTax, gotServiceFee, gotTotal := BillTotalsCents(subtotal, 8.875, 7.25)
	if gotTax != tax || gotServiceFee != serviceFee || gotTotal != total {
		t.Fatalf("BillTotalsCents() = (%d, %d, %d), want (%d, %d, %d)", gotTax, gotServiceFee, gotTotal, tax, serviceFee, total)
	}
}

func TestLineSubtotalCents_IncludesOptionsAndSkipsBundleChildren(t *testing.T) {
	opts := []OptionSurcharge{{PriceChange: 1.50}}
	got := LineSubtotalCents("menu_item", 10.00, 2, opts, 0)
	if got != 2300 {
		t.Fatalf("menu line with options: got %d want 2300", got)
	}

	got = LineSubtotalCents("bundle_item", 12.00, 4, nil, 0)
	if got != 0 {
		t.Fatalf("bundle child must be 0 even with catalog price: got %d", got)
	}

	got = LineSubtotalCents("discount", 0, 1, nil, -5.20)
	if got != -520 {
		t.Fatalf("discount must keep provided subtotal: got %d want -520", got)
	}
}

func TestPercentageCents_DoesNotSnapNYCSalesTax(t *testing.T) {
	// 8.875% of $346.80 is $30.7785 → $30.78. Rounding the rate to 8.88% first
	// produced the $30.80 modal tax (#228).
	got := PercentageCents(34680, 8.875)
	if got != 3078 {
		t.Fatalf("PercentageCents(34680, 8.875) = %d, want 3078", got)
	}
}

func TestPercentageCents_HalfUpNYCTaxOn2100(t *testing.T) {
	// 8.875% of $2,100.00 is $186.375 → 18638 cents (half-up on the final cent).
	// 8.88% would be 18648 (#537).
	got := PercentageCents(210000, 8.875)
	if got != 18638 {
		t.Fatalf("PercentageCents(210000, 8.875) = %d, want 18638", got)
	}
}

func TestPercentageCents_RegressionSubtotals(t *testing.T) {
	cases := []struct {
		cents int64
		rate  float64
		want  int64
	}{
		{100, 8.875, 9},
		{1000, 8.875, 89},
		{20000, 8.875, 1775},
		{34680, 8.875, 3078},
		{210000, 8.875, 18638},
		{210000, 4, 8400},
		{210000, 3.875, 8138},
	}
	for _, tc := range cases {
		got := PercentageCents(tc.cents, tc.rate)
		if got != tc.want {
			t.Fatalf("PercentageCents(%d, %g) = %d, want %d", tc.cents, tc.rate, got, tc.want)
		}
	}
}

func BenchmarkPercentageCents(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = PercentageCents(210000, 8.875)
	}
}

func TestBillTotalsCents_MatchesDinnerVenueBill755Shape(t *testing.T) {
	// Settings: 7.5% service + 8.875% tax on $346.80. Tax must stay $30.78
	// (not $30.80). Service $26.01. Total $403.59.
	tax, service, total := BillTotalsCents(34680, 8.875, 7.5)
	if tax != 3078 || service != 2601 || total != 40359 {
		t.Fatalf("BillTotalsCents(34680, 8.875, 7.5) = (%d, %d, %d), want (3078, 2601, 40359)", tax, service, total)
	}
}
