package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupDueReportSchedulesDB(t testing.TB, gormLogger logger.Interface) *DB {
	t.Helper()
	if gormLogger == nil {
		gormLogger = logger.Default.LogMode(logger.Silent)
	}
	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormLogger})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	prev := db
	SetTestDB(gdb)
	t.Cleanup(func() {
		SetTestDB(prev)
		_ = sqlDB.Close()
	})
	require.NoError(t, gdb.AutoMigrate(&Business{}, &ReportSchedule{}, &ReportDelivery{}))

	business := Business{
		BusinessId:     fmt.Sprintf("due-reports-%d", time.Now().UnixNano()),
		Name:           "Due Reports",
		OwnerAddress:   "0xDueReportsOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gdb.Create(&business).Error)

	base := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	schedules := make([]ReportSchedule, 60)
	for i := range schedules {
		schedules[i] = ReportSchedule{
			BusinessID: business.ID,
			Frequency:  ReportFrequencyDaily,
			Hour:       9,
			Minute:     0,
			Timezone:   "UTC",
			IsActive:   true,
			NextSendAt: base.Add(time.Duration(i) * time.Minute),
		}
	}
	require.NoError(t, gdb.Create(&schedules).Error)
	return NewDBWithConn(gdb)
}

// TestGetDueReportSchedulesAccessShape guards the scheduler tick read: one
// bounded projection, no Business preload.
func TestGetDueReportSchedulesAccessShape(t *testing.T) {
	cap := &sqlCapture{Interface: logger.Default.LogMode(logger.Silent)}
	wrapper := setupDueReportSchedulesDB(t, cap)

	cap.mu.Lock()
	cap.sqls = nil
	cap.mu.Unlock()

	got, err := wrapper.GetDueReportSchedules(time.Now().UTC().Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, got, 50)

	cap.mu.Lock()
	sqls := append([]string(nil), cap.sqls...)
	cap.mu.Unlock()

	require.Len(t, sqls, 1, "due schedules must be one statement, got %v", sqls)
	stmt := sqls[0]
	require.Contains(t, strings.ToUpper(stmt), "LIMIT")
	require.NotContains(t, strings.ToLower(stmt), "businesses")
	for _, s := range sqls {
		require.NotContains(t, strings.ToLower(s), "businesses")
	}
	require.Zero(t, got[0].Business.ID, "Business must not be preloaded")
	require.NotZero(t, got[0].BusinessID)
	require.True(t, got[0].IsActive)
	for i := 1; i < len(got); i++ {
		prev := got[i-1]
		cur := got[i]
		if cur.NextSendAt.Before(prev.NextSendAt) || (cur.NextSendAt.Equal(prev.NextSendAt) && cur.ID < prev.ID) {
			t.Fatalf("schedules not ordered by next_send_at, id: index %d id %d before %d", i, prev.ID, cur.ID)
		}
	}
}

// TestGetDueReportSchedulesSkipsUnclaimableOccurrences pins that schedules
// whose current occurrence is dead-lettered, sent, backing off or under a
// live lease cannot fill the bounded batch: dead letters never advance
// next_send_at, so they would otherwise starve every later schedule forever.
func TestGetDueReportSchedulesSkipsUnclaimableOccurrences(t *testing.T) {
	wrapper := setupDueReportSchedulesDB(t, nil)
	now := time.Now().UTC()

	var all []ReportSchedule
	require.NoError(t, db.Order("next_send_at ASC, id ASC").Find(&all).Error)
	require.Len(t, all, 60)

	later := now.Add(time.Hour)
	expired := now.Add(-time.Minute)
	blocked := map[uint]bool{}
	for i := 0; i < 55; i++ {
		s := all[i]
		row := ReportDelivery{
			ScheduleID:     s.ID,
			BusinessID:     s.BusinessID,
			ScheduledFor:   s.NextSendAt,
			WindowStart:    s.NextSendAt.Add(-24 * time.Hour),
			WindowEnd:      s.NextSendAt,
			NextAttemptAt:  s.NextSendAt,
			IdempotencyKey: fmt.Sprintf("due-%d", s.ID),
		}
		switch i % 4 {
		case 0:
			row.State = ReportDeliveryStateDeadLetter
		case 1:
			row.State = ReportDeliveryStateRetryWait
			row.NextAttemptAt = later
		case 2:
			row.State = ReportDeliveryStateLeased
			row.LeaseExpiresAt = &later
		case 3:
			row.State = ReportDeliveryStateSent
		}
		blocked[s.ID] = true
		require.NoError(t, db.Omit("Schedule").Create(&row).Error)
	}
	// A leased occurrence whose lease expired is still due: the worker must
	// reclaim it or dead-letter it.
	expiredLease := all[55]
	require.NoError(t, db.Omit("Schedule").Create(&ReportDelivery{
		ScheduleID: expiredLease.ID, BusinessID: expiredLease.BusinessID,
		ScheduledFor: expiredLease.NextSendAt, WindowStart: expiredLease.NextSendAt.Add(-24 * time.Hour),
		WindowEnd: expiredLease.NextSendAt, NextAttemptAt: expiredLease.NextSendAt,
		State: ReportDeliveryStateLeased, LeaseExpiresAt: &expired, IdempotencyKey: "due-expired",
	}).Error)

	got, err := wrapper.GetDueReportSchedules(now)
	require.NoError(t, err)
	require.Len(t, got, 5, "only the claimable schedules are due")
	for _, s := range got {
		require.False(t, blocked[s.ID], "schedule %d has an unclaimable occurrence", s.ID)
	}
	require.Equal(t, expiredLease.ID, got[0].ID)
}

func BenchmarkGetDueReportSchedules(b *testing.B) {
	wrapper := setupDueReportSchedulesDB(b, nil)
	due := time.Now().UTC().Add(time.Hour)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		schedules, err := wrapper.GetDueReportSchedules(due)
		if err != nil {
			b.Fatal(err)
		}
		if len(schedules) == 0 {
			b.Fatal("expected due schedules")
		}
	}
}
