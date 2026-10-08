package stripe

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// TestCreateBillPayment_ZeroDecimalCurrencyChargesWholeUnits locks F-ZERODEC:
// the platform stores every amount as major*100 "cents", but Stripe's
// unit_amount is the currency's smallest unit. For a zero-decimal currency
// (JPY) that is the whole unit, so a ¥1000 bill (stored 100000) must charge
// Stripe 1000 — not 100000, which would be a 100× overcharge.
func TestCreateBillPayment_ZeroDecimalCurrencyChargesWholeUnits(t *testing.T) {
	setupStripeRefundTestDB(t, map[string]interface{}{
		"secret_key":      "sk_test_jpy",
		"publishable_key": "pk_test_jpy",
	})

	var checkoutBody string
	restore := interceptStripeHTTP(t, func(r *http.Request, body string) (*http.Response, error) {
		require.Equal(t, "/v1/checkout/sessions", r.URL.Path)
		checkoutBody = body
		return stripeJSONResponse(http.StatusOK, map[string]interface{}{
			"id":  "cs_test_jpy",
			"url": "https://checkout.stripe.com/c/pay/cs_test_jpy",
		}), nil
	})
	defer restore()

	plugin := NewStripePlugin(services.NewPluginService(database.GetDBWrapper()))
	_, err := plugin.CreateBillPayment(42, 7, 100000, "JPY", map[string]interface{}{
		"return_url": "https://shop.test/ok",
		"cancel_url": "https://shop.test/no",
	})
	require.NoError(t, err)

	vals, err := url.ParseQuery(checkoutBody)
	require.NoError(t, err)
	var unitAmount string
	for k, v := range vals {
		if strings.Contains(k, "unit_amount") && len(v) > 0 {
			unitAmount = v[0]
		}
	}
	require.Equal(t, "1000", unitAmount, "JPY ¥1000 must charge Stripe 1000 yen, not 100000")
}

// A 2-decimal currency is unchanged: stored cents already are Stripe's minor
// unit, so $25.00 (2500 cents) charges 2500.
func TestCreateBillPayment_TwoDecimalCurrencyUnchanged(t *testing.T) {
	setupStripeRefundTestDB(t, map[string]interface{}{
		"secret_key":      "sk_test_usd",
		"publishable_key": "pk_test_usd",
	})

	var checkoutBody string
	restore := interceptStripeHTTP(t, func(r *http.Request, body string) (*http.Response, error) {
		checkoutBody = body
		return stripeJSONResponse(http.StatusOK, map[string]interface{}{
			"id":  "cs_test_usd",
			"url": "https://checkout.stripe.com/c/pay/cs_test_usd",
		}), nil
	})
	defer restore()

	plugin := NewStripePlugin(services.NewPluginService(database.GetDBWrapper()))
	_, err := plugin.CreateBillPayment(42, 9, 2500, "USD", map[string]interface{}{
		"return_url": "https://shop.test/ok",
		"cancel_url": "https://shop.test/no",
	})
	require.NoError(t, err)

	vals, err := url.ParseQuery(checkoutBody)
	require.NoError(t, err)
	var unitAmount string
	for k, v := range vals {
		if strings.Contains(k, "unit_amount") && len(v) > 0 {
			unitAmount = v[0]
		}
	}
	require.Equal(t, "2500", unitAmount)
}
