package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Staff JWT (staff_id present, no user_id) must be able to register a push
// subscription keyed by principal_type=staff.
func TestCreatePushSubscription_StaffPrincipalNoUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.PushSubscription{}))

	business := createOwnedBusiness(t, "0xPushStaffOwner", "Push Staff Biz")
	business.IsActive = true
	require.NoError(t, db.Save(business).Error)
	staff := createStaffMember(t, business.ID, "push-staff@example.com", "Push Staff")

	body := bytes.NewBufferString(fmt.Sprintf(`{
		"business_id":%d,
		"endpoint":"https://fcm.googleapis.com/fcm/send/staff-principal",
		"p256dh_key":"key",
		"auth_key":"auth"
	}`, business.ID))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	// Staff context only — no user_id.
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", business.ID)
	c.Set("token_type", "staff")

	CreatePushSubscription(c)

	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var sub database.PushSubscription
	require.NoError(t, db.First(&sub).Error)
	assert.Equal(t, database.PushPrincipalStaff, sub.PrincipalType)
	assert.Equal(t, staff.ID, sub.PrincipalID)
	assert.Equal(t, business.ID, sub.BusinessID)
	assert.Equal(t, "https://fcm.googleapis.com/fcm/send/staff-principal", sub.Endpoint)
}

func TestCreatePushSubscription_OwnerPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.PushSubscription{}))

	ownerUserID := uint(77)
	business := &database.Business{
		BusinessId:     "push-owner-principal",
		Name:           "Owner Principal Biz",
		OwnerAddress:   "0xOwnerP",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		UserID:         &ownerUserID,
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)

	body := bytes.NewBufferString(fmt.Sprintf(`{
		"business_id":%d,
		"endpoint":"https://fcm.googleapis.com/fcm/send/owner-principal",
		"p256dh_key":"key",
		"auth_key":"auth"
	}`, business.ID))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", ownerUserID)

	CreatePushSubscription(c)

	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var sub database.PushSubscription
	require.NoError(t, db.First(&sub).Error)
	assert.Equal(t, database.PushPrincipalOwnerUser, sub.PrincipalType)
	assert.Equal(t, ownerUserID, sub.PrincipalID)
	assert.Equal(t, ownerUserID, sub.UserID)
}
