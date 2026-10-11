package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestCreatePushSubscriptionRejectsUnauthorizedBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.PushSubscription{}))

	ownedUserID := uint(1)
	otherUserID := uint(2)
	otherBusiness := &database.Business{
		BusinessId:     "push-other-business",
		Name:           "Other Business",
		OwnerAddress:   "0xOtherOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		UserID:         &otherUserID,
		IsActive:       true,
	}
	require.NoError(t, db.Create(otherBusiness).Error)

	body := bytes.NewBufferString(fmt.Sprintf(`{
		"business_id":%d,
		"endpoint":"https://fcm.googleapis.com/fcm/send/sub",
		"p256dh_key":"key",
		"auth_key":"auth"
	}`, otherBusiness.ID))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", ownedUserID)

	CreatePushSubscription(c)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var count int64
	require.NoError(t, db.Model(&database.PushSubscription{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestCreatePushSubscriptionAllowsOwnedBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.PushSubscription{}))

	ownerUserID := uint(7)
	business := &database.Business{
		BusinessId:     "push-owned-business",
		Name:           "Owned Business",
		OwnerAddress:   "0xOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		UserID:         &ownerUserID,
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)

	body := bytes.NewBufferString(fmt.Sprintf(`{
		"business_id":%d,
		"endpoint":"https://fcm.googleapis.com/fcm/send/owned",
		"p256dh_key":"key",
		"auth_key":"auth"
	}`, business.ID))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", ownerUserID)

	CreatePushSubscription(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var subscription database.PushSubscription
	require.NoError(t, db.First(&subscription).Error)
	assert.Equal(t, ownerUserID, subscription.UserID)
	assert.Equal(t, business.ID, subscription.BusinessID)
}

func TestCreatePushSubscriptionRejectsClosedOwnedBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.PushSubscription{}))

	ownerUserID := uint(8)
	closedAt := time.Now().Add(-time.Hour)
	business := &database.Business{
		BusinessId:     "push-closed-business",
		Name:           "Closed Business",
		OwnerAddress:   "0xSuspendedOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		UserID:         &ownerUserID,
		IsActive:       true,
		ClosedAt:       &closedAt,
	}
	require.NoError(t, db.Create(business).Error)

	body := bytes.NewBufferString(fmt.Sprintf(`{
		"business_id":%d,
		"endpoint":"https://fcm.googleapis.com/fcm/send/closed",
		"p256dh_key":"key",
		"auth_key":"auth"
	}`, business.ID))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", ownerUserID)

	CreatePushSubscription(c)

	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"code":"business_closed"`)
	var count int64
	require.NoError(t, db.Model(&database.PushSubscription{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestListPushSubscriptionsAcceptsFloatUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.PushSubscription{}))

	require.NoError(t, db.Create(&database.PushSubscription{
		UserID:        9,
		BusinessID:    1,
		PrincipalType: database.PushPrincipalOwnerUser,
		PrincipalID:   9,
		Endpoint:      "https://fcm.googleapis.com/fcm/send/list",
		P256dhKey:     "key",
		AuthKey:       "auth",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("user_id", float64(9))

	ListPushSubscriptions(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "https://fcm.googleapis.com/fcm/send/list")
}

func TestDeletePushSubscriptionAcceptsFloatUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.PushSubscription{}))

	sub := &database.PushSubscription{
		UserID:        12,
		BusinessID:    1,
		PrincipalType: database.PushPrincipalOwnerUser,
		PrincipalID:   12,
		Endpoint:      "https://fcm.googleapis.com/fcm/send/delete",
		P256dhKey:     "key",
		AuthKey:       "auth",
	}
	require.NoError(t, db.Create(sub).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Params = gin.Params{{Key: "subscriptionId", Value: fmt.Sprintf("%d", sub.ID)}}
	c.Set("user_id", float64(12))

	DeletePushSubscription(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var count int64
	require.NoError(t, db.Model(&database.PushSubscription{}).Where("id = ?", sub.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

// Resubscribing with the same (user, endpoint) but a new business or rotated
// browser keys must refresh the stored row. FirstOrCreate alone returned the
// stale record: old business_id kept fanning out and rotated p256dh/auth keys
// made every send fail undetectably.
func TestCreatePushSubscriptionRefreshesBusinessAndKeysOnResubscribe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.PushSubscription{}))

	ownerUserID := uint(11)
	business := &database.Business{
		BusinessId:     "push-resub-business",
		Name:           "Resub Business",
		OwnerAddress:   "0xResubOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		UserID:         &ownerUserID,
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)

	// Stale row from a previous subscribe: old business, old keys.
	require.NoError(t, db.Create(&database.PushSubscription{
		UserID:        ownerUserID,
		BusinessID:    business.ID + 999,
		PrincipalType: database.PushPrincipalOwnerUser,
		PrincipalID:   ownerUserID,
		Endpoint:      "https://fcm.googleapis.com/fcm/send/resub",
		P256dhKey:     "old-p256dh",
		AuthKey:       "old-auth",
	}).Error)

	body := bytes.NewBufferString(fmt.Sprintf(`{
		"business_id":%d,
		"endpoint":"https://fcm.googleapis.com/fcm/send/resub",
		"p256dh_key":"new-p256dh",
		"auth_key":"new-auth"
	}`, business.ID))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", ownerUserID)

	CreatePushSubscription(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var count int64
	require.NoError(t, db.Model(&database.PushSubscription{}).Count(&count).Error)
	assert.Equal(t, int64(1), count, "resubscribe must not duplicate the row")

	var sub database.PushSubscription
	require.NoError(t, db.First(&sub).Error)
	assert.Equal(t, business.ID, sub.BusinessID, "business_id refreshed on resubscribe")
	assert.Equal(t, "new-p256dh", sub.P256dhKey, "p256dh key refreshed on resubscribe")
	assert.Equal(t, "new-auth", sub.AuthKey, "auth key refreshed on resubscribe")
}

// M-push: arbitrary endpoints (internal hosts, metadata IPs, plain http) are
// rejected before anything is stored.
func TestCreatePushSubscriptionRejectsNonPushServiceEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.PushSubscription{}))

	ownerUserID := uint(7)
	business := &database.Business{
		BusinessId:     "push-ssrf-business",
		Name:           "Owned Business",
		OwnerAddress:   "0xOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		UserID:         &ownerUserID,
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)

	for _, endpoint := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"https://internal.payverge.local/hook",
		"http://fcm.googleapis.com/fcm/send/x",
	} {
		body := bytes.NewBufferString(fmt.Sprintf(`{"business_id":%d,"endpoint":%q,"p256dh_key":"key","auth_key":"auth"}`, business.ID, endpoint))
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", body)
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user_id", ownerUserID)

		CreatePushSubscription(c)
		assert.Equalf(t, http.StatusBadRequest, w.Code, "endpoint %s must be rejected", endpoint)
	}

	var count int64
	require.NoError(t, db.Model(&database.PushSubscription{}).Count(&count).Error)
	assert.Zero(t, count)
}
