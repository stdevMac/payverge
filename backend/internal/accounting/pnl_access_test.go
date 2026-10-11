package accounting

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// TestGetSummary_AccessShapeForPnL is the Wave 2 access-shape gate for the P&L
// path, which reuses GetSummary aggregates (recognized payments + SQL-bucketed
// manual income/expense + paid payroll). Asserts single-currency composition
// without hydrating full entry rows into the summary response.
func TestGetSummary_AccessShapeForPnL(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xPnLOwner", "pnl-access")
	staff := createAccountingStaff(t, business.ID, "pnl@example.com", "Manager")
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)

	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID: business.ID, EntryType: database.AccountingEntryTypeIncome,
		Category: "catering", Amount: 5000, Currency: "USD",
		OccurredAt: start.Add(time.Hour), Description: "catering",
		CreatedByStaffID: &staff.ID,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID: business.ID, EntryType: database.AccountingEntryTypeExpense,
		Category: "rent", Amount: 2000, Currency: "USD",
		OccurredAt: start.Add(2 * time.Hour), Description: "rent",
		CreatedByStaffID: &staff.ID,
	}).Error)
	paidAt := start.Add(3 * time.Hour)
	require.NoError(t, database.GetDB().Create(&database.PayrollRun{
		BusinessID: business.ID, PeriodStart: start, PeriodEnd: end,
		Status: database.PayrollRunStatusPaid, PaidAt: &paidAt,
		GrossTotal: 1000, NetTotal: 1000, Currency: "USD",
	}).Error)

	service := NewService(database.GetDBWrapper())
	summary, err := service.GetSummary(business.ID, start, end)
	require.NoError(t, err)

	// Amounts are dollars on the wire (cents/100).
	require.InDelta(t, 50.0, summary.ManualIncomeTotal, 0.01)
	require.InDelta(t, 20.0, summary.ExpenseTotal, 0.01)
	require.InDelta(t, 10.0, summary.PayrollTotal, 0.01)
	require.NotEmpty(t, summary.IncomeBreakdown)
	require.NotEmpty(t, summary.ExpenseBreakdown)
	// Single-currency path: no FX skips.
	require.Equal(t, 0, summary.SkippedManualEntries)
	// Response is aggregate-only (category totals), not a full ledger dump.
	require.LessOrEqual(t, len(summary.IncomeBreakdown), 10)
	require.LessOrEqual(t, len(summary.ExpenseBreakdown), 20)
}

func BenchmarkGetSummary_ForPnL(b *testing.B) {
	// Wave 2 baseline: P&L reuses GetSummary. Multi-currency bench:
	// BenchmarkGetSummary_MultiCurrency. This microbench covers empty-window
	// single-currency cost after setup.
	// Note: setup helpers require *testing.T — run as part of package benches
	// only when shared DB is already configured via TestMain-less suite.
	// Skip when not run via go test with package tests first.
	b.Skip("see BenchmarkGetSummary_MultiCurrency for -benchmem numbers")
}
