package database

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var staffNotifBenchSeq atomic.Uint64

type staffNotifSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *staffNotifSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, strings.ToLower(sql))
}

func (r *staffNotifSQLRecorder) selects() []string {
	var out []string
	for _, s := range r.statements {
		if strings.Contains(s, "select") && strings.Contains(s, "staff_notifications") {
			out = append(out, s)
		}
	}
	return out
}

func newStaffNotificationTestDB(t *testing.T) (*DB, func()) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&Business{}, &Staff{}, &StaffNotification{}))
	SetTestDB(g)
	return GetDBWrapper(), func() { _ = sqlDB.Close() }
}

// TestStaffNotificationGenesisShape asserts the table + named composite index
// materialize from struct tags alone on a force-baselined (genesis) DB.
func TestStaffNotificationGenesisShape(t *testing.T) {
	db, cleanup := newStaffNotificationTestDB(t)
	defer cleanup()
	m := db.GetGorm().Migrator()
	require.True(t, m.HasTable(&StaffNotification{}), "staff_notifications table must exist on genesis")
	for _, col := range []string{"id", "business_id", "staff_id", "kind", "title", "body", "url", "read_at", "created_at"} {
		require.Truef(t, m.HasColumn(&StaffNotification{}, col), "staff_notifications missing genesis column %s", col)
	}
	require.True(t, m.HasIndex(&StaffNotification{}, "idx_staff_notifications_inbox"),
		"staff_notifications composite (business_id, staff_id, id) index must materialize on genesis")
}

func TestStaffNotificationCRUD(t *testing.T) {
	db, cleanup := newStaffNotificationTestDB(t)
	defer cleanup()

	require.NoError(t, db.CreateStaffNotifications([]StaffNotification{
		{BusinessID: 1, StaffID: 7, Kind: "shift.assigned", Title: "A", Body: "b", URL: "/x"},
		{BusinessID: 1, StaffID: 7, Kind: "chat.announcement", Title: "B", Body: "b", URL: "/y"},
		{BusinessID: 1, StaffID: 9, Kind: "shift.assigned", Title: "C", Body: "b", URL: "/z"},
	}))

	// Self-scoped list: staff 7 sees only its two rows, newest (highest id) first.
	rows, err := db.ListStaffNotifications(1, 7, 50, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Greater(t, rows[0].ID, rows[1].ID)

	// Keyset: before the first id returns the older one only.
	older, err := db.ListStaffNotifications(1, 7, 50, rows[0].ID)
	require.NoError(t, err)
	require.Len(t, older, 1)
	require.Equal(t, rows[1].ID, older[0].ID)

	// Unread count.
	n, err := db.CountUnreadStaffNotifications(1, 7)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)

	// Mark one read (idempotent on re-run); count drops to 1.
	updated, err := db.MarkStaffNotificationsRead(1, 7, []uint{rows[0].ID}, false)
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
	again, err := db.MarkStaffNotificationsRead(1, 7, []uint{rows[0].ID}, false)
	require.NoError(t, err)
	require.EqualValues(t, 0, again) // already read → no-op
	n, _ = db.CountUnreadStaffNotifications(1, 7)
	require.EqualValues(t, 1, n)

	// markAll clears the rest.
	_, err = db.MarkStaffNotificationsRead(1, 7, nil, true)
	require.NoError(t, err)
	n, _ = db.CountUnreadStaffNotifications(1, 7)
	require.EqualValues(t, 0, n)

	// Tenant/row isolation: staff 9's row untouched by staff 7's reads.
	n9, _ := db.CountUnreadStaffNotifications(1, 9)
	require.EqualValues(t, 1, n9)
}

// TestStaffNotificationListAccessShape is the perf gate: the inbox read is one
// bounded query over an explicit projection (no SELECT *), row-scoped in SQL.
func TestStaffNotificationListAccessShape(t *testing.T) {
	recorder := &staffNotifSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db, cleanup := newStaffNotificationTestDB(t)
	defer cleanup()
	rows := make([]StaffNotification, 0, 5)
	for i := 0; i < 5; i++ {
		rows = append(rows, StaffNotification{BusinessID: 1, StaffID: 7, Kind: "k", Title: "t"})
	}
	require.NoError(t, db.CreateStaffNotifications(rows))

	gdb := db.GetGorm()
	prev := gdb.Logger
	gdb.Logger = recorder
	_, err := db.ListStaffNotifications(1, 7, 50, 0)
	require.NoError(t, err)
	gdb.Logger = prev

	stmts := recorder.selects()
	require.Len(t, stmts, 1, "inbox list must be exactly one query, got: %v", recorder.statements)
	require.NotContains(t, stmts[0], "select *", "must not SELECT * the staff_notifications row")
	require.Contains(t, stmts[0], "staff_id", "inbox read must be row-scoped by staff_id in SQL")
	require.Contains(t, stmts[0], "limit", "list must be bounded by a LIMIT")
}

func BenchmarkListStaffNotifications(b *testing.B) {
	dsn := fmt.Sprintf("file:bench-staffnotif-%d?mode=memory&cache=shared", staffNotifBenchSeq.Add(1))
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(b, err)
	sqlDB, err := g.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(b, g.AutoMigrate(&Business{}, &Staff{}, &StaffNotification{}))
	prev := db
	SetTestDB(g)
	defer SetTestDB(prev)
	d := GetDBWrapper()
	rows := make([]StaffNotification, 0, 200)
	for i := 0; i < 200; i++ {
		rows = append(rows, StaffNotification{BusinessID: 1, StaffID: 7, Kind: "k", Title: "t"})
	}
	require.NoError(b, d.CreateStaffNotifications(rows))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.ListStaffNotifications(1, 7, 50, 0); err != nil {
			b.Fatal(err)
		}
	}
}
