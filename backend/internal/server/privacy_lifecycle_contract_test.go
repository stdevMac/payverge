package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPrivacyDeletionReportsManualReviewAndRetentionExceptions(t *testing.T) {
	db, user := setupAccountDeletionTest(t)

	business := database.Business{
		BusinessId: "privacy-retained", Name: "Retained records", UserID: &user.ID,
		OwnerAddress: "0xprivacy", IsActive: true,
	}
	require.NoError(t, db.Create(&business).Error)

	w := callAccountDeletion(t, user.ID, "owner@example.com")
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		DeletionMode        string   `json:"deletion_mode"`
		HardDeleteAutomated bool     `json:"hard_delete_automated"`
		RetentionExceptions []string `json:"retention_exceptions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "anonymize_after_grace", body.DeletionMode)
	require.False(t, body.HardDeleteAutomated)
	require.Equal(t, []string{
		"tax", "fiscal", "billing", "fraud", "security", "legal",
	}, body.RetentionExceptions)

	var retained database.Business
	require.NoError(t, db.First(&retained, business.ID).Error)
	require.Equal(t, business.BusinessId, retained.BusinessId,
		"account deletion must retain business records pending reviewed exceptions")
}

func TestPrivacyCorrectionUpdatesOnlyAuthenticatedWalletProfile(t *testing.T) {
	db, user := setupAccountDeletionTest(t)
	user.Address = "0xprivacy-owner"
	require.NoError(t, db.Save(user).Error)
	other := database.User{Email: "other@example.com", Address: "0xprivacy-other", Username: "Other"}
	require.NoError(t, db.Create(&other).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/update_user", bytes.NewBufferString(`{"username":"Corrected"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", user.Address)
	UpdateUser(c)
	require.Equal(t, http.StatusOK, w.Code)

	var corrected, untouched database.User
	require.NoError(t, db.First(&corrected, user.ID).Error)
	require.NoError(t, db.First(&untouched, other.ID).Error)
	require.Equal(t, "Corrected", corrected.Username)
	require.Equal(t, "Other", untouched.Username)
}

func TestPrivacyCorrectionRejectsUnverifiedEmailChange(t *testing.T) {
	_, user := setupAccountDeletionTest(t)
	user.Address = "0xprivacy-email"
	require.NoError(t, database.GetDB().Save(user).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/update_user", bytes.NewBufferString(`{"username":"Owner","email":"new@example.com"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", user.Address)
	UpdateUser(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "require verification")
}

func TestPrivacyAccessExportIsLimitedToAuthenticatedOwner(t *testing.T) {
	db, user := setupAccountDeletionTest(t)
	other := database.User{Email: "export-other@example.com"}
	require.NoError(t, db.Create(&other).Error)

	owned := database.Business{BusinessId: "export-owned", Name: "Owned", UserID: &user.ID, IsActive: true}
	unowned := database.Business{BusinessId: "export-unowned", Name: "Unowned", UserID: &other.ID, IsActive: true}
	require.NoError(t, db.Create(&owned).Error)
	require.NoError(t, db.Create(&unowned).Error)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/account/export", nil)
	c.Set("user_id", user.ID)
	ExportAccountData(c)
	require.Equal(t, http.StatusOK, w.Code)

	var payload accountExportPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Len(t, payload.Businesses, 1)
	require.Equal(t, owned.BusinessId, payload.Businesses[0].BusinessID)
	require.NotContains(t, w.Body.String(), unowned.BusinessId)
}
