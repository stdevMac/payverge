package activation

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type State struct {
	BusinessID              uint `gorm:"primaryKey"`
	RegistrationCompletedAt *time.Time
	WorkspaceCreatedAt      *time.Time
	MenuItemCreatedAt       *time.Time
	TableCreatedAt          *time.Time
	QRPreviewedAt           *time.Time
	PaymentConfiguredAt     *time.Time
	StaffInvitedAt          *time.Time
	SetupCompletedAt        *time.Time
	TestOrderCompletedAt    *time.Time
	ActivationAchievedAt    *time.Time
	FirstPaidBillAt         *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func (State) TableName() string { return "business_activation_states" }

type OutboxEvent struct {
	ID               uint `gorm:"primaryKey"`
	EventName        Name `gorm:"column:event_name"`
	SchemaVersion    int  `gorm:"column:schema_version"`
	Origin           Origin
	BusinessID       *uint
	FunnelID         *string
	IdempotencyKey   string          `gorm:"uniqueIndex:idx_activation_event_outbox_idempotency"`
	Dimensions       json.RawMessage `gorm:"type:jsonb"`
	OccurredAt       time.Time
	DeliveryStatus   string
	DeliveryAttempts int
	DeliveredAt      *time.Time
	LastError        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (OutboxEvent) TableName() string { return "activation_event_outbox" }

type Recorder struct{ db *gorm.DB }

func NewRecorder(db *gorm.DB) *Recorder { return &Recorder{db: db} }

func (r *Recorder) RecordClient(ctx context.Context, event Event) (bool, error) {
	if r == nil || r.db == nil {
		return false, gorm.ErrInvalidDB
	}
	if err := Validate(event, ClientOrigin); err != nil {
		return false, err
	}
	row, err := outboxRow(event, ClientOrigin)
	if err != nil {
		return false, err
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).Create(&row)
	return result.RowsAffected == 1, result.Error
}

func outboxRow(event Event, origin Origin) (OutboxEvent, error) {
	dimensions, err := json.Marshal(event.Dimensions)
	if err != nil {
		return OutboxEvent{}, err
	}
	row := OutboxEvent{EventName: event.Name, SchemaVersion: SchemaVersion, Origin: origin, IdempotencyKey: event.IdempotencyKey, Dimensions: dimensions, OccurredAt: event.OccurredAt.UTC(), DeliveryStatus: "pending"}
	if origin == ServerOrigin {
		row.BusinessID = &event.BusinessID
	} else {
		row.FunnelID = &event.FunnelID
	}
	return row, nil
}
