package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/logger"
)

// TimeEntry is one staff time-clock punch: clock-in opens it, clock-out closes
// it (status → pending_review), a manager approves it (→ approved). Approved
// entries feed the labor-cost ACTUALS lens (display-only, never payroll). Worked
// minutes are computed = (clock_out_at - clock_in_at)/min - break_minutes, never
// stored. No money fields live on this row (pay rate stays owner-only on
// staff_positions). Genesis-safety: struct tags match the 000106 DDL and the
// model is registered in db_config.go autoMigrate.
type TimeEntry struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// The two composite indexes mirror 000106 exactly
	// (idx_time_entries_biz_staff_clockin, idx_time_entries_biz_status) so a
	// genesis fresh DB matches a migrated one. business_id leads BOTH (tenant
	// boundary): the per-staff timesheet read + open-entry precondition ride the
	// first, the manager review queue rides the second. No standalone
	// single-column index is needed.
	BusinessID        uint       `gorm:"not null;index:idx_time_entries_biz_staff_clockin,priority:1;index:idx_time_entries_biz_status,priority:1" json:"business_id"`
	StaffID           uint       `gorm:"not null;index:idx_time_entries_biz_staff_clockin,priority:2" json:"staff_id"`
	ShiftID           *uint      `json:"shift_id"`
	ClockInAt         time.Time  `gorm:"not null;index:idx_time_entries_biz_staff_clockin,priority:3" json:"clock_in_at"`
	ClockOutAt        *time.Time `json:"clock_out_at"`
	BreakMinutes      int        `gorm:"not null;default:0" json:"break_minutes"`
	Source            string     `gorm:"not null;default:'staff_punch'" json:"source"`                                       // staff_punch|manager_manual
	Status            string     `gorm:"not null;default:'open';index:idx_time_entries_biz_status,priority:2" json:"status"` // open|pending_review|approved
	ApprovedByStaffID *uint      `json:"approved_by_staff_id"`
	Note              string     `json:"note"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (TimeEntry) TableName() string { return "time_entries" }

// Time-entry source + status enums (contracts §3).
const (
	TimeEntrySourceStaffPunch    = "staff_punch"
	TimeEntrySourceManagerManual = "manager_manual"

	TimeEntryStatusOpen          = "open"
	TimeEntryStatusPendingReview = "pending_review"
	TimeEntryStatusApproved      = "approved"
	// TimeEntryStatusRejected is a manager-rejected entry returned to the
	// staffer's attention. It is a terminal-until-edited state: a rejected entry
	// is NOT approvable (it must be edited back to pending or re-submitted) and it
	// is excluded from the approved-actuals aggregate. Added as a string status
	// value only — no schema change (the status column already accepts any string).
	TimeEntryStatusRejected = "rejected"
)

// RBACActionTimeEntry* are the audit Actions recorded for time-entry lifecycle
// transitions (contracts §9). Defined alongside the model so the slice owns its
// own audit vocabulary without touching models.go. The reject/edit actions carry
// their accountability (reason / original values) in the audit Reason field
// because the time_entries row has no dedicated audit columns.
const (
	RBACActionTimeEntryApproved RBACAction = "timeentry_approved"
	RBACActionTimeEntryRejected RBACAction = "timeentry_rejected"
	RBACActionTimeEntryEdited   RBACAction = "timeentry_edited"
)

// ErrAlreadyClockedIn is the ClockIn precondition guard: a staff member with an
// already-open entry cannot open a second one.
var ErrAlreadyClockedIn = errors.New("staff already has an open time entry")

// ErrNotClockedIn is returned by ClockOut/AddBreak when the staff member has no
// open entry to close or add a break to.
var ErrNotClockedIn = errors.New("staff has no open time entry")

// ErrInvalidBreak is returned when a break delta is negative.
var ErrInvalidBreak = errors.New("break minutes must be >= 0")

// ErrBreakExceedsElapsed is returned when accumulated break minutes would exceed
// the elapsed clocked time (you cannot break for longer than you have worked).
var ErrBreakExceedsElapsed = errors.New("break minutes exceed elapsed time")

// ErrTimeEntryNotFound is returned when a time entry does not exist within the
// caller's business (tenant-scoped lookups). Cross-tenant access is
// indistinguishable from a missing row.
var ErrTimeEntryNotFound = errors.New("time entry not found")

// ErrTimeEntryNotPendingReview is the CAS guard for ApproveTimeEntry: approving
// an entry that is not pending_review (still open, already approved, or a lost
// race) affects zero rows and returns this — the illegal-transition rejection.
var ErrTimeEntryNotPendingReview = errors.New("time entry is not pending review")

// ErrTimeEntryInvalidRange is returned when a manual entry's clock-out is missing
// or not strictly after its clock-in.
var ErrTimeEntryInvalidRange = errors.New("clock_out must be after clock_in")

// ErrTimeEntryOverlap is returned when a manual entry window collides with an
// existing non-rejected entry (open, pending_review, or approved) for the same
// staff — preventing double-counted labor actuals.
var ErrTimeEntryOverlap = errors.New("time entry overlaps an existing entry for this staff member")

// ErrTimeEntryOpenPunchBlocksManual is returned when a manager tries to file a
// manual entry while the staff member still has an open punch.
var ErrTimeEntryOpenPunchBlocksManual = errors.New("staff has an open punch; clock out before filing a manual entry")

// ErrTimeEntryNotEditable is the guard for EditTimeEntry: an already-approved
// entry is locked (editing it would silently change accepted actuals), so an edit
// affects zero rows and returns this.
var ErrTimeEntryNotEditable = errors.New("time entry is not editable (already approved)")

// WorkedMinutes computes the billable minutes for an entry:
// (clock_out - clock_in)/min - break_minutes, clamped at 0 and 0 while the entry
// is still open (no clock-out). This is the single source of truth for the
// worked-minutes math — the SQL aggregate GetApprovedWorkedMinutes mirrors it,
// and handlers reuse it for display (DRY).
func WorkedMinutes(e TimeEntry) int {
	if e.ClockOutAt == nil {
		return 0
	}
	mins := int(e.ClockOutAt.Sub(e.ClockInAt).Minutes()) - e.BreakMinutes
	if mins < 0 {
		return 0
	}
	return mins
}

// findOpenEntry loads the staff member's current open entry (there is at most
// one by the ClockIn precondition), or ErrNotClockedIn.
func (d *DB) findOpenEntry(businessID, staffID uint) (*TimeEntry, error) {
	var e TimeEntry
	if err := d.GetGorm().
		Where("business_id = ? AND staff_id = ? AND status = ?", businessID, staffID, TimeEntryStatusOpen).
		Order("clock_in_at desc, id desc").
		First(&e).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotClockedIn
		}
		return nil, err
	}
	return &e, nil
}

// ClockIn opens a fresh staff-punch entry for the caller's own staff_id. It
// rejects with ErrAlreadyClockedIn if an open entry already exists. shiftID is
// the optional shift the punch is associated with (stored as-is). No money fields.
//
// Atomicity: the genesis schema has a partial unique index
// (business_id, staff_id) WHERE status='open'. Create maps unique-violation to
// ErrAlreadyClockedIn so concurrent kiosk+phone punches cannot leave two opens.
func (d *DB) ClockIn(businessID, staffID uint, shiftID *uint) (*TimeEntry, error) {
	// Fast path: avoid unique-violation noise when already open.
	var open int64
	if err := d.GetGorm().Model(&TimeEntry{}).
		Where("business_id = ? AND staff_id = ? AND status = ?", businessID, staffID, TimeEntryStatusOpen).
		Count(&open).Error; err != nil {
		return nil, err
	}
	if open > 0 {
		return nil, ErrAlreadyClockedIn
	}
	e := &TimeEntry{
		BusinessID: businessID,
		StaffID:    staffID,
		ShiftID:    shiftID,
		ClockInAt:  time.Now().UTC(),
		Source:     TimeEntrySourceStaffPunch,
		Status:     TimeEntryStatusOpen,
	}
	if err := d.GetGorm().Create(e).Error; err != nil {
		if IsUniqueConstraintError(err) {
			return nil, ErrAlreadyClockedIn
		}
		return nil, err
	}
	return e, nil
}

// ClockOut closes the caller's open entry, stamping clock_out_at and moving it to
// pending_review. The close is a CAS guarded on status='open' so a double
// clock-out (or a lost race) affects zero rows and returns ErrNotClockedIn.
func (d *DB) ClockOut(businessID, staffID uint) (*TimeEntry, error) {
	e, err := d.findOpenEntry(businessID, staffID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	res := d.GetGorm().Model(&TimeEntry{}).
		Where("id = ? AND business_id = ? AND status = ?", e.ID, businessID, TimeEntryStatusOpen).
		Updates(map[string]interface{}{
			"clock_out_at": now,
			"status":       TimeEntryStatusPendingReview,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, ErrNotClockedIn
	}
	return d.getTimeEntry(businessID, e.ID)
}

// AddBreak adds minutes to the caller's open entry's break_minutes. The delta
// must be >= 0 and accumulated break may not exceed the minutes elapsed since
// clock-in (you cannot break longer than you have been clocked in).
func (d *DB) AddBreak(businessID, staffID uint, minutes int) (*TimeEntry, error) {
	if minutes < 0 {
		return nil, ErrInvalidBreak
	}
	e, err := d.findOpenEntry(businessID, staffID)
	if err != nil {
		return nil, err
	}
	elapsed := int(time.Now().UTC().Sub(e.ClockInAt).Minutes())
	if e.BreakMinutes+minutes > elapsed {
		return nil, ErrBreakExceedsElapsed
	}
	res := d.GetGorm().Model(&TimeEntry{}).
		Where("id = ? AND business_id = ? AND status = ?", e.ID, businessID, TimeEntryStatusOpen).
		Update("break_minutes", e.BreakMinutes+minutes)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, ErrNotClockedIn
	}
	return d.getTimeEntry(businessID, e.ID)
}

// getTimeEntry reloads one entry tenant-guarded, or ErrTimeEntryNotFound.
func (d *DB) getTimeEntry(businessID, entryID uint) (*TimeEntry, error) {
	var e TimeEntry
	if err := d.GetGorm().
		Where("id = ? AND business_id = ?", entryID, businessID).
		First(&e).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTimeEntryNotFound
		}
		return nil, err
	}
	return &e, nil
}

// timeEntryColumns is the explicit projection the timesheet/review reads use —
// every display column, selected explicitly to keep the reads off SELECT *
// (access-shape gate). No money columns exist on this row.
var timeEntryColumns = []string{
	"id", "business_id", "staff_id", "shift_id", "clock_in_at", "clock_out_at",
	"break_minutes", "source", "status", "approved_by_staff_id", "note",
	"created_at", "updated_at",
}

// timeEntryListLimit bounds the timesheet / review queue result set.
const timeEntryListLimit = 500

// ListTimesheet returns a staff member's own entries, newest first, over the
// (business_id, staff_id, clock_in_at) index. Tenant + row scoped in SQL.
func (d *DB) ListTimesheet(businessID, staffID uint) ([]TimeEntry, error) {
	var out []TimeEntry
	err := d.GetGorm().
		Select(timeEntryColumns).
		Where("business_id = ? AND staff_id = ?", businessID, staffID).
		Order("clock_in_at desc, id desc").
		Limit(timeEntryListLimit).
		Find(&out).Error
	return out, err
}

// ListTimesheetRange returns a staff member's own entries in the half-open
// window [from, to), optionally filtered by status, newest first — over the
// (business_id, staff_id, clock_in_at) index. Explicit projection, single query,
// bounded. Tenant + row scoped. Money-free (no rate join).
func (d *DB) ListTimesheetRange(businessID, staffID uint, from, to time.Time, status string) ([]TimeEntry, error) {
	q := d.GetGorm().
		Select(timeEntryColumns).
		Where("business_id = ? AND staff_id = ? AND clock_in_at >= ? AND clock_in_at < ?", businessID, staffID, from, to)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var out []TimeEntry
	err := q.Order("clock_in_at desc, id desc").Limit(timeEntryListLimit).Find(&out).Error
	return out, err
}

// ListForReview returns the manager review queue for a business. status=""
// defaults to the pending_review queue; an explicit status filters via the
// (business_id, status) index. One bounded query over an explicit projection.
func (d *DB) ListForReview(businessID uint, status string) ([]TimeEntry, error) {
	if status == "" {
		status = TimeEntryStatusPendingReview
	}
	var out []TimeEntry
	err := d.GetGorm().
		Select(timeEntryColumns).
		Where("business_id = ? AND status = ?", businessID, status).
		Order("clock_in_at desc, id desc").
		Limit(timeEntryListLimit).
		Find(&out).Error
	return out, err
}

// ListForReviewPaged is the paginated manager review read (BE-first extension of
// ListForReview): it returns a bounded page [offset, offset+limit) plus the TOTAL
// count of matching rows so the UI can show an honest "showing X of N". An empty
// status defaults to pending_review. A non-zero from/to bounds the read to the
// half-open clock-in window [from, to) (either bound may be zero to leave that
// side open). It is a COUNT + one bounded projected SELECT — never N+1. limit is
// clamped to (0, 500]; offset is floored at 0.
func (d *DB) ListForReviewPaged(businessID uint, status string, from, to time.Time, offset, limit int) ([]TimeEntry, int64, error) {
	if status == "" {
		status = TimeEntryStatusPendingReview
	}
	if limit <= 0 || limit > timeEntryListLimit {
		limit = timeEntryListLimit
	}
	if offset < 0 {
		offset = 0
	}
	base := d.GetGorm().Model(&TimeEntry{}).
		Where("business_id = ? AND status = ?", businessID, status)
	if !from.IsZero() {
		base = base.Where("clock_in_at >= ?", from.UTC())
	}
	if !to.IsZero() {
		base = base.Where("clock_in_at < ?", to.UTC())
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var out []TimeEntry
	// Rebuild the filtered query for the page read (Count consumes the statement).
	page := d.GetGorm().
		Select(timeEntryColumns).
		Where("business_id = ? AND status = ?", businessID, status)
	if !from.IsZero() {
		page = page.Where("clock_in_at >= ?", from.UTC())
	}
	if !to.IsZero() {
		page = page.Where("clock_in_at < ?", to.UTC())
	}
	err := page.Order("clock_in_at desc, id desc").
		Offset(offset).Limit(limit).
		Find(&out).Error
	return out, total, err
}

// ListEntriesForDay returns the time entries relevant to a manager's live-floor
// board for the day window [dayStart, dayEnd): every currently-open punch
// (status='open', regardless of when it opened — an overnight punch is still on
// the floor) plus every punch that clocked in during the day (closed or open).
// One bounded query over the explicit projection (no SELECT *, no money join),
// tenant-scoped in SQL. Feeds computeLiveFloor.
func (d *DB) ListEntriesForDay(businessID uint, dayStart, dayEnd time.Time) ([]TimeEntry, error) {
	var out []TimeEntry
	err := d.GetGorm().
		Select(timeEntryColumns).
		Where("business_id = ? AND (status = ? OR (clock_in_at >= ? AND clock_in_at < ?))",
			businessID, TimeEntryStatusOpen, dayStart, dayEnd).
		Order("clock_in_at asc, id asc").
		Limit(timeEntryListLimit).
		Find(&out).Error
	return out, err
}

// ApproveTimeEntry moves a pending_review entry to approved and records the
// approver. A tenant-scoped existence check runs first (cross-tenant/unknown ->
// ErrTimeEntryNotFound). The approval is a CAS guarded on status='pending_review'
// inside a transaction: RowsAffected != 1 means the entry was not pending
// (illegal transition) -> ErrTimeEntryNotPendingReview. The same transaction
// writes a best-effort RBACAuditLog row (contracts §9) — an audit failure is
// logged but never blocks the approval. Returns the reloaded entry.
func (d *DB) ApproveTimeEntry(businessID, entryID, byStaffID uint) (*TimeEntry, error) {
	existing, err := d.getTimeEntry(businessID, entryID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	err = d.GetGorm().Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&TimeEntry{}).
			Where("id = ? AND business_id = ? AND status = ?", entryID, businessID, TimeEntryStatusPendingReview).
			Updates(map[string]interface{}{
				"status":               TimeEntryStatusApproved,
				"approved_by_staff_id": byStaffID,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrTimeEntryNotPendingReview
		}
		audit := RBACAuditLog{
			StaffID:    existing.StaffID,
			BusinessID: businessID,
			Action:     RBACActionTimeEntryApproved,
			ChangedBy:  fmt.Sprintf("staff:%d", byStaffID),
			Reason:     fmt.Sprintf("timeentry approved: entry %d staff %d", entryID, existing.StaffID),
			CreatedAt:  now,
		}
		if aerr := tx.Create(&audit).Error; aerr != nil {
			logger.Logger.Warnf("time-entry approve audit write failed (entry %d): %v", entryID, aerr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return d.getTimeEntry(businessID, entryID)
}

// RejectTimeEntry moves a pending_review entry to rejected and returns it to the
// staffer's attention, recording the manager's reason on the entry Note (the
// model has no dedicated rejection-reason column). The transition is a CAS
// guarded on status='pending_review' inside a transaction: RowsAffected != 1
// means the entry was not pending (illegal transition) -> ErrTimeEntryNotPending-
// Review. The same transaction writes a best-effort RBACAuditLog row capturing
// who rejected it and why. Returns the reloaded entry.
func (d *DB) RejectTimeEntry(businessID, entryID, byStaffID uint, reason string) (*TimeEntry, error) {
	existing, err := d.getTimeEntry(businessID, entryID)
	if err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	now := time.Now().UTC()
	// Preserve any prior note, appending the rejection reason for the staffer.
	newNote := existing.Note
	if reason != "" {
		if strings.TrimSpace(newNote) != "" {
			newNote = newNote + "\n" + "Rejected: " + reason
		} else {
			newNote = "Rejected: " + reason
		}
	}
	err = d.GetGorm().Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&TimeEntry{}).
			Where("id = ? AND business_id = ? AND status = ?", entryID, businessID, TimeEntryStatusPendingReview).
			Updates(map[string]interface{}{
				"status": TimeEntryStatusRejected,
				"note":   newNote,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrTimeEntryNotPendingReview
		}
		audit := RBACAuditLog{
			StaffID:    existing.StaffID,
			BusinessID: businessID,
			Action:     RBACActionTimeEntryRejected,
			ChangedBy:  fmt.Sprintf("staff:%d", byStaffID),
			Reason:     fmt.Sprintf("timeentry rejected: entry %d staff %d reason=%q", entryID, existing.StaffID, reason),
			CreatedAt:  now,
		}
		if aerr := tx.Create(&audit).Error; aerr != nil {
			logger.Logger.Warnf("time-entry reject audit write failed (entry %d): %v", entryID, aerr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return d.getTimeEntry(businessID, entryID)
}

// EditTimeEntry lets a manager adjust a NON-approved entry's clock-in/out,
// break, and note (used to fix a mispunch before approving). The new range is
// validated (out strictly after in, break within elapsed) before any write. An
// already-approved entry is LOCKED (ErrTimeEntryNotEditable) so accepted actuals
// never silently change. Editing a rejected entry moves it back to
// pending_review so the corrected entry can be re-approved. The entry's ORIGINAL
// clock-in/out are preserved in a best-effort RBACAuditLog row (the model has no
// dedicated audit columns) — who edited what, and from what, stays accountable.
// Returns the reloaded entry.
func (d *DB) EditTimeEntry(businessID, entryID, byStaffID uint, clockIn time.Time, clockOut *time.Time, breakMinutes int, note string) (*TimeEntry, error) {
	existing, err := d.getTimeEntry(businessID, entryID)
	if err != nil {
		return nil, err
	}
	if existing.Status == TimeEntryStatusApproved {
		return nil, ErrTimeEntryNotEditable
	}
	if clockOut == nil || !clockOut.After(clockIn) {
		return nil, ErrTimeEntryInvalidRange
	}
	if breakMinutes < 0 {
		return nil, ErrInvalidBreak
	}
	if breakMinutes > int(clockOut.Sub(clockIn).Minutes()) {
		return nil, ErrBreakExceedsElapsed
	}
	origIn := existing.ClockInAt.UTC()
	origOut := "nil"
	if existing.ClockOutAt != nil {
		origOut = existing.ClockOutAt.UTC().Format(time.RFC3339)
	}
	now := time.Now().UTC()
	newNote := strings.TrimSpace(note)
	if newNote == "" {
		newNote = existing.Note
	}
	err = d.GetGorm().Transaction(func(tx *gorm.DB) error {
		// Guard on non-approved status inside the tx so a concurrent approval can't
		// be silently overwritten by an edit.
		res := tx.Model(&TimeEntry{}).
			Where("id = ? AND business_id = ? AND status <> ?", entryID, businessID, TimeEntryStatusApproved).
			Updates(map[string]interface{}{
				"clock_in_at":   clockIn.UTC(),
				"clock_out_at":  clockOut.UTC(),
				"break_minutes": breakMinutes,
				"note":          newNote,
				// A corrected entry becomes reviewable again (a rejected one returns
				// to pending; a pending one stays pending).
				"status": TimeEntryStatusPendingReview,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrTimeEntryNotEditable
		}
		audit := RBACAuditLog{
			StaffID:    existing.StaffID,
			BusinessID: businessID,
			Action:     RBACActionTimeEntryEdited,
			ChangedBy:  fmt.Sprintf("staff:%d", byStaffID),
			Reason: fmt.Sprintf("timeentry edited: entry %d staff %d original_in=%s original_out=%s",
				entryID, existing.StaffID, origIn.Format(time.RFC3339), origOut),
			CreatedAt: now,
		}
		if aerr := tx.Create(&audit).Error; aerr != nil {
			logger.Logger.Warnf("time-entry edit audit write failed (entry %d): %v", entryID, aerr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return d.getTimeEntry(businessID, entryID)
}

// CreateManualEntry inserts a manager-authored entry (source=manager_manual)
// straight into pending_review — used when a staff member forgot to punch. The
// staff member must be active in the business; clock_out must be present and after
// clock_in; break may not exceed the elapsed span (reject-before-persist).
// Also rejects when an open punch exists or any non-rejected entry overlaps
// [clockIn, clockOut) so GetApprovedWorkedMinutes cannot double-count labor.
func (d *DB) CreateManualEntry(businessID, staffID uint, shiftID *uint, clockIn time.Time, clockOut *time.Time, breakMinutes int, note string) (*TimeEntry, error) {
	if clockOut == nil || !clockOut.After(clockIn) {
		return nil, ErrTimeEntryInvalidRange
	}
	if breakMinutes < 0 {
		return nil, ErrInvalidBreak
	}
	if breakMinutes > int(clockOut.Sub(clockIn).Minutes()) {
		return nil, ErrBreakExceedsElapsed
	}
	if err := d.assertActiveStaffInBusiness(businessID, staffID); err != nil {
		return nil, err
	}
	// Open punch blocks manual entry for the same staff (would leave a second
	// labor window once both close/approve).
	var open int64
	if err := d.GetGorm().Model(&TimeEntry{}).
		Where("business_id = ? AND staff_id = ? AND status = ?", businessID, staffID, TimeEntryStatusOpen).
		Count(&open).Error; err != nil {
		return nil, err
	}
	if open > 0 {
		return nil, ErrTimeEntryOpenPunchBlocksManual
	}
	// Interval overlap: existing.start < new.end AND (existing.end IS NULL OR existing.end > new.start)
	// for non-rejected statuses. Open rows have null end and are already blocked above;
	// still include them here for belt-and-braces if the open check races.
	inUTC := clockIn.UTC()
	outUTC := clockOut.UTC()
	var overlapping int64
	if err := d.GetGorm().Model(&TimeEntry{}).
		Where("business_id = ? AND staff_id = ? AND status IN ?", businessID, staffID,
			[]string{TimeEntryStatusOpen, TimeEntryStatusPendingReview, TimeEntryStatusApproved}).
		Where("clock_in_at < ? AND (clock_out_at IS NULL OR clock_out_at > ?)", outUTC, inUTC).
		Count(&overlapping).Error; err != nil {
		return nil, err
	}
	if overlapping > 0 {
		return nil, ErrTimeEntryOverlap
	}
	e := &TimeEntry{
		BusinessID:   businessID,
		StaffID:      staffID,
		ShiftID:      shiftID,
		ClockInAt:    inUTC,
		ClockOutAt:   &outUTC,
		BreakMinutes: breakMinutes,
		Source:       TimeEntrySourceManagerManual,
		Status:       TimeEntryStatusPendingReview,
		Note:         note,
	}
	if err := d.GetGorm().Create(e).Error; err != nil {
		return nil, err
	}
	return e, nil
}

// StaffWorkedMinutes is the per-staff actuals aggregate the labor-cost ACTUALS
// lens consumes: the SUM of approved worked-minutes in a window plus that
// staff's primary StaffPosition pay rate (owner-only — RateCents is json:"-"
// so it never leaks on the wire; the labor handler decides whether to surface a
// derived dollar figure, owner/financial:read-gated). Lives in the database
// package (like PayrollRunCost) so the labor package can name it with no cycle.
type StaffWorkedMinutes struct {
	StaffID   uint  `json:"staff_id"`
	Minutes   int   `json:"minutes"`
	RateCents int64 `json:"-"`
}

// GetApprovedWorkedMinutes sums APPROVED, clocked-out time-entry worked minutes
// per staff over [start,end) and joins each staff's PRIMARY position pay rate —
// in ONE GROUP BY aggregate (not per-row hydration in Go). The per-entry minute
// expression mirrors the WorkedMinutes helper exactly:
// floor((clock_out - clock_in)/60) - break_minutes, summed per staff. The
// LEFT JOIN keeps staff with approved hours but no primary position (rate 0).
// Dialect-aware (SQLite strftime vs Postgres EXTRACT EPOCH), mirroring
// DeliveryService.secondsBetweenExpression. Pending/open and out-of-window
// entries are excluded by the WHERE.
func (d *DB) GetApprovedWorkedMinutes(businessID uint, start, end time.Time) ([]StaffWorkedMinutes, error) {
	// floored whole minutes between clock_in and clock_out, per entry.
	minutesExpr := "((CAST(strftime('%s', te.clock_out_at) AS INTEGER) - CAST(strftime('%s', te.clock_in_at) AS INTEGER)) / 60)"
	if d.GetGorm().Name() == "postgres" {
		minutesExpr = "FLOOR(EXTRACT(EPOCH FROM (te.clock_out_at - te.clock_in_at)) / 60)"
	}

	var rows []StaffWorkedMinutes
	err := d.GetGorm().
		Table("time_entries AS te").
		Select("te.staff_id AS staff_id, SUM("+minutesExpr+") - SUM(te.break_minutes) AS minutes, COALESCE(MAX(sp.pay_rate_cents), 0) AS rate_cents").
		Joins("LEFT JOIN staff_positions sp ON sp.business_id = te.business_id AND sp.staff_id = te.staff_id AND sp.is_primary = ?", true).
		Where("te.business_id = ? AND te.status = ? AND te.clock_out_at IS NOT NULL AND te.clock_in_at >= ? AND te.clock_in_at < ?",
			businessID, TimeEntryStatusApproved, start.UTC(), end.UTC()).
		Group("te.staff_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("GetApprovedWorkedMinutes businessID=%d: %w", businessID, err)
	}
	return rows, nil
}
