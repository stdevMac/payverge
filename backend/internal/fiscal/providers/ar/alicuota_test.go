package ar

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

// TestVATAlicuotaID locks F-ALICUOTA: the WSFE IVA alícuota id is DERIVED from the
// VAT rate via a single mapping, not hardcoded to 5. If the rate ever changes
// (reduced-rate vertical, per-item rate) the emitted <Id> must follow it, or AFIP
// rejects the voucher on an Importe/alícuota mismatch.
func TestVATAlicuotaID(t *testing.T) {
	cases := []struct {
		rate float64
		want int
	}{
		{27.0, 6},
		{21.0, 5},
		{10.5, 4},
		{5.0, 8},
		{2.5, 9},
		{0.0, 3},
		{fiscal.DefaultArgentinaVATRate, 5}, // the constant the pipeline uses today
	}
	for _, tc := range cases {
		if got := vatAlicuotaID(tc.rate); got != tc.want {
			t.Errorf("vatAlicuotaID(%.1f) = %d, want %d", tc.rate, got, tc.want)
		}
	}
}

// TestMapIssueInputSetsAlicuotaFromRate proves the wiring: a 21% factura_b payload
// carries alícuota id 5 derived from the rate, not a constant.
func TestMapIssueInputSetsAlicuotaFromRate(t *testing.T) {
	pos := 1
	in := fiscal.IssueInput{
		Settings:         fiscal.Settings{TaxID: "20123456789", PointOfSale: &pos},
		ReceiptType:      "factura_b",
		TotalAmountCents: 12100,
		Currency:         "ARS",
	}
	payload, err := MapIssueInputToWSFE(in)
	if err != nil {
		t.Fatalf("MapIssueInputToWSFE: %v", err)
	}
	if payload.IVAAlicuotaID != 5 {
		t.Errorf("factura_b payload IVAAlicuotaID = %d, want 5 (21%%)", payload.IVAAlicuotaID)
	}
}
