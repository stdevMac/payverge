package paypal

import (
	"encoding/base64"
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

func TestRefundPaymentRefundsCaptureWithPartialAmount(t *testing.T) {
	var requests []paypalRefundHTTPRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		requests = append(requests, paypalRefundHTTPRequest{
			Method:    r.Method,
			Path:      r.URL.Path,
			Body:      string(body),
			Auth:      r.Header.Get("Authorization"),
			RequestID: r.Header.Get("PayPal-Request-Id"),
		})

		switch r.URL.Path {
		case "/v1/oauth2/token":
			require.Equal(t, http.MethodPost, r.Method)
			auth := r.Header.Get("Authorization")
			require.True(t, strings.HasPrefix(auth, "Basic "))
			decoded, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
			require.Contains(t, string(decoded), "client-id-12345678901234567890:client-secret-12345678901234567890")
			_, _ = w.Write([]byte(`{"access_token":"access-token"}`))
		case "/v2/payments/captures/CAPTURE-SPLIT-1":
			require.Equal(t, http.MethodGet, r.Method)
			_, _ = w.Write([]byte(`{"id":"CAPTURE-SPLIT-1","amount":{"value":"28.00","currency_code":"USD"}}`))
		case "/v2/payments/captures/CAPTURE-SPLIT-1/refund":
			require.Equal(t, http.MethodPost, r.Method)
			require.Contains(t, string(body), `"value":"12.34"`)
			require.Contains(t, string(body), `"currency_code":"USD"`)
			_, _ = w.Write([]byte(`{"id":"REFUND-SPLIT-1","status":"COMPLETED"}`))
		default:
			t.Fatalf("unexpected PayPal API path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	setupPayPalRefundTestDB(t, server.URL)
	plugin := NewPayPalPlugin(services.NewPluginService(database.GetDBWrapper()))

	err := plugin.RefundPayment(42, "CAPTURE-SPLIT-1", 1234)
	require.NoError(t, err)
	require.Len(t, requests, 4)
	require.Equal(t, "Bearer access-token", requests[1].Auth)
	require.Equal(t, "Bearer access-token", requests[3].Auth)
	require.NotEmpty(t, requests[3].RequestID)
}

func TestRefundPaymentRefundsFullCaptureWithoutAmount(t *testing.T) {
	var refundBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"access-token"}`))
		case "/v2/payments/captures/CAPTURE-FULL-1/refund":
			require.Equal(t, http.MethodPost, r.Method)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			refundBody = string(body)
			_, _ = w.Write([]byte(`{"id":"REFUND-FULL-1","status":"COMPLETED"}`))
		default:
			t.Fatalf("unexpected PayPal API path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	setupPayPalRefundTestDB(t, server.URL)
	plugin := NewPayPalPlugin(services.NewPluginService(database.GetDBWrapper()))

	err := plugin.RefundPayment(42, "CAPTURE-FULL-1", 0)
	require.NoError(t, err)
	require.NotContains(t, refundBody, "amount")
}

type paypalRefundHTTPRequest struct {
	Method    string
	Path      string
	Body      string
	Auth      string
	RequestID string
}

func setupPayPalRefundTestDB(t *testing.T, apiBaseURL string) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Plugin{},
		&database.BusinessPlugin{},
	))

	business := database.Business{ID: 42, BusinessId: "paypal-refund-business", Name: "PayPal Refund Test"}
	require.NoError(t, gormDB.Create(&business).Error)
	plugin := database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, gormDB.Create(&plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"client_id":     "client-id-12345678901234567890",
		"client_secret": "client-secret-12345678901234567890",
		"environment":   "sandbox",
		"api_base_url":  apiBaseURL,
	}))
}
