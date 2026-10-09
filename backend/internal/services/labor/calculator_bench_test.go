package labor

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
)

func BenchmarkAnalyze(b *testing.B) {
	win := &fakeWindow{start: day(0), end: day(30)}
	runs := make([]database.PayrollRunCost, 0, 24)
	for i := 0; i < 24; i++ {
		runs = append(runs, database.PayrollRunCost{ID: uint(i + 1), PeriodStart: day(i), PeriodEnd: day(i + 2), GrossTotal: int64(50000 + i*1000), BonusTotal: 1000})
	}
	c := NewCalculator(&fakePayroll{runs: runs}, &fakeSales{summary: &analytics.PaymentWindowSummary{TotalRevenue: 50000}}, win)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Analyze(1, "month", nil)
	}
}
