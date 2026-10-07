package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/getsentry/sentry-go/attribute"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const testSentryDSN = "https://public@example.com/1"

func TestDisabledClient_IsSafeToUse(t *testing.T) {
	client := DisabledClient()

	require.False(t, client.Enabled())
	require.True(t, client.Flush(time.Millisecond))
	require.Empty(t, GinMiddlewares(client))
}

func TestInit_DisabledConfigDoesNotInstallHooks(t *testing.T) {
	testLogger := logrus.New()

	client, err := Init(Config{Service: "backend"}, testLogger)

	require.NoError(t, err)
	require.False(t, client.Enabled())
	require.Empty(t, testLogger.Hooks)
}

func TestInit_EnabledConfigInstallsSentryOptionsWithoutLogHook(t *testing.T) {
	restoreSentryClient(t)
	testLogger := logrus.New()

	client, err := Init(Config{
		Enabled:          true,
		DSN:              testSentryDSN,
		Environment:      "staging",
		Release:          "release-sha",
		Service:          "backend",
		EnableTracing:    true,
		TracesSampleRate: 0.25,
		EnableLogs:       false,
	}, testLogger)

	require.NoError(t, err)
	require.True(t, client.Enabled())
	require.Empty(t, testLogger.Hooks)

	options := sentry.CurrentHub().Client().Options()
	require.Equal(t, testSentryDSN, options.Dsn)
	require.Equal(t, "staging", options.Environment)
	require.Equal(t, "release-sha", options.Release)
	require.Equal(t, "backend", options.ServerName)
	require.True(t, options.EnableTracing)
	require.Equal(t, 0.25, options.TracesSampleRate)
	require.False(t, options.EnableLogs)
	require.False(t, options.SendDefaultPII)
	require.True(t, options.AttachStacktrace)
	require.Equal(t, "backend", options.Tags["service"])

	require.NotNil(t, options.BeforeSend)
	scrubbedEvent := options.BeforeSend(&sentry.Event{
		User: sentry.User{ID: "guest@example.com"},
		Request: &sentry.Request{
			URL:         "/api/v1/orders?token=secret",
			QueryString: "token=secret",
		},
	}, nil)
	require.Empty(t, scrubbedEvent.User.ID)
	require.Equal(t, "/api/v1/orders", scrubbedEvent.Request.URL)
	require.Empty(t, scrubbedEvent.Request.QueryString)

	require.NotNil(t, options.BeforeSendTransaction)
	scrubbedTransaction := options.BeforeSendTransaction(&sentry.Event{
		Request: &sentry.Request{URL: "/api/v1/customer/profile?email=a@example.com"},
	}, nil)
	require.Equal(t, "/api/v1/customer/profile", scrubbedTransaction.Request.URL)
}

func TestInit_EnabledLogsInstallsLogrusHook(t *testing.T) {
	restoreSentryClient(t)
	testLogger := logrus.New()

	client, err := Init(Config{
		Enabled:          true,
		DSN:              testSentryDSN,
		Environment:      "staging",
		Release:          "release-sha",
		Service:          "backend",
		EnableTracing:    false,
		TracesSampleRate: 0,
		EnableLogs:       true,
		LogLevels:        []logrus.Level{logrus.WarnLevel, logrus.ErrorLevel},
	}, testLogger)

	require.NoError(t, err)
	require.True(t, client.Enabled())
	require.Len(t, testLogger.Hooks[logrus.WarnLevel], 1)
	require.Len(t, testLogger.Hooks[logrus.ErrorLevel], 1)
	require.Empty(t, testLogger.Hooks[logrus.InfoLevel])

	options := sentry.CurrentHub().Client().Options()
	require.NotNil(t, options.BeforeSendLog)
	scrubbed := options.BeforeSendLog(&sentry.Log{
		Body: "owner@example.com",
		Attributes: map[string]attribute.Value{
			"Authorization": attribute.StringValue("Bearer log-secret"),
		},
	})
	require.Equal(t, redactedValue, scrubbed.Body)
	require.Equal(t, redactedValue, scrubbed.Attributes["Authorization"].AsString())
}

func TestInit_EnabledLogsFiltersTerminatingLogLevels(t *testing.T) {
	restoreSentryClient(t)
	testLogger := logrus.New()

	client, err := Init(Config{
		Enabled:          true,
		DSN:              testSentryDSN,
		Environment:      "staging",
		Release:          "release-sha",
		Service:          "backend",
		EnableTracing:    false,
		TracesSampleRate: 0,
		EnableLogs:       true,
		LogLevels:        []logrus.Level{logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel},
	}, testLogger)

	require.NoError(t, err)
	require.True(t, client.Enabled())
	require.Len(t, testLogger.Hooks[logrus.ErrorLevel], 1)
	require.Empty(t, testLogger.Hooks[logrus.FatalLevel])
	require.Empty(t, testLogger.Hooks[logrus.PanicLevel])
	require.ElementsMatch(t, []logrus.Level{logrus.ErrorLevel}, testLogger.Hooks[logrus.ErrorLevel][0].Levels())
}

func TestInit_EnabledLogsSkipsHookWhenOnlyTerminatingLevelsConfigured(t *testing.T) {
	restoreSentryClient(t)
	testLogger := logrus.New()

	client, err := Init(Config{
		Enabled:          true,
		DSN:              testSentryDSN,
		Environment:      "staging",
		Release:          "release-sha",
		Service:          "backend",
		EnableTracing:    false,
		TracesSampleRate: 0,
		EnableLogs:       true,
		LogLevels:        []logrus.Level{logrus.FatalLevel, logrus.PanicLevel},
	}, testLogger)

	require.NoError(t, err)
	require.True(t, client.Enabled())
	require.Empty(t, testLogger.Hooks)
}

func TestGinMiddlewares_EnabledReturnsSentryThenContextMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middlewares := GinMiddlewares(&Client{enabled: true})
	require.Len(t, middlewares, 2)
	require.Contains(t, functionName(middlewares[0]), "sentry-go/gin")
	require.Contains(t, functionName(middlewares[1]), "GinContextMiddleware")

	sentryClient, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       testSentryDSN,
		Transport: &sentry.MockTransport{},
	})
	require.NoError(t, err)
	hub := sentry.NewHub(sentryClient, sentry.NewScope())

	router := gin.New()
	var captured *sentry.Event
	// GinContextMiddleware applies the scope in a deferred call after c.Next(),
	// so capture the event from a middleware that reads the scope once the
	// context middleware (and handler) have unwound.
	router.Use(middlewares[0])
	router.Use(func(c *gin.Context) {
		c.Next()
		captured = eventFromGinHub(c)
	})
	router.Use(middlewares[1])
	router.GET("/api/v1/health", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req = req.WithContext(sentry.SetHubOnContext(context.Background(), hub))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.NotNil(t, captured)
	require.Equal(t, "backend", captured.Tags["service"])
	require.Equal(t, "backend", captured.Contexts["payverge"]["service"])
	require.Equal(t, http.MethodGet, captured.Contexts["payverge"]["method"])
}

func TestGinMiddlewares_EnabledRepanicsToGinRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sentryClient, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       testSentryDSN,
		Transport: &sentry.MockTransport{},
	})
	require.NoError(t, err)
	hub := sentry.NewHub(sentryClient, sentry.NewScope())

	router := gin.New()
	router.Use(gin.RecoveryWithWriter(io.Discard))
	router.Use(GinMiddlewares(&Client{enabled: true})...)
	router.GET("/panic", func(c *gin.Context) {
		panic("observability panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req = req.WithContext(sentry.SetHubOnContext(context.Background(), hub))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func restoreSentryClient(t *testing.T) {
	t.Helper()

	previousClient := sentry.CurrentHub().Client()
	t.Cleanup(func() {
		sentry.CurrentHub().BindClient(previousClient)
	})
}

func functionName(handler gin.HandlerFunc) string {
	return runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name()
}
