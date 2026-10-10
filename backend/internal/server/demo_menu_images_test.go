package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/s3"
	"github.com/stdevmac/payverge/backend/internal/services"
)

const (
	demoImageTestOrigin   = "https://demo.example.test"
	demoImageTestOwn      = demoImageTestOrigin + "/media/demo-arg/assets/carta/provoleta.jpg"
	demoImageTestOutside  = "https://images.unsplash.com/photo-1?w=800"
	demoImageTestOutside2 = "https://lh3.googleusercontent.com/abc=s800"
)

func assertDemoImageRefusal(t *testing.T, code int, body []byte) {
	t.Helper()
	require.Equal(t, http.StatusForbidden, code, string(body))
	var resp struct {
		Code   string `json:"code"`
		Error  string `json:"error"`
		Params struct {
			Kind string `json:"kind"`
		} `json:"params"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, demomode.ErrorCode, resp.Code)
	assert.Equal(t, string(demomode.KindStorefront), resp.Params.Kind)
	assert.Contains(t, resp.Error, demoOutsideImageAction)
}

func TestDemoImageAllowed(t *testing.T) {
	t.Cleanup(s3.SetPublicURL(demoImageTestOrigin))
	stored := map[string]struct{}{demoImageTestOutside2: {}}

	for raw, want := range map[string]bool{
		"":                                    true,
		"   ":                                 true,
		demoImageTestOwn:                      true,
		"/media/demo-arg/assets/carta/x.jpg":  true,
		demoImageTestOutside2:                 true, // already stored: may be kept
		demoImageTestOutside:                  false,
		"https://evil.example/media/a.jpg":    false,
		"http://demo.example.test/../etc.png": false,
		"javascript:alert(1)":                 false,
	} {
		assert.Equal(t, want, demoImageAllowed(raw, stored), raw)
	}
}

func TestMenuWrites_DemoModeRefusesOutsideImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)
	t.Cleanup(s3.SetPublicURL(demoImageTestOrigin))
	config.SetDemoModeForTesting(t, true)

	biz := createMenuInvalidationBusiness(t, "demo-menu-images")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)
	params := gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
	version := uint(1)

	outsideItems := map[string]database.MenuItem{
		"image":             {Name: "Spam", Price: 5, Image: demoImageTestOutside},
		"images":            {Name: "Spam", Price: 5, Images: []string{demoImageTestOwn, demoImageTestOutside2}},
		"composition_image": {Name: "Spam", Price: 5, CompositionImage: demoImageTestOutside},
	}
	for field, item := range outsideItems {
		t.Run("add item "+field, func(t *testing.T) {
			w := doJSON(t, http.MethodPost, "/", params, AddMenuItemRequest{CategoryID: "cat-001", Item: item, Version: &version}, AddMenuItem)
			assertDemoImageRefusal(t, w.Code, w.Body.Bytes())
		})
		t.Run("update item "+field, func(t *testing.T) {
			w := doJSON(t, http.MethodPut, "/", params, UpdateMenuItemRequest{CategoryID: "cat-001", ItemID: "item-001", Item: item, Version: &version}, UpdateMenuItem)
			assertDemoImageRefusal(t, w.Code, w.Body.Bytes())
		})
	}

	t.Run("whole menu", func(t *testing.T) {
		w := doJSON(t, http.MethodPost, "/", params, CreateMenuRequest{Categories: []database.MenuCategory{{
			ID: "cat-001", Name: "Mains",
			Items: []database.MenuItem{{ID: "item-001", Name: "Original Item", Price: 10, Image: demoImageTestOutside}},
		}}}, CreateMenu)
		assertDemoImageRefusal(t, w.Code, w.Body.Bytes())
	})

	t.Run("add category", func(t *testing.T) {
		w := doJSON(t, http.MethodPost, "/", params, AddCategoryRequest{
			Name:  "Spam",
			Items: []database.MenuItem{{Name: "Spam", Price: 5, Image: demoImageTestOutside}},
		}, AddMenuCategory)
		assertDemoImageRefusal(t, w.Code, w.Body.Bytes())
	})

	t.Run("create offer", func(t *testing.T) {
		w := doJSON(t, http.MethodPost, "/", params, OfferRequest{
			Name: "Spam", DiscountType: "percentage", DiscountValue: 10, Image: demoImageTestOutside,
		}, CreateOffer)
		assertDemoImageRefusal(t, w.Code, w.Body.Bytes())
		var count int64
		require.NoError(t, database.GetDB().Model(&database.Offer{}).Count(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("create bundle", func(t *testing.T) {
		w := doJSON(t, http.MethodPost, "/", params, map[string]any{
			"name": "Spam", "price": 5, "image": demoImageTestOutside, "items": []any{},
		}, CreateBundle)
		assertDemoImageRefusal(t, w.Code, w.Body.Bytes())
	})

	// Nothing above reached the stored menu.
	_, categories, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	require.Len(t, categories, 1)
	require.Len(t, categories[0].Items, 1)
	assert.Empty(t, categories[0].Items[0].Image)
}

func TestMenuWrites_DemoModeKeepsOwnAndStoredImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)
	t.Cleanup(s3.SetPublicURL(demoImageTestOrigin))
	config.SetDemoModeForTesting(t, true)

	biz := createMenuInvalidationBusiness(t, "demo-menu-images-ok")
	params := gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
	// A stored menu whose photo is on an outside host (written before the
	// policy, or by a seed resolved under another origin).
	raw, err := json.Marshal([]database.MenuCategory{{
		ID: "cat-001", Name: "Mains",
		Items: []database.MenuItem{{ID: "item-001", Name: "Original Item", Price: 10, Currency: "USD", IsAvailable: true, Image: demoImageTestOutside2}},
	}})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{BusinessID: biz.ID, Categories: string(raw), IsActive: true, Version: 1}).Error)

	version := uint(1)
	w := doJSON(t, http.MethodPost, "/", params, AddMenuItemRequest{
		CategoryID: "cat-001",
		Item:       database.MenuItem{Name: "Provoleta", Price: 7, Currency: "USD", IsAvailable: true, Image: demoImageTestOwn},
		Version:    &version,
	}, AddMenuItem)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	version = 2
	w = doJSON(t, http.MethodPut, "/", params, UpdateMenuItemRequest{
		CategoryID: "cat-001",
		ItemID:     "item-001",
		Item:       database.MenuItem{Name: "Renamed", Price: 10, Currency: "USD", IsAvailable: true, Image: demoImageTestOutside2},
		Version:    &version,
	}, UpdateMenuItem)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestMenuWrites_OutsideImagesAllowedWithoutDemoMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupMenuInvalidationDB(t)
	t.Cleanup(services.ResetPricingCache)
	config.SetDemoModeForTesting(t, false)

	biz := createMenuInvalidationBusiness(t, "demo-menu-images-off")
	seedMenuWithItem(t, biz.ID, "item-001", "cat-001", 10.0)
	version := uint(1)
	w := doJSON(t, http.MethodPost, "/", gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}, AddMenuItemRequest{
		CategoryID: "cat-001",
		Item:       database.MenuItem{Name: "Taco", Price: 7, Currency: "USD", IsAvailable: true, Image: demoImageTestOutside},
		Version:    &version,
	}, AddMenuItem)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}
