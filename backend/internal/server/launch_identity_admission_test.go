package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLaunchIdentityAdmissionTest(t *testing.T) (*runtimecontrol.Service, string) {
	t.Helper()
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&database.User{},
		&runtimecontrol.InviteBatch{},
		&runtimecontrol.InviteClaim{},
		&runtimecontrol.AuditEvent{},
	))
	service := runtimecontrol.New(db)
	SetLaunchIdentityAdmission(func(ctx context.Context, identity, code string, create func(*gorm.DB) error) error {
		err := service.RegisterWithInvite(ctx, identity, code, create)
		switch {
		case errors.Is(err, runtimecontrol.ErrInviteRequired):
			return ErrLaunchInviteRequired
		case errors.Is(err, runtimecontrol.ErrInviteExpired):
			return ErrLaunchInviteExpired
		case errors.Is(err, runtimecontrol.ErrCohortFull):
			return ErrLaunchCohortFull
		case errors.Is(err, runtimecontrol.ErrInviteAlreadyClaimed):
			return ErrLaunchInviteAlreadyClaimed
		default:
			return err
		}
	})
	t.Cleanup(func() { SetLaunchIdentityAdmission(nil) })
	_, code, err := service.CreateInviteBatch(context.Background(), runtimecontrol.CreateInviteBatchInput{
		Name: "identity-admission", CohortCap: 4, Owner: "release-owner",
		Reason: "launch test", Actor: "test-owner", ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	return service, code
}

func TestWalletIdentityCreationRequiresInviteButExistingIdentityCanSignIn(t *testing.T) {
	_, code := setupLaunchIdentityAdmissionTest(t)
	const address = "0x1111111111111111111111111111111111111111"

	_, _, err := getOrCreateWalletUserWithInvite(t.Context(), address, structs.RoleUser, "")
	require.ErrorIs(t, err, ErrLaunchInviteRequired)

	var users int64
	require.NoError(t, database.GetDB().Model(&database.User{}).Where("address = ?", address).Count(&users).Error)
	require.Zero(t, users, "an uninvited SIWE identity must not be persisted")

	user, created, err := getOrCreateWalletUserWithInvite(t.Context(), address, structs.RoleUser, code)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, address, user.Address)

	user, created, err = getOrCreateWalletUserWithInvite(t.Context(), address, structs.RoleUser, "")
	require.NoError(t, err, "existing identities do not need to spend another invite")
	require.False(t, created)
	require.Equal(t, address, user.Address)
}

func TestWalletAdmissionRejectsArchivedIdentity(t *testing.T) {
	setupLaunchIdentityAdmissionTest(t)
	const address = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	deleted := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, database.GetDB().Create(&database.User{
		Address:   address,
		Name:      "Archived Wallet",
		Role:      "user",
		DeletedAt: &deleted,
	}).Error)

	user, created, err := getOrCreateWalletUserWithInvite(t.Context(), address, structs.RoleUser, "")
	require.ErrorIs(t, err, ErrArchivedWalletIdentity)
	require.False(t, created)
	require.Empty(t, user.Address)

	var persisted database.User
	require.NoError(t, database.GetDB().Where("address = ?", address).First(&persisted).Error)
	require.NotNil(t, persisted.DeletedAt)
	require.Equal(t, deleted.UTC(), persisted.DeletedAt.UTC())
	require.Equal(t, "Archived Wallet", persisted.Name)
	require.Equal(t, "user", persisted.Role)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.User{}).Where("address = ?", address).Count(&count).Error)
	require.Equal(t, int64(1), count, "archived admission must not create or restore a row")
}
