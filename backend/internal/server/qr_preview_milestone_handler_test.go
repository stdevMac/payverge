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

func TestMarkQRPreviewed_WorksBeforeSubscriptionAndIsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupOnboardingHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Table{}))

	business := &database.Business{
		BusinessId:   "pre-subscription-qr-preview",
		Name:         "First Value Cafe",
		OwnerAddress: "0xOwnerBeforeSubscription",
		IsActive:     true,
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  "pre-subscription-table",
		Name:       "Table 1",
		Capacity:   2,
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	mark := func() *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]uint{"table_id": table.ID})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
		c.Request = httptest.NewRequest(http.MethodPost,
			"/inside/businesses/"+business.BusinessId+"/onboarding/qr-preview", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("address", business.OwnerAddress)
		MarkQRPreviewed(c)
		return w
	}

	first := mark()
	require.Equal(t, http.StatusOK, first.Code)
	var afterFirst database.Business
	require.NoError(t, database.GetDB().First(&afterFirst, business.ID).Error)
	require.NotNil(t, afterFirst.QRPreviewedAt)
	firstTimestamp := *afterFirst.QRPreviewedAt

	second := mark()
	require.Equal(t, http.StatusOK, second.Code)
	var afterSecond database.Business
	require.NoError(t, database.GetDB().First(&afterSecond, business.ID).Error)
	require.NotNil(t, afterSecond.QRPreviewedAt)
	require.Equal(t, firstTimestamp, *afterSecond.QRPreviewedAt,
		"reopening the same QR preview must retain the original milestone timestamp")
}

func TestMarkQRPreviewed_RejectsTableFromAnotherBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupOnboardingHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Table{}))

	owner := &database.Business{BusinessId: "qr-owner", Name: "Owner", OwnerAddress: "0xOwner", IsActive: true}
	other := &database.Business{BusinessId: "qr-other", Name: "Other", OwnerAddress: "0xOther", IsActive: true}
	require.NoError(t, database.GetDB().Create(owner).Error)
	require.NoError(t, database.GetDB().Create(other).Error)
	table := &database.Table{BusinessID: other.ID, TableCode: "other-table", Name: "Other", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)

	body, err := json.Marshal(map[string]uint{"table_id": table.ID})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: owner.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", owner.OwnerAddress)
	MarkQRPreviewed(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	var refreshed database.Business
	require.NoError(t, database.GetDB().First(&refreshed, owner.ID).Error)
	require.Nil(t, refreshed.QRPreviewedAt)
}
