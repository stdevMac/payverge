package database

import (
	"fmt"
	"time"
)

// StaffMembershipSelectionToken records one outstanding membership-selection
// proof. Only the SHA-256 of the token's jti is stored; the row is deleted
// the moment the token is redeemed, so a captured token cannot be replayed.
type StaffMembershipSelectionToken struct {
	JTIHash   string    `gorm:"column:jti_hash;type:varchar(64);primaryKey" json:"-"`
	ExpiresAt time.Time `gorm:"not null;index:idx_staff_membership_selection_tokens_expires_at" json:"-"`
}

// TableName pins the genesis table name.
func (StaffMembershipSelectionToken) TableName() string {
	return "staff_membership_selection_tokens"
}

// RecordStaffMembershipSelectionToken stores a freshly issued selection
// token's jti hash and prunes rows that have already expired.
func RecordStaffMembershipSelectionToken(jtiHash string, expiresAt time.Time) error {
	gdb := GetDB()
	if gdb == nil {
		return fmt.Errorf("database not initialized")
	}
	if err := gdb.Where("expires_at <= ?", time.Now()).
		Delete(&StaffMembershipSelectionToken{}).Error; err != nil {
		return fmt.Errorf("prune selection tokens: %w", err)
	}
	return gdb.Create(&StaffMembershipSelectionToken{JTIHash: jtiHash, ExpiresAt: expiresAt}).Error
}

// ConsumeStaffMembershipSelectionToken atomically redeems a selection token.
// It reports true only for the single caller whose DELETE removed the live
// row; a second redemption, or one after expiry, reports false.
func ConsumeStaffMembershipSelectionToken(jtiHash string) (bool, error) {
	gdb := GetDB()
	if gdb == nil {
		return false, fmt.Errorf("database not initialized")
	}
	result := gdb.Where("jti_hash = ? AND expires_at > ?", jtiHash, time.Now()).
		Delete(&StaffMembershipSelectionToken{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}
