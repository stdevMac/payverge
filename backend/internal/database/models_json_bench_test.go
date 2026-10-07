package database_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// BenchmarkBillMarshalJSON measures the per-call cost of the custom
// Bill.MarshalJSON shadow-struct path that converts the six int64-cents
// monetary fields into float64-dollar JSON shape (Money wire contract).
// No DB or container required — this is a hot-path allocation bench that
// runs in milliseconds and helps detect regressions if the shadow-struct
// pattern is replaced with reflection or otherwise rewritten.
func BenchmarkBillMarshalJSON(b *testing.B) {
	bill := database.Bill{
		ID:               1,
		BusinessID:       1,
		TableID:          1,
		BillNumber:       "PERF-BENCH-001-00042",
		Items:            "[]",
		Subtotal:         12340,
		TaxAmount:        1234,
		ServiceFeeAmount: 0,
		TotalAmount:      13574,
		PaidAmount:       13574,
		TipAmount:        2000,
		Status:           database.BillStatusPaid,
		SettlementAddr:   "0x000000000000000000000000000000000000bEEF",
		TippingAddr:      "0x000000000000000000000000000000000000bEEF",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(bill); err != nil {
			b.Fatal(err)
		}
	}
}
