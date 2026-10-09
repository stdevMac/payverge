package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReportDeliveryState is the durable state of one scheduled report window.
type ReportDeliveryState string

const (
	ReportDeliveryStatePending    ReportDeliveryState = "pending"
	ReportDeliveryStateLeased     ReportDeliveryState = "leased"
	ReportDeliveryStateRetryWait  ReportDeliveryState = "retry_wait"
	ReportDeliveryStateSent       ReportDeliveryState = "sent"
	ReportDeliveryStateDeadLetter ReportDeliveryState = "dead_letter"
)

// ReportDelivery is the outbox row for one schedule and reporting window. A
// lease is only permission to attempt transport; it is deliberately distinct
// from the sent acknowledgement.
type ReportDelivery struct {
	ID             uint                `gorm:"primaryKey" json:"id"`
	ScheduleID     uint                `gorm:"not null;uniqueIndex:idx_report_delivery_window,priority:1" json:"schedule_id"`
	BusinessID     uint                `gorm:"not null;index" json:"business_id"`
	ScheduledFor   time.Time           `gorm:"not null;index" json:"scheduled_for"`
	WindowStart    time.Time           `gorm:"not null;uniqueIndex:idx_report_delivery_window,priority:2" json:"window_start"`
	WindowEnd      time.Time           `gorm:"not null;uniqueIndex:idx_report_delivery_window,priority:3" json:"window_end"`
	State          ReportDeliveryState `gorm:"type:text;not null;default:'pending';index" json:"state"`
	LeaseToken     string              `gorm:"type:text;not null;default:''" json:"-"`
	LeaseExpiresAt *time.Time          `gorm:"index" json:"lease_expires_at,omitempty"`
	AttemptCount   int                 `gorm:"not null;default:0" json:"attempt_count"`
	NextAttemptAt  time.Time           `gorm:"not null;index" json:"next_attempt_at"`
	LastError      string              `gorm:"type:text;not null;default:''" json:"last_error"`
	SentAt         *time.Time          `json:"sent_at,omitempty"`
	IdempotencyKey string              `gorm:"type:text;not null;uniqueIndex" json:"idempotency_key"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`

	Schedule ReportSchedule `gorm:"foreignKey:ScheduleID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ReportDelivery) TableName() string { return "report_deliveries" }

// ReportWindowForSchedule returns the business-local, half-open analytics
// window associated with one scheduled occurrence. Calendar arithmetic keeps
// the window correct across 23- and 25-hour daylight-saving days.
func ReportWindowForSchedule(schedule *ReportSchedule, scheduledAt time.Time) (time.Time, time.Time) {
	loc := ResolveLocation(schedule.Timezone)
	local := scheduledAt.In(loc)
	end := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	days := -1
	if schedule.Frequency == ReportFrequencyWeekly {
		days = -7
	}
	return end.AddDate(0, 0, days), end
}

func reportDeliveryIdempotencyKey(scheduleID uint, start, end time.Time) string {
	return fmt.Sprintf("report:%d:%s:%s", scheduleID, start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339))
}

// EnsureReportDelivery creates or returns the unique outbox row for a due
// schedule occurrence. The database uniqueness constraint arbitrates two
// replicas that observe the same due schedule.
func (db *DB) EnsureReportDelivery(schedule *ReportSchedule, scheduledAt time.Time) (*ReportDelivery, bool, error) {
	if db == nil || db.conn == nil || schedule == nil || schedule.ID == 0 {
		return nil, false, errors.New("report delivery schedule is required")
	}
	start, end := ReportWindowForSchedule(schedule, scheduledAt)
	row := ReportDelivery{
		ScheduleID:     schedule.ID,
		BusinessID:     schedule.BusinessID,
		ScheduledFor:   scheduledAt,
		WindowStart:    start,
		WindowEnd:      end,
		State:          ReportDeliveryStatePending,
		NextAttemptAt:  scheduledAt,
		IdempotencyKey: reportDeliveryIdempotencyKey(schedule.ID, start, end),
	}
	result := db.conn.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "schedule_id"}, {Name: "window_start"}, {Name: "window_end"}},
		DoNothing: true,
	}).Create(&row)
	if result.Error != nil {
		return nil, false, result.Error
	}
	created := result.RowsAffected == 1
	if !created {
		if err := db.conn.Where("schedule_id = ? AND window_start = ? AND window_end = ?", schedule.ID, start, end).First(&row).Error; err != nil {
			return nil, false, err
		}
	}
	return &row, created, nil
}

func (db *DB) GetReportDeliveryByID(id uint) (*ReportDelivery, error) {
	var row ReportDelivery
	if err := db.conn.First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (db *DB) GetReportDeliveriesForSchedule(scheduleID uint) ([]ReportDelivery, error) {
	var rows []ReportDelivery
	err := db.conn.Where("schedule_id = ?", scheduleID).Order("id ASC").Find(&rows).Error
	return rows, err
}

// ClaimReportDelivery obtains or re-obtains an expired lease. It increments the
// attempt count but never mutates the owning schedule.
func (db *DB) ClaimReportDelivery(id uint, leaseToken string, now time.Time, leaseDuration time.Duration, maxAttempts int) (*ReportDelivery, error) {
	if strings.TrimSpace(leaseToken) == "" || leaseDuration <= 0 || maxAttempts <= 0 {
		return nil, errors.New("valid report delivery lease parameters are required")
	}
	expires := now.Add(leaseDuration)
	result := db.conn.Model(&ReportDelivery{}).
		Where("id = ? AND attempt_count < ? AND ((state IN ? AND next_attempt_at <= ?) OR (state = ? AND lease_expires_at <= ?))",
			id, maxAttempts,
			[]ReportDeliveryState{ReportDeliveryStatePending, ReportDeliveryStateRetryWait}, now,
			ReportDeliveryStateLeased, now).
		Updates(map[string]any{
			"state":            ReportDeliveryStateLeased,
			"lease_token":      leaseToken,
			"lease_expires_at": expires,
			"attempt_count":    gorm.Expr("attempt_count + 1"),
			"updated_at":       now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return db.GetReportDeliveryByID(id)
}

// RetryReportDelivery releases a lease and makes the row eligible at the
// supplied bounded-backoff time. A stale worker cannot overwrite a newer lease.
func (db *DB) RetryReportDelivery(id uint, leaseToken string, nextAttempt time.Time, lastError string) (bool, error) {
	result := db.conn.Model(&ReportDelivery{}).
		Where("id = ? AND state = ? AND lease_token = ?", id, ReportDeliveryStateLeased, leaseToken).
		Updates(map[string]any{
			"state":            ReportDeliveryStateRetryWait,
			"lease_token":      "",
			"lease_expires_at": nil,
			"next_attempt_at":  nextAttempt,
			"last_error":       truncateReportDeliveryError(lastError),
			"updated_at":       time.Now().UTC(),
		})
	return result.RowsAffected == 1, result.Error
}

// DeadLetterReportDelivery records a terminal failure without advancing the
// regular schedule. Manual recovery can inspect and replay the durable row.
func (db *DB) DeadLetterReportDelivery(id uint, leaseToken string, failedAt time.Time, lastError string) (bool, error) {
	result := db.conn.Model(&ReportDelivery{}).
		Where("id = ? AND state = ? AND lease_token = ?", id, ReportDeliveryStateLeased, leaseToken).
		Updates(map[string]any{
			"state":            ReportDeliveryStateDeadLetter,
			"lease_token":      "",
			"lease_expires_at": nil,
			"next_attempt_at":  failedAt,
			"last_error":       truncateReportDeliveryError(lastError),
			"updated_at":       failedAt,
		})
	return result.RowsAffected == 1, result.Error
}

// DeadLetterExpiredReportDelivery closes the otherwise-stuck state where a
// worker crashes during its final allowed attempt and its lease later expires.
// The outcome is deliberately described as unknown for operator review.
func (db *DB) DeadLetterExpiredReportDelivery(id uint, failedAt time.Time, maxAttempts int, lastError string) (bool, error) {
	result := db.conn.Model(&ReportDelivery{}).
		Where("id = ? AND state = ? AND attempt_count >= ? AND lease_expires_at <= ?", id, ReportDeliveryStateLeased, maxAttempts, failedAt).
		Updates(map[string]any{
			"state":            ReportDeliveryStateDeadLetter,
			"lease_token":      "",
			"lease_expires_at": nil,
			"next_attempt_at":  failedAt,
			"last_error":       truncateReportDeliveryError(lastError),
			"updated_at":       failedAt,
		})
	return result.RowsAffected == 1, result.Error
}

// AcknowledgeReportDelivery commits both the sent state and the schedule
// advancement. If either write fails, neither becomes visible.
func (db *DB) AcknowledgeReportDelivery(id uint, leaseToken string, sentAt, nextSendAt time.Time) (bool, error) {
	won := false
	err := db.conn.Transaction(func(tx *gorm.DB) error {
		var row ReportDelivery
		if err := tx.Where("id = ? AND state = ? AND lease_token = ?", id, ReportDeliveryStateLeased, leaseToken).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		result := tx.Model(&ReportDelivery{}).
			Where("id = ? AND state = ? AND lease_token = ?", id, ReportDeliveryStateLeased, leaseToken).
			Updates(map[string]any{
				"state":            ReportDeliveryStateSent,
				"sent_at":          sentAt,
				"lease_token":      "",
				"lease_expires_at": nil,
				"last_error":       "",
				"updated_at":       sentAt,
			})
		if result.Error != nil || result.RowsAffected != 1 {
			return result.Error
		}
		// Preserve a newer operator configuration written while transport was in
		// flight. The old occurrence is sent, but only an unchanged/due schedule
		// advances to the worker's calculated next occurrence.
		if err := tx.Model(&ReportSchedule{}).Where("id = ?", row.ScheduleID).Updates(map[string]any{
			"last_sent_at": sentAt,
			"next_send_at": gorm.Expr("CASE WHEN next_send_at <= ? THEN ? ELSE next_send_at END", row.ScheduledFor, nextSendAt),
			"updated_at":   sentAt,
		}).Error; err != nil {
			return err
		}
		won = true
		return nil
	})
	return won, err
}

func truncateReportDeliveryError(message string) string {
	message = strings.TrimSpace(message)
	const max = 1000
	if len(message) > max {
		return message[:max]
	}
	return message
}
