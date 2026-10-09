package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

type ScheduleSettingsHandler struct{ db *database.DB }

func NewScheduleSettingsHandler(db *database.DB) *ScheduleSettingsHandler {
	return &ScheduleSettingsHandler{db: db}
}

// scheduleSettingsClearMinute is the PUT sentinel that sets a nullable
// minute-of-day window back to NULL (disables the feature). 0..1439 is a valid
// minute (0 = midnight), so a separate out-of-band value is needed to mean
// "clear"; the service forwards a staged nil map value as SQL NULL.
const scheduleSettingsClearMinute = -1 // PUT sentinel: set a nullable minute-of-day window back to NULL (disable)

// scheduleSettingsUpdateDTO is the PUT body. All fields are pointers so a
// partial update only touches the keys the caller actually sent (omitted ≠ 0).
// The three nullable minute-of-day windows (minor_cutoff_min,
// quiet_hours_start_min, quiet_hours_end_min) accept 0..1439 to set a concrete
// minute or -1 (scheduleSettingsClearMinute) to clear the column back to NULL.
type scheduleSettingsUpdateDTO struct {
	WeekStartDay          *int `json:"week_start_day"`
	DefaultShiftMinutes   *int `json:"default_shift_minutes"`
	ReminderLeadHours     *int `json:"reminder_lead_hours"`
	OvertimeWeeklyMinutes *int `json:"overtime_weekly_minutes"`
	PostedLeadDays        *int `json:"posted_lead_days"`
	MinorCutoffMin        *int `json:"minor_cutoff_min"`      // 0..1439, or -1 clears (NULL)
	QuietHoursStartMin    *int `json:"quiet_hours_start_min"` // 0..1439, or -1 clears (NULL)
	QuietHoursEndMin      *int `json:"quiet_hours_end_min"`   // 0..1439, or -1 clears (NULL)
}

func (h *ScheduleSettingsHandler) Get(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	s, err := h.db.GetOrCreateBusinessScheduleSettings(businessID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load schedule settings")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": s})
}

func (h *ScheduleSettingsHandler) Put(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	var in scheduleSettingsUpdateDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	fields := map[string]interface{}{}

	if in.WeekStartDay != nil {
		if *in.WeekStartDay < 0 || *in.WeekStartDay > 6 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "week_start_day must be 0..6")
			return
		}
		fields["week_start_day"] = *in.WeekStartDay
	}
	if !scheduleSettingsSetNonNeg(c, fields, "default_shift_minutes", in.DefaultShiftMinutes, 10080) {
		return
	}
	if !scheduleSettingsSetNonNeg(c, fields, "reminder_lead_hours", in.ReminderLeadHours, 168) {
		return
	}
	if !scheduleSettingsSetNonNeg(c, fields, "overtime_weekly_minutes", in.OvertimeWeeklyMinutes, 10080) {
		return
	}
	if !scheduleSettingsSetNonNeg(c, fields, "posted_lead_days", in.PostedLeadDays, 60) {
		return
	}
	if !scheduleSettingsSetMinuteOfDay(c, fields, "minor_cutoff_min", in.MinorCutoffMin) {
		return
	}
	if !scheduleSettingsSetMinuteOfDay(c, fields, "quiet_hours_start_min", in.QuietHoursStartMin) {
		return
	}
	if !scheduleSettingsSetMinuteOfDay(c, fields, "quiet_hours_end_min", in.QuietHoursEndMin) {
		return
	}

	s, err := h.db.UpdateBusinessScheduleSettings(businessID, fields)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to update schedule settings")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": s})
}

// scheduleSettingsSetNonNeg validates an optional non-negative bounded int and stages it.
func scheduleSettingsSetNonNeg(c *gin.Context, fields map[string]interface{}, key string, v *int, max int) bool {
	if v == nil {
		return true
	}
	if *v < 0 || *v > max {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, key+" out of range")
		return false
	}
	fields[key] = *v
	return true
}

// scheduleSettingsSetMinuteOfDay stages an optional 0..1439 minute-of-day value.
// The sentinel -1 (scheduleSettingsClearMinute) clears the column back to NULL
// (disables the window) by staging a nil map value, which the service forwards
// as SQL NULL; a nil pointer means the caller omitted the key (unchanged). All
// other out-of-range values 400.
func scheduleSettingsSetMinuteOfDay(c *gin.Context, fields map[string]interface{}, key string, v *int) bool {
	if v == nil {
		return true
	}
	if *v == scheduleSettingsClearMinute {
		fields[key] = nil // explicit clear → SQL NULL
		return true
	}
	if *v < 0 || *v > 1439 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, key+" must be 0..1439 or -1 to clear")
		return false
	}
	fields[key] = *v
	return true
}
