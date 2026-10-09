package runtimecontrol

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newRuntimeControlTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, createTestSchema(db))
	return db
}

func createTestSchema(db *gorm.DB) error {
	return db.Exec(`
		CREATE TABLE runtime_controls (key TEXT PRIMARY KEY, enabled BOOLEAN NOT NULL, owner TEXT NOT NULL, reason TEXT NOT NULL, expires_at DATETIME NOT NULL, updated_by TEXT NOT NULL, updated_at DATETIME NOT NULL);
		CREATE TABLE runtime_invite_batches (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, code_digest TEXT NOT NULL UNIQUE, cohort_cap INTEGER NOT NULL, claimed_count INTEGER NOT NULL DEFAULT 0, active BOOLEAN NOT NULL DEFAULT TRUE, owner TEXT NOT NULL, reason TEXT NOT NULL, expires_at DATETIME NOT NULL, created_by TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL);
		CREATE TABLE runtime_invite_claims (id INTEGER PRIMARY KEY AUTOINCREMENT, invite_batch_id INTEGER NOT NULL, normalized_email TEXT NOT NULL UNIQUE, claimed_at DATETIME NOT NULL);
		CREATE TABLE runtime_control_audit_events (id INTEGER PRIMARY KEY AUTOINCREMENT, control_key TEXT NOT NULL, event_type TEXT NOT NULL, old_enabled BOOLEAN, new_enabled BOOLEAN, invite_batch_id INTEGER, owner TEXT NOT NULL, reason TEXT NOT NULL, expires_at DATETIME NOT NULL, actor TEXT NOT NULL, created_at DATETIME NOT NULL);
	`).Error
}

func TestRegisterWithInviteRejectsUninvitedAndCapsBatch(t *testing.T) {
	db := newRuntimeControlTestDB(t)
	svc := New(db)
	ctx := context.Background()
	_, code, err := svc.CreateInviteBatch(ctx, CreateInviteBatchInput{
		Name: "launch-one", CohortCap: 1, Owner: "launch-owner",
		Reason: "initial cohort", ExpiresAt: time.Now().Add(time.Hour), Actor: "admin@example.test",
	})
	require.NoError(t, err)

	err = svc.RegisterWithInvite(ctx, "uninvited@example.test", "", func(*gorm.DB) error { return nil })
	require.ErrorIs(t, err, ErrInviteRequired)

	require.NoError(t, svc.RegisterWithInvite(ctx, "one@example.test", code, func(*gorm.DB) error { return nil }))
	require.ErrorIs(t, svc.RegisterWithInvite(ctx, "two@example.test", code, func(*gorm.DB) error { return nil }), ErrCohortFull)
}

func TestSetControlRequiresCompleteOwnershipMetadataAndAudits(t *testing.T) {
	db := newRuntimeControlTestDB(t)
	svc := New(db)
	ctx := context.Background()
	err := svc.SetControl(ctx, SetControlInput{Key: ControlPayments, Enabled: false})
	require.ErrorIs(t, err, ErrInvalidControlChange)

	err = svc.SetControl(ctx, SetControlInput{
		Key: ControlPayments, Enabled: false, Owner: "payments-oncall",
		Reason: "sandbox drill", ExpiresAt: time.Now().Add(time.Hour), Actor: "admin@example.test",
	})
	require.NoError(t, err)
	enabled, err := svc.Enabled(ctx, ControlPayments)
	require.NoError(t, err)
	require.False(t, enabled)

	var audits int64
	require.NoError(t, db.Table("runtime_control_audit_events").Where("control_key = ?", ControlPayments).Count(&audits).Error)
	require.Equal(t, int64(1), audits)
}

func TestControlDefaultsAndExpiryAreFailClosed(t *testing.T) {
	db := newRuntimeControlTestDB(t)
	svc := New(db)
	maintenance, err := svc.Enabled(t.Context(), ControlMaintenance)
	require.NoError(t, err)
	require.False(t, maintenance)
	payments, err := svc.Enabled(t.Context(), ControlPayments)
	require.NoError(t, err)
	require.False(t, payments, "missing feature control must fail closed")
	require.NoError(t, db.Create(&Control{Key: ControlPayments, Enabled: true, Owner: "payments", Reason: "expired", ExpiresAt: time.Now().Add(-time.Minute), UpdatedBy: "test", UpdatedAt: time.Now()}).Error)
	payments, err = svc.Enabled(t.Context(), ControlPayments)
	require.NoError(t, err)
	require.False(t, payments, "expired feature control must fail closed")
}
