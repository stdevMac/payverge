package auth

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const runtimeControlTestSchema = `
	CREATE TABLE runtime_controls (key TEXT PRIMARY KEY, enabled BOOLEAN NOT NULL, owner TEXT NOT NULL, reason TEXT NOT NULL, expires_at DATETIME NOT NULL, updated_by TEXT NOT NULL, updated_at DATETIME NOT NULL);
	CREATE TABLE runtime_invite_batches (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, code_digest TEXT NOT NULL UNIQUE, cohort_cap INTEGER NOT NULL, claimed_count INTEGER NOT NULL DEFAULT 0, active BOOLEAN NOT NULL DEFAULT TRUE, owner TEXT NOT NULL, reason TEXT NOT NULL, expires_at DATETIME NOT NULL, created_by TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL);
	CREATE TABLE runtime_invite_claims (id INTEGER PRIMARY KEY AUTOINCREMENT, invite_batch_id INTEGER NOT NULL, normalized_email TEXT NOT NULL UNIQUE, claimed_at DATETIME NOT NULL);
	CREATE TABLE runtime_control_audit_events (id INTEGER PRIMARY KEY AUTOINCREMENT, control_key TEXT NOT NULL, event_type TEXT NOT NULL, old_enabled BOOLEAN, new_enabled BOOLEAN, invite_batch_id INTEGER, owner TEXT NOT NULL, reason TEXT NOT NULL, expires_at DATETIME NOT NULL, actor TEXT NOT NULL, created_at DATETIME NOT NULL);
`

func newModeRegisterHandler(t *testing.T, mode config.RegistrationModeValue) (*AuthHandler, *gorm.DB, *runtimecontrol.Service) {
	t.Helper()
	h, db, _ := newRegisterSessionHandler(t)
	require.NoError(t, db.Exec(runtimeControlTestSchema).Error)
	svc := runtimecontrol.New(db).WithRegistrationMode(func() config.RegistrationModeValue { return mode })
	h.SetRegistrationAdmission(svc)
	return h, db, svc
}

func mintTestInvite(t *testing.T, svc *runtimecontrol.Service) string {
	t.Helper()
	_, code, err := svc.CreateInviteBatch(t.Context(), runtimecontrol.CreateInviteBatchInput{
		Name: "cohort", CohortCap: 5, Owner: "self-host", Reason: "test", ExpiresAt: time.Now().Add(time.Hour), Actor: "admin@example.test",
	})
	require.NoError(t, err)
	return code
}

func userCount(t *testing.T, db *gorm.DB, email string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&database.User{}).Where("LOWER(email) = LOWER(?)", email).Count(&n).Error)
	return n
}

func TestRegisterClosedModeRefusesEvenWithInvite(t *testing.T) {
	h, db, svc := newModeRegisterHandler(t, config.RegistrationModeClosed)
	code := mintTestInvite(t, svc)

	w, c := postJSON(t, map[string]string{"email": "closed@example.test", "password": "password123", "name": "Closed", "invite_code": code})
	h.Register(c)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	var body struct {
		Code   string                 `json:"code"`
		Params map[string]interface{} `json:"params"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "registration_closed", body.Params["reason"])
	require.Zero(t, userCount(t, db, "closed@example.test"))
}

func TestRegisterClosedModeIsNotADuplicateEmailOracle(t *testing.T) {
	h, db, _ := newModeRegisterHandler(t, config.RegistrationModeClosed)
	require.NoError(t, db.Create(&database.User{Email: "taken@example.test", Role: "user", AuthMethod: "email"}).Error)

	w, c := postJSON(t, map[string]string{"email": "taken@example.test", "password": "password123", "name": "Taken"})
	h.Register(c)
	require.Equal(t, http.StatusForbidden, w.Code, "closed mode must answer 403 before the duplicate-email 409: %s", w.Body.String())
}

func TestRegisterOpenModeNeedsNoInvite(t *testing.T) {
	h, db, _ := newModeRegisterHandler(t, config.RegistrationModeOpen)
	w, c := postJSON(t, map[string]string{"email": "open@example.test", "password": "password123", "name": "Open"})
	h.Register(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Equal(t, int64(1), userCount(t, db, "open@example.test"))
	var claims int64
	require.NoError(t, db.Model(&runtimecontrol.InviteClaim{}).Count(&claims).Error)
	require.Zero(t, claims)
}

func TestRegisterInviteModeRequiresInvite(t *testing.T) {
	h, db, svc := newModeRegisterHandler(t, config.RegistrationModeInvite)
	w, c := postJSON(t, map[string]string{"email": "nocode@example.test", "password": "password123", "name": "No Code"})
	h.Register(c)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Zero(t, userCount(t, db, "nocode@example.test"))

	code := mintTestInvite(t, svc)
	w, c = postJSON(t, map[string]string{"email": "withcode@example.test", "password": "password123", "name": "With Code", "invite_code": code})
	h.Register(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

func TestGoogleNewIdentityHonoursRegistrationMode(t *testing.T) {
	cases := []struct {
		mode    config.RegistrationModeValue
		wantErr error
	}{
		{config.RegistrationModeClosed, runtimecontrol.ErrRegistrationClosed},
		{config.RegistrationModeOpen, nil},
		{config.RegistrationModeInvite, runtimecontrol.ErrInviteRequired},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			h, db, _ := newModeRegisterHandler(t, tc.mode)
			info := &GoogleUserInfo{ID: "g-" + string(tc.mode), Email: string(tc.mode) + "@google.example.test", VerifiedEmail: true, Name: "G"}
			authRecord := &UserAuth{Provider: "google", ProviderUserID: info.ID, EmailVerified: true}
			user, isNew, err := h.resolveGoogleCallbackUserWithInvite(t.Context(), info, authRecord, true, false, 0, "")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Zero(t, userCount(t, db, info.Email))
				return
			}
			require.NoError(t, err)
			require.True(t, isNew)
			require.NotZero(t, user.ID)
		})
	}
}
