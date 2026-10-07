package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoLoginIs404WhenDemoModeOff(t *testing.T) {
	config.SetDemoModeForTesting(t, false)
	h, db := newAuthEnvelopeHandler(t)
	_, err := demomode.EnsureShowroomOwner(context.Background(), db)
	require.NoError(t, err)

	for _, role := range []string{"owner", "kitchen", "waiter", "admin", ""} {
		w, c := postJSON(t, map[string]string{"role": role})
		h.DemoLogin(c)
		assert.Equal(t, http.StatusNotFound, w.Code, "role %q", role)
		assert.NotContains(t, w.Body.String(), "token")
		assert.Empty(t, w.Result().Cookies(), "no cookie may be set outside DEMO_MODE")
	}
}

func TestDemoLoginOwnerIssuesNonAdminDemoSession(t *testing.T) {
	config.SetDemoModeForTesting(t, true)
	h, db := newAuthEnvelopeHandler(t)
	require.NoError(t, db.AutoMigrate(&session.UserSession{}))
	prevStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() { session.GlobalStore = prevStore })

	ownerID, err := demomode.EnsureShowroomOwner(context.Background(), db)
	require.NoError(t, err)

	w, c := postJSON(t, map[string]string{"role": "owner"})
	h.DemoLogin(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Token    string `json:"token"`
		Role     string `json:"role"`
		Redirect string `json:"redirect"`
		User     struct {
			ID   uint   `json:"id"`
			Role string `json:"role"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotEmpty(t, body.Token)
	assert.Equal(t, "owner", body.Role)
	assert.Equal(t, "/dashboard", body.Redirect)
	assert.Equal(t, ownerID, body.User.ID)
	assert.Equal(t, "user", body.User.Role, "the demo owner is never a platform admin")

	var sess session.UserSession
	require.NoError(t, db.Where("user_id = ?", ownerID).First(&sess).Error)
	assert.Equal(t, demomode.SessionProvider, sess.Provider)

	names := map[string]bool{}
	for _, ck := range w.Result().Cookies() {
		names[ck.Name] = true
	}
	assert.True(t, names["refresh_token"], "owner demo login sets the normal refresh cookie")
}

func TestDemoLoginRejectsUnknownRoles(t *testing.T) {
	config.SetDemoModeForTesting(t, true)
	h, db := newAuthEnvelopeHandler(t)
	_, err := demomode.EnsureShowroomOwner(context.Background(), db)
	require.NoError(t, err)

	for _, role := range []string{"admin", "superuser", ""} {
		w, c := postJSON(t, map[string]string{"role": role})
		h.DemoLogin(c)
		assert.Equal(t, http.StatusBadRequest, w.Code, "role %q", role)
		assert.NotContains(t, w.Body.String(), `"token"`)
	}
}

func TestDemoLoginBeforeSeedIs503(t *testing.T) {
	config.SetDemoModeForTesting(t, true)
	h, _ := newAuthEnvelopeHandler(t)
	w, c := postJSON(t, map[string]string{"role": "owner"})
	h.DemoLogin(c)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestDemoLoginNeverHandsOutAnAdmin(t *testing.T) {
	config.SetDemoModeForTesting(t, true)
	h, db := newAuthEnvelopeHandler(t)
	require.NoError(t, db.Create(&database.User{
		Email: demomode.ShowroomOwnerEmail, Role: "admin", AuthMethod: "email", EmailVerified: true,
	}).Error)

	_, err := demomode.EnsureShowroomOwner(context.Background(), db)
	require.ErrorIs(t, err, demomode.ErrShowroomOwnerIsAdmin, "boot refuses an admin showroom owner")

	w, c := postJSON(t, map[string]string{"role": "owner"})
	h.DemoLogin(c)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.NotContains(t, w.Body.String(), `"token"`)
}

func TestEnsureShowroomOwnerIsIdempotentAndRepairs(t *testing.T) {
	_, db := newAuthEnvelopeHandler(t)
	ctx := context.Background()
	id1, err := demomode.EnsureShowroomOwner(ctx, db)
	require.NoError(t, err)
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", id1).Updates(map[string]interface{}{"auth_method": "email", "email_verified": false}).Error)

	id2, err := demomode.EnsureShowroomOwner(ctx, db)
	require.NoError(t, err)
	assert.Equal(t, id1, id2)

	var u database.User
	require.NoError(t, db.First(&u, id1).Error)
	assert.Equal(t, "user", u.Role)
	assert.Equal(t, demomode.SessionProvider, u.AuthMethod)
	assert.True(t, u.EmailVerified)
}
