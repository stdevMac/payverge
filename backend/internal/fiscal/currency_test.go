package fiscal

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestBillCurrencyResolvesFromBusinessDefault locks F-CURRENCY: because
// Bill.Currency is gorm:"-" (never loaded in the worker), the fiscal currency must
// resolve from the business's default currency, not blanket-default to ARS. A
// non-ARS business must surface its real currency so the AR mapper can reject it
// (fail closed) instead of silently declaring USD as pesos.
func TestBillCurrencyResolvesFromBusinessDefault(t *testing.T) {
	cases := []struct {
		name        string
		billCur     string
		businessCur string
		want        string
	}{
		{"explicit bill currency wins", "EUR", "USD", "EUR"},
		{"falls back to business default", "", "USD", "USD"},
		{"ARS business resolves to ARS", "", "ARS", "ARS"},
		{"both empty defaults to ARS", "", "", "ARS"},
		{"whitespace bill currency falls back", "  ", "USD", "USD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bill := database.Bill{Currency: tc.billCur}
			biz := database.Business{DefaultCurrency: tc.businessCur}
			got := billCurrency(bill, biz)
			if got != tc.want {
				t.Errorf("billCurrency(%q, %q) = %q, want %q", tc.billCur, tc.businessCur, got, tc.want)
			}
		})
	}
}

// TestBuildIssueInputThreadsBusinessCurrency proves the wiring: the IssueInput the
// worker hands to the provider carries the business's currency, so a USD business
// produces a USD IssueInput (which the AR mapper then rejects) rather than a
// silently-ARS one.
func TestBuildIssueInputThreadsBusinessCurrency(t *testing.T) {
	s := &Service{}
	jobCtx := &JobContext{
		Bill:     database.Bill{TotalAmount: 10000},
		Settings: database.BusinessFiscalSettings{TaxCondition: "responsable_inscripto"},
		Business: database.Business{DefaultCurrency: "USD"},
	}
	input := s.buildIssueInput(database.FiscalJob{}, jobCtx)
	if input.Currency != "USD" {
		t.Fatalf("IssueInput.Currency = %q, want USD (business default must flow through)", input.Currency)
	}
}
