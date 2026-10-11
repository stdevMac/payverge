package ar

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

// BenchmarkMapIssueInputToWSFE measures the cost of mapping a factura_b
// IssueInput to a WSFEPayload including CUIT parsing, VAT split, and
// all validation steps. This is a pure CPU + allocation benchmark with
// no I/O or database involvement.
func BenchmarkMapIssueInputToWSFE(b *testing.B) {
	input := fiscal.IssueInput{
		Settings: fiscal.Settings{
			TaxID:       "20111111112",
			PointOfSale: intPtr(1),
		},
		ReceiptType:       "factura_b",
		Currency:          "ARS",
		TotalAmountCents:  12100,
		CustomerDocType:   "DNI",
		CustomerDocNumber: "12345678",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := MapIssueInputToWSFE(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}
