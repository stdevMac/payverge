package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// L3-9: a client-supplied menu version must ALWAYS be enforced, on every
// addressing mode. The audited production interleave read the menu at version N,
// applied edit A (→ N+1), then submitted edit B with the stale version N using
// index addressing — and got 200, silently clobbering edit A, because the
// handlers only routed through the CAS path when ID-based addressing was also
// present. A version that is sent but ignored is worse than no version at all.

func menuVersionParams(bizID uint, extra ...gin.Param) gin.Params {
	params := gin.Params{{Key: "id", Value: fmt.Sprintf("%d", bizID)}}
	return append(params, extra...)
}

func currentMenuState(t *testing.T, bizID uint) (uint, []database.MenuCategory) {
	t.Helper()
	menu, cats, err := database.GetMenuByBusinessID(bizID)
	require.NoError(t, err)
	return menu.Version, cats
}

func TestUpdateMenuItem_StaleVersionWithoutIDsConflicts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-cas-item-put")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	// Edit A carries the current version (1) with index addressing and must win,
	// bumping the version exactly once.
	v1 := uint(1)
	editA := doJSON(t, http.MethodPut, fmt.Sprintf("/businesses/%d/menu/items", biz.ID),
		menuVersionParams(biz.ID), UpdateMenuItemRequest{
			CategoryIndex: 0,
			ItemIndex:     0,
			Item:          database.MenuItem{Name: "Edit A", Price: 11, Currency: "USD", IsAvailable: true},
			Version:       &v1,
		}, UpdateMenuItem)
	require.Equal(t, http.StatusOK, editA.Code, "current-version edit must win: %s", editA.Body.String())
	version, _ := currentMenuState(t, biz.ID)
	require.Equal(t, uint(2), version, "winning write must bump the version exactly once")

	// Edit B replays the now-stale version 1 (the audit's exact payload shape:
	// version present, no category_id/item_id) and must be rejected, not
	// silently applied over edit A.
	editB := doJSON(t, http.MethodPut, fmt.Sprintf("/businesses/%d/menu/items", biz.ID),
		menuVersionParams(biz.ID), UpdateMenuItemRequest{
			CategoryIndex: 0,
			ItemIndex:     0,
			Item:          database.MenuItem{Name: "Edit B", Price: 12, Currency: "USD", IsAvailable: true},
			Version:       &v1,
		}, UpdateMenuItem)
	require.Equal(t, http.StatusConflict, editB.Code,
		"stale-version PUT must 409, not clobber: %s", editB.Body.String())
	require.Contains(t, editB.Body.String(), "MENU_VERSION_CONFLICT")

	version, cats := currentMenuState(t, biz.ID)
	require.Equal(t, uint(2), version, "rejected write must not bump the version")
	require.Equal(t, "Edit A", cats[0].Items[0].Name, "edit A must survive the stale replay")
}

func TestAddMenuItem_StaleVersionWithoutCategoryIDConflicts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-cas-item-post")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	stale := uint(999)
	w := doJSON(t, http.MethodPost, fmt.Sprintf("/businesses/%d/menu/items", biz.ID),
		menuVersionParams(biz.ID), AddMenuItemRequest{
			CategoryIndex: 0,
			Item:          database.MenuItem{Name: "Straggler", Price: 5, Currency: "USD", IsAvailable: true},
			Version:       &stale,
		}, AddMenuItem)
	require.Equal(t, http.StatusConflict, w.Code,
		"stale-version POST must 409, not append blindly: %s", w.Body.String())

	_, cats := currentMenuState(t, biz.ID)
	require.Len(t, cats[0].Items, 1, "rejected add must not land")
}

func TestUpdateMenuCategory_IndexRouteHonorsVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-cas-cat-put")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)
	_, seeded := currentMenuState(t, biz.ID)

	// Index-addressed update carrying the CURRENT version must succeed (it used
	// to be misrouted into the ID-based branch with "0" as a UUID → 404).
	v1 := uint(1)
	ok := doJSON(t, http.MethodPut, fmt.Sprintf("/businesses/%d/menu/categories/0", biz.ID),
		menuVersionParams(biz.ID, gin.Param{Key: "category_index", Value: "0"}), UpdateCategoryRequest{
			Name:    "Renamed Mains",
			Items:   seeded[0].Items,
			Version: &v1,
		}, UpdateMenuCategory)
	require.Equal(t, http.StatusOK, ok.Code,
		"index-addressed update with current version must succeed: %s", ok.Body.String())
	version, cats := currentMenuState(t, biz.ID)
	require.Equal(t, uint(2), version)
	require.Equal(t, "Renamed Mains", cats[0].Name)

	// Replaying the stale version must 409.
	staleW := doJSON(t, http.MethodPut, fmt.Sprintf("/businesses/%d/menu/categories/0", biz.ID),
		menuVersionParams(biz.ID, gin.Param{Key: "category_index", Value: "0"}), UpdateCategoryRequest{
			Name:    "Stale Rename",
			Items:   seeded[0].Items,
			Version: &v1,
		}, UpdateMenuCategory)
	require.Equal(t, http.StatusConflict, staleW.Code,
		"stale-version category PUT must 409: %s", staleW.Body.String())
	_, cats = currentMenuState(t, biz.ID)
	require.Equal(t, "Renamed Mains", cats[0].Name, "winning rename must survive the stale replay")
}

func TestDeleteMenuItem_LegacyRouteHonorsVersionQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-cas-item-del")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/businesses/%d/menu/categories/0/items/0?version=999", biz.ID), nil)
	c.Params = menuVersionParams(biz.ID,
		gin.Param{Key: "category_index", Value: "0"},
		gin.Param{Key: "item_index", Value: "0"})
	c.Set("address", "0xowner")
	DeleteMenuItem(c)
	require.Equal(t, http.StatusConflict, w.Code,
		"stale ?version= on the legacy delete route must 409: %s", w.Body.String())

	_, cats := currentMenuState(t, biz.ID)
	require.Len(t, cats[0].Items, 1, "rejected delete must not remove the item")
}

func TestDeleteMenuCategory_LegacyRouteHonorsVersionQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-cas-cat-del")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/businesses/%d/menu/categories/0?version=999", biz.ID), nil)
	c.Params = menuVersionParams(biz.ID, gin.Param{Key: "category_index", Value: "0"})
	c.Set("address", "0xowner")
	DeleteMenuCategory(c)
	require.Equal(t, http.StatusConflict, w.Code,
		"stale ?version= on the legacy category delete must 409: %s", w.Body.String())

	_, cats := currentMenuState(t, biz.ID)
	require.Len(t, cats, 1, "rejected delete must not remove the category")
}
