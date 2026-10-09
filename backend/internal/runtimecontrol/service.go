// Package runtimecontrol owns durable launch controls and invite-cohort admission.
package runtimecontrol

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ControlKey string

const (
	ControlMaintenance ControlKey = "maintenance_mode"
	ControlReadOnly    ControlKey = "read_only_mode"
	ControlPayments    ControlKey = "payments_enabled"
	ControlFiscal      ControlKey = "fiscal_enabled"
	ControlAI          ControlKey = "ai_enabled"
	ControlUploads     ControlKey = "uploads_enabled"
	ControlGuestOrders ControlKey = "guest_orders_enabled"
)

var (
	ErrInviteRequired       = errors.New("a valid launch invite is required")
	ErrInviteExpired        = errors.New("launch invite is expired or inactive")
	ErrCohortFull           = errors.New("launch cohort is full")
	ErrInviteAlreadyClaimed = errors.New("email already claimed a launch invite")
	ErrInvalidControlChange = errors.New("runtime control owner, reason, actor, and future expiry are required")
	// ErrRegistrationClosed is returned by Admit when REGISTRATION_MODE=closed.
	ErrRegistrationClosed = errors.New("registration is closed on this instance")
)

var knownControls = map[ControlKey]bool{
	ControlMaintenance: false,
	ControlReadOnly:    false,
	ControlPayments:    false,
	ControlFiscal:      false,
	ControlAI:          false,
	ControlUploads:     false,
	ControlGuestOrders: false,
}

type Control struct {
	Key       ControlKey `gorm:"column:key;primaryKey" json:"key"`
	Enabled   bool       `json:"enabled"`
	Owner     string     `json:"owner"`
	Reason    string     `json:"reason"`
	ExpiresAt time.Time  `json:"expires_at"`
	UpdatedBy string     `json:"updated_by"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (Control) TableName() string { return "runtime_controls" }

type AuditEvent struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	ControlKey    string    `json:"control_key"`
	EventType     string    `json:"event_type"`
	OldEnabled    *bool     `json:"old_enabled,omitempty"`
	NewEnabled    *bool     `json:"new_enabled,omitempty"`
	InviteBatchID *uint64   `json:"invite_batch_id,omitempty"`
	Owner         string    `json:"owner"`
	Reason        string    `json:"reason"`
	ExpiresAt     time.Time `json:"expires_at"`
	Actor         string    `json:"actor"`
	CreatedAt     time.Time `json:"created_at"`
}

func (AuditEvent) TableName() string { return "runtime_control_audit_events" }

type InviteBatch struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	Name         string    `json:"name"`
	CodeDigest   string    `json:"-"`
	CohortCap    int       `json:"cohort_cap"`
	ClaimedCount int       `json:"claimed_count"`
	Active       bool      `json:"active"`
	Owner        string    `json:"owner"`
	Reason       string    `json:"reason"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (InviteBatch) TableName() string { return "runtime_invite_batches" }

type InviteClaim struct {
	ID              uint64    `gorm:"primaryKey"`
	InviteBatchID   uint64    `gorm:"column:invite_batch_id"`
	NormalizedEmail string    `gorm:"column:normalized_email"`
	ClaimedAt       time.Time `gorm:"column:claimed_at"`
}

func (InviteClaim) TableName() string { return "runtime_invite_claims" }

type CreateInviteBatchInput struct {
	Name, Owner, Reason, Actor string
	CohortCap                  int
	ExpiresAt                  time.Time
}

type SetControlInput struct {
	Key       ControlKey `json:"key"`
	Enabled   bool       `json:"enabled"`
	Owner     string     `json:"owner"`
	Reason    string     `json:"reason"`
	ExpiresAt time.Time  `json:"expires_at"`
	Actor     string     `json:"-"`
}

type Service struct {
	db  *gorm.DB
	now func() time.Time
	// mode resolves the instance registration mode; nil means
	// config.RegistrationMode (the cached REGISTRATION_MODE env).
	mode func() config.RegistrationModeValue
}

func New(db *gorm.DB) *Service { return &Service{db: db, now: time.Now} }

// WithRegistrationMode returns a copy of the service that resolves the
// registration mode through mode instead of REGISTRATION_MODE. Tests use it.
func (s *Service) WithRegistrationMode(mode func() config.RegistrationModeValue) *Service {
	clone := *s
	clone.mode = mode
	return &clone
}

// RegistrationMode reports the registration mode Admit enforces.
func (s *Service) RegistrationMode() config.RegistrationModeValue {
	if s.mode != nil {
		return s.mode()
	}
	return config.RegistrationMode()
}

// Admit is the single first-identity admission gate for every signup path
// (email, Google, SIWE wallet). It applies REGISTRATION_MODE:
//
//   - closed: no identity is created; returns ErrRegistrationClosed.
//   - open:   create runs in a plain transaction; any invite code is ignored.
//   - invite: RegisterWithInvite (one durable cohort slot per identity).
//
// Existing identities never reach Admit, so sign-in is unaffected by the mode,
// and operator-provisioned admins (ADMIN_EMAIL / `server admin create`) are
// created directly and never consume an invite.
func (s *Service) Admit(ctx context.Context, identity, code string, create func(*gorm.DB) error) error {
	switch s.RegistrationMode() {
	case config.RegistrationModeOpen:
		return s.db.WithContext(ctx).Transaction(create)
	case config.RegistrationModeInvite:
		return s.RegisterWithInvite(ctx, identity, code, create)
	default:
		return ErrRegistrationClosed
	}
}

func (s *Service) Enabled(ctx context.Context, key ControlKey) (bool, error) {
	defaultValue, known := knownControls[key]
	if !known {
		return false, fmt.Errorf("unknown runtime control %q", key)
	}
	var control Control
	err := s.db.WithContext(ctx).Where("key = ?", key).Take(&control).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultValue, nil
	}
	if err != nil {
		return false, err
	}
	if !control.ExpiresAt.After(s.now()) {
		return defaultValue, nil
	}
	return control.Enabled, nil
}

func (s *Service) ListControls(ctx context.Context) ([]Control, error) {
	var controls []Control
	err := s.db.WithContext(ctx).Order("key ASC").Find(&controls).Error
	return controls, err
}

func (s *Service) ListAuditEvents(ctx context.Context, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var events []AuditEvent
	err := s.db.WithContext(ctx).Order("id DESC").Limit(limit).Find(&events).Error
	return events, err
}

func (s *Service) SetControl(ctx context.Context, input SetControlInput) error {
	if _, ok := knownControls[input.Key]; !ok || strings.TrimSpace(input.Owner) == "" || strings.TrimSpace(input.Reason) == "" || strings.TrimSpace(input.Actor) == "" || !input.ExpiresAt.After(s.now()) {
		return ErrInvalidControlChange
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var prior Control
		lookupErr := tx.Where("key = ?", input.Key).Take(&prior).Error
		if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}
		now := s.now().UTC()
		next := Control{Key: input.Key, Enabled: input.Enabled, Owner: strings.TrimSpace(input.Owner), Reason: strings.TrimSpace(input.Reason), ExpiresAt: input.ExpiresAt.UTC(), UpdatedBy: strings.TrimSpace(input.Actor), UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"enabled", "owner", "reason", "expires_at", "updated_by", "updated_at"})}).Create(&next).Error; err != nil {
			return err
		}
		oldValue := prior.Enabled
		if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			oldValue = knownControls[input.Key]
		}
		newValue := input.Enabled
		return tx.Create(&AuditEvent{ControlKey: string(input.Key), EventType: "control_changed", OldEnabled: &oldValue, NewEnabled: &newValue, Owner: next.Owner, Reason: next.Reason, ExpiresAt: next.ExpiresAt, Actor: next.UpdatedBy, CreatedAt: now}).Error
	})
}

func (s *Service) CreateInviteBatch(ctx context.Context, input CreateInviteBatchInput) (*InviteBatch, string, error) {
	if strings.TrimSpace(input.Name) == "" || input.CohortCap <= 0 || strings.TrimSpace(input.Owner) == "" || strings.TrimSpace(input.Reason) == "" || strings.TrimSpace(input.Actor) == "" || !input.ExpiresAt.After(s.now()) {
		return nil, "", ErrInvalidControlChange
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", err
	}
	code := hex.EncodeToString(raw)
	now := s.now().UTC()
	batch := &InviteBatch{Name: strings.TrimSpace(input.Name), CodeDigest: digestCode(code), CohortCap: input.CohortCap, Active: true, Owner: strings.TrimSpace(input.Owner), Reason: strings.TrimSpace(input.Reason), ExpiresAt: input.ExpiresAt.UTC(), CreatedBy: strings.TrimSpace(input.Actor), CreatedAt: now, UpdatedAt: now}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(batch).Error; err != nil {
			return err
		}
		return tx.Create(&AuditEvent{ControlKey: "registration_invites", EventType: "invite_batch_created", InviteBatchID: &batch.ID, Owner: batch.Owner, Reason: batch.Reason, ExpiresAt: batch.ExpiresAt, Actor: batch.CreatedBy, CreatedAt: now}).Error
	})
	if err != nil {
		return nil, "", err
	}
	return batch, code, nil
}

// RegisterWithInvite reserves one cohort slot and runs create inside the same
// transaction. Any user/auth failure rolls back both the claim and cap counter.
func (s *Service) RegisterWithInvite(ctx context.Context, email, code string, create func(*gorm.DB) error) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrInviteRequired
	}
	normalized := strings.ToLower(strings.TrimSpace(email))
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now().UTC()
		var batch InviteBatch
		if err := tx.Where("code_digest = ? AND active = ? AND expires_at > ?", digestCode(code), true, now).Take(&batch).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInviteExpired
			}
			return err
		}
		result := tx.Model(&InviteBatch{}).Where("id = ? AND active = ? AND expires_at > ? AND claimed_count < cohort_cap", batch.ID, true, now).UpdateColumn("claimed_count", gorm.Expr("claimed_count + 1"))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCohortFull
		}
		if err := tx.Create(&InviteClaim{InviteBatchID: batch.ID, NormalizedEmail: normalized, ClaimedAt: now}).Error; err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return ErrInviteAlreadyClaimed
			}
			return err
		}
		return create(tx)
	})
}

// HasInviteClaim reports whether any normalized launch identity has already
// consumed an invitation. Identities may be verified emails or namespaced
// wallet addresses; no raw invite code is retained.
func (s *Service) HasInviteClaim(ctx context.Context, identities ...string) (bool, error) {
	normalized := make([]string, 0, len(identities))
	for _, identity := range identities {
		identity = strings.ToLower(strings.TrimSpace(identity))
		if identity != "" {
			normalized = append(normalized, identity)
		}
	}
	if len(normalized) == 0 {
		return false, nil
	}
	var count int64
	err := s.db.WithContext(ctx).Model(&InviteClaim{}).
		Where("normalized_email IN ?", normalized).
		Count(&count).Error
	return count > 0, err
}

func digestCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
