package database

import "time"

// AuthAttempt is one (principal, kind) throttle counter row. principal is the
// throttled identity (lowercased email / wallet / IP); kind names the auth
// surface. The unique (principal, kind) index is created by the genesis schema
// and mirrored by AutoMigrate via the struct tags below.
type AuthAttempt struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Principal       string     `gorm:"uniqueIndex:uni_auth_attempts_principal_kind;not null" json:"principal"`
	Kind            string     `gorm:"uniqueIndex:uni_auth_attempts_principal_kind;not null" json:"kind"`
	Count           int        `gorm:"not null;default:0" json:"count"`
	WindowStartedAt time.Time  `gorm:"not null" json:"window_started_at"`
	LockedUntil     *time.Time `json:"locked_until,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// TableName pins the table name so it matches the genesis table exactly.
func (AuthAttempt) TableName() string { return "auth_attempts" }
