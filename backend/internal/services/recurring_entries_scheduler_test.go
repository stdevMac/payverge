package services

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAdvanceNextRun_Monthly(t *testing.T) {
	from := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	got := advanceNextRun(from, "monthly", 15)
	require.Equal(t, time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC), got)

	// Anchor clamped to 28.
	got = advanceNextRun(from, "monthly", 31)
	require.Equal(t, time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), got)
}

func TestAdvanceNextRun_Weekly(t *testing.T) {
	from := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC) // Monday
	got := advanceNextRun(from, "weekly", 1)
	require.Equal(t, time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), got)
}

func TestAdvanceNextRun_YearBoundary(t *testing.T) {
	from := time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC)
	got := advanceNextRun(from, "monthly", 10)
	require.Equal(t, time.Date(2027, 1, 10, 0, 0, 0, 0, time.UTC), got)
}

func setupRecurringSchedulerDB(t *testing.T) *database.DB {
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
		&database.RecurringEntryTemplate{},
		&database.ManualLedgerEntry{},
	))
	return database.GetDBWrapper()
}

// TestProcessDue_BusinessTimezoneGate: due-ness is decided on the OWNING
// business's local calendar date, not the UTC date. A template anchored on
// March 1 for a Los Angeles business must NOT generate while it is still
// February 28 locally, even though UTC has already rolled to March 1 — and
// must generate once local midnight passes.
func TestProcessDue_BusinessTimezoneGate(t *testing.T) {
	db := setupRecurringSchedulerDB(t)
	biz := &database.Business{
		BusinessId:      "tz-biz",
		Name:            "TZ Biz",
		OwnerAddress:    "0xTZ",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		Timezone:        "America/Los_Angeles",
	}
	require.NoError(t, db.GetGorm().Create(biz).Error)
	tmpl := &database.RecurringEntryTemplate{
		BusinessID:  biz.ID,
		EntryType:   "expense",
		Category:    "rent",
		AmountCents: 150000,
		Currency:    "USD",
		Description: "Monthly rent",
		Cadence:     "monthly",
		AnchorDay:   1,
		NextRunOn:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Active:      true,
	}
	require.NoError(t, db.GetGorm().Create(tmpl).Error)

	sched := NewRecurringEntriesScheduler(db)

	// 02:00 UTC on Mar 1 == 18:00 PST on Feb 28 → not due locally yet.
	require.NoError(t, sched.ProcessDue(time.Date(2026, 3, 1, 2, 0, 0, 0, time.UTC)))
	var count int64
	require.NoError(t, db.GetGorm().Model(&database.ManualLedgerEntry{}).
		Where("business_id = ?", biz.ID).Count(&count).Error)
	require.Zero(t, count, "must not generate while the business is still on Feb 28 locally")

	// 09:00 UTC on Mar 1 == 01:00 PST on Mar 1 → due.
	require.NoError(t, sched.ProcessDue(time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)))
	require.NoError(t, db.GetGorm().Model(&database.ManualLedgerEntry{}).
		Where("business_id = ?", biz.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)

	var entry database.ManualLedgerEntry
	require.NoError(t, db.GetGorm().Where("business_id = ?", biz.ID).First(&entry).Error)
	require.Equal(t, fmt.Sprintf("recurring:%d:2026-03-01", tmpl.ID), entry.Reference)

	// Idempotent: a rerun the same day does not double-generate.
	require.NoError(t, sched.ProcessDue(time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)))
	require.NoError(t, db.GetGorm().Model(&database.ManualLedgerEntry{}).
		Where("business_id = ?", biz.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

// A period-lock lookup that errors must fail closed: no entry is posted, the
// template does not advance, and it is retried on the next tick. A locked day
// flags needs_attention without posting.
func TestProcessDue_PeriodLockLookupErrorFailsClosed(t *testing.T) {
	db := setupRecurringSchedulerDB(t)
	biz := &database.Business{
		BusinessId:      "lock-err-biz",
		Name:            "Lock Err Biz",
		OwnerAddress:    "0xLE",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		Timezone:        "UTC",
	}
	require.NoError(t, db.GetGorm().Create(biz).Error)
	due := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	tmpl := &database.RecurringEntryTemplate{
		BusinessID:  biz.ID,
		EntryType:   "expense",
		Category:    "rent",
		AmountCents: 150000,
		Currency:    "USD",
		Description: "Monthly rent",
		Cadence:     "monthly",
		AnchorDay:   15,
		NextRunOn:   due,
		Active:      true,
	}
	require.NoError(t, db.GetGorm().Create(tmpl).Error)

	lockErr := errors.New("connection reset")
	locked := false
	sched := NewRecurringEntriesScheduler(db).WithPeriodLockChecker(func(*gorm.DB, uint, time.Time) (bool, error) {
		if lockErr != nil {
			return false, lockErr
		}
		return locked, nil
	})
	countEntries := func() int64 {
		var n int64
		require.NoError(t, db.GetGorm().Model(&database.ManualLedgerEntry{}).Where("business_id = ?", biz.ID).Count(&n).Error)
		return n
	}
	reload := func() database.RecurringEntryTemplate {
		var got database.RecurringEntryTemplate
		require.NoError(t, db.GetGorm().First(&got, tmpl.ID).Error)
		return got
	}

	tick := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	require.NoError(t, sched.ProcessDue(tick))
	require.Zero(t, countEntries(), "a failed lock read must not post")
	require.True(t, reload().NextRunOn.Equal(due), "a failed lock read must not advance next_run_on")

	lockErr, locked = nil, true
	require.NoError(t, sched.ProcessDue(tick))
	require.Zero(t, countEntries())
	got := reload()
	require.True(t, got.NeedsAttention)
	require.True(t, got.NextRunOn.Equal(due))

	locked = false
	require.NoError(t, sched.ProcessDue(tick))
	require.Equal(t, int64(1), countEntries())
	require.False(t, reload().NextRunOn.Equal(due))
}
