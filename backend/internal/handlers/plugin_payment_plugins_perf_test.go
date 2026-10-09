package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// paymentPluginSQLRecorder captures executed SQL so the guest payment-plugin
// listing endpoint can be asserted against its query SHAPE (CG-6 N+1 fix).
type paymentPluginSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *paymentPluginSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *paymentPluginSQLRecorder) normalized() []string {
	out := make([]string, 0, len(r.statements))
	for _, statement := range r.statements {
		out = append(out, strings.ToLower(strings.Join(strings.Fields(statement), " ")))
	}
	return out
}

// countCountStarOnBusinessPlugins counts the per-plugin
// IsPluginEnabledForBusiness "SELECT COUNT(*) ... business_plugins" probes.
func (r *paymentPluginSQLRecorder) countCountStarOnBusinessPlugins() int {
	count := 0
	for _, sql := range r.normalized() {
		if strings.HasPrefix(sql, "select count(") && strings.Contains(sql, "business_plugins") {
			count++
		}
	}
	return count
}

// countSingleConfigReads counts the per-plugin GetBusinessPluginConfig
// single-row reads ("SELECT COALESCE(bp.config ...) ... p.name = '<one>'").
// The SQLite logger interpolates placeholders, so a single-row read matches
// "p.name =" while the batched read matches "p.name in".
func (r *paymentPluginSQLRecorder) countSingleConfigReads() int {
	count := 0
	for _, sql := range r.normalized() {
		if !strings.Contains(sql, "coalesce(bp.config") {
			continue
		}
		if strings.Contains(sql, "p.name =") && !strings.Contains(sql, "p.name in") {
			count++
		}
	}
	return count
}

// countBatchConfigReads counts the single batched config load
// "SELECT ... COALESCE(bp.config ...) ... p.name IN (...)".
func (r *paymentPluginSQLRecorder) countBatchConfigReads() int {
	count := 0
	for _, sql := range r.normalized() {
		if strings.Contains(sql, "coalesce(bp.config") && strings.Contains(sql, "p.name in") {
			count++
		}
	}
	return count
}

func setupPaymentPluginsPerfDB(t testing.TB, gormLogger logger.Interface) (*PluginHandlers, *database.Business) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Plugin{}, &database.BusinessPlugin{}))

	business := &database.Business{
		BusinessId: fmt.Sprintf("payment-plugins-%d", time.Now().UnixNano()),
		Name:       "Payment Plugins Perf",
	}
	require.NoError(t, gormDB.Create(business).Error)

	// Seed a mix mirroring the production guest-visible payment set:
	// - stripe: config {"enabled":false}  -> excluded
	// - paypal: config {"enabled":"true"} -> included (string-parsed)
	// - mercadopago: no enabled key       -> included (default-on)
	// - acmepay: not guest-visible        -> excluded (guestPaymentOptionVisible gate)
	seed := []struct {
		name   string
		config string
	}{
		{"stripe", `{"enabled":false,"secret_key":"sk_live_must_not_leak"}`},
		{"paypal", `{"enabled":"true","client_id":"paypal-public-id","client_secret":"pp_secret_must_not_leak"}`},
		{"mercadopago", `{"access_token":"v1:mp_secret_must_not_leak"}`},
		{"acmepay", `{"api_key_encrypted":"v1:must_not_leak"}`},
	}
	for _, s := range seed {
		plugin := &database.Plugin{
			Name:        s.name,
			DisplayName: s.name,
			Category:    database.PluginCategoryPayment,
			IsActive:    true,
		}
		require.NoError(t, gormDB.Create(plugin).Error)
		require.NoError(t, gormDB.Create(&database.BusinessPlugin{
			BusinessID: business.ID,
			PluginID:   plugin.ID,
			IsEnabled:  true,
			Config:     s.config,
		}).Error)
	}

	return NewPluginHandlers(nil, nil), business
}

func performPaymentPluginsRequest(handler *PluginHandlers, businessID uint) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", businessID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handler.GetBusinessPaymentPlugins(c)
	return w
}

func TestGetBusinessPaymentPluginsBatchesConfigLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &paymentPluginSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	handler, business := setupPaymentPluginsPerfDB(t, recorder)

	recorder.statements = nil
	w := performPaymentPluginsRequest(handler, business.ID)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// SHAPE: the per-plugin IsPluginEnabledForBusiness COUNT(*) probes and the
	// per-plugin single-row config reads must be gone, replaced by exactly one
	// batched name-IN config load.
	assert.Zero(t, recorder.countCountStarOnBusinessPlugins(),
		"IsPluginEnabledForBusiness COUNT(*) probes must be eliminated")
	assert.Zero(t, recorder.countSingleConfigReads(),
		"per-plugin single-row config reads must be eliminated")
	assert.Equal(t, 1, recorder.countBatchConfigReads(),
		"config must be loaded in exactly one batched name-IN query")

	// BEHAVIOR PARITY: identical visible set + order to the old per-plugin
	// implementation. GetBusinessPlugins orders by (category, display_name);
	// mercadopago < paypal alphabetically; stripe excluded (enabled:false),
	// acmepay excluded (not guest-visible).
	names := make([]string, 0, len(resp.Plugins))
	for _, p := range resp.Plugins {
		names = append(names, p["name"].(string))
	}
	assert.Equal(t, []string{"mercadopago", "paypal"}, names)

	// SECURITY: the guest response must never carry the raw config secret blob.
	for _, p := range resp.Plugins {
		_, hasConfig := p["config"]
		assert.False(t, hasConfig, "guest payment plugin map must not expose config")
	}
	rawBody := w.Body.String()
	assert.NotContains(t, rawBody, "must_not_leak", "no plugin secret may reach the guest response")
}

func BenchmarkGetBusinessPaymentPluginsSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	handler, business := setupPaymentPluginsPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := performPaymentPluginsRequest(handler, business.ID)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}
