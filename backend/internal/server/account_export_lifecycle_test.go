package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestExportAccountData_OwnedLifecycleSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := database.GetDB()
	t.Cleanup(func() { database.SetTestDB(previousDB) })

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.User{},
		&database.Business{},
	))
	database.SetTestDB(db)

	owner := database.User{
		Email: "export-owner@example.test", Name: "Export Owner", Role: "owner",
		AuthMethod: "email", EmailVerified: true, LanguageSelected: "en",
	}
	other := database.User{Email: "other-owner@example.test", Name: "Other", Role: "owner"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&other).Error)

	ownedBusiness := database.Business{
		BusinessId: "export-owned", Name: "Owned Cafe", OwnerAddress: "0xexportowned",
		UserID: &owner.ID, DefaultCurrency: "USD", DisplayCurrency: "USD",
	}
	otherBusiness := database.Business{
		BusinessId: "export-other", Name: "Other Cafe", OwnerAddress: "0xexportother",
		UserID: &other.ID, DefaultCurrency: "EUR", DisplayCurrency: "EUR",
	}
	require.NoError(t, db.Create(&ownedBusiness).Error)
	require.NoError(t, db.Create(&otherBusiness).Error)

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/inside/account/export", nil)
	ctx.Set("user_id", owner.ID)
	ExportAccountData(ctx)

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Header().Get("Content-Disposition"), "payverge-account-export-")
	var payload accountExportPayload
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Equal(t, owner.ID, payload.Profile.ID)
	require.Equal(t, owner.Email, payload.Profile.Email)
	require.Len(t, payload.Businesses, 1)
	require.Equal(t, ownedBusiness.BusinessId, payload.Businesses[0].BusinessID)
	require.WithinDuration(t, payload.GeneratedAt.Add(24*time.Hour), payload.ExportTTLAt, time.Second)
	require.NotContains(t, response.Body.String(), otherBusiness.BusinessId)
}
