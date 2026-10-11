package emails

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SuppressionReason string

const (
	SuppressionReasonBounce    SuppressionReason = "bounce"
	SuppressionReasonComplaint SuppressionReason = "complaint"
	SuppressionReasonProvider  SuppressionReason = "provider_suppressed"
	SuppressionReasonManual    SuppressionReason = "manual"
)

var ErrRecipientSuppressed = errors.New("email recipient is suppressed")

// EmailSuppression is the application-owned fail-closed copy of provider
// suppression state. Provider dashboards are not durable application state and
// cannot protect rollback transports from re-sending to a bounced recipient.
type EmailSuppression struct {
	Email           string            `gorm:"primaryKey;size:320"`
	Reason          SuppressionReason `gorm:"size:32;not null"`
	Provider        string            `gorm:"size:32;not null"`
	ProviderEventID string            `gorm:"size:255;not null;default:''"`
	ProviderEmailID string            `gorm:"size:255;not null;default:''"`
	Detail          string            `gorm:"type:text;not null;default:''"`
	SuppressedAt    time.Time         `gorm:"not null"`
	LastEventAt     time.Time         `gorm:"not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (EmailSuppression) TableName() string { return "email_suppressions" }

// EmailDeliveryEvent stores the minimum non-body data needed to make webhook
// ingestion idempotent and auditable. Subjects and raw payloads are excluded.
type EmailDeliveryEvent struct {
	ID              uint   `gorm:"primaryKey"`
	Provider        string `gorm:"size:32;not null;uniqueIndex:idx_email_delivery_provider_webhook,priority:1"`
	WebhookID       string `gorm:"size:255;not null;uniqueIndex:idx_email_delivery_provider_webhook,priority:2"`
	EventType       string `gorm:"size:64;not null;index"`
	ProviderEmailID string `gorm:"size:255;not null;default:'';index"`
	RecipientEmail  string `gorm:"size:320;not null;default:''"`
	OccurredAt      *time.Time
	ReceivedAt      time.Time `gorm:"not null"`
	CreatedAt       time.Time
}

func (EmailDeliveryEvent) TableName() string { return "email_delivery_events" }

type SuppressionStore interface {
	Lookup(ctx context.Context, email string) (*EmailSuppression, error)
}

type GormSuppressionStore struct{ db *gorm.DB }

func NewGormSuppressionStore(db *gorm.DB) *GormSuppressionStore {
	return &GormSuppressionStore{db: db}
}

func canonicalEmail(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, ",;") {
		return "", fmt.Errorf("invalid email address")
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address == "" {
		return "", fmt.Errorf("invalid email address")
	}
	return strings.ToLower(strings.TrimSpace(parsed.Address)), nil
}

func (s *GormSuppressionStore) Lookup(ctx context.Context, email string) (*EmailSuppression, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("suppression database is unavailable")
	}
	canonical, err := canonicalEmail(email)
	if err != nil {
		return nil, err
	}
	var suppression EmailSuppression
	err = s.db.WithContext(ctx).Where("email = ?", canonical).First(&suppression).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &suppression, nil
}

func (s *GormSuppressionStore) Suppress(ctx context.Context, suppression EmailSuppression) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("suppression database is unavailable")
	}
	canonical, err := canonicalEmail(suppression.Email)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	suppression.Email = canonical
	if suppression.SuppressedAt.IsZero() {
		suppression.SuppressedAt = now
	}
	if suppression.LastEventAt.IsZero() {
		suppression.LastEventAt = suppression.SuppressedAt
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "email"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"reason", "provider", "provider_event_id", "provider_email_id",
			"detail", "suppressed_at", "last_event_at", "updated_at",
		}),
	}).Create(&suppression).Error
}
