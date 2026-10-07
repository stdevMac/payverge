package accounting

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAccountingTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.WithdrawalHistory{},
		&database.ExchangeRate{},
		&database.ManualLedgerEntry{},
		&database.PayrollRun{},
		&database.PayrollLineItem{},
		// Payroll mark-paid/void/delete check the period lock in their write
		// transaction.
		&database.AccountingPeriodLock{},
		// L6-15 gave ListEntries a grouped attachment COUNT plus actor preloads
		// against users/staff. Without these tables every ListEntries call in
		// this package dies on "no such table: ledger_entry_attachments".
		&database.LedgerEntryAttachment{},
		&database.User{},
	))

	return gormDB
}

func createAccountingBusiness(t *testing.T, ownerAddress, name string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:      fmt.Sprintf("acct-%s", name),
		Name:            name,
		OwnerAddress:    ownerAddress,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DisplayCurrency: "USD",
		DefaultLanguage: "en",
		SourceLanguage:  "en",
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func createAccountingStaff(t *testing.T, businessID uint, email, name string) *database.Staff {
	t.Helper()

	staff := &database.Staff{
		BusinessID: businessID,
		Email:      email,
		Name:       name,
		Role:       database.StaffRoleManager,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, database.GetDB().Create(staff).Error)
	return staff
}

func createAccountingBill(t *testing.T, businessID uint, billNumber string, total float64, createdAt time.Time) *database.Bill {
	t.Helper()
	totalCents := int64(math.Round(total * 100))

	bill := &database.Bill{
		BusinessID:     businessID,
		BillNumber:     billNumber,
		Subtotal:       totalCents,
		TotalAmount:    totalCents,
		Status:         database.BillStatusOpen,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return bill
}

func createPaymentRecord(t *testing.T, billID uint, amount float64, status database.PaymentStatus, createdAt time.Time) *database.Payment {
	t.Helper()

	payment := &database.Payment{
		BillID:        billID,
		PayerAddr:     "0xPayer",
		Amount:        int64(math.Round(amount * 100)),
		Status:        status,
		PaymentMethod: "crypto",
		TxHash:        fmt.Sprintf("tx-%d-%d", billID, createdAt.UnixNano()),
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}
	require.NoError(t, database.GetDB().Create(payment).Error)
	return payment
}

func createAlternativePaymentRecord(t *testing.T, billID uint, amount float64, status database.AlternativePaymentStatus, createdAt time.Time) *database.AlternativePayment {
	t.Helper()

	payment := &database.AlternativePayment{
		BillID:          billID,
		ParticipantAddr: "cashier",
		Amount:          int64(math.Round(amount * 100)),
		PaymentMethod:   database.PaymentMethodCash,
		Status:          status,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	require.NoError(t, database.GetDB().Create(payment).Error)
	return payment
}

func createPluginAlternativePaymentRecord(t *testing.T, billID uint, amount float64, method database.AlternativePaymentMethod, status database.AlternativePaymentStatus, createdAt time.Time) *database.AlternativePayment {
	t.Helper()

	payment := &database.AlternativePayment{
		BillID:          billID,
		ParticipantAddr: fmt.Sprintf("plugin_intent_%d_%d", billID, createdAt.UnixNano()),
		Amount:          int64(math.Round(amount * 100)),
		PaymentMethod:   method,
		Status:          status,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	require.NoError(t, database.GetDB().Create(payment).Error)
	return payment
}

func createExchangeRateRecord(t *testing.T, fromCurrency, toCurrency string, rate float64, fetchedAt time.Time) *database.ExchangeRate {
	t.Helper()

	record := &database.ExchangeRate{
		FromCurrency: fromCurrency,
		ToCurrency:   toCurrency,
		Rate:         rate,
		Source:       "test",
		FetchedAt:    fetchedAt,
	}
	require.NoError(t, database.GetDB().Create(record).Error)
	return record
}

// seedMultiCurrencyPAndL creates one EUR income entry, one EUR expense entry,
// and one paid EUR payroll run in [start,end), plus a direct EUR→USD rate. The
// business reports USD, so all three sources force a historical-rate lookup.
func seedMultiCurrencyPAndL(t *testing.T, businessID uint, staffID uint, start time.Time) {
	t.Helper()
	createExchangeRateRecord(t, "EUR", "USD", 1.10, start.Add(-24*time.Hour))

	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:       businessID,
		EntryType:        database.AccountingEntryTypeIncome,
		Category:         "catering",
		Amount:           10000, // €100.00
		Currency:         "EUR",
		OccurredAt:       start.Add(2 * time.Hour),
		Description:      "EUR catering",
		CreatedByStaffID: &staffID,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:       businessID,
		EntryType:        database.AccountingEntryTypeExpense,
		Category:         "supplies",
		Amount:           5000, // €50.00
		Currency:         "EUR",
		OccurredAt:       start.Add(3 * time.Hour),
		Description:      "EUR supplies",
		CreatedByStaffID: &staffID,
	}).Error)

	paidAt := start.Add(4 * time.Hour)
	require.NoError(t, database.GetDB().Create(&database.PayrollRun{
		BusinessID:  businessID,
		PeriodStart: start,
		PeriodEnd:   start.Add(24 * time.Hour),
		Status:      database.PayrollRunStatusPaid,
		Currency:    "EUR",
		PaidAt:      &paidAt,
		GrossTotal:  8000, // €80.00
		NetTotal:    8000,
	}).Error)
}

// TestServiceGetSummary_SharesOneRateResolverAcrossSources is the access-shape
// guard for the exchange-rate hydration fix. Before the fix GetSummary built a
// separate historical-rate resolver for manual income, manual expense, and
// payroll — three full exchange_rates history queries per request for any
// multi-currency business. After the fix a single shared resolver covers all
// three, so exactly ONE exchange_rates SELECT fires.
func TestServiceGetSummary_SharesOneRateResolverAcrossSources(t *testing.T) {
	gormDB := setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerFX", "fx-shared")
	staff := createAccountingStaff(t, business.ID, "fx@example.com", "FXManager")

	start := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.May, 11, 0, 0, 0, 0, time.UTC)
	seedMultiCurrencyPAndL(t, business.ID, staff.ID, start)

	var rateQueryCount atomic.Int64
	const cbName = "payverge:test:accounting_rate_query_count"
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "exchange_rates" {
			rateQueryCount.Add(1)
		}
	}))
	t.Cleanup(func() { _ = gormDB.Callback().Query().Remove(cbName) })

	service := NewService(database.GetDBWrapper())
	summary, err := service.GetSummary(business.ID, start, end)
	require.NoError(t, err)

	// Sanity: the money actually converted (€100 income, €50 expense, €80 payroll
	// at 1.10 → $110 / $55 / $88), so the single resolver truly served all three.
	assert.InDelta(t, 110.0, summary.ManualIncomeTotal, 1e-6)
	assert.InDelta(t, 55.0, summary.ExpenseTotal, 1e-6)
	assert.InDelta(t, 88.0, summary.PayrollTotal, 1e-6)

	assert.Equal(t, int64(1), rateQueryCount.Load(),
		"GetSummary must build ONE shared rate resolver across income+expense+payroll (was %d, want 1)",
		rateQueryCount.Load())
}

// TestServiceGetSummary_SingleCurrencyIssuesNoRateQuery guards the common path:
// a USD-only business (reporting == entry currency) must not touch exchange_rates
// at all — the shared resolver early-returns before querying. This protects
// against a regression where consolidating the resolver adds a distinct-currency
// probe query to every single-currency summary.
func TestServiceGetSummary_SingleCurrencyIssuesNoRateQuery(t *testing.T) {
	gormDB := setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerUSD", "usd-only")
	staff := createAccountingStaff(t, business.ID, "usd@example.com", "USDManager")

	start := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.May, 11, 0, 0, 0, 0, time.UTC)
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:       business.ID,
		EntryType:        database.AccountingEntryTypeIncome,
		Category:         "catering",
		Amount:           10000,
		Currency:         "USD",
		OccurredAt:       start.Add(2 * time.Hour),
		CreatedByStaffID: &staff.ID,
	}).Error)

	var rateQueryCount atomic.Int64
	const cbName = "payverge:test:accounting_rate_query_count_usd"
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "exchange_rates" {
			rateQueryCount.Add(1)
		}
	}))
	t.Cleanup(func() { _ = gormDB.Callback().Query().Remove(cbName) })

	service := NewService(database.GetDBWrapper())
	_, err := service.GetSummary(business.ID, start, end)
	require.NoError(t, err)

	assert.Equal(t, int64(0), rateQueryCount.Load(),
		"single-currency GetSummary must not query exchange_rates (was %d, want 0)",
		rateQueryCount.Load())
}

// BenchmarkGetSummary_MultiCurrency measures a P&L summary for a business with
// foreign-currency income, expense, and payroll — the path that formerly issued
// three separate exchange_rates history queries and now issues one. Run with
// -benchmem -count=3.
func BenchmarkGetSummary_MultiCurrency(b *testing.B) {
	dsn := fmt.Sprintf("file:bench_get_summary_%d?mode=memory", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	if err := gormDB.AutoMigrate(
		&database.Business{}, &database.Staff{}, &database.Bill{}, &database.Payment{},
		&database.AlternativePayment{}, &database.WithdrawalHistory{}, &database.ExchangeRate{},
		&database.ManualLedgerEntry{}, &database.PayrollRun{}, &database.PayrollLineItem{}, &database.AccountingPeriodLock{},
	); err != nil {
		b.Fatal(err)
	}

	biz := &database.Business{
		BusinessId: "bench-summary", Name: "Bench", OwnerAddress: "0xBenchS",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD", DisplayCurrency: "USD", DefaultLanguage: "en",
		SourceLanguage: "en",
	}
	if err := gormDB.Create(biz).Error; err != nil {
		b.Fatal(err)
	}
	staff := &database.Staff{BusinessID: biz.ID, Email: "b@example.com", Name: "B", Role: database.StaffRoleManager, IsActive: true, InvitedBy: "o"}
	if err := gormDB.Create(staff).Error; err != nil {
		b.Fatal(err)
	}

	start := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.May, 11, 0, 0, 0, 0, time.UTC)
	gormDB.Create(&database.ExchangeRate{FromCurrency: "EUR", ToCurrency: "USD", Rate: 1.10, Source: "test", FetchedAt: start.Add(-24 * time.Hour)})
	// Spread several rate revisions so each resolver load carries a real timeline.
	for i := 0; i < 20; i++ {
		gormDB.Create(&database.ExchangeRate{FromCurrency: "EUR", ToCurrency: "USD", Rate: 1.10 + float64(i)*0.001, Source: "test", FetchedAt: start.Add(-time.Duration(i) * time.Hour)})
	}
	for i := 0; i < 30; i++ {
		occ := start.Add(time.Duration(i) * time.Minute)
		gormDB.Create(&database.ManualLedgerEntry{BusinessID: biz.ID, EntryType: database.AccountingEntryTypeIncome, Category: "catering", Amount: 10000, Currency: "EUR", OccurredAt: occ, CreatedByStaffID: &staff.ID})
		gormDB.Create(&database.ManualLedgerEntry{BusinessID: biz.ID, EntryType: database.AccountingEntryTypeExpense, Category: "supplies", Amount: 5000, Currency: "EUR", OccurredAt: occ, CreatedByStaffID: &staff.ID})
	}
	paidAt := start.Add(4 * time.Hour)
	gormDB.Create(&database.PayrollRun{BusinessID: biz.ID, PeriodStart: start, PeriodEnd: end, Status: database.PayrollRunStatusPaid, Currency: "EUR", PaidAt: &paidAt, GrossTotal: 8000, NetTotal: 8000})

	service := NewService(database.GetDBWrapper())
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := service.GetSummary(biz.ID, start, end); err != nil {
			b.Fatal(err)
		}
	}
}

func TestServiceGetSummary_ComputesPAndLAndReconciliation(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	otherBusiness := createAccountingBusiness(t, "0xOwnerB", "beta")
	staff := createAccountingStaff(t, business.ID, "manager@example.com", "Manager")

	start := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.January, 16, 0, 0, 0, 0, time.UTC)

	bill := createAccountingBill(t, business.ID, "B-100", 100, start.Add(2*time.Hour))
	createPaymentRecord(t, bill.ID, 60, database.PaymentStatusConfirmed, start.Add(3*time.Hour))
	createPaymentRecord(t, bill.ID, 20, database.PaymentStatusPending, start.Add(4*time.Hour))
	createAlternativePaymentRecord(t, bill.ID, 25, database.AltPaymentStatusConfirmed, start.Add(5*time.Hour))

	otherBill := createAccountingBill(t, otherBusiness.ID, "B-200", 999, start.Add(2*time.Hour))
	createPaymentRecord(t, otherBill.ID, 999, database.PaymentStatusConfirmed, start.Add(3*time.Hour))

	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:       business.ID,
		EntryType:        database.AccountingEntryTypeIncome,
		Category:         "catering",
		Amount:           3000,
		Currency:         "USD",
		OccurredAt:       start.Add(24 * time.Hour),
		Description:      "Manual catering sale",
		CreatedByStaffID: &staff.ID,
	}).Error)

	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:       business.ID,
		EntryType:        database.AccountingEntryTypeExpense,
		Category:         "inventory",
		Amount:           4000,
		Currency:         "USD",
		OccurredAt:       start.Add(24 * time.Hour),
		Description:      "Inventory restock",
		CreatedByStaffID: &staff.ID,
	}).Error)

	require.NoError(t, database.GetDB().Create(&database.WithdrawalHistory{
		BusinessID:        business.ID,
		TransactionHash:   "withdraw-1",
		TotalAmount:       70,
		WithdrawalAddress: "0xWithdraw",
		BlockchainNetwork: "base",
		Status:            "confirmed",
		CreatedAt:         start.Add(48 * time.Hour),
		UpdatedAt:         start.Add(48 * time.Hour),
	}).Error)

	draftRun := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: start,
		PeriodEnd:   start.Add(24 * time.Hour),
		Status:      database.PayrollRunStatusDraft,
		GrossTotal:  5000,
		NetTotal:    5000,
		CreatedAt:   start.Add(48 * time.Hour),
		UpdatedAt:   start.Add(48 * time.Hour),
	}
	require.NoError(t, database.GetDB().Create(draftRun).Error)

	paidAt := start.Add(72 * time.Hour)
	paidRun := &database.PayrollRun{
		BusinessID:       business.ID,
		PeriodStart:      start,
		PeriodEnd:        start.Add(48 * time.Hour),
		Status:           database.PayrollRunStatusPaid,
		PaidAt:           &paidAt,
		GrossTotal:       2000,
		NetTotal:         2000,
		CreatedByStaffID: &staff.ID,
		CreatedAt:        paidAt,
		UpdatedAt:        paidAt,
	}
	require.NoError(t, database.GetDB().Create(paidRun).Error)

	service := NewService(database.GetDBWrapper())
	summary, err := service.GetSummary(business.ID, start, end)
	require.NoError(t, err)

	assert.Equal(t, "USD", summary.Currency)
	assert.InDelta(t, 85, summary.AutoIncomeTotal, 0.001)
	assert.InDelta(t, 30, summary.ManualIncomeTotal, 0.001)
	assert.InDelta(t, 40, summary.ExpenseTotal, 0.001)
	assert.InDelta(t, 20, summary.PayrollTotal, 0.001)
	// Net lives on P&L only (includes COGS) — Summary keeps component totals.
	assert.InDelta(t, 100, summary.BilledTotal, 0.001)
	assert.InDelta(t, 85, summary.CollectedTotal, 0.001)
	// Open remaining (total − paid_amount) is in the collection gap so
	// BRECHA DE COBRO matches billed − collected (#651). PaidAmount is still
	// 0 on this fixture (payment rows aren't stamped onto the bill), so the
	// whole $100 check is outstanding.
	assert.InDelta(t, 100, summary.CollectionGap, 0.001)
	assert.Len(t, summary.IncomeBreakdown, 1)
	assert.Equal(t, "catering", summary.IncomeBreakdown[0].Category)
	assert.InDelta(t, 30, summary.IncomeBreakdown[0].Total, 0.001)
	assert.Len(t, summary.ExpenseBreakdown, 1)
	assert.Equal(t, "inventory", summary.ExpenseBreakdown[0].Category)
	assert.InDelta(t, 40, summary.ExpenseBreakdown[0].Total, 0.001)
	assert.Equal(t, 1, summary.PayrollSummary.PaidRuns)
	assert.InDelta(t, 20, summary.PayrollSummary.TotalNet, 0.001)
}

// TestServiceGetSummary_DoesNotDoublePluginSettlements is the access-shape gate
// for the P&L double-count bug. Every plugin settlement (Stripe/PayPal/etc.)
// writes BOTH a confirmed Payment row AND a confirmed AlternativePayment tracker
// row (PaymentMethod = plugin name). The auto-income aggregate must count that
// money exactly once: the alternative-payments sum must be scoped to the
// bill-managed manual methods (cash/card/venmo/other) so plugin tracker rows are
// excluded. Manual cash payments (no companion Payment row) must still count.
func TestServiceGetSummary_DoesNotDoublePluginSettlements(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")

	start := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.May, 10, 0, 0, 0, 0, time.UTC)

	// Plugin (Stripe) settlement: one confirmed Payment + one confirmed
	// AlternativePayment tracker (method = "stripe") on the SAME bill. This is
	// $100 of real money and must contribute $100 — not $200 — to AutoIncomeTotal.
	pluginBill := createAccountingBill(t, business.ID, "B-PLUGIN", 100, start.Add(1*time.Hour))
	createPaymentRecord(t, pluginBill.ID, 100, database.PaymentStatusConfirmed, start.Add(2*time.Hour))
	createPluginAlternativePaymentRecord(t, pluginBill.ID, 100, database.AlternativePaymentMethod("stripe"), database.AltPaymentStatusConfirmed, start.Add(2*time.Hour))

	// Manual cash payment: a confirmed AlternativePayment with NO companion
	// Payment row (the real-world shape for cashier-entered cash). Must count once.
	cashBill := createAccountingBill(t, business.ID, "B-CASH", 40, start.Add(3*time.Hour))
	createAlternativePaymentRecord(t, cashBill.ID, 40, database.AltPaymentStatusConfirmed, start.Add(4*time.Hour))

	service := NewService(database.GetDBWrapper())
	summary, err := service.GetSummary(business.ID, start, end)
	require.NoError(t, err)

	// $100 (plugin, counted once) + $40 (manual cash) = $140.
	// Before the fix the stripe tracker double-counted the plugin money → $240.
	assert.InDelta(t, 140, summary.AutoIncomeTotal, 0.001,
		"plugin settlement must count once and manual cash once (got %.2f, want 140)",
		summary.AutoIncomeTotal)
	assert.InDelta(t, 140, summary.CollectedTotal, 0.001)
	// Net is ComposeProfitLoss/GetProfitLoss only — components stay on Summary.
}

// TestServiceGetSummary_NetsRefundedPaymentInsteadOfErasing is the access-shape
// gate for the P&L refund-recognition bug. Auto income must use the recognized-
// payment ledger (the same basis as the analytics dashboard): a payment stays
// recognized in the period it was earned, and a later refund is netted by a
// dated negative event at refund time — it is NOT silently erased from the
// earning period. This keeps closed-period P&L immutable and consistent with
// analytics revenue for the same window.
func TestServiceGetSummary_NetsRefundedPaymentInsteadOfErasing(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")

	earnStart := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	earnEnd := earnStart.Add(24 * time.Hour)
	refundDay := time.Date(2026, time.June, 5, 0, 0, 0, 0, time.UTC)
	refundEnd := refundDay.Add(24 * time.Hour)

	// $100 confirmed crypto payment earned on June 1 (confirmed_at set).
	bill := createAccountingBill(t, business.ID, "B-REFUND", 100, earnStart.Add(1*time.Hour))
	confirmedAt := earnStart.Add(2 * time.Hour)
	payment := createPaymentRecord(t, bill.ID, 100, database.PaymentStatusConfirmed, earnStart.Add(1*time.Hour))
	require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("id = ?", payment.ID).
		Update("confirmed_at", confirmedAt).Error)

	service := NewService(database.GetDBWrapper())

	// Before the refund: June 1 shows the $100 income.
	preSummary, err := service.GetSummary(business.ID, earnStart, earnEnd)
	require.NoError(t, err)
	assert.InDelta(t, 100, preSummary.AutoIncomeTotal, 0.001)

	// Operator refunds on June 5: status->refunded, reversed_at set (mirrors
	// database.RefundBillPayment).
	refundAt := refundDay.Add(3 * time.Hour)
	require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("id = ?", payment.ID).
		Updates(map[string]interface{}{
			"status":      database.PaymentStatusRefunded,
			"reversed_at": &refundAt,
			"updated_at":  refundAt,
		}).Error)

	// June 1 P&L must still show $100 — the earning period is immutable.
	afterEarn, err := service.GetSummary(business.ID, earnStart, earnEnd)
	require.NoError(t, err)
	assert.InDelta(t, 100, afterEarn.AutoIncomeTotal, 0.001,
		"refund must not retroactively erase income from the earning period (got %.2f)", afterEarn.AutoIncomeTotal)

	// June 5 P&L must carry the -$100 dated reversal.
	refundSummary, err := service.GetSummary(business.ID, refundDay, refundEnd)
	require.NoError(t, err)
	assert.InDelta(t, -100, refundSummary.AutoIncomeTotal, 0.001,
		"refund must be netted as a dated negative event at refund time (got %.2f)", refundSummary.AutoIncomeTotal)
}

// TestServiceGetSummary_ExcludesVoidedBillsFromBilledTotal is the access-shape
// gate for the collection-gap bug: a voided bill carries no collection
// expectation, so it must not inflate billed_total or collection_gap.
func TestServiceGetSummary_ExcludesVoidedBillsFromBilledTotal(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	start := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	// A real, paid bill.
	paidBill := createAccountingBill(t, business.ID, "B-OK", 100, start.Add(1*time.Hour))
	createPaymentRecord(t, paidBill.ID, 100, database.PaymentStatusConfirmed, start.Add(2*time.Hour))
	require.NoError(t, database.GetDB().Model(paidBill).Updates(map[string]interface{}{
		"paid_amount": 10000,
		"status":      database.BillStatusPaid,
	}).Error)

	// A mistaken bill that was voided (no payment). VoidBill preserves
	// total_amount but flips status to voided.
	voidedBill := createAccountingBill(t, business.ID, "B-VOID", 200, start.Add(3*time.Hour))
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", voidedBill.ID).
		Update("status", database.BillStatusVoided).Error)

	service := NewService(database.GetDBWrapper())
	summary, err := service.GetSummary(business.ID, start, end)
	require.NoError(t, err)

	assert.InDelta(t, 100, summary.BilledTotal, 0.001,
		"voided bill must not count toward billed_total (got %.2f)", summary.BilledTotal)
	assert.InDelta(t, 0, summary.CollectionGap, 0.001,
		"voided bill must not inflate collection_gap (got %.2f)", summary.CollectionGap)
}

func TestServiceGetSummary_AbandonedBillCountsTowardCollectionGap(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	start := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	abandoned := createAccountingBill(t, business.ID, "B-WALKOUT", 248.34, start.Add(1*time.Hour))
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", abandoned.ID).
		Update("status", database.BillStatusAbandoned).Error)

	service := NewService(database.GetDBWrapper())
	summary, err := service.GetSummary(business.ID, start, end)
	require.NoError(t, err)

	assert.InDelta(t, 248.34, summary.BilledTotal, 0.01,
		"abandoned walk-out still billed (got %.2f)", summary.BilledTotal)
	assert.InDelta(t, 0, summary.CollectedTotal, 0.001)
	assert.InDelta(t, 248.34, summary.CollectionGap, 0.01,
		"abandoned remaining due must be in collection_gap so Billed-Collected=Gap (got %.2f)", summary.CollectionGap)
}

func TestServiceVoidManualEntry_ExcludesEntryFromSummaryButPreservesRow(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	staff := createAccountingStaff(t, business.ID, "manager@example.com", "Manager")
	start := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)

	entry := &database.ManualLedgerEntry{
		BusinessID:       business.ID,
		EntryType:        database.AccountingEntryTypeExpense,
		Category:         "software",
		Amount:           9900,
		Currency:         "USD",
		OccurredAt:       start.Add(2 * time.Hour),
		Description:      "SaaS tools",
		CreatedByStaffID: &staff.ID,
	}
	require.NoError(t, database.GetDB().Create(entry).Error)

	service := NewService(database.GetDBWrapper())
	require.NoError(t, service.VoidManualEntry(business.ID, entry.ID, nil, &staff.ID))

	summary, err := service.GetSummary(business.ID, start, end)
	require.NoError(t, err)
	assert.InDelta(t, 0, summary.ExpenseTotal, 0.001)

	var persisted database.ManualLedgerEntry
	require.NoError(t, database.GetDB().First(&persisted, entry.ID).Error)
	require.NotNil(t, persisted.VoidedAt)
	require.NotNil(t, persisted.VoidedByStaffID)
	assert.Equal(t, staff.ID, *persisted.VoidedByStaffID)

	// Re-voiding is rejected (CAS on voided_at IS NULL) and must NOT overwrite
	// the original void attribution with a different actor.
	otherStaff := createAccountingStaff(t, business.ID, "other@example.com", "Other")
	err = service.VoidManualEntry(business.ID, entry.ID, nil, &otherStaff.ID)
	require.Error(t, err, "second void of the same entry must fail")

	var afterSecond database.ManualLedgerEntry
	require.NoError(t, database.GetDB().First(&afterSecond, entry.ID).Error)
	require.NotNil(t, afterSecond.VoidedByStaffID)
	assert.Equal(t, staff.ID, *afterSecond.VoidedByStaffID,
		"original void attribution must survive a losing concurrent void")
}

func TestServiceCreatePayrollRun_CalculatesTotalsAndCopiesStaffIdentity(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	staff := createAccountingStaff(t, business.ID, "manager@example.com", "Manager")

	service := NewService(database.GetDBWrapper())
	run, err := service.CreatePayrollRun(CreatePayrollRunInput{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		LineItems: []CreatePayrollLineItemInput{
			{
				PayeeType:       database.PayrollPayeeTypeStaff,
				StaffID:         &staff.ID,
				GrossAmount:     100,
				BonusAmount:     10,
				DeductionAmount: 5,
				Notes:           "Biweekly payroll",
			},
			{
				PayeeType:       database.PayrollPayeeTypeContractor,
				PayeeName:       "External Contractor",
				GrossAmount:     50,
				BonusAmount:     0,
				DeductionAmount: 0,
				Notes:           "Freelance design",
			},
		},
		CreatedByStaffID: &staff.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, database.PayrollRunStatusDraft, run.Status)
	assert.InDelta(t, 15000, run.GrossTotal, 0.001)
	assert.InDelta(t, 1000, run.BonusTotal, 0.001)
	assert.InDelta(t, 500, run.DeductionTotal, 0.001)
	assert.InDelta(t, 15500, run.NetTotal, 0.001)
	require.Len(t, run.LineItems, 2)
	assert.Equal(t, staff.Name, run.LineItems[0].PayeeName)
	assert.Equal(t, database.PayrollPayeeTypeContractor, run.LineItems[1].PayeeType)
	assert.Equal(t, "External Contractor", run.LineItems[1].PayeeName)
	assert.InDelta(t, 10500, run.LineItems[0].NetAmount, 0.001)
	assert.InDelta(t, 5000, run.LineItems[1].NetAmount, 0.001)
}

// TestServiceCreatePayrollRun_StampsBusinessCurrency locks PAY-5 part 1: a run
// records the business reporting currency at creation so P&L can convert later.
func TestServiceCreatePayrollRun_StampsBusinessCurrency(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerCur", "currency")
	business.DefaultCurrency = "ARS"
	require.NoError(t, database.GetDB().Save(business).Error)
	staff := createAccountingStaff(t, business.ID, "mgr-cur@example.com", "Mgr")

	service := NewService(database.GetDBWrapper())
	run, err := service.CreatePayrollRun(CreatePayrollRunInput{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		LineItems: []CreatePayrollLineItemInput{
			{PayeeType: database.PayrollPayeeTypeStaff, StaffID: &staff.ID, GrossAmount: 100},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "ARS", run.Currency)

	var persisted database.PayrollRun
	require.NoError(t, database.GetDB().First(&persisted, run.ID).Error)
	assert.Equal(t, "ARS", persisted.Currency)
}

// TestPayrollRunStatusVoidConstant locks the new terminal void status used by PAY-2.
func TestPayrollRunStatusVoidConstant(t *testing.T) {
	assert.Equal(t, database.PayrollRunStatus("void"), database.PayrollRunStatusVoid)
}

func TestServiceCreatePayrollRun_RejectsInvalidStaffPayee(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	service := NewService(database.GetDBWrapper())

	_, err := service.CreatePayrollRun(CreatePayrollRunInput{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		LineItems: []CreatePayrollLineItemInput{
			{
				PayeeType:       database.PayrollPayeeTypeStaff,
				GrossAmount:     100,
				BonusAmount:     0,
				DeductionAmount: 0,
			},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "staff_id")
}

func TestServiceCreatePayrollRun_AllowsSameDayRange(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	staff := createAccountingStaff(t, business.ID, "manager@example.com", "Manager")
	service := NewService(database.GetDBWrapper())

	run, err := service.CreatePayrollRun(CreatePayrollRunInput{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		LineItems: []CreatePayrollLineItemInput{
			{
				PayeeType:       database.PayrollPayeeTypeStaff,
				StaffID:         &staff.ID,
				GrossAmount:     120,
				BonusAmount:     0,
				DeductionAmount: 0,
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC), run.PeriodStart)
	assert.Equal(t, time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC), run.PeriodEnd)
}

func TestServiceCreatePayrollRun_RejectsZeroGrossAmount(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	staff := createAccountingStaff(t, business.ID, "manager@example.com", "Manager")
	service := NewService(database.GetDBWrapper())

	_, err := service.CreatePayrollRun(CreatePayrollRunInput{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		LineItems: []CreatePayrollLineItemInput{
			{
				PayeeType:       database.PayrollPayeeTypeStaff,
				StaffID:         &staff.ID,
				GrossAmount:     0,
				BonusAmount:     0,
				DeductionAmount: 0,
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gross_amount")
}

func TestServiceCreateManualEntry_UsesBusinessDefaultCurrencyWhenOmitted(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	business.DefaultCurrency = "EUR"
	require.NoError(t, database.GetDB().Save(business).Error)

	service := NewService(database.GetDBWrapper())
	entry, err := service.CreateManualEntry(CreateManualLedgerEntryInput{
		BusinessID:      business.ID,
		EntryType:       database.AccountingEntryTypeExpense,
		Category:        "software",
		Amount:          12.5,
		OccurredAt:      time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC),
		Description:     "Software license",
		CreatedByUserID: nil,
	})
	require.NoError(t, err)
	assert.Equal(t, "EUR", entry.Currency)
}

func TestServiceCreateManualEntry_FallsBackWhenRequestedCurrencyIsMalformed(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	business.DefaultCurrency = "EUR"
	require.NoError(t, database.GetDB().Save(business).Error)

	service := NewService(database.GetDBWrapper())
	entry, err := service.CreateManualEntry(CreateManualLedgerEntryInput{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeExpense,
		Category:    "software",
		Amount:      12.5,
		Currency:    "usd dollars",
		OccurredAt:  time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC),
		Description: "Software license",
	})
	require.NoError(t, err)
	assert.Equal(t, "EUR", entry.Currency)
}

func TestServiceCreateManualEntry_PreservesKnownNonStandardCurrency(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	business.DefaultCurrency = "EUR"
	require.NoError(t, database.GetDB().Save(business).Error)
	createExchangeRateRecord(t, "USDC", "EUR", 0.92, time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC))

	service := NewService(database.GetDBWrapper())
	entry, err := service.CreateManualEntry(CreateManualLedgerEntryInput{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeIncome,
		Category:    "service",
		Amount:      50,
		Currency:    "USDC",
		OccurredAt:  time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC),
		Description: "Crypto payment adjustment",
	})
	require.NoError(t, err)
	assert.Equal(t, "USDC", entry.Currency)
}

func TestServiceCreateManualEntry_RejectsCurrencyWithoutHistoricalConversionRate(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	business.DefaultCurrency = "ARS"
	require.NoError(t, database.GetDB().Save(business).Error)

	service := NewService(database.GetDBWrapper())
	_, err := service.CreateManualEntry(CreateManualLedgerEntryInput{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeIncome,
		Category:    "service",
		Amount:      25,
		Currency:    "BTC",
		OccurredAt:  time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC),
		Description: "Back-office crypto adjustment",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be converted")
	assert.Contains(t, err.Error(), "BTC")
	assert.Contains(t, err.Error(), "ARS")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.ManualLedgerEntry{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestServiceMarkPayrollRunPaid_PersistsAuditActor(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	staff := createAccountingStaff(t, business.ID, "manager@example.com", "Manager")
	userID := uint(33)

	run := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusDraft,
		GrossTotal:  10000,
		NetTotal:    10000,
	}
	require.NoError(t, database.GetDB().Create(run).Error)

	service := NewService(database.GetDBWrapper())
	paidAt := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.UTC)
	paidRun, err := service.MarkPayrollRunPaid(business.ID, run.ID, paidAt, &userID, &staff.ID)
	require.NoError(t, err)

	require.NotNil(t, paidRun.PaidAt)
	assert.Equal(t, database.PayrollRunStatusPaid, paidRun.Status)
	require.NotNil(t, paidRun.PaidByUserID)
	require.NotNil(t, paidRun.PaidByStaffID)
	assert.Equal(t, userID, *paidRun.PaidByUserID)
	assert.Equal(t, staff.ID, *paidRun.PaidByStaffID)
}

// TestServiceMarkPayrollRunPaid_RejectsVoidedRun locks PAY-3: mark-paid is a
// compare-and-set on status='draft', so a voided run can never be flipped back to
// paid (which would silently re-add its net to P&L).
func TestServiceMarkPayrollRunPaid_RejectsVoidedRun(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerVoid", "voidcase")
	run := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusVoid,
		GrossTotal:  10000,
		NetTotal:    10000,
	}
	require.NoError(t, database.GetDB().Create(run).Error)

	service := NewService(database.GetDBWrapper())
	_, err := service.MarkPayrollRunPaid(business.ID, run.ID, time.Now().UTC(), nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "void")

	var persisted database.PayrollRun
	require.NoError(t, database.GetDB().First(&persisted, run.ID).Error)
	assert.Equal(t, database.PayrollRunStatusVoid, persisted.Status, "voided run must stay void")
}

// TestServiceMarkPayrollRunPaid_IdempotentDoesNotOverwriteActor locks PAY-3's
// idempotency: re-marking an already-paid run returns it unchanged and keeps the
// original paid actor/time (a second caller cannot rewrite the audit trail).
func TestServiceMarkPayrollRunPaid_IdempotentDoesNotOverwriteActor(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerIdem", "idem")
	firstUser := uint(7)
	run := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusDraft,
		GrossTotal:  10000,
		NetTotal:    10000,
	}
	require.NoError(t, database.GetDB().Create(run).Error)

	service := NewService(database.GetDBWrapper())
	firstPaidAt := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.UTC)
	_, err := service.MarkPayrollRunPaid(business.ID, run.ID, firstPaidAt, &firstUser, nil)
	require.NoError(t, err)

	secondUser := uint(99)
	secondPaidAt := time.Date(2026, time.March, 25, 8, 0, 0, 0, time.UTC)
	again, err := service.MarkPayrollRunPaid(business.ID, run.ID, secondPaidAt, &secondUser, nil)
	require.NoError(t, err)

	require.NotNil(t, again.PaidByUserID)
	assert.Equal(t, firstUser, *again.PaidByUserID, "original payer must be preserved")
	require.NotNil(t, again.PaidAt)
	assert.Equal(t, firstPaidAt.UTC(), again.PaidAt.UTC(), "original paid time must be preserved")
}

// --- PAY-5: currency-aware payroll in P&L ---

// TestServiceGetSummary_ConvertsPayrollByStampedCurrency locks PAY-5: a run stamped
// in a non-reporting currency is converted to the reporting currency in P&L (using
// the rate at paid_at), instead of being summed as raw cents.
func TestServiceGetSummary_ConvertsPayrollByStampedCurrency(t *testing.T) {
	setupAccountingTestDB(t)

	// Business reported in ARS when the run was created (stamps ARS)...
	business := createAccountingBusiness(t, "0xOwnerPayCur", "paycur")
	business.DefaultCurrency = "ARS"
	require.NoError(t, database.GetDB().Save(business).Error)
	service := NewService(database.GetDBWrapper())

	run := createDraftRunWithLine(t, service, business.ID, nil) // net 100 ARS
	assert.Equal(t, "ARS", run.Currency)
	paidAt := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC)
	_, err := service.MarkPayrollRunPaid(business.ID, run.ID, paidAt, nil, nil)
	require.NoError(t, err)

	// 1 USD = 1000 ARS (fetched before paid_at).
	createExchangeRateRecord(t, "USD", "ARS", 1000, time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC))

	// ...then the business switches its reporting currency to USD.
	business.DefaultCurrency = "USD"
	require.NoError(t, database.GetDB().Save(business).Error)

	summary, err := service.GetSummary(business.ID,
		time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, "USD", summary.Currency)
	// 100 ARS / 1000 = $0.10 USD (NOT 100 if currency were ignored).
	assert.InDelta(t, 0.10, summary.PayrollTotal, 0.0001)
	assert.InDelta(t, 0.10, summary.PayrollSummary.TotalNet, 0.0001)
	assert.Equal(t, 1, summary.PayrollSummary.PaidRuns)
}

// --- PAY-4: duplicate-period guard ---

func TestServiceCreatePayrollRun_RejectsDuplicatePeriod(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerDup", "dup")
	service := NewService(database.GetDBWrapper())

	input := CreatePayrollRunInput{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		LineItems: []CreatePayrollLineItemInput{
			{PayeeType: database.PayrollPayeeTypeContractor, PayeeName: "Freelancer", GrossAmount: 100},
		},
	}
	_, err := service.CreatePayrollRun(input)
	require.NoError(t, err)

	_, err = service.CreatePayrollRun(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.PayrollRun{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "duplicate must not be persisted")
}

// PAY-OVERLAP: a partially overlapping non-void run is refused like an exact
// duplicate; an adjacent run (next day) and a run overlapping only a voided
// run are allowed.
func TestServiceCreatePayrollRun_RejectsOverlappingPeriod(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerOverlap", "overlap")
	service := NewService(database.GetDBWrapper())

	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	mk := func(start, end time.Time) CreatePayrollRunInput {
		return CreatePayrollRunInput{
			BusinessID:  business.ID,
			PeriodStart: start,
			PeriodEnd:   end,
			LineItems: []CreatePayrollLineItemInput{
				{PayeeType: database.PayrollPayeeTypeContractor, PayeeName: "Freelancer", GrossAmount: 100},
			},
		}
	}

	first, err := service.CreatePayrollRun(mk(day(time.March, 1), day(time.March, 15)))
	require.NoError(t, err)

	for _, tc := range []struct {
		name       string
		start, end time.Time
	}{
		{"inside", day(time.March, 5), day(time.March, 10)},
		{"straddles end", day(time.March, 10), day(time.March, 20)},
		{"straddles start", day(time.February, 20), day(time.March, 1)},
		{"shares last day", day(time.March, 15), day(time.March, 31)},
		{"contains", day(time.February, 1), day(time.April, 1)},
	} {
		_, err := service.CreatePayrollRun(mk(tc.start, tc.end))
		require.ErrorIs(t, err, ErrDuplicatePayrollPeriod, tc.name)
	}

	_, err = service.CreatePayrollRun(mk(day(time.March, 16), day(time.March, 31)))
	require.NoError(t, err, "adjacent period must be allowed")

	// Voiding the first run frees its days for a corrected re-run.
	_, err = service.MarkPayrollRunPaid(business.ID, first.ID, time.Now().UTC(), nil, nil)
	require.NoError(t, err)
	_, err = service.VoidPayrollRun(business.ID, first.ID, time.Now().UTC(), nil, nil)
	require.NoError(t, err)
	_, err = service.CreatePayrollRun(mk(day(time.March, 5), day(time.March, 10)))
	require.NoError(t, err, "overlap with a void run only must be allowed")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.PayrollRun{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Equal(t, int64(3), count)
}

// TestServiceCreatePayrollRun_ConcurrentDuplicatesCreateExactlyOne guards the
// double-payroll invariant: however many identical submissions race, exactly one
// run is created and every other caller gets ErrDuplicatePayrollPeriod. The
// pre-check alone was a read-then-write across statements; the authoritative
// re-check under the business-row lock inside the create transaction is what
// makes this hold under concurrency.
func TestServiceCreatePayrollRun_ConcurrentDuplicatesCreateExactlyOne(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerRace", "race")
	service := NewService(database.GetDBWrapper())

	input := CreatePayrollRunInput{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC),
		LineItems: []CreatePayrollLineItemInput{
			{PayeeType: database.PayrollPayeeTypeContractor, PayeeName: "Freelancer", GrossAmount: 100},
		},
	}

	const workers = 8
	var success, dup, other int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all goroutines together to maximize overlap
			_, err := service.CreatePayrollRun(input)
			switch {
			case err == nil:
				atomic.AddInt64(&success, 1)
			case errors.Is(err, ErrDuplicatePayrollPeriod):
				atomic.AddInt64(&dup, 1)
			default:
				atomic.AddInt64(&other, 1)
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int64(0), other, "no unexpected errors")
	assert.Equal(t, int64(1), atomic.LoadInt64(&success), "exactly one run must be created")
	assert.Equal(t, int64(workers-1), atomic.LoadInt64(&dup), "all other submissions must be rejected as duplicates")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.PayrollRun{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "exactly one payroll run persisted")
}

func TestServiceCreatePayrollRun_AllowsSamePeriodAfterVoid(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerDupVoid", "dupvoid")
	service := NewService(database.GetDBWrapper())

	input := CreatePayrollRunInput{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		LineItems: []CreatePayrollLineItemInput{
			{PayeeType: database.PayrollPayeeTypeContractor, PayeeName: "Freelancer", GrossAmount: 100},
		},
	}
	first, err := service.CreatePayrollRun(input)
	require.NoError(t, err)
	_, err = service.MarkPayrollRunPaid(business.ID, first.ID, time.Now().UTC(), nil, nil)
	require.NoError(t, err)
	_, err = service.VoidPayrollRun(business.ID, first.ID, time.Now().UTC(), nil, nil)
	require.NoError(t, err)

	// Once the prior run for this period is voided, a corrected run is allowed.
	_, err = service.CreatePayrollRun(input)
	require.NoError(t, err)
}

// --- PAY-2: delete drafts + void paid runs ---

func createDraftRunWithLine(t *testing.T, service *Service, businessID uint, staffID *uint) *database.PayrollRun {
	t.Helper()
	line := CreatePayrollLineItemInput{PayeeType: database.PayrollPayeeTypeContractor, PayeeName: "Freelancer", GrossAmount: 100}
	if staffID != nil {
		line = CreatePayrollLineItemInput{PayeeType: database.PayrollPayeeTypeStaff, StaffID: staffID, GrossAmount: 100}
	}
	run, err := service.CreatePayrollRun(CreatePayrollRunInput{
		BusinessID:  businessID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		LineItems:   []CreatePayrollLineItemInput{line},
	})
	require.NoError(t, err)
	return run
}

func TestServiceDeletePayrollRun_DeletesDraftAndLineItems(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerDel", "del")
	service := NewService(database.GetDBWrapper())
	run := createDraftRunWithLine(t, service, business.ID, nil)

	require.NoError(t, service.DeletePayrollRun(business.ID, run.ID))

	var runCount, lineCount int64
	require.NoError(t, database.GetDB().Model(&database.PayrollRun{}).Where("id = ?", run.ID).Count(&runCount).Error)
	require.NoError(t, database.GetDB().Model(&database.PayrollLineItem{}).Where("payroll_run_id = ?", run.ID).Count(&lineCount).Error)
	assert.Equal(t, int64(0), runCount, "draft run must be removed")
	assert.Equal(t, int64(0), lineCount, "line items must be removed")
}

func TestServiceDeletePayrollRun_RejectsPaidRun(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerDelPaid", "delpaid")
	service := NewService(database.GetDBWrapper())
	run := createDraftRunWithLine(t, service, business.ID, nil)
	_, err := service.MarkPayrollRunPaid(business.ID, run.ID, time.Now().UTC(), nil, nil)
	require.NoError(t, err)

	err = service.DeletePayrollRun(business.ID, run.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "draft")

	var runCount int64
	require.NoError(t, database.GetDB().Model(&database.PayrollRun{}).Where("id = ?", run.ID).Count(&runCount).Error)
	assert.Equal(t, int64(1), runCount, "paid run must NOT be deleted")
}

func TestServiceVoidPayrollRun_VoidsPaidRunAndExcludesFromSummary(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerVoidPaid", "voidpaid")
	service := NewService(database.GetDBWrapper())
	run := createDraftRunWithLine(t, service, business.ID, nil)
	paidAt := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.UTC)
	_, err := service.MarkPayrollRunPaid(business.ID, run.ID, paidAt, nil, nil)
	require.NoError(t, err)

	voidUser := uint(5)
	voided, err := service.VoidPayrollRun(business.ID, run.ID, time.Date(2026, time.March, 21, 0, 0, 0, 0, time.UTC), &voidUser, nil)
	require.NoError(t, err)
	assert.Equal(t, database.PayrollRunStatusVoid, voided.Status)
	require.NotNil(t, voided.VoidedAt)
	require.NotNil(t, voided.VoidedByUserID)
	assert.Equal(t, voidUser, *voided.VoidedByUserID)

	// A voided run must not contribute to P&L (window covers paidAt).
	summary, err := service.GetSummary(business.ID,
		time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.InDelta(t, 0, summary.PayrollTotal, 0.001, "voided run excluded from payroll P&L")
	assert.Equal(t, 0, summary.PayrollSummary.PaidRuns)
}

func TestServiceVoidPayrollRun_RejectsDraftRun(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerVoidDraft", "voiddraft")
	service := NewService(database.GetDBWrapper())
	run := createDraftRunWithLine(t, service, business.ID, nil)

	_, err := service.VoidPayrollRun(business.ID, run.ID, time.Now().UTC(), nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "paid")

	var persisted database.PayrollRun
	require.NoError(t, database.GetDB().First(&persisted, run.ID).Error)
	assert.Equal(t, database.PayrollRunStatusDraft, persisted.Status)
}

func TestServiceListPayrollRuns_IncludesVoidRuns(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerListVoid", "listvoid")
	service := NewService(database.GetDBWrapper())
	run := createDraftRunWithLine(t, service, business.ID, nil)
	_, err := service.MarkPayrollRunPaid(business.ID, run.ID, time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC), nil, nil)
	require.NoError(t, err)
	_, err = service.VoidPayrollRun(business.ID, run.ID, time.Now().UTC(), nil, nil)
	require.NoError(t, err)

	start := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC)
	runsPage, err := service.ListPayrollRunsPage(ListPayrollRunsParams{BusinessID: business.ID, Start: &start, End: &end, Page: 1, PageSize: 100})
	require.NoError(t, err)
	runs := runsPage.Runs
	require.Len(t, runs, 1, "voided run must still be listed for its period")
	assert.Equal(t, database.PayrollRunStatusVoid, runs[0].Status)
}

func TestServiceGetSummary_UsesPaidAtForPaidRuns(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	paidAt := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)

	run := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusPaid,
		PaidAt:      &paidAt,
		GrossTotal:  10000,
		NetTotal:    10000,
	}
	require.NoError(t, database.GetDB().Create(run).Error)

	service := NewService(database.GetDBWrapper())

	marchSummary, err := service.GetSummary(
		business.ID,
		time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 31, 23, 59, 59, 0, time.UTC),
	)
	require.NoError(t, err)
	assert.InDelta(t, 0, marchSummary.PayrollTotal, 0.001)
	assert.Equal(t, 0, marchSummary.PayrollSummary.PaidRuns)

	aprilSummary, err := service.GetSummary(
		business.ID,
		time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.April, 30, 23, 59, 59, 0, time.UTC),
	)
	require.NoError(t, err)
	assert.InDelta(t, 100, aprilSummary.PayrollTotal, 0.001)
	assert.Equal(t, 1, aprilSummary.PayrollSummary.PaidRuns)
}

func TestServiceListPayrollRuns_UsesPaidAtForPaidAndPeriodForDraft(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	paidAt := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)

	paidRun := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 25, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.April, 7, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusPaid,
		PaidAt:      &paidAt,
		GrossTotal:  10000,
		NetTotal:    10000,
	}
	require.NoError(t, database.GetDB().Create(paidRun).Error)

	draftRun := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 30, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusDraft,
		GrossTotal:  5000,
		NetTotal:    5000,
	}
	require.NoError(t, database.GetDB().Create(draftRun).Error)

	service := NewService(database.GetDBWrapper())

	marchStart := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	marchEnd := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	marchRunsPage, err := service.ListPayrollRunsPage(ListPayrollRunsParams{BusinessID: business.ID, Start: &marchStart, End: &marchEnd, Page: 1, PageSize: 100})
	require.NoError(t, err)
	marchRuns := marchRunsPage.Runs
	require.Len(t, marchRuns, 1)
	assert.Equal(t, draftRun.ID, marchRuns[0].ID)

	aprilStart := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	aprilEnd := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	aprilRunsPage, err := service.ListPayrollRunsPage(ListPayrollRunsParams{BusinessID: business.ID, Start: &aprilStart, End: &aprilEnd, Page: 1, PageSize: 100})
	require.NoError(t, err)
	aprilRuns := aprilRunsPage.Runs
	require.Len(t, aprilRuns, 1)
	assert.Equal(t, paidRun.ID, aprilRuns[0].ID)
}

// seedPayrollRunsWithLines creates nRuns draft runs with linesPerRun contractor
// line items each. Periods are non-overlapping (14-day blocks from base).
func seedPayrollRunsWithLines(t testing.TB, businessID uint, nRuns, linesPerRun int) {
	t.Helper()
	base := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < nRuns; i++ {
		start := base.AddDate(0, 0, i*14)
		end := start.AddDate(0, 0, 14)
		gross := int64(10000 * linesPerRun)
		run := &database.PayrollRun{
			BusinessID:  businessID,
			PeriodStart: start,
			PeriodEnd:   end,
			Status:      database.PayrollRunStatusDraft,
			Currency:    "USD",
			GrossTotal:  gross,
			NetTotal:    gross,
		}
		require.NoError(t, database.GetDB().Create(run).Error)
		for j := 0; j < linesPerRun; j++ {
			require.NoError(t, database.GetDB().Create(&database.PayrollLineItem{
				PayrollRunID: run.ID,
				BusinessID:   businessID,
				PayeeType:    database.PayrollPayeeTypeContractor,
				PayeeName:    fmt.Sprintf("Payee-%d-%d", i, j),
				GrossAmount:  10000,
				NetAmount:    10000,
			}).Error)
		}
	}
}

func TestServiceListPayrollRunsPage_Paginated(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerPage", "payroll-page")
	service := NewService(database.GetDBWrapper())

	seedPayrollRunsWithLines(t, business.ID, 25, 2)

	page, err := service.ListPayrollRunsPage(ListPayrollRunsParams{
		BusinessID: business.ID,
		Page:       1,
		PageSize:   10,
	})
	require.NoError(t, err)
	require.NotNil(t, page)
	require.Len(t, page.Runs, 10)
	assert.Equal(t, int64(25), page.Total)
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 10, page.PageSize)
	assert.Equal(t, 3, page.TotalPages)
	for _, row := range page.Runs {
		assert.Equal(t, 2, row.PayeeCount, "each seeded run has 2 line items")
		// Paged path must not hydrate LineItems (use payee_count instead).
		assert.Empty(t, row.LineItems)
	}

	// ORDER BY period_start DESC, id DESC — first row is the latest period.
	assert.True(t, page.Runs[0].PeriodStart.After(page.Runs[len(page.Runs)-1].PeriodStart) ||
		page.Runs[0].PeriodStart.Equal(page.Runs[len(page.Runs)-1].PeriodStart))
}

// TestServiceListPayrollRunsPage_PayeeCountQueryShape proves payee counts come
// from a single aggregate on payroll_line_items, not one query per run and not
// a full LineItems Preload fan-out.
func TestServiceListPayrollRunsPage_PayeeCountQueryShape(t *testing.T) {
	gormDB := setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerShape", "payroll-shape")
	service := NewService(database.GetDBWrapper())

	seedPayrollRunsWithLines(t, business.ID, 12, 3)

	var lineItemQueryCount atomic.Int64
	var sawGroupBy atomic.Bool
	// GORM Scan() routes through the Row processor (not Query). Count both so
	// a future switch to Find() still asserts the single-aggregate shape.
	captureLineItemSQL := func(tx *gorm.DB) {
		if tx.Statement == nil {
			return
		}
		sql := strings.ToLower(tx.Statement.SQL.String())
		table := tx.Statement.Table
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table != "" {
			table = tx.Statement.Schema.Table
		}
		if table != "payroll_line_items" && !strings.Contains(sql, "payroll_line_items") {
			return
		}
		lineItemQueryCount.Add(1)
		if _, ok := tx.Statement.Clauses["GROUP BY"]; ok {
			sawGroupBy.Store(true)
		}
		if strings.Contains(sql, "group by") || strings.Contains(sql, "count(") {
			sawGroupBy.Store(true)
		}
	}
	const cbQuery = "payverge:test:payroll_line_items_query_count"
	const cbRow = "payverge:test:payroll_line_items_row_count"
	require.NoError(t, gormDB.Callback().Query().After("gorm:query").Register(cbQuery, captureLineItemSQL))
	require.NoError(t, gormDB.Callback().Row().After("gorm:row").Register(cbRow, captureLineItemSQL))
	t.Cleanup(func() {
		_ = gormDB.Callback().Query().Remove(cbQuery)
		_ = gormDB.Callback().Row().Remove(cbRow)
	})

	page, err := service.ListPayrollRunsPage(ListPayrollRunsParams{
		BusinessID: business.ID,
		Page:       1,
		PageSize:   10,
	})
	require.NoError(t, err)
	require.Len(t, page.Runs, 10)
	assert.Equal(t, 3, page.Runs[0].PayeeCount)

	assert.Equal(t, int64(1), lineItemQueryCount.Load(),
		"payee counts must use ONE payroll_line_items query (got %d)", lineItemQueryCount.Load())
	assert.True(t, sawGroupBy.Load(), "payroll_line_items query must be a GROUP BY / COUNT aggregate")
}

func TestServiceGetSummary_NormalizesManualEntriesToReportingCurrency(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	business.DefaultCurrency = "ARS"
	require.NoError(t, database.GetDB().Save(business).Error)

	now := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	createExchangeRateRecord(t, "USDC", "USD", 1, now)
	createExchangeRateRecord(t, "USDC", "ARS", 1000, now)

	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeIncome,
		Category:    "service",
		Amount:      1000,
		Currency:    "USD",
		OccurredAt:  now.Add(2 * time.Hour),
		Description: "USD service payment",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeIncome,
		Category:    "service",
		Amount:      1000,
		Currency:    "USDC",
		OccurredAt:  now.Add(3 * time.Hour),
		Description: "USDC service payment",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeExpense,
		Category:    "software",
		Amount:      500,
		Currency:    "USD",
		OccurredAt:  now.Add(4 * time.Hour),
		Description: "USD software expense",
	}).Error)

	service := NewService(database.GetDBWrapper())
	summary, err := service.GetSummary(
		business.ID,
		now,
		now.Add(7*24*time.Hour),
	)
	require.NoError(t, err)

	assert.Equal(t, "ARS", summary.Currency)
	assert.InDelta(t, 20000, summary.ManualIncomeTotal, 0.001)
	assert.InDelta(t, 5000, summary.ExpenseTotal, 0.001)
	require.Len(t, summary.IncomeBreakdown, 1)
	assert.Equal(t, "service", summary.IncomeBreakdown[0].Category)
	assert.InDelta(t, 20000, summary.IncomeBreakdown[0].Total, 0.001)
	require.Len(t, summary.ExpenseBreakdown, 1)
	assert.Equal(t, "software", summary.ExpenseBreakdown[0].Category)
	assert.InDelta(t, 5000, summary.ExpenseBreakdown[0].Total, 0.001)
}

func TestServiceGetSummary_UsesHistoricalRateAtOccurredAt(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	business.DefaultCurrency = "ARS"
	require.NoError(t, database.GetDB().Save(business).Error)

	rateTime := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	occurredAt := time.Date(2026, time.February, 10, 12, 0, 0, 0, time.UTC)
	createExchangeRateRecord(t, "USDC", "USD", 1, rateTime)
	createExchangeRateRecord(t, "USDC", "ARS", 1000, rateTime)

	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeIncome,
		Category:    "service",
		Amount:      1000,
		Currency:    "USD",
		OccurredAt:  occurredAt,
		Description: "Historical USD sale",
	}).Error)

	service := NewService(database.GetDBWrapper())
	firstSummary, err := service.GetSummary(
		business.ID,
		time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	assert.InDelta(t, 10000, firstSummary.ManualIncomeTotal, 0.001)

	createExchangeRateRecord(t, "USDC", "ARS", 2000, time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC))

	secondSummary, err := service.GetSummary(
		business.ID,
		time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	assert.InDelta(t, 10000, secondSummary.ManualIncomeTotal, 0.001)
}

func TestServiceGetSummary_UsesEntrySpecificHistoricalRatesWithinSamePeriod(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	business.DefaultCurrency = "ARS"
	require.NoError(t, database.GetDB().Save(business).Error)

	periodStart := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)

	createExchangeRateRecord(t, "USDC", "USD", 1, time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC))
	createExchangeRateRecord(t, "USDC", "ARS", 1000, time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC))
	createExchangeRateRecord(t, "USDC", "ARS", 2000, time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC))

	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeIncome,
		Category:    "service",
		Amount:      1000,
		Currency:    "USD",
		OccurredAt:  time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC),
		Description: "Before rate update",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeIncome,
		Category:    "service",
		Amount:      1000,
		Currency:    "USD",
		OccurredAt:  time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC),
		Description: "After rate update",
	}).Error)

	service := NewService(database.GetDBWrapper())
	summary, err := service.GetSummary(business.ID, periodStart, periodEnd)
	require.NoError(t, err)
	assert.InDelta(t, 30000, summary.ManualIncomeTotal, 0.001)
	require.Len(t, summary.IncomeBreakdown, 1)
	assert.InDelta(t, 30000, summary.IncomeBreakdown[0].Total, 0.001)
}

func TestServiceGetSummary_SkipsLegacyUnconvertibleEntriesAndReportsWarning(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerA", "alpha")
	business.DefaultCurrency = "ARS"
	require.NoError(t, database.GetDB().Save(business).Error)

	now := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	createExchangeRateRecord(t, "USDC", "USD", 1, now)
	createExchangeRateRecord(t, "USDC", "ARS", 1000, now)

	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeIncome,
		Category:    "service",
		Amount:      1000,
		Currency:    "USD",
		OccurredAt:  now.Add(90 * time.Minute),
		Description: "Convertible service payment",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeIncome,
		Category:    "service",
		Amount:      1000,
		Currency:    "BTC",
		OccurredAt:  now.Add(2 * time.Hour),
		Description: "BTC service payment",
	}).Error)

	service := NewService(database.GetDBWrapper())
	summary, err := service.GetSummary(
		business.ID,
		now,
		now.Add(7*24*time.Hour),
	)
	require.NoError(t, err)
	assert.InDelta(t, 10000, summary.ManualIncomeTotal, 0.001)
	assert.Equal(t, 1, summary.SkippedManualEntries)
	require.Len(t, summary.Warnings, 1)
	assert.Equal(t, "fx_rate_missing", summary.Warnings[0].Code)
	assert.Equal(t, "1", summary.Warnings[0].Params["count"])
	assert.Contains(t, summary.Warnings[0].Params["details"], "BTC")
}

// TestCreatePayrollRun_StaffValidationIssuesOneQuery is the access-shape gate for
// N1-04. With K staff line items the old code issued K per-item First() lookups.
// After the fix a single batch SELECT must cover all staff IDs.
func TestCreatePayrollRun_StaffValidationIssuesOneQuery(t *testing.T) {
	gormDB := setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerP", "payroll-batch")

	const K = 5
	staffIDs := make([]*uint, K)
	for i := 0; i < K; i++ {
		s := createAccountingStaff(t, business.ID,
			fmt.Sprintf("staff%d@example.com", i),
			fmt.Sprintf("Staff%d", i),
		)
		staffIDs[i] = &s.ID
	}

	// Count SELECT queries that touch the staffs table.
	var staffQueryCount atomic.Int64
	const cbName = "payverge:test:payroll_staff_query_count"
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "staff" {
			staffQueryCount.Add(1)
		}
	}))
	t.Cleanup(func() { _ = gormDB.Callback().Query().Remove(cbName) })

	items := make([]CreatePayrollLineItemInput, K)
	for i := 0; i < K; i++ {
		items[i] = CreatePayrollLineItemInput{
			PayeeType:       database.PayrollPayeeTypeStaff,
			StaffID:         staffIDs[i],
			GrossAmount:     100,
			BonusAmount:     0,
			DeductionAmount: 0,
		}
	}

	service := NewService(database.GetDBWrapper())
	run, err := service.CreatePayrollRun(CreatePayrollRunInput{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC),
		LineItems:   items,
	})
	require.NoError(t, err)
	require.Len(t, run.LineItems, K)

	// After the fix: exactly 1 SELECT to the staffs table, not K.
	assert.Equal(t, int64(1), staffQueryCount.Load(),
		"CreatePayrollRun must batch staff lookup into a single query (was %d, want 1)",
		staffQueryCount.Load())
}

// TestCreatePayrollRun_InvalidStaffIDAtIndex asserts that an unknown or
// cross-business staff_id at position j returns the exact error string with
// the correct index, and that an earlier gross_amount<=0 error still fires
// first (order preserved).
func TestCreatePayrollRun_InvalidStaffIDAtIndex(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerQ", "payroll-invalid")
	otherBusiness := createAccountingBusiness(t, "0xOwnerR", "other-biz")
	validStaff := createAccountingStaff(t, business.ID, "ok@example.com", "OkStaff")
	otherStaff := createAccountingStaff(t, otherBusiness.ID, "other@example.com", "OtherStaff")

	service := NewService(database.GetDBWrapper())

	t.Run("invalid staff_id returns correct index error", func(t *testing.T) {
		fakeID := uint(999999)
		_, err := service.CreatePayrollRun(CreatePayrollRunInput{
			BusinessID:  business.ID,
			PeriodStart: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:   time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC),
			LineItems: []CreatePayrollLineItemInput{
				{
					PayeeType:   database.PayrollPayeeTypeStaff,
					StaffID:     &validStaff.ID,
					GrossAmount: 100,
				},
				{
					PayeeType:   database.PayrollPayeeTypeStaff,
					StaffID:     &fakeID,
					GrossAmount: 100,
				},
			},
		})
		require.Error(t, err)
		assert.Equal(t, "line_items[1].staff_id is invalid", err.Error())
	})

	t.Run("cross-business staff_id is rejected as invalid", func(t *testing.T) {
		_, err := service.CreatePayrollRun(CreatePayrollRunInput{
			BusinessID:  business.ID,
			PeriodStart: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:   time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC),
			LineItems: []CreatePayrollLineItemInput{
				{
					PayeeType:   database.PayrollPayeeTypeStaff,
					StaffID:     &otherStaff.ID,
					GrossAmount: 100,
				},
			},
		})
		require.Error(t, err)
		assert.Equal(t, "line_items[0].staff_id is invalid", err.Error())
	})

	t.Run("gross<=0 at earlier index fires before invalid staff at later index", func(t *testing.T) {
		fakeID := uint(888888)
		_, err := service.CreatePayrollRun(CreatePayrollRunInput{
			BusinessID:  business.ID,
			PeriodStart: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:   time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC),
			LineItems: []CreatePayrollLineItemInput{
				{
					PayeeType:   database.PayrollPayeeTypeStaff,
					StaffID:     &validStaff.ID,
					GrossAmount: 0, // fires first: gross<=0 at index 0
				},
				{
					PayeeType:   database.PayrollPayeeTypeStaff,
					StaffID:     &fakeID,
					GrossAmount: 100,
				},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "line_items[0]")
		assert.Contains(t, err.Error(), "gross_amount")
	})
}

// BenchmarkCreatePayrollRun measures a K=10 staff payroll run end-to-end. With
// the batched staff validation this is one staff SELECT regardless of K; before
// N1-04 it was K per-item First() lookups. Run with -benchmem -count=3.
func BenchmarkCreatePayrollRun(b *testing.B) {
	dsn := fmt.Sprintf("file:bench_payroll_run_%d?mode=memory", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	if err := gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.PayrollRun{},
		&database.PayrollLineItem{},
	); err != nil {
		b.Fatal(err)
	}

	biz := &database.Business{
		BusinessId:      "bench-payroll",
		Name:            "Bench",
		OwnerAddress:    "0xBench",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DisplayCurrency: "USD",
		DefaultLanguage: "en",
		SourceLanguage:  "en",
	}
	if err := gormDB.Create(biz).Error; err != nil {
		b.Fatal(err)
	}

	const K = 10
	staffIDs := make([]*uint, K)
	for i := 0; i < K; i++ {
		s := &database.Staff{
			BusinessID: biz.ID,
			Email:      fmt.Sprintf("bench%d@example.com", i),
			Name:       fmt.Sprintf("Bench%d", i),
			Role:       database.StaffRoleManager,
			IsActive:   true,
			InvitedBy:  "owner",
		}
		if err := gormDB.Create(s).Error; err != nil {
			b.Fatal(err)
		}
		id := s.ID
		staffIDs[i] = &id
	}

	items := make([]CreatePayrollLineItemInput, K)
	for i := 0; i < K; i++ {
		items[i] = CreatePayrollLineItemInput{
			PayeeType:       database.PayrollPayeeTypeStaff,
			StaffID:         staffIDs[i],
			GrossAmount:     100,
			BonusAmount:     5,
			DeductionAmount: 2,
		}
	}

	service := NewService(database.GetDBWrapper())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Clean up between iterations so auto-inc IDs don't accumulate.
		gormDB.Exec("DELETE FROM payroll_line_items")
		gormDB.Exec("DELETE FROM payroll_runs")

		if _, err := service.CreatePayrollRun(CreatePayrollRunInput{
			BusinessID:  biz.ID,
			PeriodStart: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:   time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC),
			LineItems:   items,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkListPayrollRuns measures payroll list load for a dense business
// (100 runs × 8 line items). Baseline is the legacy Preload path; after Task 2
// the paged path uses COUNT + LIMIT/OFFSET + one payee_count aggregate.
// Run: go test ./internal/accounting/ -bench BenchmarkListPayrollRuns -benchmem -count=3 -run '^$'
func BenchmarkListPayrollRuns(b *testing.B) {
	dsn := fmt.Sprintf("file:bench_list_payroll_%d?mode=memory", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	if err := gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.PayrollRun{},
		&database.PayrollLineItem{},
	); err != nil {
		b.Fatal(err)
	}

	biz := &database.Business{
		BusinessId:      "bench-list-payroll",
		Name:            "BenchList",
		OwnerAddress:    "0xBenchList",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DisplayCurrency: "USD",
		DefaultLanguage: "en",
		SourceLanguage:  "en",
	}
	if err := gormDB.Create(biz).Error; err != nil {
		b.Fatal(err)
	}

	const nRuns = 100
	const linesPerRun = 8
	base := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < nRuns; i++ {
		start := base.AddDate(0, 0, i*14)
		end := start.AddDate(0, 0, 14)
		run := &database.PayrollRun{
			BusinessID:  biz.ID,
			PeriodStart: start,
			PeriodEnd:   end,
			Status:      database.PayrollRunStatusDraft,
			Currency:    "USD",
			GrossTotal:  int64(10000 * linesPerRun),
			NetTotal:    int64(10000 * linesPerRun),
		}
		if err := gormDB.Create(run).Error; err != nil {
			b.Fatal(err)
		}
		for j := 0; j < linesPerRun; j++ {
			if err := gormDB.Create(&database.PayrollLineItem{
				PayrollRunID: run.ID,
				BusinessID:   biz.ID,
				PayeeType:    database.PayrollPayeeTypeContractor,
				PayeeName:    fmt.Sprintf("P-%d-%d", i, j),
				GrossAmount:  10000,
				NetAmount:    10000,
			}).Error; err != nil {
				b.Fatal(err)
			}
		}
	}

	service := NewService(database.GetDBWrapper())
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Paged path: COUNT + LIMIT/OFFSET page + one payee_count aggregate.
		if _, err := service.ListPayrollRunsPage(ListPayrollRunsParams{
			BusinessID: biz.ID,
			Page:       1,
			PageSize:   20,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func TestServiceListEntries_Filters(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerListFilt", "listfilt")
	staff := createAccountingStaff(t, business.ID, "listfilt@example.com", "ListFilt")
	service := NewService(database.GetDBWrapper())

	base := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)

	// Active expense — rent, searchable description
	rentActive := &database.ManualLedgerEntry{
		BusinessID:       business.ID,
		EntryType:        database.AccountingEntryTypeExpense,
		Category:         "rent",
		Amount:           185000, // $1850
		Currency:         "USD",
		OccurredAt:       base,
		Description:      "March Rent Payment",
		Reference:        "INV-100",
		CreatedByStaffID: &staff.ID,
	}
	// Active expense — utilities, different category/amount
	utilActive := &database.ManualLedgerEntry{
		BusinessID:       business.ID,
		EntryType:        database.AccountingEntryTypeExpense,
		Category:         "utilities",
		Amount:           42000, // $420
		Currency:         "USD",
		OccurredAt:       base.Add(1 * time.Hour),
		Description:      "Electric bill",
		Reference:        "UTIL-9",
		CreatedByStaffID: &staff.ID,
	}
	// Active income — catering, searchable reference
	cateringActive := &database.ManualLedgerEntry{
		BusinessID:       business.ID,
		EntryType:        database.AccountingEntryTypeIncome,
		Category:         "catering",
		Amount:           99000, // $990
		Currency:         "USD",
		OccurredAt:       base.Add(2 * time.Hour),
		Description:      "Wedding deposit",
		Reference:        "CAT-REF-42",
		CreatedByStaffID: &staff.ID,
	}
	// Soft-void candidate — marketing
	mktToVoid := &database.ManualLedgerEntry{
		BusinessID:       business.ID,
		EntryType:        database.AccountingEntryTypeExpense,
		Category:         "marketing",
		Amount:           15000, // $150
		Currency:         "USD",
		OccurredAt:       base.Add(3 * time.Hour),
		Description:      "Ad spend",
		Reference:        "ADS-1",
		CreatedByStaffID: &staff.ID,
	}

	for _, e := range []*database.ManualLedgerEntry{rentActive, utilActive, cateringActive, mktToVoid} {
		require.NoError(t, database.GetDB().Create(e).Error)
	}
	require.NoError(t, service.VoidManualEntry(business.ID, mktToVoid.ID, nil, &staff.ID))

	// --- status: active (voided_at IS NULL) ---
	activePage, err := service.ListEntries(ListEntriesInput{
		BusinessID: business.ID,
		Status:     "active",
		Page:       1,
		PageSize:   50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), activePage.Total)
	for _, e := range activePage.Entries {
		assert.Nil(t, e.VoidedAt, "active filter must exclude voided rows")
	}

	// --- status: voided ---
	voidedPage, err := service.ListEntries(ListEntriesInput{
		BusinessID: business.ID,
		Status:     "voided",
		Page:       1,
		PageSize:   50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), voidedPage.Total)
	require.Len(t, voidedPage.Entries, 1)
	assert.Equal(t, mktToVoid.ID, voidedPage.Entries[0].ID)
	require.NotNil(t, voidedPage.Entries[0].VoidedAt)

	// --- status: all / empty → no status filter (includes voided) ---
	allPage, err := service.ListEntries(ListEntriesInput{
		BusinessID: business.ID,
		Status:     "all",
		Page:       1,
		PageSize:   50,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(4), allPage.Total)

	emptyStatusPage, err := service.ListEntries(ListEntriesInput{
		BusinessID: business.ID,
		Page:       1,
		PageSize:   50,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(4), emptyStatusPage.Total)

	// --- category exact match ---
	rentPage, err := service.ListEntries(ListEntriesInput{
		BusinessID: business.ID,
		Category:   "rent",
		Page:       1,
		PageSize:   50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), rentPage.Total)
	assert.Equal(t, rentActive.ID, rentPage.Entries[0].ID)

	// --- search (q): case-insensitive match on description ---
	descPage, err := service.ListEntries(ListEntriesInput{
		BusinessID: business.ID,
		Query:      "march rent",
		Page:       1,
		PageSize:   50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), descPage.Total)
	assert.Equal(t, rentActive.ID, descPage.Entries[0].ID)

	// --- search (q): case-insensitive match on reference ---
	refPage, err := service.ListEntries(ListEntriesInput{
		BusinessID: business.ID,
		Query:      "cat-ref",
		Page:       1,
		PageSize:   50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), refPage.Total)
	assert.Equal(t, cateringActive.ID, refPage.Entries[0].ID)

	// --- sort: amount_desc ---
	amountDesc, err := service.ListEntries(ListEntriesInput{
		BusinessID: business.ID,
		Status:     "active",
		Sort:       "amount_desc",
		Page:       1,
		PageSize:   50,
	})
	require.NoError(t, err)
	require.Len(t, amountDesc.Entries, 3)
	assert.Equal(t, rentActive.ID, amountDesc.Entries[0].ID, "largest amount first")
	assert.Equal(t, cateringActive.ID, amountDesc.Entries[1].ID)
	assert.Equal(t, utilActive.ID, amountDesc.Entries[2].ID)

	// --- sort: amount_asc ---
	amountAsc, err := service.ListEntries(ListEntriesInput{
		BusinessID: business.ID,
		Status:     "active",
		Sort:       "amount_asc",
		Page:       1,
		PageSize:   50,
	})
	require.NoError(t, err)
	require.Len(t, amountAsc.Entries, 3)
	assert.Equal(t, utilActive.ID, amountAsc.Entries[0].ID, "smallest amount first")
	assert.Equal(t, cateringActive.ID, amountAsc.Entries[1].ID)
	assert.Equal(t, rentActive.ID, amountAsc.Entries[2].ID)

	// --- sort: occurred_at_asc ---
	occAsc, err := service.ListEntries(ListEntriesInput{
		BusinessID: business.ID,
		Status:     "active",
		Sort:       "occurred_at_asc",
		Page:       1,
		PageSize:   50,
	})
	require.NoError(t, err)
	require.Len(t, occAsc.Entries, 3)
	assert.Equal(t, rentActive.ID, occAsc.Entries[0].ID)
	assert.Equal(t, utilActive.ID, occAsc.Entries[1].ID)
	assert.Equal(t, cateringActive.ID, occAsc.Entries[2].ID)

	// --- default sort remains occurred_at DESC ---
	defaultSort, err := service.ListEntries(ListEntriesInput{
		BusinessID: business.ID,
		Status:     "active",
		Page:       1,
		PageSize:   50,
	})
	require.NoError(t, err)
	require.Len(t, defaultSort.Entries, 3)
	assert.Equal(t, cateringActive.ID, defaultSort.Entries[0].ID)
	assert.Equal(t, utilActive.ID, defaultSort.Entries[1].ID)
	assert.Equal(t, rentActive.ID, defaultSort.Entries[2].ID)
}

// TestGetTimeseriesBucketsByDay locks the day-bucket timeseries contract used by
// Overview sparklines: manual income/expense (voided excluded) and paid payroll
// are summed in dollars per business-local day, with contiguous zero-filled
// series. Amounts are int64 cents in DB → float64 dollars on the wire.
func TestGetTimeseriesBucketsByDay(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerTS", "ts-buckets")
	staff := createAccountingStaff(t, business.ID, "ts@example.com", "TSManager")

	// Half-open [Jan 10, Jan 13) → three contiguous day buckets.
	start := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.January, 13, 0, 0, 0, 0, time.UTC)

	// Day 0: two income entries (10.00 + 16.50 = 26.50).
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:       business.ID,
		EntryType:        database.AccountingEntryTypeIncome,
		Category:         "catering",
		Amount:           1000, // $10.00
		Currency:         "USD",
		OccurredAt:       start.Add(2 * time.Hour),
		Description:      "income a",
		CreatedByStaffID: &staff.ID,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:       business.ID,
		EntryType:        database.AccountingEntryTypeIncome,
		Category:         "event",
		Amount:           1650, // $16.50
		Currency:         "USD",
		OccurredAt:       start.Add(3 * time.Hour),
		Description:      "income b",
		CreatedByStaffID: &staff.ID,
	}).Error)

	// Voided income same day — must be excluded from the series.
	voidedAt := start.Add(5 * time.Hour)
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:       business.ID,
		EntryType:        database.AccountingEntryTypeIncome,
		Category:         "other",
		Amount:           99900, // $999.00 would pollute if included
		Currency:         "USD",
		OccurredAt:       start.Add(4 * time.Hour),
		Description:      "voided income",
		CreatedByStaffID: &staff.ID,
		VoidedAt:         &voidedAt,
	}).Error)

	// Day 1: one expense ($7.60).
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:       business.ID,
		EntryType:        database.AccountingEntryTypeExpense,
		Category:         "supplies",
		Amount:           760, // $7.60
		Currency:         "USD",
		OccurredAt:       start.Add(26 * time.Hour),
		Description:      "supplies",
		CreatedByStaffID: &staff.ID,
	}).Error)

	// Day 2: paid payroll ($486.00 net). Draft payroll must not appear.
	paidAt := start.Add(50 * time.Hour)
	require.NoError(t, database.GetDB().Create(&database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: start,
		PeriodEnd:   start.Add(24 * time.Hour),
		Status:      database.PayrollRunStatusPaid,
		Currency:    "USD",
		PaidAt:      &paidAt,
		GrossTotal:  48600,
		NetTotal:    48600, // $486.00
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: start,
		PeriodEnd:   start.Add(24 * time.Hour),
		Status:      database.PayrollRunStatusDraft,
		Currency:    "USD",
		GrossTotal:  99900,
		NetTotal:    99900,
	}).Error)

	service := NewService(database.GetDBWrapper())
	ts, err := service.GetTimeseries(business.ID, start, end, time.UTC)
	require.NoError(t, err)
	require.NotNil(t, ts)

	require.Equal(t, "day", ts.Bucket)
	require.Equal(t, "USD", ts.Currency)
	require.Equal(t, "2026-01-10", ts.Start)
	require.Equal(t, "2026-01-12", ts.End)
	require.Len(t, ts.Series, 3, "contiguous zero-filled series for [start,end)")

	assert.Equal(t, "2026-01-10", ts.Series[0].Date)
	assert.InDelta(t, 26.5, ts.Series[0].Income, 0.001)
	assert.InDelta(t, 0.0, ts.Series[0].Expense, 0.001)
	assert.InDelta(t, 0.0, ts.Series[0].Payroll, 0.001)

	assert.Equal(t, "2026-01-11", ts.Series[1].Date)
	assert.InDelta(t, 0.0, ts.Series[1].Income, 0.001)
	assert.InDelta(t, 7.6, ts.Series[1].Expense, 0.001)
	assert.InDelta(t, 0.0, ts.Series[1].Payroll, 0.001)

	assert.Equal(t, "2026-01-12", ts.Series[2].Date)
	assert.InDelta(t, 0.0, ts.Series[2].Income, 0.001)
	assert.InDelta(t, 0.0, ts.Series[2].Expense, 0.001)
	assert.InDelta(t, 486.0, ts.Series[2].Payroll, 0.001)
}

// TestGetTimeseries_RejectsRangeOver400Days locks the 400-day cap so a hand-
// edited URL cannot request unbounded series payloads.
func TestGetTimeseries_RejectsRangeOver400Days(t *testing.T) {
	setupAccountingTestDB(t)
	business := createAccountingBusiness(t, "0xOwnerTSCap", "ts-cap")

	start := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 401) // 401 day buckets

	service := NewService(database.GetDBWrapper())
	_, err := service.GetTimeseries(business.ID, start, end, time.UTC)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRangeTooLarge)
}

// TestGetSummaryPreAggregationEquivalence locks GetSummary totals, category
// breakdowns, payroll rollups, voided exclusion, paid_at windowing, multi-currency
// FX, and skipped-entry warnings so SQL pre-aggregation cannot change the public
// Summary JSON shape or values for a messy multi-currency fixture.
func TestGetSummaryPreAggregationEquivalence(t *testing.T) {
	setupAccountingTestDB(t)

	business := createAccountingBusiness(t, "0xOwnerPreAgg", "preagg")
	staff := createAccountingStaff(t, business.ID, "preagg@example.com", "PreAgg")

	// Half-open [June 1, June 11) — ~10 calendar days.
	start := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.June, 11, 0, 0, 0, 0, time.UTC)

	// Rates: EUR steps mid-window; GBP stable; BTC never convertible.
	createExchangeRateRecord(t, "EUR", "USD", 1.10, time.Date(2026, time.May, 31, 0, 0, 0, 0, time.UTC))
	createExchangeRateRecord(t, "EUR", "USD", 1.20, time.Date(2026, time.June, 5, 0, 0, 0, 0, time.UTC))
	createExchangeRateRecord(t, "GBP", "USD", 1.25, time.Date(2026, time.May, 31, 0, 0, 0, 0, time.UTC))

	staffID := staff.ID
	mkEntry := func(entryType database.AccountingEntryType, category string, amount int64, currency string, occurredAt time.Time, voided bool) *database.ManualLedgerEntry {
		t.Helper()
		e := &database.ManualLedgerEntry{
			BusinessID:       business.ID,
			EntryType:        entryType,
			Category:         category,
			Amount:           amount,
			Currency:         currency,
			OccurredAt:       occurredAt,
			Description:      fmt.Sprintf("%s %s %s", entryType, category, currency),
			CreatedByStaffID: &staffID,
		}
		if voided {
			v := occurredAt.Add(time.Hour)
			e.VoidedAt = &v
		}
		require.NoError(t, database.GetDB().Create(e).Error)
		return e
	}

	// --- Income (active + voided + unconvertible) across several days ---
	mkEntry(database.AccountingEntryTypeIncome, "catering", 10000, "USD", start.Add(10*time.Hour), false)                         // $100
	mkEntry(database.AccountingEntryTypeIncome, "catering", 5000, "EUR", start.Add(14*time.Hour), false)                          // €50 → $55 @1.10
	mkEntry(database.AccountingEntryTypeIncome, "catering", 99999, "EUR", start.Add(36*time.Hour), true)                          // voided — excluded
	mkEntry(database.AccountingEntryTypeIncome, "service", 4000, "GBP", start.Add(58*time.Hour), false)                           // £40 → $50 @1.25
	mkEntry(database.AccountingEntryTypeIncome, "catering", 10000, "EUR", start.Add(4*24*time.Hour+10*time.Hour), false)          // €100 → $120 @1.20 (June 5)
	btcIncome := mkEntry(database.AccountingEntryTypeIncome, "other", 1000, "BTC", start.Add(5*24*time.Hour+10*time.Hour), false) // skipped
	mkEntry(database.AccountingEntryTypeIncome, "event", 2500, "USD", start.Add(7*24*time.Hour+10*time.Hour), false)              // $25
	mkEntry(database.AccountingEntryTypeIncome, "service", 2000, "EUR", start.Add(7*24*time.Hour+15*time.Hour), false)            // €20 → $24 @1.20

	// Outside window — must not appear.
	mkEntry(database.AccountingEntryTypeIncome, "catering", 88800, "USD", start.Add(-24*time.Hour), false)
	mkEntry(database.AccountingEntryTypeIncome, "catering", 77700, "USD", end.Add(time.Hour), false)

	// --- Expense ---
	mkEntry(database.AccountingEntryTypeExpense, "supplies", 2000, "USD", start.Add(11*time.Hour), false)                             // $20
	mkEntry(database.AccountingEntryTypeExpense, "rent", 3000, "EUR", start.Add(3*24*time.Hour+10*time.Hour), false)                  // €30 → $33 @1.10
	mkEntry(database.AccountingEntryTypeExpense, "utilities", 5000, "USD", start.Add(6*24*time.Hour+10*time.Hour), true)              // voided
	mkEntry(database.AccountingEntryTypeExpense, "software", 800, "GBP", start.Add(8*24*time.Hour+10*time.Hour), false)               // £8 → $10
	btcExpense := mkEntry(database.AccountingEntryTypeExpense, "supplies", 500, "BTC", start.Add(5*24*time.Hour+14*time.Hour), false) // skipped

	// --- Payroll: paid in-window, draft, void, paid outside window, unconvertible ---
	paidAt := func(d time.Duration) *time.Time {
		t := start.Add(d)
		return &t
	}
	require.NoError(t, database.GetDB().Create(&database.PayrollRun{
		BusinessID: business.ID, PeriodStart: start, PeriodEnd: start.Add(24 * time.Hour),
		Status: database.PayrollRunStatusPaid, Currency: "EUR", PaidAt: paidAt(36 * time.Hour),
		GrossTotal: 8000, BonusTotal: 0, DeductionTotal: 0, NetTotal: 8000, // €80 → $88
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.PayrollRun{
		BusinessID: business.ID, PeriodStart: start, PeriodEnd: start.Add(24 * time.Hour),
		Status: database.PayrollRunStatusDraft, Currency: "USD",
		GrossTotal: 99900, NetTotal: 99900, // draft — excluded
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.PayrollRun{
		BusinessID: business.ID, PeriodStart: start, PeriodEnd: start.Add(24 * time.Hour),
		Status: database.PayrollRunStatusPaid, Currency: "USD", PaidAt: paidAt(5*24*time.Hour + 12*time.Hour),
		GrossTotal: 15000, BonusTotal: 1000, DeductionTotal: 500, NetTotal: 15500, // $150 / $10 / $5 / $155
	}).Error)
	voidPaidAt := paidAt(7*24*time.Hour + 12*time.Hour)
	require.NoError(t, database.GetDB().Create(&database.PayrollRun{
		BusinessID: business.ID, PeriodStart: start, PeriodEnd: start.Add(24 * time.Hour),
		Status: database.PayrollRunStatusVoid, Currency: "USD", PaidAt: voidPaidAt,
		GrossTotal: 50000, NetTotal: 50000, // void — excluded
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.PayrollRun{
		BusinessID: business.ID, PeriodStart: start, PeriodEnd: start.Add(24 * time.Hour),
		Status: database.PayrollRunStatusPaid, Currency: "GBP", PaidAt: paidAt(9*24*time.Hour + 10*time.Hour),
		GrossTotal: 1000, BonusTotal: 0, DeductionTotal: 0, NetTotal: 1000, // £10 → $12.50
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.PayrollRun{
		BusinessID: business.ID, PeriodStart: start, PeriodEnd: start.Add(24 * time.Hour),
		Status: database.PayrollRunStatusPaid, Currency: "BTC", PaidAt: paidAt(8*24*time.Hour + 10*time.Hour),
		GrossTotal: 500, NetTotal: 500, // unconvertible — skipped
	}).Error)
	// Paid outside window (period inside — cash basis uses paid_at).
	require.NoError(t, database.GetDB().Create(&database.PayrollRun{
		BusinessID: business.ID, PeriodStart: start, PeriodEnd: start.Add(24 * time.Hour),
		Status: database.PayrollRunStatusPaid, Currency: "USD", PaidAt: paidAt(-2 * time.Hour),
		GrossTotal: 70000, NetTotal: 70000,
	}).Error)

	service := NewService(database.GetDBWrapper())
	summary, err := service.GetSummary(business.ID, start, end)
	require.NoError(t, err)
	require.NotNil(t, summary)

	// Public field names / shape.
	assert.Equal(t, "USD", summary.Currency)
	assert.True(t, summary.StartDate.Equal(start))
	assert.True(t, summary.EndDate.Equal(end))

	// Manual income: 100 + 55 + 50 + 120 + 25 + 24 = 374 (BTC skipped).
	assert.InDelta(t, 374.0, summary.ManualIncomeTotal, 1e-6)
	// Expense: 20 + 33 + 10 = 63 (voided + BTC skipped).
	assert.InDelta(t, 63.0, summary.ExpenseTotal, 1e-6)
	// Payroll net: 88 + 155 + 12.50 = 255.50
	assert.InDelta(t, 255.50, summary.PayrollTotal, 1e-6)
	assert.InDelta(t, 0.0, summary.AutoIncomeTotal, 1e-6)
	assert.InDelta(t, 0.0, summary.BilledTotal, 1e-6)
	assert.InDelta(t, 0.0, summary.CollectedTotal, 1e-6)
	assert.InDelta(t, 0.0, summary.CollectionGap, 1e-6)
	// Component net without COGS would be 374 - 63 - 255.50 = 55.50; full Net
	// (with COGS) is ComposeProfitLoss only — not on Summary.

	// Category breakdowns (sorted by category name).
	require.Len(t, summary.IncomeBreakdown, 3)
	assert.Equal(t, "catering", summary.IncomeBreakdown[0].Category)
	assert.InDelta(t, 275.0, summary.IncomeBreakdown[0].Total, 1e-6) // 100+55+120
	assert.Equal(t, "event", summary.IncomeBreakdown[1].Category)
	assert.InDelta(t, 25.0, summary.IncomeBreakdown[1].Total, 1e-6)
	assert.Equal(t, "service", summary.IncomeBreakdown[2].Category)
	assert.InDelta(t, 74.0, summary.IncomeBreakdown[2].Total, 1e-6) // 50+24

	require.Len(t, summary.ExpenseBreakdown, 3)
	assert.Equal(t, "rent", summary.ExpenseBreakdown[0].Category)
	assert.InDelta(t, 33.0, summary.ExpenseBreakdown[0].Total, 1e-6)
	assert.Equal(t, "software", summary.ExpenseBreakdown[1].Category)
	assert.InDelta(t, 10.0, summary.ExpenseBreakdown[1].Total, 1e-6)
	assert.Equal(t, "supplies", summary.ExpenseBreakdown[2].Category)
	assert.InDelta(t, 20.0, summary.ExpenseBreakdown[2].Total, 1e-6)

	// Payroll rollup: 3 paid convertible runs.
	assert.Equal(t, 3, summary.PayrollSummary.PaidRuns)
	assert.InDelta(t, 88.0+150.0+12.50, summary.PayrollSummary.TotalGross, 1e-6)
	assert.InDelta(t, 0.0+10.0+0.0, summary.PayrollSummary.TotalBonus, 1e-6)
	assert.InDelta(t, 0.0+5.0+0.0, summary.PayrollSummary.TotalDeduction, 1e-6)
	assert.InDelta(t, 255.50, summary.PayrollSummary.TotalNet, 1e-6)

	// 1 income BTC + 1 expense BTC + 1 payroll BTC.
	assert.Equal(t, 3, summary.SkippedManualEntries)
	require.Len(t, summary.Warnings, 3)
	for _, w := range summary.Warnings {
		assert.Equal(t, "fx_rate_missing", w.Code)
	}
	// Income warning samples entry id + currency.
	assert.Equal(t, "1", summary.Warnings[0].Params["count"])
	assert.Equal(t, "income", summary.Warnings[0].Params["entry_type"])
	assert.Contains(t, summary.Warnings[0].Params["details"], fmt.Sprintf("#%d BTC", btcIncome.ID))
	assert.Equal(t, "1", summary.Warnings[1].Params["count"])
	assert.Equal(t, "expense", summary.Warnings[1].Params["entry_type"])
	assert.Contains(t, summary.Warnings[1].Params["details"], fmt.Sprintf("#%d BTC", btcExpense.ID))
	assert.Equal(t, "1", summary.Warnings[2].Params["count"])
	assert.Equal(t, "payroll", summary.Warnings[2].Params["scope"])
	assert.Contains(t, summary.Warnings[2].Params["details"], "BTC")
}

// BenchmarkGetSummary measures P&L at scale: many manual entries + paid payroll
// runs across multiple currencies/days. Use 2k entries + 100 runs when 10k is
// too slow for local iteration. Run with -benchmem -count=3.
func BenchmarkGetSummary(b *testing.B) {
	dsn := fmt.Sprintf("file:bench_get_summary_scale_%d?mode=memory", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	if err := gormDB.AutoMigrate(
		&database.Business{}, &database.Staff{}, &database.Bill{}, &database.Payment{},
		&database.AlternativePayment{}, &database.WithdrawalHistory{}, &database.ExchangeRate{},
		&database.ManualLedgerEntry{}, &database.PayrollRun{}, &database.PayrollLineItem{},
	); err != nil {
		b.Fatal(err)
	}

	biz := &database.Business{
		BusinessId: "bench-summary-scale", Name: "BenchScale", OwnerAddress: "0xBenchScale",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD", DisplayCurrency: "USD", DefaultLanguage: "en",
		SourceLanguage: "en",
	}
	if err := gormDB.Create(biz).Error; err != nil {
		b.Fatal(err)
	}
	staff := &database.Staff{
		BusinessID: biz.ID, Email: "scale@example.com", Name: "Scale",
		Role: database.StaffRoleManager, IsActive: true, InvitedBy: "o",
	}
	if err := gormDB.Create(staff).Error; err != nil {
		b.Fatal(err)
	}

	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 60) // 60-day window
	currencies := []string{"USD", "EUR", "GBP"}
	rates := map[string]float64{"EUR": 1.10, "GBP": 1.25}
	for cur, rate := range rates {
		if err := gormDB.Create(&database.ExchangeRate{
			FromCurrency: cur, ToCurrency: "USD", Rate: rate, Source: "test",
			FetchedAt: start.Add(-24 * time.Hour),
		}).Error; err != nil {
			b.Fatal(err)
		}
		// A few mid-window revisions so historical lookup is non-trivial.
		for i := 0; i < 10; i++ {
			_ = gormDB.Create(&database.ExchangeRate{
				FromCurrency: cur, ToCurrency: "USD", Rate: rate + float64(i)*0.001, Source: "test",
				FetchedAt: start.Add(time.Duration(i*3) * 24 * time.Hour),
			}).Error
		}
	}

	const nEntries = 2000
	const nPayroll = 100
	categoriesIncome := []string{"catering", "event", "service", "other"}
	categoriesExpense := []string{"rent", "supplies", "software", "utilities"}

	entries := make([]database.ManualLedgerEntry, 0, nEntries)
	for i := 0; i < nEntries; i++ {
		cur := currencies[i%len(currencies)]
		day := start.Add(time.Duration(i%55) * 24 * time.Hour)
		entryType := database.AccountingEntryTypeIncome
		cat := categoriesIncome[i%len(categoriesIncome)]
		if i%2 == 1 {
			entryType = database.AccountingEntryTypeExpense
			cat = categoriesExpense[i%len(categoriesExpense)]
		}
		e := database.ManualLedgerEntry{
			BusinessID:       biz.ID,
			EntryType:        entryType,
			Category:         cat,
			Amount:           int64(1000 + (i%50)*10),
			Currency:         cur,
			OccurredAt:       day.Add(time.Duration(i%20) * time.Hour),
			Description:      fmt.Sprintf("bench-%d", i),
			CreatedByStaffID: &staff.ID,
		}
		// ~5% voided — must be excluded from summary.
		if i%20 == 0 {
			v := e.OccurredAt.Add(time.Hour)
			e.VoidedAt = &v
		}
		entries = append(entries, e)
	}
	if err := gormDB.CreateInBatches(entries, 200).Error; err != nil {
		b.Fatal(err)
	}

	runs := make([]database.PayrollRun, 0, nPayroll)
	for i := 0; i < nPayroll; i++ {
		cur := currencies[i%len(currencies)]
		paid := start.Add(time.Duration(i%50)*24*time.Hour + 12*time.Hour)
		status := database.PayrollRunStatusPaid
		if i%15 == 0 {
			status = database.PayrollRunStatusDraft
		}
		if i%17 == 0 {
			status = database.PayrollRunStatusVoid
		}
		run := database.PayrollRun{
			BusinessID:     biz.ID,
			PeriodStart:    start,
			PeriodEnd:      start.Add(24 * time.Hour),
			Status:         status,
			Currency:       cur,
			GrossTotal:     5000 + int64(i*10),
			BonusTotal:     100,
			DeductionTotal: 50,
			NetTotal:       5050 + int64(i*10),
		}
		if status != database.PayrollRunStatusDraft {
			run.PaidAt = &paid
		}
		runs = append(runs, run)
	}
	if err := gormDB.CreateInBatches(runs, 50).Error; err != nil {
		b.Fatal(err)
	}

	service := NewService(database.GetDBWrapper())
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := service.GetSummary(biz.ID, start, end); err != nil {
			b.Fatal(err)
		}
	}
}

// TestPayrollRunRowMarshalJSON_PayeeCountOnWire is the regression for the
// promoted-marshaler bug: PayrollRunRow embeds database.PayrollRun, whose
// custom MarshalJSON (cents→dollars) was promoted to the row and silently
// dropped payee_count from the wire (frontend rendered "0 payees" for every
// run). The row must emit payee_count, keep dollar money fields, and not leak
// the zero-value business relation blob.
func TestPayrollRunRowMarshalJSON_PayeeCountOnWire(t *testing.T) {
	row := PayrollRunRow{
		PayrollRun: database.PayrollRun{
			ID:         12,
			BusinessID: 2,
			Status:     database.PayrollRunStatusPaid,
			Currency:   "USD",
			GrossTotal: 505000,
			NetTotal:   486000,
		},
		PayeeCount: 5,
	}

	raw, err := json.Marshal(row)
	require.NoError(t, err)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &m))

	count, ok := m["payee_count"]
	require.True(t, ok, "payee_count missing from wire payload: %s", raw)
	assert.EqualValues(t, 5, count)

	// Money wire contract must survive the row wrapper (dollars, not cents).
	assert.EqualValues(t, 5050.00, m["gross_total"])
	assert.EqualValues(t, 4860.00, m["net_total"])

	// The unloaded Business relation must not serialize as a zero-value blob.
	_, hasBusiness := m["business"]
	assert.False(t, hasBusiness, "zero-value business relation leaked onto the wire")
}
