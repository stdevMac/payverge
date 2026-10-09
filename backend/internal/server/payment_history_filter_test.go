package server

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/database/dbtest"
)

// TestLoadPaymentHistoryItemsFilteredPaginates asserts the paginated path bounds
// the returned rows at page_size and reports the true total for the window.
func TestLoadPaymentHistoryItemsFilteredPaginates(t *testing.T) {
	business, start, end := setupPaymentHistoryPerfDB(t, nil)

	// Page 1 of 20 out of 500.
	page1, total, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(500), total, "total must reflect the whole window, not the page")
	assert.Len(t, page1, 20, "page must be bounded at page_size")

	// Page 2 returns a disjoint set (newest-first ordering is stable).
	page2, _, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Page: 2, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, page2, 20)
	assert.NotEqual(t, page1[0].ID, page2[0].ID, "page 2 must not repeat page 1's first row")
}

// TestLoadPaymentHistoryItemsFilteredClampsPageSize asserts page_size can't
// exceed the server cap even if a caller passes a huge value.
func TestLoadPaymentHistoryItemsFilteredClampsPageSize(t *testing.T) {
	business, start, end := setupPaymentHistoryPerfDB(t, nil)

	items, total, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Page: 1, PageSize: 100000})
	require.NoError(t, err)
	assert.Equal(t, int64(500), total)
	assert.LessOrEqual(t, len(items), paymentHistoryMaxPageSize, "page_size must be clamped to the server cap")
	assert.Len(t, items, paymentHistoryMaxPageSize)
}

// TestLoadPaymentHistoryItemsFilteredSearch asserts server-side search narrows
// the result set so the client no longer needs the full window.
func TestLoadPaymentHistoryItemsFilteredSearch(t *testing.T) {
	business, start, end := setupPaymentHistoryPerfDB(t, nil)

	// Bill numbers are PAY-HIST-BILL-000..499. "BILL-001" matches exactly one.
	items, total, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Search: "BILL-001"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, "PAY-HIST-BILL-001", items[0].BillNumber)
}

// TestLoadPaymentHistoryItemsFilteredMethod asserts the method filter reaches SQL
// via the canonical vocabulary (Task 7). method=crypto expands to raw aliases;
// non-canonical keys like "stripe" match zero rows (FE must send "card").
func TestLoadPaymentHistoryItemsFilteredMethod(t *testing.T) {
	business, start, end := setupPaymentHistoryPerfDB(t, nil)

	// All seeded payments use payment_method "crypto".
	crypto, _, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Method: "crypto"})
	require.NoError(t, err)
	assert.Len(t, crypto, 500)
	for _, item := range crypto {
		assert.Equal(t, "crypto", item.Method, "row Method must be canonical")
	}

	// Legacy FE key "stripe" is rejected — canonical key is "card".
	none, _, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Method: "stripe"})
	require.NoError(t, err)
	assert.Len(t, none, 0, "non-canonical method filter must match zero rows")

	// card has no rows in the crypto-only seed.
	card, _, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Method: "card"})
	require.NoError(t, err)
	assert.Len(t, card, 0)
}

// TestLoadPaymentHistoryAvailableMethodsAndCanonicalFilter is the Task 7 gate:
// available_methods is generated from rows present for the business+window, and
// filtering by the canonical key "card" matches both raw "card" and "stripe"
// settlements (plugin settlement type → card).
func TestLoadPaymentHistoryAvailableMethodsAndCanonicalFilter(t *testing.T) {
	gormDB := setupStaffHandlerTestDBForTB(t)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
	))
	require.NoError(t, dbtest.EnsurePaymentEventsView(gormDB))

	business := &database.Business{
		BusinessId:     "payment-method-vocab",
		Name:           "Method Vocab",
		OwnerAddress:   "0xMethodVocabOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gormDB.Create(business).Error)

	start := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	end := start.Add(4 * time.Hour)
	mid := start.Add(time.Hour)

	table := &database.Table{BusinessID: business.ID, TableCode: "MV-1", Name: "Bar", IsActive: true}
	require.NoError(t, gormDB.Create(table).Error)

	// crypto payment
	billCrypto := &database.Bill{
		BusinessID: business.ID, TableID: table.ID, BillNumber: "MV-CRYPTO",
		TotalAmount: 1000, Status: database.BillStatusPaid,
		SettlementAddr: business.SettlementAddr, TippingAddr: business.TippingAddr,
		CreatedAt: mid, UpdatedAt: mid,
	}
	require.NoError(t, gormDB.Create(billCrypto).Error)
	confirmed := mid
	require.NoError(t, gormDB.Create(&database.Payment{
		BillID: billCrypto.ID, PayerAddr: "0xcrypto", Amount: 1000, TipAmount: 0,
		TxHash: "0xmv_crypto", Status: database.PaymentStatusConfirmed,
		PaymentMethod: "crypto", ConfirmedAt: &confirmed, CreatedAt: mid, UpdatedAt: mid,
	}).Error)

	// stripe plugin settlement on payments table (raw method = "stripe")
	billStripe := &database.Bill{
		BusinessID: business.ID, TableID: table.ID, BillNumber: "MV-STRIPE",
		TotalAmount: 2000, Status: database.BillStatusPaid,
		SettlementAddr: business.SettlementAddr, TippingAddr: business.TippingAddr,
		CreatedAt: mid, UpdatedAt: mid,
	}
	require.NoError(t, gormDB.Create(billStripe).Error)
	require.NoError(t, gormDB.Create(&database.Payment{
		BillID: billStripe.ID, PayerAddr: "plugin", Amount: 2000, TipAmount: 0,
		TxHash: "plugin_mv_1", Status: database.PaymentStatusConfirmed,
		PaymentMethod: "stripe", ConfirmedAt: &confirmed, CreatedAt: mid, UpdatedAt: mid,
	}).Error)

	// cash alternative payment
	billCash := &database.Bill{
		BusinessID: business.ID, TableID: table.ID, BillNumber: "MV-CASH",
		TotalAmount: 500, Status: database.BillStatusPaid,
		SettlementAddr: business.SettlementAddr, TippingAddr: business.TippingAddr,
		CreatedAt: mid, UpdatedAt: mid,
	}
	require.NoError(t, gormDB.Create(billCash).Error)
	require.NoError(t, gormDB.Create(&database.AlternativePayment{
		BillID: billCash.ID, ParticipantAddr: "walk-in", Amount: 500,
		BillAmountCents: 500, PaymentMethod: database.PaymentMethodCash,
		Status: database.AltPaymentStatusConfirmed, ConfirmedAt: &confirmed,
		CreatedAt: mid, UpdatedAt: mid,
	}).Error)

	available, err := loadPaymentHistoryAvailableMethods(business.ID, start, end)
	require.NoError(t, err)
	// Only methods with rows: crypto, card (from stripe), cash. No wallet/cross_chain/other.
	assert.Equal(t, []string{"crypto", "card", "cash"}, available,
		"available_methods must be the canonical Methods present, ordered, no zero-row options")

	// Filtering by canonical "card" must hit the stripe row.
	cardItems, cardTotal, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Method: "card"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), cardTotal)
	require.Len(t, cardItems, 1)
	assert.Equal(t, "card", cardItems[0].Method)
	assert.Equal(t, "plugin_mv_1", cardItems[0].TxHash)

	// Filtering by "cash" hits the alt payment.
	cashItems, cashTotal, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Method: "cash"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), cashTotal)
	require.Len(t, cashItems, 1)
	assert.Equal(t, "cash", cashItems[0].Method)

	// wallet has zero rows for this business → filter returns empty (and is not in available).
	walletItems, walletTotal, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Method: "wallet"})
	require.NoError(t, err)
	assert.Equal(t, int64(0), walletTotal)
	assert.Len(t, walletItems, 0)
	assert.NotContains(t, available, "wallet")
}

// TestLoadPaymentHistoryItemsFilteredSearchEscapesWildcards asserts the LIKE
// wildcards are escaped so a "%" in the search string matches literally rather
// than acting as a SQL wildcard.
func TestLoadPaymentHistoryItemsFilteredSearchEscapesWildcards(t *testing.T) {
	business, start, end := setupPaymentHistoryPerfDB(t, nil)

	items, total, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Search: "%"})
	require.NoError(t, err)
	// No bill number literally contains "%", so an escaped LIKE returns nothing.
	assert.Equal(t, int64(0), total)
	assert.Len(t, items, 0)
}

// TestLoadPaymentHistoryItemsFilteredAccessShape asserts the paginated path keeps
// the projected-join shape (no SELECT *, no per-row bill/table query).
func TestLoadPaymentHistoryItemsFilteredAccessShape(t *testing.T) {
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	business, start, end := setupPaymentHistoryPerfDB(t, recorder)

	_, _, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Zero(t, recorder.selectStarCount("bills"), "paginated payment history must not hydrate full bill rows")
	assert.Zero(t, recorder.selectStarCount("tables"), "paginated payment history must not hydrate full table rows")
}

// TestLoadPaymentHistoryUnionsBothPaymentTables is the Task 6 access-shape guard:
// Payment History must surface crypto rows from `payments` AND card/cash rows
// from `alternative_payments` so the ledger matches analytics revenue.
// Seeds 5 crypto + 3 card in one window; expects 8 rows, amount sum ==
// recognized revenue, and no SELECT * / unused Preload on the path.
func TestLoadPaymentHistoryUnionsBothPaymentTables(t *testing.T) {
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	gormDB := setupStaffHandlerTestDBForTB(t)
	gormDB = gormDB.Session(&gorm.Session{Logger: recorder})
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
	))
	require.NoError(t, dbtest.EnsurePaymentEventsView(gormDB))

	business := &database.Business{
		BusinessId:     "payment-events-union",
		Name:           "Payment Events Union",
		OwnerAddress:   "0xPaymentEventsOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gormDB.Create(business).Error)

	start := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	end := start.Add(4 * time.Hour)
	mid := start.Add(time.Hour)

	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  "PE-1",
		Name:       "Patio Events",
		IsActive:   true,
	}
	require.NoError(t, gormDB.Create(table).Error)

	// 5 crypto payments (payments table).
	var cryptoCents int64
	for i := 0; i < 5; i++ {
		amount := 1000 + int64(i*100)
		bill := &database.Bill{
			BusinessID:     business.ID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("CRYPTO-%d", i),
			TotalAmount:    amount,
			TipAmount:      100,
			Status:         database.BillStatusPaid,
			SettlementAddr: "0x1111111111111111111111111111111111111111",
			TippingAddr:    "0x2222222222222222222222222222222222222222",
			CreatedAt:      mid,
			UpdatedAt:      mid,
		}
		require.NoError(t, gormDB.Create(bill).Error)
		cryptoCents += amount
		confirmed := mid
		require.NoError(t, gormDB.Create(&database.Payment{
			BillID:        bill.ID,
			PayerAddr:     fmt.Sprintf("0xcrypto%d", i),
			Amount:        amount,
			TipAmount:     100,
			TxHash:        fmt.Sprintf("0xcrypto_tx_%d", i),
			Status:        database.PaymentStatusConfirmed,
			PaymentMethod: "crypto",
			ConfirmedAt:   &confirmed,
			CreatedAt:     mid,
			UpdatedAt:     mid,
		}).Error)
	}

	// 3 card alternative_payments (the gap Payment History currently hides).
	var cardCents int64
	for i := 0; i < 3; i++ {
		amount := 2000 + int64(i*50)
		bill := &database.Bill{
			BusinessID:     business.ID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("CARD-%d", i),
			TotalAmount:    amount,
			TipAmount:      200,
			Status:         database.BillStatusPaid,
			SettlementAddr: "0x1111111111111111111111111111111111111111",
			TippingAddr:    "0x2222222222222222222222222222222222222222",
			CreatedAt:      mid,
			UpdatedAt:      mid,
		}
		require.NoError(t, gormDB.Create(bill).Error)
		cardCents += amount
		confirmed := mid
		require.NoError(t, gormDB.Create(&database.AlternativePayment{
			BillID:          bill.ID,
			ParticipantAddr: fmt.Sprintf("card-guest-%d", i),
			ParticipantName: "Card Guest",
			Amount:          amount,
			BillAmountCents: amount,
			TipAmountCents:  200,
			PaymentMethod:   database.PaymentMethodCard,
			Status:          database.AltPaymentStatusConfirmed,
			IdempotencyKey:  fmt.Sprintf("auth_code_CARD%d", i),
			ConfirmedAt:     &confirmed,
			CreatedAt:       mid,
			UpdatedAt:       mid,
		}).Error)
	}

	items, total, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{})
	require.NoError(t, err)
	require.Equal(t, int64(8), total, "payment history must union payments + alternative_payments")
	require.Len(t, items, 8, "expected 5 crypto + 3 card rows")

	var historyCents int64
	for _, item := range items {
		historyCents += int64(item.Amount*100 + 0.5)
	}
	expectedCents := cryptoCents + cardCents
	assert.Equal(t, expectedCents, historyCents, "history amount sum must match seeded totals")

	// Analytics revenue for the same window must agree (both tables recognized).
	summary, err := database.GetRecognizedPaymentSummary(business.ID, start, end)
	require.NoError(t, err)
	assert.Equal(t, expectedCents, summary.TotalRevenueCents,
		"payment history sum must equal analytics recognized revenue for the window")

	// Access shape: projected selects only — no SELECT * on bills/tables, no Preload fan-out.
	assert.Zero(t, recorder.selectStarCount("bills"), "must not SELECT * from bills")
	assert.Zero(t, recorder.selectStarCount("tables"), "must not SELECT * from tables")
	assert.Zero(t, recorder.selectStarCount("payments"), "must not SELECT * from payments")
	assert.Zero(t, recorder.selectStarCount("alternative_payments"), "must not SELECT * from alternative_payments")
	for _, stmt := range recorder.statements {
		lower := strings.ToLower(stmt)
		assert.NotContains(t, lower, "preload", "payment history path must not use Preload")
	}

	// Card auth codes on alternative_payments must be searchable via external_ref.
	found, foundTotal, err := loadPaymentHistoryItemsFiltered(business.ID, start, end, PaymentHistoryFilter{Search: "auth_code_CARD1"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), foundTotal)
	require.Len(t, found, 1)
	assert.Equal(t, "auth_code_CARD1", found[0].TxHash)
}

// TestResolvePaymentHistoryRangeClampsTo366Days asserts an explicit range wider
// than a year is capped (mirrors the analytics timeseries clamp).
func TestResolvePaymentHistoryRangeClampsTo366Days(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	start, end, err := resolvePaymentHistoryRange("", "2020-01-01", "2026-07-21", now, time.UTC)
	require.NoError(t, err)

	span := end.Sub(start)
	assert.LessOrEqual(t, span, time.Duration(paymentHistoryMaxRangeDays+1)*24*time.Hour,
		"a multi-year explicit range must be clamped to ~366 days")
}

// L6-2: period=yesterday must resolve to the previous local calendar day
// (start inclusive, end exclusive at today's midnight) — not error or
// silently fall through to today/month.
func TestResolvePaymentHistoryRangeYesterday(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 8, 6, 15, 30, 0, 0, loc)
	start, end, err := resolvePaymentHistoryRange("yesterday", "", "", now, loc)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 8, 5, 0, 0, 0, 0, loc), start)
	assert.Equal(t, time.Date(2026, 8, 6, 0, 0, 0, 0, loc), end)
	assert.True(t, end.After(start))
	// A row at yesterday 12:00 is inside; a row at today 00:00 is not.
	midYesterday := time.Date(2026, 8, 5, 12, 0, 0, 0, loc)
	assert.False(t, midYesterday.Before(start))
	assert.True(t, midYesterday.Before(end))
	assert.False(t, end.Before(end)) // end exclusive boundary is end itself
}

// TestResolvePaymentHistoryRangeInterpretsDatesInBusinessTimezone asserts the
// window boundary is the business-local midnight, not the server clock. A row at
// business-local 00:30 must fall inside a range whose end is that same day.
func TestResolvePaymentHistoryRangeInterpretsDatesInBusinessTimezone(t *testing.T) {
	// America/New_York is UTC-4/-5. Local midnight is 04:00/05:00 UTC.
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	start, end, err := resolvePaymentHistoryRange("", "2026-07-20", "2026-07-20", now, loc)
	require.NoError(t, err)

	// A payment at 2026-07-20 00:30 New-York-local (= 04:30 UTC) must be in range.
	localMidnightPlus := time.Date(2026, 7, 20, 0, 30, 0, 0, loc)
	assert.False(t, localMidnightPlus.Before(start), "start must be at business-local midnight")
	assert.True(t, localMidnightPlus.Before(end), "a same-day business-local row must fall inside the window")

	// Verify the start really is anchored at NY-local midnight (04:00 or 05:00 UTC).
	assert.Equal(t, 0, start.In(loc).Hour(), "range start must be business-local midnight")
}

var _ = database.ResolveBusinessLocation
