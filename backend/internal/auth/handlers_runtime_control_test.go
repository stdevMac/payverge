package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"

	"github.com/stretchr/testify/require"
)

func TestRegisterRequiresDurableLaunchInvite(t *testing.T) {
	h, db, _ := newRegisterSessionHandler(t)
	require.NoError(t, db.Exec(`
		CREATE TABLE runtime_controls (key TEXT PRIMARY KEY, enabled BOOLEAN NOT NULL, owner TEXT NOT NULL, reason TEXT NOT NULL, expires_at DATETIME NOT NULL, updated_by TEXT NOT NULL, updated_at DATETIME NOT NULL);
		CREATE TABLE runtime_invite_batches (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, code_digest TEXT NOT NULL UNIQUE, cohort_cap INTEGER NOT NULL, claimed_count INTEGER NOT NULL DEFAULT 0, active BOOLEAN NOT NULL DEFAULT TRUE, owner TEXT NOT NULL, reason TEXT NOT NULL, expires_at DATETIME NOT NULL, created_by TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL);
		CREATE TABLE runtime_invite_claims (id INTEGER PRIMARY KEY AUTOINCREMENT, invite_batch_id INTEGER NOT NULL, normalized_email TEXT NOT NULL UNIQUE, claimed_at DATETIME NOT NULL);
		CREATE TABLE runtime_control_audit_events (id INTEGER PRIMARY KEY AUTOINCREMENT, control_key TEXT NOT NULL, event_type TEXT NOT NULL, old_enabled BOOLEAN, new_enabled BOOLEAN, invite_batch_id INTEGER, owner TEXT NOT NULL, reason TEXT NOT NULL, expires_at DATETIME NOT NULL, actor TEXT NOT NULL, created_at DATETIME NOT NULL);
	`).Error)
	svc := runtimecontrol.New(db)
	h.SetRegistrationAdmission(svc)

	w, c := postJSON(t, map[string]string{"email": "outside@example.test", "password": "password123", "name": "Outside"})
	h.Register(c)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	_, inviteCode, err := svc.CreateInviteBatch(t.Context(), runtimecontrol.CreateInviteBatchInput{Name: "cohort", CohortCap: 1, Owner: "launch", Reason: "test", ExpiresAt: time.Now().Add(time.Hour), Actor: "admin@example.test"})
	require.NoError(t, err)
	w, c = postJSON(t, map[string]string{"email": "inside@example.test", "password": "password123", "name": "Inside", "invite_code": inviteCode})
	h.Register(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

func TestNewGoogleIdentityAlsoRequiresLaunchInvite(t *testing.T) {
	h, db, _ := newRegisterSessionHandler(t)
	require.NoError(t, db.Exec(`
		CREATE TABLE runtime_controls (key TEXT PRIMARY KEY, enabled BOOLEAN NOT NULL, owner TEXT NOT NULL, reason TEXT NOT NULL, expires_at DATETIME NOT NULL, updated_by TEXT NOT NULL, updated_at DATETIME NOT NULL);
		CREATE TABLE runtime_invite_batches (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, code_digest TEXT NOT NULL UNIQUE, cohort_cap INTEGER NOT NULL, claimed_count INTEGER NOT NULL DEFAULT 0, active BOOLEAN NOT NULL DEFAULT TRUE, owner TEXT NOT NULL, reason TEXT NOT NULL, expires_at DATETIME NOT NULL, created_by TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL);
		CREATE TABLE runtime_invite_claims (id INTEGER PRIMARY KEY AUTOINCREMENT, invite_batch_id INTEGER NOT NULL, normalized_email TEXT NOT NULL UNIQUE, claimed_at DATETIME NOT NULL);
		CREATE TABLE runtime_control_audit_events (id INTEGER PRIMARY KEY AUTOINCREMENT, control_key TEXT NOT NULL, event_type TEXT NOT NULL, old_enabled BOOLEAN, new_enabled BOOLEAN, invite_batch_id INTEGER, owner TEXT NOT NULL, reason TEXT NOT NULL, expires_at DATETIME NOT NULL, actor TEXT NOT NULL, created_at DATETIME NOT NULL);
	`).Error)
	svc := runtimecontrol.New(db)
	h.SetRegistrationAdmission(svc)
	info := &GoogleUserInfo{ID: "google-launch-id", Email: "google@example.test", VerifiedEmail: true, Name: "Google Launch"}
	authRecord := &UserAuth{Provider: "google", ProviderUserID: info.ID, EmailVerified: true}

	_, _, err := h.resolveGoogleCallbackUserWithInvite(t.Context(), info, authRecord, true, false, 0, "")
	require.ErrorIs(t, err, runtimecontrol.ErrInviteRequired)

	_, code, err := svc.CreateInviteBatch(t.Context(), runtimecontrol.CreateInviteBatchInput{Name: "google-cohort", CohortCap: 1, Owner: "launch", Reason: "test", ExpiresAt: time.Now().Add(time.Hour), Actor: "admin@example.test"})
	require.NoError(t, err)
	user, isNew, err := h.resolveGoogleCallbackUserWithInvite(t.Context(), info, authRecord, true, false, 0, code)
	require.NoError(t, err)
	require.True(t, isNew)
	require.NotZero(t, user.ID)
}
