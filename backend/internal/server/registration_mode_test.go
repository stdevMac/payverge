package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupRegistrationModeAdmission wires the same mode-aware admission main.go
// installs (runtimecontrol.Admit + error mapping) and returns a valid invite.
func setupRegistrationModeAdmission(t *testing.T, mode config.RegistrationModeValue) string {
	t.Helper()
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&database.User{},
		&runtimecontrol.InviteBatch{},
		&runtimecontrol.InviteClaim{},
		&runtimecontrol.AuditEvent{},
	))
	config.SetRegistrationModeForTesting(t, mode)
	service := runtimecontrol.New(db)
	SetLaunchIdentityAdmission(func(ctx context.Context, identity, code string, create func(*gorm.DB) error) error {
		err := service.Admit(ctx, identity, code, create)
		switch {
		case errors.Is(err, runtimecontrol.ErrRegistrationClosed):
			return ErrRegistrationClosed
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
		Name: "registration-mode", CohortCap: 4, Owner: "owner",
		Reason: "test", Actor: "admin", ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	return code
}

func countWalletUsers(t *testing.T, address string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, database.GetDB().Model(&database.User{}).Where("address = ?", address).Count(&n).Error)
	return n
}

func TestWalletAdmissionClosedModeRefusesNewButAdmitsExisting(t *testing.T) {
	code := setupRegistrationModeAdmission(t, config.RegistrationModeClosed)
	const fresh = "0x2222222222222222222222222222222222222222"
	_, created, err := getOrCreateWalletUserWithInvite(t.Context(), fresh, structs.RoleUser, code)
	require.ErrorIs(t, err, ErrRegistrationClosed)
	require.False(t, created)
	require.Zero(t, countWalletUsers(t, fresh), "closed mode must not persist a new identity even with a valid invite")

	const existing = "0x3333333333333333333333333333333333333333"
	require.NoError(t, database.GetDB().Create(&database.User{Address: existing, Role: "user"}).Error)
	user, created, err := getOrCreateWalletUserWithInvite(t.Context(), existing, structs.RoleUser, "")
	require.NoError(t, err, "existing identities still sign in when registration is closed")
	require.False(t, created)
	require.Equal(t, existing, user.Address)
}

func TestWalletAdmissionOpenModeNeedsNoInvite(t *testing.T) {
	setupRegistrationModeAdmission(t, config.RegistrationModeOpen)
	const fresh = "0x4444444444444444444444444444444444444444"
	user, created, err := getOrCreateWalletUserWithInvite(t.Context(), fresh, structs.RoleUser, "")
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, fresh, user.Address)
	var claims int64
	require.NoError(t, database.GetDB().Model(&runtimecontrol.InviteClaim{}).Count(&claims).Error)
	require.Zero(t, claims, "open mode does not spend invites")
}

func TestWalletAdmissionInviteModeRequiresInvite(t *testing.T) {
	code := setupRegistrationModeAdmission(t, config.RegistrationModeInvite)
	const fresh = "0x5555555555555555555555555555555555555555"
	_, _, err := getOrCreateWalletUserWithInvite(t.Context(), fresh, structs.RoleUser, "")
	require.ErrorIs(t, err, ErrLaunchInviteRequired)
	require.Zero(t, countWalletUsers(t, fresh))
	_, created, err := getOrCreateWalletUserWithInvite(t.Context(), fresh, structs.RoleUser, code)
	require.NoError(t, err)
	require.True(t, created)
}

func TestRespondLaunchInviteAdmissionErrorRegistrationClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.True(t, respondLaunchInviteAdmissionError(c, ErrRegistrationClosed))
	require.Equal(t, http.StatusForbidden, w.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, ErrCodeForbidden, body.Code)
	require.Equal(t, "registration_closed", body.Params["reason"])
}

func TestGetPlatformRegistrationMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []config.RegistrationModeValue{config.RegistrationModeInvite, config.RegistrationModeOpen, config.RegistrationModeClosed} {
		t.Run(string(mode), func(t *testing.T) {
			config.SetRegistrationModeForTesting(t, mode)
			r := gin.New()
			r.GET("/api/v1/platform/registration-mode", GetPlatformRegistrationMode)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/platform/registration-mode", nil))
			require.Equal(t, http.StatusOK, w.Code)
			var body map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, string(mode), body["registration_mode"])
		})
	}
}
