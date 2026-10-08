package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/guestsession"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSplittingHandlerTestDB(t *testing.T) *SplittingHandler {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.BusinessMilestoneEvent{},
		&database.BusinessRevenueAggregate{},
		&database.BillSplitShare{},
	))
	require.NoError(t, createSQLitePaymentRegressionBillItemsTable(gormDB))

	database.SetTestDB(gormDB)
	return NewSplittingHandler(database.GetDBWrapper())
}

func createSplittingHandlerBill(
	t *testing.T,
	billNumber string,
	status database.BillStatus,
	items []database.BillItem,
) *database.Bill {
	t.Helper()

	business := &database.Business{
		BusinessId:     "biz-splitting-handler-" + billNumber,
		OwnerAddress:   "0x1111111111111111111111111111111111111111",
		Name:           "Split Test Business",
		SettlementAddr: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		TippingAddr:    "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
	}
	require.NoError(t, database.GetDB().Create(business).Error)

	itemsJSON, err := json.Marshal(items)
	require.NoError(t, err)

	bill := &database.Bill{
		BusinessID:       business.ID,
		BillNumber:       billNumber,
		Subtotal:         1500,
		TaxAmount:        150,
		ServiceFeeAmount: 75,
		TotalAmount:      1725,
		Status:           status,
		Items:            string(itemsJSON),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	for i := range items {
		items[i].BillID = bill.ID
	}
	if len(items) > 0 {
		require.NoError(t, database.GetDB().Create(&items).Error)
	}

	return bill
}

func TestGetBillSplitOptions_UsesBillNumber(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B42-opaque", database.BillStatusOpen, []database.BillItem{
		{ID: "burger-1", Name: "Burger", Price: 10, Quantity: 1, Subtotal: 10},
		{ID: "fries-1", Name: "Fries", Price: 5, Quantity: 1, Subtotal: 5},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/"+bill.PublicToken+"/split/options", nil)

	handler.GetBillSplitOptions(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"bill_number":"B42-opaque"`)
	assert.Contains(t, w.Body.String(), `"success":true`)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	billPayload, ok := payload["bill"].(map[string]any)
	require.True(t, ok)
	_, hasBillID := billPayload["id"]
	assert.False(t, hasBillID)

	itemsPayload, ok := payload["items"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, itemsPayload)
	firstItem, ok := itemsPayload[0].(map[string]any)
	require.True(t, ok)
	_, hasItemBillID := firstItem["bill_id"]
	assert.False(t, hasItemBillID)
}

func TestGetBillSplitOptions_ExcludesDiscountLines(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B43-discount", database.BillStatusOpen, []database.BillItem{
		{ID: "tea-1", Name: "Iced Tea", Price: 5, Quantity: 1, Subtotal: 5, ItemType: "menu_item"},
		{ID: "dessert-1", Name: "Chocolate Tart", Price: 12, Quantity: 1, Subtotal: 12, ItemType: "menu_item"},
		{ID: "disc-1", Name: "Weekday Lunch 15% Off", Price: -2.55, Quantity: 1, Subtotal: -2.55, ItemType: "discount"},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/"+bill.PublicToken+"/split/options", nil)

	handler.GetBillSplitOptions(c)
	require.Equal(t, http.StatusOK, w.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	itemsPayload, ok := payload["items"].([]any)
	require.True(t, ok)
	require.Len(t, itemsPayload, 2, "discount line must not appear as claimable")
	for _, raw := range itemsPayload {
		item, ok := raw.(map[string]any)
		require.True(t, ok)
		assert.NotEqual(t, "discount", item["item_type"])
		assert.NotEqual(t, "disc-1", item["id"])
	}
	options, ok := payload["split_options"].(map[string]any)
	require.True(t, ok)
	itemsOpt, ok := options["items"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 2, itemsOpt["total_items"])
}

func TestCalculateItemSplit_RejectsDuplicateAssignmentsByBillNumber(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B99-opaque", database.BillStatusOpen, []database.BillItem{
		{ID: "burger-1", Name: "Burger", Price: 10, Quantity: 1, Subtotal: 10},
		{ID: "fries-1", Name: "Fries", Price: 5, Quantity: 1, Subtotal: 5},
	})

	body, err := json.Marshal(map[string]any{
		"item_selections": map[string][]string{
			"alice": {"burger-1"},
			"bob":   {"burger-1", "fries-1"},
		},
		"people": map[string]string{
			"alice": "Alice",
			"bob":   "Bob",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/guest/bill/"+bill.PublicToken+"/split/items",
		bytes.NewReader(body),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	handler.CalculateItemSplit(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "assigned to multiple people")
}

func TestValidateSplitRejectsEqualSplitAboveMaxPeople(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B100-validate-cap", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/validate", handler.ValidateSplit)

	body, err := json.Marshal(map[string]any{
		"method":     "equal",
		"num_people": MaxSplitPeople + 1,
	})
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "Number of people must be between 1 and 100")
}

func TestCreateSplitHoldCustomUsesGuestSessionCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B77-hold", database.BillStatusOpen, []database.BillItem{
		{ID: "dessert-1", Name: "Cake", Price: 7, Quantity: 1, Subtotal: 7},
	})

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)

	body, err := json.Marshal(map[string]any{
		"mode":         "custom",
		"amount":       "6.25",
		"display_name": "Sara",
	})
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	require.NotEmpty(t, resp.Result().Cookies(), "hold creation must issue a no-login guest session cookie")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	assert.Equal(t, true, payload["success"])
	assert.Equal(t, 6.25, payload["share"].(map[string]any)["amount"])
	assert.Equal(t, 6.25, payload["state"].(map[string]any)["held_amount"])
	assert.Equal(t, 11.0, payload["state"].(map[string]any)["available_amount"])
}

func TestCreateSplitHoldCoverRemainingUsesAvailableAfterActiveHolds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B77-cover-remaining", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)

	firstBody, err := json.Marshal(map[string]any{
		"mode":   "custom",
		"amount": "6.25",
	})
	require.NoError(t, err)

	firstResp := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(firstBody))
	firstReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(firstResp, firstReq)
	require.Equal(t, http.StatusOK, firstResp.Code, firstResp.Body.String())

	remainingBody, err := json.Marshal(map[string]any{
		"mode":            "custom",
		"cover_remaining": true,
	})
	require.NoError(t, err)

	remainingResp := httptest.NewRecorder()
	remainingReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(remainingBody))
	remainingReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(remainingResp, remainingReq)
	require.Equal(t, http.StatusOK, remainingResp.Code, remainingResp.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(remainingResp.Body.Bytes(), &payload))
	assert.Equal(t, true, payload["success"])
	assert.Equal(t, 11.0, payload["share"].(map[string]any)["amount"])
	assert.Equal(t, float64(1100), payload["share"].(map[string]any)["amount_cents"])
	assert.Equal(t, 17.25, payload["state"].(map[string]any)["held_amount"])
	assert.Equal(t, 0.0, payload["state"].(map[string]any)["available_amount"])
}

func TestCreateSplitHoldCustomClampsToAvailableAfterActiveHolds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B77-custom-clamp", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)

	firstBody, err := json.Marshal(map[string]any{
		"mode":   "custom",
		"amount": "6.25",
	})
	require.NoError(t, err)
	firstResp := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(firstBody))
	firstReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(firstResp, firstReq)
	require.Equal(t, http.StatusOK, firstResp.Code, firstResp.Body.String())

	oversizedBody, err := json.Marshal(map[string]any{
		"mode":   "custom",
		"amount": "20.00",
	})
	require.NoError(t, err)
	oversizedResp := httptest.NewRecorder()
	oversizedReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(oversizedBody))
	oversizedReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(oversizedResp, oversizedReq)
	require.Equal(t, http.StatusOK, oversizedResp.Code, oversizedResp.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(oversizedResp.Body.Bytes(), &payload))
	assert.Equal(t, true, payload["success"])
	assert.Equal(t, 11.0, payload["share"].(map[string]any)["amount"])
	assert.Equal(t, float64(1100), payload["share"].(map[string]any)["amount_cents"])
	assert.Equal(t, 17.25, payload["state"].(map[string]any)["held_amount"])
	assert.Equal(t, 0.0, payload["state"].(map[string]any)["available_amount"])
}

func TestCreateSplitHoldEqualUsesAvailableAfterActiveHolds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B77-equal-available", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)

	firstBody, err := json.Marshal(map[string]any{
		"mode":   "custom",
		"amount": "6.25",
	})
	require.NoError(t, err)
	firstResp := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(firstBody))
	firstReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(firstResp, firstReq)
	require.Equal(t, http.StatusOK, firstResp.Code, firstResp.Body.String())

	equalBody, err := json.Marshal(map[string]any{
		"mode":           "equal",
		"num_people":     2,
		"shares_covered": 1,
	})
	require.NoError(t, err)
	equalResp := httptest.NewRecorder()
	equalReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(equalBody))
	equalReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(equalResp, equalReq)
	require.Equal(t, http.StatusOK, equalResp.Code, equalResp.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(equalResp.Body.Bytes(), &payload))
	assert.Equal(t, true, payload["success"])
	assert.Equal(t, 5.5, payload["share"].(map[string]any)["amount"])
	assert.Equal(t, float64(550), payload["share"].(map[string]any)["amount_cents"])
	assert.Equal(t, 11.75, payload["state"].(map[string]any)["held_amount"])
	assert.Equal(t, 5.5, payload["state"].(map[string]any)["available_amount"])
}

func TestCreateSplitHoldItemsComputesAmountFromPersistedFractions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B79-items-hold", database.BillStatusOpen, []database.BillItem{
		{ID: "shared-bottle", Name: "Shared Bottle", Price: 9, Quantity: 1, Subtotal: 9},
		{ID: "pasta", Name: "Pasta", Price: 6, Quantity: 1, Subtotal: 6},
	})

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)

	body, err := json.Marshal(map[string]any{
		"mode":              "items",
		"claimed_item_ids":  []string{"shared-bottle"},
		"claimed_fractions": map[string]string{"shared-bottle": "1/3"},
		"display_name":      "Sara",
	})
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	assert.Equal(t, true, payload["success"])
	assert.Equal(t, 3.45, payload["share"].(map[string]any)["amount"])
	assert.Equal(t, float64(345), payload["share"].(map[string]any)["amount_cents"])
	assert.Equal(t, 3.45, payload["state"].(map[string]any)["held_amount"])
}

func TestCreateSplitHoldPublishesGuestSafeSplitEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B81-hold-event", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(bill.BusinessID, 0)
	defer cancel()

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)

	body, err := json.Marshal(map[string]any{
		"mode":   "custom",
		"amount": "5.00",
	})
	require.NoError(t, err)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	select {
	case event := <-ch:
		require.Equal(t, "bill.split.updated", event.Type)
		assert.Contains(t, string(event.Data), `"bill_number":"B81-hold-event"`)
		assert.Contains(t, string(event.Data), `"held_cents":500`)
		assert.NotContains(t, string(event.Data), `"business_id"`)
	case <-time.After(time.Second):
		t.Fatal("expected split hold to publish bill.split.updated")
	}
}

func TestStreamSplitEventsReplaysOnlyGuestSafeSplitState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B80-split-events", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.GET("/guest/bill/:bill_token/split/events", handler.StreamSplitEvents)

	hub := events.GetHub()
	hub.Publish(events.BusinessEvent{
		ID:         9001,
		BusinessID: bill.BusinessID,
		Type:       "bill.updated",
		Data:       json.RawMessage(`{"bill_number":"B80-split-events","owner_secret":"do-not-leak"}`),
		Timestamp:  time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
	})
	hub.Publish(events.BusinessEvent{
		ID:         9002,
		BusinessID: bill.BusinessID,
		Type:       "bill.split.updated",
		Data:       json.RawMessage(`{"bill_number":"B80-split-events","paid_cents":0,"held_cents":500,"available_cents":1225}`),
		Timestamp:  time.Date(2026, 6, 13, 12, 0, 1, 0, time.UTC),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/guest/bill/"+bill.PublicToken+"/split/events", nil).WithContext(ctx)
	req.Header.Set("Last-Event-ID", "9000")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	require.Equal(t, http.StatusOK, resp.Code, body)
	assert.Contains(t, body, "event: bill.split.updated")
	assert.Contains(t, body, `"bill_number":"B80-split-events"`)
	assert.NotContains(t, body, "owner_secret")
	assert.False(t, strings.Contains(body, `"business_id"`), body)
}

func TestStreamSplitEventsFreshConnectSkipsRetainedRing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B80-fresh-replay", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.GET("/guest/bill/:bill_token/split/events", handler.StreamSplitEvents)

	hub := events.GetHub()
	hub.Publish(events.BusinessEvent{
		ID:         9101,
		BusinessID: bill.BusinessID,
		Type:       "bill.split.updated",
		Data:       json.RawMessage(`{"bill_number":"B80-fresh-replay","display_name":"QA-lane","held_amount":11.29}`),
		Timestamp:  time.Date(2026, 8, 16, 14, 34, 36, 0, time.UTC),
	})
	hub.Publish(events.BusinessEvent{
		ID:         9102,
		BusinessID: bill.BusinessID,
		Type:       "bill.split.updated",
		Data:       json.RawMessage(`{"bill_number":"B80-fresh-replay","display_name":"QA-eq","held_amount":8.75}`),
		Timestamp:  time.Date(2026, 8, 16, 14, 35, 15, 0, time.UTC),
	})

	fresh := func(lastEventID string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		req := httptest.NewRequest(http.MethodGet, "/guest/bill/"+bill.PublicToken+"/split/events", nil).WithContext(ctx)
		if lastEventID != "" {
			req.Header.Set("Last-Event-ID", lastEventID)
		}
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		return resp.Body.String()
	}

	noHeader := fresh("")
	assert.Contains(t, noHeader, "event: connected")
	assert.NotContains(t, noHeader, "QA-lane")
	assert.NotContains(t, noHeader, "QA-eq")
	assert.NotContains(t, noHeader, "event: bill.split.updated")

	garbage := fresh("abc:xyz")
	assert.Contains(t, garbage, "event: connected")
	assert.NotContains(t, garbage, "QA-lane")
	assert.NotContains(t, garbage, "event: bill.split.updated")

	resumed := fresh("9101")
	assert.Contains(t, resumed, "event: bill.split.updated")
	assert.Contains(t, resumed, "QA-eq")
	assert.NotContains(t, resumed, "QA-lane")
}

// TestStreamSplitEventsRejectsOverConcurrencyCap pins the held-connection DoS
// guard: once the hub's per-business concurrent-connection ceiling is full, the
// public split SSE returns 429 (with Retry-After) WITHOUT opening another
// stream, and releasing a held connection frees a slot so the next request is
// served again. Rate limiters count arrivals; this asserts the hub bounds
// concurrently-HELD streams.
func TestStreamSplitEventsRejectsOverConcurrencyCap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B80-cap", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.GET("/guest/bill/:bill_token/split/events", handler.StreamSplitEvents)

	hub := events.GetHub()
	// The public split stream draws on the hub's guest pool (M-sse). Tighten
	// the guest per-business cap to 1 for this test; high per-IP cap so the
	// per-business cap is the binding constraint. Restore generous limits after
	// so other tests sharing the singleton hub are unaffected.
	hub.SetGuestConnectionLimits(1, 1000)
	t.Cleanup(func() { hub.SetGuestConnectionLimits(1000, 1000) })

	// An authenticated stream for the same business never blocks guests.
	_, _, staffCancel, staffOK := hub.SubscribeLimited(bill.BusinessID, 0, "8.8.8.8", nil)
	require.True(t, staffOK)
	t.Cleanup(staffCancel)

	// Occupy the single guest slot for this business with a live subscription.
	_, _, holdCancel, ok := hub.SubscribeGuestLimited(bill.BusinessID, 0, "9.9.9.9", []string{"bill.split.updated"})
	require.True(t, ok, "first subscription within cap must be accepted")

	// A request now must be rejected with 429 before any SSE stream is opened.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/guest/bill/"+bill.PublicToken+"/split/events", nil).WithContext(ctx)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusTooManyRequests, resp.Code, resp.Body.String())
	assert.NotEmpty(t, resp.Header().Get("Retry-After"))
	assert.NotContains(t, resp.Header().Get("Content-Type"), "text/event-stream")

	// Releasing the held connection frees the slot; the next request streams.
	holdCancel()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel2()
	req2 := httptest.NewRequest(http.MethodGet, "/guest/bill/"+bill.PublicToken+"/split/events", nil).WithContext(ctx2)
	resp2 := httptest.NewRecorder()
	router.ServeHTTP(resp2, req2)

	require.Equal(t, http.StatusOK, resp2.Code, resp2.Body.String())
	assert.Contains(t, resp2.Header().Get("Content-Type"), "text/event-stream")
}

func TestStreamSplitEventsIssuesGuestSessionCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B80-split-cookie", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.GET("/guest/bill/:bill_token/split/events", handler.StreamSplitEvents)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/guest/bill/"+bill.PublicToken+"/split/events", nil).WithContext(ctx)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var sessionCookie *http.Cookie
	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == guestSplitSessionCookie {
			sessionCookie = cookie
			break
		}
	}
	require.NotNil(t, sessionCookie, "split event stream should issue the signed guest session cookie")
	assert.True(t, sessionCookie.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, sessionCookie.SameSite)
	sessionID, ok := guestsession.Verify(sessionCookie.Value)
	require.True(t, ok)
	assert.NotEmpty(t, sessionID)
}

func TestGetSplitStateDoesNotLeakInternalPaymentIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B80-safe-state", database.BillStatusPartial, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})
	payment := &database.Payment{
		BillID:    bill.ID,
		Amount:    500,
		TxHash:    "safe-state-payment",
		PayerAddr: "0xguest",
	}
	require.NoError(t, database.GetDB().Create(payment).Error)
	alt := &database.AlternativePayment{
		BillID:          bill.ID,
		Amount:          250,
		PaymentMethod:   "cash",
		Status:          "confirmed",
		ParticipantAddr: "guest_split_share:22",
		ParticipantName: "Guest",
	}
	require.NoError(t, database.GetDB().Create(alt).Error)
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	require.NoError(t, database.GetDB().Create(&database.BillSplitShare{
		BillID:               bill.ID,
		GuestSessionID:       "guest-safe-state",
		DisplayName:          "Sara",
		Mode:                 database.BillSplitModeCustom,
		AmountCents:          500,
		Status:               database.BillSplitShareStatusSettled,
		PaymentID:            &payment.ID,
		AlternativePaymentID: &alt.ID,
		HoldExpiresAt:        &expiresAt,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/"+bill.PublicToken+"/split/state", nil)

	handler.GetSplitState(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	assert.Contains(t, body, `"bill_number":"B80-safe-state"`)
	assert.Contains(t, body, `"display_name":"Sara"`)
	assert.NotContains(t, body, "payment_id")
	assert.NotContains(t, body, "alternative_payment_id")
	assert.NotContains(t, body, "guest_session_id")
	assert.NotContains(t, body, "business_id")
}

func TestExecuteSplitPaymentSettlesHeldShareIdempotently(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B78-execute", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)
	router.POST("/guest/bill/:bill_token/split/execute", handler.ExecuteSplitPayment)

	holdBody, err := json.Marshal(map[string]any{
		"mode":   "custom",
		"amount": "5.00",
	})
	require.NoError(t, err)
	holdResp := httptest.NewRecorder()
	holdReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(holdBody))
	holdReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(holdResp, holdReq)
	require.Equal(t, http.StatusOK, holdResp.Code, holdResp.Body.String())

	var holdPayload map[string]any
	require.NoError(t, json.Unmarshal(holdResp.Body.Bytes(), &holdPayload))
	shareID := uint(holdPayload["share"].(map[string]any)["id"].(float64))

	_, paymentApplied, err := database.ApplyConfirmedPayment(database.ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "crypto_guest",
		Amount:        500,
		TipAmount:     125,
		TxHash:        "split-handler-tx",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}, nil)
	require.NoError(t, err)
	require.True(t, paymentApplied)

	executeBody, err := json.Marshal(map[string]any{
		"share_id":         shareID,
		"payment_method":   "crypto",
		"transaction_hash": "split-handler-tx",
		"payer_address":    "0xguest",
		"tip_amount":       "1.25",
	})
	require.NoError(t, err)
	execResp := httptest.NewRecorder()
	execReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/execute", bytes.NewReader(executeBody))
	execReq.Header.Set("Content-Type", "application/json")
	execReq.Header.Set("X-Request-Id", "split-handler-request")
	for _, cookie := range holdResp.Result().Cookies() {
		execReq.AddCookie(cookie)
	}
	router.ServeHTTP(execResp, execReq)

	require.Equal(t, http.StatusOK, execResp.Code, execResp.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(execResp.Body.Bytes(), &payload))
	assert.Equal(t, true, payload["success"])
	assert.Equal(t, true, payload["applied"])
	assert.Equal(t, "partial", payload["bill_status"])
	assert.Equal(t, 12.25, payload["remaining_amount"])

	retryResp := httptest.NewRecorder()
	retryReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/execute", bytes.NewReader(executeBody))
	retryReq.Header.Set("Content-Type", "application/json")
	retryReq.Header.Set("X-Request-Id", "split-handler-request")
	for _, cookie := range holdResp.Result().Cookies() {
		retryReq.AddCookie(cookie)
	}
	router.ServeHTTP(retryResp, retryReq)
	require.Equal(t, http.StatusOK, retryResp.Code, retryResp.Body.String())

	var retryPayload map[string]any
	require.NoError(t, json.Unmarshal(retryResp.Body.Bytes(), &retryPayload))
	assert.Equal(t, false, retryPayload["applied"])

	var paymentCount int64
	require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
	assert.EqualValues(t, 1, paymentCount)
}

func TestGetSplitShareReceiptReturnsOnlyGuestOwnedSettledShare(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B82-receipt", database.BillStatusOpen, []database.BillItem{
		{ID: "shared-bottle", Name: "Shared Bottle", Price: 9, Quantity: 1, Subtotal: 9},
		{ID: "pasta", Name: "Pasta", Price: 6, Quantity: 1, Subtotal: 6},
	})

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)
	router.POST("/guest/bill/:bill_token/split/execute", handler.ExecuteSplitPayment)
	router.GET("/guest/bill/:bill_token/split/shares/:share_id/receipt", handler.GetSplitShareReceipt)

	holdBody, err := json.Marshal(map[string]any{
		"mode":              "items",
		"claimed_item_ids":  []string{"shared-bottle"},
		"claimed_fractions": map[string]string{"shared-bottle": "1/3"},
		"display_name":      "Sara",
	})
	require.NoError(t, err)
	holdResp := httptest.NewRecorder()
	holdReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(holdBody))
	holdReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(holdResp, holdReq)
	require.Equal(t, http.StatusOK, holdResp.Code, holdResp.Body.String())

	var holdPayload map[string]any
	require.NoError(t, json.Unmarshal(holdResp.Body.Bytes(), &holdPayload))
	shareID := uint(holdPayload["share"].(map[string]any)["id"].(float64))

	_, paymentApplied, err := database.ApplyConfirmedPayment(database.ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "crypto_guest",
		Amount:        345,
		TipAmount:     125,
		TxHash:        "split-receipt-tx",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}, nil)
	require.NoError(t, err)
	require.True(t, paymentApplied)

	executeBody, err := json.Marshal(map[string]any{
		"share_id":         shareID,
		"payment_method":   "crypto",
		"transaction_hash": "split-receipt-tx",
		"payer_address":    "0xguest",
		"tip_amount":       "1.25",
	})
	require.NoError(t, err)
	execResp := httptest.NewRecorder()
	execReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/execute", bytes.NewReader(executeBody))
	execReq.Header.Set("Content-Type", "application/json")
	execReq.Header.Set("X-Request-Id", "split-receipt-request")
	for _, cookie := range holdResp.Result().Cookies() {
		execReq.AddCookie(cookie)
	}
	router.ServeHTTP(execResp, execReq)
	require.Equal(t, http.StatusOK, execResp.Code, execResp.Body.String())

	unauthorizedResp := httptest.NewRecorder()
	unauthorizedReq := httptest.NewRequest(http.MethodGet, "/guest/bill/"+bill.PublicToken+"/split/shares/"+strconv.FormatUint(uint64(shareID), 10)+"/receipt", nil)
	router.ServeHTTP(unauthorizedResp, unauthorizedReq)
	require.Equal(t, http.StatusForbidden, unauthorizedResp.Code)

	receiptResp := httptest.NewRecorder()
	receiptReq := httptest.NewRequest(http.MethodGet, "/guest/bill/"+bill.PublicToken+"/split/shares/"+strconv.FormatUint(uint64(shareID), 10)+"/receipt", nil)
	for _, cookie := range holdResp.Result().Cookies() {
		receiptReq.AddCookie(cookie)
	}
	router.ServeHTTP(receiptResp, receiptReq)
	require.Equal(t, http.StatusOK, receiptResp.Code, receiptResp.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(receiptResp.Body.Bytes(), &payload))
	assert.Equal(t, true, payload["success"])
	receipt := payload["receipt"].(map[string]any)
	assert.Equal(t, float64(shareID), receipt["share_id"])
	assert.Equal(t, "B82-receipt", receipt["bill_number"])
	assert.Equal(t, "Sara", receipt["display_name"])
	assert.Equal(t, "settled", receipt["status"])
	assert.Equal(t, "crypto", receipt["tender"])
	assert.Equal(t, float64(300), receipt["subtotal_cents"])
	assert.Equal(t, float64(30), receipt["tax_cents"])
	assert.Equal(t, float64(15), receipt["service_fee_cents"])
	assert.Equal(t, float64(345), receipt["amount_cents"])
	assert.Equal(t, float64(125), receipt["tip_cents"])
	assert.Equal(t, float64(470), receipt["grand_total_cents"])
	assert.NotContains(t, receiptResp.Body.String(), "business_id")

	items := receipt["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.Equal(t, "shared-bottle", item["id"])
	assert.Equal(t, "Shared Bottle", item["name"])
	assert.Equal(t, "1/3", item["fraction"])
	assert.Equal(t, float64(300), item["subtotal_cents"])
}

func TestExecuteSplitPaymentRejectsUnconfirmedExternalTender(t *testing.T) {
	for _, tc := range []struct {
		name   string
		suffix string
		tender string
		body   map[string]any
	}{
		{
			name:   "crypto",
			suffix: "crypto",
			tender: "crypto",
			body: map[string]any{
				"payment_method":   "crypto",
				"transaction_hash": "0xunverified-split-crypto",
				"payer_address":    "0xguest",
			},
		},
		{
			name:   "cross chain",
			suffix: "cross-chain",
			tender: "cross-chain",
			body: map[string]any{
				"payment_method":   "cross-chain",
				"transaction_hash": "0xunverified-split-cross-chain",
				"payer_address":    "0xguest",
				"source_chain":     "ethereum",
				"source_token":     "USDC",
			},
		},
		{
			name:   "plugin",
			suffix: "plugin",
			tender: "plugin",
			body: map[string]any{
				"payment_method":   "plugin",
				"transaction_hash": "plugin_unconfirmed_split",
				"payer_address":    "plugin",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			handler := setupSplittingHandlerTestDB(t)
			bill := createSplittingHandlerBill(t, "B85-unconfirmed-"+tc.suffix, database.BillStatusOpen, []database.BillItem{
				{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
			})

			router := gin.New()
			router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)
			router.POST("/guest/bill/:bill_token/split/execute", handler.ExecuteSplitPayment)

			holdBody, err := json.Marshal(map[string]any{
				"mode":   "custom",
				"amount": "5.00",
			})
			require.NoError(t, err)
			holdResp := httptest.NewRecorder()
			holdReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(holdBody))
			holdReq.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(holdResp, holdReq)
			require.Equal(t, http.StatusOK, holdResp.Code, holdResp.Body.String())

			var holdPayload map[string]any
			require.NoError(t, json.Unmarshal(holdResp.Body.Bytes(), &holdPayload))
			shareID := uint(holdPayload["share"].(map[string]any)["id"].(float64))

			tc.body["share_id"] = shareID
			executeBody, err := json.Marshal(tc.body)
			require.NoError(t, err)
			execResp := httptest.NewRecorder()
			execReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/execute", bytes.NewReader(executeBody))
			execReq.Header.Set("Content-Type", "application/json")
			execReq.Header.Set("X-Request-Id", "unconfirmed-"+tc.tender)
			for _, cookie := range holdResp.Result().Cookies() {
				execReq.AddCookie(cookie)
			}
			router.ServeHTTP(execResp, execReq)
			require.Equal(t, http.StatusConflict, execResp.Code, execResp.Body.String())

			var paymentCount int64
			require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
			assert.EqualValues(t, 0, paymentCount)

			var reloadedBill database.Bill
			require.NoError(t, database.GetDB().First(&reloadedBill, bill.ID).Error)
			assert.Equal(t, int64(0), reloadedBill.PaidAmount)
			assert.Equal(t, database.BillStatusOpen, reloadedBill.Status)
		})
	}
}

func TestExecuteSplitPaymentRejectsCashierTenderWithoutStaffConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B86-cashier-direct", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)
	router.POST("/guest/bill/:bill_token/split/execute", handler.ExecuteSplitPayment)

	holdBody, err := json.Marshal(map[string]any{
		"mode":   "custom",
		"amount": "5.00",
	})
	require.NoError(t, err)
	holdResp := httptest.NewRecorder()
	holdReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(holdBody))
	holdReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(holdResp, holdReq)
	require.Equal(t, http.StatusOK, holdResp.Code, holdResp.Body.String())

	var holdPayload map[string]any
	require.NoError(t, json.Unmarshal(holdResp.Body.Bytes(), &holdPayload))
	shareID := uint(holdPayload["share"].(map[string]any)["id"].(float64))

	executeBody, err := json.Marshal(map[string]any{
		"share_id":       shareID,
		"payment_method": "cash",
		"payer_address":  "cashier",
	})
	require.NoError(t, err)
	execResp := httptest.NewRecorder()
	execReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/execute", bytes.NewReader(executeBody))
	execReq.Header.Set("Content-Type", "application/json")
	execReq.Header.Set("X-Request-Id", "direct-cashier")
	for _, cookie := range holdResp.Result().Cookies() {
		execReq.AddCookie(cookie)
	}
	router.ServeHTTP(execResp, execReq)
	require.Equal(t, http.StatusBadRequest, execResp.Code, execResp.Body.String())

	var altCount int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).Where("bill_id = ?", bill.ID).Count(&altCount).Error)
	assert.EqualValues(t, 0, altCount)

	var reloadedBill database.Bill
	require.NoError(t, database.GetDB().First(&reloadedBill, bill.ID).Error)
	assert.Equal(t, int64(0), reloadedBill.PaidAmount)
	assert.Equal(t, database.BillStatusOpen, reloadedBill.Status)
}

func TestGetMySplitSharesReturnsOnlyCurrentGuestShares(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B83-my-shares", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)
	router.GET("/guest/bill/:bill_token/split/my-shares", handler.GetMySplitShares)

	firstBody, err := json.Marshal(map[string]any{
		"mode":         "custom",
		"amount":       "5.00",
		"display_name": "Sara",
	})
	require.NoError(t, err)
	firstHold := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(firstBody))
	firstReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(firstHold, firstReq)
	require.Equal(t, http.StatusOK, firstHold.Code, firstHold.Body.String())

	secondBody, err := json.Marshal(map[string]any{
		"mode":         "custom",
		"amount":       "4.00",
		"display_name": "Luis",
	})
	require.NoError(t, err)
	secondHold := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(secondBody))
	secondReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(secondHold, secondReq)
	require.Equal(t, http.StatusOK, secondHold.Code, secondHold.Body.String())

	recovery := httptest.NewRecorder()
	recoveryReq := httptest.NewRequest(http.MethodGet, "/guest/bill/"+bill.PublicToken+"/split/my-shares", nil)
	for _, cookie := range firstHold.Result().Cookies() {
		recoveryReq.AddCookie(cookie)
	}
	router.ServeHTTP(recovery, recoveryReq)
	require.Equal(t, http.StatusOK, recovery.Code, recovery.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recovery.Body.Bytes(), &payload))
	require.Equal(t, true, payload["success"])
	shares := payload["shares"].([]any)
	require.Len(t, shares, 1)
	share := shares[0].(map[string]any)
	assert.Equal(t, "Sara", share["display_name"])
	assert.Equal(t, float64(500), share["amount_cents"])
}

func TestReleaseSplitShareFreesHeldAmountAndPublishesState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B84-release-share", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 17.25, Quantity: 1, Subtotal: 17.25},
	})

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)
	router.POST("/guest/bill/:bill_token/split/shares/:share_id/release", handler.ReleaseSplitShare)

	holdBody, err := json.Marshal(map[string]any{
		"mode":   "custom",
		"amount": "5.00",
	})
	require.NoError(t, err)
	holdResp := httptest.NewRecorder()
	holdReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(holdBody))
	holdReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(holdResp, holdReq)
	require.Equal(t, http.StatusOK, holdResp.Code, holdResp.Body.String())

	var holdPayload map[string]any
	require.NoError(t, json.Unmarshal(holdResp.Body.Bytes(), &holdPayload))
	shareID := uint(holdPayload["share"].(map[string]any)["id"].(float64))

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(bill.BusinessID, 0)
	defer cancel()

	releaseResp := httptest.NewRecorder()
	releaseReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/shares/"+strconv.FormatUint(uint64(shareID), 10)+"/release", nil)
	for _, cookie := range holdResp.Result().Cookies() {
		releaseReq.AddCookie(cookie)
	}
	router.ServeHTTP(releaseResp, releaseReq)
	require.Equal(t, http.StatusOK, releaseResp.Code, releaseResp.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(releaseResp.Body.Bytes(), &payload))
	assert.Equal(t, true, payload["success"])
	assert.Equal(t, "released", payload["share"].(map[string]any)["status"])
	assert.Equal(t, float64(0), payload["state"].(map[string]any)["held_cents"])
	assert.Equal(t, float64(1725), payload["state"].(map[string]any)["available_cents"])

	select {
	case event := <-ch:
		require.Equal(t, "bill.split.updated", event.Type)
		assert.Contains(t, string(event.Data), `"held_cents":0`)
		assert.Contains(t, string(event.Data), `"available_cents":1725`)
	case <-time.After(time.Second):
		t.Fatal("expected release to publish bill.split.updated")
	}

	unauthorized := httptest.NewRecorder()
	unauthorizedReq := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/shares/"+strconv.FormatUint(uint64(shareID), 10)+"/release", nil)
	router.ServeHTTP(unauthorized, unauthorizedReq)
	require.Equal(t, http.StatusForbidden, unauthorized.Code)
}

func TestExecuteSplitPaymentRejectsSecondShareWithSamePayment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupSplittingHandlerTestDB(t)
	bill := createSplittingHandlerBill(t, "B99-double-settle", database.BillStatusOpen, []database.BillItem{
		{ID: "main-1", Name: "Main", Price: 10.00, Quantity: 1, Subtotal: 10.00},
	})
	txHash := "double-settle-tx"

	router := gin.New()
	router.POST("/guest/bill/:bill_token/split/holds", handler.CreateSplitHold)
	router.POST("/guest/bill/:bill_token/split/execute", handler.ExecuteSplitPayment)

	// Create two equal-amount held shares.
	holdBody, err := json.Marshal(map[string]any{"mode": "custom", "amount": "5.00"})
	require.NoError(t, err)

	holdResp1 := httptest.NewRecorder()
	holdReq1 := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(holdBody))
	holdReq1.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(holdResp1, holdReq1)
	require.Equal(t, http.StatusOK, holdResp1.Code, holdResp1.Body.String())
	var h1 map[string]any
	require.NoError(t, json.Unmarshal(holdResp1.Body.Bytes(), &h1))
	share1ID := uint(h1["share"].(map[string]any)["id"].(float64))

	holdResp2 := httptest.NewRecorder()
	holdReq2 := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/holds", bytes.NewReader(holdBody))
	holdReq2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(holdResp2, holdReq2)
	require.Equal(t, http.StatusOK, holdResp2.Code, holdResp2.Body.String())
	var h2 map[string]any
	require.NoError(t, json.Unmarshal(holdResp2.Body.Bytes(), &h2))
	share2ID := uint(h2["share"].(map[string]any)["id"].(float64))

	// Apply one confirmed payment of $5.00.
	_, paymentApplied, err := database.ApplyConfirmedPayment(database.ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "crypto_guest_1",
		Amount:        500,
		TipAmount:     0,
		TxHash:        txHash,
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}, nil)
	require.NoError(t, err)
	require.True(t, paymentApplied)

	executeBody := func(shareID uint, key string) *http.Request {
		b, _ := json.Marshal(map[string]any{
			"share_id":         shareID,
			"payment_method":   "crypto",
			"transaction_hash": txHash,
			"payer_address":    "0xguest",
			"tip_amount":       "0",
			"idempotency_key":  key,
		})
		req := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/split/execute", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	// Settle share1 — should succeed.
	req1 := executeBody(share1ID, "double-key-1")
	for _, cookie := range holdResp1.Result().Cookies() {
		req1.AddCookie(cookie)
	}
	execResp1 := httptest.NewRecorder()
	router.ServeHTTP(execResp1, req1)
	require.Equal(t, http.StatusOK, execResp1.Code, execResp1.Body.String())

	// Settle share2 with the SAME tx hash, different idempotency key — 409.
	req2 := executeBody(share2ID, "double-key-2")
	for _, cookie := range holdResp2.Result().Cookies() {
		req2.AddCookie(cookie)
	}
	execResp2 := httptest.NewRecorder()
	router.ServeHTTP(execResp2, req2)
	require.Equal(t, http.StatusConflict, execResp2.Code, execResp2.Body.String())

	// Ensure only one payment was applied.
	var paymentCount int64
	require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
	assert.EqualValues(t, 1, paymentCount)

	// Ensure paid_amount was charged only once.
	updatedBill, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(500), updatedBill.PaidAmount)
}
