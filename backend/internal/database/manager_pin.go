package database

import (
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ManagerPIN errors.
var (
	// ErrPinTooShort is returned when a staff tries to set a PIN below the
	// minimum digit length (4). We deliberately do not enforce more complex
	// strength rules per IMP-14 scope.
	ErrPinTooShort = errors.New("PIN must be at least 4 digits")
	// ErrPinTooLong guards against unbounded hashes; restaurant ops never need
	// more than 12 digits and bcrypt has a 72-byte input limit.
	ErrPinTooLong = errors.New("PIN must be at most 12 digits")
	// ErrPinNotNumeric flags non-digit characters.
	ErrPinNotNumeric = errors.New("PIN must be numeric")
	// ErrPinInvalid is returned by VerifyStaffPin when the supplied PIN does
	// not match the stored hash.
	ErrPinInvalid = errors.New("PIN is invalid")
	// ErrPinNotSet is returned by VerifyStaffPin when the staff has no PIN
	// configured.
	ErrPinNotSet = errors.New("PIN is not set")
)

const (
	managerPinMinDigits = 4
	managerPinMaxDigits = 12
)

// validateRawPin enforces the lightweight rules from IMP-14: digits only,
// 4-12 chars. Anything stricter is out-of-scope for this drop.
func validateRawPin(pin string) error {
	if len(pin) < managerPinMinDigits {
		return ErrPinTooShort
	}
	if len(pin) > managerPinMaxDigits {
		return ErrPinTooLong
	}
	for _, r := range pin {
		if r < '0' || r > '9' {
			return ErrPinNotNumeric
		}
	}
	return nil
}

// SetStaffPin hashes `pin` with bcrypt and persists it on the staff row,
// stamping `pin_set_at` with the current time. Validation errors are returned
// before any DB write.
func SetStaffPin(staffID uint, pin string) error {
	if err := validateRawPin(pin); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := time.Now()
	return GetDB().Model(&Staff{}).
		Where("id = ?", staffID).
		Updates(map[string]interface{}{
			"pin_hash":   string(hash),
			"pin_set_at": &now,
		}).Error
}

// VerifyStaffPin compares a supplied PIN against the stored bcrypt hash for
// `staffID`. Returns nil on a match, ErrPinNotSet if the staff hasn't
// enrolled, or ErrPinInvalid for a mismatch.
func VerifyStaffPin(staffID uint, pin string) error {
	var staff Staff
	if err := GetDB().Select("id", "pin_hash").First(&staff, staffID).Error; err != nil {
		return err
	}
	if strings.TrimSpace(staff.PinHash) == "" {
		return ErrPinNotSet
	}
	if err := bcrypt.CompareHashAndPassword([]byte(staff.PinHash), []byte(pin)); err != nil {
		return ErrPinInvalid
	}
	return nil
}

// StaffHasPin returns true when the staff row carries a non-empty bcrypt hash.
// Used by the middleware to decide whether to require a PIN header or fall
// through (a staff member who has never enrolled is allowed to act, but the
// audit row will note pin_present=false).
func StaffHasPin(staffID uint) (bool, error) {
	var staff Staff
	if err := GetDB().Select("id", "pin_hash").First(&staff, staffID).Error; err != nil {
		return false, err
	}
	return strings.TrimSpace(staff.PinHash) != "", nil
}

// RecordCompVoidAudit appends a row to the comp_void_audit log. The audit
// table is append-only; callers should never UPDATE rows.
func RecordCompVoidAudit(entry *CompVoidAudit) error {
	if entry == nil {
		return errors.New("comp_void_audit entry is nil")
	}
	return GetDB().Create(entry).Error
}
