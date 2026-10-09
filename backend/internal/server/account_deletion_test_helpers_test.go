package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
)

// setupAccountDeletionTest backs RequestAccountDeletion with sqlite and a nil
// session store (revocation is nil-guarded in the handler and not under test).
func setupAccountDeletionTest(t *testing.T) (*gorm.DB, *database.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	prevDB := database.GetDB()
	prevStore := session.GlobalStore
	t.Cleanup(func() {
		database.SetTestDB(prevDB)
		session.GlobalStore = prevStore
	})
	session.GlobalStore = nil

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&database.User{}, &database.Business{}))
	database.SetTestDB(db)

	user := &database.User{Email: "owner@example.com", Name: "Owner", Role: "user"}
	require.NoError(t, db.Create(user).Error)
	return db, user
}

func callAccountDeletion(t *testing.T, userID uint, confirmEmail string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"confirm_email": confirmEmail})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", userID)
	RequestAccountDeletion(c)
	return w
}
