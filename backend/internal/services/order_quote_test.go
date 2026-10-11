package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplySettlementRates_NYCTaxOn2100DoesNotSnapToWholeBasisPoints(t *testing.T) {
	// 8.875% of $2,100.00 = $186.375. Half-up once on the final cent is
	// $186.38 (18638). Snapping the rate to 8.88% first yields $186.48 (18648).
	quote := Quote{TotalCents: 210000}
	applySettlementRates(&quote, 8.875, 4.00, 0)
	assert.EqualValues(t, 210000, quote.NetSubtotalCents)
	assert.EqualValues(t, 18638, quote.TaxCents)
	assert.EqualValues(t, 8400, quote.ServiceFeeCents)
	assert.EqualValues(t, 0, quote.TipCents)
	assert.EqualValues(t, 237038, quote.FinalTotalCents)
}

func TestApplySettlementRates_PreservesRatePrecisionAcrossSubtotals(t *testing.T) {
	cases := []struct {
		name             string
		netSubtotalCents int64
		taxRate          float64
		serviceFeeRate   float64
		wantTax          int64
		wantService      int64
	}{
		{name: "one_dollar_no_divergence", netSubtotalCents: 100, taxRate: 8.875, serviceFeeRate: 4, wantTax: 9, wantService: 4},
		{name: "ten_dollars_no_divergence", netSubtotalCents: 1000, taxRate: 8.875, serviceFeeRate: 4, wantTax: 89, wantService: 40},
		{name: "two_hundred_diverges_from_888bps", netSubtotalCents: 20000, taxRate: 8.875, serviceFeeRate: 4, wantTax: 1775, wantService: 800},
		{name: "dinner_bill_346_80", netSubtotalCents: 34680, taxRate: 8.875, serviceFeeRate: 7.5, wantTax: 3078, wantService: 2601},
		{name: "large_2100_diverges_ten_cents", netSubtotalCents: 210000, taxRate: 8.875, serviceFeeRate: 4, wantTax: 18638, wantService: 8400},
		{name: "fractional_service_fee", netSubtotalCents: 210000, taxRate: 0, serviceFeeRate: 3.875, wantTax: 0, wantService: 8138},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			quote := Quote{TotalCents: tc.netSubtotalCents}
			applySettlementRates(&quote, tc.taxRate, tc.serviceFeeRate, 250)
			assert.EqualValues(t, tc.netSubtotalCents, quote.NetSubtotalCents)
			assert.EqualValues(t, tc.wantTax, quote.TaxCents)
			assert.EqualValues(t, tc.wantService, quote.ServiceFeeCents)
			assert.EqualValues(t, 250, quote.TipCents)
			assert.EqualValues(t, tc.netSubtotalCents+tc.wantTax+tc.wantService+250, quote.FinalTotalCents)
		})
	}
}

func TestApplySettlementRates_ServiceFeeUsesSameHelper(t *testing.T) {
	// 3.875% of $2,100.00 = $81.375 → 8138 cents half-up. Whole-basis-point
	// truncation would apply 3.88% and produce 8148.
	quote := Quote{TotalCents: 210000}
	applySettlementRates(&quote, 0, 3.875, 0)
	assert.EqualValues(t, 8138, quote.ServiceFeeCents)
	assert.Zero(t, quote.TaxCents)
	assert.EqualValues(t, 218138, quote.FinalTotalCents)
}

func BenchmarkApplySettlementRates(b *testing.B) {
	quote := Quote{TotalCents: 210000}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		applySettlementRates(&quote, 8.875, 4.00, 0)
	}
}
