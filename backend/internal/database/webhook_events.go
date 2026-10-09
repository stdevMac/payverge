package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateWebhookEventIfNotExists inserts a webhook event record for idempotency.
// It returns duplicate=true when an event with the same provider+webhook_id already exists.
func (db *DB) CreateWebhookEventIfNotExists(event *WebhookEvent) (duplicate bool, err error) {
	if event == nil {
		return false, fmt.Errorf("event is nil")
	}

	if event.ReceivedAt.IsZero() {
		event.ReceivedAt = time.Now()
	}

	result := db.conn.
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "provider"}, {Name: "webhook_id"}},
			DoNothing: true,
		}).
		Create(event)
	if result.Error != nil {
		return false, fmt.Errorf("failed to create webhook event: %w", result.Error)
	}

	return result.RowsAffected == 0, nil
}

// GetWebhookEvent returns a webhook event by provider and webhook ID.
func (db *DB) GetWebhookEvent(provider, webhookID string) (*WebhookEvent, error) {
	var event WebhookEvent
	err := db.conn.
		Where("provider = ? AND webhook_id = ?", provider, webhookID).
		First(&event).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get webhook event: %w", err)
	}
	return &event, nil
}

// MarkWebhookEventProcessed marks webhook processing as successful.
func (db *DB) MarkWebhookEventProcessed(provider, webhookID string) error {
	now := time.Now()
	return db.conn.Model(&WebhookEvent{}).
		Where("provider = ? AND webhook_id = ?", provider, webhookID).
		Updates(map[string]interface{}{
			"status":       "processed",
			"processed_at": &now,
			"error":        "",
		}).Error
}

// WebhookEventStatusRetryPending marks a verified event that could not be
// applied yet because a prerequisite has not landed (a refund, dispute or
// reversal that arrived before its capture was recorded in the ledger). It is
// reprocessable like "failed", so the provider's redelivery is claimed and run
// again instead of being deduplicated away, but it is not an error: it stays
// out of the failed-events admin list and the retention janitor keeps it. A
// row the provider stops redelivering is expired by the capture-pending
// sweeper (handlers.SweepExpiredPluginCapturePendingWebhooks).
const WebhookEventStatusRetryPending = "retry_pending"

// MarkWebhookEventRetryPending records that the event must be redelivered by
// the provider before it can be applied. reason is stored in error for
// forensics.
func (db *DB) MarkWebhookEventRetryPending(provider, webhookID, reason string) error {
	return db.conn.Model(&WebhookEvent{}).
		Where("provider = ? AND webhook_id = ?", provider, webhookID).
		Updates(map[string]interface{}{
			"status":       WebhookEventStatusRetryPending,
			"processed_at": nil,
			"error":        reason,
		}).Error
}

// MarkWebhookEventFailed marks webhook processing as failed and stores an error.
func (db *DB) MarkWebhookEventFailed(provider, webhookID, errMsg string) error {
	now := time.Now()
	return db.conn.Model(&WebhookEvent{}).
		Where("provider = ? AND webhook_id = ?", provider, webhookID).
		Updates(map[string]interface{}{
			"status":       "failed",
			"processed_at": &now,
			"error":        errMsg,
		}).Error
}

// GetFailedWebhookEvents returns all failed webhook events for a provider, ordered by most recent first.
func (db *DB) GetFailedWebhookEvents(provider string) ([]WebhookEvent, error) {
	var events []WebhookEvent
	err := db.conn.
		Where("provider = ? AND status = ?", provider, "failed").
		Order("created_at DESC").
		Limit(50).
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get failed webhook events: %w", err)
	}
	return events, nil
}

// GetAllFailedWebhookEvents returns failed webhook events across every provider.
func (db *DB) GetAllFailedWebhookEvents(limit int) ([]WebhookEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var events []WebhookEvent
	err := db.conn.
		Where("status = ?", "failed").
		Order("created_at DESC").
		Limit(limit).
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get failed webhook events: %w", err)
	}
	return events, nil
}

// GetWebhookEventByID returns a webhook event by its primary key.
func (db *DB) GetWebhookEventByID(id uint) (*WebhookEvent, error) {
	var event WebhookEvent
	err := db.conn.First(&event, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get webhook event: %w", err)
	}
	return &event, nil
}

// StaleWebhookProcessingAfter is how long a webhook event may sit in
// "processing" before it is treated as crashed mid-flight and made
// reprocessable. A handler completes in seconds; a row still "processing" after
// this was abandoned by a killed process between row-create and the next Mark*.
const StaleWebhookProcessingAfter = 5 * time.Minute

// WebhookEventReprocessable reports whether a duplicate webhook delivery should
// be reprocessed rather than treated as already-handled: a failed event (the
// provider is retrying it) or a stale "processing" event (the prior attempt
// crashed before recording an outcome). A fresh "processing" event is genuinely
// in flight and must not be reprocessed. Settlement is independently idempotent,
// so a reprocess is safe.
func WebhookEventReprocessable(e *WebhookEvent, now time.Time) bool {
	if e == nil {
		return false
	}
	switch e.Status {
	case "failed", WebhookEventStatusRetryPending:
		return true
	case "processing":
		return now.Sub(e.ReceivedAt) > StaleWebhookProcessingAfter
	default:
		return false
	}
}

// ResetWebhookEventForRetry resets a failed event to "processing" so it can be retried.
func (db *DB) ResetWebhookEventForRetry(id uint) error {
	return db.conn.Model(&WebhookEvent{}).
		Where("id = ? AND status = ?", id, "failed").
		Updates(map[string]interface{}{
			"status":       "processing",
			"processed_at": nil,
			"error":        "",
		}).Error
}

// ClaimWebhookEventForProcessing CAS-claims a reprocessable webhook event so
// concurrent provider retries cannot both re-enter settlement side effects.
// Succeeds (claimed=true) only when exactly one row transitions into
// "processing" from status=failed, status=retry_pending, or a stale
// status=processing row
// (received_at older than StaleWebhookProcessingAfter). Callers must treat
// claimed=false as already-handled and return 200 without reprocessing.
func (db *DB) ClaimWebhookEventForProcessing(id uint, now time.Time) (claimed bool, err error) {
	if id == 0 {
		return false, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	staleBefore := now.Add(-StaleWebhookProcessingAfter)
	res := db.conn.Model(&WebhookEvent{}).
		Where("id = ? AND (status IN ? OR (status = ? AND received_at < ?))",
			id, []string{"failed", WebhookEventStatusRetryPending}, "processing", staleBefore).
		Updates(map[string]interface{}{
			"status":       "processing",
			"processed_at": nil,
			"error":        "",
			// Bump received_at so a concurrent claim cannot re-match the stale
			// window while this attempt is in flight.
			"received_at": now,
		})
	if res.Error != nil {
		return false, fmt.Errorf("failed to claim webhook event: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}

// MarkWebhookEventIgnored acknowledges a failed webhook after admin review.
func (db *DB) MarkWebhookEventIgnored(id uint, reason string) (*WebhookEvent, error) {
	now := time.Now()
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "acknowledged by admin"
	}
	err := db.conn.Model(&WebhookEvent{}).
		Where("id = ? AND status = ?", id, "failed").
		Updates(map[string]interface{}{
			"status":       "ignored",
			"processed_at": &now,
			"error":        "ignored by admin: " + reason,
		}).Error
	if err != nil {
		return nil, fmt.Errorf("failed to acknowledge webhook event: %w", err)
	}
	return db.GetWebhookEventByID(id)
}

// DeleteProcessedWebhookEventsBefore deletes up to `limit` webhook_events rows
// whose status is 'processed' and that were received before cutoff. 'failed'
// rows are intentionally retained for forensics. Callers loop until a pass
// deletes 0. Rides idx_webhook_events_status_received.
func DeleteProcessedWebhookEventsBefore(cutoff time.Time, limit int) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	if limit <= 0 {
		limit = 500
	}
	var ids []uint
	if err := db.Model(&WebhookEvent{}).
		Where("status = ? AND received_at < ?", "processed", cutoff.UTC()).
		Order("received_at ASC").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return 0, fmt.Errorf("failed to select expired webhook events: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	res := db.Where("id IN ?", ids).Delete(&WebhookEvent{})
	if res.Error != nil {
		return 0, fmt.Errorf("failed to delete expired webhook events: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// ListRetryPendingWebhookEventsBefore returns up to limit retry_pending rows
// first stored before cutoff, oldest first. It reads only the columns the
// capture-pending sweeper needs, never the stored payload.
func (db *DB) ListRetryPendingWebhookEventsBefore(cutoff time.Time, limit int) ([]WebhookEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var events []WebhookEvent
	err := db.conn.
		Select("id", "provider", "webhook_id", "event_type", "status", "error", "created_at").
		Where("status = ? AND created_at < ?", WebhookEventStatusRetryPending, cutoff).
		Order("created_at ASC").
		Limit(limit).
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list retry-pending webhook events: %w", err)
	}
	return events, nil
}

// ExpireRetryPendingWebhookEvent acknowledges a retry_pending row that will
// not be retried any further: it moves the row to processed only while it is
// still retry_pending, so a provider redelivery that has already claimed the
// row wins. expired is true only for the call that moved the row.
func (db *DB) ExpireRetryPendingWebhookEvent(id uint, note string) (expired bool, err error) {
	if id == 0 {
		return false, nil
	}
	now := time.Now()
	res := db.conn.Model(&WebhookEvent{}).
		Where("id = ? AND status = ?", id, WebhookEventStatusRetryPending).
		Updates(map[string]interface{}{
			"status":       "processed",
			"processed_at": &now,
			"error":        note,
		})
	if res.Error != nil {
		return false, fmt.Errorf("failed to expire retry-pending webhook event: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}
