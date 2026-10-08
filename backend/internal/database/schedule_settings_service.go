package database

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// BusinessScheduleSettings holds per-business schedule/compliance configuration:
// builder defaults, reminder lead, overtime/posted-late/minor warning thresholds,
// and an optional quiet-hours window. Exactly one row per business, created lazily
// with sensible defaults on first access (mirrors ReservationSettings). No money
// fields — nothing here is owner-gated. Struct tags match the genesis
// business_schedule_settings table.
type BusinessScheduleSettings struct {
	ID                    uint      `gorm:"primaryKey" json:"id"`
	BusinessID            uint      `gorm:"uniqueIndex;not null" json:"business_id"`
	WeekStartDay          int       `gorm:"not null;default:1" json:"week_start_day"`
	DefaultShiftMinutes   int       `gorm:"not null;default:480" json:"default_shift_minutes"`
	ReminderLeadHours     int       `gorm:"not null;default:3" json:"reminder_lead_hours"`
	OvertimeWeeklyMinutes int       `gorm:"not null;default:2400" json:"overtime_weekly_minutes"`
	PostedLeadDays        int       `gorm:"not null;default:7" json:"posted_lead_days"`
	MinorCutoffMin        *int      `json:"minor_cutoff_min"`
	QuietHoursStartMin    *int      `json:"quiet_hours_start_min"`
	QuietHoursEndMin      *int      `json:"quiet_hours_end_min"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func (BusinessScheduleSettings) TableName() string { return "business_schedule_settings" }

func defaultBusinessScheduleSettings(businessID uint) BusinessScheduleSettings {
	return BusinessScheduleSettings{
		BusinessID:            businessID,
		WeekStartDay:          1,
		DefaultShiftMinutes:   480,
		ReminderLeadHours:     3,
		OvertimeWeeklyMinutes: 2400,
		PostedLeadDays:        7,
	}
}

// GetOrCreateBusinessScheduleSettings returns the schedule settings for a
// business, lazily creating a defaults row on first access (mirrors
// GetReservationSettings). Single-row, unique-key read — no fan-out.
func (d *DB) GetOrCreateBusinessScheduleSettings(businessID uint) (*BusinessScheduleSettings, error) {
	var s BusinessScheduleSettings
	err := d.GetGorm().Where("business_id = ?", businessID).First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s = defaultBusinessScheduleSettings(businessID)
			if cErr := d.GetGorm().Create(&s).Error; cErr != nil {
				return nil, fmt.Errorf("create default schedule settings: %w", cErr)
			}
			return &s, nil
		}
		return nil, fmt.Errorf("get schedule settings: %w", err)
	}
	return &s, nil
}

// scheduleSettingsUpdatable whitelists the columns a caller may patch via
// UpdateBusinessScheduleSettings — id/business_id/timestamps are never writable.
var scheduleSettingsUpdatable = map[string]struct{}{
	"week_start_day":          {},
	"default_shift_minutes":   {},
	"reminder_lead_hours":     {},
	"overtime_weekly_minutes": {},
	"posted_lead_days":        {},
	"minor_cutoff_min":        {},
	"quiet_hours_start_min":   {},
	"quiet_hours_end_min":     {},
}

// UpdateBusinessScheduleSettings applies a whitelisted field map to a business's
// schedule settings (lazily creating the row first if absent), tenant-scoped by
// business_id, and returns the reloaded row.
func (d *DB) UpdateBusinessScheduleSettings(businessID uint, fields map[string]interface{}) (*BusinessScheduleSettings, error) {
	if _, err := d.GetOrCreateBusinessScheduleSettings(businessID); err != nil {
		return nil, err
	}
	clean := map[string]interface{}{}
	for k, v := range fields {
		if _, ok := scheduleSettingsUpdatable[k]; ok {
			clean[k] = v
		}
	}
	if len(clean) > 0 {
		if err := d.GetGorm().Model(&BusinessScheduleSettings{}).
			Where("business_id = ?", businessID).
			Updates(clean).Error; err != nil {
			return nil, fmt.Errorf("update schedule settings: %w", err)
		}
	}
	var s BusinessScheduleSettings
	if err := d.GetGorm().Where("business_id = ?", businessID).First(&s).Error; err != nil {
		return nil, fmt.Errorf("reload schedule settings: %w", err)
	}
	return &s, nil
}
