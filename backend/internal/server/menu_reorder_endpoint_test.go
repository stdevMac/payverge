package server

import (
	"bytes"
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
)

func callReorder(t *testing.T, bizID uint, body ReorderMenuRequest) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/businesses/%d/menu/reorder", bizID), bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", bizID)}}
	c.Set("address", "0xowner")
	ReorderMenu(c)
	return w
}

// seedTwoCategoriesTwoItems seeds a menu with 2 categories, the first holding 2
// items, so category-level and item-level reorders both have something to move.
func seedTwoCategoriesTwoItems(t *testing.T, bizID uint) {
	t.Helper()
	cats := []database.MenuCategory{
		{ID: "cat-A", Name: "A", Items: []database.MenuItem{
			{ID: "item-1", Name: "One", Price: 1, Currency: "USD", IsAvailable: true},
			{ID: "item-2", Name: "Two", Price: 2, Currency: "USD", IsAvailable: true},
		}},
		{ID: "cat-B", Name: "B", Items: []database.MenuItem{}},
	}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: bizID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)
}

// TestReorderMenu_CategoryMove asserts a category move applies server-side and
// bumps the version — without the client uploading the whole menu.
func TestReorderMenu_CategoryMove(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-reorder-cat")
	seedTwoCategoriesTwoItems(t, biz.ID)

	// Move category at index 1 (cat-B) to index 0.
	w := callReorder(t, biz.ID, ReorderMenuRequest{Scope: "category", From: 1, To: 0, Version: 1})
	require.Equal(t, http.StatusOK, w.Code, "reorder should succeed: %s", w.Body.String())

	var resp struct {
		Version uint `json:"version"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, uint(2), resp.Version)

	_, cats, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	require.Len(t, cats, 2)
	assert.Equal(t, "cat-B", cats[0].ID, "cat-B must now be first")
	assert.Equal(t, "cat-A", cats[1].ID)
}

// TestReorderMenu_ItemMove asserts an item move within a category applies.
func TestReorderMenu_ItemMove(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-reorder-item")
	seedTwoCategoriesTwoItems(t, biz.ID)

	// Move item at index 1 (item-2) to index 0 within cat-A.
	w := callReorder(t, biz.ID, ReorderMenuRequest{Scope: "item", CategoryID: "cat-A", From: 1, To: 0, Version: 1})
	require.Equal(t, http.StatusOK, w.Code, "item reorder should succeed: %s", w.Body.String())

	_, cats, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	require.Len(t, cats[0].Items, 2)
	assert.Equal(t, "item-2", cats[0].Items[0].ID, "item-2 must now be first")
	assert.Equal(t, "item-1", cats[0].Items[1].ID)
}

// TestReorderMenu_StaleVersionRejected asserts a reorder on a stale version is
// rejected with 409 (CAS) rather than clobbering a concurrent edit.
func TestReorderMenu_StaleVersionRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-reorder-stale")
	seedTwoCategoriesTwoItems(t, biz.ID)

	// Bump the version out-of-band to 2.
	_, err := database.ApplyMenuCategories(biz.ID, []database.MenuCategory{
		{ID: "cat-A", Name: "A"}, {ID: "cat-B", Name: "B"},
	}, 1)
	require.NoError(t, err)

	// A reorder posted with the stale version 1 must 409.
	w := callReorder(t, biz.ID, ReorderMenuRequest{Scope: "category", From: 0, To: 1, Version: 1})
	assert.Equal(t, http.StatusConflict, w.Code, "stale reorder must 409: %s", w.Body.String())
}

// TestReorderMenu_OutOfRange asserts an out-of-range index returns 400.
func TestReorderMenu_OutOfRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-reorder-oob")
	seedTwoCategoriesTwoItems(t, biz.ID)

	w := callReorder(t, biz.ID, ReorderMenuRequest{Scope: "category", From: 0, To: 9, Version: 1})
	assert.Equal(t, http.StatusBadRequest, w.Code, "out-of-range reorder must 400: %s", w.Body.String())
}
