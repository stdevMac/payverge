package database

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Position is a role/station within a business (Server, Line Cook, Host,
// Bartender, …). It drives scheduling eligibility and schedule-chip color.
type Position struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	BusinessID uint      `gorm:"index;not null" json:"business_id"`
	Name       string    `gorm:"type:varchar(255);not null" json:"name"`
	ColorHex   string    `gorm:"type:varchar(9);not null;default:''" json:"color_hex"`
	Department string    `gorm:"type:varchar(16);not null;default:''" json:"department"`
	IsActive   bool      `gorm:"not null;default:true" json:"is_active"`
	SortOrder  int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// StaffPosition links a staff member to a position with an optional per-role
// pay rate. PayRateCents is OWNER-ONLY: it is stripped from JSON (`json:"-"`)
// and exposed only via the owner-gated rate sub-resource, never on the
// assignment payload — mirroring Staff compensation.
type StaffPosition struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	BusinessID   uint      `gorm:"index;not null" json:"business_id"`
	StaffID      uint      `gorm:"not null;uniqueIndex:idx_staff_position" json:"staff_id"`
	PositionID   uint      `gorm:"not null;uniqueIndex:idx_staff_position" json:"position_id"`
	PayRateCents int64     `gorm:"not null;default:0" json:"-"`
	IsPrimary    bool      `gorm:"not null;default:false" json:"is_primary"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ErrPositionNotFound is returned when a position does not exist within the
// caller's business (tenant-scoped lookups).
var ErrPositionNotFound = errors.New("position not found")

// ErrStaffNotFound is returned when a staff member does not exist within the
// caller's business (tenant-scoped lookups).
var ErrStaffNotFound = errors.New("staff not found")

// ListPositions returns active positions for a business, ordered for display.
func (d *DB) ListPositions(businessID uint) ([]Position, error) {
	var out []Position
	err := d.GetGorm().
		Where("business_id = ? AND is_active = ?", businessID, true).
		Order("sort_order asc, name asc").
		Find(&out).Error
	return out, err
}

// GetPosition loads one position, enforcing it belongs to businessID.
func (d *DB) GetPosition(businessID, id uint) (*Position, error) {
	var p Position
	if err := d.GetGorm().
		Where("id = ? AND business_id = ?", id, businessID).
		First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPositionNotFound
		}
		return nil, err
	}
	return &p, nil
}

// CreatePosition inserts a position (BusinessID/Name must be set by the caller).
func (d *DB) CreatePosition(p *Position) error {
	return d.GetGorm().Create(p).Error
}

// UpdatePosition applies a whitelisted field map to a position, tenant-guarded.
// Returns ErrPositionNotFound if no row in businessID matches.
func (d *DB) UpdatePosition(businessID, id uint, fields map[string]interface{}) error {
	delete(fields, "id")
	delete(fields, "ID")
	delete(fields, "business_id")
	delete(fields, "BusinessID")
	res := d.GetGorm().Model(&Position{}).
		Where("id = ? AND business_id = ?", id, businessID).
		Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrPositionNotFound
	}
	return nil
}

// DeletePosition soft-retires a position (is_active=false) so historical
// schedule references stay intact. Tenant-guarded.
func (d *DB) DeletePosition(businessID, id uint) error {
	return d.UpdatePosition(businessID, id, map[string]interface{}{"is_active": false})
}

// ErrStaffPositionNotFound is returned when a staff↔position link does not
// exist within the caller's business.
var ErrStaffPositionNotFound = errors.New("staff position not found")

// ErrStaffPositionRated is returned when unassign is refused because the link
// carries a pay rate and the caller cannot clear payroll data.
var ErrStaffPositionRated = errors.New("staff position has a pay rate; payroll:write is required to unassign it")

// assertPositionInBusiness errors unless the position exists, is active, and
// belongs to biz. Assigning staff to a soft-retired position is rejected
// (returns ErrPositionNotFound), consistent with DeletePosition's soft-retire.
func (d *DB) assertPositionInBusiness(businessID, positionID uint) error {
	p, err := d.GetPosition(businessID, positionID)
	if err != nil {
		return err
	}
	if !p.IsActive {
		return ErrPositionNotFound
	}
	return nil
}

// assertStaffInBusiness errors unless the staff row belongs to the business.
func (d *DB) assertStaffInBusiness(businessID, staffID uint) error {
	var s Staff
	if err := d.GetGorm().
		Where("id = ? AND business_id = ?", staffID, businessID).
		First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStaffNotFound
		}
		return err
	}
	return nil
}

// AssignPosition upserts a staff↔position link (idempotent via the unique
// index). When isPrimary is true, all other primaries for that staff are
// cleared in the same transaction so exactly one primary exists. Pay rate is
// never touched here (owner-only sub-resource).
//
// Concurrency note: under READ COMMITTED, two concurrent
// AssignPosition(..., isPrimary=true) calls for the SAME staff but DIFFERENT
// positions can each clear the other's primaries and then both insert
// is_primary=true, colliding on the partial unique index idx_staff_one_primary.
// That surfaces as a Postgres unique-violation which the handler maps to a 500.
// Data integrity is preserved (the partial index holds — at most one primary is
// ever committed), and no Slice-0 UI path triggers this (assignment is not yet
// wired into the frontend). Translating the unique-violation into a clean
// retry/409 is deferred to the slice that wires assignment into the UI.
func (d *DB) AssignPosition(businessID, staffID, positionID uint, isPrimary bool) error {
	if err := d.assertStaffInBusiness(businessID, staffID); err != nil {
		return err
	}
	if err := d.assertPositionInBusiness(businessID, positionID); err != nil {
		return err
	}
	return d.GetGorm().Transaction(func(tx *gorm.DB) error {
		if isPrimary {
			if err := tx.Model(&StaffPosition{}).
				Where("business_id = ? AND staff_id = ?", businessID, staffID).
				Update("is_primary", false).Error; err != nil {
				return err
			}
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "staff_id"}, {Name: "position_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"is_primary"}),
		}).Create(&StaffPosition{
			BusinessID: businessID, StaffID: staffID, PositionID: positionID, IsPrimary: isPrimary,
		}).Error
	})
}

// UnassignPosition removes a staff↔position link, tenant-guarded.
//
// PayRateCents is owner-only. When canClearPayRate is false the DELETE matches
// only pay_rate_cents = 0, so a schedule:write caller cannot wipe a wage. A
// rated row is left in place and returned as ErrStaffPositionRated.
func (d *DB) UnassignPosition(businessID, staffID, positionID uint, canClearPayRate bool) error {
	q := d.GetGorm().Where(
		"business_id = ? AND staff_id = ? AND position_id = ?",
		businessID, staffID, positionID,
	)
	if !canClearPayRate {
		q = q.Where("pay_rate_cents = ?", 0)
	}
	res := q.Delete(&StaffPosition{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		if canClearPayRate {
			return ErrStaffPositionNotFound
		}
		if _, err := d.getStaffPositionLink(businessID, staffID, positionID); err != nil {
			return err
		}
		return ErrStaffPositionRated
	}
	return nil
}

// ListStaffPositions returns a staff member's position links (no pay rate on
// the wire — PayRateCents is json:"-").
func (d *DB) ListStaffPositions(businessID, staffID uint) ([]StaffPosition, error) {
	var out []StaffPosition
	err := d.GetGorm().
		Where("business_id = ? AND staff_id = ?", businessID, staffID).
		Order("is_primary desc, position_id asc").
		Find(&out).Error
	return out, err
}

// getStaffPositionLink loads one link tenant-guarded, or ErrStaffPositionNotFound.
func (d *DB) getStaffPositionLink(businessID, staffID, positionID uint) (*StaffPosition, error) {
	var link StaffPosition
	if err := d.GetGorm().
		Where("business_id = ? AND staff_id = ? AND position_id = ?", businessID, staffID, positionID).
		First(&link).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStaffPositionNotFound
		}
		return nil, err
	}
	return &link, nil
}

// SetPayRateCents sets the owner-only per-role pay rate on an existing link.
func (d *DB) SetPayRateCents(businessID, staffID, positionID uint, cents int64) error {
	link, err := d.getStaffPositionLink(businessID, staffID, positionID)
	if err != nil {
		return err
	}
	return d.GetGorm().Model(&StaffPosition{}).Where("id = ?", link.ID).
		Update("pay_rate_cents", cents).Error
}

// GetPayRateCents reads the owner-only per-role pay rate, erroring if unassigned.
func (d *DB) GetPayRateCents(businessID, staffID, positionID uint) (int64, error) {
	link, err := d.getStaffPositionLink(businessID, staffID, positionID)
	if err != nil {
		return 0, err
	}
	return link.PayRateCents, nil
}

// roleDisplayName title-cases a staff role for a default position name.
func roleDisplayName(role StaffRole) string {
	s := string(role)
	if s == "" {
		return "Staff"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
