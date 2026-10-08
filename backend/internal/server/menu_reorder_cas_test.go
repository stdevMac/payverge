package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callCreateMenu drives the CreateMenu handler (POST /menu — the whole-menu save
// used by drag-reorder) with the given categories and optional version, returning
// the recorder.
func callCreateMenu(t *testing.T, bizID uint, categories []database.MenuCategory, version *uint) *httptest.ResponseRecorder {
	t.Helper()
	return callCreateMenuRequest(t, bizID, CreateMenuRequest{Categories: categories, Version: version})
}

func callCreateMenuRequest(t *testing.T, bizID uint, request CreateMenuRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(request)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/businesses/%d/menu", bizID), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", bizID)}}
	c.Set("address", "0xowner")

	CreateMenu(c)
	return w
}

func TestCreateMenu_WholeDocumentDropsRequireConfirmationBeforeEveryWritePath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		seed       bool
		version    *uint
		statusCode int
	}{
		{name: "initial create", statusCode: http.StatusCreated},
		{name: "legacy update", seed: true, statusCode: http.StatusOK},
		{name: "versioned update", seed: true, version: func() *uint { value := uint(1); return &value }(), statusCode: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupMenuInvalidationDB(t)
			t.Cleanup(services.ResetPricingCache)
			biz := createMenuInvalidationBusiness(t, "whole-menu-confirm-"+strings.ReplaceAll(tt.name, " ", "-"))
			if tt.seed {
				seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10)
			}

			incoming := []database.MenuCategory{{
				ID: "cat-001", Name: "Changed", Items: []database.MenuItem{{
					ID: "item-001", Name: "Changed Item", Price: 12,
					Allergens: []string{"unknown-allergen"},
				}},
			}}
			review := callCreateMenuRequest(t, biz.ID, CreateMenuRequest{
				Categories: incoming, Version: tt.version,
			})
			require.Equal(t, http.StatusOK, review.Code, review.Body.String())
			var reviewResponse menuMutationResponse
			require.NoError(t, json.Unmarshal(review.Body.Bytes(), &reviewResponse))
			require.True(t, reviewResponse.RequiresConfirmation)
			require.Equal(t, 1, reviewResponse.Sanitization.DroppedAllergens)

			_, categories, loadErr := database.GetMenuByBusinessID(biz.ID)
			if tt.seed {
				require.NoError(t, loadErr)
				require.Equal(t, "Mains", categories[0].Name, "review response must not mutate the menu")
			} else {
				require.Error(t, loadErr, "review response must not create a menu")
			}

			confirmed := callCreateMenuRequest(t, biz.ID, CreateMenuRequest{
				Categories: incoming, Version: tt.version, ConfirmSanitization: true,
			})
			require.Equal(t, tt.statusCode, confirmed.Code, confirmed.Body.String())
			_, categories, loadErr = database.GetMenuByBusinessID(biz.ID)
			require.NoError(t, loadErr)
			require.Equal(t, "Changed", categories[0].Name)
			require.Empty(t, categories[0].Items[0].Allergens)
		})
	}
}

// TestCreateMenu_ReorderRejectsStaleVersion is the R3-MB-1 regression: the
// drag-reorder whole-menu save must go through optimistic CAS, so a reorder
// built on a stale version snapshot cannot silently overwrite a concurrent edit.
func TestCreateMenu_ReorderRejectsStaleVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-reorder-cas")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	// The menu is seeded at version 1. Simulate a concurrent writer bumping the
	// version out-of-band (e.g. another operator edited an item).
	_, err := database.ApplyMenuCategories(biz.ID, []database.MenuCategory{
		{ID: "cat-001", Name: "Mains", Items: []database.MenuItem{
			{ID: "item-001", Name: "Concurrent Edit", Price: 10.0, Currency: "USD", IsAvailable: true},
		}},
	}, 1)
	require.NoError(t, err, "out-of-band concurrent edit should apply")

	// A stale reorder snapshot posts with version=1 (the version the client last
	// saw), but the DB is now at version 2. It must be rejected with 409, not
	// clobber the concurrent edit.
	staleReorder := []database.MenuCategory{
		{ID: "cat-001", Name: "Mains", Items: []database.MenuItem{
			{ID: "item-001", Name: "Original Item", Price: 10.0, Currency: "USD", IsAvailable: true},
		}},
	}
	staleVersion := uint(1)
	w := callCreateMenu(t, biz.ID, staleReorder, &staleVersion)

	assert.Equal(t, http.StatusConflict, w.Code,
		"a reorder built on a stale version must be rejected (got %d: %s)", w.Code, w.Body.String())

	// The concurrent edit must survive — the stale reorder did not overwrite it.
	_, cats, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	require.Len(t, cats, 1)
	require.Len(t, cats[0].Items, 1)
	assert.Equal(t, "Concurrent Edit", cats[0].Items[0].Name,
		"the concurrent edit must survive; the stale reorder must not have clobbered it")
}

// TestCreateMenu_ReorderWithCurrentVersionSucceedsAndBumps confirms a reorder on
// the current version applies and returns the bumped version so the client can
// keep its optimistic-lock counter in sync (avoids a false 409 on the next edit).
func TestCreateMenu_ReorderWithCurrentVersionSucceedsAndBumps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)

	biz := createMenuInvalidationBusiness(t, "menu-reorder-ok")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)

	reorder := []database.MenuCategory{
		{ID: "cat-001", Name: "Mains", Items: []database.MenuItem{
			{ID: "item-001", Name: "Original Item", Price: 10.0, Currency: "USD", IsAvailable: true},
		}},
	}
	currentVersion := uint(1)
	w := callCreateMenu(t, biz.ID, reorder, &currentVersion)

	require.Equal(t, http.StatusOK, w.Code, "reorder on current version should succeed: %s", w.Body.String())

	var resp struct {
		Version uint `json:"version"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, uint(2), resp.Version, "handler must return the bumped version")

	_, cats, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	require.Len(t, cats, 1)
}
