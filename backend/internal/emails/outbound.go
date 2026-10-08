package emails

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	OutboundStatusSent       = "sent"
	OutboundStatusDelivered  = "delivered"
	OutboundStatusBounced    = "bounced"
	OutboundStatusComplained = "complained"
	OutboundStatusOpened     = "opened"
	OutboundStatusClicked    = "clicked"
	OutboundStatusFailed     = "failed"
)

// EmailOutboundSend is the application-owned join row between a Payverge
// send and the provider's message ID. Bounce/delivery webhooks update it
// by (provider, provider_message_id). Recipient is stored redacted only.
type EmailOutboundSend struct {
	ID                uint       `gorm:"primaryKey"`
	Provider          string     `gorm:"size:32;not null;uniqueIndex:idx_email_outbound_provider_msg,priority:1"`
	ProviderMessageID string     `gorm:"size:255;not null;uniqueIndex:idx_email_outbound_provider_msg,priority:2"`
	RecipientRedacted string     `gorm:"size:320;not null;default:''"`
	TemplateName      string     `gorm:"size:128;not null;default:''"`
	Tag               string     `gorm:"size:64;not null;default:''"`
	MessageType       string     `gorm:"size:32;not null;default:''"`
	Status            string     `gorm:"size:32;not null;default:'sent'"`
	LastEventType     string     `gorm:"size:64;not null;default:''"`
	SentAt            time.Time  `gorm:"not null"`
	LastEventAt       *time.Time `gorm:"default:null"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (EmailOutboundSend) TableName() string { return "email_outbound_sends" }

type OutboundStore interface {
	RecordSend(ctx context.Context, rec EmailOutboundSend) error
	ApplyDeliveryEvent(ctx context.Context, provider, providerMessageID, eventType string, occurredAt time.Time) error
}

type GormOutboundStore struct{ db *gorm.DB }

func NewGormOutboundStore(db *gorm.DB) *GormOutboundStore {
	return &GormOutboundStore{db: db}
}

func (s *GormOutboundStore) RecordSend(ctx context.Context, rec EmailOutboundSend) error {
	rec.Provider = strings.ToLower(strings.TrimSpace(rec.Provider))
	rec.ProviderMessageID = strings.TrimSpace(rec.ProviderMessageID)
	if rec.Provider == "" || rec.ProviderMessageID == "" {
		return nil
	}
	if rec.Status == "" {
		rec.Status = OutboundStatusSent
	}
	if rec.SentAt.IsZero() {
		rec.SentAt = time.Now().UTC()
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider"}, {Name: "provider_message_id"}},
		DoNothing: true,
	}).Create(&rec).Error
}

func (s *GormOutboundStore) ApplyDeliveryEvent(ctx context.Context, provider, providerMessageID, eventType string, occurredAt time.Time) error {
	return applyOutboundDeliveryEvent(s.db.WithContext(ctx), provider, providerMessageID, eventType, occurredAt)
}

func applyOutboundDeliveryEvent(db *gorm.DB, provider, providerMessageID, eventType string, occurredAt time.Time) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	providerMessageID = strings.TrimSpace(providerMessageID)
	eventType = strings.TrimSpace(eventType)
	if provider == "" || providerMessageID == "" || eventType == "" {
		return nil
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	updates := map[string]interface{}{
		"last_event_type": eventType,
		"last_event_at":   occurredAt,
	}
	if status := outboundStatusForEvent(eventType); status != "" {
		updates["status"] = status
	}
	return db.Model(&EmailOutboundSend{}).
		Where("provider = ? AND provider_message_id = ?", provider, providerMessageID).
		Updates(updates).Error
}

func outboundStatusForEvent(eventType string) string {
	switch eventType {
	case "email.delivered":
		return OutboundStatusDelivered
	case "email.bounced":
		return OutboundStatusBounced
	case "email.complained":
		return OutboundStatusComplained
	case "email.opened":
		return OutboundStatusOpened
	case "email.clicked":
		return OutboundStatusClicked
	default:
		return ""
	}
}
