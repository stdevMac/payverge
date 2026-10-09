package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// seedConfirmedCashBill creates a paid bill with a confirmed alternative
// payment of the given method/amount at confirmedAt.
func seedConfirmedCashBill(t *testing.T, db *gorm.DB, svc *Service, businessID uint, number string, amount int64, method database.AlternativePaymentMethod, confirmedAt time.Time) {
	t.Helper()
	bill := database.Bill{BusinessID: businessID, BillNumber: number, Status: database.BillStatusPaid,
		TotalAmount: amount, PaidAmount: amount, CreatedAt: confirmedAt.Add(-time.Hour), ClosedAt: &confirmedAt}
	require.NoError(t, db.Create(&bill).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.createConfirmedAlternativePayment(context.Background(), tx, &bill, amount, method, confirmedAt)
	}))
}

// #796: the caja session's cash_sales must be DERIVED from the day's confirmed
// cash alternative payments in the books — not fabricated from a hash. The
// fabricated band (60000–120000) had zero cash payments behind it, so caja
// showed cash weeks where sales endpoints showed $0.
func TestGenerateCashSessionDerivesCashSalesFromBooks(t *testing.T) {
	db := newDemoServiceTestDB(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})

	business := database.Business{BusinessId: "cash-books", Name: "Cash Books", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	// Two cash payments on the day = the till's cash sales.
	seedConfirmedCashBill(t, db, svc, business.ID, "B-cash-1", 30000, database.PaymentMethodCash, day.Add(13*time.Hour))
	seedConfirmedCashBill(t, db, svc, business.ID, "B-cash-2", 15000, database.PaymentMethodCash, day.Add(20*time.Hour))
	// Card money and other-day cash must NOT count.
	seedConfirmedCashBill(t, db, svc, business.ID, "B-card-1", 99900, database.PaymentMethodCard, day.Add(14*time.Hour))
	seedConfirmedCashBill(t, db, svc, business.ID, "B-cash-next", 77700, database.PaymentMethodCash, day.Add(27*time.Hour))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.generateCashSessionForDay(context.Background(), tx, business.ID, day)
	}))

	var session database.CashRegisterSession
	require.NoError(t, db.Where("business_id = ? AND opened_at >= ? AND opened_at < ?",
		business.ID, day, day.Add(24*time.Hour)).First(&session).Error)
	require.Equal(t, int64(45000), session.CashSalesCents,
		"caja cash_sales must equal the day's confirmed cash payments in the books (#796)")
	expected := session.OpeningFloatCents + session.CashSalesCents - session.CashRefundsCents + session.CashInCents - session.CashOutCents
	require.Equal(t, expected, session.ExpectedCashCents)
	require.Equal(t, session.CountedCashCents-session.ExpectedCashCents, session.VarianceCents)

	// A day with no cash payments has an honest zero-cash till. (July 2 holds
	// the 27h spillover bill above, so use a clean day.)
	emptyDay := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.generateCashSessionForDay(context.Background(), tx, business.ID, emptyDay)
	}))
	var emptySession database.CashRegisterSession
	require.NoError(t, db.Where("business_id = ? AND opened_at >= ? AND opened_at < ?",
		business.ID, emptyDay, emptyDay.Add(24*time.Hour)).First(&emptySession).Error)
	require.Zero(t, emptySession.CashSalesCents, "no cash in the books = no cash sales on the till")
}

// #796: the hourly append revisits today after more bills (and their cash
// payments) have materialized. The existing session must be UPDATED to match
// the books — the old code returned early and left the till frozen at
// whatever it claimed when the day was first touched.
func TestGenerateCashSessionUpdatesOnRevisit(t *testing.T) {
	db := newDemoServiceTestDB(t)
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	now := day.Add(12 * time.Hour)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})

	business := database.Business{BusinessId: "cash-revisit", Name: "Cash Revisit", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	seedConfirmedCashBill(t, db, svc, business.ID, "B-rev-1", 30000, database.PaymentMethodCash, day.Add(11*time.Hour))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.generateCashSessionForDay(context.Background(), tx, business.ID, day)
	}))

	var first database.CashRegisterSession
	require.NoError(t, db.Where("business_id = ?", business.ID).First(&first).Error)
	require.Equal(t, int64(30000), first.CashSalesCents)

	// Afternoon: another cash bill materializes; the clock moves to 18:00.
	seedConfirmedCashBill(t, db, svc, business.ID, "B-rev-2", 20000, database.PaymentMethodCash, day.Add(15*time.Hour))
	now = day.Add(18 * time.Hour)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.generateCashSessionForDay(context.Background(), tx, business.ID, day)
	}))

	var sessions []database.CashRegisterSession
	require.NoError(t, db.Where("business_id = ?", business.ID).Find(&sessions).Error)
	require.Len(t, sessions, 1, "revisit must update, never duplicate, the day's session")
	require.Equal(t, int64(50000), sessions[0].CashSalesCents,
		"revisit must fold newly-materialized cash into the till (#796)")
	expected := sessions[0].OpeningFloatCents + sessions[0].CashSalesCents - sessions[0].CashRefundsCents + sessions[0].CashInCents - sessions[0].CashOutCents
	require.Equal(t, expected, sessions[0].ExpectedCashCents)
	require.Equal(t, sessions[0].CountedCashCents-sessions[0].ExpectedCashCents, sessions[0].VarianceCents)
	require.NotNil(t, sessions[0].ClosedAt)
	require.False(t, sessions[0].ClosedAt.After(now))
}

// Cash-register history must vary per day (real varying cash in the books)
// and always reconcile.
func TestGenerateCashSessionVariesPerDay(t *testing.T) {
	db := newDemoServiceTestDB(t)
	// A "now" comfortably after every day in the range so nothing is clamped.
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})

	business := database.Business{BusinessId: "cash-variety", Name: "Cash Variety", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	const days = 20

	for i := 0; i < days; i++ {
		day := base.AddDate(0, 0, i)
		seedConfirmedCashBill(t, db, svc, business.ID, fmt.Sprintf("B-var-%d", i),
			int64(30000+i*1317), database.PaymentMethodCash, day.Add(13*time.Hour))
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			return svc.generateCashSessionForDay(context.Background(), tx, business.ID, day)
		}))
	}

	var sessions []database.CashRegisterSession
	require.NoError(t, db.Where("business_id = ?", business.ID).Order("opened_at asc").Find(&sessions).Error)
	require.Len(t, sessions, days)

	// (a) Cash sales track the books, so different days differ.
	distinct := map[int64]struct{}{}
	for i, s := range sessions {
		require.Equal(t, int64(30000+i*1317), s.CashSalesCents, "day %d cash must equal the books", i)
		distinct[s.CashSalesCents] = struct{}{}
	}
	require.Len(t, distinct, days)

	// (b) The reconciliation invariant holds for every day, and the hash-varied
	// figures stay in band.
	nonZeroVariance := 0
	for _, s := range sessions {
		expected := s.OpeningFloatCents + s.CashSalesCents - s.CashRefundsCents + s.CashInCents - s.CashOutCents
		require.Equal(t, expected, s.ExpectedCashCents, "expected cash must reconcile")
		require.Equal(t, s.CountedCashCents-s.ExpectedCashCents, s.VarianceCents, "variance = counted - expected")

		// ARS band: AR$60.000–80.000 opening float.
		require.GreaterOrEqual(t, s.OpeningFloatCents, int64(6000000))
		require.LessOrEqual(t, s.OpeningFloatCents, int64(8001000))

		if s.VarianceCents != 0 {
			nonZeroVariance++
		}

		// Movements must match the session's cash-in / cash-out.
		var movements []database.CashRegisterMovement
		require.NoError(t, db.Where("session_id = ? AND movement_type IN ?", s.ID,
			[]database.CashRegisterMovementType{database.CashRegisterMovementTypeCashIn, database.CashRegisterMovementTypeCashOut}).
			Order("movement_type asc").Find(&movements).Error)
		require.Len(t, movements, 2)
		var seenIn, seenOut bool
		for _, m := range movements {
			switch m.MovementType {
			case database.CashRegisterMovementTypeCashIn:
				require.Equal(t, s.CashInCents, m.AmountCents, "cash-in movement must match session")
				seenIn = true
			case database.CashRegisterMovementTypeCashOut:
				require.Equal(t, s.CashOutCents, m.AmountCents, "cash-out movement must match session")
				seenOut = true
			}
		}
		require.True(t, seenIn && seenOut)

		// #857: every cash tender is backed by a cash_sale movement on the
		// drawer that took it, so cash_sales is never an unbacked aggregate.
		var cashSaleTotal int64
		require.NoError(t, db.Model(&database.CashRegisterMovement{}).
			Where("session_id = ? AND movement_type = ?", s.ID, database.CashRegisterMovementTypeCashSale).
			Select("COALESCE(SUM(amount_cents), 0)").Scan(&cashSaleTotal).Error)
		require.Equal(t, s.CashSalesCents, cashSaleTotal, "cash_sales must equal the drawer's cash_sale movements")
	}

	// (c) At least one day in the range tells a variance story.
	require.Greater(t, nonZeroVariance, 0, "some days should have non-zero variance")
}

// computeCashSessionAmounts must be deterministic: same day + same books ->
// same figures.
func TestComputeCashSessionAmountsIsDeterministic(t *testing.T) {
	day := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	a := computeCashSessionAmounts(day, 45000)
	b := computeCashSessionAmounts(day, 45000)
	require.Equal(t, a, b)
	require.Equal(t, int64(45000), a.cashSales, "cash sales come from the books, not the hash")
}

// F: seeding the marketing surfaces (offers + bundle) is idempotent.
func TestEnsureMarketingIsIdempotent(t *testing.T) {
	db := newDemoServiceTestDB(t)
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})

	business := database.Business{BusinessId: "marketing-idem", Name: "Marketing Idem", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	seed := func() {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			return svc.ensureMarketing(context.Background(), tx, business.ID)
		}))
	}

	// Two ensures must not duplicate.
	seed()
	seed()

	var offers []database.Offer
	require.NoError(t, db.Where("business_id = ?", business.ID).Order("name asc").Find(&offers).Error)
	require.Len(t, offers, 2, "double-ensure must yield exactly 2 offers")
	// Binary name order: "AR$ 3.000…" sorts before "Almuerzo…".
	require.Equal(t, "AR$ 3.000 menos en el bife", offers[0].Name)
	require.Equal(t, "fixed", offers[0].DiscountType)
	require.Equal(t, 3000.0, offers[0].DiscountValue)
	require.True(t, offers[0].IsActive)
	require.Equal(t, "Almuerzo de semana 15% off", offers[1].Name)
	require.Equal(t, "percentage", offers[1].DiscountType)
	require.Equal(t, int16(62), offers[1].WeekdayMask, "weekday demo offer must be Monday through Friday")
	require.NotNil(t, offers[1].StartMinute)
	require.NotNil(t, offers[1].EndMinute)
	require.Equal(t, 12*60, *offers[1].StartMinute)
	require.Equal(t, 16*60, *offers[1].EndMinute)
	require.True(t, database.OfferActiveAt(offers[1], time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)))
	require.False(t, database.OfferActiveAt(offers[1], time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)))
	// Dinner on a weekday must NOT get the lunch daypart discount.
	require.False(t, database.OfferActiveAt(offers[1], time.Date(2026, 7, 20, 17, 28, 0, 0, time.UTC)))

	// Existing demo rows created before recurring schedules shipped were
	// backfilled to the all-days default. A normal idempotent ensure must repair
	// that managed demo row without requiring a destructive full reseed.
	require.NoError(t, db.Model(&database.Offer{}).
		Where("business_id = ? AND name = ?", business.ID, "Almuerzo de semana 15% off").
		Updates(map[string]interface{}{
			"weekday_mask": 127,
			"start_minute": nil,
			"end_minute":   nil,
		}).Error)
	seed()
	require.NoError(t, db.Where("business_id = ? AND name = ?", business.ID, "Almuerzo de semana 15% off").First(&offers[1]).Error)
	require.Equal(t, int16(62), offers[1].WeekdayMask, "idempotent ensure must repair the legacy all-days mask")
	require.NotNil(t, offers[1].StartMinute, "idempotent ensure must repair missing lunch start_minute")
	require.NotNil(t, offers[1].EndMinute, "idempotent ensure must repair missing lunch end_minute")
	require.Equal(t, 12*60, *offers[1].StartMinute)
	require.Equal(t, 16*60, *offers[1].EndMinute)

	var bundles []database.Bundle
	require.NoError(t, db.Where("business_id = ?", business.ID).Find(&bundles).Error)
	require.Len(t, bundles, 1, "double-ensure must yield exactly 1 bundle")
	require.Equal(t, "Noche de parrilla para dos", bundles[0].Name)
	require.Equal(t, 105000.00, bundles[0].Price)
	require.Equal(t, "ARS", bundles[0].Currency)

	// Items must be valid JSON referencing demo menu items.
	var refs []database.BundleItemRef
	require.NoError(t, json.Unmarshal([]byte(bundles[0].Items), &refs))
	require.NotEmpty(t, refs)
	require.Equal(t, "demo-parrillada", refs[0].MenuItemID)
}

// FirstOrCreate + Attrs cannot repair a stale Image on an existing bundle
// row. A healthy ensure must Assign the seed-owned photo so Marketing preview
// and photo_ready agree after reload.
func TestEnsureMarketingRepairsStaleDateNightImage(t *testing.T) {
	db := newDemoServiceTestDB(t)
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})

	business := database.Business{BusinessId: "marketing-repair", Name: "Marketing Repair", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	seed := func() {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			return svc.ensureMarketing(context.Background(), tx, business.ID)
		}))
	}

	seed()
	require.NoError(t, db.Model(&database.Bundle{}).
		Where("business_id = ? AND name = ?", business.ID, "Noche de parrilla para dos").
		Updates(map[string]interface{}{"image": "", "description": "stale"}).Error)

	seed()

	var bundle database.Bundle
	require.NoError(t, db.Where("business_id = ? AND name = ?", business.ID, "Noche de parrilla para dos").First(&bundle).Error)
	require.Equal(t, svc.hostedAsset(menuImage("demo-parrillada")), bundle.Image)
	require.Contains(t, bundle.Description, "Parrillada")
	require.NotEmpty(t, bundle.Image)
}
