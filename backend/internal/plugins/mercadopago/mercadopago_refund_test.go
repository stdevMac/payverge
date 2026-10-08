package mercadopago

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestCreateBillPaymentUsesPayvergeTrackerMetadata(t *testing.T) {
	var preferenceBody map[string]interface{}
	var idempotencyKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/checkout/preferences", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		idempotencyKey = r.Header.Get("X-Idempotency-Key")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &preferenceBody))
		_, _ = w.Write([]byte(`{"id":"PREF-123","sandbox_init_point":"https://mercadopago.test/checkout"}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	response, err := plugin.CreateBillPayment(42, 99, 1234, "USD", map[string]interface{}{
		"return_url":     "https://payverge.test/return",
		"cancel_url":     "https://payverge.test/cancel",
		"split_share_id": float64(7),
	})

	require.NoError(t, err)
	require.NotEmpty(t, response.PaymentID)
	require.NotEqual(t, "PREF-123", response.PaymentID)
	require.Contains(t, response.PaymentID, "mp_tracker_")
	require.Equal(t, "PREF-123", response.Metadata["preference_id"])
	require.NotEmpty(t, idempotencyKey)

	metadata, ok := preferenceBody["metadata"].(map[string]interface{})
	require.True(t, ok, "MercadoPago preference must carry Payverge metadata")
	require.Equal(t, response.PaymentID, metadata["payverge_tracker_id"])
	require.Equal(t, float64(7), metadata["split_share_id"])
}

func TestHandleWebhookCarriesPayvergeTrackerMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/payments/987654", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		_, _ = w.Write([]byte(`{
			"id":987654,
			"status":"approved",
			"currency_id":"USD",
			"transaction_amount":12.34,
			"external_reference":"bill_99_business_42",
			"metadata":{
				"payverge_tracker_id":"mp_tracker_abc",
				"split_share_id":7
			}
		}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	response, err := plugin.HandleWebhook(42, []byte(`{"action":"payment.updated","data":{"id":"987654"}}`), nil)

	require.NoError(t, err)
	require.True(t, response.Success)
	require.Equal(t, "987654", response.PaymentID)
	require.Equal(t, "mp_tracker_abc", response.Metadata["payverge_tracker_id"])
	require.Equal(t, float64(7), response.Metadata["split_share_id"])
}

func TestRefundPaymentCreatesMercadoPagoPartialRefund(t *testing.T) {
	var refundBody string
	var refundAuth string
	var refundIdempotencyKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/payments/123456" && r.Method == http.MethodGet:
			// RefundPayment resolves the payment currency before rounding the
			// refund amount to the currency's decimals.
			_, _ = w.Write([]byte(`{"id":123456,"currency_id":"USD","transaction_amount":28.00}`))
		case r.URL.Path == "/v1/payments/123456/refunds" && r.Method == http.MethodPost:
			refundAuth = r.Header.Get("Authorization")
			refundIdempotencyKey = r.Header.Get("X-Idempotency-Key")
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			refundBody = string(body)
			_, _ = w.Write([]byte(`{"id":98765,"payment_id":123456,"status":"approved"}`))
		default:
			t.Fatalf("unexpected MercadoPago API path: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	err := plugin.RefundPayment(42, "123456", 1234)
	require.NoError(t, err)
	require.Equal(t, "Bearer APP_USR-refund-access-token-1234567890", refundAuth)
	require.NotEmpty(t, refundIdempotencyKey)
	require.Contains(t, refundBody, `"amount":12.34`)
}

func TestRefundPaymentCreatesMercadoPagoFullRefundWithoutAmount(t *testing.T) {
	var refundBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/payments/123456/refunds", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		refundBody = string(body)
		_, _ = w.Write([]byte(`{"id":98765,"payment_id":123456}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	err := plugin.RefundPayment(42, "123456", 0)
	require.NoError(t, err)
	require.False(t, strings.Contains(refundBody, "amount"))
}

// R2: ORD… ids must refund via /v1/orders/{id}/refund, never payments-refund
// (auto-refund of unsettleable Point/QR captures passes the order id).
func TestRefundPayment_OrderIDUsesOrdersRefundEndpoint(t *testing.T) {
	var refundPath string
	var paymentsRefundHit bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/payments/") && strings.HasSuffix(r.URL.Path, "/refunds") {
			paymentsRefundHit = true
			t.Fatalf("ORD id must not hit payments-refund path: %s", r.URL.Path)
		}
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/orders/") && strings.HasSuffix(r.URL.Path, "/refund") {
			refundPath = r.URL.Path
			_, _ = w.Write([]byte(`{"id":"ORD01TEST","status":"refunded"}`))
			return
		}
		t.Fatalf("unexpected MercadoPago API path: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	err := plugin.RefundPayment(42, "ORD01TESTORDERID123", 0)
	require.NoError(t, err)
	require.Equal(t, "/v1/orders/ORD01TESTORDERID123/refund", refundPath)
	require.False(t, paymentsRefundHit)
}

func setupMercadoPagoRefundTestDB(t *testing.T, apiBaseURL string) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Plugin{},
		&database.BusinessPlugin{},
	))

	business := database.Business{ID: 42, BusinessId: "mp-refund-business", Name: "MercadoPago Refund Test"}
	require.NoError(t, gormDB.Create(&business).Error)
	plugin := database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, gormDB.Create(&plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"access_token":   "APP_USR-refund-access-token-1234567890",
		"public_key":     "APP_USR-refund-public-key-1234567890",
		"environment":    "sandbox",
		"base_url":       "https://api.staging.example",
		"api_base_url":   apiBaseURL,
		"webhook_secret": "mp-webhook-secret",
	}))
}

func TestCreateBillPaymentHonorsInstallmentsAndAutoReturn(t *testing.T) {
	var preferenceBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/checkout/preferences", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &preferenceBody))
		_, _ = w.Write([]byte(`{"id":"PREF-456","sandbox_init_point":"https://mercadopago.test/checkout"}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDBWithConfig(t, map[string]interface{}{
		"access_token":   "APP_USR-refund-access-token-1234567890",
		"public_key":     "APP_USR-refund-public-key-1234567890",
		"environment":    "sandbox",
		"base_url":       "https://api.staging.example",
		"api_base_url":   server.URL,
		"webhook_secret": "mp-webhook-secret",
		"auto_return":    "all",
		"installments":   "6",
	})
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	_, err := plugin.CreateBillPayment(42, 99, 1234, "USD", map[string]interface{}{
		"return_url": "https://payverge.test/return",
		"cancel_url": "https://payverge.test/cancel",
	})
	require.NoError(t, err)
	require.Equal(t, "all", preferenceBody["auto_return"])
	pm, ok := preferenceBody["payment_methods"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, float64(6), pm["installments"])
}

// R2: guest Checkout Pro for COP must put the same canonical amount_cents in
// response metadata that the preference unit_price charges (whole major units).
func TestCreateBillPayment_COPMetadataMatchesPreference(t *testing.T) {
	var preferenceBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/checkout/preferences", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &preferenceBody))
		_, _ = w.Write([]byte(`{"id":"PREF-COP","sandbox_init_point":"https://mercadopago.test/checkout"}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	// Whole COP 1500.00 stored as 150000 cents — representable and unchanged.
	resp, err := plugin.CreateBillPayment(42, 7, 150000, "COP", map[string]interface{}{
		"return_url": "https://payverge.test/return",
		"cancel_url": "https://payverge.test/cancel",
	})
	require.NoError(t, err)
	require.Equal(t, int64(150000), resp.Metadata["amount_cents"])

	items := preferenceBody["items"].([]interface{})
	item0 := items[0].(map[string]interface{})
	require.Equal(t, float64(1500), item0["unit_price"])
	require.Equal(t, "COP", item0["currency_id"])
}

func TestGetConfigSchemaHasNoWebhookURL(t *testing.T) {
	p := NewMercadoPagoPlugin(nil)
	schema := p.GetConfigSchema()
	require.NotContains(t, schema, "webhook_url")
	require.Contains(t, schema, "installments")
	require.Contains(t, schema, "auto_return")
}

func setupMercadoPagoRefundTestDBWithConfig(t *testing.T, cfg map[string]interface{}) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Plugin{},
		&database.BusinessPlugin{},
	))

	business := database.Business{ID: 42, BusinessId: "mp-refund-business", Name: "MercadoPago Refund Test"}
	require.NoError(t, gormDB.Create(&business).Error)
	plugin := database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, gormDB.Create(&plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, cfg))
}

func TestCapturePaymentReturnApproved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/payments/987654", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		_, _ = w.Write([]byte(`{
			"id":987654,
			"status":"approved",
			"currency_id":"USD",
			"transaction_amount":12.34,
			"external_reference":"bill_99_business_42",
			"metadata":{"payverge_tracker_id":"mp_tracker_abc"}
		}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	response, err := plugin.CapturePaymentReturn(42, 99, "987654")
	require.NoError(t, err)
	require.True(t, response.Success)
	require.Equal(t, "completed", response.Status)
	require.Equal(t, uint(99), response.BillID)
	require.Equal(t, "987654", response.PaymentID)
	require.EqualValues(t, 1234, response.Amount)
}

func TestCapturePaymentReturnRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"id":111,
			"status":"rejected",
			"currency_id":"USD",
			"transaction_amount":10.00,
			"external_reference":"bill_99_business_42"
		}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	response, err := plugin.CapturePaymentReturn(42, 99, "111")
	require.NoError(t, err)
	require.False(t, response.Success)
}

func TestGetPaymentStatusOrderIDUsesOrdersAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/orders/ORD123", r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		_, _ = w.Write([]byte(`{"id":"ORD123","status":"expired","external_reference":"bill_1_business_42","transactions":{"payments":[{"amount":"10.00"}]}}`))
	}))
	defer server.Close()
	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))
	status, err := plugin.GetPaymentStatus(42, "ORD123")
	require.NoError(t, err)
	require.Equal(t, "expired", status)
}
