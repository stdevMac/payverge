package auth

import (
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Registration must not touch the normal session store. Even if normal-session
// persistence is unavailable, it can create only an unverified reservation and
// return no credential; verification/login owns session issuance.
func TestRegisterDoesNotDependOnOperatorSessionPersistence(t *testing.T) {
	h, db, _ := newRegisterSessionHandler(t)
	require.NoError(t, db.AutoMigrate(&session.UserSession{}))

	prevStore := session.GlobalStore
	t.Cleanup(func() { session.GlobalStore = prevStore })
	session.GlobalStore = session.NewStore(db)

	// Block UPDATEs on user_sessions: Create (INSERT) succeeds, then the
	// token-hash persistence (UPDATE) fails — exactly the partial failure the
	// old code shipped to the user as a 201.
	require.NoError(t, db.Exec(`CREATE TRIGGER block_session_updates
		BEFORE UPDATE ON user_sessions
		BEGIN SELECT RAISE(ABORT, 'update blocked'); END;`).Error)

	w, c := postJSON(t, map[string]string{
		"email":    "failfast@example.com",
		"password": "password123",
		"name":     "Fail Fast",
	})
	h.Register(c)

	require.Equal(t, http.StatusCreated, w.Code,
		"pending registration must not attempt normal-session persistence")

	// The account exists (user+auth tx committed before the session block) —
	// the registrant recovers via normal login + verification, with no
	// duplicate-email dead end.
	var user database.User
	require.NoError(t, db.Where("email = ?", "failfast@example.com").First(&user).Error)

	// No session cookie was issued and the blocking trigger was never reached.
	for _, ck := range w.Result().Cookies() {
		assert.NotEqual(t, "session_token", ck.Name, "must not set a session cookie on failure")
	}
}
