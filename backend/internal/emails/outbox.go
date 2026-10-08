package emails

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Transactional email outbox (#551).
//
// This mirrors the Telegram outbox (database.PluginNotificationDelivery +
// services.PluginNotificationWorker): a durable row is written before the
// provider is ever called, a background worker claims due rows, and failures
// walk a bounded backoff schedule into a terminal failed/dropped state instead
// of vanishing into a log line.

const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusSent       = "sent"
	OutboxStatusFailed     = "failed"
	OutboxStatusDropped    = "dropped"
)

// ErrOutboxDuplicate is returned by Enqueue when a row with the same
// idempotency key is already queued (or already delivered). The send is a
// no-op, not a failure.
var ErrOutboxDuplicate = errors.New("email outbox: duplicate idempotency key")

// EmailOutbox is one durable queued send. Payload carries the fully rendered
// message so a process restart between enqueue and delivery cannot lose it.
type EmailOutbox struct {
	ID                uint         `gorm:"primaryKey"`
	IdempotencyKey    string       `gorm:"size:255;not null;uniqueIndex"`
	Status            string       `gorm:"size:16;not null;default:'pending';index:idx_email_outbox_due,priority:1"`
	Provider          string       `gorm:"size:32;not null;default:''"`
	ProviderMessageID string       `gorm:"size:255;not null;default:''"`
	TemplateName      string       `gorm:"size:128;not null;default:''"`
	Tag               string       `gorm:"size:64;not null;default:''"`
	MessageType       string       `gorm:"size:32;not null;default:''"`
	RecipientRedacted string       `gorm:"size:320;not null;default:''"`
	Payload           EmailMessage `gorm:"serializer:json;type:text;not null"`
	AttemptCount      int          `gorm:"not null;default:0"`
	NextAttemptAt     time.Time    `gorm:"not null;index:idx_email_outbox_due,priority:2"`
	LastAttemptAt     *time.Time
	SentAt            *time.Time
	FailedAt          *time.Time
	LastErrorCode     string `gorm:"size:64;not null;default:''"`
	LastErrorMessage  string `gorm:"type:text;not null;default:''"`

	// Origin references captured at enqueue time (#562). They are what turns a
	// provider bounce webhook — which carries only a message ID — into "the
	// guest never received the confirmation for reservation 4211".
	BusinessID              *uint `gorm:"index"`
	ReservationID           *uint `gorm:"index"`
	BillID                  *uint `gorm:"index"`
	DeliveryOrderID         *uint `gorm:"index"`
	CustomerCommunicationID *uint

	// Provider verdict, written by the delivery webhook.
	DeliveryStatus  string `gorm:"size:24;not null;default:''"`
	DeliveryDetail  string `gorm:"size:255;not null;default:''"`
	DeliveryEventAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Origin returns the entity references captured when this send was queued.
func (r EmailOutbox) Origin() EmailOrigin {
	return EmailOrigin{
		BusinessID:      derefID(r.BusinessID),
		ReservationID:   derefID(r.ReservationID),
		BillID:          derefID(r.BillID),
		DeliveryOrderID: derefID(r.DeliveryOrderID),
	}
}

func (EmailOutbox) TableName() string { return "email_outbox" }

// OutboxStore is the durable queue behind EmailServer.dispatch and the outbox
// worker. Every method is safe to call from multiple processes: Claim locks
// the rows it takes (SKIP LOCKED on Postgres).
type OutboxStore interface {
	Enqueue(ctx context.Context, rec *EmailOutbox) error
	Claim(ctx context.Context, limit int, now time.Time) ([]EmailOutbox, error)
	ReclaimStale(ctx context.Context, staleAfter time.Duration, maxAttempts int, now time.Time) (int64, error)
	MarkSent(ctx context.Context, id uint, provider, providerMessageID string, now time.Time) error
	MarkRetry(ctx context.Context, id uint, code, message string, nextAttempt, now time.Time) error
	MarkTerminal(ctx context.Context, id uint, status, code, message string, now time.Time) error
	CountBacklog(ctx context.Context, now time.Time) (int64, error)
	ExpirePending(ctx context.Context, cutoff, now time.Time) (int64, error)
	PurgeTerminal(ctx context.Context, cutoff time.Time) (int64, error)
}

type GormOutboxStore struct{ db *gorm.DB }

func NewGormOutboxStore(db *gorm.DB) *GormOutboxStore { return &GormOutboxStore{db: db} }

// Enqueue writes the queued send. A duplicate idempotency key returns
// ErrOutboxDuplicate so the caller can treat the send as already accepted.
func (s *GormOutboxStore) Enqueue(ctx context.Context, rec *EmailOutbox) error {
	if s == nil || s.db == nil || rec == nil {
		return gorm.ErrInvalidDB
	}
	if strings.TrimSpace(rec.IdempotencyKey) == "" {
		return errors.New("email outbox: idempotency key is required")
	}
	if rec.Status == "" {
		rec.Status = OutboxStatusPending
	}
	if rec.NextAttemptAt.IsZero() {
		rec.NextAttemptAt = time.Now().UTC()
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(rec)
	if result.Error != nil {
		return fmt.Errorf("failed to enqueue email outbox row: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrOutboxDuplicate
	}
	return nil
}

// Claim atomically moves up to limit due rows into "processing" and increments
// their attempt count, exactly like ClaimPluginNotificationDeliveries.
func (s *GormOutboxStore) Claim(ctx context.Context, limit int, now time.Time) ([]EmailOutbox, error) {
	if s == nil || s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if limit <= 0 {
		return nil, nil
	}
	now = now.UTC()

	var rows []EmailOutbox
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("status = ? AND next_attempt_at <= ?", OutboxStatusPending, now)
		if tx.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := query.Order("next_attempt_at ASC, id ASC").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uint, 0, len(rows))
		for i := range rows {
			ids = append(ids, rows[i].ID)
		}
		result := tx.Model(&EmailOutbox{}).
			Where("id IN ? AND status = ?", ids, OutboxStatusPending).
			Updates(map[string]interface{}{
				"status":          OutboxStatusProcessing,
				"last_attempt_at": now,
				"attempt_count":   gorm.Expr("attempt_count + ?", 1),
				"updated_at":      now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(rows)) {
			return fmt.Errorf("email outbox claim conflict: claimed %d of %d selected rows", result.RowsAffected, len(rows))
		}
		for i := range rows {
			rows[i].Status = OutboxStatusProcessing
			rows[i].LastAttemptAt = &now
			rows[i].AttemptCount++
			rows[i].UpdatedAt = now
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to claim email outbox rows: %w", err)
	}
	return rows, nil
}

// ReclaimStale rescues rows stranded in "processing" by a worker that died
// between claim and terminal write. Rows still under the attempt cap go back to
// pending (claimable now); exhausted ones go terminal so they cannot loop.
// staleAfter MUST exceed a legitimate send duration.
func (s *GormOutboxStore) ReclaimStale(ctx context.Context, staleAfter time.Duration, maxAttempts int, now time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, gorm.ErrInvalidDB
	}
	if maxAttempts <= 0 {
		maxAttempts = defaultOutboxMaxAttempts
	}
	now = now.UTC()
	cutoff := now.Add(-staleAfter)

	var total int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		failed := tx.Model(&EmailOutbox{}).
			Where("status = ? AND last_attempt_at < ? AND attempt_count >= ?", OutboxStatusProcessing, cutoff, maxAttempts).
			Updates(map[string]interface{}{
				"status":             OutboxStatusFailed,
				"failed_at":          now,
				"last_error_code":    "reclaimed_exhausted",
				"last_error_message": "reclaimed: email worker presumed crashed mid-send, attempts exhausted",
				"updated_at":         now,
			})
		if failed.Error != nil {
			return failed.Error
		}
		total += failed.RowsAffected

		retry := tx.Model(&EmailOutbox{}).
			Where("status = ? AND last_attempt_at < ? AND attempt_count < ?", OutboxStatusProcessing, cutoff, maxAttempts).
			Updates(map[string]interface{}{
				"status":             OutboxStatusPending,
				"next_attempt_at":    now,
				"last_error_code":    "reclaimed_stale",
				"last_error_message": "reclaimed: email worker presumed crashed mid-send",
				"updated_at":         now,
			})
		if retry.Error != nil {
			return retry.Error
		}
		total += retry.RowsAffected
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to reclaim stale email outbox rows: %w", err)
	}
	return total, nil
}

// redactedOutboxPayload keeps only the fields a terminal row still needs.
// Bounce webhooks read Subject from the stored payload (attribution.go);
// the recipient, body, headers, and attachments are delivery-time secrets.
func redactedOutboxPayload(msg EmailMessage) EmailMessage {
	return EmailMessage{
		From:           msg.From,
		Subject:        msg.Subject,
		Tag:            msg.Tag,
		MessageType:    msg.MessageType,
		TemplateName:   msg.TemplateName,
		IdempotencyKey: msg.IdempotencyKey,
	}
}

// redactedPayloadJSON loads the row's current payload and returns the JSON
// stub to write back. The column is serializer:json, and map Updates do not
// run that serializer, so the value has to be an encoded JSON string.
func (s *GormOutboxStore) redactedPayloadJSON(ctx context.Context, id uint) (string, error) {
	var current EmailOutbox
	if err := s.db.WithContext(ctx).Select("payload").First(&current, id).Error; err != nil {
		return "", err
	}
	encoded, err := json.Marshal(redactedOutboxPayload(current.Payload))
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// MarkSent records the provider acknowledgement. The provider message ID is the
// join key delivery webhooks use to attribute a bounce back to this send (#562).
// The stored payload is replaced with a redacted stub in the same update.
func (s *GormOutboxStore) MarkSent(ctx context.Context, id uint, provider, providerMessageID string, now time.Time) error {
	if s == nil || s.db == nil {
		return gorm.ErrInvalidDB
	}
	now = now.UTC()
	payload, err := s.redactedPayloadJSON(ctx, id)
	if err != nil {
		return fmt.Errorf("email outbox: redact sent payload: %w", err)
	}
	return s.db.WithContext(ctx).Model(&EmailOutbox{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":              OutboxStatusSent,
		"provider":            strings.ToLower(strings.TrimSpace(provider)),
		"provider_message_id": strings.TrimSpace(providerMessageID),
		"sent_at":             now,
		"last_error_code":     "",
		"last_error_message":  "",
		"payload":             payload,
		"updated_at":          now,
	}).Error
}

func (s *GormOutboxStore) MarkRetry(ctx context.Context, id uint, code, message string, nextAttempt, now time.Time) error {
	if s == nil || s.db == nil {
		return gorm.ErrInvalidDB
	}
	now = now.UTC()
	return s.db.WithContext(ctx).Model(&EmailOutbox{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":             OutboxStatusPending,
		"next_attempt_at":    nextAttempt.UTC(),
		"last_error_code":    code,
		"last_error_message": truncateOutboxError(message),
		"updated_at":         now,
	}).Error
}

func (s *GormOutboxStore) MarkTerminal(ctx context.Context, id uint, status, code, message string, now time.Time) error {
	if s == nil || s.db == nil {
		return gorm.ErrInvalidDB
	}
	now = now.UTC()
	payload, err := s.redactedPayloadJSON(ctx, id)
	if err != nil {
		return fmt.Errorf("email outbox: redact terminal payload: %w", err)
	}
	return s.db.WithContext(ctx).Model(&EmailOutbox{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":             status,
		"failed_at":          now,
		"last_error_code":    code,
		"last_error_message": truncateOutboxError(message),
		"payload":            payload,
		"updated_at":         now,
	}).Error
}

// CountBacklog is the due queue depth (pending rows whose attempt time arrived).
func (s *GormOutboxStore) CountBacklog(ctx context.Context, now time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, gorm.ErrInvalidDB
	}
	var count int64
	err := s.db.WithContext(ctx).Model(&EmailOutbox{}).
		Where("status = ? AND next_attempt_at <= ?", OutboxStatusPending, now.UTC()).
		Count(&count).Error
	return count, err
}

// ExpirePending drops pending rows older than cutoff so the queue stays bounded
// when no worker is running (mirrors the plugin notification janitor).
func (s *GormOutboxStore) ExpirePending(ctx context.Context, cutoff, now time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, gorm.ErrInvalidDB
	}
	now = now.UTC()
	result := s.db.WithContext(ctx).Model(&EmailOutbox{}).
		Where("status = ? AND created_at < ?", OutboxStatusPending, cutoff.UTC()).
		Updates(map[string]interface{}{
			"status":             OutboxStatusDropped,
			"failed_at":          now,
			"last_error_code":    "expired",
			"last_error_message": "expired by email outbox janitor (TTL exceeded)",
			// An expired row was never delivered, so no bounce webhook will
			// read its Subject; drop the recipient, body, and links now
			// instead of holding them until PurgeTerminal.
			"payload":    "{}",
			"updated_at": now,
		})
	return result.RowsAffected, result.Error
}

// PurgeTerminal deletes terminal rows older than cutoff. MarkSent and
// MarkTerminal already replace the payload with a stub (From, Subject, Tag,
// MessageType, TemplateName, IdempotencyKey), so the guest address, body,
// headers, and attachments are not retained. Subject is kept because bounce
// webhooks read it from the row. Terminal rows are still removed after cutoff:
// they are delivery history for attribution, not an archive.
func (s *GormOutboxStore) PurgeTerminal(ctx context.Context, cutoff time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, gorm.ErrInvalidDB
	}
	result := s.db.WithContext(ctx).
		Where("status IN ? AND updated_at < ?",
			[]string{OutboxStatusSent, OutboxStatusFailed, OutboxStatusDropped},
			cutoff.UTC(),
		).
		Delete(&EmailOutbox{})
	return result.RowsAffected, result.Error
}

func truncateOutboxError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 1024 {
		return message[:1024]
	}
	return message
}
