package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// BenchmarkMenuGet exercises GET /api/v1/business/:customUrl/menu against a
// real Postgres. This is the template other Postgres benches follow:
// setupServerBenchmarkDB (genesis-bootstrapped database → SetTestDB → truncate
// → testperf.LoadFixtures) → register a single route → b.ResetTimer → loop
// httptest requests. The schema is the genesis baseline, the same one an empty
// production database bootstraps from.
func BenchmarkMenuGet(b *testing.B) {
	setupServerBenchmarkDB(b)

	router := buildMenuBenchRouter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/business/perf-bench-001/menu", nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

// buildMenuBenchRouter wires only the menu GET route. No middleware chain —
// the bench measures the handler + db roundtrip, not the full production stack.
// Tasks 8b–8f will add small auth/RBAC stubs where their benches need them.
func buildMenuBenchRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/api/v1/business/:customUrl/menu", server.GetMenuByBusinessCustomUrl)
	return r
}

// seedRealisticMenu replaces the empty categories JSON on perf-bench-001's
// menu row with one category containing 50 menu items. This gives benches
// (a) realistic payload sizes for menu reads and (b) a stable
// menu_item_id ("perf-bench-001-item-NNN") that the order-create bench can
// reference for server-side price resolution.
func seedRealisticMenu(b *testing.B) {
	b.Helper()
	items := make([]database.MenuItem, 50)
	for i := 0; i < 50; i++ {
		items[i] = database.MenuItem{
			ID:          fmt.Sprintf("perf-bench-001-item-%03d", i+1),
			Name:        fmt.Sprintf("Item %d", i+1),
			Description: "A realistic-sized menu item description used for the perf bench.",
			Price:       5.00 + float64(i)*0.5,
			Currency:    "USD",
			IsAvailable: true,
			SortOrder:   i,
		}
	}
	categories := []database.MenuCategory{{
		ID:        "perf-bench-001-cat-1",
		Name:      "Mains",
		SortOrder: 0,
		Items:     items,
	}}
	raw, err := json.Marshal(categories)
	if err != nil {
		b.Fatalf("marshal categories: %v", err)
	}
	db := database.GetDB()
	if err := db.Exec(`
        UPDATE menus SET categories = ?
        WHERE business_id = (SELECT id FROM businesses WHERE business_id = 'perf-bench-001')
    `, string(raw)).Error; err != nil {
		b.Fatalf("update menu: %v", err)
	}
}

// BenchmarkMenuGetWithItems mirrors BenchmarkMenuGet but seeds 50 menu items
// into the menu's categories JSON BEFORE the timed loop, so the alloc + decode
// cost reflects a realistically-sized menu rather than an empty one.
func BenchmarkMenuGetWithItems(b *testing.B) {
	setupServerBenchmarkDB(b)
	seedRealisticMenu(b)

	router := buildMenuBenchRouter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/business/perf-bench-001/menu", nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}
