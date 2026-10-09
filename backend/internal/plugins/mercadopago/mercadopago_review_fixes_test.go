package mercadopago

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// TestRefundPaymentRejectsRejectedStatus locks M2: a MercadoPago refund reported
// as rejected must surface an error, not be recorded as complete.
func TestRefundPaymentRejectsRejectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/payments/123456" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id":123456,"currency_id":"ARS","transaction_amount":50.00}`))
		case r.URL.Path == "/v1/payments/123456/refunds" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id":98765,"status":"rejected"}`))
		default:
			t.Fatalf("unexpected MercadoPago API path: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	err := plugin.RefundPayment(42, "123456", 1234)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "rejected"), "error should name the rejected status: %v", err)
}

// TestCreateBillPaymentZeroDecimalCurrency locks M3 (outbound, preference): a
// zero-decimal currency (CLP) amount is sent as a whole number, even when the
// stored cents (major×100) carry a sub-unit remainder from a split/rounding.
func TestCreateBillPaymentZeroDecimalCurrency(t *testing.T) {
	var preferenceBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/checkout/preferences", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &preferenceBody))
		_, _ = w.Write([]byte(`{"id":"PREF-CLP","sandbox_init_point":"https://mercadopago.test/checkout"}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	// CLP 333.33 is impossible; a split can still store 33333 cents. It must be
	// sent as whole CLP 333, never 333.33 (which MercadoPago rejects for CLP).
	_, err := plugin.CreateBillPayment(42, 99, 33333, "CLP", map[string]interface{}{
		"return_url": "https://payverge.test/return",
		"cancel_url": "https://payverge.test/cancel",
	})
	require.NoError(t, err)

	items, ok := preferenceBody["items"].([]interface{})
	require.True(t, ok)
	require.Len(t, items, 1)
	item := items[0].(map[string]interface{})
	require.EqualValues(t, 333, item["unit_price"])
	require.NotContains(t, string(mustJSON(t, item)), "333.33")
}

// TestRefundPaymentZeroDecimalRoundsWhole locks M3 (outbound, refund): a refund
// in a zero-decimal currency is rounded to a whole major unit.
func TestRefundPaymentZeroDecimalRoundsWhole(t *testing.T) {
	var refundBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/payments/555" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id":555,"currency_id":"CLP","transaction_amount":1000}`))
		case r.URL.Path == "/v1/payments/555/refunds" && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			refundBody = string(body)
			_, _ = w.Write([]byte(`{"id":777,"status":"approved"}`))
		default:
			t.Fatalf("unexpected MercadoPago API path: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	// 33333 cents in CLP -> whole CLP 333.
	err := plugin.RefundPayment(42, "555", 33333)
	require.NoError(t, err)
	require.Contains(t, refundBody, `"amount":333`)
	require.NotContains(t, refundBody, "333.33")
}

// F4: partial COP refund must not send 2-decimal sub-units (MP rejects them).
func TestRefundPayment_COPPartialIsWholeUnit(t *testing.T) {
	var refundBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/payments/666" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id":666,"currency_id":"COP","transaction_amount":10000}`))
		case r.URL.Path == "/v1/payments/666/refunds" && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			refundBody = string(body)
			_, _ = w.Write([]byte(`{"id":888,"status":"approved"}`))
		default:
			t.Fatalf("unexpected MercadoPago API path: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	// 12345 cents → whole COP 123 (not 123.45).
	err := plugin.RefundPayment(42, "666", 12345)
	require.NoError(t, err)
	require.Contains(t, refundBody, `"amount":123`)
	require.NotContains(t, refundBody, "123.45")
}

// TestGetPaymentStatusUsesConfiguredHost locks M4: GetPaymentStatus (payment-info
// lookup) respects the configured api_base_url instead of the hardcoded prod host.
func TestGetPaymentStatusUsesConfiguredHost(t *testing.T) {
	var hit bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/payments/123456", r.URL.Path)
		hit = true
		_, _ = w.Write([]byte(`{"id":123456,"status":"approved"}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	status, err := plugin.GetPaymentStatus(42, "123456")
	require.NoError(t, err)
	require.True(t, hit, "payment-info lookup must hit the configured host")
	require.Equal(t, "approved", status)
}

func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
