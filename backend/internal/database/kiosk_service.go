package database

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// KioskStaffMember is the lean, money-free projection the shared-terminal
// clock-in surface uses (Phase 4b). It carries only what a kiosk needs to
// render a name tile and toggle a punch — never a pay rate or any dollar field
// (contracts §0 no-staff-dollars). HasPin lets the terminal show who can clock
// in; OnClock / ClockInAt reflect the staffer's current punch state.
type KioskStaffMember struct {
	StaffID   uint       `json:"staff_id"`
	Name      string     `json:"name"`
	Role      StaffRole  `json:"role"`
	HasPin    bool       `json:"has_pin"`
	OnClock   bool       `json:"on_clock"`
	ClockInAt *time.Time `json:"clock_in_at,omitempty"`
}

// kioskRosterLimit bounds both kiosk reads so a pathological roster can never
// unbound the result set.
const kioskRosterLimit = 500

// kioskStaffColumns is the explicit projection for the kiosk roster/lookup:
// identity + role + pin_set_at (the has-pin proxy). pin_hash is deliberately
// NEVER selected — the kiosk only needs to know a PIN EXISTS, not its value, so
// the bcrypt material never leaves the DB on these reads (defense in depth).
var kioskStaffColumns = []string{"id", "name", "role", "pin_set_at"}

// kioskStaffRow is the scan target for the projected staff reads.
type kioskStaffRow struct {
	ID       uint
	Name     string
	Role     StaffRole
	PinSetAt *time.Time
}

// KioskRoster returns the active staff of a business for a shared clock-in
// terminal, each annotated with whether they have a PIN and their current
// on-clock state. Two bounded, tenant-scoped, explicitly-projected queries
// (no SELECT *, no pin_hash, no money join, no per-row reload): one over staff,
// one over the currently-open time entries, joined in memory. O(1) queries
// regardless of roster size.
func (d *DB) KioskRoster(businessID uint) ([]KioskStaffMember, error) {
	var rows []kioskStaffRow
	if err := d.GetGorm().
		Table("staff").
		Select(kioskStaffColumns).
		Where("business_id = ? AND is_active = ?", businessID, true).
		Order("name asc, id asc").
		Limit(kioskRosterLimit).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	// Currently-open punches for the business → on-clock set (single query).
	type openRow struct {
		StaffID   uint
		ClockInAt time.Time
	}
	var open []openRow
	if err := d.GetGorm().
		Table("time_entries").
		Select("staff_id", "clock_in_at").
		Where("business_id = ? AND status = ?", businessID, TimeEntryStatusOpen).
		Limit(kioskRosterLimit).
		Find(&open).Error; err != nil {
		return nil, err
	}
	openBy := make(map[uint]time.Time, len(open))
	for _, o := range open {
		// Keep the earliest open punch if somehow more than one exists.
		if prev, ok := openBy[o.StaffID]; !ok || o.ClockInAt.Before(prev) {
			openBy[o.StaffID] = o.ClockInAt
		}
	}

	out := make([]KioskStaffMember, 0, len(rows))
	for _, r := range rows {
		m := KioskStaffMember{
			StaffID: r.ID,
			Name:    r.Name,
			Role:    r.Role,
			HasPin:  r.PinSetAt != nil,
		}
		if ci, ok := openBy[r.ID]; ok {
			m.OnClock = true
			cic := ci
			m.ClockInAt = &cic
		}
		out = append(out, m)
	}
	return out, nil
}

// GetKioskStaff loads one active staffer scoped to the business for a kiosk
// punch. Tenant + active scoped in SQL: a staffer from another business, or an
// inactive one, resolves to ErrStaffNotFound. This is what prevents a manager
// operating the kiosk on business A from punching business B's staff via
// VerifyStaffPin — which is NOT itself tenant-scoped. Explicit projection, no
// pin_hash, no money.
func (d *DB) GetKioskStaff(businessID, staffID uint) (*KioskStaffMember, error) {
	var r kioskStaffRow
	if err := d.GetGorm().
		Table("staff").
		Select(kioskStaffColumns).
		Where("id = ? AND business_id = ? AND is_active = ?", staffID, businessID, true).
		First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStaffNotFound
		}
		return nil, err
	}
	return &KioskStaffMember{
		StaffID: r.ID,
		Name:    r.Name,
		Role:    r.Role,
		HasPin:  r.PinSetAt != nil,
	}, nil
}

// GetOpenEntry returns the staffer's current open punch, or ErrNotClockedIn
// when none is open. Public wrapper over findOpenEntry so callers outside this
// package (the kiosk toggle) can decide clock-in vs clock-out.
func (d *DB) GetOpenEntry(businessID, staffID uint) (*TimeEntry, error) {
	return d.findOpenEntry(businessID, staffID)
}
