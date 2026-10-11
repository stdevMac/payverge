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

// P1: partial ORD refund body must match the verified MP contract:
//
//	{"transactions":[{"id":"<PAY…>","amount":"<decimal string>"}]}
//
// Full refund = empty body (omit transactions). Currency formatting comes from
// the fetched order (never hard-coded "ARS"). Multi-payment orders reject.

func TestBuildPartialOrderRefundBody_COPWholeUnitWithPayID(t *testing.T) {
	order := &mpOrder{
		ID:         "ORD01COP",
		CurrencyID: "COP",
		Transactions: mpOrderTransactions{
			Payments: []mpOrderPayment{
				{ID: "PAY01COPTEST123", Amount: "150000"},
			},
		},
	}
	// 1234500 platform cents → 12345 whole COP major units after floor.
	body, err := buildPartialOrderRefundBody(order, 1234500)
	require.NoError(t, err)
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &parsed))
	txs, ok := parsed["transactions"].([]interface{})
	require.True(t, ok, "transactions must be a top-level array, not nested under payments")
	require.Len(t, txs, 1)
	tx0 := txs[0].(map[string]interface{})
	require.Equal(t, "PAY01COPTEST123", tx0["id"])
	require.Equal(t, "12345", tx0["amount"])
	// Must not use the old wrong shape.
	require.NotContains(t, string(raw), `"payments"`)
}

func TestBuildPartialOrderRefundBody_ARSTwoDecimal(t *testing.T) {
	order := &mpOrder{
		ID:         "ORD01ARS",
		CurrencyID: "ARS",
		Transactions: mpOrderTransactions{
			Payments: []mpOrderPayment{
				{ID: "PAY01ARSTEST456", Amount: "100.00"},
			},
		},
	}
	// 2450 cents → "24.50"
	body, err := buildPartialOrderRefundBody(order, 2450)
	require.NoError(t, err)
	raw, _ := json.Marshal(body)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &parsed))
	tx0 := parsed["transactions"].([]interface{})[0].(map[string]interface{})
	require.Equal(t, "PAY01ARSTEST456", tx0["id"])
	require.Equal(t, "24.50", tx0["amount"])
}

func TestBuildPartialOrderRefundBody_MultiPaymentRejects(t *testing.T) {
	order := &mpOrder{
		ID:         "ORD01MULTI",
		CurrencyID: "ARS",
		Transactions: mpOrderTransactions{
			Payments: []mpOrderPayment{
				{ID: "PAY01A", Amount: "50.00"},
				{ID: "PAY01B", Amount: "50.00"},
			},
		},
	}
	_, err := buildPartialOrderRefundBody(order, 2500)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "multi")
}

func TestBuildPartialOrderRefundBody_MissingCurrencyErrors(t *testing.T) {
	order := &mpOrder{
		ID: "ORD01NOCUR",
		Transactions: mpOrderTransactions{
			Payments: []mpOrderPayment{{ID: "PAY01X", Amount: "10.00"}},
		},
	}
	_, err := buildPartialOrderRefundBody(order, 500)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "currency")
}

func TestBuildPartialOrderRefundBody_MissingPaymentIDErrors(t *testing.T) {
	order := &mpOrder{
		ID:         "ORD01NOPAYID",
		CurrencyID: "ARS",
		Transactions: mpOrderTransactions{
			Payments: []mpOrderPayment{{Amount: "10.00"}},
		},
	}
	_, err := buildPartialOrderRefundBody(order, 500)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "id")
}

func TestRefundPayment_OrderPartial_COPBodyWithPayID(t *testing.T) {
	var refundBody string
	var gotGET bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/orders/ORD01PARTIALCOP":
			gotGET = true
			_, _ = w.Write([]byte(`{
				"id":"ORD01PARTIALCOP",
				"status":"processed",
				"currency_id":"COP",
				"transactions":{"payments":[{"id":"PAY01COPXXX","amount":"150000"}]}
			}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/orders/ORD01PARTIALCOP/refund":
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			refundBody = string(body)
			require.NotEmpty(t, r.Header.Get("X-Idempotency-Key"))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"ORD01PARTIALCOP","status":"processed","status_detail":"partially_refunded"}`))
		default:
			t.Fatalf("unexpected path: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	// 500000 cents = 5000 whole COP
	err := plugin.RefundPayment(42, "ORD01PARTIALCOP", 500000)
	require.NoError(t, err)
	require.True(t, gotGET, "partial refund must fetch the order first")
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(refundBody), &parsed))
	txs := parsed["transactions"].([]interface{})
	require.Len(t, txs, 1)
	tx0 := txs[0].(map[string]interface{})
	require.Equal(t, "PAY01COPXXX", tx0["id"])
	require.Equal(t, "5000", tx0["amount"])
}

func TestRefundPayment_OrderPartial_ARSBody(t *testing.T) {
	var refundBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/orders/"):
			_, _ = w.Write([]byte(`{
				"id":"ORD01PARTIALARS",
				"status":"processed",
				"currency_id":"ARS",
				"transactions":{"payments":[{"id":"PAY01ARSXXX","amount":"100.00"}]}
			}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/refund"):
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			refundBody = string(body)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"ORD01PARTIALARS","status":"processed"}`))
		default:
			t.Fatalf("unexpected path: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	err := plugin.RefundPayment(42, "ORD01PARTIALARS", 2450)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(refundBody), &parsed))
	tx0 := parsed["transactions"].([]interface{})[0].(map[string]interface{})
	require.Equal(t, "PAY01ARSXXX", tx0["id"])
	require.Equal(t, "24.50", tx0["amount"])
}

func TestRefundPayment_OrderFull_EmptyBody(t *testing.T) {
	var refundBody string
	var gotGET bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gotGET = true
			// Full refund should not need a GET; if it does, still respond.
			_, _ = w.Write([]byte(`{"id":"ORD01FULL","status":"processed","currency_id":"ARS","transactions":{"payments":[{"id":"PAYX","amount":"10.00"}]}}`))
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/refund") {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			refundBody = string(body)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"ORD01FULL","status":"refunded"}`))
			return
		}
		t.Fatalf("unexpected path: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	err := plugin.RefundPayment(42, "ORD01FULL", 0)
	require.NoError(t, err)
	// Empty body: nil payload → no request body bytes (or empty/null).
	require.True(t, refundBody == "" || refundBody == "null" || refundBody == "{}",
		"full refund body must omit transactions; got %q", refundBody)
	require.NotContains(t, refundBody, "transactions")
	_ = gotGET // full refund may skip GET; either is fine
}

func TestRefundPayment_OrderPartial_FetchFailureNoGuess(t *testing.T) {
	var refundPosted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/refund") {
			refundPosted = true
			t.Fatalf("must not POST refund when order fetch fails")
		}
		t.Fatalf("unexpected path: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	err := plugin.RefundPayment(42, "ORD01FETCHFAIL", 1000)
	require.Error(t, err)
	require.False(t, refundPosted)
	// Must not silently fall back to ARS-formatted body.
	require.NotContains(t, err.Error(), `"amount":"10.00"`)
}

func TestRefundPayment_OrderPartial_MultiPaymentCleanError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{
				"id":"ORD01MULTI",
				"status":"processed",
				"currency_id":"ARS",
				"transactions":{"payments":[
					{"id":"PAYA","amount":"50.00"},
					{"id":"PAYB","amount":"50.00"}
				]}
			}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/refund") {
			t.Fatalf("must not refund multi-payment partial without policy")
		}
		t.Fatalf("unexpected: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	err := plugin.RefundPayment(42, "ORD01MULTI", 2500)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "multi")
}

// A refund the Orders API answers with a non-success status (or no status at
// all) must surface as an error so the payment stays refund_pending.
func TestRefundOrder_NonSuccessStatusIsError(t *testing.T) {
	for _, body := range []string{
		`{"id":"ORD01REJ","status":"rejected"}`,
		`{"id":"ORD01REJ","status":"failed"}`,
		`{"id":"ORD01REJ","status":"action_required"}`,
		`{"id":"ORD01REJ"}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/refund") {
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(body))
					return
				}
				t.Fatalf("unexpected: %s %s", r.Method, r.URL.Path)
			}))
			defer server.Close()

			setupMercadoPagoRefundTestDB(t, server.URL)
			plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

			require.Error(t, plugin.RefundPayment(42, "ORD01REJ", 0))
		})
	}
}
