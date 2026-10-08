package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// callGetMenu drives the operator GetMenu handler for bizID and returns the raw
// response body as a generic map so access-shape assertions can inspect exactly
// which keys were serialized.
func callGetMenu(t *testing.T, bizID uint, language string) (map[string]json.RawMessage, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	url := fmt.Sprintf("/businesses/%d/menu", bizID)
	if language != "" {
		url += "?language=" + language
	}
	c.Request = httptest.NewRequest(http.MethodGet, url, nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", bizID)}}
	c.Set("address", "0xowner")

	GetMenu(c)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw), "response body: %s", w.Body.String())
	return raw, w
}

// TestGetMenu_WireContractOmitsRawCategoriesAndBusiness is the §3.7 access-shape
// regression: the operator GET /menu response must NOT ship
//   - the raw "categories" JSON string (a second, redundant copy of the menu), and
//   - the ~200-field zero-value embedded "business" object (struct omitempty is a
//     no-op, so it used to serialize a full blank Business).
//
// The frontend consumes only "parsed_categories" (verified: parseMenuCategories
// prefers it), so both are dead weight on the largest operator payload.
func TestGetMenu_WireContractOmitsRawCategoriesAndBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-wire-contract")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	raw, w := callGetMenu(t, biz.ID, "")
	require.Equal(t, http.StatusOK, w.Code, "GetMenu should succeed: %s", w.Body.String())

	// The raw categories JSON string must be gone.
	if v, ok := raw["categories"]; ok {
		t.Errorf("operator GET /menu still ships redundant raw \"categories\" string: %s", v)
	}

	// The zero-value embedded Business must be gone.
	if v, ok := raw["business"]; ok {
		t.Errorf("operator GET /menu still ships embedded zero-value \"business\" blob: %s", v)
	}

	// parsed_categories must still be present and correct.
	require.Contains(t, raw, "parsed_categories", "parsed_categories must be present")
	var cats []database.MenuCategory
	require.NoError(t, json.Unmarshal(raw["parsed_categories"], &cats))
	require.Len(t, cats, 1)
	require.Len(t, cats[0].Items, 1)
	assert.Equal(t, "cat-001", cats[0].ID)

	// version, language, item_orderability, and menu metadata must survive.
	require.Contains(t, raw, "version")
	require.Contains(t, raw, "language")
	require.Contains(t, raw, "item_orderability")
	require.Contains(t, raw, "id")
	require.Contains(t, raw, "business_id")
	require.Contains(t, raw, "is_active")

	var version uint
	require.NoError(t, json.Unmarshal(raw["version"], &version))
	assert.Equal(t, uint(1), version)
}

// TestGetMenu_EmptyMenuShapeUnchanged confirms the not-found empty-menu branch
// still returns the legacy shape (categories: []), which parseMenuCategories
// handles, so a brand-new business with no menu row is unaffected by the slim.
func TestGetMenu_EmptyMenuShapeUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-wire-empty")

	raw, w := callGetMenu(t, biz.ID, "")
	require.Equal(t, http.StatusOK, w.Code, "GetMenu empty should succeed: %s", w.Body.String())
	require.Contains(t, raw, "categories", "empty-menu branch keeps categories:[] for legacy compat")
}

// seedLargeMenu writes a realistic 20-category × 30-item menu to bizID's active
// menu row so the payload benchmark reflects a large operator menu.
func seedLargeMenuForBench(tb testing.TB, bizID uint) {
	tb.Helper()
	cats := make([]database.MenuCategory, 20)
	for ci := range cats {
		items := make([]database.MenuItem, 30)
		for ii := range items {
			items[ii] = database.MenuItem{
				ID:          fmt.Sprintf("cat-%02d-item-%02d", ci, ii),
				Name:        fmt.Sprintf("Item %d-%d", ci, ii),
				Description: "A realistic-sized menu item description used for the payload bench measurement.",
				Price:       5.00 + float64(ii)*0.5,
				Currency:    "USD",
				IsAvailable: true,
				SortOrder:   ii,
			}
		}
		cats[ci] = database.MenuCategory{
			ID:        fmt.Sprintf("cat-%02d", ci),
			Name:      fmt.Sprintf("Category %d", ci),
			SortOrder: ci,
			Items:     items,
		}
	}
	raw, err := json.Marshal(cats)
	require.NoError(tb, err)
	require.NoError(tb, database.GetDB().Create(&database.Menu{
		BusinessID: bizID,
		Categories: string(raw),
		IsActive:   true,
		Version:    1,
	}).Error)
}

// BenchmarkGetMenuPayload measures the serialized operator GET /menu payload
// size (bytes/op) and allocations for a 20×30 menu on an in-memory SQLite DB.
// Deterministic microbench per CLAUDE.md — records the before/after payload
// bytes for the §3.7 wire-contract slim (drop the raw categories string + the
// zero-value embedded Business).
func BenchmarkGetMenuPayload(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	prev := runStampOnboardingAsync
	runStampOnboardingAsync = func(func()) {}
	b.Cleanup(func() { runStampOnboardingAsync = prev })

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(b, err)
	sqlDB, err := gormDB.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	require.NoError(b, gormDB.AutoMigrate(&database.Business{}, &database.Menu{}))

	biz := &database.Business{
		BusinessId:      "menu-payload-bench",
		Name:            "menu-payload-bench",
		OwnerAddress:    "0xowner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		IsActive:        true,
	}
	require.NoError(b, database.GetDB().Create(biz).Error)
	seedLargeMenuForBench(b, biz.ID)

	b.ReportAllocs()
	b.ResetTimer()
	var lastLen int
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/businesses/%d/menu", biz.ID), nil)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
		c.Set("address", "0xowner")
		GetMenu(c)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d: %s", w.Code, w.Body.String())
		}
		lastLen = w.Body.Len()
	}
	b.StopTimer()
	b.ReportMetric(float64(lastLen), "payload-bytes")
}
