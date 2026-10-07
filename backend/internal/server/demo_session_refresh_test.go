package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/session"
)

// seedProviderRefreshSession seeds a refreshable session for userID under
// provider.
func seedProviderRefreshSession(t *testing.T, userID uint, provider, refreshRaw string) *session.UserSession {
	t.Helper()
	sess := seedRefreshSession(t, userID, refreshRaw)
	require.NoError(t, database.GetDB().Model(&session.UserSession{}).
		Where("id = ?", sess.ID).Update("provider", provider).Error)
	sess.Provider = provider
	return sess
}

func ageRefreshRotation(t *testing.T, sessionID uint, refreshRaw string) {
	t.Helper()
	stale := time.Now().Add(-(session.RefreshReuseGracePeriod + time.Minute))
	require.NoError(t, database.GetDB().Table("user_session_refresh_history").
		Where("session_id = ? AND token_hash = ?", sessionID, session.HashToken(refreshRaw)).
		Update("rotated_at", stale).Error)
}

func requireRevoked(t *testing.T, id uint, want bool, msg string) {
	t.Helper()
	var row session.UserSession
	require.NoError(t, database.GetDB().First(&row, id).Error)
	require.Equal(t, want, row.Revoked, msg)
}

// TestRefreshToken_DemoSharedIdentityReuseRevokesOnlyThatSession locks review
// finding F1: every public-demo visitor shares one owner (or staff) identity,
// so a stale refresh-token replay by one visitor must revoke only that
// visitor's session. A family revoke would let anyone sign every concurrent
// demo visitor out with one scripted POST /auth/refresh.
func TestRefreshToken_DemoSharedIdentityReuseRevokesOnlyThatSession(t *testing.T) {
	for _, provider := range []string{demomode.SessionProvider, demomode.StaffSessionProvider} {
		t.Run(provider, func(t *testing.T) {
			config.SetDemoModeForTesting(t, true)
			db, user := setupRefreshHandlerTest(t)
			if provider == demomode.StaffSessionProvider {
				require.NoError(t, db.AutoMigrate(&database.Staff{}))
				staff := &database.Staff{BusinessID: 1, Name: "Demo waiter", Email: "waiter@demo.payverge.invalid", Role: database.StaffRoleServer, IsActive: true}
				require.NoError(t, db.Create(staff).Error)
				user = &database.User{ID: staff.ID}
			}
			attacker := seedProviderRefreshSession(t, user.ID, provider, "visitor-a-1")
			victim := seedProviderRefreshSession(t, user.ID, provider, "visitor-b-1")

			w, _ := callRefresh(t, "visitor-a-1")
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			ageRefreshRotation(t, attacker.ID, "visitor-a-1")

			replay, minted := callRefresh(t, "visitor-a-1")
			require.Equal(t, http.StatusUnauthorized, replay.Code)
			require.Contains(t, replay.Body.String(), ErrCodeSessionUnknown)
			require.Empty(t, minted)

			requireRevoked(t, attacker.ID, true, "the replayed session itself is revoked")
			requireRevoked(t, victim.ID, false, "another visitor sharing the demo identity must stay signed in")

			w, _ = callRefresh(t, "visitor-b-1")
			require.Equal(t, http.StatusOK, w.Code, "the other visitor can still refresh: %s", w.Body.String())
		})
	}
}

// TestRefreshToken_NonDemoReuseStillRevokesFamily keeps the breach response for
// ordinary accounts: the demo carve-out must not weaken it.
func TestRefreshToken_NonDemoReuseStillRevokesFamily(t *testing.T) {
	config.SetDemoModeForTesting(t, true)
	_, user := setupRefreshHandlerTest(t)
	stolen := seedRefreshSession(t, user.ID, "real-a-1")
	sibling := seedRefreshSession(t, user.ID, "real-b-1")

	w, _ := callRefresh(t, "real-a-1")
	require.Equal(t, http.StatusOK, w.Code)
	ageRefreshRotation(t, stolen.ID, "real-a-1")

	replay, _ := callRefresh(t, "real-a-1")
	require.Equal(t, http.StatusUnauthorized, replay.Code)
	requireRevoked(t, stolen.ID, true, "replayed session revoked")
	requireRevoked(t, sibling.ID, true, "a real account's whole family is still revoked on reuse")
}

// TestRefreshToken_DemoStaffSessionDiesWithoutDemoMode locks review finding F6:
// a one-click demo staff session must not outlive DEMO_MODE.
func TestRefreshToken_DemoStaffSessionDiesWithoutDemoMode(t *testing.T) {
	config.SetDemoModeForTesting(t, false)
	db, _ := setupRefreshHandlerTest(t)
	require.NoError(t, db.AutoMigrate(&database.Staff{}))
	staff := &database.Staff{BusinessID: 1, Name: "Demo kitchen", Email: "kitchen@demo.payverge.invalid", Role: database.StaffRoleKitchen, IsActive: true}
	require.NoError(t, db.Create(staff).Error)
	demo := seedProviderRefreshSession(t, staff.ID, demomode.StaffSessionProvider, "demo-staff-1")
	ordinary := seedProviderRefreshSession(t, staff.ID, "staff_code", "real-staff-1")

	w, minted := callRefresh(t, "demo-staff-1")
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	require.Empty(t, minted)
	requireRevoked(t, demo.ID, true, "the demo staff session is revoked once DEMO_MODE is off")

	w, _ = callRefresh(t, "real-staff-1")
	require.Equal(t, http.StatusOK, w.Code, "an ordinary staff_code session is unaffected: %s", w.Body.String())
	requireRevoked(t, ordinary.ID, false, "ordinary staff session stays live")
}

func TestVerifyPersistedOperatorSession_DemoStaffFollowsDemoMode(t *testing.T) {
	sess := &session.UserSession{Provider: demomode.StaffSessionProvider}
	config.SetDemoModeForTesting(t, true)
	ok, err := verifyPersistedOperatorSession(sess)
	require.NoError(t, err)
	require.True(t, ok)
	config.SetDemoModeForTesting(t, false)
	ok, err = verifyPersistedOperatorSession(sess)
	require.NoError(t, err)
	require.False(t, ok, "every request on a demo staff session is refused once DEMO_MODE is off")
}

// TestIssueDemoStaffSession_UsesDemoStaffProvider pins the tag that F1 and F6
// rely on: one-click staff sessions are demo_staff, not staff_code.
func TestIssueDemoStaffSession_UsesDemoStaffProvider(t *testing.T) {
	prev := prepareStaffSession
	t.Cleanup(func() { prepareStaffSession = prev })
	var gotProvider string
	prepareStaffSession = func(_ *gin.Context, _ *database.Staff, provider string) (*preparedStaffLogin, error) {
		gotProvider = provider
		return nil, errors.New("stop after capturing the provider")
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := IssueDemoStaffSession(c, &database.Staff{ID: 7, IsActive: true})
	require.Error(t, err)
	require.Equal(t, demomode.StaffSessionProvider, gotProvider)
}
