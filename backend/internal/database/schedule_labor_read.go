package database

import "time"

// ScheduleShiftLine is the minimal shift projection the labor preview needs —
// never a full Shift row, never a Preload. StaffID nil = open shift (unpriced).
type ScheduleShiftLine struct {
	StaffID      *uint     `json:"staff_id"`
	PositionID   uint      `json:"position_id"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	BreakMinutes int       `json:"break_minutes"`
}

// StaffPositionKey indexes a per-role pay rate.
type StaffPositionKey struct{ StaffID, PositionID uint }

// GetScheduleShiftLines returns one schedule's shifts as narrow projections,
// tenant-scoped and bounded by schedule_id. No SELECT *, no Preload.
func (d *DB) GetScheduleShiftLines(businessID, scheduleID uint) ([]ScheduleShiftLine, error) {
	var rows []ScheduleShiftLine
	err := d.GetGorm().
		Model(&Shift{}).
		Select("staff_id", "position_id", "starts_at", "ends_at", "break_minutes").
		Where("business_id = ? AND schedule_id = ?", businessID, scheduleID).
		Order("position_id asc, starts_at asc").
		Scan(&rows).Error
	return rows, err
}

// GetScheduleShiftLinesInWindow returns the business's shifts whose StartsAt
// falls inside the half-open window [start, end), as narrow projections. It backs
// the accounting labor-cost "scheduled" basis, which lenses scheduled labor over a
// rolling period window (week|month) rather than a single schedule — so it is
// window-scoped, not schedule-scoped. One query, no SELECT *, no Preload.
func (d *DB) GetScheduleShiftLinesInWindow(businessID uint, start, end time.Time) ([]ScheduleShiftLine, error) {
	var rows []ScheduleShiftLine
	err := d.GetGorm().
		Model(&Shift{}).
		Select("staff_id", "position_id", "starts_at", "ends_at", "break_minutes").
		Where("business_id = ? AND starts_at >= ? AND starts_at < ?", businessID, start.UTC(), end.UTC()).
		Order("position_id asc, starts_at asc").
		Scan(&rows).Error
	return rows, err
}

// GetStaffPositionRates returns every (staff,position)->pay_rate_cents for the
// business in ONE query — the preview joins this map in memory instead of a
// per-shift rate lookup (no N+1). Bounded by team size.
func (d *DB) GetStaffPositionRates(businessID uint) (map[StaffPositionKey]int64, error) {
	type rateRow struct {
		StaffID      uint
		PositionID   uint
		PayRateCents int64
	}
	var rows []rateRow
	if err := d.GetGorm().
		Model(&StaffPosition{}).
		Select("staff_id", "position_id", "pay_rate_cents").
		Where("business_id = ?", businessID).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[StaffPositionKey]int64, len(rows))
	for _, r := range rows {
		out[StaffPositionKey{StaffID: r.StaffID, PositionID: r.PositionID}] = r.PayRateCents
	}
	return out, nil
}

// IsMinor reports whether a staff member is flagged as a scheduled-as-minor.
// Slice 6 adds NO migration and no Staff.IsMinor column exists yet, so this
// returns false for every staff today: the minor-late warning's COMPUTE path
// ships and is unit-tested via an injected predicate, while its production data
// source is deferred to the slice that introduces the minor flag. MinorCutoffMin
// (nil = off) on BusinessScheduleSettings already gates the chip, so no false
// chips fire in the interim. Documented deferral, not a hidden gap.
func (d *DB) IsMinor(_, _ uint) bool { return false }
