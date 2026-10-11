package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// setupShiftReminderTestDB opens an isolated in-memory SQLite DB, migrates the
// tables the shift-reminder scheduler reads/writes (Business, schedule settings,
// Schedule, Shift, and the notification outbox the reminder enqueues into),
// wires it as the global test DB (so EnqueuePluginNotification persists), and
// returns it.
func setupShiftReminderTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, gdb.AutoMigrate(
		&database.Business{}, &database.BusinessScheduleSettings{},
		&database.Schedule{}, &database.Shift{}, &database.PluginNotificationDelivery{},
		// Staff-notification last-mile: the reminder now fans out through
		// NotifyStaff (inbox + SSE + push), which resolves staff→user and writes
		// staff_notifications rows.
		&database.Staff{}, &database.User{}, &database.StaffNotification{},
		// Telegram outbox is gated on a connected config (don't enqueue into the
		// void); tests that assert the outbox enqueue seed one via these tables.
		&database.Plugin{}, &database.BusinessPlugin{},
	))
	prev := database.GetDB()
	database.SetTestDB(gdb)
	t.Cleanup(func() { database.SetTestDB(prev) })
	return gdb
}

func seedReminderBusiness(t *testing.T, gdb *gorm.DB, id uint, tz string) {
	t.Helper()
	require.NoError(t, gdb.Create(&database.Business{
		ID: id, BusinessId: fmt.Sprintf("biz-%d", id), Name: "Bistro", IsActive: true,
		Timezone: tz, SettlementAddr: "0xset", TippingAddr: "0xtip",
	}).Error)
}

// seedConnectedTelegramShiftReminder wires a connected Telegram config with the
// (opt-in) shift_reminder event enabled, so the reminder scheduler's gated
// outbox enqueue fires. Without this the gate fails closed (no config).
func seedConnectedTelegramShiftReminder(t *testing.T, gdb *gorm.DB, businessID uint) {
	t.Helper()
	plugin := database.Plugin{Name: "telegram", DisplayName: "Telegram", IsActive: true}
	require.NoError(t, gdb.Create(&plugin).Error)
	require.NoError(t, gdb.Create(&database.BusinessPlugin{
		BusinessID: businessID, PluginID: plugin.ID, IsEnabled: true,
		Config: `{"is_connected":true,"chat_id":"123456","notifications":{"shift_reminder":true}}`,
	}).Error)
	database.SetTestDB(gdb) // fire OnDBChange → clear eligibility cache
}

// seedUpcomingShift inserts a published, filled, not-yet-reminded shift starting
// startsAt for the given business + staff.
func seedUpcomingShift(t *testing.T, gdb *gorm.DB, businessID, staffID uint, startsAt time.Time) database.Shift {
	t.Helper()
	sp := staffID
	sh := database.Shift{
		BusinessID: businessID, ScheduleID: 1, PositionID: 1, StaffID: &sp,
		StartsAt: startsAt, EndsAt: startsAt.Add(8 * time.Hour),
		Status: database.ShiftStatusFilled, Published: true, CreatedByStaffID: 1,
	}
	require.NoError(t, gdb.Create(&sh).Error)
	return sh
}

func TestShiftReminderClaimSingleWinner(t *testing.T) {
	gdb := setupShiftReminderTestDB(t)
	seedReminderBusiness(t, gdb, 1, "UTC")
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	sh := seedUpcomingShift(t, gdb, 1, 7, now.Add(1*time.Hour))

	s := NewShiftReminderScheduler(database.GetDBWrapper(), nil)

	// Two claims race for the same shift; exactly one wins (RowsAffected==1).
	first := s.claimReminder(sh.ID, now)
	second := s.claimReminder(sh.ID, now)
	require.True(t, first, "first claim must win")
	require.False(t, second, "second claim must lose (already reminded)")

	var reloaded database.Shift
	require.NoError(t, gdb.First(&reloaded, sh.ID).Error)
	require.NotNil(t, reloaded.RemindedAt, "reminded_at must be stamped by the winning claim")
}

func TestShiftReminderWithinQuietHours(t *testing.T) {
	mk := func(v int) *int { return &v }
	// Daytime window 09:00-17:00 (540..1020).
	require.True(t, withinQuietHours(600, mk(540), mk(1020))) // 10:00 inside
	require.False(t, withinQuietHours(60, mk(540), mk(1020))) // 01:00 outside
	// Overnight window 22:00-06:00 (1320..360) wraps midnight.
	require.True(t, withinQuietHours(1380, mk(1320), mk(360))) // 23:00 inside
	require.True(t, withinQuietHours(120, mk(1320), mk(360)))  // 02:00 inside
	require.False(t, withinQuietHours(720, mk(1320), mk(360))) // 12:00 outside
	// Nil bounds = no quiet hours = never suppressed.
	require.False(t, withinQuietHours(600, nil, mk(1020)))
	require.False(t, withinQuietHours(600, mk(540), nil))
}

func TestShiftReminderQuietHoursSuppresses(t *testing.T) {
	gdb := setupShiftReminderTestDB(t)
	seedReminderBusiness(t, gdb, 1, "UTC")
	seedConnectedTelegramShiftReminder(t, gdb, 1)
	// now = local 02:00 UTC; quiet hours 00:00-06:00 (0..360) cover it.
	now := time.Date(2026, 6, 30, 2, 0, 0, 0, time.UTC)
	qs, qe := 0, 360
	require.NoError(t, gdb.Create(&database.BusinessScheduleSettings{
		BusinessID: 1, WeekStartDay: 1, DefaultShiftMinutes: 480, ReminderLeadHours: 3,
		OvertimeWeeklyMinutes: 2400, PostedLeadDays: 7,
		QuietHoursStartMin: &qs, QuietHoursEndMin: &qe,
	}).Error)
	sh := seedUpcomingShift(t, gdb, 1, 7, now.Add(1*time.Hour)) // within the 3h lead

	s := NewShiftReminderScheduler(database.GetDBWrapper(), nil)
	s.checkAt(now)

	var reloaded database.Shift
	require.NoError(t, gdb.First(&reloaded, sh.ID).Error)
	require.Nil(t, reloaded.RemindedAt, "quiet hours must suppress the reminder (no claim)")
	var notifs int64
	require.NoError(t, gdb.Model(&database.PluginNotificationDelivery{}).Count(&notifs).Error)
	require.Equal(t, int64(0), notifs)

	// Outside quiet hours the same check reminds and enqueues exactly once.
	dayNow := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	// Re-point the shift to be within lead of the daytime 'now' (end follows).
	require.NoError(t, gdb.Model(&database.Shift{}).Where("id = ?", sh.ID).
		Updates(map[string]interface{}{
			"starts_at": dayNow.Add(1 * time.Hour),
			"ends_at":   dayNow.Add(9 * time.Hour),
		}).Error)
	s.checkAt(dayNow)

	require.NoError(t, gdb.First(&reloaded, sh.ID).Error)
	require.NotNil(t, reloaded.RemindedAt, "outside quiet hours the shift must be reminded")
	require.NoError(t, gdb.Model(&database.PluginNotificationDelivery{}).Count(&notifs).Error)
	require.Equal(t, int64(1), notifs)
}

// TestShiftReminderDeliversStaffNotification is the last-mile regression: a
// reminder must reach the assigned staffer's durable inbox (NotifyStaff), not
// just the Telegram outbox. A staffer with no push subscription still gets the
// inbox row (web-push service nil here) — proving the staff app sees reminders
// even without a Telegram integration.
func TestShiftReminderDeliversStaffNotification(t *testing.T) {
	gdb := setupShiftReminderTestDB(t)
	seedReminderBusiness(t, gdb, 1, "UTC")
	require.NoError(t, gdb.Create(&database.Staff{
		ID: 7, BusinessID: 1, Email: "dana@bistro.test", Name: "Dana",
		Role: "server", IsActive: true,
	}).Error)
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	sh := seedUpcomingShift(t, gdb, 1, 7, now.Add(1*time.Hour)) // within the default 3h lead

	s := NewShiftReminderScheduler(database.GetDBWrapper(), nil) // nil push → inbox + SSE only
	s.checkAt(now)

	var reloaded database.Shift
	require.NoError(t, gdb.First(&reloaded, sh.ID).Error)
	require.NotNil(t, reloaded.RemindedAt, "the shift must be claimed + reminded")

	var notes []database.StaffNotification
	require.NoError(t, gdb.Where("business_id = ? AND staff_id = ?", 1, 7).Find(&notes).Error)
	require.Len(t, notes, 1, "the reminder must land in the assigned staffer's inbox")
	require.Equal(t, "shift.reminder", notes[0].Kind)
	require.Equal(t, "/staff/home?tab=schedule", notes[0].URL)
	require.NotContains(t, notes[0].Title+notes[0].Body, "$", "staff notifications are money-free")
}

func TestShiftReminderOutsideLeadWindowNotReminded(t *testing.T) {
	gdb := setupShiftReminderTestDB(t)
	seedReminderBusiness(t, gdb, 1, "UTC")
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	// Default lead is 3h (no settings row -> defaults). A shift 10h out is too far.
	sh := seedUpcomingShift(t, gdb, 1, 7, now.Add(10*time.Hour))

	s := NewShiftReminderScheduler(database.GetDBWrapper(), nil)
	s.checkAt(now)

	var reloaded database.Shift
	require.NoError(t, gdb.First(&reloaded, sh.ID).Error)
	require.Nil(t, reloaded.RemindedAt, "a shift beyond the lead window must not be reminded yet")
}

// TestShiftReminderQuietHoursSwallowedShiftLateSend is the quiet-hours-swallow
// regression: a shift whose ENTIRE lead window falls inside quiet hours (e.g. a
// 05:00 shift with quiet hours until 06:00 and a 3h lead) used to never remind,
// because post-quiet ticks required starts_at > now. The first tick after quiet
// hours must still send a late reminder for a shift that already started but has
// not yet ended — exactly once.
func TestShiftReminderQuietHoursSwallowedShiftLateSend(t *testing.T) {
	gdb := setupShiftReminderTestDB(t)
	seedReminderBusiness(t, gdb, 1, "UTC")
	seedConnectedTelegramShiftReminder(t, gdb, 1)
	// Quiet hours 22:00-06:00 (1320..360, overnight wrap); default 3h lead.
	qs, qe := 1320, 360
	require.NoError(t, gdb.Create(&database.BusinessScheduleSettings{
		BusinessID: 1, WeekStartDay: 1, DefaultShiftMinutes: 480, ReminderLeadHours: 3,
		OvertimeWeeklyMinutes: 2400, PostedLeadDays: 7,
		QuietHoursStartMin: &qs, QuietHoursEndMin: &qe,
	}).Error)

	// Shift at 05:00 — its whole 02:00-05:00 lead window is inside quiet hours.
	sh := seedUpcomingShift(t, gdb, 1, 7, time.Date(2026, 6, 30, 5, 0, 0, 0, time.UTC))

	s := NewShiftReminderScheduler(database.GetDBWrapper(), nil)

	// Ticks during quiet hours hold the reminder (no claim).
	s.checkAt(time.Date(2026, 6, 30, 2, 30, 0, 0, time.UTC))
	s.checkAt(time.Date(2026, 6, 30, 5, 30, 0, 0, time.UTC))
	var reloaded database.Shift
	require.NoError(t, gdb.First(&reloaded, sh.ID).Error)
	require.Nil(t, reloaded.RemindedAt, "quiet-hours ticks must not claim")

	// First post-quiet tick: 06:15. The shift started at 05:00 (in the past) but
	// runs until 13:00 — a late reminder must still go out, exactly once.
	s.checkAt(time.Date(2026, 6, 30, 6, 15, 0, 0, time.UTC))
	require.NoError(t, gdb.First(&reloaded, sh.ID).Error)
	require.NotNil(t, reloaded.RemindedAt, "post-quiet tick must late-send a swallowed reminder")
	var notifs int64
	require.NoError(t, gdb.Model(&database.PluginNotificationDelivery{}).Count(&notifs).Error)
	require.Equal(t, int64(1), notifs)

	// A later tick must not double-send.
	s.checkAt(time.Date(2026, 6, 30, 7, 0, 0, 0, time.UTC))
	require.NoError(t, gdb.Model(&database.PluginNotificationDelivery{}).Count(&notifs).Error)
	require.Equal(t, int64(1), notifs)
}

// TestShiftReminderLateWindowBounded: the late-send fallback is bounded — a
// shift that started long ago (>2h) or has already ENDED must never get a
// "coming up" reminder.
func TestShiftReminderLateWindowBounded(t *testing.T) {
	gdb := setupShiftReminderTestDB(t)
	seedReminderBusiness(t, gdb, 1, "UTC")
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)

	// Started 3h ago (outside the 2h late window) — no reminder.
	stale := seedUpcomingShift(t, gdb, 1, 7, now.Add(-3*time.Hour))
	// Started 1h ago but already ended — no reminder.
	sp := uint(8)
	ended := database.Shift{
		BusinessID: 1, ScheduleID: 1, PositionID: 1, StaffID: &sp,
		StartsAt: now.Add(-90 * time.Minute), EndsAt: now.Add(-30 * time.Minute),
		Status: database.ShiftStatusFilled, Published: true, CreatedByStaffID: 1,
	}
	require.NoError(t, gdb.Create(&ended).Error)

	s := NewShiftReminderScheduler(database.GetDBWrapper(), nil)
	s.checkAt(now)

	var reloaded database.Shift
	require.NoError(t, gdb.First(&reloaded, stale.ID).Error)
	require.Nil(t, reloaded.RemindedAt, "a shift started >2h ago must not be late-reminded")
	reloaded = database.Shift{}
	require.NoError(t, gdb.First(&reloaded, ended.ID).Error)
	require.Nil(t, reloaded.RemindedAt, "an already-ended shift must never be reminded")
}
