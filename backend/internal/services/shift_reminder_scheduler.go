package services

import (
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// shiftReminderMaxLeadHours caps how far ahead the candidate scan looks. The
// per-business ReminderLeadHours is bounded to 0..168 at the settings handler,
// so a 168h (7-day) ceiling covers every configurable lead while keeping the
// candidate set bounded regardless of how far out shifts are scheduled.
const shiftReminderMaxLeadHours = 168

// shiftReminderDefaultLeadHours mirrors BusinessScheduleSettings' default for a
// business that has not materialized a settings row yet.
const shiftReminderDefaultLeadHours = 3

// shiftReminderMaxLateWindow bounds the late-send fallback for shifts whose
// entire lead window was swallowed by quiet hours (or scheduler downtime): a
// shift that already STARTED is still a candidate if it started less than this
// long ago AND has not yet ended. The push copy is time-phrased ("Your shift on
// %s is coming up"), so the window is kept tight — a 2h-late nudge is still
// useful and roughly true; older than that is noise.
const shiftReminderMaxLateWindow = 2 * time.Hour

// ShiftReminderScheduler enqueues a per-shift reminder a configurable lead time
// before each published, assigned (filled) shift starts. Deduplication is
// DB-backed via an atomic claim on shifts.reminded_at, so
// restarts and multi-replica deployments cannot double-remind. Quiet hours
// (BusinessScheduleSettings.QuietHoursStartMin/EndMin, evaluated in the
// business timezone) suppress reminders so nobody is pinged overnight.
type ShiftReminderScheduler struct {
	dbw       *database.DB
	db        *gorm.DB
	wp        *WebPushService
	stopChan  chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	isRunning bool
}

// NewShiftReminderScheduler creates a ShiftReminderScheduler bound to the DB
// wrapper (for the scan/claim queries and the staff-notification fan-out) and
// the web-push service (best-effort phone push; may be nil).
func NewShiftReminderScheduler(dbw *database.DB, wp *WebPushService) *ShiftReminderScheduler {
	return &ShiftReminderScheduler{dbw: dbw, db: dbw.GetGorm(), wp: wp, stopChan: make(chan struct{})}
}

// shiftReminderKind is the durable-notification Kind for a shift reminder (the
// staff inbox + SSE + push channel). shiftReminderURL deep-links the staff app
// to their schedule.
const (
	shiftReminderKind = "shift.reminder"
	shiftReminderURL  = "/staff/home?tab=schedule"
)

// Start begins the scheduler loop in a background goroutine.
func (s *ShiftReminderScheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isRunning {
		return fmt.Errorf("shift reminder scheduler already running")
	}
	s.isRunning = true
	s.stopChan = make(chan struct{}) // re-create so a Stop→Start cycle is safe
	s.wg.Add(1)
	go s.loop()
	log.Println("Shift reminder scheduler started")
	return nil
}

// Stop gracefully shuts down the scheduler and waits for in-flight work.
func (s *ShiftReminderScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.isRunning {
		return
	}
	close(s.stopChan)
	s.wg.Wait()
	s.isRunning = false
	log.Println("Shift reminder scheduler stopped")
}

func (s *ShiftReminderScheduler) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	// Run immediately on startup so the first check is not delayed 15 minutes.
	logger.SafeTick("shift-reminder-scheduler", s.check)

	for {
		select {
		case <-ticker.C:
			logger.SafeTick("shift-reminder-scheduler", s.check)
		case <-s.stopChan:
			return
		}
	}
}

func (s *ShiftReminderScheduler) check() {
	s.checkAt(time.Now().UTC())
}

// reminderCandidate is the projected shape the scan needs — never SELECT *.
type reminderCandidate struct {
	ID         uint
	BusinessID uint
	StaffID    *uint
	StartsAt   time.Time
}

// checkAt scans for published, assigned, not-yet-reminded shifts starting within
// the per-business lead window, suppresses during quiet hours (business-TZ), and
// atomically claims + enqueues a reminder for each remaining shift. Driven by an
// explicit now so the schedule is deterministic under test.
func (s *ShiftReminderScheduler) checkAt(now time.Time) {
	horizon := now.Add(shiftReminderMaxLeadHours * time.Hour)
	// Late-send fallback: include shifts that already STARTED within the bounded
	// late window and have not yet ENDED, so a reminder whose whole lead window
	// fell inside quiet hours (or a downtime gap) is sent late instead of never.
	lateFloor := now.Add(-shiftReminderMaxLateWindow)
	var candidates []reminderCandidate
	if err := s.db.Model(&database.Shift{}).
		Select("id", "business_id", "staff_id", "starts_at").
		Where("published = ? AND status = ? AND staff_id IS NOT NULL AND reminded_at IS NULL AND starts_at > ? AND starts_at <= ? AND ends_at > ?",
			true, database.ShiftStatusFilled, lateFloor, horizon, now).
		Order("business_id asc, starts_at asc").
		Find(&candidates).Error; err != nil {
		log.Printf("[ShiftReminderScheduler] candidate scan failed: %v", err)
		return
	}
	if len(candidates) == 0 {
		return
	}

	byBiz := map[uint][]reminderCandidate{}
	for _, cand := range candidates {
		byBiz[cand.BusinessID] = append(byBiz[cand.BusinessID], cand)
	}
	for bizID, shifts := range byBiz {
		s.processBusiness(bizID, shifts, now)
	}
}

func (s *ShiftReminderScheduler) processBusiness(businessID uint, shifts []reminderCandidate, now time.Time) {
	leadHours, quietStart, quietEnd := s.loadReminderSettings(businessID)

	// Quiet-hours suppression is evaluated in the business timezone. While the
	// local wall-clock is inside the quiet window, reminders are held (no claim)
	// so they fire on a later, non-quiet tick.
	loc := s.loadLocation(businessID)
	localNow := now.In(loc)
	localMinute := localNow.Hour()*60 + localNow.Minute()
	if withinQuietHours(localMinute, quietStart, quietEnd) {
		return
	}

	leadCutoff := now.Add(time.Duration(leadHours) * time.Hour)
	for _, cand := range shifts {
		if cand.StaffID == nil {
			continue
		}
		if cand.StartsAt.After(leadCutoff) {
			continue // not yet within this business's lead window
		}
		if !s.claimReminder(cand.ID, now) {
			continue // another replica/tick already claimed it
		}
		s.enqueueReminder(businessID, cand, now, loc)
	}
}

// loadReminderSettings returns the business's reminder lead hours and quiet-hours
// window, falling back to defaults when no settings row exists. Read-only — it
// never materializes a settings row as a side effect.
func (s *ShiftReminderScheduler) loadReminderSettings(businessID uint) (leadHours int, quietStart, quietEnd *int) {
	var settings database.BusinessScheduleSettings
	if err := s.db.Where("business_id = ?", businessID).First(&settings).Error; err != nil {
		return shiftReminderDefaultLeadHours, nil, nil
	}
	lead := settings.ReminderLeadHours
	if lead <= 0 {
		lead = shiftReminderDefaultLeadHours
	}
	return lead, settings.QuietHoursStartMin, settings.QuietHoursEndMin
}

// loadLocation resolves the business timezone, defaulting to UTC.
func (s *ShiftReminderScheduler) loadLocation(businessID uint) *time.Location {
	var biz database.Business
	if err := s.db.Select("timezone").Where("id = ?", businessID).First(&biz).Error; err != nil {
		return time.UTC
	}
	if biz.Timezone == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(biz.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// claimReminder atomically reserves a shift's reminder: it stamps reminded_at=now
// WHERE the row is still un-reminded. RowsAffected==1 means this caller won;
// replicas/restarts that lose see RowsAffected==0. Multi-replica safe with no
// advisory lock (the WHERE predicate is the guard).
func (s *ShiftReminderScheduler) claimReminder(shiftID uint, now time.Time) bool {
	res := s.db.Model(&database.Shift{}).
		Where("id = ? AND reminded_at IS NULL", shiftID).
		Update("reminded_at", now)
	if res.Error != nil {
		log.Printf("[ShiftReminderScheduler] claim failed for shift %d: %v", shiftID, res.Error)
		return false
	}
	return res.RowsAffected == 1
}

// enqueueReminder delivers the reminder on two channels. The primary, staff-
// facing channel is a durable notification (inbox + live SSE + best-effort web
// push) to the assigned staffer, localized in their own language — this is what
// the staff app actually reads, and it lands whether or not the business wired a
// Telegram integration. The Telegram outbox is a secondary operator channel; a
// transient enqueue failure is logged but no longer releases the reminded_at
// claim, because the staff notification above is the reminder of record and
// re-firing would double-notify the staffer.
func (s *ShiftReminderScheduler) enqueueReminder(businessID uint, cand reminderCandidate, now time.Time, loc *time.Location) {
	var staffID uint
	if cand.StaffID != nil {
		staffID = *cand.StaffID
	}

	shiftDate := cand.StartsAt.In(loc).Format("2006-01-02")
	NotifyStaff(s.dbw, s.wp, businessID, []uint{staffID}, shiftReminderKind,
		PushKeyShiftReminder, PushArgs{ShiftDate: shiftDate}, shiftReminderURL)

	// The staff notification above is the reminder of record; the Telegram outbox
	// is a secondary operator channel. Gate it so businesses without a connected
	// Telegram config (or with the event disabled) never accrue dead outbox rows.
	if !ShouldEnqueueTelegramNotification(businessID, PluginEventShiftReminder) {
		return
	}

	if _, _, err := EnqueuePluginNotification(PluginNotificationEvent{
		BusinessID: businessID,
		EventType:  PluginEventShiftReminder,
		EventID:    fmt.Sprintf("shift:%d:reminder", cand.ID),
		Payload: map[string]interface{}{
			"shift_id": cand.ID, "staff_id": staffID, "starts_at": cand.StartsAt,
		},
		CreatedAt: now,
	}, "telegram"); err != nil {
		log.Printf("[ShiftReminderScheduler] telegram enqueue failed for shift %d: %v", cand.ID, err)
	}
}

// withinQuietHours reports whether a minute-of-day falls inside the quiet-hours
// window [startMin, endMin). A nil bound means quiet hours are disabled (never
// suppressed). An overnight window (start > end, e.g. 22:00-06:00) wraps midnight.
func withinQuietHours(minuteOfDay int, startMin, endMin *int) bool {
	if startMin == nil || endMin == nil {
		return false
	}
	start, end := *startMin, *endMin
	if start == end {
		return false
	}
	if start < end {
		return minuteOfDay >= start && minuteOfDay < end
	}
	// Overnight wrap: inside if at/after start OR before end.
	return minuteOfDay >= start || minuteOfDay < end
}
