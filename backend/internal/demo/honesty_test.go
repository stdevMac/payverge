package demo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Task 17 — the demo dataset stops lying.
// These tests lock the honesty contract: heartbeat, verifier recency, distinct
// PINs, cash-session realism, payer/tx variety, bill numbers, payroll ratio,
// and flagship demo venue hero KPI coverage.

func TestDemoHeartbeatReportsStaleWhenAppendIsDead(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "heartbeat-admin@example.com")

	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "honesty-seed", BaselineDays: 7})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	// Fresh right after ensure/seed.
	summary, err := svc.SummaryForAdmin(context.Background(), admin.ID, false)
	require.NoError(t, err)
	require.NotNil(t, summary.Heartbeat)
	require.Equal(t, HeartbeatFresh, summary.Heartbeat.Status, "just-seeded demo must report fresh heartbeat")

	// Jump clock past 2× the hourly append interval without any successful append.
	now = now.Add(2*DefaultAppendInterval + time.Minute)
	summary, err = svc.SummaryForAdmin(context.Background(), admin.ID, false)
	require.NoError(t, err)
	require.NotNil(t, summary.Heartbeat)
	require.Equal(t, HeartbeatStale, summary.Heartbeat.Status,
		"Demo Center must report stale when last successful append is older than 2× append interval")
	require.NotNil(t, summary.Heartbeat.LastAppendAt)
}

func TestVerifierFailsWhenPaymentsPredateClaimedWindowEnd(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "recency-admin@example.com")

	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "honesty-seed", BaselineDays: 7})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	require.NotNil(t, instance.LastSimulatedBusinessDate)

	// Backdate every payment / bill close to 22 days before the claimed window end
	// while leaving LastSimulatedBusinessDate claiming the recent end — the live
	// audit failure mode ("Verification passed, 0 errors" over data that stopped weeks earlier).
	claimedEnd := *instance.LastSimulatedBusinessDate
	staleAt := claimedEnd.AddDate(0, 0, -22)
	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)
	require.NoError(t, db.Model(&database.Payment{}).
		Where("bill_id IN (?)", db.Model(&database.Bill{}).Select("id").Where("business_id IN ?", businessIDs)).
		Updates(map[string]interface{}{"confirmed_at": staleAt, "created_at": staleAt, "updated_at": staleAt}).Error)
	require.NoError(t, db.Model(&database.Bill{}).
		Where("business_id IN ?", businessIDs).
		Updates(map[string]interface{}{"closed_at": staleAt, "created_at": staleAt, "updated_at": staleAt}).Error)

	result, err := svc.VerifyInstance(context.Background(), instance.ID)
	require.NoError(t, err)
	require.Equal(t, VerificationFailed, result.Status, "verifier must fail when newest payment predates claimed window end")
	require.NotEmpty(t, result.Errors)
	joined := strings.Join(result.Errors, " | ")
	require.True(t,
		strings.Contains(strings.ToLower(joined), "recency") || strings.Contains(strings.ToLower(joined), "stale") || strings.Contains(strings.ToLower(joined), "window"),
		"error should name the recency/window failure, got: %s", joined)
}

func TestVerifierTreatsInProgressTodayAsNotYetClaimed(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "recency-today-admin@example.com")

	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "honesty-seed", BaselineDays: 7})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	loc, err := time.LoadLocation(instance.Timezone)
	require.NoError(t, err)
	today := normalizeBusinessDate(now, loc)
	instance.LastSimulatedBusinessDate = &today
	require.NoError(t, db.Omit("Businesses").Save(instance).Error)

	yesterday := today.AddDate(0, 0, -1)
	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)
	require.NoError(t, db.Model(&database.Payment{}).
		Where("bill_id IN (?)", db.Model(&database.Bill{}).Select("id").Where("business_id IN ?", businessIDs)).
		Updates(map[string]interface{}{"confirmed_at": yesterday, "created_at": yesterday, "updated_at": yesterday}).Error)
	require.NoError(t, db.Model(&database.Bill{}).
		Where("business_id IN ?", businessIDs).
		Updates(map[string]interface{}{"closed_at": yesterday, "created_at": yesterday, "updated_at": yesterday}).Error)

	result, err := svc.VerifyInstance(context.Background(), instance.ID)
	require.NoError(t, err)
	for _, errText := range result.Errors {
		require.NotContains(t, strings.ToLower(errText), "recency", errText)
	}
}

func TestSeededStaffPINsArePairwiseDistinct(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "pins-admin@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "honesty-seed", BaselineDays: 5})
	summary, err := svc.SummaryForAdmin(context.Background(), admin.ID, true)
	require.NoError(t, err)
	require.NotEmpty(t, summary.Access)

	// Per-business PIN sets must be pairwise distinct.
	byBiz := map[uint][]string{}
	for _, identity := range summary.Access {
		require.NotEmpty(t, identity.PINHint, "PIN must be surfaced once in Demo Center")
		require.Len(t, identity.PINHint, 4)
		byBiz[identity.BusinessID] = append(byBiz[identity.BusinessID], identity.PINHint)
	}
	for bizID, pins := range byBiz {
		seen := map[string]bool{}
		for _, pin := range pins {
			require.Falsef(t, seen[pin], "business %d has duplicate PIN %s", bizID, pin)
			seen[pin] = true
		}
		require.GreaterOrEqual(t, len(pins), 4, "business %d should seed multiple staff", bizID)
	}

	// bcrypt hashes on staff rows must match the surfaced PIN (not a shared default).
	var staff []database.Staff
	require.NoError(t, db.Where("business_id IN ?", keysUint(byBiz)).Find(&staff).Error)
	pinByEmail := map[string]string{}
	for _, identity := range summary.Access {
		pinByEmail[identity.Email] = identity.PINHint
	}
	for _, st := range staff {
		pin, ok := pinByEmail[st.Email]
		require.True(t, ok, "access list missing staff %s", st.Email)
		require.NoError(t, bcrypt.CompareHashAndPassword([]byte(st.PinHash), []byte(pin)),
			"pin_hash for %s must match surfaced PIN", st.Email)
	}
}

func TestCashSessionsHaveRealOpenCloseAndMixedIDs(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "cash-admin@example.com")

	// Mid-service clock so today's session is open (not closed_at==opened_at).
	loc, err := time.LoadLocation(defaultTimezone)
	require.NoError(t, err)
	now := time.Date(2026, 7, 2, 15, 0, 0, 0, loc)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "honesty-seed", BaselineDays: 10})
	require.NoError(t, mustEnsure(svc, admin.ID))

	var sessions []database.CashRegisterSession
	require.NoError(t, db.
		Joins("JOIN businesses ON businesses.id = cash_register_sessions.business_id").
		Where("businesses.demo_owner_user_id = ?", admin.ID).
		Order("cash_register_sessions.id ASC").
		Find(&sessions).Error)
	require.NotEmpty(t, sessions)

	oddOnly := true
	for _, session := range sessions {
		if session.ClosedAt != nil {
			require.Falsef(t, session.OpenedAt.Equal(*session.ClosedAt),
				"session %d has opened_at == closed_at", session.ID)
			require.Truef(t, session.ClosedAt.After(session.OpenedAt),
				"session %d closed_at must be after opened_at", session.ID)
		}
		if session.ID%2 == 0 {
			oddOnly = false
		}
	}
	require.False(t, oddOnly, "session IDs must not all be odd (generator stride leak)")
}

func TestSeededPaymentsHavePayerPoolAndNonExplorerTxHashes(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "payers-admin@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "honesty-seed", BaselineDays: 14})
	require.NoError(t, mustEnsure(svc, admin.ID))

	var payments []database.Payment
	require.NoError(t, db.
		Joins("JOIN bills ON bills.id = payments.bill_id").
		Joins("JOIN businesses ON businesses.id = bills.business_id").
		Where("businesses.demo_owner_user_id = ?", admin.ID).
		Find(&payments).Error)
	require.NotEmpty(t, payments)

	payers := map[string]struct{}{}
	for _, p := range payments {
		payers[p.PayerAddr] = struct{}{}
		require.NotEqual(t, demoWallet, p.PayerAddr,
			"must not use the single demo zero-address for every payment")
		require.Falsef(t, isExplorerLinkableTxHash(p.TxHash),
			"tx_hash %q must not be explorer-linkable", p.TxHash)
	}
	require.GreaterOrEqual(t, len(payers), 8, "need ≥ 8 distinct payer addresses, got %d", len(payers))
}

func TestSeededBillNumbersHaveNoDEMOPrefix(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "bills-admin@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "honesty-seed", BaselineDays: 5})
	require.NoError(t, mustEnsure(svc, admin.ID))

	var numbers []string
	require.NoError(t, db.Model(&database.Bill{}).
		Joins("JOIN businesses ON businesses.id = bills.business_id").
		Where("businesses.demo_owner_user_id = ?", admin.ID).
		Pluck("bill_number", &numbers).Error)
	require.NotEmpty(t, numbers)
	for _, n := range numbers {
		require.Falsef(t, strings.HasPrefix(n, "DEMO-") || strings.HasPrefix(n, "#DEMO-"),
			"bill_number %q carries a DEMO- prefix", n)
		require.NotContainsf(t, n, "DEMO-", "bill_number %q must not embed DEMO-", n)
	}
}

func TestSeededPayrollIsAtMost40PercentOfRevenue(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "payroll-admin@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "honesty-seed", BaselineDays: 30})
	require.NoError(t, mustEnsure(svc, admin.ID))

	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&businesses).Error)
	require.Len(t, businesses, 2)

	for _, biz := range businesses {
		var revenueCents int64
		require.NoError(t, db.Model(&database.Bill{}).
			Where("business_id = ? AND status = ?", biz.ID, database.BillStatusPaid).
			Select("COALESCE(SUM(total_amount), 0)").Scan(&revenueCents).Error)
		require.Greater(t, revenueCents, int64(0), "business %d needs paid revenue", biz.ID)

		var payrollCents int64
		require.NoError(t, db.Model(&database.PayrollRun{}).
			Where("business_id = ? AND status = ?", biz.ID, database.PayrollRunStatusPaid).
			Select("COALESCE(SUM(net_total), 0)").Scan(&payrollCents).Error)
		require.Greater(t, payrollCents, int64(0), "business %d needs payroll", biz.ID)

		// payroll ≤ 40% of revenue for the same window
		maxPayroll := revenueCents * 40 / 100
		require.LessOrEqualf(t, payrollCents, maxPayroll,
			"business %d payroll %d exceeds 40%% of revenue %d", biz.ID, payrollCents, revenueCents)
	}
}

func TestAIGrowthFlagshipHeroKPIsAreNonZero(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "flagship-admin@example.com")

	// Mid-afternoon ET so some of today's bills have closed and live tables are occupied.
	loc, err := time.LoadLocation(defaultTimezone)
	require.NoError(t, err)
	now := time.Date(2026, 7, 2, 16, 30, 0, 0, loc)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "honesty-seed", BaselineDays: 14})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	require.NotNil(t, instance.SecondaryBusinessID)
	bizID := *instance.SecondaryBusinessID

	// Four hero KPIs: today revenue, today tips, today bills closed, open tables.
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)

	var revenueCents, tipsCents int64
	var billsToday int64
	require.NoError(t, db.Model(&database.Bill{}).
		Where("business_id = ? AND status = ? AND closed_at >= ? AND closed_at < ?",
			bizID, database.BillStatusPaid, dayStart.UTC(), dayEnd.UTC()).
		Select("COALESCE(SUM(total_amount), 0)").Scan(&revenueCents).Error)
	require.NoError(t, db.Model(&database.Bill{}).
		Where("business_id = ? AND status = ? AND closed_at >= ? AND closed_at < ?",
			bizID, database.BillStatusPaid, dayStart.UTC(), dayEnd.UTC()).
		Select("COALESCE(SUM(tip_amount), 0)").Scan(&tipsCents).Error)
	require.NoError(t, db.Model(&database.Bill{}).
		Where("business_id = ? AND status = ? AND closed_at >= ? AND closed_at < ?",
			bizID, database.BillStatusPaid, dayStart.UTC(), dayEnd.UTC()).
		Count(&billsToday).Error)

	var openTables int64
	require.NoError(t, db.Model(&database.Bill{}).
		Where("business_id = ? AND status IN ? AND table_id > 0",
			bizID, []database.BillStatus{database.BillStatusOpen, database.BillStatusPartial}).
		Distinct("table_id").
		Count(&openTables).Error)

	require.Greater(t, revenueCents, int64(0), "flagship demo hero KPI revenue must be non-zero")
	require.Greater(t, tipsCents, int64(0), "flagship demo hero KPI tips must be non-zero")
	require.Greater(t, billsToday, int64(0), "flagship demo hero KPI bills closed must be non-zero")
	require.Greater(t, openTables, int64(0), "flagship demo hero KPI open tables must be non-zero")
}

func keysUint(m map[uint][]string) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
