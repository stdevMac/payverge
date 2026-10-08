package mercadopago

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestMercadoPagoDecimalAmount(t *testing.T) {
	require.Equal(t, "1500.00", mercadoPagoDecimalAmount(150000, "ARS"))
	require.Equal(t, "1500", mercadoPagoDecimalAmount(150000, "CLP"))
	require.Equal(t, "1500", mercadoPagoDecimalAmount(150000, "COP"))
	require.Equal(t, "12.34", mercadoPagoDecimalAmount(1234, "USD"))
}

func TestMercadoPagoOrderStatus(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"processed", "completed"},
		{"canceled", "cancelled"},
		{"cancelled", "cancelled"}, // tolerate US spelling if MP ever sends it
		{"failed", "cancelled"},
		{"refunded", "refunded"},
		{"expired", "expired"},
		{"created", "pending"},
		{"action_required", "pending"},
		{"PROCESSED", "completed"},
		{"unknown", "pending"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.want, mercadoPagoOrderStatus(tc.in))
		})
	}
}

func TestCreateOrderPointBodyHeadersAndAmount(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotAuth   string
		gotIdem   string
		gotBody   map[string]interface{}
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotIdem = r.Header.Get("X-Idempotency-Key")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		// Zero commission: marketplace / platform fee fields must never appear.
		require.NotContains(t, gotBody, "marketplace_fee")
		require.NotContains(t, gotBody, "application_fee")
		require.NotContains(t, gotBody, "platform_fee")
		_, _ = w.Write([]byte(`{
			"id":"ORD01TESTPOINT",
			"status":"created",
			"external_reference":"bill_99_business_42",
			"transactions":{"payments":[{"amount":"1500.00"}]}
		}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	amount := mercadoPagoDecimalAmount(150000, "ARS")
	require.Equal(t, "1500.00", amount)

	order, err := plugin.createOrder(42, mpOrderRequest{
		Type:              "point",
		ExternalReference: "bill_99_business_42",
		ExpirationTime:    "PT30M",
		Description:       "Payverge bill #99",
		Transactions: mpOrderTransactions{
			Payments: []mpOrderPayment{{Amount: amount}},
		},
		Config: mpOrderConfig{
			Point: &mpPointConfig{TerminalID: "NEWLAND_N950__N950NCB801293324"},
		},
	}, "idem-point-1")

	require.NoError(t, err)
	require.Equal(t, "ORD01TESTPOINT", order.ID)
	require.Equal(t, "created", order.Status)

	require.Equal(t, http.MethodPost, gotMethod)
	require.Equal(t, "/v1/orders", gotPath)
	require.Equal(t, "Bearer APP_USR-refund-access-token-1234567890", gotAuth)
	require.Equal(t, "idem-point-1", gotIdem)

	require.Equal(t, "point", gotBody["type"])
	require.Equal(t, "bill_99_business_42", gotBody["external_reference"])
	require.Equal(t, "PT30M", gotBody["expiration_time"])
	require.Equal(t, "Payverge bill #99", gotBody["description"])

	tx, ok := gotBody["transactions"].(map[string]interface{})
	require.True(t, ok)
	payments, ok := tx["payments"].([]interface{})
	require.True(t, ok)
	require.Len(t, payments, 1)
	p0, ok := payments[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "1500.00", p0["amount"])

	cfg, ok := gotBody["config"].(map[string]interface{})
	require.True(t, ok)
	point, ok := cfg["point"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "NEWLAND_N950__N950NCB801293324", point["terminal_id"])
}

func TestCreateOrderCLPAmountIsWholeNumber(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		_, _ = w.Write([]byte(`{"id":"ORDCLP","status":"created"}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	amount := mercadoPagoDecimalAmount(150000, "CLP")
	require.Equal(t, "1500", amount)

	_, err := plugin.createOrder(42, mpOrderRequest{
		Type:              "point",
		ExternalReference: "bill_1_business_42",
		Transactions: mpOrderTransactions{
			Payments: []mpOrderPayment{{Amount: amount}},
		},
		Config: mpOrderConfig{
			Point: &mpPointConfig{TerminalID: "TERM1"},
		},
	}, "idem-clp")
	require.NoError(t, err)

	tx := gotBody["transactions"].(map[string]interface{})
	payments := tx["payments"].([]interface{})
	p0 := payments[0].(map[string]interface{})
	require.Equal(t, "1500", p0["amount"])
}

func TestGetOrderParsesTypeResponseQRData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/orders/ORDQR123", r.URL.Path)
		require.Equal(t, "Bearer APP_USR-refund-access-token-1234567890", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{
			"id":"ORDQR123",
			"status":"created",
			"external_reference":"bill_7_business_42",
			"type_response":{"qr_data":"00020101021243650016com.mercadolibre0201305015204"},
			"transactions":{"payments":[{"amount":"250.50"}]}
		}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	order, err := plugin.getOrder(42, "ORDQR123")
	require.NoError(t, err)
	require.Equal(t, "ORDQR123", order.ID)
	require.Equal(t, "created", order.Status)
	require.Equal(t, "bill_7_business_42", order.ExternalReference)
	require.Equal(t, "00020101021243650016com.mercadolibre0201305015204", order.TypeResponse.QRData)
	require.Len(t, order.Transactions.Payments, 1)
	require.Equal(t, "250.50", order.Transactions.Payments[0].Amount)
}

func TestCreateOrderTypedErrors403And409(t *testing.T) {
	t.Run("409 point is ErrTerminalBusy", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"terminal is busy"}`))
		}))
		defer server.Close()

		setupMercadoPagoRefundTestDB(t, server.URL)
		plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

		_, err := plugin.createOrder(42, mpOrderRequest{
			Type:              "point",
			ExternalReference: "bill_1_business_42",
			Transactions: mpOrderTransactions{
				Payments: []mpOrderPayment{{Amount: "10.00"}},
			},
			Config: mpOrderConfig{
				Point: &mpPointConfig{TerminalID: "TERM-BUSY"},
			},
		}, "idem-409")
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrTerminalBusy), "got %v", err)
	})

	t.Run("403 is ErrForbidden", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"forbidden"}`))
		}))
		defer server.Close()

		setupMercadoPagoRefundTestDB(t, server.URL)
		plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

		_, err := plugin.createOrder(42, mpOrderRequest{
			Type:              "point",
			ExternalReference: "bill_1_business_42",
			Transactions: mpOrderTransactions{
				Payments: []mpOrderPayment{{Amount: "10.00"}},
			},
			Config: mpOrderConfig{
				Point: &mpPointConfig{TerminalID: "TERM1"},
			},
		}, "idem-403")
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrForbidden), "got %v", err)
	})
}

func TestListTerminalsAndSetTerminalMode(t *testing.T) {
	var setupBody map[string]interface{}
	var setupMethod, setupPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/terminals/v1/list":
			require.Equal(t, "Bearer APP_USR-refund-access-token-1234567890", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{
				"data":{
					"terminals":[{
						"id":"NEWLAND_N950__N950NCB801293324",
						"pos_id":"23545678",
						"store_id":"12354567",
						"external_pos_id":"SUC0101POS",
						"operating_mode":"STANDALONE"
					}]
				},
				"paging":{"total":1,"offset":0,"limit":50}
			}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/terminals/v1/setup":
			setupMethod = r.Method
			setupPath = r.URL.Path
			require.NotEmpty(t, r.Header.Get("X-Idempotency-Key"))
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &setupBody))
			_, _ = w.Write([]byte(`{"terminals":[{"id":"NEWLAND_N950__N950NCB801293324","operating_mode":"PDV"}]}`))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	terminals, err := plugin.listTerminals(42)
	require.NoError(t, err)
	require.Len(t, terminals, 1)
	require.Equal(t, "NEWLAND_N950__N950NCB801293324", terminals[0].ID)
	require.Equal(t, "STANDALONE", terminals[0].OperatingMode)
	require.Equal(t, "SUC0101POS", terminals[0].ExternalPosID)

	require.NoError(t, plugin.setTerminalMode(42, "NEWLAND_N950__N950NCB801293324", "PDV"))
	require.Equal(t, http.MethodPatch, setupMethod)
	require.Equal(t, "/terminals/v1/setup", setupPath)
	terms, ok := setupBody["terminals"].([]interface{})
	require.True(t, ok)
	require.Len(t, terms, 1)
	t0 := terms[0].(map[string]interface{})
	require.Equal(t, "NEWLAND_N950__N950NCB801293324", t0["id"])
	require.Equal(t, "PDV", t0["operating_mode"])
}

func TestCancelOrder(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		require.Equal(t, "Bearer APP_USR-refund-access-token-1234567890", r.Header.Get("Authorization"))
		require.NotEmpty(t, r.Header.Get("X-Idempotency-Key"))
		_, _ = w.Write([]byte(`{"id":"ORDCANCEL","status":"canceled"}`))
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	require.NoError(t, plugin.cancelOrder(42, "ORDCANCEL"))
	require.Equal(t, http.MethodPost, gotMethod)
	require.Equal(t, "/v1/orders/ORDCANCEL/cancel", gotPath)
}
