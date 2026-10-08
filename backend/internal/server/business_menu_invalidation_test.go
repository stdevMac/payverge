package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupMenuInvalidationDB wires an in-memory SQLite DB with the tables needed
// by the menu and offer/bundle write handlers. Called once per test; the
// database.RegisterOnDBChange hook automatically clears the pricing cache when
// SetTestDB fires.
func setupMenuInvalidationDB(t *testing.T) {
	t.Helper()
	disableAsyncOnboardingStampForTest(t)
	disableAsyncMenuCategoryTranslationForTest(t)
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
	))
}

func disableAsyncOnboardingStampForTest(t *testing.T) {
	t.Helper()
	previous := runStampOnboardingAsync
	runStampOnboardingAsync = func(func()) {}
	t.Cleanup(func() {
		runStampOnboardingAsync = previous
	})
}

func disableAsyncMenuCategoryTranslationForTest(t *testing.T) {
	t.Helper()
	previous := runMenuCategoryTranslationAsync
	runMenuCategoryTranslationAsync = func(func()) {}
	t.Cleanup(func() {
		runMenuCategoryTranslationAsync = previous
	})
}

// createMenuInvalidationBusiness creates a minimal Business row in the test DB.
func createMenuInvalidationBusiness(t *testing.T, slug string) *database.Business {
	t.Helper()
	biz := &database.Business{
		BusinessId:      slug + "-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Name:            slug,
		OwnerAddress:    "0xowner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	return biz
}

// seedMenuWithItem creates a Menu row with one category + one item for bizID.
func seedMenuWithItem(t *testing.T, bizID uint, itemID, catID string, price float64) {
	t.Helper()
	item := database.MenuItem{
		ID:          itemID,
		Name:        "Original Item",
		Price:       price,
		Currency:    "USD",
		IsAvailable: true,
	}
	cat := database.MenuCategory{
		ID:    catID,
		Name:  "Mains",
		Items: []database.MenuItem{item},
	}
	raw, err := json.Marshal([]database.MenuCategory{cat})
	require.NoError(t, err)
	menu := &database.Menu{
		BusinessID: bizID,
		Categories: string(raw),
		IsActive:   true,
		Version:    1,
	}
	require.NoError(t, database.GetDB().Create(menu).Error)
}

// primeCache injects a stale snapshot into the pricing cache so subsequent
// reads would be served from cache without hitting the DB.
func primeCache(t *testing.T, bizID uint) {
	t.Helper()
	// Use ResetPricingCache-then-prime so prior test state is clean.
	services.ResetPricingCache()
	// Prime via MenuDataForBusiness: it reads the DB and stores the snapshot.
	// We just need the entry to exist so we can assert it's evicted.
	// Direct prime via exported helper avoids needing to expose the mutex.
	// Instead call MenuDataForBusiness which writes into the cache.
	_, _, _, _, err := services.MenuDataForBusiness(bizID)
	require.NoError(t, err, "primeCache: MenuDataForBusiness failed")
}

// TestUpdateMenuItem_InvalidatesPricingCache asserts that after a successful
// UpdateMenuItem handler call, the pricing cache entry for that business is
// evicted so the next read returns the updated menu (not the stale snapshot).
//
// RED: before the fix, the handler does not call InvalidatePricingCache, so
// reading MenuDataForBusiness within the 5s TTL returns the old (stale) price.
// GREEN: after the fix, InvalidatePricingCache is called, the snapshot is gone,
// and MenuDataForBusiness re-fetches and returns the new price.
func TestUpdateMenuItem_InvalidatesPricingCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-invalidation")
	const itemID = "item-001"
	const catID = "cat-001"
	const oldPrice = 10.0
	const newPrice = 25.0
	seedMenuWithItem(t, biz.ID, itemID, catID, oldPrice)

	// Prime: reads DB and caches the snapshot (price = oldPrice).
	primeCache(t, biz.ID)

	// Verify the cache is primed: reading MenuDataForBusiness now returns old menu.
	_, cats, _, _, err := services.MenuDataForBusiness(biz.ID)
	require.NoError(t, err)
	require.Len(t, cats, 1)
	require.Len(t, cats[0].Items, 1)
	assert.Equal(t, oldPrice, cats[0].Items[0].Price, "cache should be primed with old price")

	// Now call UpdateMenuItem (legacy index-based path) via the handler
	// directly using a Gin test context — simulating the write path.
	updatedItem := database.MenuItem{
		ID:          itemID,
		Name:        "Updated Item",
		Price:       newPrice,
		Currency:    "USD",
		IsAvailable: true,
	}
	body, err := json.Marshal(UpdateMenuItemRequest{
		CategoryIndex: 0,
		ItemIndex:     0,
		Item:          updatedItem,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/businesses/%d/menu/items", biz.ID), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
	// Wire requireBusinessAccess: it reads address + business owner.
	c.Set("address", "0xowner")

	UpdateMenuItem(c)

	require.Equal(t, http.StatusOK, w.Code, "UpdateMenuItem handler should succeed: %s", w.Body.String())

	// The key assertion: after the write, MenuDataForBusiness must return the
	// NEW price even though we are still inside the original 5-second TTL window.
	// If the cache was NOT invalidated, we'd still see oldPrice (stale snapshot).
	_, cats2, _, _, err := services.MenuDataForBusiness(biz.ID)
	require.NoError(t, err)
	require.Len(t, cats2, 1)
	require.Len(t, cats2[0].Items, 1)
	assert.Equal(t, newPrice, cats2[0].Items[0].Price,
		"after UpdateMenuItem, pricing cache must be invalidated so the new price "+
			"(%.2f) is visible immediately — got %.2f (stale snapshot still cached)",
		newPrice, cats2[0].Items[0].Price)
}

// TestCreateOffer_InvalidatesPricingCache asserts that CreateOffer evicts the
// pricing snapshot for the business. Offers are loaded by getPricingSnapshot
// (via LoadActivePromotionsForBusiness) so a stale cache would serve old offers.
func TestCreateOffer_InvalidatesPricingCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "offer-invalidation")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	// Prime the cache before the offer write.
	primeCache(t, biz.ID)

	// Call CreateOffer handler via test context.
	reqBody := OfferRequest{
		Name:          "Lunch Deal",
		Description:   "20% off",
		DiscountType:  "percentage",
		DiscountValue: 20.0,
		ApplicableTo:  "all",
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/businesses/%d/offers", biz.ID), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Set("address", "0xowner")

	CreateOffer(c)

	require.Equal(t, http.StatusCreated, w.Code, "CreateOffer handler should succeed: %s", w.Body.String())

	// After CreateOffer the cache must be invalidated: we can observe this by
	// checking that the offers slice returned by MenuDataForBusiness includes the
	// newly created offer. If stale, the empty offer list would be returned.
	_, _, offers, _, err := services.MenuDataForBusiness(biz.ID)
	require.NoError(t, err)
	assert.Len(t, offers, 1,
		"after CreateOffer, pricing cache must be invalidated so the new offer is visible — got %d offers (stale snapshot still cached)",
		len(offers))
}
