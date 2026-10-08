//go:build integration
// +build integration

package server_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/session"
)

func useIntegrationSessionStore(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&session.UserSession{}), "automigrate user_sessions")
	previousStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() {
		session.GlobalStore = previousStore
	})
}

func generateIntegrationStaffToken(t *testing.T, staff *database.Staff) string {
	t.Helper()
	require.NotNil(t, session.GlobalStore, "integration session store must be configured")
	staffID := staff.ID
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &staffID,
		TokenHash: "placeholder",
		Provider:  "staff",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err, "create staff session")

	token, err := server.GenerateStaffToken(staff, sess.ID)
	require.NoError(t, err, "mint session-backed staff token")
	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)), "store staff token hash")
	return token
}
