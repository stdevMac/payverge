package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetGuestOrdersByBillNumber_Success(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	order := &database.Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: fmt.Sprintf("O-%d", time.Now().UnixNano()),
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest",
		Items:       "[]",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, database.GetDB().Create(order).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetGuestOrdersByBillNumber(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp OrdersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Orders, 1)
	assert.Equal(t, bill.ID, resp.Orders[0].BillID)
}

func TestGetGuestOrdersByBillNumber_DoesNotLeakBusinessRecord(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(&database.Business{}).Where("id = ?", biz.ID).
		Updates(map[string]interface{}{
			"onboarding_state": `{"secret":"LEAKTEST"}`,
			"owner_address":    "0xLEAKOWNER",
		}).Error)
	bill := createTestBill(t, biz.ID)

	order := &database.Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: fmt.Sprintf("O-%d", time.Now().UnixNano()),
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest",
		Items:       "[]",
	}
	require.NoError(t, database.GetDB().Create(order).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetGuestOrdersByBillNumber(c)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.NotContains(t, body, "LEAKTEST")
	assert.NotContains(t, body, "0xLEAKOWNER")
	assert.NotContains(t, body, "onboarding_state")
	assert.NotContains(t, body, "owner_address")
	assert.NotContains(t, body, `"business"`)
}

func TestGetGuestOrdersByBillNumber_DoesNotLeakActorPII(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	order := &database.Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: fmt.Sprintf("O-%d", time.Now().UnixNano()),
		Status:      database.OrderStatusPending,
		CreatedBy:   "owner@example.com",
		ApprovedBy:  "manager@example.com",
		CancelledBy: "staff@example.com",
		Items:       "[]",
	}
	require.NoError(t, database.GetDB().Create(order).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetGuestOrdersByBillNumber(c)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.NotContains(t, body, "owner@example.com")
	assert.NotContains(t, body, "manager@example.com")
	assert.NotContains(t, body, "staff@example.com")
	assert.NotContains(t, body, `"created_by"`)
	assert.NotContains(t, body, `"approved_by"`)
	assert.NotContains(t, body, `"cancelled_by"`)

	// Guest-visible fields must still be present.
	assert.Contains(t, body, `"id"`)
	assert.Contains(t, body, `"bill_id"`)
	assert.Contains(t, body, `"status"`)
	assert.Contains(t, body, `"order_number"`)
}

func TestGetGuestOrdersByBillNumber_NotFound(t *testing.T) {
	setupHandlerTestDB(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: "missing"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetGuestOrdersByBillNumber(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestGetGuestOrdersByBillNumber_InactiveBusiness locks in the lean resolver's
// is_active gating: a bill belonging to a deactivated business must not be
// resolvable from the unauthenticated guest poll. (JSON-01/PRELOAD-01/OF-03)
func TestGetGuestOrdersByBillNumber_InactiveBusiness(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)
	require.NoError(t, database.GetDB().Model(&database.Business{}).Where("id = ?", biz.ID).
		Update("is_active", false).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetGuestOrdersByBillNumber(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
