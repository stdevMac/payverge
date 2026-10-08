package stripe

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestRefundPaymentResolvesCheckoutSessionAndCreatesStripeRefund(t *testing.T) {
	setupStripeRefundTestDB(t, map[string]interface{}{
		"secret_key":      "sk_test_refund",
		"publishable_key": "pk_test_refund",
	})

	var calls []stripeRefundHTTPCall
	restore := interceptStripeHTTP(t, func(r *http.Request, body string) (*http.Response, error) {
		calls = append(calls, stripeRefundHTTPCall{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   body,
			Auth:   r.Header.Get("Authorization"),
		})

		switch r.URL.Path {
		case "/v1/checkout/sessions/cs_test_split_123":
			return stripeJSONResponse(http.StatusOK, map[string]interface{}{
				"id":             "cs_test_split_123",
				"payment_intent": "pi_test_split_123",
			}), nil
		case "/v1/refunds":
			require.Contains(t, body, "payment_intent=pi_test_split_123")
			require.Contains(t, body, "amount=2800")
			return stripeJSONResponse(http.StatusOK, map[string]interface{}{
				"id":     "re_test_split_123",
				"object": "refund",
				"status": "succeeded",
			}), nil
		default:
			t.Fatalf("unexpected Stripe request path %s", r.URL.Path)
			return nil, nil
		}
	})
	defer restore()

	plugin := NewStripePlugin(services.NewPluginService(database.GetDBWrapper()))
	err := plugin.RefundPayment(42, "cs_test_split_123", 2800)
	require.NoError(t, err)
	require.Len(t, calls, 2)
	require.Equal(t, "GET", calls[0].Method)
	require.Equal(t, "/v1/checkout/sessions/cs_test_split_123", calls[0].Path)
	require.Equal(t, "POST", calls[1].Method)
	require.Equal(t, "/v1/refunds", calls[1].Path)
	require.Equal(t, "Bearer sk_test_refund", calls[1].Auth)
}

func TestRefundPaymentUsesPaymentIntentDirectly(t *testing.T) {
	setupStripeRefundTestDB(t, map[string]interface{}{
		"secret_key":      "sk_test_refund",
		"publishable_key": "pk_test_refund",
	})

	var calls []stripeRefundHTTPCall
	restore := interceptStripeHTTP(t, func(r *http.Request, body string) (*http.Response, error) {
		calls = append(calls, stripeRefundHTTPCall{Method: r.Method, Path: r.URL.Path, Body: body})
		require.Equal(t, "/v1/refunds", r.URL.Path)
		require.Contains(t, body, "payment_intent=pi_test_direct_123")
		require.NotContains(t, body, "amount=", "amount=0 should request a full Stripe refund")
		return stripeJSONResponse(http.StatusOK, map[string]interface{}{
			"id":     "re_test_direct_123",
			"object": "refund",
			"status": "succeeded",
		}), nil
	})
	defer restore()

	plugin := NewStripePlugin(services.NewPluginService(database.GetDBWrapper()))
	err := plugin.RefundPayment(42, "pi_test_direct_123", 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
}

// TestRefundPaymentSetsIdempotencyAndVersionHeaders locks the MED fixes: a
// refund POST must carry a deterministic Idempotency-Key (so a network retry
// cannot double-refund) and pin Stripe-Version (so response parsing is stable).
func TestRefundPaymentSetsIdempotencyAndVersionHeaders(t *testing.T) {
	setupStripeRefundTestDB(t, map[string]interface{}{
		"secret_key":      "sk_test_refund",
		"publishable_key": "pk_test_refund",
	})

	var refundReq *http.Request
	restore := interceptStripeHTTP(t, func(r *http.Request, body string) (*http.Response, error) {
		switch r.URL.Path {
		case "/v1/payment_intents/pi_test_hdr":
			return stripeJSONResponse(http.StatusOK, map[string]interface{}{
				"id":       "pi_test_hdr",
				"currency": "usd",
			}), nil
		case "/v1/refunds":
			refundReq = r
			return stripeJSONResponse(http.StatusOK, map[string]interface{}{
				"id":     "re_test_hdr",
				"object": "refund",
				"status": "succeeded",
			}), nil
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
		return nil, nil
	})
	defer restore()

	plugin := NewStripePlugin(services.NewPluginService(database.GetDBWrapper()))
	require.NoError(t, plugin.RefundPayment(42, "pi_test_hdr", 500))
	require.NotNil(t, refundReq)
	require.NotEmpty(t, refundReq.Header.Get("Idempotency-Key"), "refund POST must carry an Idempotency-Key")
	require.Equal(t, "refund_pi_test_hdr_500", refundReq.Header.Get("Idempotency-Key"), "idempotency key must be deterministic")
	require.Equal(t, stripeAPIVersion, refundReq.Header.Get("Stripe-Version"), "refund POST must pin Stripe-Version")
}

// TestRefundPaymentRejectsFailedRefundStatus locks the MED fix: a refund object
// returned with status "failed"/"canceled" must not be treated as success.
func TestRefundPaymentRejectsFailedRefundStatus(t *testing.T) {
	setupStripeRefundTestDB(t, map[string]interface{}{
		"secret_key":      "sk_test_refund",
		"publishable_key": "pk_test_refund",
	})

	restore := interceptStripeHTTP(t, func(r *http.Request, body string) (*http.Response, error) {
		return stripeJSONResponse(http.StatusOK, map[string]interface{}{
			"id":     "re_test_failed",
			"object": "refund",
			"status": "failed",
		}), nil
	})
	defer restore()

	plugin := NewStripePlugin(services.NewPluginService(database.GetDBWrapper()))
	err := plugin.RefundPayment(42, "pi_test_failed", 0)
	require.Error(t, err, "a failed refund status must surface as an error")
	require.Contains(t, err.Error(), "failed")
}

// A "pending" refund is a legitimate success per Stripe semantics (async
// settlement) and must not error.
func TestRefundPaymentAcceptsPendingRefundStatus(t *testing.T) {
	setupStripeRefundTestDB(t, map[string]interface{}{
		"secret_key":      "sk_test_refund",
		"publishable_key": "pk_test_refund",
	})

	restore := interceptStripeHTTP(t, func(r *http.Request, body string) (*http.Response, error) {
		return stripeJSONResponse(http.StatusOK, map[string]interface{}{
			"id":     "re_test_pending",
			"object": "refund",
			"status": "pending",
		}), nil
	})
	defer restore()

	plugin := NewStripePlugin(services.NewPluginService(database.GetDBWrapper()))
	require.NoError(t, plugin.RefundPayment(42, "pi_test_pending", 0))
}

type stripeRefundHTTPCall struct {
	Method string
	Path   string
	Body   string
	Auth   string
}

type stripeRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn stripeRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func interceptStripeHTTP(t *testing.T, handler func(*http.Request, string) (*http.Response, error)) func() {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = stripeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "api.stripe.com", r.URL.Host)
		var bodyBytes []byte
		if r.Body != nil {
			var err error
			bodyBytes, err = io.ReadAll(r.Body)
			require.NoError(t, err)
		}
		return handler(r, string(bodyBytes))
	})
	return func() {
		http.DefaultTransport = original
	}
}

func stripeJSONResponse(status int, payload map[string]interface{}) *http.Response {
	body, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
}

func setupStripeRefundTestDB(t *testing.T, config map[string]interface{}) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Plugin{},
		&database.BusinessPlugin{},
	))

	business := database.Business{ID: 42, BusinessId: "stripe-refund-business", Name: "Stripe Refund Test"}
	require.NoError(t, gormDB.Create(&business).Error)
	require.Equal(t, uint(42), business.ID)

	plugin := database.Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, gormDB.Create(&plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, config))
}
