package runtimecontrol

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func fixedMode(mode config.RegistrationModeValue) func() config.RegistrationModeValue {
	return func() config.RegistrationModeValue { return mode }
}

func countClaims(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&InviteClaim{}).Count(&n).Error)
	return n
}

func TestAdmitClosedCreatesNothing(t *testing.T) {
	db := newRuntimeControlTestDB(t)
	svc := New(db).WithRegistrationMode(fixedMode(config.RegistrationModeClosed))
	ctx := context.Background()
	_, code, err := svc.CreateInviteBatch(ctx, CreateInviteBatchInput{
		Name: "batch", CohortCap: 5, Owner: "owner", Reason: "r", ExpiresAt: time.Now().Add(time.Hour), Actor: "admin",
	})
	require.NoError(t, err)

	called := false
	err = svc.Admit(ctx, "someone@example.test", code, func(*gorm.DB) error { called = true; return nil })
	require.ErrorIs(t, err, ErrRegistrationClosed)
	require.False(t, called, "closed mode must not run the create callback, even with a valid invite")
	require.Zero(t, countClaims(t, db))
}

func TestAdmitOpenIgnoresInvite(t *testing.T) {
	db := newRuntimeControlTestDB(t)
	svc := New(db).WithRegistrationMode(fixedMode(config.RegistrationModeOpen))
	ctx := context.Background()

	called := 0
	require.NoError(t, svc.Admit(ctx, "a@example.test", "", func(*gorm.DB) error { called++; return nil }))
	require.NoError(t, svc.Admit(ctx, "b@example.test", "not-a-real-code", func(*gorm.DB) error { called++; return nil }))
	require.Equal(t, 2, called)
	require.Zero(t, countClaims(t, db), "open mode must not consume invite slots")
}

func TestAdmitOpenRollsBackOnCreateError(t *testing.T) {
	db := newRuntimeControlTestDB(t)
	svc := New(db).WithRegistrationMode(fixedMode(config.RegistrationModeOpen))
	err := svc.Admit(context.Background(), "a@example.test", "", func(tx *gorm.DB) error {
		require.NoError(t, tx.Create(&InviteClaim{InviteBatchID: 1, NormalizedEmail: "x", ClaimedAt: time.Now()}).Error)
		return gorm.ErrInvalidData
	})
	require.ErrorIs(t, err, gorm.ErrInvalidData)
	require.Zero(t, countClaims(t, db), "create runs inside the admission transaction")
}

func TestAdmitInviteRequiresAndConsumesInvite(t *testing.T) {
	db := newRuntimeControlTestDB(t)
	svc := New(db).WithRegistrationMode(fixedMode(config.RegistrationModeInvite))
	ctx := context.Background()
	_, code, err := svc.CreateInviteBatch(ctx, CreateInviteBatchInput{
		Name: "batch", CohortCap: 1, Owner: "owner", Reason: "r", ExpiresAt: time.Now().Add(time.Hour), Actor: "admin",
	})
	require.NoError(t, err)

	require.ErrorIs(t, svc.Admit(ctx, "a@example.test", "", func(*gorm.DB) error { return nil }), ErrInviteRequired)
	require.ErrorIs(t, svc.Admit(ctx, "a@example.test", "bogus", func(*gorm.DB) error { return nil }), ErrInviteExpired)
	require.NoError(t, svc.Admit(ctx, "a@example.test", code, func(*gorm.DB) error { return nil }))
	require.Equal(t, int64(1), countClaims(t, db))
	require.ErrorIs(t, svc.Admit(ctx, "b@example.test", code, func(*gorm.DB) error { return nil }), ErrCohortFull)
}

func TestAdmitDefaultsToConfigRegistrationMode(t *testing.T) {
	db := newRuntimeControlTestDB(t)
	svc := New(db)
	config.SetRegistrationModeForTesting(t, config.RegistrationModeClosed)
	require.Equal(t, config.RegistrationModeClosed, svc.RegistrationMode())
	require.ErrorIs(t, svc.Admit(context.Background(), "a@example.test", "", func(*gorm.DB) error { return nil }), ErrRegistrationClosed)

	config.SetRegistrationModeForTesting(t, config.RegistrationModeOpen)
	require.NoError(t, svc.Admit(context.Background(), "a@example.test", "", func(*gorm.DB) error { return nil }))
}
