package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeleteAccount_RecordsAccountDisabledReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&database.User{}, &session.UserSession{}))

	previousDB := database.GetDB()
	previousStore := session.GlobalStore
	database.SetTestDB(db)
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() {
		database.SetTestDB(previousDB)
		session.GlobalStore = previousStore
	})

	user := &database.User{Email: "delete-owner@example.com", Role: "owner"}
	require.NoError(t, db.Create(user).Error)
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &user.ID,
		TokenHash: session.HashToken("delete-owner-token"),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/account/delete", bytes.NewBufferString(`{"confirm_email":"delete-owner@example.com"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", user.ID)

	RequestAccountDeletion(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
	assert.Equal(t, string(session.RevocationReasonAccountDisabled), persisted.RevocationReason)
}

func TestDeleteAccount_RevokesOnlyUserProviderClass(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&database.User{}, &session.UserSession{}))

	previousDB := database.GetDB()
	previousStore := session.GlobalStore
	database.SetTestDB(db)
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() {
		database.SetTestDB(previousDB)
		session.GlobalStore = previousStore
	})

	user := &database.User{Email: "delete-isolation@example.com", Role: "owner"}
	require.NoError(t, db.Create(user).Error)
	sessions := make(map[string]*session.UserSession)
	for _, provider := range []string{"google", "web3", "staff_invite", "customer"} {
		sessions[provider], err = session.GlobalStore.Create(session.CreateInput{
			UserID:    &user.ID,
			TokenHash: session.HashToken("delete-isolation-" + provider),
			Provider:  provider,
			ExpiresAt: time.Now().Add(time.Hour),
		})
		require.NoError(t, err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/account/delete", bytes.NewBufferString(`{"confirm_email":"delete-isolation@example.com"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", user.ID)

	RequestAccountDeletion(c)
	require.Equal(t, http.StatusOK, w.Code)

	for _, provider := range []string{"google", "web3"} {
		var persisted session.UserSession
		require.NoError(t, db.First(&persisted, sessions[provider].ID).Error)
		assert.True(t, persisted.Revoked, "%s belongs to the user identity namespace", provider)
		assert.Equal(t, string(session.RevocationReasonAccountDisabled), persisted.RevocationReason)
	}
	for _, provider := range []string{"staff_invite", "customer"} {
		var persisted session.UserSession
		require.NoError(t, db.First(&persisted, sessions[provider].ID).Error)
		assert.False(t, persisted.Revoked, "%s shares only the numeric id and must survive", provider)
		assert.Nil(t, persisted.RevokedAt)
		assert.Empty(t, persisted.RevocationReason)
	}
}

func TestDeleteAccount_RevokesAddressKeyedOperatorSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&database.User{}, &session.UserSession{}))

	previousDB := database.GetDB()
	previousStore := session.GlobalStore
	database.SetTestDB(db)
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() {
		database.SetTestDB(previousDB)
		session.GlobalStore = previousStore
	})

	const wallet = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	user := &database.User{Email: "delete-wallet@example.com", Role: "owner", Address: wallet}
	require.NoError(t, db.Create(user).Error)
	addressSess, err := session.GlobalStore.Create(session.CreateInput{
		Address:   wallet,
		TokenHash: session.HashToken("delete-address-only"),
		Provider:  "web3",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.Nil(t, addressSess.UserID)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/account/delete", bytes.NewBufferString(`{"confirm_email":"delete-wallet@example.com"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", user.ID)

	RequestAccountDeletion(c)
	require.Equal(t, http.StatusOK, w.Code)

	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, addressSess.ID).Error)
	assert.True(t, persisted.Revoked)
	assert.Equal(t, string(session.RevocationReasonAccountDisabled), persisted.RevocationReason)
}
