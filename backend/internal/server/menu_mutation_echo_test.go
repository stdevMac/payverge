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

// menuMutationResponse mirrors the enriched mutation response: message + version
// plus the mutated entity so the client can patch state in place instead of
// re-downloading the whole menu after every add/update/delete (§3.7 fix 3).
type menuMutationResponse struct {
	Message              string                               `json:"message"`
	Version              uint                                 `json:"version"`
	RequiresConfirmation bool                                 `json:"requires_confirmation"`
	Category             *database.MenuCategory               `json:"category,omitempty"`
	Item                 *database.MenuItem                   `json:"item,omitempty"`
	Sanitization         services.GeneratedMenuSanitizeReport `json:"sanitization"`
}

func doJSON(t *testing.T, method, url string, params gin.Params, body any, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, url, bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	c.Set("address", "0xowner")
	handler(c)
	return w
}

// TestAddMenuItem_RejectsBlankName is FIND-031: empty/zero-value POSTs used to
// create nameless $0 rows under category_index 0 because MenuItem has no gin
// binding tags. Operators and API clients must get a clear 400 instead.
func TestAddMenuItem_RejectsBlankName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-add-blank-name")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	// Completely empty body historically defaulted category_index=0 and wrote "".
	wEmpty := doJSON(t, http.MethodPost, fmt.Sprintf("/businesses/%d/menu/items", biz.ID),
		gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}, map[string]any{}, AddMenuItem)
	require.Equal(t, http.StatusBadRequest, wEmpty.Code, "empty body: %s", wEmpty.Body.String())
	assert.Contains(t, wEmpty.Body.String(), "name is required")
	assert.Contains(t, wEmpty.Body.String(), "VALIDATION_INVALID_INPUT")

	version := uint(1)
	wBlank := doJSON(t, http.MethodPost, fmt.Sprintf("/businesses/%d/menu/items", biz.ID),
		gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}, AddMenuItemRequest{
			CategoryID: "cat-001",
			Item:       database.MenuItem{Name: "   ", Price: 5},
			Version:    &version,
		}, AddMenuItem)
	require.Equal(t, http.StatusBadRequest, wBlank.Code, "whitespace name: %s", wBlank.Body.String())
	assert.Contains(t, wBlank.Body.String(), "name is required")

	wNeg := doJSON(t, http.MethodPost, fmt.Sprintf("/businesses/%d/menu/items", biz.ID),
		gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}, AddMenuItemRequest{
			CategoryID: "cat-001",
			Item:       database.MenuItem{Name: "Bad Price", Price: -1},
			Version:    &version,
		}, AddMenuItem)
	require.Equal(t, http.StatusBadRequest, wNeg.Code, "negative price: %s", wNeg.Body.String())
	assert.Contains(t, wNeg.Body.String(), "negative")
}

// TestAddMenuItem_ReturnsMutatedItemAndVersion is the §3.7 fix-3 regression: an
// ID-based AddMenuItem must echo back the created item (with the server-assigned
// UUID) and the bumped version so the client patches in place — no full-menu
// reload. It must also NOT require a second full-document read to compute the
// version (fix 8).
func TestAddMenuItem_ReturnsMutatedItemAndVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-add-item-echo")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	version := uint(1)
	reqBody := AddMenuItemRequest{
		CategoryID: "cat-001",
		Item: database.MenuItem{
			Name:        "New Taco",
			Price:       12.5,
			Currency:    "USD",
			IsAvailable: true,
		},
		Version: &version,
	}
	w := doJSON(t, http.MethodPost, fmt.Sprintf("/businesses/%d/menu/items", biz.ID),
		gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}, reqBody, AddMenuItem)

	require.Equal(t, http.StatusCreated, w.Code, "AddMenuItem should succeed: %s", w.Body.String())

	var resp menuMutationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, uint(2), resp.Version, "version must be bumped and returned")
	require.NotNil(t, resp.Item, "response must echo the created item so the client can patch in place")
	assert.Equal(t, "New Taco", resp.Item.Name)
	assert.NotEmpty(t, resp.Item.ID, "created item must carry the server-assigned ID")
}

// TestUpdateMenuItem_ReturnsMutatedItemAndVersion asserts the ID-based
// UpdateMenuItem echoes the mutated item + bumped version.
func TestUpdateMenuItem_ReturnsMutatedItemAndVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-update-item-echo")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	version := uint(1)
	reqBody := UpdateMenuItemRequest{
		CategoryID: "cat-001",
		ItemID:     "item-001",
		Item: database.MenuItem{
			Name:        "Renamed Item",
			Price:       9.0,
			Currency:    "USD",
			IsAvailable: true,
		},
		Version: &version,
	}
	w := doJSON(t, http.MethodPut, fmt.Sprintf("/businesses/%d/menu/items", biz.ID),
		gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}, reqBody, UpdateMenuItem)

	require.Equal(t, http.StatusOK, w.Code, "UpdateMenuItem should succeed: %s", w.Body.String())

	var resp menuMutationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, uint(2), resp.Version)
	require.NotNil(t, resp.Item)
	assert.Equal(t, "Renamed Item", resp.Item.Name)
	assert.Equal(t, "item-001", resp.Item.ID, "updated item must preserve its ID")
}

func TestManualMenuItemMutationsUseSharedSanitizerAndReturnReport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-item-shared-sanitizer")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	version := uint(1)
	addRequest := AddMenuItemRequest{
		CategoryID: "cat-001",
		Item: database.MenuItem{
			Name: "Alias Taco", Price: 12.5,
			Allergens:   []string{"peanuts", "mystery"},
			DietaryTags: []string{"gluten_free", "keto"},
		},
		Version: &version,
	}
	review := doJSON(t, http.MethodPost, fmt.Sprintf("/businesses/%d/menu/items", biz.ID),
		gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}, AddMenuItemRequest{
			CategoryID: addRequest.CategoryID,
			Item:       addRequest.Item,
			Version:    addRequest.Version,
		}, AddMenuItem)
	require.Equal(t, http.StatusOK, review.Code, review.Body.String())
	var reviewResp menuMutationResponse
	require.NoError(t, json.Unmarshal(review.Body.Bytes(), &reviewResp))
	assert.True(t, reviewResp.RequiresConfirmation)
	assert.Nil(t, reviewResp.Item)
	_, categories, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	require.Len(t, categories[0].Items, 1, "review response must not mutate the menu")

	addRequest.ConfirmSanitization = true
	add := doJSON(t, http.MethodPost, fmt.Sprintf("/businesses/%d/menu/items", biz.ID),
		gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}, addRequest, AddMenuItem)
	require.Equal(t, http.StatusCreated, add.Code, add.Body.String())

	var addResp menuMutationResponse
	require.NoError(t, json.Unmarshal(add.Body.Bytes(), &addResp))
	require.NotNil(t, addResp.Item)
	assert.Equal(t, []string{"peanut"}, addResp.Item.Allergens)
	assert.Equal(t, []string{"gluten-free"}, addResp.Item.DietaryTags)
	assert.Equal(t, 1, addResp.Sanitization.DroppedAllergens)
	assert.Equal(t, 1, addResp.Sanitization.DroppedDietaryTags)
	assert.NotEmpty(t, addResp.Sanitization.Retained)
	assert.NotEmpty(t, addResp.Sanitization.Dropped)
	assert.False(t, addResp.RequiresConfirmation)

	version = addResp.Version
	updateRequest := UpdateMenuItemRequest{
		CategoryID: "cat-001", ItemID: "item-001",
		Item: database.MenuItem{
			Name: "Updated Taco", Price: 11,
			Allergens:   []string{"soy", "unknown"},
			DietaryTags: []string{"dairy_free", "paleo"},
		},
		Version: &version,
	}
	updateReview := doJSON(t, http.MethodPut, fmt.Sprintf("/businesses/%d/menu/items", biz.ID),
		gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}, updateRequest, UpdateMenuItem)
	require.Equal(t, http.StatusOK, updateReview.Code, updateReview.Body.String())
	require.NoError(t, json.Unmarshal(updateReview.Body.Bytes(), &reviewResp))
	assert.True(t, reviewResp.RequiresConfirmation)
	_, categories, err = database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	assert.Equal(t, "Original Item", categories[0].Items[0].Name, "review response must not update the item")

	updateRequest.ConfirmSanitization = true
	update := doJSON(t, http.MethodPut, fmt.Sprintf("/businesses/%d/menu/items", biz.ID),
		gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}, updateRequest, UpdateMenuItem)
	require.Equal(t, http.StatusOK, update.Code, update.Body.String())

	var updateResp menuMutationResponse
	require.NoError(t, json.Unmarshal(update.Body.Bytes(), &updateResp))
	require.NotNil(t, updateResp.Item)
	assert.Equal(t, []string{"soya"}, updateResp.Item.Allergens)
	assert.Equal(t, []string{"dairy-free"}, updateResp.Item.DietaryTags)
	assert.Equal(t, 1, updateResp.Sanitization.DroppedAllergens)
	assert.Equal(t, 1, updateResp.Sanitization.DroppedDietaryTags)
	assert.NotEmpty(t, updateResp.Sanitization.Retained)
	assert.NotEmpty(t, updateResp.Sanitization.Dropped)
	assert.False(t, updateResp.RequiresConfirmation)
}

// TestAddMenuCategory_ReturnsMutatedCategoryAndVersion asserts the versioned
// AddMenuCategory echoes the created category (with server-assigned ID).
func TestAddMenuCategory_ReturnsMutatedCategoryAndVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-add-cat-echo")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	version := uint(1)
	reqBody := AddCategoryRequest{
		Name: "Desserts",
		Items: []database.MenuItem{{
			Name: "Cake", Price: 8,
			Allergens:   []string{"nuts", "unknown"},
			DietaryTags: []string{"dairy_free", "paleo"},
		}},
		Version: &version,
	}
	review := doJSON(t, http.MethodPost, fmt.Sprintf("/businesses/%d/menu/categories", biz.ID),
		gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}, reqBody, AddMenuCategory)
	require.Equal(t, http.StatusOK, review.Code, review.Body.String())
	var reviewResp menuMutationResponse
	require.NoError(t, json.Unmarshal(review.Body.Bytes(), &reviewResp))
	assert.True(t, reviewResp.RequiresConfirmation)
	_, categories, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	require.Len(t, categories, 1, "review response must not add the category")

	reqBody.ConfirmSanitization = true
	w := doJSON(t, http.MethodPost, fmt.Sprintf("/businesses/%d/menu/categories", biz.ID),
		gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}, reqBody, AddMenuCategory)
	require.Equal(t, http.StatusCreated, w.Code, "AddMenuCategory should succeed: %s", w.Body.String())

	var resp menuMutationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, uint(2), resp.Version)
	require.NotNil(t, resp.Category, "response must echo the created category")
	assert.Equal(t, "Desserts", resp.Category.Name)
	assert.NotEmpty(t, resp.Category.ID, "created category must carry a server-assigned ID")
	require.Len(t, resp.Category.Items, 1)
	assert.Equal(t, []string{"treenuts"}, resp.Category.Items[0].Allergens)
	assert.Equal(t, []string{"dairy-free"}, resp.Category.Items[0].DietaryTags)
	assert.Equal(t, 1, resp.Sanitization.DroppedAllergens)
	assert.Equal(t, 1, resp.Sanitization.DroppedDietaryTags)
	assert.False(t, resp.RequiresConfirmation)
}

func TestUpdateMenuCategoryRequiresSanitizationConfirmationBeforeMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-update-cat-sanitization")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	version := uint(1)
	reqBody := UpdateCategoryRequest{
		Name: "Updated Mains",
		Items: []database.MenuItem{{
			ID: "item-001", Name: "Updated Item", Price: 12,
			Allergens:   []string{"peanuts", "unknown"},
			DietaryTags: []string{"gluten_free", "paleo"},
		}},
		Version: &version,
	}
	params := gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", biz.ID)},
		{Key: "category_id", Value: "cat-001"},
	}

	review := doJSON(t, http.MethodPut, fmt.Sprintf("/businesses/%d/menu/categories/cat-001", biz.ID),
		params, reqBody, UpdateMenuCategory)
	require.Equal(t, http.StatusOK, review.Code, review.Body.String())
	var reviewResp menuMutationResponse
	require.NoError(t, json.Unmarshal(review.Body.Bytes(), &reviewResp))
	assert.True(t, reviewResp.RequiresConfirmation)
	assert.Nil(t, reviewResp.Category)
	_, categories, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	assert.Equal(t, "Mains", categories[0].Name, "review response must not update the category")
	assert.Equal(t, "Original Item", categories[0].Items[0].Name, "review response must not update nested items")

	reqBody.ConfirmSanitization = true
	updated := doJSON(t, http.MethodPut, fmt.Sprintf("/businesses/%d/menu/categories/cat-001", biz.ID),
		params, reqBody, UpdateMenuCategory)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())

	var resp menuMutationResponse
	require.NoError(t, json.Unmarshal(updated.Body.Bytes(), &resp))
	require.NotNil(t, resp.Category)
	assert.Equal(t, "Updated Mains", resp.Category.Name)
	require.Len(t, resp.Category.Items, 1)
	assert.Equal(t, []string{"peanut"}, resp.Category.Items[0].Allergens)
	assert.Equal(t, []string{"gluten-free"}, resp.Category.Items[0].DietaryTags)
	assert.Equal(t, 1, resp.Sanitization.DroppedAllergens)
	assert.Equal(t, 1, resp.Sanitization.DroppedDietaryTags)
	assert.False(t, resp.RequiresConfirmation)
}
