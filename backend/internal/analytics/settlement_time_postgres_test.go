package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf"

	"github.com/stretchr/testify/require"
)

func TestSettlementTimeReportsReconcileAcrossDailyWeeklyMonthlyAndCustom_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short")
	}
	ctx := context.Background()
	pg, err := testperf.StartIsolatedPostgres(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	gdb := pg.DB
	require.NoError(t, gdb.Exec(`
		CREATE TABLE bills (
			id BIGSERIAL PRIMARY KEY,
			business_id BIGINT NOT NULL,
			bill_number TEXT NOT NULL,
			total_amount BIGINT NOT NULL DEFAULT 0,
			paid_amount BIGINT NOT NULL DEFAULT 0,
			tip_amount BIGINT NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			created_by_staff_id BIGINT,
			closed_by_staff_id BIGINT,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			closed_at TIMESTAMPTZ,
			settled_at TIMESTAMPTZ
		);
		CREATE TABLE payments (
			id BIGSERIAL PRIMARY KEY,
			bill_id BIGINT NOT NULL,
			payer_addr TEXT NOT NULL DEFAULT '',
			amount BIGINT NOT NULL,
			tip_amount BIGINT NOT NULL DEFAULT 0,
			tx_hash TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			payment_method TEXT NOT NULL DEFAULT '',
			confirmed_at TIMESTAMPTZ,
			reversed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);
		CREATE TABLE alternative_payments (
			id BIGSERIAL PRIMARY KEY,
			bill_id BIGINT NOT NULL,
			participant_addr TEXT NOT NULL DEFAULT '',
			amount BIGINT NOT NULL,
			tip_amount_cents BIGINT NOT NULL DEFAULT 0,
			payment_method TEXT NOT NULL,
			status TEXT NOT NULL,
			confirmed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);
		CREATE TABLE staff (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT ''
		);
	`).Error)

	previousDB := database.GetDBWrapper().GetGorm()
	database.SetTestDB(gdb)
	t.Cleanup(func() { database.SetTestDB(previousDB) })

	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	openedBeforeMonth := time.Date(2026, 10, 31, 23, 55, 0, 0, ny)
	cardConfirmed := time.Date(2026, 11, 1, 0, 15, 0, 0, ny)
	// The fall-back hour occurs twice. Use both instants to prove timestamptz
	// windows include them exactly once in the same local reporting day.
	firstOneThirty := time.Date(2026, 11, 1, 1, 30, 0, 0, time.FixedZone("EDT", -4*60*60))
	secondOneThirty := time.Date(2026, 11, 1, 1, 30, 0, 0, time.FixedZone("EST", -5*60*60))
	reversedAt := time.Date(2026, 11, 2, 10, 0, 0, 0, ny)

	require.NoError(t, gdb.Exec(`INSERT INTO staff (id, name) VALUES (11, 'Alex')`).Error)
	require.NoError(t, gdb.Exec(`
		INSERT INTO bills
			(id, business_id, bill_number, total_amount, paid_amount, tip_amount, status,
			 created_by_staff_id, closed_by_staff_id, created_at, updated_at, closed_at, settled_at)
		VALUES
			(1, 77, 'DST-CARD', 5000, 5000, 500, 'paid', 11, 11, ?, ?, ?, ?),
			(2, 77, 'DST-CASH', 3000, 3000, 300, 'paid', 11, 11, ?, ?, ?, ?),
			(3, 77, 'DST-LEGACY', 2000, 2000, 200, 'paid', 11, 11, ?, ?, ?, ?)
	`, openedBeforeMonth, cardConfirmed, cardConfirmed, cardConfirmed,
		openedBeforeMonth.AddDate(0, -1, 0), firstOneThirty, firstOneThirty, firstOneThirty,
		openedBeforeMonth.AddDate(0, -1, 0), secondOneThirty, secondOneThirty, secondOneThirty).Error)
	require.NoError(t, gdb.Exec(`
		INSERT INTO payments
			(bill_id, payer_addr, amount, tip_amount, tx_hash, status, payment_method,
			 confirmed_at, reversed_at, created_at, updated_at)
		VALUES (1, 'guest-card', 5000, 500, 'dst-card', 'reversed', 'card', ?, ?, ?, ?)
	`, cardConfirmed, reversedAt, cardConfirmed, reversedAt).Error)
	require.NoError(t, gdb.Exec(`
		INSERT INTO alternative_payments
			(bill_id, participant_addr, amount, tip_amount_cents, payment_method, status,
			 confirmed_at, created_at, updated_at)
		VALUES (2, 'cashier', 3000, 300, 'cash', 'confirmed', ?, ?, ?)
	`, firstOneThirty, firstOneThirty, firstOneThirty).Error)

	// Tuesday after the DST Sunday; calendar week is Mon 2026-11-02 → now,
	// so week no longer includes Nov 1 earn events (L6-1 calendar periods).
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, ny)
	svc := NewAnalyticsService(database.GetDBWrapper()).WithClock(func() time.Time { return now })
	customStart := time.Date(2026, 11, 1, 0, 0, 0, 0, ny)
	customEnd := customStart.AddDate(0, 0, 1)

	daily, err := svc.GetDailySales(77, customStart)
	require.NoError(t, err)
	custom, err := svc.GetPaymentWindowSummary(77, customStart, customEnd)
	require.NoError(t, err)
	require.InDelta(t, 100.0, daily.TotalRevenue, 0.001)
	require.InDelta(t, 10.0, daily.TotalTips, 0.001)
	require.InDelta(t, daily.TotalRevenue, custom.TotalRevenue, 0.001)
	require.InDelta(t, daily.TotalTips, custom.TotalTips, 0.001)
	require.Equal(t, daily.TransactionCount, custom.TransactionCount)
	require.Equal(t, daily.BillCount, custom.BillCount)

	// Calendar month (Nov 1 00:00 local → now) still spans earn + reverse.
	month, err := svc.GetPeriodReport(77, "month", ny)
	require.NoError(t, err)
	require.InDelta(t, 50.0, month.TotalRevenue, 0.001, "month")
	require.InDelta(t, 5.0, month.TotalTips, 0.001, "month")
	require.Equal(t, 4, month.TransactionCount, "month")
	require.Equal(t, 2, month.BillCount, "month")

	// Calendar week starts Monday 2026-11-02 — only the Nov 2 reverse lands in it.
	week, err := svc.GetPeriodReport(77, "week", ny)
	require.NoError(t, err)
	require.InDelta(t, -50.0, week.TotalRevenue, 0.001, "week")
	require.InDelta(t, -5.0, week.TotalTips, 0.001, "week")
	require.Equal(t, 1, week.TransactionCount, "week")

	tips, err := svc.GetTipAnalytics(77, "month", ny)
	require.NoError(t, err)
	require.InDelta(t, 5.0, tips.TotalTips, 0.001)
	staffTips, err := svc.GetTipsByStaff(77, "month", ny)
	require.NoError(t, err)
	require.Len(t, staffTips, 1)
	require.InDelta(t, 5.0, staffTips[0].TotalTips, 0.001)
	require.EqualValues(t, 2, staffTips[0].BillCount)
}
