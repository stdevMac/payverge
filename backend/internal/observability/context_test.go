package observability

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/middleware"
)

func TestBuildRequestContext_ExtractsSafeGinFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	var got RequestContext
	router.GET("/api/v1/inside/businesses/:business_id/orders", func(c *gin.Context) {
		c.Set(middleware.RequestIDKey, "req-1")
		c.Set("token_type", "staff")
		c.Set("staff_id", uint(7))
		c.Set("staff_business_id", float64(42))
		c.Status(http.StatusCreated)

		got = BuildRequestContext(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/inside/businesses/42/orders?token=secret", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, "backend", got.Tags["service"])
	require.Equal(t, "req-1", got.Tags["request_id"])
	require.Equal(t, "staff", got.Tags["auth_source"])
	require.Equal(t, "42", got.Tags["business_id"])
	require.Equal(t, "staff:7", got.UserID)
	require.Equal(t, http.MethodGet, got.Context["method"])
	require.Equal(t, "/api/v1/inside/businesses/:business_id/orders", got.Context["route"])
	// path is the route template, never the raw URL (capability tokens).
	require.Equal(t, "/api/v1/inside/businesses/:business_id/orders", got.Context["path"])
	require.Equal(t, http.StatusCreated, got.Context["status"])
	require.Equal(t, "req-1", got.Context["request_id"])
	require.Equal(t, "staff", got.Context["auth_source"])
	require.Equal(t, "42", got.Context["business_id"])
	require.Equal(t, "7", got.Context["staff_id"])
}

func TestBuildRequestContext_DoesNotExposeEmailsOrRawQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	var got RequestContext
	router.POST("/api/v1/customer/profile", func(c *gin.Context) {
		c.Set("token_type", "customer")
		c.Set("customer_id", uint(9))
		c.Set("customer_email", "guest@example.com")

		got = BuildRequestContext(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/profile?email=a@example.com", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "customer:9", got.UserID)
	require.Equal(t, "/api/v1/customer/profile", got.Context["path"])
	require.NotContains(t, got.Context, "customer_email")
	require.NotContains(t, fmt.Sprint(got.Context), "guest@example.com")
	require.NotContains(t, fmt.Sprint(got.Context), "a@example.com")
	require.NotContains(t, fmt.Sprint(got.Context), "?email=")
}

func TestBuildRequestContext_DoesNotExposeRawWalletAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const address = "0x1111111111111111111111111111111111111111"

	router := gin.New()
	var got RequestContext
	router.GET("/api/v1/inside/me", func(c *gin.Context) {
		c.Set("token_type", "wallet")
		c.Set("address", address)

		got = BuildRequestContext(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/inside/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, got.UserID, address)
	if got.UserID != "" {
		require.True(t, strings.HasPrefix(got.UserID, "web3:"), "web3 user IDs must be pseudonymous")
	}
	require.NotContains(t, fmt.Sprint(got.Context), address)
	require.NotContains(t, fmt.Sprint(got.Tags), address)
}

func TestBuildRequestContext_DoesNotTreatNonBusinessIDParamAsBusinessID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	var got RequestContext
	router.GET("/api/v1/admin/users/:id/detail", func(c *gin.Context) {
		got = BuildRequestContext(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/42/detail", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, got.Tags, "business_id")
	require.NotContains(t, got.Context, "business_id")
}

func TestBuildRequestContext_TreatsBusinessIDRouteParamAsBusinessID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	var got RequestContext
	router.GET("/api/v1/inside/businesses/:id/detail", func(c *gin.Context) {
		got = BuildRequestContext(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/inside/businesses/42/detail", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "42", got.Tags["business_id"])
	require.Equal(t, "42", got.Context["business_id"])
}

func TestApplyRequestContext_SetsTagsUserAndPayvergeContext(t *testing.T) {
	scope := sentry.NewScope()
	requestContext := RequestContext{
		Tags: map[string]string{
			"service":     "backend",
			"request_id":  "req-apply",
			"business_id": "42",
		},
		UserID: "staff:7",
		Context: map[string]interface{}{
			"service":    "backend",
			"method":     http.MethodGet,
			"request_id": "req-apply",
			"status":     http.StatusAccepted,
		},
	}

	ApplyRequestContext(scope, requestContext)

	event := scope.ApplyToEvent(&sentry.Event{}, nil, nil)
	require.Equal(t, "backend", event.Tags["service"])
	require.Equal(t, "req-apply", event.Tags["request_id"])
	require.Equal(t, "42", event.Tags["business_id"])
	require.Equal(t, "staff:7", event.User.ID)
	require.Equal(t, "backend", event.Contexts["payverge"]["service"])
	require.Equal(t, http.MethodGet, event.Contexts["payverge"]["method"])
	require.Equal(t, "req-apply", event.Contexts["payverge"]["request_id"])
	require.Equal(t, http.StatusAccepted, event.Contexts["payverge"]["status"])
}

func TestGinContextMiddleware_AppliesSentryGinHubAfterNext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sentryClient, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       testSentryDSN,
		Transport: &sentry.MockTransport{},
	})
	require.NoError(t, err)
	hub := sentry.NewHub(sentryClient, sentry.NewScope())

	router := gin.New()
	var duringHandler *sentry.Event
	var afterNext *sentry.Event
	router.Use(sentrygin.New(sentrygin.Options{}))
	router.Use(func(c *gin.Context) {
		c.Next()
		afterNext = eventFromGinHub(c)
	})
	router.Use(GinContextMiddleware())
	router.GET("/api/v1/inside/businesses/:business_id/orders", func(c *gin.Context) {
		duringHandler = eventFromGinHub(c)
		c.Set(middleware.RequestIDKey, "req-after")
		c.Set("token_type", "staff")
		c.Set("staff_id", uint(7))
		c.Status(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/inside/businesses/42/orders?token=secret", nil)
	req = req.WithContext(sentry.SetHubOnContext(context.Background(), hub))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	// The context is applied only in the deferred call after c.Next(), so during
	// handler execution the payverge scope is not yet populated. This avoids the
	// wasted pre-Next apply() that ran against an empty auth/status context.
	require.NotNil(t, duringHandler)
	require.Nil(t, duringHandler.Contexts["payverge"])
	require.Empty(t, duringHandler.User.ID)

	require.NotNil(t, afterNext)
	afterContext := afterNext.Contexts["payverge"]
	require.Equal(t, "req-after", afterNext.Tags["request_id"])
	require.Equal(t, "staff", afterNext.Tags["auth_source"])
	require.Equal(t, "42", afterNext.Tags["business_id"])
	require.Equal(t, "staff:7", afterNext.User.ID)
	require.Equal(t, "req-after", afterContext["request_id"])
	require.Equal(t, "staff", afterContext["auth_source"])
	require.Equal(t, "7", afterContext["staff_id"])
	require.Equal(t, http.StatusCreated, afterContext["status"])
	require.NotContains(t, fmt.Sprint(afterContext), "token=secret")
}

func eventFromGinHub(c *gin.Context) *sentry.Event {
	hub := sentrygin.GetHubFromContext(c)
	if hub == nil || hub.Scope() == nil {
		return nil
	}

	return hub.Scope().ApplyToEvent(&sentry.Event{}, nil, hub.Client())
}

func TestBuildRequestContext_NeverExposesCapabilityTokensInPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct{ name, url, want string }{
		{"matched route reports template", "/api/v1/guest/bill/BILLSECRET/split?token=Q", "/api/v1/guest/bill/:bill_token/split"},
		{"unmatched route reports redacted raw path", "/api/v1/guest/bill/NOROUTESECRET/x?token=Q", "/api/v1/guest/bill/%5BFiltered%5D/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			var got RequestContext
			router.GET("/api/v1/guest/bill/:bill_token/split", func(c *gin.Context) { got = BuildRequestContext(c) })
			router.NoRoute(func(c *gin.Context) { got = BuildRequestContext(c) })
			router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tc.url, nil))
			require.Equal(t, tc.want, got.Context["path"])
			require.NotContains(t, fmt.Sprint(got.Context), "SECRET")
			require.NotContains(t, fmt.Sprint(got.Tags), "SECRET")
		})
	}
}
