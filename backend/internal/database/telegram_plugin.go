package database

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrTelegramConnectionTokenUnavailable = errors.New("telegram connection token unavailable")

type PluginNotificationDeliveryStatus string

const (
	PluginNotificationDeliveryStatusPending    PluginNotificationDeliveryStatus = "pending"
	PluginNotificationDeliveryStatusProcessing PluginNotificationDeliveryStatus = "processing"
	PluginNotificationDeliveryStatusDelivered  PluginNotificationDeliveryStatus = "delivered"
	PluginNotificationDeliveryStatusRetry      PluginNotificationDeliveryStatus = "retry"
	PluginNotificationDeliveryStatusFailed     PluginNotificationDeliveryStatus = "failed"
	PluginNotificationDeliveryStatusDropped    PluginNotificationDeliveryStatus = "dropped"
)

type TelegramConnectionToken struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	BusinessID       uint       `gorm:"not null;index:idx_telegram_connection_tokens_business_pending,priority:1" json:"business_id"`
	TokenHash        string     `gorm:"not null;uniqueIndex" json:"token_hash"`
	ExpiresAt        time.Time  `gorm:"not null;index;index:idx_telegram_connection_tokens_business_pending,priority:2" json:"expires_at"`
	UsedAt           *time.Time `json:"used_at,omitempty"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	UsedByChatID     *int64     `json:"used_by_chat_id,omitempty"`
	UsedByUsername   string     `json:"used_by_username,omitempty"`
	CreatedByUserID  *uint      `json:"created_by_user_id,omitempty"`
	CreatedByStaffID *uint      `json:"created_by_staff_id,omitempty"`
	CreatedIP        string     `json:"created_ip,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`

	Business Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

type TelegramUpdateReceipt struct {
	UpdateID    int64     `gorm:"primaryKey" json:"update_id"`
	BusinessID  *uint     `gorm:"index" json:"business_id,omitempty"`
	ChatID      *int64    `json:"chat_id,omitempty"`
	Status      string    `gorm:"not null" json:"status"`
	ErrorCode   string    `json:"error_code,omitempty"`
	ProcessedAt time.Time `gorm:"not null;index" json:"processed_at"`

	Business Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

type PluginNotificationDelivery struct {
	ID               uint                             `gorm:"primaryKey" json:"id"`
	BusinessID       uint                             `gorm:"not null;index;uniqueIndex:idx_plugin_notification_unique,priority:1" json:"business_id"`
	PluginName       string                           `gorm:"not null;index;uniqueIndex:idx_plugin_notification_unique,priority:2" json:"plugin_name"`
	EventType        string                           `gorm:"not null;uniqueIndex:idx_plugin_notification_unique,priority:3" json:"event_type"`
	EventID          string                           `gorm:"not null;uniqueIndex:idx_plugin_notification_unique,priority:4" json:"event_id"`
	Status           PluginNotificationDeliveryStatus `gorm:"not null;default:'pending';index" json:"status"`
	Payload          map[string]interface{}           `gorm:"serializer:json" json:"payload"`
	AttemptCount     int                              `gorm:"not null;default:0" json:"attempt_count"`
	NextAttemptAt    time.Time                        `gorm:"not null;index" json:"next_attempt_at"`
	LastAttemptAt    *time.Time                       `json:"last_attempt_at,omitempty"`
	DeliveredAt      *time.Time                       `json:"delivered_at,omitempty"`
	FailedAt         *time.Time                       `json:"failed_at,omitempty"`
	LastErrorCode    string                           `json:"last_error_code,omitempty"`
	LastErrorMessage string                           `json:"last_error_message,omitempty"`
	CreatedAt        time.Time                        `gorm:"index" json:"created_at"`
	UpdatedAt        time.Time                        `json:"updated_at"`

	Business Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

type PluginNotificationDeliveryAttempt struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	DeliveryID        uint      `gorm:"not null;index" json:"delivery_id"`
	AttemptNumber     int       `gorm:"not null" json:"attempt_number"`
	Status            string    `gorm:"not null" json:"status"`
	ProviderMessageID string    `json:"provider_message_id,omitempty"`
	ErrorCode         string    `json:"error_code,omitempty"`
	ErrorMessage      string    `json:"error_message,omitempty"`
	AttemptedAt       time.Time `gorm:"not null;index" json:"attempted_at"`

	Delivery PluginNotificationDelivery `gorm:"foreignKey:DeliveryID" json:"delivery,omitempty"`
}

func CreateTelegramConnectionToken(token *TelegramConnectionToken) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now().UTC()
	}
	if err := db.Create(token).Error; err != nil {
		return fmt.Errorf("failed to create telegram connection token: %w", err)
	}
	return nil
}

func RevokePendingTelegramConnectionTokens(businessID uint) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	now := time.Now().UTC()
	if err := db.Model(&TelegramConnectionToken{}).
		Where("business_id = ? AND used_at IS NULL AND revoked_at IS NULL", businessID).
		Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("failed to revoke telegram connection tokens: %w", err)
	}
	return nil
}

func ConsumeTelegramConnectionTokenTx(tx *gorm.DB, tokenHash string, chatID int64, username string, now time.Time) (*TelegramConnectionToken, error) {
	var consumed TelegramConnectionToken
	if err := tx.Where("token_hash = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?", tokenHash, now).
		First(&consumed).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTelegramConnectionTokenUnavailable
		}
		return nil, err
	}

	updates := map[string]interface{}{
		"used_at":          now,
		"used_by_chat_id":  chatID,
		"used_by_username": username,
	}
	result := tx.Model(&TelegramConnectionToken{}).
		Where("id = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?", consumed.ID, now).
		Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrTelegramConnectionTokenUnavailable
	}

	consumed.UsedAt = &now
	consumed.UsedByChatID = &chatID
	consumed.UsedByUsername = username
	return &consumed, nil
}

func GetPendingTelegramConnectionToken(businessID uint, now time.Time) (*TelegramConnectionToken, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var token TelegramConnectionToken
	if err := db.Where("business_id = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?", businessID, now).
		Order("created_at DESC").
		First(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

func CountRecentTelegramConnectionTokens(businessID uint, since time.Time) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	var count int64
	if err := db.Model(&TelegramConnectionToken{}).
		Where("business_id = ? AND created_at >= ?", businessID, since).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count recent telegram connection tokens: %w", err)
	}
	return count, nil
}

func RecordTelegramUpdateReceipt(receipt *TelegramUpdateReceipt) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	if receipt.ProcessedAt.IsZero() {
		receipt.ProcessedAt = time.Now().UTC()
	}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(receipt)
	if result.Error != nil {
		return false, fmt.Errorf("failed to record telegram update receipt: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// DeleteTelegramUpdateReceipt removes the receipt for an update so a transiently
// failed delivery can be retried by Telegram instead of being deduped away.
func DeleteTelegramUpdateReceipt(updateID int64) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	return db.Where("update_id = ?", updateID).Delete(&TelegramUpdateReceipt{}).Error
}

func CreatePluginNotificationDelivery(delivery *PluginNotificationDelivery) (*PluginNotificationDelivery, bool, error) {
	if db == nil {
		return nil, false, gorm.ErrInvalidDB
	}
	return CreatePluginNotificationDeliveryTx(db, delivery)
}

// CreatePluginNotificationDeliveryTx inserts an outbox delivery on the provided
// gorm handle. When callers pass their domain-write transaction, the outbox row
// commits atomically with the domain change (true transactional outbox): a crash
// between the domain commit and the notification insert can no longer silently
// lose the notification. Passing the package-level db yields the non-tx variant.
func CreatePluginNotificationDeliveryTx(tx *gorm.DB, delivery *PluginNotificationDelivery) (*PluginNotificationDelivery, bool, error) {
	if tx == nil {
		return nil, false, gorm.ErrInvalidDB
	}
	now := time.Now().UTC()
	if delivery.Status == "" {
		delivery.Status = PluginNotificationDeliveryStatusPending
	}
	if delivery.NextAttemptAt.IsZero() {
		delivery.NextAttemptAt = now
	}
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = now
	}
	if delivery.UpdatedAt.IsZero() {
		delivery.UpdatedAt = now
	}

	result := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "business_id"},
			{Name: "plugin_name"},
			{Name: "event_type"},
			{Name: "event_id"},
		},
		DoNothing: true,
	}).Create(delivery)
	if result.Error != nil {
		return nil, false, fmt.Errorf("failed to create plugin notification delivery: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return delivery, true, nil
	}

	var existing PluginNotificationDelivery
	if err := tx.Where("business_id = ? AND plugin_name = ? AND event_type = ? AND event_id = ?",
		delivery.BusinessID, delivery.PluginName, delivery.EventType, delivery.EventID).
		First(&existing).Error; err != nil {
		return nil, false, fmt.Errorf("failed to load existing plugin notification delivery: %w", err)
	}
	return &existing, false, nil
}

func ClaimPluginNotificationDeliveries(pluginName string, limit int, now time.Time) ([]PluginNotificationDelivery, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if limit <= 0 {
		return nil, nil
	}

	var deliveries []PluginNotificationDelivery
	err := db.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("plugin_name = ? AND status IN ? AND next_attempt_at <= ?",
			pluginName,
			[]PluginNotificationDeliveryStatus{
				PluginNotificationDeliveryStatusPending,
				PluginNotificationDeliveryStatusRetry,
			},
			now,
		)
		query = applyPluginNotificationClaimLock(query)
		if err := query.
			Order("next_attempt_at ASC, id ASC").
			Limit(limit).
			Find(&deliveries).Error; err != nil {
			return err
		}
		if len(deliveries) == 0 {
			return nil
		}

		ids := make([]uint, 0, len(deliveries))
		for i := range deliveries {
			ids = append(ids, deliveries[i].ID)
		}

		result := tx.Model(&PluginNotificationDelivery{}).
			Where("id IN ? AND status IN ?", ids, []PluginNotificationDeliveryStatus{
				PluginNotificationDeliveryStatusPending,
				PluginNotificationDeliveryStatusRetry,
			}).
			Updates(map[string]interface{}{
				"status":          PluginNotificationDeliveryStatusProcessing,
				"last_attempt_at": now,
				"attempt_count":   gorm.Expr("attempt_count + ?", 1),
				"updated_at":      now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(deliveries)) {
			return fmt.Errorf("plugin notification claim conflict: claimed %d of %d selected deliveries", result.RowsAffected, len(deliveries))
		}

		for i := range deliveries {
			deliveries[i].Status = PluginNotificationDeliveryStatusProcessing
			deliveries[i].LastAttemptAt = &now
			deliveries[i].AttemptCount++
			deliveries[i].UpdatedAt = now
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to claim plugin notification deliveries: %w", err)
	}
	return deliveries, nil
}

func applyPluginNotificationClaimLock(query *gorm.DB) *gorm.DB {
	if pluginNotificationClaimUsesSkipLocked(query.Name()) {
		return query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
	}
	return query
}

func pluginNotificationClaimUsesSkipLocked(dialect string) bool {
	return dialect == "postgres"
}

func telegramPluginProductionModels() []interface{} {
	return []interface{}{
		&TelegramConnectionToken{},
		&TelegramUpdateReceipt{},
		&PluginNotificationDelivery{},
		&PluginNotificationDeliveryAttempt{},
	}
}

func CountPluginNotificationBacklog(pluginName string, now time.Time) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	var count int64
	if err := db.Model(&PluginNotificationDelivery{}).
		Where("plugin_name = ? AND status IN ? AND next_attempt_at <= ?",
			pluginName,
			[]PluginNotificationDeliveryStatus{
				PluginNotificationDeliveryStatusPending,
				PluginNotificationDeliveryStatusRetry,
			},
			now,
		).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count plugin notification backlog: %w", err)
	}
	return count, nil
}

// CountPluginNotificationPending returns how many deliveries for a plugin are
// still waiting to be attempted (pending or retry), regardless of next_attempt_at.
// Used by the outbox janitor to warn when a backlog exists with no running worker.
func CountPluginNotificationPending(pluginName string) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	var count int64
	if err := db.Model(&PluginNotificationDelivery{}).
		Where("plugin_name = ? AND status IN ?",
			pluginName,
			[]PluginNotificationDeliveryStatus{
				PluginNotificationDeliveryStatusPending,
				PluginNotificationDeliveryStatusRetry,
			},
		).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count pending plugin notifications: %w", err)
	}
	return count, nil
}

// ExpireStalePluginNotificationDeliveries marks pending/retry deliveries created
// before cutoff as dropped, so the outbox self-heals and stays bounded even when
// a delivery worker is disabled or crashed. Returns the number of rows expired.
// Terminal rows (delivered/failed/dropped) and processing rows are left alone.
func ExpireStalePluginNotificationDeliveries(cutoff, now time.Time) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	result := db.Model(&PluginNotificationDelivery{}).
		Where("status IN ? AND created_at < ?",
			[]PluginNotificationDeliveryStatus{
				PluginNotificationDeliveryStatusPending,
				PluginNotificationDeliveryStatusRetry,
			},
			cutoff.UTC(),
		).
		Updates(map[string]interface{}{
			"status":             PluginNotificationDeliveryStatusDropped,
			"failed_at":          now.UTC(),
			"last_error_code":    "expired",
			"last_error_message": "expired by outbox janitor (TTL exceeded)",
			"updated_at":         now.UTC(),
		})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to expire stale plugin notifications: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// ReclaimStaleProcessingPluginNotificationDeliveries rescues rows stranded in the
// "processing" state by a worker that crashed/restarted between claiming a batch
// (status -> processing, attempt_count++) and writing terminal state. Neither the
// claim (selects pending/retry only) nor the outbox janitor (skips processing
// rows) would ever touch these again, so they leak forever and their notification
// is silently lost.
//
// A row is presumed orphaned when its last_attempt_at (the claim timestamp) is
// older than now-staleAfter. staleAfter MUST exceed any legitimate send so an
// actively-sending row is never reclaimed mid-flight. Rows still under the attempt
// cap are moved back to "retry" with next_attempt_at = now so they are re-claimed
// promptly; rows that already reached maxAttempts go terminal ("failed") instead
// of looping forever — mirroring the worker's own exhaustion check
// (delivery.AttemptCount >= maxAttempts). Returns the number of rows reclaimed.
func ReclaimStaleProcessingPluginNotificationDeliveries(pluginName string, staleAfter time.Duration, maxAttempts int, now time.Time) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	now = now.UTC()
	cutoff := now.Add(-staleAfter)

	var total int64
	err := db.Transaction(func(tx *gorm.DB) error {
		// Exhausted orphans -> terminal failure. attempt_count was already
		// incremented at claim time, so >= maxAttempts means no retries remain.
		failed := tx.Model(&PluginNotificationDelivery{}).
			Where("plugin_name = ? AND status = ? AND last_attempt_at < ? AND attempt_count >= ?",
				pluginName,
				PluginNotificationDeliveryStatusProcessing,
				cutoff,
				maxAttempts,
			).
			Updates(map[string]interface{}{
				"status":             PluginNotificationDeliveryStatusFailed,
				"failed_at":          now,
				"last_error_code":    "reclaimed_exhausted",
				"last_error_message": "reclaimed: worker presumed crashed mid-send, attempts exhausted",
				"updated_at":         now,
			})
		if failed.Error != nil {
			return failed.Error
		}
		total += failed.RowsAffected

		// Remaining orphans (under the cap) -> retry, claimable immediately.
		retry := tx.Model(&PluginNotificationDelivery{}).
			Where("plugin_name = ? AND status = ? AND last_attempt_at < ? AND attempt_count < ?",
				pluginName,
				PluginNotificationDeliveryStatusProcessing,
				cutoff,
				maxAttempts,
			).
			Updates(map[string]interface{}{
				"status":             PluginNotificationDeliveryStatusRetry,
				"next_attempt_at":    now,
				"last_error_code":    "reclaimed_stale",
				"last_error_message": "reclaimed: worker presumed crashed mid-send",
				"updated_at":         now,
			})
		if retry.Error != nil {
			return retry.Error
		}
		total += retry.RowsAffected
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to reclaim stale processing plugin notifications: %w", err)
	}
	return total, nil
}

// MarkPluginNotificationDeliveryDeliveredAttempt marks one delivery delivered
// and records the attempt row under the given attempt number, avoiding the
// attempt-count re-read updatePluginNotificationDeliveryFinal does when passed
// attempt 0. Used by the worker's per-message delivered-mark (N-4), where the
// claimed delivery already carries its (post-claim) attempt count.
func MarkPluginNotificationDeliveryDeliveredAttempt(id uint, attempt int, providerMessageID string, now time.Time) error {
	return updatePluginNotificationDeliveryFinal(id, PluginNotificationDeliveryStatusDelivered, providerMessageID, "", "", &now, nil, now, attempt)
}

func MarkPluginNotificationDeliveryRetry(id uint, attempt int, code string, message string, nextAttempt time.Time, now time.Time) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&PluginNotificationDelivery{}).Where("id = ?", id).Updates(map[string]interface{}{
			"status":             PluginNotificationDeliveryStatusRetry,
			"next_attempt_at":    nextAttempt,
			"last_error_code":    code,
			"last_error_message": message,
			"updated_at":         now,
		}).Error; err != nil {
			return err
		}
		return createPluginNotificationDeliveryAttempt(tx, id, attempt, string(PluginNotificationDeliveryStatusRetry), "", code, message, now)
	})
}

func MarkPluginNotificationDeliveryFailed(id uint, attempt int, code string, message string, now time.Time) error {
	return updatePluginNotificationDeliveryFinal(id, PluginNotificationDeliveryStatusFailed, "", code, message, nil, &now, now, attempt)
}

func MarkPluginNotificationDeliveryDropped(id uint, attempt int, code string, message string, now time.Time) error {
	return updatePluginNotificationDeliveryFinal(id, PluginNotificationDeliveryStatusDropped, "", code, message, nil, &now, now, attempt)
}

func updatePluginNotificationDeliveryFinal(id uint, status PluginNotificationDeliveryStatus, providerMessageID, code, message string, deliveredAt, failedAt *time.Time, now time.Time, attempts ...int) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	attempt := 0
	if len(attempts) > 0 {
		attempt = attempts[0]
	}
	return db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"status":             status,
			"last_error_code":    code,
			"last_error_message": message,
			"updated_at":         now,
		}
		if deliveredAt != nil {
			updates["delivered_at"] = *deliveredAt
		}
		if failedAt != nil {
			updates["failed_at"] = *failedAt
		}
		if err := tx.Model(&PluginNotificationDelivery{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return createPluginNotificationDeliveryAttempt(tx, id, attempt, string(status), providerMessageID, code, message, now)
	})
}

func createPluginNotificationDeliveryAttempt(tx *gorm.DB, deliveryID uint, attempt int, status, providerMessageID, code, message string, now time.Time) error {
	if attempt <= 0 {
		var delivery PluginNotificationDelivery
		if err := tx.Select("attempt_count").First(&delivery, deliveryID).Error; err != nil {
			return err
		}
		attempt = delivery.AttemptCount
	}

	return tx.Create(&PluginNotificationDeliveryAttempt{
		DeliveryID:        deliveryID,
		AttemptNumber:     attempt,
		Status:            status,
		ProviderMessageID: providerMessageID,
		ErrorCode:         code,
		ErrorMessage:      message,
		AttemptedAt:       now,
	}).Error
}
