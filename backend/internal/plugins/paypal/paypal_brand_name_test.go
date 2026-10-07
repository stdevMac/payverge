package paypal

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// The merchant label PayPal shows the payer at checkout is this instance's
// PRODUCT_NAME (default Payverge), never a hard-coded upstream brand.
func TestCreateBillPaymentBrandNameFollowsProductName(t *testing.T) {
	originalCallbackBase := callbackBaseURL
	callbackBaseURL = func() string { return "https://pos.example.org" }
	t.Cleanup(func() { callbackBaseURL = originalCallbackBase })

	for _, tc := range []struct{ productName, want string }{
		{"", "Payverge"},
		{"Acme POS", "Acme POS"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Setenv("PRODUCT_NAME", tc.productName)
			var order map[string]interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/oauth2/token":
					_, _ = w.Write([]byte(`{"access_token":"access-token"}`))
				case r.URL.Path == "/v2/checkout/orders" && r.Method == http.MethodPost:
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.NoError(t, json.Unmarshal(body, &order))
					_, _ = w.Write([]byte(`{"id":"ORDER-1","links":[{"rel":"approve","href":"https://paypal.example/approve"}]}`))
				default:
					t.Fatalf("unexpected PayPal API path: %s %s", r.Method, r.URL.Path)
				}
			}))
			defer server.Close()

			setupPayPalRefundTestDB(t, server.URL)
			resetPayPalTokenCache()
			plugin := NewPayPalPlugin(services.NewPluginService(database.GetDBWrapper()))

			resp, err := plugin.CreateBillPayment(42, 7, 2500, "USD", nil)
			require.NoError(t, err)
			require.Equal(t, "ORDER-1", resp.PaymentID)
			appContext, ok := order["application_context"].(map[string]interface{})
			require.True(t, ok, "order request must carry application_context")
			require.Equal(t, tc.want, appContext["brand_name"])
		})
	}
}
