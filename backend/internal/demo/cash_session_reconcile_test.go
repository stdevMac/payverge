package demo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// #796: the hourly append pointer only walks forward, so every caja session
// written before cash_sales was derived from the books kept its fabricated
// figure forever. Live demo venue 86 painted ~$4,907 of cash across an ISO
// week whose bills and payments summed to $0. ReconcileCashSessions re-derives
// the closed history and must leave today's live drawer alone.
func TestReconcileCashSessionsRewritesUnbackedCashAndSparesToday(t *testing.T) {
	db := newDemoServiceTestDB(t)
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	ctx := context.Background()

	business := database.Business{BusinessId: "caja-reconcile", Name: "Caja Reconcile", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	today := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	fabricated := today.AddDate(0, 0, -3) // cash on the books: none
	backed := today.AddDate(0, 0, -2)     // cash on the books: $123.45

	// A confirmed CASH tender on the `backed` day — the only thing that may
	// ever justify a cash_sales figure.
	bill := database.Bill{BusinessID: business.ID, BillNumber: "B-caja-backed", Status: database.BillStatusPaid, TotalAmount: 12345, PaidAmount: 12345, CreatedAt: backed.Add(13 * time.Hour)}
	require.NoError(t, db.Create(&bill).Error)
	confirmedAt := backed.Add(14 * time.Hour)
	require.NoError(t, db.Create(&database.AlternativePayment{
		BillID:        bill.ID,
		Amount:        12345,
		PaymentMethod: database.PaymentMethodCash,
		Status:        database.AltPaymentStatusConfirmed,
		ConfirmedAt:   &confirmedAt,
	}).Error)

	newSession := func(day time.Time, status database.CashRegisterSessionStatus, closed *time.Time) database.CashRegisterSession {
		return database.CashRegisterSession{
			BusinessID:        business.ID,
			Status:            status,
			OpeningFloatCents: 21800,
			OpenedAt:          day.Add(11 * time.Hour),
			ClosedAt:          closed,
			// The hash-fabricated band the old generator invented.
			CashSalesCents:    90000,
			ExpectedCashCents: 111800,
			CountedCashCents:  111800,
		}
	}
	fabClose := fabricated.Add(23 * time.Hour)
	backClose := backed.Add(23 * time.Hour)
	fabSession := newSession(fabricated, database.CashRegisterSessionStatusClosed, &fabClose)
	backSession := newSession(backed, database.CashRegisterSessionStatusClosed, &backClose)
	openSession := newSession(today, database.CashRegisterSessionStatusOpen, nil)
	require.NoError(t, db.Create(&fabSession).Error)
	require.NoError(t, db.Create(&backSession).Error)
	require.NoError(t, db.Create(&openSession).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.ReconcileCashSessions(ctx, tx, business.ID, today.AddDate(0, 0, -30), today)
	}))

	reload := func(id uint) database.CashRegisterSession {
		var out database.CashRegisterSession
		require.NoError(t, db.First(&out, id).Error)
		return out
	}

	got := reload(fabSession.ID)
	require.Equal(t, int64(0), got.CashSalesCents,
		"a day with no confirmed cash payments must not report cash sales")
	require.Equal(t,
		got.OpeningFloatCents+got.CashSalesCents-got.CashRefundsCents+got.CashInCents-got.CashOutCents,
		got.ExpectedCashCents, "expected cash must be recomputed from the new sales figure")
	require.Equal(t, got.CountedCashCents-got.ExpectedCashCents, got.VarianceCents)

	got = reload(backSession.ID)
	require.Equal(t, int64(12345), got.CashSalesCents,
		"cash sales must equal the confirmed cash tenders on that day")

	// The floor's live drawer is outside the window and must be untouched.
	got = reload(openSession.ID)
	require.Equal(t, database.CashRegisterSessionStatusOpen, got.Status)
	require.Nil(t, got.ClosedAt, "reconcile must never close an open session")
	require.Equal(t, int64(90000), got.CashSalesCents,
		"today's open session is outside [start, end) and must not be rewritten")
}
