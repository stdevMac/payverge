package server

// SSE emit-correctness regression tests (bills/payments flow).
//
// The operator dashboard subscribes to bill.created / bill.updated /
// bill.closed / payment.received and refetches on each. These tests pin the
// contract that every operator-visible bill mutation actually publishes to
// the hub — before the fix, UpdateBill / AddBillItem / AdjustBillItem /
// loyalty redemption were silent, leaving dashboards stale until the next
// poll. Payment-settlement SSE (payment.received) is covered on the live
// MarkAlternativePayment path in internal/handlers/sse_emit_correctness_test.go.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// collectBusinessEventTypes drains every event already buffered (plus any that
// arrive within the grace window) and returns the type list.
func collectBusinessEventTypes(ch <-chan events.BusinessEvent, grace time.Duration) []string {
	var types []string
	for {
		select {
		case ev := <-ch:
			types = append(types, ev.Type)
		case <-time.After(grace):
			return types
		}
	}
}

func setupBillSSEEmitTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.BillHistoryEvent{},
		&database.BusinessMilestoneEvent{},
		&database.BusinessRevenueAggregate{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
		&database.BusinessAlertSettings{},
	))
	createSQLiteBusinessRegressionBillItemsTable(t, gormDB)
	return gormDB
}

func newBillSSEOwnerRouter(ownerAddress string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", ownerAddress)
		c.Next()
	})
	return router
}

func createBillSSEOpenBill(t *testing.T, businessID uint, totalCents int64) *database.Bill {
	t.Helper()
	bill := &database.Bill{
		BusinessID:     businessID,
		BillNumber:     fmt.Sprintf("SSE-%d-%d", businessID, time.Now().UnixNano()),
		Status:         database.BillStatusOpen,
		Subtotal:       totalCents,
		TotalAmount:    totalCents,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		Items:          "[]",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return bill
}

func TestUpdateBill_EmitsBillUpdatedSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBillSSEEmitTestDB(t)
	business := createOwnedBusiness(t, "0xSSEOwner1", "SSE Update Biz")
	bill := createBillSSEOpenBill(t, business.ID, 0)

	router := newBillSSEOwnerRouter("0xSSEOwner1")
	router.PUT("/bills/:bill_id", UpdateBill)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	w := performBusinessJSONRequest(t, router, http.MethodPut, fmt.Sprintf("/bills/%d", bill.ID), map[string]any{
		"items": []map[string]any{
			{"id": "item-1", "name": "Burger", "price": 10.0, "quantity": 1},
		},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	types := collectBusinessEventTypes(ch, 250*time.Millisecond)
	require.Contains(t, types, "bill.updated", "UpdateBill must publish bill.updated so dashboards refresh in realtime")
}

func TestAddBillItem_EmitsBillUpdatedSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBillSSEEmitTestDB(t)
	business := createOwnedBusiness(t, "0xSSEOwner2", "SSE AddItem Biz")
	bill := createBillSSEOpenBill(t, business.ID, 0)

	router := newBillSSEOwnerRouter("0xSSEOwner2")
	router.POST("/bills/:bill_id/items", AddBillItem)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	w := performBusinessJSONRequest(t, router, http.MethodPost, fmt.Sprintf("/bills/%d/items", bill.ID), map[string]any{
		"menu_item_id": "item-1",
		"name":         "Fries",
		"price":        4.5,
		"quantity":     2,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	types := collectBusinessEventTypes(ch, 250*time.Millisecond)
	require.Contains(t, types, "bill.updated", "AddBillItem must publish bill.updated")
}

func TestAdjustBillItem_EmitsBillUpdatedSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBillSSEEmitTestDB(t)
	business := createOwnedBusiness(t, "0xSSEOwner3", "SSE Adjust Biz")
	bill := createBillSSEOpenBill(t, business.ID, 0)

	router := newBillSSEOwnerRouter("0xSSEOwner3")
	router.POST("/bills/:bill_id/items", AddBillItem)
	router.PATCH("/bills/:bill_id/items/:item_id", AdjustBillItem)

	// Seed an item through the handler so the persisted shape matches.
	w := performBusinessJSONRequest(t, router, http.MethodPost, fmt.Sprintf("/bills/%d/items", bill.ID), map[string]any{
		"menu_item_id": "item-1",
		"name":         "Soup",
		"price":        6.0,
		"quantity":     1,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var addResp struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &addResp))
	require.NotEmpty(t, addResp.Items)
	itemID := addResp.Items[len(addResp.Items)-1].ID

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	quantity := 3
	w = performBusinessJSONRequest(t, router, http.MethodPatch, fmt.Sprintf("/bills/%d/items/%s", bill.ID, itemID), map[string]any{
		"quantity": quantity,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	types := collectBusinessEventTypes(ch, 250*time.Millisecond)
	require.Contains(t, types, "bill.updated", "AdjustBillItem must publish bill.updated")
}

func TestRedeemAndUndoLoyalty_EmitBillUpdatedSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupBillSSEEmitTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.LoyaltyProgram{},
		&database.LoyaltyTier{},
		&database.Customer{},
		&database.CustomerBusiness{},
	))

	business := createOwnedBusiness(t, "0xSSEOwner6", "SSE Loyalty Biz")
	require.NoError(t, database.GetDB().Create(&database.LoyaltyProgram{
		BusinessID:                business.ID,
		Enabled:                   true,
		PointsPerDollar:           1,
		RedemptionPointsPerDollar: 10,
	}).Error)

	table := &database.Table{BusinessID: business.ID, Name: "T1", TableCode: "sse-loyalty-t1", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)

	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "SSE-LOYALTY-1",
		Status:         database.BillStatusOpen,
		Subtotal:       2000,
		TotalAmount:    2000,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		Items:          "[]",
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	customer := &database.Customer{Email: "loyal@example.com", Name: "Loyal Guest"}
	require.NoError(t, database.GetDB().Create(customer).Error)
	require.NoError(t, database.GetDB().Create(&database.CustomerBusiness{
		CustomerID:    customer.ID,
		BusinessID:    business.ID,
		LoyaltyPoints: 500,
		IsActive:      true,
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("customer_id", customer.ID)
		c.Next()
	})
	router.POST("/table/:code/loyalty/redeem", RedeemLoyaltyPoints)
	router.POST("/table/:code/loyalty/undo", UndoLoyaltyRedemption)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	w := performBusinessJSONRequest(t, router, http.MethodPost, fmt.Sprintf("/table/%s/loyalty/redeem", table.TableCode), map[string]any{
		"points": 100,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	types := collectBusinessEventTypes(ch, 250*time.Millisecond)
	require.Contains(t, types, "bill.updated", "loyalty redemption changes the bill total and must publish bill.updated")

	w = performBusinessJSONRequest(t, router, http.MethodPost, fmt.Sprintf("/table/%s/loyalty/undo", table.TableCode), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	types = collectBusinessEventTypes(ch, 250*time.Millisecond)
	require.Contains(t, types, "bill.updated", "loyalty undo changes the bill total and must publish bill.updated")
}
