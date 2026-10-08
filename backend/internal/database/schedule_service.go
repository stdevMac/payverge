package database

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Schedule is a per-business weekly schedule. Exactly one row per
// (business_id, week_start); the draft is created lazily and flipped to
// published in a single transaction (see PublishSchedule). No money fields.
// Struct tags match the genesis schedules table.
type Schedule struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Composite UNIQUE (business_id, week_start) — must match idx_schedules_biz_week
	// in 000104 so a force-baselined fresh DB enforces one schedule per week and
	// GetOrCreateDraftSchedule's create-race recovery works. The composite covers
	// business_id-leading lookups, so no standalone single-column index is needed.
	BusinessID         uint       `gorm:"not null;uniqueIndex:idx_schedules_biz_week,priority:1" json:"business_id"`
	WeekStart          time.Time  `gorm:"not null;uniqueIndex:idx_schedules_biz_week,priority:2" json:"week_start"` // date, business-TZ week start
	Status             string     `gorm:"not null;default:'draft'" json:"status"`                                   // draft|published
	PublishedAt        *time.Time `json:"published_at"`
	PublishedByStaffID *uint      `json:"published_by_staff_id"`
	Notes              string     `json:"notes"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (Schedule) TableName() string { return "schedules" }

// Shift is a single scheduled shift within a Schedule. StaffID NULL = an open
// (unassigned) shift. RemindedAt is the atomic-claim column the shift-reminder
// scheduler (Slice 2.6) stamps so a reminder is enqueued at most once per shift,
// multi-replica safe. No money fields. Genesis-safety: tags match 000104 DDL.
type Shift struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// The three composite indexes mirror 000104 exactly (idx_shifts_biz_schedule,
	// idx_shifts_biz_staff_starts, idx_shifts_biz_status) so a genesis fresh DB
	// matches a migrated one. business_id leads all three (tenant boundary).
	BusinessID       uint       `gorm:"not null;index:idx_shifts_biz_schedule,priority:1;index:idx_shifts_biz_staff_starts,priority:1;index:idx_shifts_biz_status,priority:1" json:"business_id"`
	ScheduleID       uint       `gorm:"not null;index:idx_shifts_biz_schedule,priority:2" json:"schedule_id"`
	StaffID          *uint      `gorm:"index:idx_shifts_biz_staff_starts,priority:2" json:"staff_id"` // NULL = open shift
	PositionID       uint       `gorm:"not null" json:"position_id"`
	StartsAt         time.Time  `gorm:"not null;index:idx_shifts_biz_staff_starts,priority:3" json:"starts_at"`
	EndsAt           time.Time  `gorm:"not null" json:"ends_at"`
	BreakMinutes     int        `gorm:"not null;default:0" json:"break_minutes"`
	Status           string     `gorm:"not null;default:'private_draft';index:idx_shifts_biz_status,priority:2" json:"status"` // private_draft|open|filled|locked
	Published        bool       `gorm:"not null;default:false" json:"published"`
	Notes            string     `json:"notes"`
	CreatedByStaffID uint       `gorm:"not null" json:"created_by_staff_id"`
	RemindedAt       *time.Time `json:"reminded_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (Shift) TableName() string { return "shifts" }

// Schedule + shift status enums (contracts §3).
const (
	ScheduleStatusDraft     = "draft"
	ScheduleStatusPublished = "published"

	ShiftStatusPrivateDraft = "private_draft"
	ShiftStatusOpen         = "open"
	ShiftStatusFilled       = "filled"
	ShiftStatusLocked       = "locked"
)

// ErrScheduleNotFound is returned when a schedule does not exist within the
// caller's business (tenant-scoped lookups).
var ErrScheduleNotFound = errors.New("schedule not found")

// ErrScheduleNotDraft is the idempotency guard for PublishSchedule: a second
// publish (or publishing a non-existent/cross-tenant schedule) affects zero
// draft rows and returns this so the handler can map it to a 409.
var ErrScheduleNotDraft = errors.New("schedule is not a draft")

// ErrShiftNotFound is returned when a shift does not exist within the caller's
// business (tenant-scoped lookups/mutations).
var ErrShiftNotFound = errors.New("shift not found")

// ErrShiftInvalidRange is returned when a shift's end is not strictly after its
// start.
var ErrShiftInvalidRange = errors.New("shift end must be after start")

// assertActiveStaffInBusiness errors unless the staff row belongs to the
// business AND is active. Assigning a shift to an inactive/cross-tenant staff
// member is rejected.
func (d *DB) assertActiveStaffInBusiness(businessID, staffID uint) error {
	var s Staff
	if err := d.GetGorm().
		Where("id = ? AND business_id = ? AND is_active = ?", staffID, businessID, true).
		First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStaffNotFound
		}
		return err
	}
	return nil
}

// NormalizeWeekStart returns UTC midnight of the calendar day t names in its OWN
// location — the canonical schedules.week_start value. week_start identifies a
// calendar week, not an instant, so the day must be read with t.Date() (never
// t.UTC().Date(), which walks a venue-local midnight east of Greenwich back into
// the previous day).
func NormalizeWeekStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// weekStartWindow is the exclusive (lo, hi) instant range that can hold the
// week_start row for the requested week. Readers build UTC midnight while some
// writers store venue-local midnight for the same calendar day, and every real
// UTC offset is inside (-24h, +24h); consecutive weeks are 7 days apart, so at
// most one week's row can fall in the window.
func weekStartWindow(weekStart time.Time) (time.Time, time.Time) {
	base := NormalizeWeekStart(weekStart)
	return base.Add(-24 * time.Hour), base.Add(24 * time.Hour)
}

// weekStartWindowScanLimit bounds the window read. Legitimate data holds one row
// per (business, week); the headroom only exists so a duplicated legacy row
// cannot turn the read unbounded.
const weekStartWindowScanLimit = 4

// findScheduleForWeek resolves the (business, week) schedule row, tenant-scoped
// and bounded. It returns (nil, nil) when the business has no schedule for that
// week.
//
// The canonical key is UTC midnight, and that is the only read the steady state
// pays for: one indexed LIMIT 1 on idx_schedules_biz_week, the same shape (and
// the same allocation profile) as before this function existed. Only when that
// misses does it fall back to a bounded window scan, which recovers a row whose
// week_start was written at venue-local midnight for the same calendar day.
// Exact-match-first also means a re-seeded canonical row is never shadowed by a
// leftover off-grid one.
func findScheduleForWeek(tx *gorm.DB, businessID uint, weekStart time.Time, publishedOnly bool) (*Schedule, error) {
	canonical := NormalizeWeekStart(weekStart)

	q := tx.Where("business_id = ? AND week_start = ?", businessID, canonical)
	if publishedOnly {
		q = q.Where("status = ?", ScheduleStatusPublished)
	}
	var sched Schedule
	err := q.First(&sched).Error
	if err == nil {
		return &sched, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	lo, hi := weekStartWindow(weekStart)
	wq := tx.Where("business_id = ? AND week_start > ? AND week_start < ?", businessID, lo, hi)
	if publishedOnly {
		wq = wq.Where("status = ?", ScheduleStatusPublished)
	}
	var rows []Schedule
	if err := wq.Order("week_start asc, id asc").Limit(weekStartWindowScanLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// GetScheduleForWeek loads a business's schedule for a given week-start plus its
// shifts. Two bounded, tenant-scoped queries (no Preload fan-out). A missing
// schedule is NOT an error: it returns (nil, nil, nil) so callers can render an
// empty/absent week.
func (d *DB) GetScheduleForWeek(businessID uint, weekStart time.Time) (*Schedule, []Shift, error) {
	sched, err := findScheduleForWeek(d.GetGorm(), businessID, weekStart, false)
	if err != nil {
		return nil, nil, err
	}
	if sched == nil {
		return nil, nil, nil
	}
	var shifts []Shift
	if err := d.GetGorm().
		Where("business_id = ? AND schedule_id = ?", businessID, sched.ID).
		Order("starts_at asc, id asc").
		Find(&shifts).Error; err != nil {
		return nil, nil, err
	}
	return sched, shifts, nil
}

// GetOrCreateDraftSchedule returns the schedule for (business, week-start),
// lazily creating a draft on first access. Tenant-scoped; idempotent (the
// unique (business_id, week_start) index plus a re-read guarantees no duplicate
// row under the single-writer admin path).
func (d *DB) GetOrCreateDraftSchedule(businessID uint, weekStart time.Time, byStaffID uint) (*Schedule, error) {
	existing, err := findScheduleForWeek(d.GetGorm(), businessID, weekStart, false)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	sched := Schedule{BusinessID: businessID, WeekStart: NormalizeWeekStart(weekStart), Status: ScheduleStatusDraft}
	if cErr := d.GetGorm().Create(&sched).Error; cErr != nil {
		// Lost a create race: re-read the row the winner inserted.
		if won, rErr := findScheduleForWeek(d.GetGorm(), businessID, weekStart, false); rErr == nil && won != nil {
			return won, nil
		}
		return nil, cErr
	}
	return &sched, nil
}

// validateShiftRange enforces end strictly after start.
func validateShiftRange(startsAt, endsAt time.Time) error {
	if !endsAt.After(startsAt) {
		return ErrShiftInvalidRange
	}
	return nil
}

// CreateShift inserts a shift after validating: end>start, the parent schedule
// belongs to the business, the position belongs to the business and is active,
// and (if assigned) the staff member is an active member of the business. Tenant
// boundary is the caller-set BusinessID. Returns ErrScheduleNotFound if the
// parent schedule is missing/cross-tenant.
//
// Post-publish visibility: a shift added to an ALREADY-published schedule is born
// published + status-resolved (assigned→filled, open→open) so it is immediately
// staff-visible (GetPublishedScheduleForStaff filters published=true) and the
// handler's shift.assigned SSE branch fires. Shifts in a draft keep the
// private_draft/published=false defaults until PublishSchedule flips them.
func (d *DB) CreateShift(sh *Shift) error {
	if err := validateShiftRange(sh.StartsAt, sh.EndsAt); err != nil {
		return err
	}
	sched, err := d.GetScheduleByID(sh.BusinessID, sh.ScheduleID)
	if err != nil {
		return err
	}
	if err := d.assertPositionInBusiness(sh.BusinessID, sh.PositionID); err != nil {
		return err
	}
	if sh.StaffID != nil {
		if err := d.assertActiveStaffInBusiness(sh.BusinessID, *sh.StaffID); err != nil {
			return err
		}
	}
	if sched.Status == ScheduleStatusPublished {
		sh.Published = true
		if sh.StaffID != nil {
			sh.Status = ShiftStatusFilled
		} else {
			sh.Status = ShiftStatusOpen
		}
	}
	return d.GetGorm().Create(sh).Error
}

// CopyWeekResult reports the outcome of a transactional week copy: how many
// shifts were created in the destination week and how many source shifts were
// skipped (only-empty-days mode, when their weekday already had a shift in the
// destination).
type CopyWeekResult struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
}

// CopyScheduleWeek copies every shift from fromWeek into toWeek in ONE
// transaction — the server-side replacement for the client's N serial per-shift
// POSTs (which lost the copy button on any partial failure). Each source shift's
// start/end is shifted by the exact fromWeek→toWeek delta (a whole number of
// days, preserving the local time-of-day across the week boundary), and its
// staff/position/break/notes are preserved. The destination draft is
// get-or-created. Copied shifts inherit the fresh-shift defaults
// (private_draft/unpublished) so they are staged in a draft until the operator
// publishes — the copy never silently goes live.
//
// When onlyEmptyDays is true, a source shift is SKIPPED if the destination
// already has any shift on that weekday (the "copy into empty days" idempotent
// path): re-running is safe and never duplicates. Both weeks are floored to their
// UTC calendar day by the handler before this is called; the delta is computed
// from those floors so it is a clean multiple of 24h.
func (d *DB) CopyScheduleWeek(businessID uint, fromWeek, toWeek time.Time, byStaffID uint, onlyEmptyDays bool) (*CopyWeekResult, error) {
	result := &CopyWeekResult{}
	fromWeek = NormalizeWeekStart(fromWeek)
	toWeek = NormalizeWeekStart(toWeek)
	delta := toWeek.Sub(fromWeek)

	// Source shifts (tenant-scoped read; a missing source schedule yields none).
	_, srcShifts, err := d.GetScheduleForWeek(businessID, fromWeek)
	if err != nil {
		return nil, err
	}
	if len(srcShifts) == 0 {
		return result, nil
	}

	err = d.GetGorm().Transaction(func(tx *gorm.DB) error {
		// Get-or-create the destination draft inside the tx so the copy is atomic.
		found, derr := findScheduleForWeek(tx, businessID, toWeek, false)
		if derr != nil {
			return derr
		}
		dest := Schedule{BusinessID: businessID, WeekStart: toWeek, Status: ScheduleStatusDraft}
		if found != nil {
			dest = *found
		} else if cErr := tx.Create(&dest).Error; cErr != nil {
			return cErr
		}

		// Which destination weekdays already have a shift (only-empty-days mode).
		filledDays := map[int]bool{}
		if onlyEmptyDays {
			var existing []Shift
			if lerr := tx.Select("starts_at").
				Where("business_id = ? AND schedule_id = ?", businessID, dest.ID).
				Find(&existing).Error; lerr != nil {
				return lerr
			}
			for _, s := range existing {
				filledDays[dayOfWeekUTC(s.StartsAt)] = true
			}
		}

		toCreate := make([]Shift, 0, len(srcShifts))
		for _, s := range srcShifts {
			newStart := s.StartsAt.Add(delta)
			if onlyEmptyDays && filledDays[dayOfWeekUTC(newStart)] {
				result.Skipped++
				continue
			}
			toCreate = append(toCreate, Shift{
				BusinessID:       businessID,
				ScheduleID:       dest.ID,
				StaffID:          s.StaffID,
				PositionID:       s.PositionID,
				StartsAt:         newStart,
				EndsAt:           s.EndsAt.Add(delta),
				BreakMinutes:     s.BreakMinutes,
				Notes:            s.Notes,
				Status:           ShiftStatusPrivateDraft,
				Published:        false,
				CreatedByStaffID: byStaffID,
			})
		}
		if len(toCreate) == 0 {
			return nil
		}
		if cErr := tx.Create(&toCreate).Error; cErr != nil {
			return cErr
		}
		result.Created = len(toCreate)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// dayOfWeekUTC returns the weekday (0=Sunday..6=Saturday) of t in UTC — the
// grouping key for the only-empty-days copy filter.
func dayOfWeekUTC(t time.Time) int {
	return int(t.UTC().Weekday())
}

// shiftUpdatable whitelists the columns UpdateShiftFields may patch. Identity,
// tenant, lifecycle (published/status flip happens via PublishSchedule), the
// reminder claim, and timestamps are never caller-writable.
var shiftUpdatable = map[string]struct{}{
	"staff_id":      {},
	"position_id":   {},
	"starts_at":     {},
	"ends_at":       {},
	"break_minutes": {},
	"notes":         {},
}

// UpdateShiftFields applies a whitelisted field map to a shift, tenant-scoped by
// business_id. It loads the current row first so a partial start/end update is
// validated against the effective (merged) range, and re-validates a changed
// position/staff against the business. A staged nil staff_id unassigns the shift
// (SQL NULL). Returns ErrShiftNotFound if no row in businessID matches.
func (d *DB) UpdateShiftFields(businessID, shiftID uint, fields map[string]interface{}) error {
	var current Shift
	if err := d.GetGorm().
		Where("id = ? AND business_id = ?", shiftID, businessID).
		First(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrShiftNotFound
		}
		return err
	}

	clean := map[string]interface{}{}
	for k, v := range fields {
		if _, ok := shiftUpdatable[k]; ok {
			clean[k] = v
		}
	}
	if len(clean) == 0 {
		return nil
	}

	// Effective range after applying the proposed start/end.
	effStart, effEnd := current.StartsAt, current.EndsAt
	if v, ok := clean["starts_at"].(time.Time); ok {
		effStart = v
	}
	if v, ok := clean["ends_at"].(time.Time); ok {
		effEnd = v
	}
	if err := validateShiftRange(effStart, effEnd); err != nil {
		return err
	}

	if v, ok := clean["position_id"]; ok {
		posID, ok := toUintValue(v)
		if !ok {
			return ErrPositionNotFound
		}
		if err := d.assertPositionInBusiness(businessID, posID); err != nil {
			return err
		}
	}
	if v, ok := clean["staff_id"]; ok && v != nil {
		staffID, ok := toUintValue(v)
		if !ok {
			return ErrStaffNotFound
		}
		if err := d.assertActiveStaffInBusiness(businessID, staffID); err != nil {
			return err
		}
	}

	// Post-publish visibility: if the parent schedule is already published, keep
	// the edited shift published and re-resolve its status from the EFFECTIVE
	// assignment so an open↔assigned edit on a live week stays consistent and
	// staff-visible. These are server-derived (not caller-supplied) so they are
	// safe to add after the whitelist filter.
	if sched, err := d.GetScheduleByID(businessID, current.ScheduleID); err == nil && sched.Status == ScheduleStatusPublished {
		assigned := current.StaffID != nil
		if v, ok := clean["staff_id"]; ok {
			assigned = v != nil
		}
		clean["published"] = true
		if assigned {
			clean["status"] = ShiftStatusFilled
		} else {
			clean["status"] = ShiftStatusOpen
		}
	}

	// Reminder re-arm: if the assignment or start time actually CHANGES, the
	// already-sent reminder (if any) is stale — clear the reminded_at claim in
	// the SAME update so the scheduler re-reminds the right staffer for the new
	// time. A no-op edit (same staff/start) or an unrelated patch must NOT clear
	// the claim, or an edited-but-unchanged shift would double-remind.
	if shiftReminderResetNeeded(&current, clean) {
		clean["reminded_at"] = nil
	}

	res := d.GetGorm().Model(&Shift{}).
		Where("id = ? AND business_id = ?", shiftID, businessID).
		Updates(clean)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrShiftNotFound
	}
	return nil
}

// shiftReminderResetNeeded reports whether a whitelisted update actually changes
// the shift's assignment (staff_id) or start time (starts_at) relative to the
// loaded row — the two facts a sent reminder is keyed on. Only a real change
// re-arms the reminder; absent keys and same-value patches do not.
func shiftReminderResetNeeded(current *Shift, clean map[string]interface{}) bool {
	if v, ok := clean["staff_id"]; ok {
		if v == nil {
			if current.StaffID != nil {
				return true
			}
		} else if newID, okN := toUintValue(v); okN {
			if current.StaffID == nil || *current.StaffID != newID {
				return true
			}
		}
	}
	if v, ok := clean["starts_at"].(time.Time); ok && !v.Equal(current.StartsAt) {
		return true
	}
	return false
}

// toUintValue normalizes the numeric shapes a staff/position id can arrive as
// (handler-decoded JSON float64, a typed uint, or an int) to uint.
func toUintValue(v interface{}) (uint, bool) {
	switch n := v.(type) {
	case uint:
		return n, true
	case uint64:
		return uint(n), true
	case int:
		if n < 0 {
			return 0, false
		}
		return uint(n), true
	case int64:
		if n < 0 {
			return 0, false
		}
		return uint(n), true
	case float64:
		if n < 0 {
			return 0, false
		}
		return uint(n), true
	}
	return 0, false
}

// DeleteShift hard-deletes a shift, tenant-scoped. Returns ErrShiftNotFound if no
// row in businessID matches.
func (d *DB) DeleteShift(businessID, shiftID uint) error {
	res := d.GetGorm().
		Where("id = ? AND business_id = ?", shiftID, businessID).
		Delete(&Shift{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrShiftNotFound
	}
	return nil
}

// PublishSchedule flips a draft schedule to published in a single transaction:
// (1) a CAS update on status='draft' (RowsAffected!=1 -> ErrScheduleNotDraft,
// the idempotency guard against double-publish and cross-tenant publishes),
// then (2) publishes its shifts, splitting assigned->filled and open(NULL)->open.
// Returns the reloaded schedule + its now-published shifts for fan-out.
func (d *DB) PublishSchedule(businessID, scheduleID, byStaffID uint) (*Schedule, []Shift, error) {
	now := time.Now().UTC()
	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Schedule{}).
			Where("id = ? AND business_id = ? AND status = ?", scheduleID, businessID, ScheduleStatusDraft).
			Updates(map[string]interface{}{
				"status":                ScheduleStatusPublished,
				"published_at":          now,
				"published_by_staff_id": byStaffID,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrScheduleNotDraft
		}
		if err := tx.Model(&Shift{}).
			Where("schedule_id = ? AND business_id = ? AND staff_id IS NOT NULL", scheduleID, businessID).
			Updates(map[string]interface{}{"published": true, "status": ShiftStatusFilled}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Shift{}).
			Where("schedule_id = ? AND business_id = ? AND staff_id IS NULL", scheduleID, businessID).
			Updates(map[string]interface{}{"published": true, "status": ShiftStatusOpen}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	var sched Schedule
	if err := d.GetGorm().
		Where("id = ? AND business_id = ?", scheduleID, businessID).
		First(&sched).Error; err != nil {
		return nil, nil, err
	}
	var shifts []Shift
	if err := d.GetGorm().
		Where("business_id = ? AND schedule_id = ?", businessID, scheduleID).
		Order("starts_at asc, id asc").
		Find(&shifts).Error; err != nil {
		return nil, nil, err
	}
	return &sched, shifts, nil
}

// staffShiftColumns is the explicit projection a plain staff member's schedule
// view needs — every display column, but NOT the operator-only reminder claim
// (reminded_at) or audit (created_by_staff_id). Selecting columns explicitly
// keeps the staff read off SELECT * (access-shape gate).
var staffShiftColumns = []string{
	"id", "business_id", "schedule_id", "staff_id", "position_id",
	"starts_at", "ends_at", "break_minutes", "status", "published", "notes",
}

// GetPublishedScheduleForStaff is the row-scoped read for a plain staff member:
// it returns the week's schedule ONLY if it is published, and only that staff's
// own (assigned) shifts plus open (unassigned) shifts — never another person's
// assignment. The scoping is done in SQL (staff_id = ? OR staff_id IS NULL),
// bounded to the one week's schedule, with an explicit column projection. A
// missing/draft schedule returns (nil, nil, nil).
func (d *DB) GetPublishedScheduleForStaff(businessID uint, weekStart time.Time, staffID uint) (*Schedule, []Shift, error) {
	sched, err := findScheduleForWeek(d.GetGorm(), businessID, weekStart, true)
	if err != nil {
		return nil, nil, err
	}
	if sched == nil {
		return nil, nil, nil
	}
	var shifts []Shift
	if err := d.GetGorm().
		Select(staffShiftColumns).
		Where("business_id = ? AND schedule_id = ? AND published = ? AND (staff_id = ? OR staff_id IS NULL)",
			businessID, sched.ID, true, staffID).
		Order("starts_at asc, id asc").
		Find(&shifts).Error; err != nil {
		return nil, nil, err
	}
	return sched, shifts, nil
}

// liveFloorShiftLimit bounds the live-floor day scan. A single day of assigned,
// published shifts for one business is small (crew size), so 500 is generous
// headroom while keeping the read bounded (access-shape gate).
const liveFloorShiftLimit = 500

// GetActiveShiftsForDay returns the published, assigned (filled) shifts that
// overlap the half-open day window [dayStart, dayEnd) for a business — the
// manager live-floor scan. A shift overlaps the day if it starts before the
// window ends AND ends after the window starts, so an overnight shift that
// straddles midnight is still captured. Open (unassigned) shifts are excluded
// (nobody to be on the floor). One bounded query over the explicit staff-shift
// projection (no SELECT *, no operator-only reminded_at/created_by columns),
// ordered by start. Tenant-scoped in SQL. Money-free.
func (d *DB) GetActiveShiftsForDay(businessID uint, dayStart, dayEnd time.Time) ([]Shift, error) {
	var shifts []Shift
	err := d.GetGorm().
		Select(staffShiftColumns).
		Where("business_id = ? AND published = ? AND status = ? AND staff_id IS NOT NULL AND starts_at < ? AND ends_at > ?",
			businessID, true, ShiftStatusFilled, dayEnd, dayStart).
		Order("starts_at asc, id asc").
		Limit(liveFloorShiftLimit).
		Find(&shifts).Error
	return shifts, err
}

// GetScheduleByID loads one schedule, enforcing it belongs to businessID.
// Returns ErrScheduleNotFound if no row matches.
func (d *DB) GetScheduleByID(businessID, scheduleID uint) (*Schedule, error) {
	var sched Schedule
	if err := d.GetGorm().
		Where("id = ? AND business_id = ?", scheduleID, businessID).
		First(&sched).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrScheduleNotFound
		}
		return nil, err
	}
	return &sched, nil
}

// GetShiftByID loads one shift, enforcing it belongs to businessID. Returns
// ErrShiftNotFound if no row matches.
func (d *DB) GetShiftByID(businessID, shiftID uint) (*Shift, error) {
	var sh Shift
	if err := d.GetGorm().
		Where("id = ? AND business_id = ?", shiftID, businessID).
		First(&sh).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrShiftNotFound
		}
		return nil, err
	}
	return &sh, nil
}
