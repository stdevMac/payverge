package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func setupMercadoPagoPointHandlerTest(t *testing.T, apiHandler http.HandlerFunc) (
	business *database.Business,
	mpServer *httptest.Server,
	plugin *mercadopago.MercadoPagoPlugin,
) {
	t.Helper()
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.AlternativePayment{},
	))

	mpServer = httptest.NewServer(apiHandler)
	t.Cleanup(mpServer.Close)

	business = createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "ARS").Error)

	pluginRecord := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{
		"access_token":   "APP_USR-point-access-token-1234567890",
		"public_key":     "APP_USR-point-public-key-1234567890",
		"environment":    "sandbox",
		"base_url":       "https://api.staging.example",
		"api_base_url":   mpServer.URL,
		"webhook_secret": "mp-point-webhook-secret",
	}))

	plugin = mercadopago.NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))
	original, hadOriginal := plugins.GlobalRegistry.GetPlugin("mercadopago")
	plugins.GlobalRegistry.RegisterPlugin(plugin)
	prevResolver := resolveMercadoPagoPlugin
	resolveMercadoPagoPlugin = func() (*mercadopago.MercadoPagoPlugin, error) {
		return plugin, nil
	}
	t.Cleanup(func() {
		resolveMercadoPagoPlugin = prevResolver
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(original)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin("mercadopago")
		}
	})

	return business, mpServer, plugin
}

func createPointTestBill(t *testing.T, businessID uint, totalCents, paidCents int64) *database.Bill {
	t.Helper()
	bill := &database.Bill{
		BusinessID:  businessID,
		BillNumber:  fmt.Sprintf("B-point-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: totalCents,
		PaidAmount:  paidCents,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return bill
}

func TestListMercadoPagoTerminals_ProxiesPlugin(t *testing.T) {
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/terminals/v1/list", r.URL.Path)
		_, _ = w.Write([]byte(`{
			"data":{"terminals":[{
				"id":"NEWLAND_N950__T1",
				"pos_id":"1",
				"store_id":"2",
				"external_pos_id":"POS1",
				"operating_mode":"STANDALONE"
			}]}
		}`))
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/terminals", nil)

	NewPluginHandlers(nil, nil).ListMercadoPagoTerminals(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Terminals []struct {
			ID            string `json:"id"`
			OperatingMode string `json:"operating_mode"`
			ExternalPosID string `json:"external_pos_id"`
		} `json:"terminals"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Terminals, 1)
	assert.Equal(t, "NEWLAND_N950__T1", body.Terminals[0].ID)
	assert.Equal(t, "STANDALONE", body.Terminals[0].OperatingMode)
	assert.Equal(t, "POS1", body.Terminals[0].ExternalPosID)
}

func TestSetMercadoPagoTerminalMode_PDV(t *testing.T) {
	var setupBody map[string]interface{}
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPatch, r.Method)
		require.Equal(t, "/terminals/v1/setup", r.URL.Path)
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &setupBody))
		_, _ = w.Write([]byte(`{}`))
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "terminal_id", Value: "NEWLAND_N950__T1"},
	}
	c.Request = httptest.NewRequest(http.MethodPatch, "/mode", strings.NewReader(`{"mode":"PDV"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).SetMercadoPagoTerminalMode(c)

	require.Equal(t, http.StatusOK, w.Code)
	terms, ok := setupBody["terminals"].([]interface{})
	require.True(t, ok)
	require.Len(t, terms, 1)
	t0 := terms[0].(map[string]interface{})
	assert.Equal(t, "NEWLAND_N950__T1", t0["id"])
	assert.Equal(t, "PDV", t0["operating_mode"])
}

// R2: CLP remaining 150050 must floor to 150000 (never overshoot remaining)
// so settlement can accept the charge.
func TestChargeMercadoPagoPoint_CLPRemainingFloorsWithoutOvershoot(t *testing.T) {
	var createBody map[string]interface{}
	var createHits int
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/orders" && r.Method == http.MethodPost {
			createHits++
			raw, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &createBody))
			_, _ = w.Write([]byte(`{
				"id":"ORD01CLPFLOOR",
				"status":"created",
				"transactions":{"payments":[{"amount":"1500"}]}
			}`))
			return
		}
		t.Fatalf("unexpected path %s %s", r.Method, r.URL.Path)
	}))
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "CLP").Error)

	// Remaining 150050 cents → floor to CLP 1500 (150000 cents), not 1501.
	bill := createPointTestBill(t, business.ID, 150050, 0)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/charge", bytes.NewBufferString(`{"terminal_id":"NEWLAND_N950__T1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).ChargeMercadoPagoPoint(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Equal(t, 1, createHits)
	tx := createBody["transactions"].(map[string]interface{})
	payments := tx["payments"].([]interface{})
	p0 := payments[0].(map[string]interface{})
	assert.Equal(t, "1500", p0["amount"])

	var tracker database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ? AND participant_addr = ?", bill.ID, "ORD01CLPFLOOR").First(&tracker).Error)
	assert.Equal(t, int64(150000), tracker.Amount)
	assert.LessOrEqual(t, tracker.Amount, int64(150050))
}

// R2: sub-half-unit CLP remainder must 4xx without creating an MP order.
func TestChargeMercadoPagoPoint_CLPSubUnitRejectsWithoutOrder(t *testing.T) {
	createHits := 0
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		createHits++
		t.Fatalf("must not call MP API for sub-unit amount: %s", r.URL.Path)
	}))
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "CLP").Error)

	bill := createPointTestBill(t, business.ID, 49, 0) // < 1 CLP

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/charge", bytes.NewBufferString(`{"terminal_id":"NEWLAND_N950__T1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).ChargeMercadoPagoPoint(c)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, 0, createHits)
}

func TestChargeMercadoPagoPoint_CreatesOrderAndTracker(t *testing.T) {
	var createBody map[string]interface{}
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/orders", r.URL.Path)
		require.NotEmpty(t, r.Header.Get("X-Idempotency-Key"))
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &createBody))
		_, _ = w.Write([]byte(`{
			"id":"ORD01POINTCHARGE",
			"status":"created",
			"external_reference":"ignored-by-handler",
			"transactions":{"payments":[{"amount":"1500.00"}]}
		}`))
	}))

	bill := createPointTestBill(t, business.ID, 150000, 0) // ARS 1500.00 outstanding

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
	}
	payload := `{"terminal_id":"NEWLAND_N950__T1"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/charge", bytes.NewBufferString(payload))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).ChargeMercadoPagoPoint(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ORD01POINTCHARGE", resp["order_id"])
	assert.Equal(t, "pending", resp["status"])

	// MP order body contract
	assert.Equal(t, "point", createBody["type"])
	assert.Equal(t, fmt.Sprintf("bill_%d_business_%d", bill.ID, business.ID), createBody["external_reference"])
	assert.Equal(t, "PT30M", createBody["expiration_time"])
	assert.Equal(t, fmt.Sprintf("Payverge bill #%d", bill.ID), createBody["description"])
	tx := createBody["transactions"].(map[string]interface{})
	payments := tx["payments"].([]interface{})
	assert.Equal(t, "1500.00", payments[0].(map[string]interface{})["amount"])
	cfg := createBody["config"].(map[string]interface{})
	point := cfg["point"].(map[string]interface{})
	assert.Equal(t, "NEWLAND_N950__T1", point["terminal_id"])

	// Pending AlternativePayment tracker
	var tracker database.AlternativePayment
	require.NoError(t, database.GetDB().
		Where("bill_id = ? AND participant_addr = ?", bill.ID, "ORD01POINTCHARGE").
		First(&tracker).Error)
	assert.Equal(t, database.AlternativePaymentMethod("mercadopago"), tracker.PaymentMethod)
	assert.Equal(t, database.AltPaymentStatusPending, tracker.Status)
	assert.Equal(t, int64(150000), tracker.Amount)
	assert.Equal(t, "mercadopago", tracker.ParticipantName)
	assert.NotEmpty(t, tracker.IdempotencyKey)
}

func TestChargeMercadoPagoPoint_PendingChargeReturns409(t *testing.T) {
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("MP API must not be called when a pending charge exists: %s %s", r.Method, r.URL.Path)
	}))

	bill := createPointTestBill(t, business.ID, 5000, 0)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORD_EXISTING",
		ParticipantName: "mercadopago",
		Amount:          5000,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
		IdempotencyKey:  "plugin:mercadopago:ORD_EXISTING",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/charge", strings.NewReader(`{"terminal_id":"T1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).ChargeMercadoPagoPoint(c)

	require.Equal(t, http.StatusConflict, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ORD_EXISTING", resp["order_id"])
}

// F8: abandoned guest Checkout Pro trackers (mp_tracker_*) must not 409 staff Point charges.
func TestChargeMercadoPagoPoint_GuestTrackerDoesNotBlock(t *testing.T) {
	createCalls := 0
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/orders" {
			createCalls++
			_, _ = w.Write([]byte(`{"id":"ORD01AFTERGUEST","status":"created"}`))
			return
		}
		t.Fatalf("unexpected MP call: %s %s", r.Method, r.URL.Path)
	}))

	bill := createPointTestBill(t, business.ID, 5000, 0)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "mp_tracker_abandoned_guest_checkout",
		ParticipantName: "mercadopago",
		Amount:          5000,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
		IdempotencyKey:  "plugin:mercadopago:mp_tracker_abandoned",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/charge", strings.NewReader(`{"terminal_id":"T1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).ChargeMercadoPagoPoint(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Equal(t, 1, createCalls)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ORD01AFTERGUEST", resp["order_id"])
	// Must not surface the guest placeholder as order_id.
	assert.NotContains(t, w.Body.String(), "mp_tracker_")
}

func TestIsMercadoPagoOrdersAPITracker(t *testing.T) {
	assert.True(t, isMercadoPagoOrdersAPITracker("ORD01JQ4S4KY8HWQ6NA5PXB65B3D3"))
	assert.True(t, isMercadoPagoOrdersAPITracker("ord_lower"))
	assert.False(t, isMercadoPagoOrdersAPITracker("mp_tracker_abc"))
	assert.False(t, isMercadoPagoOrdersAPITracker(""))
	assert.False(t, isMercadoPagoOrdersAPITracker("pref_123"))
}

func TestFindPendingMercadoPagoTracker_IgnoresGuestPlaceholder(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}))

	bill := createPointTestBill(t, 1, 1000, 0)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "mp_tracker_x", ParticipantName: "mercadopago",
		Amount: 1000, PaymentMethod: "mercadopago", Status: database.AltPaymentStatusPending,
		IdempotencyKey: "k1",
	}).Error)

	_, found, err := findPendingMercadoPagoTracker(bill.ID)
	require.NoError(t, err)
	require.False(t, found)

	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "ORD01PENDING", ParticipantName: "mercadopago",
		Amount: 1000, PaymentMethod: "mercadopago", Status: database.AltPaymentStatusPending,
		IdempotencyKey: "k2",
	}).Error)
	row, found, err := findPendingMercadoPagoTracker(bill.ID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "ORD01PENDING", row.ParticipantAddr)
}

func TestChargeMercadoPagoPoint_WrongBusinessBill404(t *testing.T) {
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected MP call")
	}))
	other := createTestBusiness(t)
	bill := createPointTestBill(t, other.ID, 1000, 0)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "bill_id", Value: fmt.Sprintf("%d", bill.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/charge", strings.NewReader(`{"terminal_id":"T1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).ChargeMercadoPagoPoint(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetMercadoPagoOrder_MapsStatus(t *testing.T) {
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/orders/ORDSTATUS", r.URL.Path)
		_, _ = w.Write([]byte(`{"id":"ORDSTATUS","status":"processed","external_reference":"bill_1_business_1"}`))
	}))

	bill := createPointTestBill(t, business.ID, 1000, 0)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORDSTATUS",
		ParticipantName: "mercadopago",
		Amount:          1000,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
		IdempotencyKey:  "plugin:mercadopago:ORDSTATUS",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "order_id", Value: "ORDSTATUS"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/orders/ORDSTATUS", nil)

	NewPluginHandlers(nil, nil).GetMercadoPagoOrder(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ORDSTATUS", resp["order_id"])
	assert.Equal(t, "completed", resp["status"])
}

func TestCancelMercadoPagoOrder_CancelsAndMarksTracker(t *testing.T) {
	var cancelPath string
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancelPath = r.URL.Path
		require.Equal(t, http.MethodPost, r.Method)
		_, _ = w.Write([]byte(`{"id":"ORDCANCEL","status":"canceled"}`))
	}))

	bill := createPointTestBill(t, business.ID, 2500, 0)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORDCANCEL",
		ParticipantName: "mercadopago",
		Amount:          2500,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
		IdempotencyKey:  "plugin:mercadopago:ORDCANCEL",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "order_id", Value: "ORDCANCEL"},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/orders/ORDCANCEL/cancel", nil)

	NewPluginHandlers(nil, nil).CancelMercadoPagoOrder(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "/v1/orders/ORDCANCEL/cancel", cancelPath)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "cancelled", resp["status"])

	var tracker database.AlternativePayment
	require.NoError(t, database.GetDB().Where("participant_addr = ?", "ORDCANCEL").First(&tracker).Error)
	assert.Equal(t, database.AltPaymentStatusCancelled, tracker.Status)
}

// R4: when MP reports the order already cancelled, still mark local tracker cancelled.
func TestCancelMercadoPagoOrder_AlreadyCancelledStillMarksTracker(t *testing.T) {
	business, _, _ := setupMercadoPagoPointHandlerTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel"):
			// Simulate MP "already cancelled" failure.
			http.Error(w, `{"message":"order already canceled"}`, http.StatusConflict)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/orders/"):
			_, _ = w.Write([]byte(`{"id":"ORDALREADY","status":"canceled"}`))
		default:
			t.Fatalf("unexpected path %s %s", r.Method, r.URL.Path)
		}
	}))

	bill := createPointTestBill(t, business.ID, 2500, 0)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORDALREADY",
		ParticipantName: "mercadopago",
		Amount:          2500,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
		IdempotencyKey:  "plugin:mercadopago:ORDALREADY",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "order_id", Value: "ORDALREADY"},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/orders/ORDALREADY/cancel", nil)

	NewPluginHandlers(nil, nil).CancelMercadoPagoOrder(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var tracker database.AlternativePayment
	require.NoError(t, database.GetDB().Where("participant_addr = ?", "ORDALREADY").First(&tracker).Error)
	assert.Equal(t, database.AltPaymentStatusCancelled, tracker.Status)
}
