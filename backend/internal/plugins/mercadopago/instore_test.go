package mercadopago

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// F9: manual-mode config without mp_user_id fetches /users/me and persists it.
func TestEnsureStoreAndPOS_ResolvesMPUserIDFromUsersMe(t *testing.T) {
	var (
		usersMeCalls int32
		storePosts   int32
		posPosts     int32
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/me":
			atomic.AddInt32(&usersMeCalls, 1)
			_, _ = w.Write([]byte(`{"id":777888,"nickname":"manual-merchant"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/users/777888/stores":
			atomic.AddInt32(&storePosts, 1)
			_, _ = w.Write([]byte(`{"id":555,"external_id":"payverge-store-42"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/pos":
			atomic.AddInt32(&posPosts, 1)
			_, _ = w.Write([]byte(`{"id":666,"external_id":"PAYVERGEPOS42"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// No mp_user_id — simulates manual access-token config.
	setupMercadoPagoInStoreTestDB(t, server.URL, map[string]interface{}{
		"access_token":   "APP_USR-manual-access-token-1234567890",
		"public_key":     "APP_USR-manual-public-key-1234567890",
		"environment":    "sandbox",
		"base_url":       "https://api.staging.example",
		"api_base_url":   server.URL,
		"webhook_secret": "mp-webhook-secret",
	})
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	externalPOSID, err := plugin.ensureStoreAndPOS(42)
	require.NoError(t, err)
	require.Equal(t, "PAYVERGEPOS42", externalPOSID)
	require.Equal(t, int32(1), atomic.LoadInt32(&usersMeCalls))
	require.Equal(t, int32(1), atomic.LoadInt32(&storePosts))
	require.Equal(t, int32(1), atomic.LoadInt32(&posPosts))

	cfg, err := plugin.pluginService.GetPluginConfig(42, "mercadopago")
	require.NoError(t, err)
	require.Equal(t, "777888", configString(cfg, configKeyMPUserID))
}

func TestEnsureStoreAndPOS_UsersMeFailureIsActionable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/users/me" {
			http.Error(w, `{"message":"invalid access token"}`, http.StatusUnauthorized)
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	setupMercadoPagoInStoreTestDB(t, server.URL, map[string]interface{}{
		"access_token":   "APP_USR-bad-access-token-1234567890",
		"public_key":     "APP_USR-bad-public-key-1234567890",
		"environment":    "sandbox",
		"base_url":       "https://api.staging.example",
		"api_base_url":   server.URL,
		"webhook_secret": "mp-webhook-secret",
	})
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	_, err := plugin.ensureStoreAndPOS(42)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUsersMeFailed)
	require.Contains(t, err.Error(), "access token")
}

func TestEnsureStoreAndPOS_CreatesAndPersists(t *testing.T) {
	// The POS label in the merchant's MP account names this instance.
	t.Setenv("PRODUCT_NAME", "Acme POS")
	var (
		storePosts int32
		posPosts   int32
		gotStore   map[string]interface{}
		gotPOS     map[string]interface{}
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/users/999001/stores":
			atomic.AddInt32(&storePosts, 1)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &gotStore))
			_, _ = w.Write([]byte(`{"id":12354567,"external_id":"payverge-store-42","name":"MercadoPago Refund Test"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/pos":
			atomic.AddInt32(&posPosts, 1)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &gotPOS))
			_, _ = w.Write([]byte(`{"id":23545678,"external_id":"PAYVERGEPOS42","store_id":12354567}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupMercadoPagoInStoreTestDB(t, server.URL, map[string]interface{}{
		"access_token":   "APP_USR-refund-access-token-1234567890",
		"public_key":     "APP_USR-refund-public-key-1234567890",
		"environment":    "sandbox",
		"base_url":       "https://api.staging.example",
		"api_base_url":   server.URL,
		"webhook_secret": "mp-webhook-secret",
		"mp_user_id":     "999001",
	})
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	externalPOSID, err := plugin.ensureStoreAndPOS(42)
	require.NoError(t, err)
	require.Equal(t, "PAYVERGEPOS42", externalPOSID)

	require.Equal(t, int32(1), atomic.LoadInt32(&storePosts))
	require.Equal(t, int32(1), atomic.LoadInt32(&posPosts))

	require.Equal(t, "MercadoPago Refund Test", gotStore["name"])
	require.Equal(t, "payverge-store-42", gotStore["external_id"])
	require.NotNil(t, gotStore["location"])

	require.Equal(t, "Acme POS", gotPOS["name"])
	require.Equal(t, "payverge-store-42", gotPOS["external_store_id"])
	require.Equal(t, "PAYVERGEPOS42", gotPOS["external_id"])
	// MP rejects non-alphanumeric POS external_ids ("external_id must be
	// alphanumeric") — the store endpoint tolerates hyphens, the POS one doesn't.
	require.Regexp(t, `^[A-Za-z0-9]+$`, gotPOS["external_id"])
	require.Equal(t, false, gotPOS["fixed_amount"])
	// store_id sent as JSON number
	require.Equal(t, float64(12354567), gotPOS["store_id"])

	cfg, err := plugin.pluginService.GetPluginConfig(42, "mercadopago")
	require.NoError(t, err)
	require.Equal(t, "12354567", configString(cfg, configKeyMPStoreID))
	require.Equal(t, "PAYVERGEPOS42", configString(cfg, configKeyMPExternalPOSID))
}

func TestEnsureStoreAndPOS_IdempotentSecondCallZeroHTTP(t *testing.T) {
	var httpCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&httpCalls, 1)
		t.Errorf("second ensureStoreAndPOS must not hit Mercado Pago: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	setupMercadoPagoInStoreTestDB(t, server.URL, map[string]interface{}{
		"access_token":       "APP_USR-refund-access-token-1234567890",
		"public_key":         "APP_USR-refund-public-key-1234567890",
		"environment":        "sandbox",
		"base_url":           "https://api.staging.example",
		"api_base_url":       server.URL,
		"webhook_secret":     "mp-webhook-secret",
		"mp_user_id":         "999001",
		"mp_store_id":        "12354567",
		"mp_external_pos_id": "payverge-pos-42",
	})
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	externalPOSID, err := plugin.ensureStoreAndPOS(42)
	require.NoError(t, err)
	require.Equal(t, "payverge-pos-42", externalPOSID)
	require.Equal(t, int32(0), atomic.LoadInt32(&httpCalls))
}

func TestEnsureStoreAndPOS_AlreadyExistsUsesGETPos(t *testing.T) {
	var (
		storePosts int32
		posPosts   int32
		posGets    int32
		storeGets  int32
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/users/999001/stores":
			atomic.AddInt32(&storePosts, 1)
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"store already exists","error":"conflict"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/users/999001/stores/search":
			atomic.AddInt32(&storeGets, 1)
			require.Equal(t, "payverge-store-42", r.URL.Query().Get("external_id"))
			_, _ = w.Write([]byte(`{"results":[{"id":12354567,"external_id":"payverge-store-42"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/pos":
			atomic.AddInt32(&posPosts, 1)
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"pos already exists","error":"conflict"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/pos":
			atomic.AddInt32(&posGets, 1)
			require.Equal(t, "PAYVERGEPOS42", r.URL.Query().Get("external_id"))
			_, _ = w.Write([]byte(`{"id":23545678,"external_id":"PAYVERGEPOS42","store_id":12354567}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupMercadoPagoInStoreTestDB(t, server.URL, map[string]interface{}{
		"access_token":   "APP_USR-refund-access-token-1234567890",
		"public_key":     "APP_USR-refund-public-key-1234567890",
		"environment":    "sandbox",
		"base_url":       "https://api.staging.example",
		"api_base_url":   server.URL,
		"webhook_secret": "mp-webhook-secret",
		"mp_user_id":     "999001",
	})
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	externalPOSID, err := plugin.ensureStoreAndPOS(42)
	require.NoError(t, err)
	require.Equal(t, "PAYVERGEPOS42", externalPOSID)

	require.Equal(t, int32(1), atomic.LoadInt32(&storePosts))
	require.Equal(t, int32(1), atomic.LoadInt32(&storeGets))
	require.Equal(t, int32(1), atomic.LoadInt32(&posPosts))
	require.Equal(t, int32(1), atomic.LoadInt32(&posGets))

	cfg, err := plugin.pluginService.GetPluginConfig(42, "mercadopago")
	require.NoError(t, err)
	require.Equal(t, "12354567", configString(cfg, configKeyMPStoreID))
	require.Equal(t, "PAYVERGEPOS42", configString(cfg, configKeyMPExternalPOSID))

	// After persist, a second call short-circuits with zero HTTP (server still
	// fails loudly if anything is requested).
	beforePosts := atomic.LoadInt32(&storePosts) + atomic.LoadInt32(&posPosts) +
		atomic.LoadInt32(&posGets) + atomic.LoadInt32(&storeGets)
	externalPOSID2, err := plugin.ensureStoreAndPOS(42)
	require.NoError(t, err)
	require.Equal(t, "PAYVERGEPOS42", externalPOSID2)
	afterPosts := atomic.LoadInt32(&storePosts) + atomic.LoadInt32(&posPosts) +
		atomic.LoadInt32(&posGets) + atomic.LoadInt32(&storeGets)
	require.Equal(t, beforePosts, afterPosts, "second ensureStoreAndPOS must make zero HTTP")
}

func TestIsMercadoPagoAlreadyExists(t *testing.T) {
	require.True(t, isMercadoPagoAlreadyExists(fmtErr("MercadoPago API error 409: conflict")))
	require.True(t, isMercadoPagoAlreadyExists(fmtErr("MercadoPago API error 400: external_id already exists")))
	require.True(t, isMercadoPagoAlreadyExists(fmtErr("something already_exists in body")))
	// Live sandbox shape for a duplicate store create.
	require.True(t, isMercadoPagoAlreadyExists(fmtErr(`MercadoPago API error 400: {"error":"bad_request","message":"external id 'payverge-store-1' is already assigned to this user 3606264032"}`)))
	require.False(t, isMercadoPagoAlreadyExists(fmtErr("MercadoPago API error 500: boom")))
	require.False(t, isMercadoPagoAlreadyExists(nil))
}

func fmtErr(msg string) error {
	return &simpleErr{msg: msg}
}

type simpleErr struct{ msg string }

func (e *simpleErr) Error() string { return e.msg }

func setupMercadoPagoInStoreTestDB(t *testing.T, apiBaseURL string, cfg map[string]interface{}) {
	t.Helper()
	// Reuse the shared refund-test helper when we only need defaults; here we
	// always need custom cfg (mp_user_id, optional pre-seeded store/pos ids).
	if cfg["api_base_url"] == nil && apiBaseURL != "" {
		cfg["api_base_url"] = apiBaseURL
	}
	setupMercadoPagoRefundTestDBWithConfig(t, cfg)
}

// A business without a name gets an MP store named after this instance
// (PRODUCT_NAME), never the upstream product.
func TestEnsureStoreAndPOS_NamelessBusinessUsesProductName(t *testing.T) {
	t.Setenv("PRODUCT_NAME", "Acme POS")
	var gotStore map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/users/999001/stores":
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &gotStore))
			_, _ = w.Write([]byte(`{"id":12354567,"external_id":"payverge-store-42"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/pos":
			_, _ = w.Write([]byte(`{"id":23545678,"external_id":"PAYVERGEPOS42","store_id":12354567}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupMercadoPagoInStoreTestDB(t, server.URL, map[string]interface{}{
		"access_token":   "APP_USR-refund-access-token-1234567890",
		"public_key":     "APP_USR-refund-public-key-1234567890",
		"environment":    "sandbox",
		"base_url":       "https://api.staging.example",
		"api_base_url":   server.URL,
		"webhook_secret": "mp-webhook-secret",
		"mp_user_id":     "999001",
	})
	require.NoError(t, database.GetDB().Model(&database.Business{}).Where("id = ?", 42).Update("name", "").Error)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	_, err := plugin.ensureStoreAndPOS(42)
	require.NoError(t, err)
	require.Equal(t, "Acme POS", gotStore["name"])
}
