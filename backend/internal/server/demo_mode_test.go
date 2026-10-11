package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceReportsDemoModeAndForcedSettings(t *testing.T) {
	clearInstanceTestEnv(t)
	t.Setenv("DEMO_DATA", "true")
	t.Setenv("REGISTRATION_MODE", "open")
	t.Setenv("EMAIL_PROVIDER", "resend")
	config.SetDemoModeForTesting(t, true)
	resetInstanceInfoForTest()

	_, body := getInstance(t)
	demo := body["demo"].(map[string]any)
	assert.Equal(t, true, demo["enabled"])
	assert.Equal(t, true, demo["mode"])
	assert.Equal(t, "03:00", demo["reset_utc"])
	assert.Equal(t, "closed", body["registration_mode"], "DEMO_MODE closes signup whatever REGISTRATION_MODE says")
}

func TestInstanceDemoModeOffByDefault(t *testing.T) {
	clearInstanceTestEnv(t)
	config.SetDemoModeForTesting(t, false)
	resetInstanceInfoForTest()

	_, body := getInstance(t)
	demo := body["demo"].(map[string]any)
	assert.Equal(t, false, demo["mode"])
	assert.Equal(t, "", demo["reset_utc"])
}

func TestGetDemoTablesIs404WhenDemoModeOff(t *testing.T) {
	config.SetDemoModeForTesting(t, false)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/demo/tables", GetDemoTables)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/demo/tables", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), "venues")
}

func TestGetDemoTablesListsShowroomTableCodes(t *testing.T) {
	db := setupAdminLiveRoleDB(t)
	require.NoError(t, db.AutoMigrate(&database.Business{}, &database.Table{}))
	config.SetDemoModeForTesting(t, true)

	ownerID, err := demomode.EnsureShowroomOwner(context.Background(), db)
	require.NoError(t, err)
	other := database.User{Email: "someone@example.com", Role: "user"}
	require.NoError(t, db.Create(&other).Error)

	core := database.Business{BusinessId: "demo-admin-1-core", Name: "Bodegón Doña Rosa", CustomURL: "bodegon", UserID: &ownerID, IsDemo: true, IsActive: true}
	require.NoError(t, db.Create(&core).Error)
	private := database.Business{BusinessId: "real-1", Name: "Not the demo", CustomURL: "real", UserID: &other.ID, IsDemo: false, IsActive: true}
	require.NoError(t, db.Create(&private).Error)
	for i, code := range []string{"T1", "T2", "T3", "T4", "T5", "T6", "T7"} {
		require.NoError(t, db.Create(&database.Table{BusinessID: core.ID, Name: "Mesa " + code, TableCode: "demo-" + code, IsActive: true}).Error, i)
	}
	require.NoError(t, db.Create(&database.Table{BusinessID: private.ID, Name: "Secret", TableCode: "secret-code", IsActive: true}).Error)

	r := gin.New()
	r.GET("/api/v1/demo/tables", GetDemoTables)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/demo/tables", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "secret-code", "only showroom venues are advertised")

	var body struct {
		Venues []struct {
			Name      string                        `json:"name"`
			CustomURL string                        `json:"custom_url"`
			Tables    []struct{ Name, Code string } `json:"tables"`
		} `json:"venues"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Venues, 1)
	assert.Equal(t, "bodegon", body.Venues[0].CustomURL)
	assert.Len(t, body.Venues[0].Tables, demoTableLimit)
	assert.Equal(t, "demo-T1", body.Venues[0].Tables[0].Code)
}

// The showroom owner is a plain user: /admin stays unreachable for anyone who
// clicked "Enter demo as Owner".
func TestDemoOwnerCannotReachAdmin(t *testing.T) {
	db := setupAdminLiveRoleDB(t)
	ownerID, err := demomode.EnsureShowroomOwner(context.Background(), db)
	require.NoError(t, err)
	var owner database.User
	require.NoError(t, db.First(&owner, ownerID).Error)

	r := setupAdminLiveRoleRouter(t)
	for _, claimedRole := range []string{owner.Role, "admin"} {
		token, err := GenerateUserToken(owner.ID, owner.Email, owner.Address, claimedRole)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/probe", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code, "claimed role %q: %s", claimedRole, w.Body.String())
	}
}

func TestDemoOwnerSessionLivesOnlyWhileDemoModeIsOn(t *testing.T) {
	_, user := setupRefreshHandlerTest(t)

	config.SetDemoModeForTesting(t, true)
	_, token := seedOperatorAccessSession(t, user, demomode.SessionProvider, "demo-access-on", "")
	assert.Equal(t, http.StatusNoContent, probeOperatorSession(t, token), "demo session works in DEMO_MODE")

	ok, err := verifyPersistedOperatorSession(&session.UserSession{Provider: demomode.SessionProvider})
	require.NoError(t, err)
	assert.True(t, ok)

	config.SetDemoModeForTesting(t, false)
	ok, err = verifyPersistedOperatorSession(&session.UserSession{Provider: demomode.SessionProvider})
	require.NoError(t, err)
	assert.False(t, ok, "turning DEMO_MODE off ends demo owner sessions")
	assert.NotEqual(t, http.StatusNoContent, probeOperatorSession(t, token))
}

func TestDemoOwnerSessionRefreshes(t *testing.T) {
	_, user := setupRefreshHandlerTest(t)
	config.SetDemoModeForTesting(t, true)
	seedOperatorAccessSession(t, user, demomode.SessionProvider, "demo-refresh-access", "demo-refresh")
	w, newRefresh := callRefresh(t, "demo-refresh")
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotEmpty(t, newRefresh)
}
