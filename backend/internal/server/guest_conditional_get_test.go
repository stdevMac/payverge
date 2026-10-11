package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// Guest menu/table polls revalidate with If-None-Match. A matching validator
// must short-circuit to 304 before the menu snapshot, promotions, inventory
// orderability projection or translations are loaded: zero SQL on the hit.

type guestConditionalFixture struct {
	recorder *publicGuestSQLRecorder
	business *database.Business
	table    *database.Table
}

func setupGuestConditionalFixture(t *testing.T, code string) guestConditionalFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicGuestTableHandlerTestDBWithLogger(t, recorder)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.InventorySettings{}, &database.InventoryItem{}, &database.InventoryRecipe{},
		&database.BusinessOperatingHours{},
	))
	services.ResetPricingCache()
	t.Cleanup(services.ResetPricingCache)

	business := createSensitiveGuestTableBusiness(t, code)
	require.NoError(t, database.GetDB().Model(business).Updates(map[string]any{"custom_url": "cond-" + strings.ToLower(code), "business_page_enabled": true, "is_active": true}).Error)
	business.CustomURL = "cond-" + strings.ToLower(code)
	table := createGuestPublicTable(t, business.ID, code)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: `[{"id":"mains","name":"Mains","items":[{"id":"taco","name":"Taco","price":5,"is_available":true}]}]`,
		IsActive:   true,
	}).Error)
	return guestConditionalFixture{recorder: recorder, business: business, table: table}
}

type guestConditionalEndpoint struct {
	name    string
	handler gin.HandlerFunc
	params  func(f guestConditionalFixture) gin.Params
}

func guestConditionalEndpoints() []guestConditionalEndpoint {
	byCode := func(f guestConditionalFixture) gin.Params {
		return gin.Params{{Key: "code", Value: f.table.TableCode}}
	}
	return []guestConditionalEndpoint{
		{name: "menu-by-table", handler: GetMenuByTableCode, params: byCode},
		{name: "table", handler: GetTableByCodePublic, params: byCode},
		{name: "menu-by-custom-url", handler: GetMenuByBusinessCustomUrl, params: func(f guestConditionalFixture) gin.Params {
			return gin.Params{{Key: "customUrl", Value: f.business.CustomURL}}
		}},
	}
}

func serveGuestConditional(handler gin.HandlerFunc, params gin.Params, ifNoneMatch string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = params
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if ifNoneMatch != "" {
		c.Request.Header.Set("If-None-Match", ifNoneMatch)
	}
	handler(c)
	// AbortWithStatus on a bare test context leaves the recorder header
	// unflushed; mirror what the HTTP server would send.
	c.Writer.WriteHeaderNow()
	return w
}

func TestGuestConditionalGetReturns304WithoutLoading(t *testing.T) {
	for i, endpoint := range guestConditionalEndpoints() {
		t.Run(endpoint.name, func(t *testing.T) {
			f := setupGuestConditionalFixture(t, fmt.Sprintf("COND%d", i))
			params := endpoint.params(f)

			first := serveGuestConditional(endpoint.handler, params, "")
			require.Equal(t, http.StatusOK, first.Code, first.Body.String())
			etag := first.Header().Get("ETag")
			require.NotEmpty(t, etag, "%s must emit an ETag", endpoint.name)

			f.recorder.statements = nil
			second := serveGuestConditional(endpoint.handler, params, etag)
			require.Equal(t, http.StatusNotModified, second.Code)
			require.Equal(t, etag, second.Header().Get("ETag"))
			require.Empty(t, second.Body.String())
			require.Empty(t, f.recorder.statements, "a matching If-None-Match must not load menu, promotions or inventory")
		})
	}
}

func TestGuestConditionalGetRevalidatesAfterMenuWrite(t *testing.T) {
	for i, endpoint := range guestConditionalEndpoints() {
		t.Run(endpoint.name, func(t *testing.T) {
			f := setupGuestConditionalFixture(t, fmt.Sprintf("CONDW%d", i))
			params := endpoint.params(f)

			first := serveGuestConditional(endpoint.handler, params, "")
			require.Equal(t, http.StatusOK, first.Code, first.Body.String())
			etag := first.Header().Get("ETag")

			// A menu write bumps the version and runs the pricing-cache hook,
			// which must also drop the memoized validator.
			require.NoError(t, database.GetDB().Model(&database.Menu{}).
				Where("business_id = ?", f.business.ID).
				Updates(map[string]any{
					"version":    5,
					"categories": `[{"id":"mains","name":"Mains","items":[{"id":"taco","name":"Taco Supreme","price":6,"is_available":true}]}]`,
				}).Error)
			services.InvalidatePricingCache(f.business.ID)

			f.recorder.statements = nil
			second := serveGuestConditional(endpoint.handler, params, etag)
			require.Equal(t, http.StatusOK, second.Code, "stale validator must get the new body")
			require.NotEqual(t, etag, second.Header().Get("ETag"))
			require.Contains(t, second.Body.String(), "Taco Supreme")
			require.NotEmpty(t, f.recorder.statements)
		})
	}
}

func TestGuestTableEndpointETagIsPrivate(t *testing.T) {
	f := setupGuestConditionalFixture(t, "CONDPRIV")
	w := serveGuestConditional(GetTableByCodePublic, gin.Params{{Key: "code", Value: f.table.TableCode}}, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotEmpty(t, w.Header().Get("ETag"))
	require.Contains(t, w.Header().Get("Cache-Control"), "private", "table context must never be stored by a shared cache")
}

func BenchmarkGuestMenuByTableCodeConditionalSQLite(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	setupPublicGuestTableHandlerBenchmarkDB(b)
	require.NoError(b, database.GetDB().AutoMigrate(&database.InventorySettings{}, &database.InventoryItem{}, &database.InventoryRecipe{}, &database.BusinessOperatingHours{}))
	services.ResetPricingCache()
	b.Cleanup(services.ResetPricingCache)

	business := createSensitiveGuestTableBusiness(b, "CONDBENCH1")
	table := createGuestPublicTable(b, business.ID, "CONDBENCH1")
	require.NoError(b, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: `[{"id":"mains","name":"Mains","items":[{"id":"taco","name":"Taco","price":5,"is_available":true}]}]`,
		IsActive:   true,
	}).Error)

	r := gin.New()
	r.GET("/guest/table/:code/menu", GetMenuByTableCode)
	path := fmt.Sprintf("/guest/table/%s/menu", table.TableCode)
	warm := httptest.NewRecorder()
	r.ServeHTTP(warm, httptest.NewRequest(http.MethodGet, path, nil))
	etag := warm.Header().Get("ETag")
	if warm.Code != http.StatusOK || etag == "" {
		b.Fatalf("warm request: %d etag=%q", warm.Code, etag)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("If-None-Match", etag)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotModified {
			b.Fatalf("got %d", w.Code)
		}
	}
}

func TestGuestValidatorKeyOnlyMemoizesDefaultLanguage(t *testing.T) {
	require.Equal(t, "menu-table|T1|es", guestValidatorKey("menu-table", "T1", "", "es"))
	require.Equal(t, "menu-table|T1|es", guestValidatorKey("menu-table", "T1", "es", "es"))
	require.Equal(t, "menu-table|T1|en", guestValidatorKey("menu-table", "T1", "", ""))
	require.Empty(t, guestValidatorKey("menu-table", "T1", "fr", "es"), "translated bodies must recompute their translation digest")
}
