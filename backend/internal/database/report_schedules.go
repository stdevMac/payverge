package database

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateReportSchedule creates a new report schedule
func (db *DB) CreateReportSchedule(schedule *ReportSchedule) error {
	return db.conn.Create(schedule).Error
}

// GetReportScheduleByID retrieves a report schedule by ID
func (db *DB) GetReportScheduleByID(id uint) (*ReportSchedule, error) {
	var schedule ReportSchedule
	err := db.conn.Preload("Business").First(&schedule, id).Error
	if err != nil {
		return nil, err
	}
	return &schedule, nil
}

// GetReportSchedulesByBusinessID retrieves all report schedules for a business
func (db *DB) GetReportSchedulesByBusinessID(businessID uint) ([]ReportSchedule, error) {
	var schedules []ReportSchedule
	err := db.conn.Where("business_id = ?", businessID).Find(&schedules).Error
	return schedules, err
}

// GetReportScheduleByBusinessAndFrequency retrieves a report schedule by business ID and frequency
func (db *DB) GetReportScheduleByBusinessAndFrequency(businessID uint, frequency ReportFrequency) (*ReportSchedule, error) {
	var schedule ReportSchedule
	err := db.conn.Where("business_id = ? AND frequency = ?", businessID, frequency).First(&schedule).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &schedule, nil
}

// UpdateReportSchedule updates a report schedule
func (db *DB) UpdateReportSchedule(schedule *ReportSchedule) error {
	return db.conn.Omit(clause.Associations).Save(schedule).Error
}

// dueReportSchedulesBatchLimit caps one scheduler tick. The next one-minute
// tick picks up the rest.
const dueReportSchedulesBatchLimit = 50

// GetDueReportSchedules returns active schedules whose next send time has
// arrived, oldest first, at most dueReportSchedulesBatchLimit rows.
// Business is not loaded: the renderer reloads it by ID. Every schedule
// column is selected because UpdateReportSchedule saves the struct.
//
// A schedule whose current occurrence cannot be claimed right now is left
// out: a dead-lettered (or already sent) occurrence never advances
// next_send_at, and a retry_wait or live lease is not claimable until later.
// Those rows keep the oldest next_send_at, so without this filter enough of
// them would fill every bounded batch and starve every other schedule.
func (db *DB) GetDueReportSchedules(beforeTime time.Time) ([]ReportSchedule, error) {
	var schedules []ReportSchedule
	err := db.conn.
		Select(
			"id", "business_id", "frequency", "day_of_week", "hour", "minute",
			"timezone", "is_active", "last_sent_at", "next_send_at", "created_at", "updated_at",
		).
		Where("is_active = ? AND next_send_at <= ?", true, beforeTime).
		Where(`NOT EXISTS (
			SELECT 1 FROM report_deliveries d
			WHERE d.schedule_id = report_schedules.id
			  AND d.scheduled_for = report_schedules.next_send_at
			  AND (d.state IN ?
			    OR (d.state IN ? AND d.next_attempt_at > ?)
			    OR (d.state = ? AND d.lease_expires_at > ?))
		)`,
			[]ReportDeliveryState{ReportDeliveryStateDeadLetter, ReportDeliveryStateSent},
			[]ReportDeliveryState{ReportDeliveryStatePending, ReportDeliveryStateRetryWait}, beforeTime,
			ReportDeliveryStateLeased, beforeTime,
		).
		Order("next_send_at ASC, id ASC").
		Limit(dueReportSchedulesBatchLimit).
		Find(&schedules).Error
	return schedules, err
}

// calculateNextSendTime calculates the next send time based on frequency and schedule settings
func calculateNextSendTime(schedule *ReportSchedule) time.Time {
	return nextSendTimeFrom(schedule, time.Now())
}

// nextSendTimeFrom is calculateNextSendTime with an injectable reference time so
// the DST behavior can be tested deterministically around a transition date.
func nextSendTimeFrom(schedule *ReportSchedule, ref time.Time) time.Time {
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		location = time.UTC
	}

	now := ref.In(location)

	// Start with today at the scheduled wall-clock time. time.Date maps a
	// nonexistent spring-forward wall time backwards (02:30 -> 01:30 in New
	// York), which would send early. Shift that mismatch forward by the DST gap
	// instead (02:30 -> 03:30). An ambiguous fall-back time resolves to the first
	// occurrence, producing one report for that local calendar day.
	wallTime := func(date time.Time) time.Time {
		candidate := time.Date(date.Year(), date.Month(), date.Day(), schedule.Hour, schedule.Minute, 0, 0, location)
		if candidate.Hour() != schedule.Hour || candidate.Minute() != schedule.Minute {
			wantedMinutes := schedule.Hour*60 + schedule.Minute
			actualMinutes := candidate.Hour()*60 + candidate.Minute()
			gapMinutes := wantedMinutes - actualMinutes
			if gapMinutes < 0 {
				gapMinutes += 24 * 60
			}
			candidate = candidate.Add(time.Duration(gapMinutes) * time.Minute)
		}
		return candidate
	}
	nextSend := wallTime(now)

	// Advance by whole CALENDAR days (AddDate), not fixed 24h durations. Across a
	// DST transition a day is 23 or 25 hours, so adding 24*time.Hour shifts the
	// send by an hour (a 9am report goes out at 8am or 10am). Recomputing the
	// wall-clock time with time.Date on the target calendar date lets the tz
	// database absorb the offset change and keeps the send at the configured hour.
	advanceDays := func(days int) time.Time {
		d := now.AddDate(0, 0, days)
		return wallTime(d)
	}

	switch schedule.Frequency {
	case ReportFrequencyDaily:
		// If the time has already passed today, schedule for tomorrow.
		if !nextSend.After(now) {
			nextSend = advanceDays(1)
		}
	case ReportFrequencyWeekly:
		// Find the next occurrence of the specified day of week.
		currentWeekday := int(now.Weekday())
		targetWeekday := schedule.DayOfWeek

		daysUntilTarget := (targetWeekday - currentWeekday + 7) % 7

		// If it's the same day but time has passed, schedule for next week.
		if daysUntilTarget == 0 && !nextSend.After(now) {
			daysUntilTarget = 7
		}

		if daysUntilTarget > 0 {
			nextSend = advanceDays(daysUntilTarget)
		}
	}

	return nextSend
}

// CalculateNextSendTime is a public wrapper for calculateNextSendTime
func CalculateNextSendTime(schedule *ReportSchedule) time.Time {
	return calculateNextSendTime(schedule)
}

// CalculateNextSendTimeFrom calculates the occurrence following ref. Workers
// use the claimed schedule occurrence as ref so delayed retries do not skip a
// daily or weekly window.
func CalculateNextSendTimeFrom(schedule *ReportSchedule, ref time.Time) time.Time {
	return nextSendTimeFrom(schedule, ref)
}
