package database

import "time"

// Shift-note (logbook) categories. Any staff may log a handover note; the
// category is a closed whitelist validated reject-before-persist in the handler.
const (
	ShiftNoteCategorySales       = "sales"
	ShiftNoteCategoryGuests      = "guests"
	ShiftNoteCategoryStaffing    = "staffing"
	ShiftNoteCategoryMaintenance = "maintenance"
	ShiftNoteCategoryOther       = "other"
)

// ShiftNote is one shift-handover / logbook entry, scoped to a business and a
// calendar date (ForDate), optionally to a shift. No money — Content is free
// text. Read by schedule:read (all staff); written by any staff (also
// schedule:read, per spec §7 / contracts §5). Append-only: no UpdatedAt.
type ShiftNote struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	BusinessID    uint      `gorm:"not null;index:idx_shift_notes_business_for_date,priority:1" json:"business_id"`
	ShiftID       *uint     `json:"shift_id"`
	ForDate       time.Time `gorm:"not null;index:idx_shift_notes_business_for_date,priority:2" json:"for_date"`
	AuthorStaffID uint      `gorm:"not null" json:"author_staff_id"`
	Category      string    `gorm:"type:varchar(16);not null" json:"category"`
	Content       string    `gorm:"type:text;not null;default:''" json:"content"`
	CreatedAt     time.Time `json:"created_at"`
}

// TableName pins the table name so GORM does not guess; equals the 000109 DDL.
func (ShiftNote) TableName() string { return "shift_notes" }

// shiftNoteColumns is the explicit projection used by ListShiftNotes so reads
// never widen to SELECT * (perf access-shape gate, CLAUDE.md).
const shiftNoteColumns = "id, business_id, shift_id, for_date, author_staff_id, category, content, created_at"

// IsValidShiftNoteCategory reports whether c is a recognized logbook category.
func IsValidShiftNoteCategory(c string) bool {
	switch c {
	case ShiftNoteCategorySales, ShiftNoteCategoryGuests, ShiftNoteCategoryStaffing,
		ShiftNoteCategoryMaintenance, ShiftNoteCategoryOther:
		return true
	default:
		return false
	}
}

// CreateShiftNote inserts a note (BusinessID/ForDate/AuthorStaffID/Category set
// by the caller). Append-only — there is no update path.
func (d *DB) CreateShiftNote(n *ShiftNote) error {
	return d.GetGorm().Create(n).Error
}

// ListShiftNotes returns a business's notes in the half-open day window
// [from, to), newest first, capped at limit. Explicit projection (no SELECT *),
// single query (no N+1), bounded result — the (business_id, for_date) index
// serves the filter+order.
func (d *DB) ListShiftNotes(businessID uint, from, to time.Time, limit int) ([]ShiftNote, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	var out []ShiftNote
	err := d.GetGorm().
		Select(shiftNoteColumns).
		Where("business_id = ? AND for_date >= ? AND for_date < ?", businessID, from, to).
		Order("for_date desc, id desc").
		Limit(limit).
		Find(&out).Error
	return out, err
}
