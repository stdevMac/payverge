package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/s3"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type snapshotCaptureWaiterProvider struct {
	request llm.GenerateRequest
	resp    *llm.Response
	calls   int
}

func init() {
	productionLoader := loadUnrecommendableMenuItemIDs
	loadUnrecommendableMenuItemIDs = func(businessID uint) (map[string]bool, error) {
		hidden, err := productionLoader(businessID)
		db := database.GetDB()
		if err != nil && db != nil && db.Dialector != nil && db.Dialector.Name() == "sqlite" &&
			strings.Contains(err.Error(), "no such table: inventory_settings") {
			return map[string]bool{}, nil
		}
		return hidden, err
	}
}

func (provider *snapshotCaptureWaiterProvider) Generate(_ context.Context, request llm.GenerateRequest) (*llm.Response, error) {
	provider.calls++
	provider.request = request
	return provider.resp, nil
}

func translatedWaiterMenuSnapshotFixture(t *testing.T) (*database.Business, []database.MenuCategory, []database.Offer, []database.Bundle) {
	t.Helper()
	previousDB := database.GetDB()
	dsn := fmt.Sprintf("file:waiter-menu-snapshot-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(db)
	t.Cleanup(func() {
		database.SetTestDB(previousDB)
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&database.Translation{}))

	const businessID = uint(42)
	business := &database.Business{
		ID: businessID, DefaultLanguage: "en", IsActive: true, KitchenEnabled: true, OrdersEnabled: true,
	}
	categories := []database.MenuCategory{
		{
			ID: "mains", Name: "Mains", SortOrder: 20,
			Items: []database.MenuItem{
				{
					ID: "550e8400-e29b-41d4-a716-446655440000", Name: "Harvest Bowl", Description: "Greens and grains",
					Price: 18.5, Currency: "USD", Images: []string{"https://cdn.payverge.test/harvest.jpg"},
					Allergens: []string{"Dairy"}, DietaryTags: []string{"Vegetarian"}, IsAvailable: true, SortOrder: 2,
				},
				{ID: "7", Name: "House Soup", Price: 9, Currency: "USD", Image: "https://cdn.payverge.test/soup.jpg", IsAvailable: true, SortOrder: 1},
				{ID: "manual-off", Name: "Seasonal Tart", Price: 12, Currency: "USD", IsAvailable: false, SortOrder: 3},
				{ID: "operator-hidden", Name: "Secret Supper", Price: 50, Currency: "USD", IsAvailable: true, SortOrder: 4},
				{ID: "depleted", Name: "Sold Out Steak", Price: 42, Currency: "USD", IsAvailable: true, SortOrder: 5},
			},
		},
		{ID: "starters", Name: "Starters", SortOrder: 10, Items: []database.MenuItem{{ID: "bread", Name: "Bread", Price: 5, IsAvailable: true}}},
	}

	seed := func(entityType string, entityID uint, field, original, translated string) {
		t.Helper()
		require.NoError(t, db.Create(&database.Translation{
			BusinessID: businessID, EntityType: entityType, EntityID: entityID,
			FieldName: field, LanguageCode: "es-AR", OriginalText: original, TranslatedText: translated,
		}).Error)
	}
	seed("category", 0, "name", "Mains", "Principales")
	seed("menu_item", 0, "name", "Harvest Bowl", "Bowl de la cosecha")
	seed("menu_item", 0, "description", "Greens and grains", "Hojas verdes y granos")
	seed("allergen", 0, "name", "Dairy", "Lácteos")
	seed("offer", 7, "name", "Soup savings", "Oferta de sopa")
	seed("bundle", 7, "name", "Lunch duo", "Dúo de almuerzo")

	translatedCategories, _ := applyTranslationsToMenu(businessID, categories, "es-AR")
	targetSoup := "7"
	targetHidden := "operator-hidden"
	offers, _ := applyTranslationsToOffers([]database.Offer{
		{ID: 7, BusinessID: businessID, Name: "Soup savings", Description: "Save 10%", DiscountType: "percentage", DiscountValue: 10, ApplicableTo: "item", TargetID: &targetSoup, IsActive: true},
		{ID: 8, BusinessID: businessID, Name: "Hidden supper deal", DiscountType: "fixed", DiscountValue: 5, ApplicableTo: "item", TargetID: &targetHidden, IsActive: true},
		{ID: 9, BusinessID: 999, Name: "Foreign offer", DiscountType: "fixed", DiscountValue: 99, ApplicableTo: "all", IsActive: true},
	}, "es-AR")
	bundleItems := func(ids ...string) string {
		t.Helper()
		refs := make([]database.BundleItemRef, 0, len(ids))
		for _, id := range ids {
			refs = append(refs, database.BundleItemRef{MenuItemID: id, Quantity: 1})
		}
		encoded, err := json.Marshal(refs)
		require.NoError(t, err)
		return string(encoded)
	}
	bundles, _ := applyTranslationsToBundles([]database.Bundle{
		{ID: 7, BusinessID: businessID, Name: "Lunch duo", Price: 24, Currency: "USD", Image: "https://cdn.payverge.test/lunch.jpg", Items: bundleItems("7", "550e8400-e29b-41d4-a716-446655440000"), IsActive: true},
		{ID: 8, BusinessID: businessID, Name: "Secret bundle", Price: 30, Items: bundleItems("operator-hidden"), IsActive: true},
		{ID: 10, BusinessID: businessID, Name: "Seasonal bundle", Price: 20, Items: bundleItems("manual-off"), IsActive: true},
		{ID: 11, BusinessID: businessID, Name: "Malformed bundle", Price: 20, Items: "not-json", IsActive: true},
		{ID: 12, BusinessID: 999, Name: "Foreign bundle", Price: 20, Items: bundleItems("7"), IsActive: true},
	}, "es-AR")

	return business, translatedCategories, offers, bundles
}

func waiterFixtureHiddenIDs() map[string]bool {
	return map[string]bool{"operator-hidden": true}
}

func waiterFixtureSoldOutIDs() map[string]bool {
	return map[string]bool{"depleted": true}
}

func buildOpenWaiterSnapshotFixture(t *testing.T) WaiterMenuSnapshot {
	t.Helper()
	business, categories, offers, bundles := translatedWaiterMenuSnapshotFixture(t)
	snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business: business, Locale: " es-ar ", Mode: "ordering", BusinessOpen: true,
		Categories: categories, Offers: offers, Bundles: bundles,
		HiddenItemIDs: waiterFixtureHiddenIDs(), SoldOutItemIDs: waiterFixtureSoldOutIDs(),
		TrustedImageHost: "cdn.payverge.test",
	})
	require.NoError(t, err)
	return snapshot
}

func TestBuildWaiterMenuSnapshot(t *testing.T) {
	t.Run("preserves localized stable identities and excludes hidden entities", func(t *testing.T) {
		snapshot := buildOpenWaiterSnapshotFixture(t)
		require.Equal(t, uint(42), snapshot.BusinessID)
		require.Equal(t, "es-AR", snapshot.Locale)
		require.Equal(t, "ordering", snapshot.Mode)
		require.True(t, snapshot.OrderingOpen)

		require.Len(t, snapshot.Categories, 2)
		require.Equal(t, "starters", snapshot.Categories[0].ID)
		require.Equal(t, "Principales", snapshot.Categories[1].DisplayName)

		bowlKey := WaiterMenuEntityKey{Type: "menu_item", ID: "550e8400-e29b-41d4-a716-446655440000"}
		bowl, ok := snapshot.ByKey[bowlKey]
		require.True(t, ok)
		require.Equal(t, bowlKey.ID, bowl.ID, "UUID identity must round trip without coercion")
		require.Equal(t, "Bowl de la cosecha", bowl.DisplayName)
		require.Equal(t, "https://cdn.payverge.test/harvest.jpg", bowl.ImageURL)
		require.Equal(t, []string{"Lácteos"}, bowl.Allergens)
		require.True(t, bowl.Available)
		require.True(t, bowl.Orderable)

		hiddenKey := WaiterMenuEntityKey{Type: "menu_item", ID: "operator-hidden"}
		require.NotContains(t, snapshot.ByKey, hiddenKey)
		require.NotContains(t, snapshot.RecommendableByKey, hiddenKey)
		require.NotContains(t, snapshot.OrderableByKey, hiddenKey)

		soldOutKey := WaiterMenuEntityKey{Type: "menu_item", ID: "depleted"}
		soldOut, ok := snapshot.ByKey[soldOutKey]
		require.True(t, ok, "86'd items remain visible so the waiter can say they are sold out")
		require.Equal(t, "Sold Out Steak", soldOut.DisplayName)
		require.False(t, soldOut.Available)
		require.False(t, soldOut.Orderable)
		require.NotContains(t, snapshot.RecommendableByKey, soldOutKey)
		require.NotContains(t, snapshot.OrderableByKey, soldOutKey)
		require.NotContains(t, snapshot.ByKey, WaiterMenuEntityKey{Type: "bundle", ID: "8"})
		require.NotContains(t, snapshot.ByKey, WaiterMenuEntityKey{Type: "bundle", ID: "11"})
		require.NotContains(t, snapshot.ByKey, WaiterMenuEntityKey{Type: "bundle", ID: "12"})
		require.NotContains(t, snapshot.ByKey, WaiterMenuEntityKey{Type: "offer", ID: "8"})
		require.NotContains(t, snapshot.ByKey, WaiterMenuEntityKey{Type: "offer", ID: "9"})

		manualKey := WaiterMenuEntityKey{Type: "menu_item", ID: "manual-off"}
		manual, ok := snapshot.ByKey[manualKey]
		require.True(t, ok, "manual-unavailable items remain visible for honest explanations")
		require.False(t, manual.Available)
		require.False(t, manual.Orderable)
		require.NotContains(t, snapshot.RecommendableByKey, manualKey)
		require.NotContains(t, snapshot.OrderableByKey, manualKey)

		seasonalBundleKey := WaiterMenuEntityKey{Type: "bundle", ID: "10"}
		seasonalBundle := snapshot.ByKey[seasonalBundleKey]
		require.False(t, seasonalBundle.Available)
		require.False(t, seasonalBundle.Orderable)
		require.NotContains(t, snapshot.RecommendableByKey, seasonalBundleKey)
	})

	t.Run("namespaces equal textual ids by entity type and indexes localized names", func(t *testing.T) {
		snapshot := buildOpenWaiterSnapshotFixture(t)
		for _, entityType := range []string{"menu_item", "bundle", "offer"} {
			key := WaiterMenuEntityKey{Type: entityType, ID: "7"}
			require.Contains(t, snapshot.ByKey, key, "same textual IDs must remain distinct across types")
		}
		require.Equal(t,
			[]WaiterMenuEntityKey{{Type: "menu_item", ID: "550e8400-e29b-41d4-a716-446655440000"}},
			snapshot.ByNormalizedName["bowl de la cosecha"],
		)
	})

	t.Run("serializes the canonical safe projection and returns defensive copies", func(t *testing.T) {
		business, categories, offers, bundles := translatedWaiterMenuSnapshotFixture(t)
		snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
			Business: business, Locale: "es-AR", Mode: "ordering", BusinessOpen: true,
			Categories: categories, Offers: offers, Bundles: bundles,
			HiddenItemIDs: waiterFixtureHiddenIDs(), SoldOutItemIDs: waiterFixtureSoldOutIDs(),
			TrustedImageHost: "cdn.payverge.test",
		})
		require.NoError(t, err)

		categories[0].Items[0].Name = "MUTATED INPUT"
		categories[0].Items[0].Allergens[0] = "MUTATED ALLERGEN"
		offers[0].Name = "MUTATED OFFER"
		bundles[0].Name = "MUTATED BUNDLE"

		menuJSON := snapshot.PromptMenuJSON()
		offersJSON := snapshot.PromptOffersJSON()
		bundlesJSON := snapshot.PromptBundlesJSON()
		require.Contains(t, menuJSON, "Bowl de la cosecha")
		require.Contains(t, menuJSON, "Seasonal Tart", "manual-unavailable items remain explainable")
		require.NotContains(t, menuJSON, "Secret Supper")
		require.Contains(t, menuJSON, "Sold Out Steak", "86'd items stay in the prompt as unavailable")
		require.NotContains(t, menuJSON, "business_id")
		require.Contains(t, offersJSON, "Oferta de sopa")
		require.NotContains(t, offersJSON, "Foreign offer")
		require.Contains(t, bundlesJSON, "Dúo de almuerzo")
		require.NotContains(t, bundlesJSON, "Foreign bundle")
		for _, mutated := range []string{"MUTATED INPUT", "MUTATED ALLERGEN", "MUTATED OFFER", "MUTATED BUNDLE"} {
			require.NotContains(t, menuJSON+offersJSON+bundlesJSON, mutated)
		}

		visible := snapshot.VisibleMenuCategories()
		require.NotEmpty(t, visible)
		visible[0].Name = "MUTATED COPY"
		require.NotEqual(t, "MUTATED COPY", snapshot.VisibleMenuCategories()[0].Name)

		orderable := snapshot.OrderableMenuCategories()
		for _, category := range orderable {
			for _, item := range category.Items {
				require.True(t, item.IsAvailable)
				require.NotEqual(t, "manual-off", item.ID)
			}
		}
	})

	t.Run("lapsed subscription closes ordering like kitchen-off", func(t *testing.T) {
		business, categories, offers, bundles := translatedWaiterMenuSnapshotFixture(t)
		business.IsActive = false
		snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
			Business: business, Locale: "es-AR", Mode: "ordering", BusinessOpen: true,
			Categories: categories, Offers: offers, Bundles: bundles,
			HiddenItemIDs: waiterFixtureHiddenIDs(), SoldOutItemIDs: waiterFixtureSoldOutIDs(),
		})
		require.NoError(t, err)
		require.False(t, snapshot.OrderingOpen)
		require.Empty(t, snapshot.OrderableByKey)
		require.Empty(t, snapshot.OrderableMenuCategories())
	})

	t.Run("closed and non-ordering modes keep facts visible but expose no orderable identities", func(t *testing.T) {
		for _, tc := range []struct {
			name         string
			mode         string
			businessOpen bool
		}{
			{name: "closed ordering", mode: "ordering", businessOpen: false},
			{name: "concierge browse", mode: "concierge", businessOpen: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				business, categories, offers, bundles := translatedWaiterMenuSnapshotFixture(t)
				snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
					Business: business, Locale: "es-AR", Mode: tc.mode, BusinessOpen: tc.businessOpen,
					Categories: categories, Offers: offers, Bundles: bundles,
					HiddenItemIDs: waiterFixtureHiddenIDs(), SoldOutItemIDs: waiterFixtureSoldOutIDs(),
				})
				require.NoError(t, err)
				require.False(t, snapshot.OrderingOpen)
				require.Empty(t, snapshot.OrderableByKey)
				require.Empty(t, snapshot.OrderableMenuCategories())
				bowl := snapshot.ByKey[WaiterMenuEntityKey{Type: "menu_item", ID: "550e8400-e29b-41d4-a716-446655440000"}]
				require.True(t, bowl.Available)
				require.False(t, bowl.Orderable)
				require.Contains(t, snapshot.PromptMenuJSON(), "Bowl de la cosecha")
			})
		}
	})

	t.Run("canonicalizes bundle refs from localized stable items", func(t *testing.T) {
		business := &database.Business{ID: 42, DefaultLanguage: "es", IsActive: true, KitchenEnabled: true, OrdersEnabled: true}
		bundleItems, err := json.Marshal([]database.BundleItemRef{{
			MenuItemID: " item-1 ", Name: "Stale source name", Quantity: 2,
		}})
		require.NoError(t, err)
		snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
			Business: business, Locale: "es", Mode: "ordering", BusinessOpen: true,
			Categories: []database.MenuCategory{{
				ID: "mains", Name: "Principales",
				Items: []database.MenuItem{{ID: "item-1", Name: "Nombre localizado", IsAvailable: true}},
			}},
			Bundles: []database.Bundle{{
				ID: 5, BusinessID: 42, Name: "Dúo", Items: string(bundleItems), IsActive: true,
			}},
		})
		require.NoError(t, err)
		require.JSONEq(t, `[{"id":5,"name":"Dúo","price":0,"items":[{"menu_item_id":"item-1","name":"Nombre localizado","quantity":2}],"available":true,"orderable":true}]`, snapshot.PromptBundlesJSON())
		require.NotContains(t, snapshot.PromptBundlesJSON(), "Stale source name")
	})

	t.Run("keeps inactive promotions explainable without recommending them", func(t *testing.T) {
		business := &database.Business{ID: 42, DefaultLanguage: "en", IsActive: true, KitchenEnabled: true, OrdersEnabled: true}
		itemID := "off"
		categoryID := "mains"
		bundleID := "6"
		bundleItems, err := json.Marshal([]database.BundleItemRef{{MenuItemID: itemID, Quantity: 1}})
		require.NoError(t, err)
		snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
			Business: business, Locale: "en", Mode: "ordering", BusinessOpen: true,
			Categories: []database.MenuCategory{{
				ID: categoryID, Name: "Mains",
				Items: []database.MenuItem{{ID: itemID, Name: "Unavailable plate", IsAvailable: false}},
			}},
			Bundles: []database.Bundle{{ID: 6, BusinessID: 42, Name: "Unavailable duo", Items: string(bundleItems), IsActive: true}},
			Offers: []database.Offer{
				{ID: 1, BusinessID: 42, Name: "Whole-menu deal", ApplicableTo: "all", IsActive: true},
				{ID: 2, BusinessID: 42, Name: "Category deal", ApplicableTo: "category", TargetID: &categoryID, IsActive: true},
				{ID: 3, BusinessID: 42, Name: "Bundle deal", ApplicableTo: "bundle", TargetID: &bundleID, IsActive: true},
			},
		})
		require.NoError(t, err)
		for _, key := range []WaiterMenuEntityKey{
			{Type: "bundle", ID: bundleID},
			{Type: "offer", ID: "1"},
			{Type: "offer", ID: "2"},
			{Type: "offer", ID: "3"},
		} {
			entity, ok := snapshot.ByKey[key]
			require.True(t, ok, "%v remains visible for an honest unavailable explanation", key)
			require.False(t, entity.Available, "%v cannot advertise availability without an available component", key)
			require.False(t, entity.Orderable)
			require.NotContains(t, snapshot.RecommendableByKey, key)
			require.NotContains(t, snapshot.OrderableByKey, key)
		}
	})

	t.Run("fails closed without a scoped business", func(t *testing.T) {
		_, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{Locale: "es-AR", Mode: "ordering", BusinessOpen: true})
		require.ErrorContains(t, err, "business")
		_, err = BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{Business: &database.Business{}, Locale: "es-AR", Mode: "ordering", BusinessOpen: true})
		require.ErrorContains(t, err, "business")
	})

	t.Run("keeps anonymous legacy allergen facts outside trusted identity maps", func(t *testing.T) {
		business := &database.Business{ID: 42, DefaultLanguage: "en", IsActive: true, KitchenEnabled: true, OrdersEnabled: true}
		snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
			Business: business, Locale: "en", Mode: "ordering", BusinessOpen: true,
			Categories: []database.MenuCategory{{
				Name:  "Mains",
				Items: []database.MenuItem{{Name: "Peanut Burger", Allergens: []string{"peanuts"}, IsAvailable: true}},
			}},
		})
		require.NoError(t, err)
		require.Empty(t, snapshot.ByKey, "blank database identities must never become trusted menu entities")
		require.Empty(t, snapshot.RecommendableByKey)
		require.Empty(t, snapshot.OrderableByKey)
		require.NotContains(t, snapshot.PromptMenuJSON(), "Peanut Burger", "anonymous legacy rows must not enter model grounding")

		allergenFacts := snapshot.AllergenMenuCategories()
		require.Len(t, allergenFacts, 1)
		require.Len(t, allergenFacts[0].Items, 1)
		require.Equal(t, "Peanut Burger", allergenFacts[0].Items[0].Name)
		require.Equal(t, []string{"peanuts"}, allergenFacts[0].Items[0].Allergens)
	})
}

func TestBuildWaiterMenuSnapshot_DeterministicAcrossInputOrdering(t *testing.T) {
	business, categories, offers, bundles := translatedWaiterMenuSnapshotFixture(t)
	first, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business: business, Locale: "es-AR", Mode: "ordering", BusinessOpen: true,
		Categories: categories, Offers: offers, Bundles: bundles,
		HiddenItemIDs: waiterFixtureHiddenIDs(), SoldOutItemIDs: waiterFixtureSoldOutIDs(),
	})
	require.NoError(t, err)

	for left, right := 0, len(offers)-1; left < right; left, right = left+1, right-1 {
		offers[left], offers[right] = offers[right], offers[left]
	}
	for left, right := 0, len(bundles)-1; left < right; left, right = left+1, right-1 {
		bundles[left], bundles[right] = bundles[right], bundles[left]
	}
	for _, category := range categories {
		for left, right := 0, len(category.Items)-1; left < right; left, right = left+1, right-1 {
			category.Items[left], category.Items[right] = category.Items[right], category.Items[left]
		}
	}
	for left, right := 0, len(categories)-1; left < right; left, right = left+1, right-1 {
		categories[left], categories[right] = categories[right], categories[left]
	}
	second, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business: business, Locale: "ES-ar", Mode: "ordering", BusinessOpen: true,
		Categories: categories, Offers: offers, Bundles: bundles,
		HiddenItemIDs: waiterFixtureHiddenIDs(), SoldOutItemIDs: waiterFixtureSoldOutIDs(),
	})
	require.NoError(t, err)

	assert.Equal(t, first.Categories, second.Categories)
	assert.Equal(t, first.Entities, second.Entities)
	assert.Equal(t, first.AllergenMenuCategories(), second.AllergenMenuCategories())
	assert.JSONEq(t, first.PromptOffersJSON(), second.PromptOffersJSON())
	assert.JSONEq(t, first.PromptBundlesJSON(), second.PromptBundlesJSON())
}

func TestBuildWaiterMenuSnapshot_RejectsDuplicateNormalizedCategoryIDs(t *testing.T) {
	business := &database.Business{ID: 42, DefaultLanguage: "en", IsActive: true, KitchenEnabled: true, OrdersEnabled: true}
	categoryID := "mains"
	categories := []database.MenuCategory{
		{ID: " mains ", Name: "Unavailable mains", Items: []database.MenuItem{{ID: "off", Name: "Unavailable", IsAvailable: false}}},
		{ID: "mains", Name: "Available mains", Items: []database.MenuItem{{ID: "on", Name: "Available", IsAvailable: true}}},
	}

	for _, input := range [][]database.MenuCategory{categories, {categories[1], categories[0]}} {
		_, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
			Business: business, Locale: "en", Mode: "ordering", BusinessOpen: true,
			Categories: input,
			Offers: []database.Offer{{
				ID: 1, BusinessID: business.ID, Name: "Mains offer", ApplicableTo: "category", TargetID: &categoryID, IsActive: true,
			}},
		})
		require.EqualError(t, err, "duplicate waiter menu category mains")
	}
}

func TestBuildWaiterMenuSnapshot_SanitizesEveryImageProjection(t *testing.T) {
	business := &database.Business{ID: 42, DefaultLanguage: "en", IsActive: true, KitchenEnabled: true, OrdersEnabled: true}
	refs, err := json.Marshal([]database.BundleItemRef{{MenuItemID: "item-1", Quantity: 1}})
	require.NoError(t, err)
	snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business: business, Locale: "en", Mode: "ordering", BusinessOpen: true,
		TrustedImageHost: "images.payverge.io",
		Categories: []database.MenuCategory{{
			ID: "mains", Name: "Mains", Items: []database.MenuItem{{
				ID: "item-1", Name: "Safe dish", IsAvailable: true,
				Image:            "javascript:alert('item')",
				Images:           []string{" https://images.payverge.io/menu/safe.jpg ", "data:image/svg+xml,bad", "https://evil.example/item.jpg", ":// malformed"},
				CompositionImage: "https://prot-images.payverge.io/menu/composition.jpg",
			}},
		}},
		Bundles: []database.Bundle{
			{ID: 1, BusinessID: business.ID, Name: "Safe bundle", Image: " https://images.payverge.io/bundles/safe.jpg ", Items: string(refs), IsActive: true},
			{ID: 2, BusinessID: business.ID, Name: "Unsafe bundle", Image: "https://foreign.example/bundle.jpg", Items: string(refs), IsActive: true},
		},
		Offers: []database.Offer{{
			ID: 1, BusinessID: business.ID, Name: "Unsafe offer", Image: "data:image/png;base64,bad", ApplicableTo: "all", IsActive: true,
		}},
	})
	require.NoError(t, err)

	require.Equal(t, "https://images.payverge.io/menu/safe.jpg", snapshot.ByKey[WaiterMenuEntityKey{Type: "menu_item", ID: "item-1"}].ImageURL)
	require.Equal(t, "https://images.payverge.io/bundles/safe.jpg", snapshot.ByKey[WaiterMenuEntityKey{Type: "bundle", ID: "1"}].ImageURL)
	require.Empty(t, snapshot.ByKey[WaiterMenuEntityKey{Type: "bundle", ID: "2"}].ImageURL)
	require.Empty(t, snapshot.ByKey[WaiterMenuEntityKey{Type: "offer", ID: "1"}].ImageURL)
	visibleItem := snapshot.VisibleMenuCategories()[0].Items[0]
	require.Empty(t, visibleItem.Image)
	require.Empty(t, visibleItem.CompositionImage)
	require.Equal(t, []string{"https://images.payverge.io/menu/safe.jpg"}, visibleItem.Images)

	projection := snapshot.PromptMenuJSON() + snapshot.PromptBundlesJSON() + snapshot.PromptOffersJSON()
	for _, unsafe := range []string{"javascript:", "data:image", "evil.example", "foreign.example", "prot-images.payverge.io", ":// malformed"} {
		require.NotContains(t, projection, unsafe)
	}
	require.Contains(t, projection, "https://images.payverge.io/menu/safe.jpg")
	require.Contains(t, projection, "https://images.payverge.io/bundles/safe.jpg")
	for _, key := range []WaiterMenuEntityKey{
		{Type: "menu_item", ID: "item-1"},
		{Type: "bundle", ID: "1"},
		{Type: "bundle", ID: "2"},
		{Type: "offer", ID: "1"},
	} {
		require.Contains(t, snapshot.ByKey, key, "unsafe image metadata must not drop the valid entity")
	}
}

func TestBuildWaiterMenuSnapshot_InventoryGroundingErrorFailsClosedBeforeModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "snapshot-inventory-failure", true)
	table := createAIWaiterTable(t, business.ID, "SNAPSHOT-INVENTORY-FAILURE")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	categories := []database.MenuCategory{{
		ID: "mains", Name: "Mains", Items: []database.MenuItem{{
			ID: "secret-item", Name: "Secret Inventory Item", Price: 12, Currency: "USD", IsAvailable: true,
		}},
	}}
	encoded, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: string(encoded), IsActive: true, Version: 1}).Error)
	previousLoader := loadUnrecommendableMenuItemIDs
	loadUnrecommendableMenuItemIDs = func(uint) (map[string]bool, error) {
		return nil, errors.New("inventory grounding unavailable")
	}
	t.Cleanup(func() { loadUnrecommendableMenuItemIDs = previousLoader })

	provider := &snapshotCaptureWaiterProvider{resp: &llm.Response{ToolCalls: []llm.ToolCall{{
		ID: "call-secret", Name: "add_to_cart", Args: map[string]any{"item_name": "Secret Inventory Item", "quantity": float64(1)},
	}}}}
	aiService, err := services.NewAIService(provider, llm.ModelConfig{
		Chat: "test-chat", Image: "test-image", Director: "test-director", Menu: "test-menu",
	})
	require.NoError(t, err)
	SetAIService(aiService)
	t.Cleanup(func() { SetAIService(nil) })

	conversation := createAIWaiterConversation(t, business.ID, "snapshot-inventory-failure-session")
	conversation.TableCode = table.TableCode
	require.NoError(t, db.Save(conversation).Error)

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	response := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "snapshot-inventory-failure-session", "mode": "ordering", "table_code": table.TableCode,
		"language": "en", "history": []map[string]any{{"role": "user", "content": "Add the secret item"}},
	})

	require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
	require.Zero(t, provider.calls, "the model must not see a menu whose stock grounding failed")
	require.Empty(t, provider.request.System)
	require.NotContains(t, response.Body.String(), "Secret Inventory Item")
	require.NotContains(t, response.Body.String(), "functionCall")
}

func TestBuildWaiterMenuSnapshot_HandlerUsesCanonicalOrderableProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "snapshot-handler", true)
	table := createAIWaiterTable(t, business.ID, "SNAPSHOT-TABLE")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	categories := []database.MenuCategory{{
		ID: "desserts", Name: "Desserts",
		Items: []database.MenuItem{{
			ID: "seasonal-tart", Name: "Seasonal Tart", Price: 12, Currency: "USD",
			Allergens: []string{"dairy"}, IsAvailable: false,
		}},
	}}
	encoded, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID, Categories: string(encoded), IsActive: true, Version: 1,
	}).Error)

	provider := &snapshotCaptureWaiterProvider{resp: &llm.Response{ToolCalls: []llm.ToolCall{{
		ID: "call-unavailable", Name: "add_to_cart",
		Args: map[string]any{"item_name": "Seasonal Tart", "quantity": float64(1)},
	}}}}
	aiService, err := services.NewAIService(provider, llm.ModelConfig{
		Chat: "test-chat", Image: "test-image", Director: "test-director", Menu: "test-menu",
	})
	require.NoError(t, err)
	SetAIService(aiService)
	t.Cleanup(func() { SetAIService(nil) })

	conversation := createAIWaiterConversation(t, business.ID, "snapshot-handler-session")
	conversation.TableCode = table.TableCode
	require.NoError(t, db.Save(conversation).Error)

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	response := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "snapshot-handler-session", "mode": "ordering", "table_code": table.TableCode,
		"language": "en", "history": []map[string]any{{"role": "user", "content": "Add the seasonal tart"}},
	})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var body struct {
		Parts []struct {
			Text         string          `json:"text"`
			FunctionCall json.RawMessage `json:"functionCall"`
		} `json:"parts"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Parts, 1)
	require.Empty(t, body.Parts[0].FunctionCall, "manual-unavailable identity must not reach cart validation")
	require.Contains(t, strings.ToLower(body.Parts[0].Text), "seasonal tart")
	require.Contains(t, strings.ToLower(body.Parts[0].Text), "unavailable")
	require.NotEqual(t, services.ClarifyItemMessage("en"), body.Parts[0].Text)
	require.NotContains(t, response.Body.String(), "functionCall")
}

// waiterDiscoverySnapshotFixture builds a translated snapshot whose display
// names differ from the stored (source) names, so alias resolution and dietary
// filtering are both exercised. Prices are float64 DOLLARS.
func waiterDiscoverySnapshotFixture(t *testing.T) WaiterMenuSnapshot {
	t.Helper()
	source := []database.MenuCategory{
		{
			ID: "mains", Name: "Mains", SortOrder: 1,
			Items: []database.MenuItem{
				{ID: "harvest-bowl", Name: "Harvest Bowl", Price: 14, IsAvailable: true, DietaryTags: []string{"Vegetarian"}, SortOrder: 1},
				{ID: "steak-plate", Name: "Steak Plate", Price: 29, IsAvailable: false, SortOrder: 2},
				{ID: "garden-tart", Name: "Garden Tart", Price: 13, IsAvailable: true, DietaryTags: []string{"vegetarian"}, SortOrder: 3},
			},
		},
		{
			ID: "drinks", Name: "Drinks", SortOrder: 2,
			Items: []database.MenuItem{{ID: "cider", Name: "House Cider", Price: 7, IsAvailable: true, SortOrder: 1}},
		},
	}
	// The guest-facing (translated) menu: display names differ, and translation
	// has localized the dietary tag away from the canonical DIETARY_TAGS id.
	display := []database.MenuCategory{
		{
			ID: "mains", Name: "Principales", SortOrder: 1,
			Items: []database.MenuItem{
				{ID: "harvest-bowl", Name: "Bowl de la cosecha", Price: 14, IsAvailable: true, DietaryTags: []string{"vegetariano"}, SortOrder: 1},
				{ID: "steak-plate", Name: "Plato de bife", Price: 29, IsAvailable: false, SortOrder: 2},
				{ID: "garden-tart", Name: "Tarta de la huerta", Price: 13, IsAvailable: true, DietaryTags: []string{"vegetariano"}, SortOrder: 3},
			},
		},
		{
			ID: "drinks", Name: "Bebidas", SortOrder: 2,
			Items: []database.MenuItem{{ID: "cider", Name: "Sidra de la casa", Price: 7, IsAvailable: true, SortOrder: 1}},
		},
	}
	snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business:   &database.Business{ID: 85, DefaultLanguage: "en", IsActive: true, KitchenEnabled: true, OrdersEnabled: true},
		Locale:     "es-AR",
		Mode:       "concierge",
		Categories: display, SourceCategories: source, HiddenItemIDs: map[string]bool{},
	})
	require.NoError(t, err)
	return snapshot
}

// #577: a guest naming a dish in the menu's source language on a translated
// menu must still resolve to canonical menu identity.
func TestWaiterMenuSnapshot_MatchEntitiesInTextResolvesSourceNameAliases(t *testing.T) {
	snapshot := waiterDiscoverySnapshotFixture(t)

	localized := snapshot.MatchEntitiesInText("¿Qué va bien con el Bowl de la cosecha?")
	require.Len(t, localized, 1)
	assert.Equal(t, "harvest-bowl", localized[0].ID)

	aliased := snapshot.MatchEntitiesInText("Que me conseillez-vous avec le Harvest Bowl ?")
	require.Len(t, aliased, 1)
	assert.Equal(t, "harvest-bowl", aliased[0].ID)
	assert.Equal(t, "Bowl de la cosecha", aliased[0].DisplayName, "the localized display name stays canonical")

	assert.Empty(t, snapshot.MatchEntitiesInText("Do you have a house salad?"))
	assert.Empty(t, snapshot.MatchEntitiesInText(""))

	steak := snapshot.MatchEntitiesInText("el steak esta?")
	require.Len(t, steak, 1)
	assert.Equal(t, "steak-plate", steak[0].ID)
}

func TestWaiterMenuSnapshot_MatchEntitiesInTextResolvesDateNightPrefix(t *testing.T) {
	snapshot := waiterSpeechSnapshot(t, "en", "concierge")
	matched := snapshot.MatchEntitiesInText("What's Date Night?")
	require.Len(t, matched, 1)
	assert.Contains(t, strings.ToLower(matched[0].DisplayName), "date night")
}

// #577: recommendation retrieval must return the business's own available menu
// items — an empty result here is the exact production failure.
func TestWaiterMenuSnapshot_RecommendableEntitiesReturnsAvailableMenuItems(t *testing.T) {
	snapshot := waiterDiscoverySnapshotFixture(t)

	all := snapshot.RecommendableEntities(nil, 0)
	require.NotEmpty(t, all, "recommendation retrieval must not return an empty menu")
	ids := make([]string, 0, len(all))
	for _, entity := range all {
		ids = append(ids, entity.ID)
		assert.True(t, entity.Available, "%s must be available", entity.ID)
		assert.NotEqual(t, waiterMenuEntityTypeOffer, entity.Type, "a discount is not a dish")
	}
	assert.Equal(t, []string{"harvest-bowl", "garden-tart", "cider"}, ids, "snapshot order is deterministic")

	assert.Len(t, snapshot.RecommendableEntities(nil, 2), 2, "limit is honored")
}

// #577 (ES-AR): dietary filtering reads the canonical DIETARY_TAGS id from the
// SOURCE menu, so a translated tag ("vegetariano") does not silently drop the
// dish out of a "¿qué plato vegetariano...?" answer.
func TestWaiterMenuSnapshot_RecommendableEntitiesFiltersByCanonicalDietaryTags(t *testing.T) {
	snapshot := waiterDiscoverySnapshotFixture(t)

	vegetarian := snapshot.RecommendableEntities([]string{"vegetarian"}, 0)
	ids := make([]string, 0, len(vegetarian))
	for _, entity := range vegetarian {
		ids = append(ids, entity.ID)
	}
	assert.Equal(t, []string{"harvest-bowl", "garden-tart"}, ids)

	glutenFree := snapshot.RecommendableEntities([]string{"gluten-free"}, 0)
	ids = make([]string, 0, len(glutenFree))
	for _, entity := range glutenFree {
		ids = append(ids, entity.ID)
	}
	assert.Contains(t, ids, "cider", "untagged dishes that do not list gluten are a safe fallback")
	assert.NotContains(t, ids, "steak-plate", "unavailable dishes stay out of recommendations")
}

// On the local storage driver menu images are same-origin "/media/<key>" URLs
// and there is no trusted CDN host; the waiter must keep them (and still drop
// foreign, protected or malformed ones).
func TestSanitizeWaiterMenuImageURLAcceptsOwnMedia(t *testing.T) {
	restore := s3.SetPublicURL("https://pos.example.test")
	defer restore()

	for _, keep := range []string{
		"/media/businesses/42/menu-item/0123456789abcdef_20260101_120000.png",
		"https://pos.example.test/media/demo-arg/assets/carta/flan.jpg",
		" /media/businesses/42/gallery/a.webp ",
	} {
		require.Equal(t, strings.TrimSpace(keep), sanitizeWaiterMenuImageURL(keep, ""), keep)
	}
	require.Equal(t, "https://cdn.payverge.test/harvest.jpg",
		sanitizeWaiterMenuImageURL("https://cdn.payverge.test/harvest.jpg", "cdn.payverge.test"))

	for _, drop := range []string{
		"https://evil.test/media/businesses/42/a.png",
		"//evil.test/media/a.png",
		"/media/../etc/passwd",
		"/media/fiscal-receipts/42/receipt-1.pdf",
		"/media/businesses/42/contracts/nda.pdf",
		"/media/a.png?x=1",
		"javascript:alert(1)",
		"http://cdn.payverge.test/harvest.jpg",
		"",
	} {
		require.Empty(t, sanitizeWaiterMenuImageURL(drop, "cdn.payverge.test"), drop)
	}
}
