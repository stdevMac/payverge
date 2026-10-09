package database

import "time"

const (
	DemoInstanceStatusCreating  = "creating"
	DemoInstanceStatusReady     = "ready"
	DemoInstanceStatusFailed    = "failed"
	DemoInstanceStatusResetting = "resetting"

	DemoRunTypeEnsure    = "ensure"
	DemoRunTypeAppendDay = "append_day"
	DemoRunTypeReset     = "reset"
	DemoRunTypeVerify    = "verify"

	DemoRunStatusRunning   = "running"
	DemoRunStatusSucceeded = "succeeded"
	DemoRunStatusFailed    = "failed"
)

// DemoInstance records the generated demo estate owned by one admin account.
// Business data stays in normal production tables; this row tracks lifecycle.
type DemoInstance struct {
	ID                        uint       `gorm:"primaryKey" json:"id"`
	AdminUserID               uint       `gorm:"uniqueIndex;not null" json:"admin_user_id"`
	PrimaryBusinessID         *uint      `gorm:"index" json:"primary_business_id,omitempty"`
	SecondaryBusinessID       *uint      `gorm:"index" json:"secondary_business_id,omitempty"`
	Status                    string     `gorm:"not null;default:'creating'" json:"status"`
	SeedVersion               string     `gorm:"not null" json:"seed_version"`
	BaselineStartDate         time.Time  `gorm:"type:date;not null" json:"baseline_start_date"`
	LastSimulatedBusinessDate *time.Time `gorm:"type:date" json:"last_simulated_business_date,omitempty"`
	Timezone                  string     `gorm:"not null;default:'America/New_York'" json:"timezone"`
	LastError                 string     `gorm:"type:text" json:"last_error,omitempty"`
	LastVerifiedAt            *time.Time `json:"last_verified_at,omitempty"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`

	AdminUser         User      `gorm:"foreignKey:AdminUserID" json:"-"`
	PrimaryBusiness   *Business `gorm:"foreignKey:PrimaryBusinessID" json:"primary_business,omitempty"`
	SecondaryBusiness *Business `gorm:"foreignKey:SecondaryBusinessID" json:"secondary_business,omitempty"`
}

func (DemoInstance) TableName() string { return "demo_instances" }

// DemoRun is append-only run history for automatic and manual demo operations.
type DemoRun struct {
	ID               uint           `gorm:"primaryKey" json:"id"`
	DemoInstanceID   uint           `gorm:"index:idx_demo_runs_instance_started,priority:1;not null" json:"demo_instance_id"`
	AdminUserID      uint           `gorm:"index;not null" json:"admin_user_id"`
	RunType          string         `gorm:"not null" json:"run_type"`
	Status           string         `gorm:"not null" json:"status"`
	SeedVersion      string         `gorm:"not null" json:"seed_version"`
	StartedAt        time.Time      `gorm:"index:idx_demo_runs_instance_started,priority:2,sort:desc;not null" json:"started_at"`
	FinishedAt       *time.Time     `json:"finished_at,omitempty"`
	BusinessDateFrom *time.Time     `gorm:"type:date" json:"business_date_from,omitempty"`
	BusinessDateTo   *time.Time     `gorm:"type:date" json:"business_date_to,omitempty"`
	RecordsCreated   JSONRawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"records_created"`
	Verification     JSONRawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"verification"`
	Error            string         `gorm:"type:text" json:"error,omitempty"`

	DemoInstance DemoInstance `gorm:"foreignKey:DemoInstanceID" json:"-"`
}

func (DemoRun) TableName() string { return "demo_runs" }
