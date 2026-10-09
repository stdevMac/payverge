package database

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/logger"
)

// StaffAvailability is a single weekly availability window a staff member sets
// for themselves: a preferred or unavailable block on one weekday, expressed in
// minutes-from-midnight in the business timezone. A staff may hold several
// windows per weekday (the composite index is intentionally NON-unique). It
// feeds the scheduler's overlay; no money fields. Struct tags match the genesis
// staff_availabilities table.
type StaffAvailability struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// business_id leads the (business_id, staff_id, weekday) composite (tenant
	// boundary + the overlay lookup), so no standalone single-column index is
	// needed — the composite covers business_id-leading reads.
	BusinessID uint      `gorm:"not null;index:idx_staff_availabilities_biz_staff_weekday,priority:1" json:"business_id"`
	StaffID    uint      `gorm:"not null;index:idx_staff_availabilities_biz_staff_weekday,priority:2" json:"staff_id"`
	Weekday    int       `gorm:"not null;index:idx_staff_availabilities_biz_staff_weekday,priority:3" json:"weekday"` // 0-6 (Sunday..Saturday)
	StartMin   int       `gorm:"not null" json:"start_min"`                                                           // minutes from midnight, business-TZ
	EndMin     int       `gorm:"not null" json:"end_min"`
	Kind       string    `gorm:"not null" json:"kind"` // preferred|unavailable
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (StaffAvailability) TableName() string { return "staff_availabilities" }

// TimeOffRequest is a staff member's request to be off between two instants.
// It moves through the timeoff state machine (pending → approved|denied|
// cancelled) via DecideTimeOff's CAS guard. DecidedByStaffID/DecidedAt are set
// on the approve/deny transition. No money fields. Genesis-safety: tags match
// the 000105 DDL.
type TimeOffRequest struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// business_id leads BOTH composites (the manager queue keyed by status and
	// the per-staff "my requests" list), so no standalone single-column index is
	// needed.
	BusinessID       uint       `gorm:"not null;index:idx_time_off_requests_biz_status,priority:1;index:idx_time_off_requests_biz_staff,priority:1" json:"business_id"`
	StaffID          uint       `gorm:"not null;index:idx_time_off_requests_biz_staff,priority:2" json:"staff_id"`
	StartsAt         time.Time  `gorm:"not null" json:"starts_at"`
	EndsAt           time.Time  `gorm:"not null" json:"ends_at"`
	Reason           string     `json:"reason"`
	Status           string     `gorm:"not null;default:'pending';index:idx_time_off_requests_biz_status,priority:2" json:"status"` // pending|approved|denied|cancelled
	DecidedByStaffID *uint      `json:"decided_by_staff_id"`
	DecidedAt        *time.Time `json:"decided_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (TimeOffRequest) TableName() string { return "time_off_requests" }

// Availability + time-off status enums (contracts §3).
const (
	AvailabilityKindPreferred   = "preferred"
	AvailabilityKindUnavailable = "unavailable"

	TimeOffStatusPending   = "pending"
	TimeOffStatusApproved  = "approved"
	TimeOffStatusDenied    = "denied"
	TimeOffStatusCancelled = "cancelled"
)

// RBACActionTimeOffDecided is the audit Action recorded when a manager approves
// or denies a time-off request (contracts §9). Defined alongside the model so
// the slice owns its own audit vocabulary without touching models.go.
const RBACActionTimeOffDecided RBACAction = "timeoff_decided"

// ErrTimeOffNotFound is returned when a time-off request does not exist within
// the caller's business (tenant-scoped lookups/decisions). Cross-tenant access
// is indistinguishable from a missing row.
var ErrTimeOffNotFound = errors.New("time-off request not found")

// ErrTimeOffNotPending is the CAS guard for DecideTimeOff: deciding a request
// that is no longer pending (already approved/denied/cancelled, or a lost race)
// affects zero rows and returns this — the illegal-transition rejection.
var ErrTimeOffNotPending = errors.New("time-off request is not pending")

// ErrTimeOffInvalidRange is returned when a time-off request's end is not
// strictly after its start.
var ErrTimeOffInvalidRange = errors.New("time-off end must be after start")

// ErrTimeOffExpired is returned when a still-pending request's ends_at is
// already in the past. Normal Approve/Deny must fail closed after expiry.
var ErrTimeOffExpired = errors.New("time-off request has expired")

// InstantElapsed reports whether endsAt is a real instant strictly before now.
// A zero endsAt is treated as unknown (not elapsed) so fixtures without times
// do not fail closed.
func InstantElapsed(endsAt, now time.Time) bool {
	return !endsAt.IsZero() && endsAt.Before(now)
}

// ErrAvailabilityInvalid is returned when an availability window fails
// validation (weekday out of 0..6, minutes out of 0 ≤ start < end ≤ 1440, or an
// unrecognized kind). It is a reject-before-persist guard: a batch with any
// invalid window writes nothing.
var ErrAvailabilityInvalid = errors.New("invalid availability window")

// GetAvailability returns a staff member's own availability windows, ordered for
// stable rendering. Tenant-scoped by business_id and the caller's staff_id.
func (d *DB) GetAvailability(businessID, staffID uint) ([]StaffAvailability, error) {
	var out []StaffAvailability
	if err := d.GetGorm().
		Where("business_id = ? AND staff_id = ?", businessID, staffID).
		Order("weekday asc, start_min asc, id asc").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// teamAvailabilityColumns is the explicit projection the manager team-overlay
// read uses — every window column, selected explicitly so the overlay read stays
// off SELECT * (access-shape gate). No money fields exist on this row.
var teamAvailabilityColumns = []string{
	"id", "business_id", "staff_id", "weekday", "start_min", "end_min", "kind", "created_at", "updated_at",
}

// teamAvailabilityLimit bounds the manager overlay read. Availability is a small
// recurring set per staff (a handful of windows × up to 7 weekdays), so even a
// large roster stays well under this cap; it is the same defensive bound the
// time-off list carries.
const teamAvailabilityLimit = 5000

// GetTeamAvailability returns every staff member's recurring availability windows
// for a business, grouped by staff_id, for the manager's schedule-builder
// overlay. Availability is a per-weekday recurring pattern (not date-scoped), so
// no date range is needed — the same windows apply to any week the manager views.
// It is a SINGLE bounded query over an explicit projection (no SELECT *, no N+1):
// the (business_id, staff_id, weekday) composite carries the read and the rows
// are grouped in memory. Windows are returned for all staff in the business; the
// grid overlays only the staff rows it renders, so windows belonging to a
// since-deactivated staff are simply ignored client-side rather than joined away
// here. The map is always non-nil (empty on a business with no windows).
func (d *DB) GetTeamAvailability(businessID uint) (map[uint][]StaffAvailability, error) {
	var rows []StaffAvailability
	if err := d.GetGorm().
		Select(teamAvailabilityColumns).
		Where("business_id = ?", businessID).
		Order("staff_id asc, weekday asc, start_min asc, id asc").
		Limit(teamAvailabilityLimit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[uint][]StaffAvailability, len(rows))
	for i := range rows {
		out[rows[i].StaffID] = append(out[rows[i].StaffID], rows[i])
	}
	return out, nil
}

// validateAvailabilityWindow enforces weekday 0..6, 0 ≤ start < end ≤ 1440, and
// a recognized kind.
func validateAvailabilityWindow(w StaffAvailability) error {
	if w.Weekday < 0 || w.Weekday > 6 {
		return ErrAvailabilityInvalid
	}
	if w.StartMin < 0 || w.EndMin > 1440 || w.StartMin >= w.EndMin {
		return ErrAvailabilityInvalid
	}
	if w.Kind != AvailabilityKindPreferred && w.Kind != AvailabilityKindUnavailable {
		return ErrAvailabilityInvalid
	}
	return nil
}

// ReplaceAvailability replaces a staff member's entire availability set in one
// transaction (delete-then-insert), scoped to the caller's OWN staff_id. The
// business_id and staff_id on every window are stamped from the trusted
// arguments — a body-supplied staff_id is ignored, so a caller can never write
// another person's availability. Validates the whole batch BEFORE any write
// (reject-before-persist); an invalid window aborts the replace with nothing
// persisted. Returns the freshly stored windows.
func (d *DB) ReplaceAvailability(businessID, staffID uint, windows []StaffAvailability) ([]StaffAvailability, error) {
	// Stamp tenant + owner identity and validate the whole batch first.
	for i := range windows {
		windows[i].ID = 0
		windows[i].BusinessID = businessID
		windows[i].StaffID = staffID
		if err := validateAvailabilityWindow(windows[i]); err != nil {
			return nil, err
		}
	}
	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("business_id = ? AND staff_id = ?", businessID, staffID).
			Delete(&StaffAvailability{}).Error; err != nil {
			return err
		}
		if len(windows) > 0 {
			if err := tx.Create(&windows).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return d.GetAvailability(businessID, staffID)
}

// timeOffColumns is the explicit projection the time-off list reads — every
// display column, selected explicitly to keep the queue/own-list reads off
// SELECT * (access-shape gate). No money fields exist on this row.
var timeOffColumns = []string{
	"id", "business_id", "staff_id", "starts_at", "ends_at", "reason",
	"status", "decided_by_staff_id", "decided_at", "created_at", "updated_at",
}

// timeOffListLimit bounds the manager queue / staff "my requests" result set.
const timeOffListLimit = 500

// CreateTimeOff inserts a pending time-off request for the caller's own staff_id
// (the handler passes the context staff_id; a body staff_id is never trusted).
// Validates end strictly after start (reject-before-persist).
func (d *DB) CreateTimeOff(businessID, staffID uint, startsAt, endsAt time.Time, reason string) (*TimeOffRequest, error) {
	if !endsAt.After(startsAt) {
		return nil, ErrTimeOffInvalidRange
	}
	req := &TimeOffRequest{
		BusinessID: businessID,
		StaffID:    staffID,
		StartsAt:   startsAt,
		EndsAt:     endsAt,
		Reason:     reason,
		Status:     TimeOffStatusPending,
	}
	if err := d.GetGorm().Create(req).Error; err != nil {
		return nil, err
	}
	return req, nil
}

// ListTimeOff returns time-off requests for a business, row-scoped in SQL.
// scopeStaffID nil = the manager queue (all rows, optionally filtered by status
// via the (business_id, status) index); scopeStaffID set = a single staff's own
// requests via the (business_id, staff_id) index. The result is a single bounded
// query over an explicit projection (no SELECT *, no N+1), newest first.
func (d *DB) ListTimeOff(businessID uint, status string, scopeStaffID *uint) ([]TimeOffRequest, error) {
	q := d.GetGorm().
		Select(timeOffColumns).
		Where("business_id = ?", businessID)
	if scopeStaffID != nil {
		q = q.Where("staff_id = ?", *scopeStaffID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var out []TimeOffRequest
	if err := q.Order("created_at desc, id desc").Limit(timeOffListLimit).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// DecideTimeOff approves or denies a pending time-off request and records the
// decision. A tenant-scoped existence check runs first so a cross-tenant or
// unknown request returns ErrTimeOffNotFound (404). The decision itself is a CAS
// update guarded on status='pending' inside a transaction: RowsAffected != 1
// means the request was no longer pending (illegal transition) -> the update is
// the no-op and ErrTimeOffNotPending is returned. The same transaction writes a
// best-effort RBACAuditLog row (contracts §9) — an audit failure is logged but
// never blocks the decision. Returns the reloaded request.
func (d *DB) DecideTimeOff(businessID, reqID, byStaffID uint, approve bool, reason string) (*TimeOffRequest, error) {
	var existing TimeOffRequest
	if err := d.GetGorm().
		Where("id = ? AND business_id = ?", reqID, businessID).
		First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTimeOffNotFound
		}
		return nil, err
	}

	if InstantElapsed(existing.EndsAt, time.Now().UTC()) {
		return nil, ErrTimeOffExpired
	}

	decided := TimeOffStatusDenied
	if approve {
		decided = TimeOffStatusApproved
	}
	now := time.Now().UTC()

	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&TimeOffRequest{}).
			Where("id = ? AND business_id = ? AND status = ?", reqID, businessID, TimeOffStatusPending).
			Updates(map[string]interface{}{
				"status":              decided,
				"decided_by_staff_id": byStaffID,
				"decided_at":          now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrTimeOffNotPending
		}
		// Best-effort RBAC audit: the subject is the requesting staff; the actor
		// (ChangedBy) is the approver. A failure here must not roll back the
		// decision.
		audit := RBACAuditLog{
			StaffID:    existing.StaffID,
			BusinessID: businessID,
			Action:     RBACActionTimeOffDecided,
			ChangedBy:  fmt.Sprintf("staff:%d", byStaffID),
			Reason:     fmt.Sprintf("timeoff %s: req %d staff %d (%s)", decided, reqID, existing.StaffID, reason),
			CreatedAt:  now,
		}
		if aerr := tx.Create(&audit).Error; aerr != nil {
			logger.Logger.Warnf("time-off decision audit write failed (req %d): %v", reqID, aerr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var updated TimeOffRequest
	if err := d.GetGorm().
		Where("id = ? AND business_id = ?", reqID, businessID).
		First(&updated).Error; err != nil {
		return nil, err
	}
	return &updated, nil
}
