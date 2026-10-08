package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStaffMutation_RejectsCrossRouteBusiness proves that a multi-business owner
// cannot mutate Business B's staff through a /businesses/A/... URL. The actor
// owns both businesses (so CheckBusinessAccess against B passes), but the route
// :id points at A while the target staff belongs to B — this must be rejected so
// the audit trail isn't mis-attributed to the wrong business.
func TestStaffMutation_RejectsCrossRouteBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupRBACLimitDB(t)

	owner := "0xownermultibiz"

	bizA := &database.Business{
		BusinessId:      "biz-a",
		Name:            "Biz A",
		OwnerAddress:    owner,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(t, db.GetGorm().Create(bizA).Error)

	bizB := &database.Business{
		BusinessId:      "biz-b",
		Name:            "Biz B",
		OwnerAddress:    owner,
		SettlementAddr:  "0x3333333333333333333333333333333333333333",
		TippingAddr:     "0x4444444444444444444444444444444444444444",
		DefaultCurrency: "USD",
	}
	require.NoError(t, db.GetGorm().Create(bizB).Error)

	// Target staff belongs to Business B.
	staffB := &database.Staff{
		BusinessID: bizB.ID,
		Email:      "staffb@example.com",
		Name:       "Staff B",
		Role:       database.StaffRoleServer,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, db.GetGorm().Create(staffB).Error)

	handlers := NewRBACHandlers(db)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", owner)
		c.Set("business_owner_address", owner)
		c.Next()
	})
	// Route :id is Business A; target staff lives in Business B.
	router.POST("/inside/businesses/:id/staff/:staffId/deactivate", handlers.DeactivateStaff)

	url := fmt.Sprintf("/inside/businesses/%s/staff/%d/deactivate", bizA.BusinessId, staffB.ID)
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{"reason":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code,
		"mutating Business B staff through a /businesses/A URL must be rejected; body=%s", w.Body.String())

	// And the staff row must NOT have been deactivated.
	var after database.Staff
	require.NoError(t, db.GetGorm().First(&after, staffB.ID).Error)
	assert.True(t, after.IsActive, "target staff must remain active when the mutation is rejected")
}

// TestStaffMutation_AllowsMatchingRouteBusiness is the positive control: the
// same owner mutating Business B staff through the correct /businesses/B URL
// must succeed.
func TestStaffMutation_AllowsMatchingRouteBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupRBACLimitDB(t)

	owner := "0xownersinglebiz"

	bizB := &database.Business{
		BusinessId:      "biz-b-ok",
		Name:            "Biz B OK",
		OwnerAddress:    owner,
		SettlementAddr:  "0x5555555555555555555555555555555555555555",
		TippingAddr:     "0x6666666666666666666666666666666666666666",
		DefaultCurrency: "USD",
	}
	require.NoError(t, db.GetGorm().Create(bizB).Error)

	staffB := &database.Staff{
		BusinessID: bizB.ID,
		Email:      "staffbok@example.com",
		Name:       "Staff B OK",
		Role:       database.StaffRoleServer,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, db.GetGorm().Create(staffB).Error)

	handlers := NewRBACHandlers(db)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", owner)
		c.Set("business_owner_address", owner)
		c.Next()
	})
	router.POST("/inside/businesses/:id/staff/:staffId/deactivate", handlers.DeactivateStaff)

	url := fmt.Sprintf("/inside/businesses/%s/staff/%d/deactivate", bizB.BusinessId, staffB.ID)
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{"reason":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code,
		"owner mutating their own business staff via the correct URL must succeed; body=%s", w.Body.String())
}
