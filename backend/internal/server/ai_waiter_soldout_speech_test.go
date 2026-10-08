package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waiterSpeechSnapshot(t *testing.T, locale, mode string) WaiterMenuSnapshot {
	t.Helper()
	business := &database.Business{
		ID: 86, DefaultLanguage: "en", IsActive: true, KitchenEnabled: true, OrdersEnabled: true,
		Name: "Payverge AI Pro Demo Lounge",
	}
	steakID := "demo-steak"
	dateNight, err := json.Marshal([]database.BundleItemRef{
		{MenuItemID: steakID, Name: "Steak Plate", Quantity: 1},
		{MenuItemID: "demo-spritz", Name: "Demo Spritz", Quantity: 2},
	})
	require.NoError(t, err)
	categories := []database.MenuCategory{
		{ID: "mains", Name: "Mains", SortOrder: 1, Items: []database.MenuItem{
			{ID: "quinoa-salad", Name: "Quinoa Salad", Price: 13, Currency: "USD", IsAvailable: true, DietaryTags: []string{"gluten-free", "vegetarian"}, SortOrder: 0},
			{ID: "harvest-bowl", Name: "Harvest Bowl", Price: 14, Currency: "USD", IsAvailable: true, Allergens: []string{"gluten", "sesame"}, DietaryTags: []string{"vegetarian"}, SortOrder: 1},
			{ID: steakID, Name: "Steak Plate", Price: 29, Currency: "USD", IsAvailable: true, SortOrder: 2},
			{ID: "market-tacos", Name: "Market Tacos", Price: 16, Currency: "USD", IsAvailable: true, Allergens: []string{"gluten"}, SortOrder: 3},
		}},
		{ID: "drinks", Name: "Drinks", SortOrder: 2, Items: []database.MenuItem{
			{ID: "iced-tea", Name: "Iced Tea", Price: 4, Currency: "USD", IsAvailable: true, SortOrder: 1},
			{ID: "demo-spritz", Name: "Demo Spritz", Price: 11, Currency: "USD", IsAvailable: true, SortOrder: 2},
		}},
		{ID: "desserts", Name: "Desserts", SortOrder: 3, Items: []database.MenuItem{
			{ID: "chocolate-tart", Name: "Chocolate Tart", Price: 9, Currency: "USD", IsAvailable: true, SortOrder: 1},
		}},
	}
	snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business: business, Locale: locale, Mode: mode, BusinessOpen: true,
		Categories: categories, SourceCategories: categories,
		Offers: []database.Offer{{
			ID: 5, BusinessID: 86, Name: "$5 Off the Steak Plate", DiscountType: "fixed",
			DiscountValue: 5, ApplicableTo: "item", TargetID: &steakID, IsActive: true,
		}},
		Bundles: []database.Bundle{{
			ID: 9, BusinessID: 86, Name: "Date Night for Two", Price: 48, Currency: "USD",
			Items: string(dateNight), IsActive: true,
		}},
		SoldOutItemIDs: map[string]bool{steakID: true},
	})
	require.NoError(t, err)
	return snapshot
}

func TestFinalizeWaiterV2_EightySixedSteakIsSoldOutNotMissing(t *testing.T) {
	snapshot := waiterSpeechSnapshot(t, "en", "ordering")
	steak, ok := snapshot.ByKey[WaiterMenuEntityKey{Type: waiterMenuEntityTypeMenuItem, ID: "demo-steak"}]
	require.True(t, ok)
	require.False(t, steak.Available)

	for _, msg := range []string{
		"Is the Steak Plate available?",
		"Can I add the steak anyway and the kitchen will confirm?",
		"el steak esta?",
	} {
		t.Run(msg, func(t *testing.T) {
			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-628", Locale: "en", Mode: "ordering",
				UserMessage: msg, Snapshot: snapshot,
			})
			require.NoError(t, err)
			requireValidWaiterV2(t, response)
			assert.NotEqual(t, services.ClarifyItemMessage("en"), response.Answer.Content)
			assert.NotContains(t, strings.ToLower(response.Answer.Content), "couldn't find")
			assert.Contains(t, strings.ToLower(response.Answer.Content), "steak plate")
			assert.Contains(t, strings.ToLower(response.Answer.Content), "unavailable")
			require.NotEmpty(t, response.Entities)
			assert.Equal(t, "unavailable", response.Entities[0].Availability)
			assert.Empty(t, response.Actions)
		})
	}
}

func TestFinalizeWaiterV2_SpanishEightySixAskNamesSteak(t *testing.T) {
	snapshot := waiterSpeechSnapshot(t, "es", "ordering")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-700", Locale: "es", Mode: "ordering",
		UserMessage: "Che, hay bife o asado? Si el steak plate no esta, que me recomendas?",
		Snapshot:    snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	lower := strings.ToLower(response.Answer.Content)
	assert.Contains(t, lower, "steak plate")
	assert.NotEqual(t, services.ClarifyItemMessage("es"), response.Answer.Content)
	assert.NotEqual(t, waiterFinalizerCopy("es").safeFallback, response.Answer.Content)
	namedSteak := false
	for _, entity := range response.Entities {
		if strings.Contains(strings.ToLower(entity.DisplayName), "steak") {
			namedSteak = true
			assert.Equal(t, "unavailable", entity.Availability)
		}
	}
	assert.True(t, namedSteak, "must name the 86'd steak, not silently list other dishes")
}

func TestFinalizeWaiterV2_DateNightWineDealAndSoldOutAreGrounded(t *testing.T) {
	snapshot := waiterSpeechSnapshot(t, "en", "ordering")
	visit := services.WaiterVisitFacts{HoursKnown: true, TodayOpen: "11:00", TodayClose: "23:00"}

	t.Run("date night bundle stays explainable when steak is 86", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-629-date", Locale: "en", Mode: "ordering",
			UserMessage: "Tell me about Date Night for Two", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotEqual(t, waiterFinalizerCopy("en").safeFallback, response.Answer.Content)
		assert.Contains(t, strings.ToLower(response.Answer.Content), "date night")
		assert.Contains(t, strings.ToLower(response.Answer.Content), "unavailable")
	})

	t.Run("sold out listing names the 86'd steak", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-629-86", Locale: "en", Mode: "ordering",
			UserMessage: "What's sold out tonight? Anything 86'd I should skip?", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Equal(t, assistantcontract.StatusComplete, response.Status)
		assert.Contains(t, strings.ToLower(response.Answer.Content), "steak plate")
		assert.NotEqual(t, waiterFinalizerCopy("en").safeFallback, response.Answer.Content)
	})

	t.Run("five dollar steak deal is named as unavailable", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-629-deal", Locale: "en", Mode: "ordering",
			UserMessage: "I saw a five dollar off steak deal. Can I still get that?", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotEqual(t, waiterFinalizerCopy("en").safeFallback, response.Answer.Content)
		assert.Contains(t, strings.ToLower(response.Answer.Content), "steak")
	})

	t.Run("wine pairing with 86'd steak stays grounded", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-629-wine", Locale: "en", Mode: "ordering",
			UserMessage: "What wine goes with the steak plate?", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotEqual(t, waiterFinalizerCopy("en").safeFallback, response.Answer.Content)
		assert.Contains(t, strings.ToLower(response.Answer.Content), "steak plate")
		assert.NotContains(t, strings.ToLower(response.Answer.Content), "vintage")
	})

	t.Run("hours use server facts not the degraded stub", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-630-hours", Locale: "en", Mode: "concierge",
			UserMessage: "When do you close?", Snapshot: waiterSpeechSnapshot(t, "en", "concierge"),
			Visit: visit,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Equal(t, assistantcontract.StatusComplete, response.Status)
		assert.Contains(t, response.Answer.Content, "23:00")
		assert.NotEqual(t, waiterFinalizerCopy("en").safeFallback, response.Answer.Content)
	})

	t.Run("desserts plus close names chocolate tart and hours", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-630-dessert", Locale: "en", Mode: "concierge",
			UserMessage: "What desserts do you have and when do you close?",
			Snapshot:    waiterSpeechSnapshot(t, "en", "concierge"), Visit: visit,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotEqual(t, services.ClarifyItemMessage("en"), response.Answer.Content)
		assert.Contains(t, strings.ToLower(response.Answer.Content), "chocolate tart")
		assert.Contains(t, response.Answer.Content, "23:00")
	})

	t.Run("reservations are a grounded visit answer", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-630-resv", Locale: "en", Mode: "concierge",
			UserMessage: "Do you take reservations tonight for two at 8pm?",
			Snapshot:    waiterSpeechSnapshot(t, "en", "concierge"),
			Visit:       services.WaiterVisitFacts{Reservations: true, PartyMin: 2, PartyMax: 8},
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotEqual(t, waiterFinalizerCopy("en").safeFallback, response.Answer.Content)
		assert.Contains(t, response.Answer.Content, "2")
		assert.Contains(t, response.Answer.Content, "8")
	})

	t.Run("parking is not treated as a missing menu item", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-630-park", Locale: "es", Mode: "concierge",
			UserMessage: "Hay estacionamiento? Y el steak plate se puede pedir para delivery?",
			Snapshot:    waiterSpeechSnapshot(t, "es", "concierge"),
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotEqual(t, services.ClarifyItemMessage("es"), response.Answer.Content)
		assert.NotContains(t, strings.ToLower(response.Answer.Content), "no encontré")
	})

	t.Run("bill ask describes the check instead of the stub", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-679-bill", Locale: "en", Mode: "ordering",
			UserMessage: "can I get the bill please?", Snapshot: snapshot,
			Visit: services.WaiterVisitFacts{HasOpenBill: true, BillSummary: "- 1x Iced Tea ($4.00)\nTotal: $4.00"},
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotEqual(t, waiterFinalizerCopy("en").safeFallback, response.Answer.Content)
		assert.Contains(t, response.Answer.Content, "Iced Tea")
	})
}

func TestFinalizeWaiterV2_SpanishCeliacAskNamesGlutenFreeTaggedAlternative(t *testing.T) {
	snapshot := waiterSpeechSnapshot(t, "es", "ordering")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-634", Locale: "es", Mode: "ordering",
		UserMessage: "Hola, para celiaquicos que hay que no sea milanesa?",
		Snapshot:    snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	lower := strings.ToLower(response.Answer.Content)
	assert.Contains(t, lower, "milanesa")
	assert.Contains(t, lower, "quinoa salad", "must name a gluten-free tagged alternative, not just off-menu")
	assert.NotContains(t, lower, "harvest bowl", "Harvest Bowl lists gluten and must not be the celiac rec")
	assert.NotContains(t, lower, strings.ToLower(services.AllergenRefusal("es")))
	assert.NotEqual(t, services.ClarifyItemMessage("es"), response.Answer.Content)
	assert.NotEqual(t, waiterFinalizerCopy("es").safeFallback, response.Answer.Content)
	namedGF := false
	for _, entity := range response.Entities {
		if strings.Contains(strings.ToLower(entity.DisplayName), "quinoa") {
			namedGF = true
			assert.Equal(t, "available", entity.Availability)
		}
		assert.NotContains(t, strings.ToLower(entity.DisplayName), "harvest")
	}
	assert.True(t, namedGF)
}

func TestFinalizeWaiterV2_GlutenRecommendationNamesTaggedSafeItem(t *testing.T) {
	snapshot := waiterSpeechSnapshot(t, "es", "ordering")
	intent, _ := classifyWaiterV2Intent("es", "hola que recomendas que no tenga gluten", snapshot)
	assert.Equal(t, waiterIntentRecommendation, intent)

	first, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-680-a", Locale: "es", Mode: "ordering",
		UserMessage: "hola que recomendas que no tenga gluten", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, first)
	second, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-680-b", Locale: "es", Mode: "ordering",
		UserMessage: "hola que recomendas que no tenga gluten", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, second)

	for _, response := range []assistantcontract.Response{first, second} {
		lower := strings.ToLower(response.Answer.Content)
		assert.Contains(t, lower, "quinoa salad")
		assert.NotContains(t, lower, "harvest bowl")
		assert.NotContains(t, lower, strings.ToLower(services.AllergenRefusal("es")))
		assert.NotEqual(t, services.ClarifyItemMessage("es"), response.Answer.Content)
		assert.NotEqual(t, waiterFinalizerCopy("es").safeFallback, response.Answer.Content)
	}
	assert.Equal(t, first.Answer.Content, second.Answer.Content, "same gluten rec must not flip answers")
}

func TestHandleAIWaiter_SoldOutSteakDoesNotNeedLLM(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "eighty-six-speech", true)
	table := createAIWaiterTable(t, business.ID, "M03")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID,
		Categories: `[{"id":"mains","name":"Mains","items":[{"id":"quinoa-salad","name":"Quinoa Salad","price":13,"is_available":true,"currency":"USD","dietary_tags":["gluten-free","vegetarian"]},{"id":"demo-steak","name":"Steak Plate","price":29,"is_available":true,"currency":"USD"},{"id":"harvest-bowl","name":"Harvest Bowl","price":14,"is_available":true,"currency":"USD","allergens":["gluten"],"dietary_tags":["vegetarian"]}]}]`,
		IsActive:   true, Version: 1}).Error)

	previous := loadUnrecommendableMenuItemIDs
	loadUnrecommendableMenuItemIDs = func(uint) (map[string]bool, error) {
		return map[string]bool{"demo-steak": true}, nil
	}
	t.Cleanup(func() { loadUnrecommendableMenuItemIDs = previous })

	conv := createAIWaiterConversation(t, business.ID, "s-86-speech")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)

	SetAIService(nil)
	t.Cleanup(func() { SetAIService(nil) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": conv.SessionID, "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "Can I add the steak anyway and the kitchen will confirm?"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, strings.ToLower(w.Body.String()), "couldn't find")
	assert.Contains(t, strings.ToLower(w.Body.String()), "steak plate")
	assert.Contains(t, strings.ToLower(w.Body.String()), "unavailable")
}

func TestHandleAIWaiter_SpanishCeliacAskNamesGlutenFreeTaggedAlternative(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "celiac-gf-alt", true)
	table := createAIWaiterTable(t, business.ID, "M03")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID,
		Categories: `[{"id":"mains","name":"Mains","items":[{"id":"quinoa-salad","name":"Quinoa Salad","price":13,"is_available":true,"currency":"USD","dietary_tags":["gluten-free"]},{"id":"harvest-bowl","name":"Harvest Bowl","price":14,"is_available":true,"currency":"USD","allergens":["gluten","sesame"],"dietary_tags":["vegetarian"]}]}]`,
		IsActive:   true, Version: 1}).Error)

	conv := createAIWaiterConversation(t, business.ID, "s-634-celiac")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)

	SetAIService(nil)
	t.Cleanup(func() { SetAIService(nil) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": conv.SessionID, "mode": "ordering", "table_code": table.TableCode, "language": "es",
		"history": []map[string]any{{"role": "user", "content": "para celiaquicos que hay que no sea milanesa?"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := strings.ToLower(w.Body.String())
	assert.Contains(t, body, "milanesa")
	assert.Contains(t, body, "quinoa salad")
	assert.NotContains(t, body, "harvest bowl")
	assert.NotContains(t, body, strings.ToLower(services.AllergenRefusal("es")))
}

func TestFinalizeWaiterV2_SpanishSteakEstaOnTranslatedMenuIsSoldOutNotStub(t *testing.T) {
	business := &database.Business{
		ID: 86, DefaultLanguage: "en", IsActive: true, KitchenEnabled: true, OrdersEnabled: true, Name: "Lounge",
	}
	source := []database.MenuCategory{
		{ID: "mains", Name: "Mains", SortOrder: 1, Items: []database.MenuItem{
			{ID: "demo-steak", Name: "Steak Plate", Price: 29, Currency: "USD", IsAvailable: true, SortOrder: 1},
			{ID: "quinoa-salad", Name: "Quinoa Salad", Price: 13, Currency: "USD", IsAvailable: true, DietaryTags: []string{"gluten-free"}, SortOrder: 2},
		}},
	}
	display := []database.MenuCategory{
		{ID: "mains", Name: "Principales", SortOrder: 1, Items: []database.MenuItem{
			{ID: "demo-steak", Name: "Plato de bife", Price: 29, Currency: "USD", IsAvailable: true, SortOrder: 1},
			{ID: "quinoa-salad", Name: "Ensalada de quinoa", Price: 13, Currency: "USD", IsAvailable: true, DietaryTags: []string{"sin tacc"}, SortOrder: 2},
		}},
	}
	snapshot, err := BuildWaiterMenuSnapshot(WaiterMenuSnapshotInput{
		Business: business, Locale: "es", Mode: "ordering", BusinessOpen: true,
		Categories: display, SourceCategories: source,
		SoldOutItemIDs: map[string]bool{"demo-steak": true},
	})
	require.NoError(t, err)

	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-679-esta", Locale: "es", Mode: "ordering",
		UserMessage: "el steak esta?", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	lower := strings.ToLower(response.Answer.Content)
	assert.NotEqual(t, waiterFinalizerCopy("es").safeFallback, response.Answer.Content)
	assert.NotEqual(t, services.ClarifyItemMessage("es"), response.Answer.Content)
	assert.Contains(t, lower, "plato de bife")
	assert.Contains(t, lower, waiterFinalizerCopy("es").unavailable)
	require.NotEmpty(t, response.Entities)
	assert.Equal(t, "unavailable", response.Entities[0].Availability)
}

func TestFinalizeWaiterV2_DinerIntentsAreGroundedNotKitchenStub(t *testing.T) {
	snapshot := waiterSpeechSnapshot(t, "es", "ordering")
	visit := services.WaiterVisitFacts{HasOpenBill: true, BillSummary: "- 1x Iced Tea ($4.00)\nTotal: $4.00"}

	t.Run("date night is named, not not-found", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-678-date", Locale: "es", Mode: "ordering",
			UserMessage: "hay date night?", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotEqual(t, services.ClarifyItemMessage("es"), response.Answer.Content)
		assert.NotEqual(t, waiterFinalizerCopy("es").safeFallback, response.Answer.Content)
		assert.Contains(t, strings.ToLower(response.Answer.Content), "date night")
	})

	t.Run("iced tea order names the drink", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-678-tea", Locale: "es", Mode: "ordering",
			UserMessage: "mandame un iced tea", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotEqual(t, waiterFinalizerCopy("es").safeFallback, response.Answer.Content)
		assert.Contains(t, strings.ToLower(response.Answer.Content), "iced tea")
	})

	t.Run("la cuenta describes the open check", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-678-bill", Locale: "es", Mode: "ordering",
			UserMessage: "la cuenta", Snapshot: snapshot, Visit: visit,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Contains(t, response.Answer.Content, "Iced Tea")
	})

	t.Run("llama al mozo is a grounded service call", func(t *testing.T) {
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-678-mozo", Locale: "es", Mode: "ordering",
			UserMessage: "llama al mozo, agua", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.Equal(t, services.WaiterServiceCallAnswer("es"), response.Answer.Content)
		assert.NotEqual(t, waiterFinalizerCopy("es").safeFallback, response.Answer.Content)
	})
}

func TestHandleAIWaiter_OperatorSandbox86AndTablesAreNotCannedStub(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "op-sandbox-86", true)
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID,
		Categories: `[{"id":"mains","name":"Mains","items":[{"id":"ribs","name":"Ribs","price":22,"is_available":true,"currency":"USD"}]}]`,
		IsActive:   true, Version: 1}).Error)
	previous := loadUnrecommendableMenuItemIDs
	loadUnrecommendableMenuItemIDs = func(uint) (map[string]bool, error) {
		return map[string]bool{"ribs": true}, nil
	}
	t.Cleanup(func() { loadUnrecommendableMenuItemIDs = previous })

	conv, err := database.GetOrCreateAiWaiterConversation(
		"optest-sandbox-86", business.ID, database.AiWaiterOperatorTestTableCode, "es-AR", aiWaiterOperatorTestMode,
	)
	require.NoError(t, err)

	SetAIService(nil)
	t.Cleanup(func() { SetAIService(nil) })

	router := gin.New()
	router.POST("/businesses/:id/ai/test-chat", HandleAIWaiterTestChat)

	w := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/test-chat", business.ID),
		map[string]any{
			"session_token": conv.SessionID, "language": "es-AR",
			"table_code": "", "mode": "concierge",
			"history": []map[string]any{
				{"role": "assistant", "content": "Hola, soy Alfred."},
				{"role": "user", "content": "86 the ribs — is the ribs available?"},
			},
		})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "Te puedo ayudar con preguntas sobre el menú y tu visita.")
	assert.Contains(t, strings.ToLower(w.Body.String()), "ribs")

	w2 := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/test-chat", business.ID),
		map[string]any{
			"session_token": conv.SessionID, "language": "es-AR",
			"table_code": "", "mode": "concierge",
			"history": []map[string]any{
				{"role": "assistant", "content": "Hola, soy Alfred."},
				{"role": "user", "content": "qué mesas están libres?"},
			},
		})
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	assert.NotContains(t, w2.Body.String(), "Te puedo ayudar con preguntas sobre el menú y tu visita.")
	assert.Contains(t, w2.Body.String(), services.WaiterTablesAnswer("es-AR"))
}

func TestHandleAIWaiter_ConciergeGreetingIsNotOrderingScript(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "concierge-greet", true)
	config.SetAIProviderConfiguredForTesting(t, true)

	router := gin.New()
	router.POST("/ai-waiter/:businessId/session", CreateAIWaiterSession)
	w := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/ai-waiter/%d/session", biz.ID),
		map[string]any{"mode": "concierge", "table_code": "", "language": "en"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Greeting string `json:"greeting"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotContains(t, strings.ToLower(resp.Greeting), "help ordering")
	assert.Equal(t, services.WaiterGreeting("en", "Alfred", biz.Name, "concierge", true), resp.Greeting)
}

func TestFinalizeWaiterV2_AvailabilityAskRecommendsOrderableNotEightySixed(t *testing.T) {
	snapshot := waiterSpeechSnapshot(t, "en", "ordering")
	steak, ok := snapshot.ByKey[WaiterMenuEntityKey{Type: waiterMenuEntityTypeMenuItem, ID: "demo-steak"}]
	require.True(t, ok)
	require.False(t, steak.Available)
	dateNight, ok := snapshot.ByKey[WaiterMenuEntityKey{Type: waiterMenuEntityTypeBundle, ID: "9"}]
	require.True(t, ok)
	require.False(t, dateNight.Available)

	asks := []struct {
		name    string
		message string
	}{
		{"production 86 skip", "what's good that's actually available right now? nothing 86'd please"},
		{"available now", "what's available right now?"},
	}
	orderable := []string{"harvest bowl", "market tacos", "iced tea", "demo spritz", "chocolate tart"}
	for _, tc := range asks {
		t.Run(tc.name, func(t *testing.T) {
			intent, _ := classifyWaiterV2Intent("en", tc.message, snapshot)
			assert.Equal(t, waiterIntentRecommendation, intent, "availability ask must not route to sold-out listing")

			response, err := FinalizeWaiterV2(WaiterFinalizeInput{
				ResponseID: "waiter-767-" + strings.ReplaceAll(tc.name, " ", "-"),
				Locale:     "en", Mode: "ordering",
				UserMessage: tc.message, Snapshot: snapshot,
			})
			require.NoError(t, err)
			requireValidWaiterV2(t, response)
			assert.Equal(t, assistantcontract.StatusComplete, response.Status)
			lower := strings.ToLower(response.Answer.Content)
			assert.NotEqual(t, services.WaiterSoldOutIntro("en"), strings.TrimSpace(strings.Split(response.Answer.Content, "\n")[0]))
			assert.NotContains(t, lower, "these are sold out right now")
			assert.Contains(t, lower, "available options")
			require.NotEmpty(t, response.Entities, "must recommend orderable dishes, not an empty available set")

			available := 0
			for _, entity := range response.Entities {
				assert.Equal(t, "available", entity.Availability, "%s must be orderable, not 86'd", entity.DisplayName)
				assert.NotContains(t, strings.ToLower(entity.DisplayName), "steak")
				assert.NotContains(t, strings.ToLower(entity.DisplayName), "date night")
				available++
			}
			assert.Greater(t, available, 0)
			serialized := responseJSONLower(t, response)
			namedOrderable := false
			for _, dish := range orderable {
				if strings.Contains(serialized, dish) {
					namedOrderable = true
					break
				}
			}
			assert.True(t, namedOrderable, "must name an orderable dish, got %q", response.Answer.Content)
			assert.NotContains(t, serialized, "date night")
		})
	}
}

func TestWaiterDiscovery_SoldOutHoursBillAndDietary(t *testing.T) {
	assert.True(t, services.DetectSoldOutIntent("en", "What's sold out tonight? Anything 86'd I should skip?"))
	assert.True(t, services.DetectSoldOutIntent("es", "che esta 86 el asado?"))
	assert.True(t, services.DetectHoursIntent("en", "When do you close?"))
	assert.True(t, services.DetectBillIntent("en", "can I get the bill please?"))
	assert.True(t, services.DetectBillIntent("es", "la cuenta"))
	assert.True(t, services.DietaryRecommendationIntent("es", "hola que recomendas que no tenga gluten"))
	assert.True(t, services.DetectTablesIntent("es-AR", "qué mesas están libres?"))
	assert.True(t, services.DetectTablesIntent("en", "qué mesas están libres?"))
	assert.True(t, services.DetectServiceCallIntent("es", "llama al mozo, agua"))
	assert.Equal(t, "milanesa", services.ExtractExcludedDishName("es", "para celiaquicos que hay que no sea milanesa?"))
	assert.Equal(t, []string{"gluten-free"}, services.DetectDietaryTags("es", "que no tenga gluten"))
}
