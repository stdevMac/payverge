package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// setupDeleteBusinessTest backs the handler with an in-memory sqlite database
// (mirrors setupRefreshHandlerTest) and restores the previous handle after.
func setupDeleteBusinessTest(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)

	prevDB := database.GetDB()
	t.Cleanup(func() { database.SetTestDB(prevDB) })

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&database.Business{}, &database.BusinessCreationRequest{}))
	database.SetTestDB(db)
	return db
}

// callDeleteBusiness invokes the handler as the business owner (web3 address
// path of CheckBusinessOwnership).
func callDeleteBusiness(t *testing.T, businessID uint, ownerAddress string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set("address", ownerAddress)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(businessID)}}
	DeleteBusiness(c)
	return w
}

func TestDeleteBusiness_CleansWorkspaceCreationReplayLedger(t *testing.T) {
	db := setupDeleteBusinessTest(t)

	biz := database.Business{
		BusinessId: "abandoned-workspace", Name: "Abandoned", OwnerAddress: "0xowner",
		IsActive: true,
	}
	require.NoError(t, db.Create(&biz).Error)
	request := database.BusinessCreationRequest{
		OwnerScopeHash:     "a" + fmt.Sprintf("%063d", 0),
		IdempotencyKeyHash: "b" + fmt.Sprintf("%063d", 0),
		PayloadHash:        "c" + fmt.Sprintf("%063d", 0),
		BusinessID:         &biz.ID,
	}
	require.NoError(t, db.Create(&request).Error)

	w := callDeleteBusiness(t, biz.ID, "0xowner")
	require.Equal(t, http.StatusOK, w.Code)

	var replayRows int64
	require.NoError(t, db.Model(&database.BusinessCreationRequest{}).
		Where("business_id = ?", biz.ID).Count(&replayRows).Error)
	require.Zero(t, replayRows,
		"an explicitly abandoned workspace must not replay forever from the creation ledger")
}

// TestDeleteBusiness_DeactivatesBusiness: deleting a business soft-deletes it
// by flipping is_active off.
func TestDeleteBusiness_DeactivatesBusiness(t *testing.T) {
	db := setupDeleteBusinessTest(t)

	biz := database.Business{
		BusinessId: "del-plain", Name: "Del Plain", OwnerAddress: "0xowner", IsActive: true,
	}
	require.NoError(t, db.Create(&biz).Error)

	w := callDeleteBusiness(t, biz.ID, "0xowner")
	require.Equal(t, http.StatusOK, w.Code)

	var after database.Business
	require.NoError(t, db.First(&after, biz.ID).Error)
	require.False(t, after.IsActive)
}
