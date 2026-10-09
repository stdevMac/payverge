package aicontract

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/agents/ops_guides"
	"github.com/stdevmac/payverge/backend/internal/agents/ops_tools"
	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guardrails"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/services/director_tools"
	"github.com/stretchr/testify/require"

	"gopkg.in/yaml.v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Real-path adapters exercise production packages on shipped contracts.

func TestRealPath_WaiterCartValidation_ProductionGate(t *testing.T) {
	// Production cart validator used by the waiter handler serializer.
	kept, dropped := server.ValidateCartToolCallsForContract(
		[]llm.ToolCall{
			{Name: "add_to_cart", Args: map[string]any{"item_name": "Unicorn Steak", "quantity": float64(1)}},
			{Name: "add_to_cart", Args: map[string]any{"item_name": "House Burger", "quantity": float64(1)}},
		},
		[]database.MenuCategory{{
			ID:    "mains",
			Items: []database.MenuItem{{ID: "house-burger", Name: "House Burger", IsAvailable: true}},
		}},
		nil,
	)
	require.Equal(t, 1, dropped)
	require.Len(t, kept, 1)
	require.Equal(t, "House Burger", kept[0].Args["item_name"])
	require.Equal(t, "house-burger", kept[0].Args["menu_item_id"], "V1 name compatibility must emit the stable server-owned ID")

	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)
	for _, s := range scenarios {
		if s.ID == "waiter-invalid-item-tool-dropped-everywhere" {
			require.Equal(t, 0, s.Expect.Effects["cart_add"])
		}
	}
}

var waiterV2ContractScenarioIDs = []string{
	"waiter-in-stock-action-en",
	"waiter-translated-name-ambiguous-es",
	"waiter-ordering-disabled-en",
	"waiter-allergen-cart-safety-es",
	"waiter-demo-menu-off-menu-prose-removed",
	"waiter-long-menu-v2-sections-and-sources",
	"waiter-translated-item-stable-id",
	"waiter-unavailable-id-rejected",
	"waiter-cart-answer-does-not-preclaim-success",
	"waiter-v2-history-roundtrip",
	"waiter-malicious-link-image-rejected",
	"waiter-hours-open-from-settings-typo-en",
	"waiter-price-for-item-question-en",
	"waiter-vegetarian-pick-not-relist-en",
	"waiter-gluten-free-uses-allergen-chips-en",
	"waiter-wait-time-followthrough-en",
}

var waiterScenarioDBSequence atomic.Uint64

func init() {
	for _, scenarioID := range waiterV2ContractScenarioIDs {
		RegisterScenarioRunner(scenarioID, runWaiterV2MatrixScenario)
	}
}

func waiterMatrixInputString(scenario Scenario, key string) (string, error) {
	value, ok := scenario.Input[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("scenario %s input.%s must be a non-empty string", scenario.ID, key)
	}
	return value, nil
}

func waiterMatrixInputInt(scenario Scenario, key string) (int, error) {
	value, ok := scenario.Input[key].(int)
	if !ok || value < 1 {
		return 0, fmt.Errorf("scenario %s input.%s must be a positive integer", scenario.ID, key)
	}
	return value, nil
}

func waiterMatrixSnapshot(locale string, categories []database.MenuCategory, trustedImageHost string) (server.WaiterMenuSnapshot, error) {
	business := &database.Business{
		ID: 47, DefaultLanguage: "en", IsActive: true,
		KitchenEnabled: true, OrdersEnabled: true,
	}
	return server.BuildWaiterMenuSnapshot(server.WaiterMenuSnapshotInput{
		Business: business, Locale: locale, Mode: "ordering", BusinessOpen: true,
		Categories: categories, HiddenItemIDs: map[string]bool{}, TrustedImageHost: trustedImageHost,
	})
}

func waiterMatrixSnapshotWithOrdering(locale string, categories []database.MenuCategory, orderingEnabled bool) (server.WaiterMenuSnapshot, error) {
	business := &database.Business{
		ID: 47, DefaultLanguage: "en", IsActive: true,
		KitchenEnabled: orderingEnabled, OrdersEnabled: orderingEnabled,
	}
	return server.BuildWaiterMenuSnapshot(server.WaiterMenuSnapshotInput{
		Business: business, Locale: locale, Mode: "ordering", BusinessOpen: true,
		Categories: categories, HiddenItemIDs: map[string]bool{},
	})
}

func waiterMatrixBaseCategories() []database.MenuCategory {
	return []database.MenuCategory{{
		ID: "mains", Name: "Mains", SortOrder: 1,
		Items: []database.MenuItem{
			{ID: "harvest-bowl", Name: "Harvest Bowl", Description: "Roasted vegetables and grains", Price: 14, Currency: "USD", IsAvailable: true, SortOrder: 1},
			{ID: "seasonal-soup", Name: "Seasonal Soup", Description: "Ask staff about today's preparation", Price: 9, Currency: "USD", IsAvailable: false, SortOrder: 2},
		},
	}}
}

// waiterMatrixDietaryCategories mirrors the production QR-menu shape behind
// issues 790/816: an item chipped with allergens (Gluten, Sesame) plus a
// dietary tag, and a cross-category tagged drink for the companion suggestion.
func waiterMatrixDietaryCategories() []database.MenuCategory {
	return []database.MenuCategory{
		{
			ID: "mains", Name: "Mains", SortOrder: 1,
			Items: []database.MenuItem{
				{ID: "harvest-bowl", Name: "Harvest Bowl", Description: "Roasted vegetables and grains", Price: 14, Currency: "USD", IsAvailable: true, Allergens: []string{"Gluten", "Sesame"}, DietaryTags: []string{"vegetarian"}, SortOrder: 1},
				{ID: "seasonal-soup", Name: "Seasonal Soup", Description: "Ask staff about today's preparation", Price: 9, Currency: "USD", IsAvailable: false, SortOrder: 2},
			},
		},
		{
			ID: "drinks", Name: "Drinks", SortOrder: 2,
			Items: []database.MenuItem{
				{ID: "iced-tea", Name: "Iced Tea", Price: 4, Currency: "USD", IsAvailable: true, DietaryTags: []string{"vegetarian", "vegan"}, SortOrder: 1},
			},
		},
	}
}

func waiterMatrixLongCategories() []database.MenuCategory {
	counts := []int{3, 3, 2, 2, 2, 1}
	categories := make([]database.MenuCategory, 0, len(counts))
	dish := 1
	for categoryIndex, count := range counts {
		items := make([]database.MenuItem, 0, count)
		for itemIndex := 0; itemIndex < count; itemIndex++ {
			items = append(items, database.MenuItem{
				ID: fmt.Sprintf("dish-%02d", dish), Name: fmt.Sprintf("Dish %02d", dish),
				Description: "Production-shaped menu item", Price: float64(10 + dish), Currency: "USD",
				IsAvailable: true, SortOrder: itemIndex + 1,
			})
			dish++
		}
		categories = append(categories, database.MenuCategory{
			ID: fmt.Sprintf("category-%02d", categoryIndex+1), Name: fmt.Sprintf("Category %02d", categoryIndex+1),
			Items: items, SortOrder: categoryIndex + 1,
		})
	}
	return categories
}

func waiterMatrixFinalize(scenario Scenario, responseID string, categories []database.MenuCategory, calls []llm.ToolCall, trustedImageHost string) (assistantcontract.Response, int, error) {
	message, err := waiterMatrixInputString(scenario, "message")
	if err != nil {
		return assistantcontract.Response{}, 0, err
	}
	modelText, _ := scenario.Input["model_text"].(string)
	validated, dropped := server.ValidateCartToolCallsForContract(calls, categories, nil)
	snapshot, err := waiterMatrixSnapshot(scenario.Locale, categories, trustedImageHost)
	if err != nil {
		return assistantcontract.Response{}, dropped, err
	}
	response, err := server.FinalizeWaiterV2(server.WaiterFinalizeInput{
		ResponseID: responseID, Locale: scenario.Locale, Mode: "ordering",
		UserMessage: message, ModelText: modelText, ValidatedCalls: validated, Snapshot: snapshot,
	})
	return response, dropped, err
}

func waiterMatrixResult(code string, response assistantcontract.Response, effects map[string]int) (ScenarioRunResult, error) {
	if err := assistantcontract.Validate(response); err != nil {
		return ScenarioRunResult{}, err
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return ScenarioRunResult{}, err
	}
	if effects == nil {
		effects = map[string]int{}
	}
	effects["action_count"] = len(response.Actions)
	effects["source_count"] = len(response.Sources)
	effects["entity_count"] = len(response.Entities)
	effects["section_count"] = len(response.Sections)
	request, err := llm.NewGenerateRequest("waiter")
	if err != nil {
		return ScenarioRunResult{}, err
	}
	return ScenarioRunResult{
		Code: code, Text: string(encoded), Effects: effects,
		PrivacyClass: string(request.PrivacyClass), ZDR: request.RequiresZDR(),
	}, nil
}

func runWaiterV2MatrixScenario(scenario Scenario) (ScenarioRunResult, error) {
	switch scenario.ID {
	case "waiter-in-stock-action-en":
		stableID, err := waiterMatrixInputString(scenario, "stable_id")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		response, dropped, err := waiterMatrixFinalize(scenario, "contract_waiter_in_stock", waiterMatrixBaseCategories(), []llm.ToolCall{{
			ID: "in-stock", Name: "add_to_cart", Args: map[string]any{"menu_item_id": stableID, "quantity": float64(1)},
		}}, "")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		return waiterMatrixResult("action_ready", response, map[string]int{"dropped_cart": dropped})

	case "waiter-translated-name-ambiguous-es":
		categories := []database.MenuCategory{{ID: "principales", Name: "Principales", Items: []database.MenuItem{
			{ID: "house-salad-a", Name: "Ensalada de la Casa", Price: 12, Currency: "USD", IsAvailable: true},
			{ID: "house-salad-b", Name: "Ensalada de la Casa", Price: 13, Currency: "USD", IsAvailable: true},
		}}}
		response, dropped, err := waiterMatrixFinalize(scenario, "contract_waiter_ambiguous", categories, []llm.ToolCall{{
			ID: "ambiguous-name", Name: "add_to_cart", Args: map[string]any{"item_name": "Ensalada de la Casa", "quantity": float64(1)},
		}}, "")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		return waiterMatrixResult("needs_clarification", response, map[string]int{"dropped_cart": dropped})

	case "waiter-ordering-disabled-en":
		message, err := waiterMatrixInputString(scenario, "message")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		stableID, err := waiterMatrixInputString(scenario, "stable_id")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		categories := waiterMatrixBaseCategories()
		validated, dropped := server.ValidateCartToolCallsForContract([]llm.ToolCall{{
			ID: "ordering-disabled", Name: "add_to_cart", Args: map[string]any{"menu_item_id": stableID, "quantity": float64(1)},
		}}, categories, nil)
		snapshot, err := waiterMatrixSnapshotWithOrdering(scenario.Locale, categories, false)
		if err != nil {
			return ScenarioRunResult{}, err
		}
		response, err := server.FinalizeWaiterV2(server.WaiterFinalizeInput{
			ResponseID: "contract_waiter_ordering_disabled", Locale: scenario.Locale, Mode: "ordering",
			UserMessage: message, ValidatedCalls: validated, Snapshot: snapshot,
		})
		if err != nil {
			return ScenarioRunResult{}, err
		}
		return waiterMatrixResult("ordering_blocked", response, map[string]int{"dropped_cart": dropped})

	case "waiter-allergen-cart-safety-es":
		message, err := waiterMatrixInputString(scenario, "message")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		categories := []database.MenuCategory{{ID: "principales", Name: "Principales", Items: []database.MenuItem{{
			ID: "salsa-mani", Name: "Plato con maní", Price: 14, Currency: "USD", IsAvailable: true, Allergens: []string{"maní"},
		}}}}
		validated, _ := server.ValidateCartToolCallsForContract([]llm.ToolCall{{
			ID: "allergen-cart", Name: "add_to_cart", Args: map[string]any{"menu_item_id": "salsa-mani", "quantity": float64(1)},
		}}, categories, nil)
		snapshot, err := waiterMatrixSnapshot(scenario.Locale, categories, "")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		response, err := server.FinalizeWaiterV2(server.WaiterFinalizeInput{
			ResponseID: "contract_waiter_allergen_cart", Locale: scenario.Locale, Mode: "ordering",
			UserMessage: message, ValidatedCalls: validated, Snapshot: snapshot,
		})
		if err != nil {
			return ScenarioRunResult{}, err
		}
		return waiterMatrixResult("allergen_safe", response, nil)

	case "waiter-demo-menu-off-menu-prose-removed":
		response, _, err := waiterMatrixFinalize(scenario, "contract_waiter_off_menu", waiterMatrixBaseCategories(), nil, "")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		return waiterMatrixResult("needs_clarification", response, nil)

	case "waiter-long-menu-v2-sections-and-sources":
		if oversized, _ := scenario.Input["oversized_model_text"].(bool); oversized {
			input := make(map[string]any, len(scenario.Input))
			for key, value := range scenario.Input {
				input[key] = value
			}
			input["model_text"] = strings.Repeat("OVERSIZED_WAITER_MODEL ", 800)
			scenario.Input = input
		}
		categories := waiterMatrixLongCategories()
		for left, right := 0, len(categories)-1; left < right; left, right = left+1, right-1 {
			categories[left], categories[right] = categories[right], categories[left]
		}
		for index := range categories {
			for left, right := 0, len(categories[index].Items)-1; left < right; left, right = left+1, right-1 {
				categories[index].Items[left], categories[index].Items[right] = categories[index].Items[right], categories[index].Items[left]
			}
		}
		response, _, err := waiterMatrixFinalize(scenario, "contract_waiter_long_menu", categories, nil, "")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		degraded := 0
		if response.Status == assistantcontract.StatusDegraded {
			degraded = 1
		}
		orderedCap := 0
		if len(response.Entities) == 12 && response.Entities[0].ID == "menu_item:dish-01" && response.Entities[11].ID == "menu_item:dish-12" {
			orderedCap = 1
		}
		return waiterMatrixResult("menu_catalog", response, map[string]int{"degraded": degraded, "ordered_cap": orderedCap})

	case "waiter-translated-item-stable-id":
		stableID, err := waiterMatrixInputString(scenario, "stable_id")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		translatedName, err := waiterMatrixInputString(scenario, "translated_name")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		categories := []database.MenuCategory{{ID: "ensaladas", Name: "Ensaladas", Items: []database.MenuItem{{
			ID: stableID, Name: translatedName, Description: "Verduras frescas", Price: 12, Currency: "USD", IsAvailable: true,
		}}}}
		response, dropped, err := waiterMatrixFinalize(scenario, "contract_waiter_translated", categories, []llm.ToolCall{{
			ID: "recommendation-evidence", Name: "add_to_cart", Args: map[string]any{"menu_item_id": stableID, "quantity": float64(1)},
		}}, "")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		if dropped != 0 {
			return ScenarioRunResult{}, fmt.Errorf("translated stable-ID evidence was rejected by production cart validation")
		}
		stableMatch := 0
		if len(response.Entities) == 1 && response.Entities[0].ID == "menu_item:"+stableID && response.Entities[0].DisplayName == translatedName {
			stableMatch = 1
		}
		return waiterMatrixResult("grounded", response, map[string]int{"stable_id_match": stableMatch})

	case "waiter-unavailable-id-rejected":
		stableID, err := waiterMatrixInputString(scenario, "stable_id")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		categories := waiterMatrixBaseCategories()
		response, dropped, err := waiterMatrixFinalize(scenario, "contract_waiter_unavailable", categories, []llm.ToolCall{{
			ID: "unavailable", Name: "add_to_cart", Args: map[string]any{"menu_item_id": stableID, "quantity": float64(1)},
		}}, "")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		return waiterMatrixResult("needs_clarification", response, map[string]int{"dropped_cart": dropped})

	case "waiter-cart-answer-does-not-preclaim-success", "waiter-v2-history-roundtrip":
		stableID, err := waiterMatrixInputString(scenario, "stable_id")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		quantity, err := waiterMatrixInputInt(scenario, "quantity")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		responseID := "contract_waiter_cart"
		if scenario.ID == "waiter-v2-history-roundtrip" {
			responseID = "contract_waiter_history"
		}
		response, _, err := waiterMatrixFinalize(scenario, responseID, waiterMatrixBaseCategories(), []llm.ToolCall{{
			ID: "cart-action", Name: "add_to_cart", Args: map[string]any{"menu_item_id": stableID, "quantity": float64(quantity)},
		}}, "")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		if scenario.ID == "waiter-v2-history-roundtrip" {
			encoded, marshalErr := json.Marshal(response)
			if marshalErr != nil {
				return ScenarioRunResult{}, marshalErr
			}
			dsn := fmt.Sprintf("file:aicontract_waiter_history_%d?mode=memory&cache=shared", waiterScenarioDBSequence.Add(1))
			gdb, dbErr := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
			if dbErr != nil {
				return ScenarioRunResult{}, dbErr
			}
			database.SetTestDB(gdb)
			if sqlDB, sqlErr := gdb.DB(); sqlErr == nil {
				sqlDB.SetMaxOpenConns(1)
			}
			if migrateErr := gdb.AutoMigrate(&database.AiWaiterConversation{}, &database.AiWaiterMessage{}); migrateErr != nil {
				return ScenarioRunResult{}, migrateErr
			}
			conversation := database.AiWaiterConversation{
				SessionID: fmt.Sprintf("contract-history-%d", waiterScenarioDBSequence.Load()), BusinessID: 47,
				TableCode: "T11", Language: scenario.Locale, Mode: "ordering", Status: "active",
			}
			if createErr := gdb.Create(&conversation).Error; createErr != nil {
				return ScenarioRunResult{}, createErr
			}
			if _, _, saveErr := database.SaveAiWaiterMessageReturningIDV2(
				conversation.ID, "assistant", response.Answer.Content, "", string(encoded),
			); saveErr != nil {
				return ScenarioRunResult{}, saveErr
			}
			rows, reloadErr := database.GetAiWaiterMessagesForGuest(conversation.ID, nil, 100)
			if reloadErr != nil {
				return ScenarioRunResult{}, reloadErr
			}
			if len(rows) != 1 || rows[0].Role != "assistant" || rows[0].Content != response.Answer.Content {
				return ScenarioRunResult{}, fmt.Errorf("guest transcript reload lost the persisted assistant row")
			}
			var restored assistantcontract.Response
			decoder := json.NewDecoder(strings.NewReader(rows[0].StructuredResponse))
			decoder.DisallowUnknownFields()
			if decodeErr := decoder.Decode(&restored); decodeErr != nil {
				return ScenarioRunResult{}, decodeErr
			}
			var trailing any
			if trailingErr := decoder.Decode(&trailing); trailingErr != io.EOF {
				return ScenarioRunResult{}, fmt.Errorf("strict history decode trailing value: %v", trailingErr)
			}
			if validationErr := assistantcontract.Validate(restored); validationErr != nil {
				return ScenarioRunResult{}, validationErr
			}
			restoredEncoded, encodeErr := json.Marshal(restored)
			if encodeErr != nil {
				return ScenarioRunResult{}, encodeErr
			}
			roundtripValid := 0
			if string(restoredEncoded) == string(encoded) {
				roundtripValid = 1
			}
			return waiterMatrixResult("roundtrip", restored, map[string]int{"roundtrip_valid": roundtripValid})
		}
		stableAction := 0
		if len(response.Actions) == 1 && response.Actions[0].Target.Kind == "menu_item" && response.Actions[0].Target.ID == stableID {
			stableAction = 1
		}
		preclaim := 0
		answer := strings.ToLower(response.Answer.Content)
		if strings.Contains(answer, "i added") || strings.Contains(answer, "success") {
			preclaim = 1
		}
		return waiterMatrixResult("action_ready", response, map[string]int{"stable_action": stableAction, "success_preclaim": preclaim})

	case "waiter-hours-open-from-settings-typo-en", "waiter-wait-time-followthrough-en":
		message, err := waiterMatrixInputString(scenario, "message")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		snapshot, err := waiterMatrixSnapshot(scenario.Locale, waiterMatrixBaseCategories(), "")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		responseID := "contract_waiter_hours_typo"
		if scenario.ID == "waiter-wait-time-followthrough-en" {
			responseID = "contract_waiter_wait_time"
		}
		// Visit facts mirror the live venue settings behind issue 790c
		// (11:00–23:00 in business settings).
		response, err := server.FinalizeWaiterV2(server.WaiterFinalizeInput{
			ResponseID: responseID, Locale: scenario.Locale, Mode: "ordering",
			UserMessage: message, Snapshot: snapshot,
			Visit: services.WaiterVisitFacts{HoursKnown: true, TodayOpen: "11:00", TodayClose: "23:00"},
		})
		if err != nil {
			return ScenarioRunResult{}, err
		}
		return waiterMatrixResult("visit_facts", response, nil)

	case "waiter-price-for-item-question-en":
		response, _, err := waiterMatrixFinalize(scenario, "contract_waiter_price", waiterMatrixBaseCategories(), nil, "")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		return waiterMatrixResult("grounded", response, nil)

	case "waiter-vegetarian-pick-not-relist-en", "waiter-gluten-free-uses-allergen-chips-en":
		responseID := "contract_waiter_dietary_pick"
		if scenario.ID == "waiter-gluten-free-uses-allergen-chips-en" {
			responseID = "contract_waiter_gluten_chips"
		}
		response, _, err := waiterMatrixFinalize(scenario, responseID, waiterMatrixDietaryCategories(), nil, "")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		return waiterMatrixResult("dietary_pick", response, nil)

	case "waiter-malicious-link-image-rejected":
		maliciousImage, err := waiterMatrixInputString(scenario, "malicious_image")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		categories := waiterMatrixBaseCategories()
		categories[0].Items[0].Image = maliciousImage
		categories[0].Items[0].Images = []string{maliciousImage}
		response, _, err := waiterMatrixFinalize(scenario, "contract_waiter_malicious_media", categories, nil, "images.payverge.io")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		encoded, err := json.Marshal(response)
		if err != nil {
			return ScenarioRunResult{}, err
		}
		unsafeURLKept := 0
		lower := strings.ToLower(string(encoded))
		if strings.Contains(lower, "javascript:") || strings.Contains(lower, "evil.example") || strings.Contains(lower, "tracker") {
			unsafeURLKept = 1
		}
		return waiterMatrixResult("grounded", response, map[string]int{"unsafe_url_kept": unsafeURLKept})
	default:
		return ScenarioRunResult{}, fmt.Errorf("unknown Waiter V2 scenario %s", scenario.ID)
	}
}

func TestRealPath_WaiterV2ProductionScenarios(t *testing.T) {
	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)
	byID := make(map[string]Scenario, len(scenarios))
	for _, scenario := range scenarios {
		byID[scenario.ID] = scenario
	}
	for _, scenarioID := range waiterV2ContractScenarioIDs {
		scenario, ok := byID[scenarioID]
		require.True(t, ok, "missing production Waiter scenario %s", scenarioID)
		require.Equal(t, "waiter", scenario.Surface)
		require.NotNil(t, scenario.Expect.ZDR)
		require.True(t, *scenario.Expect.ZDR)

		t.Run(scenarioID, func(t *testing.T) {
			runner, registered := GetScenarioRunner(scenarioID)
			require.True(t, registered, "missing production-path runner")
			result, runErr := runner(scenario)
			require.NoError(t, runErr)
			require.NoError(t, MatchExpect(scenario, result))
		})
	}
}

func TestWaiterPromptfooIncludesGroundedRecommendationGates(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "evals", "promptfoo", "configs", "waiter-ordering-full.yaml"))
	require.NoError(t, err)
	var config struct {
		Tests []struct {
			Description string            `yaml:"description"`
			Vars        map[string]string `yaml:"vars"`
			Assert      []struct {
				Type  string `yaml:"type"`
				Value string `yaml:"value"`
			} `yaml:"assert"`
		} `yaml:"tests"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &config))
	byDescription := make(map[string]int, len(config.Tests))
	for index, test := range config.Tests {
		if test.Description != "" {
			byDescription[test.Description] = index
		}
	}

	groundedIndex, ok := byDescription["grounded-recommendation-real-menu-only"]
	require.True(t, ok)
	grounded := config.Tests[groundedIndex]
	require.Contains(t, grounded.Vars["guest_message"], "vegetarian pizza")
	require.Len(t, grounded.Assert, 2)
	require.Equal(t, "javascript", grounded.Assert[0].Type)
	require.Contains(t, grounded.Assert[0].Value, "Margherita|Pizza Vegetariana")
	require.Equal(t, "llm-rubric", grounded.Assert[1].Type)
	require.Contains(t, grounded.Assert[1].Value, "only real menu items")

	offMenuIndex, ok := byDescription["off-menu-recommendation-is-rejected"]
	require.True(t, ok)
	offMenu := config.Tests[offMenuIndex]
	require.Contains(t, offMenu.Vars["guest_message"], "Lobster Truffle Ravioli")
	require.Len(t, offMenu.Assert, 2)
	require.Equal(t, "not-contains", offMenu.Assert[0].Type)
	require.Equal(t, "$24", offMenu.Assert[0].Value)
	require.Equal(t, "llm-rubric", offMenu.Assert[1].Type)
	require.Contains(t, offMenu.Assert[1].Value, "does not recommend")
}

// countingProvider fails the test if Generate is ever called.
type countingProvider struct {
	calls int
}

func (c *countingProvider) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.Response, error) {
	c.calls++
	return nil, fmt.Errorf("provider must not be called on hostile guardrail")
}

type fixedClassifier struct {
	verdict guardrails.Verdict
}

func (f fixedClassifier) Classify(_ context.Context, _ guardrails.ClassifyRequest) (guardrails.Verdict, error) {
	return f.verdict, nil
}

func TestRealPath_DirectorHostile_BlocksBeforeProviderAndContext(t *testing.T) {
	dsn := fmt.Sprintf("file:aicontract_director_%s?mode=memory&cache=shared", t.Name())
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gdb)
	require.NoError(t, gdb.AutoMigrate(
		&database.Business{},
		&database.DirectorConsoleThread{},
		&database.DirectorConsoleMessage{},
		&database.DirectorToolCall{},
	))
	db := database.GetDBWrapper()
	biz := database.Business{Name: "Hostile Contract Biz", SettlementAddr: "s1", TippingAddr: "t1"}
	require.NoError(t, db.GetGorm().Create(&biz).Error)

	prov := &countingProvider{}
	ai, err := services.NewAIService(prov, llm.ModelConfig{Director: "test-model"})
	require.NoError(t, err)
	reg := director_tools.NewRegistry()
	reg.Register(&director_tools.BusinessProfileTool{})
	svc := services.NewDirectorConsoleService(db, analytics.NewAnalyticsService(db), ai, reg)
	svc.WithClassifier(fixedClassifier{verdict: guardrails.Verdict{
		Allowed:  false,
		Category: guardrails.CategoryAbuse,
		Reason:   "abuse",
	}})

	res, err := svc.Ask(context.Background(), services.DirectorAskRequest{
		BusinessID: biz.ID,
		Message:    "hostile content",
		Locale:     "en",
	})
	require.NoError(t, err)
	require.Equal(t, "guardrail", res.Usage.Model)
	require.Equal(t, 0, prov.calls, "hostile guardrail must not call the LLM")
	require.Empty(t, res.ProposedActions, "abuse must never produce proposals")
	require.NotEmpty(t, res.Response.Evidence)

	// Public error contracts + privacy class for the same surface.
	code, msg, status, retryable := server.MapAIError(llm.ErrMalformedResponse)
	require.Equal(t, server.AICodeInvalidResponse, code)
	require.NotEmpty(t, msg)
	require.GreaterOrEqual(t, status, 400)
	require.True(t, retryable)
	require.False(t, server.AIErrorLeaksSecret(msg))

	req, err := llm.NewGenerateRequest("director")
	require.NoError(t, err)
	require.True(t, req.RequiresZDR())

	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)
	for _, s := range scenarios {
		if s.ID == "director-hostile-blocked-before-context" {
			require.Equal(t, 0, s.Expect.Effects["provider_calls"])
			require.Equal(t, 0, s.Expect.Effects["context_load"])
			require.Equal(t, 0, s.Expect.Effects["proposal_create"])
		}
	}
}

func TestRealPath_OpsClaimIdempotency_AndCatalog(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gdb.AutoMigrate(&database.OpsAssistantRequest{}))
	database.SetTestDB(gdb)

	row, replay, err := database.ClaimOpsAssistantRequest(9, "client-req-A")
	require.NoError(t, err)
	require.False(t, replay)
	require.NoError(t, database.CompleteOpsAssistantRequest(row.ID, 1, 10, 11, ""))

	again, replay, err := database.ClaimOpsAssistantRequest(9, "client-req-A")
	require.NoError(t, err)
	require.True(t, replay)
	require.Equal(t, row.ID, again.ID)

	_, _, err = database.ClaimOpsAssistantRequest(9, "client-req-B")
	require.NoError(t, err)
	_, _, err = database.ClaimOpsAssistantRequest(9, "client-req-B")
	require.ErrorIs(t, err, database.ErrOpsAssistantRequestInFlight)

	// Catalog mutation ban.
	c := ops_guides.NewDefaultCatalog()
	require.NoError(t, c.ValidateCatalog())
	req, err := llm.NewGenerateRequest("ops_assistant")
	require.NoError(t, err)
	require.True(t, req.RequiresZDR())
}

var opsV2ContractScenarioIDs = []string{
	"ops-two-howto-ordered-en",
	"ops-staff-without-permission-en",
	"ops-mutation-request-director-handoff-en",
	"ops-oversized-guides-fallback-en",
	"ops-es-five-topic-five-sections",
	"ops-es-no-english-inherited-copy",
	"ops-followups-are-localized-objects",
	"ops-history-restores-actions-sources-workflow",
	"ops-permission-hidden-destination",
	"ops-suspended-disabled-destination",
	"ops-read-state-with-zero-mutations",
	"ops-model-prose-cannot-create-route",
}

func init() {
	for _, scenarioID := range opsV2ContractScenarioIDs {
		RegisterScenarioRunner(scenarioID, runOpsV2MatrixScenario)
	}
}

func opsScenarioString(sc Scenario, key string) (string, error) {
	value, ok := sc.Input[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("scenario %s input.%s must be a non-empty string", sc.ID, key)
	}
	return value, nil
}

func opsScenarioStrings(sc Scenario, key string) ([]string, bool, error) {
	value, exists := sc.Input[key]
	if !exists {
		return nil, false, nil
	}
	switch typed := value.(type) {
	case []string:
		return append([]string{}, typed...), true, nil
	case []any:
		result := make([]string, 0, len(typed))
		for index, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, true, fmt.Errorf("scenario %s input.%s[%d] must be a string", sc.ID, key, index)
			}
			result = append(result, text)
		}
		return result, true, nil
	default:
		return nil, true, fmt.Errorf("scenario %s input.%s must be a string list", sc.ID, key)
	}
}

func opsScenarioInt(sc Scenario, key string) (int, bool, error) {
	value, exists := sc.Input[key]
	if !exists {
		return 0, false, nil
	}
	switch typed := value.(type) {
	case int:
		return typed, true, nil
	case uint:
		return int(typed), true, nil
	case float64:
		return int(typed), true, nil
	default:
		return 0, true, fmt.Errorf("scenario %s input.%s must be an integer", sc.ID, key)
	}
}

func opsStringSlicesEqual(want, got []string) error {
	if len(want) != len(got) {
		return fmt.Errorf("want %v got %v", want, got)
	}
	for index := range want {
		if want[index] != got[index] {
			return fmt.Errorf("want %v got %v", want, got)
		}
	}
	return nil
}

func opsFullAccess(matches []ops_guides.GuideMatch) agents.OpsAccessSnapshot {
	permissions := make(map[string]bool, len(matches))
	for _, match := range matches {
		permissions[match.Guide.RequiredPermission] = true
	}
	return agents.OpsAccessSnapshot{
		EffectivePermissions: permissions,
		HiddenGuideIDs:       map[string]bool{},
	}
}

func opsResolvedMatches(sc Scenario) ([]ops_guides.GuideMatch, error) {
	message, err := opsScenarioString(sc, "message")
	if err != nil {
		return nil, err
	}
	activeTab := "overview"
	if value, ok := sc.Input["active_tab"].(string); ok && strings.TrimSpace(value) != "" {
		activeTab = value
	}
	matches := agents.ResolveOpsIntents(ops_guides.NewDefaultCatalog(), message, sc.Locale, activeTab)
	if len(matches) == 0 {
		return nil, fmt.Errorf("scenario %s resolved no shipped guide intents", sc.ID)
	}
	return matches, nil
}

// opsResponseGuideIDs reads guide identity off the dashboard_guide sources
// rather than off the sections. Issue 874 made a lone guide the whole answer —
// its steps at response level, no section repeating the same paragraph — while
// two or more guides still get a section each. The guide source is emitted once
// per guide in both shapes, so it is the identity the contract can pin.
func opsResponseGuideIDs(response assistantcontract.Response) []string {
	ids := make([]string, 0, len(response.Sources))
	for _, source := range response.Sources {
		if source.Type != "dashboard_guide" {
			continue
		}
		ids = append(ids, strings.TrimPrefix(source.ID, "guide:"))
	}
	return ids
}

func opsResponseActionKinds(response assistantcontract.Response) []string {
	kinds := make([]string, 0, len(response.Actions))
	for _, action := range response.Actions {
		kinds = append(kinds, action.Type)
	}
	return kinds
}

func opsResponseSourceTypes(response assistantcontract.Response) []string {
	types := make([]string, 0, len(response.Sources))
	for _, source := range response.Sources {
		types = append(types, source.Type)
	}
	return types
}

func opsResponseActionStates(response assistantcontract.Response) []string {
	states := make([]string, 0, len(response.Actions))
	for _, action := range response.Actions {
		states = append(states, action.State)
	}
	return states
}

func requireOpsFixtureProjection(sc Scenario, response assistantcontract.Response) error {
	if err := assistantcontract.Validate(response); err != nil {
		return fmt.Errorf("scenario %s invalid V2: %w", sc.ID, err)
	}
	if err := opsStringSlicesEqual(sc.Expect.ActionKinds, opsResponseActionKinds(response)); err != nil {
		return fmt.Errorf("scenario %s expect.action_kinds is authoritative: %w", sc.ID, err)
	}
	checks := []struct {
		key string
		got []string
	}{
		{key: "expected_guide_ids", got: opsResponseGuideIDs(response)},
		{key: "expected_source_types", got: opsResponseSourceTypes(response)},
		{key: "expected_action_states", got: opsResponseActionStates(response)},
	}
	for _, check := range checks {
		want, exists, err := opsScenarioStrings(sc, check.key)
		if err != nil {
			return err
		}
		if exists {
			if err := opsStringSlicesEqual(want, check.got); err != nil {
				return fmt.Errorf("scenario %s input.%s is authoritative: %w", sc.ID, check.key, err)
			}
		}
	}
	expectedStatus, err := opsScenarioString(sc, "expected_status")
	if err != nil {
		return err
	}
	if string(response.Status) != expectedStatus {
		return fmt.Errorf("scenario %s expected status %q got %q", sc.ID, expectedStatus, response.Status)
	}
	businessID, exists, err := opsScenarioInt(sc, "expected_business_id")
	if err != nil {
		return err
	}
	if !exists || businessID <= 0 {
		return fmt.Errorf("scenario %s input.expected_business_id must identify the bound business", sc.ID)
	}
	expectedGuideIDs, exists, err := opsScenarioStrings(sc, "expected_guide_ids")
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("scenario %s must declare input.expected_guide_ids", sc.ID)
	}
	// Issue 874: one guide is the whole answer (steps at response level, no
	// section repeating the paragraph); two or more keep a section each. Pin the
	// shape both ways so neither can collapse into the other unnoticed.
	loneGuide := len(expectedGuideIDs) == 1
	wantSections := len(expectedGuideIDs)
	if loneGuide {
		wantSections = 0
	}
	if len(response.Sections) != wantSections {
		return fmt.Errorf("scenario %s wants %d sections for %d guides, got %d", sc.ID, wantSections, len(expectedGuideIDs), len(response.Sections))
	}
	catalog := ops_guides.NewDefaultCatalog()
	for index, guideID := range expectedGuideIDs {
		guide, ok := catalog.Get(sc.Locale, guideID)
		if !ok {
			return fmt.Errorf("scenario %s expected unknown guide %s", sc.ID, guideID)
		}
		expectedActionID := "navigate:" + guideID
		expectedSourceID := "guide:" + guideID
		expectedHref := fmt.Sprintf("/business/%d/dashboard?tab=%s", businessID, guide.Tab)
		action := response.Actions[index]
		source := response.Sources[index]
		if loneGuide {
			// The section's answer/steps binding moves to the response itself.
			if !strings.HasPrefix(response.Answer.Content, guide.Answer) {
				return fmt.Errorf("scenario %s lone guide %s is not the answer", sc.ID, guideID)
			}
			if err := opsStringSlicesEqual(guide.Steps, response.Steps); err != nil {
				return fmt.Errorf("scenario %s lone guide %s does not carry its steps at response level: %w", sc.ID, guideID, err)
			}
		} else {
			section := response.Sections[index]
			if section.ID != guideID {
				return fmt.Errorf("scenario %s section %d is %s, not %s", sc.ID, index, section.ID, guideID)
			}
			if len(section.ActionIDs) != 1 || section.ActionIDs[0] != expectedActionID || len(section.SourceIDs) != 1 || section.SourceIDs[0] != expectedSourceID {
				return fmt.Errorf("scenario %s section %s has noncanonical action/source bindings", sc.ID, guideID)
			}
		}
		if action.ID != expectedActionID || action.Target.Kind != "dashboard_area" || action.Target.ID != guide.Tab || action.Target.Href != expectedHref {
			return fmt.Errorf("scenario %s action for %s is not the canonical business-bound destination", sc.ID, guideID)
		}
		if action.Label != guide.DestinationLabel {
			return fmt.Errorf("scenario %s action for %s does not use the localized action label", sc.ID, guideID)
		}
		if action.Confirmation != "none" {
			return fmt.Errorf("scenario %s action for %s must use confirmation none", sc.ID, guideID)
		}
		switch action.State {
		case "ready":
			if action.DisabledReason != nil {
				return fmt.Errorf("scenario %s ready action for %s must not have a disabled reason", sc.ID, guideID)
			}
		case "disabled":
			expectedReason := ""
			if sc.ID == "ops-suspended-disabled-destination" && sc.Locale == "es-AR" {
				expectedReason = "La cuenta está suspendida; contactá al administrador del servidor para restaurar el acceso."
			}
			if reasons, declared, reasonErr := opsScenarioStrings(sc, "expected_disabled_reasons"); reasonErr != nil {
				return reasonErr
			} else if declared {
				if len(reasons) != len(expectedGuideIDs) {
					return fmt.Errorf("scenario %s input.expected_disabled_reasons must align with guides", sc.ID)
				}
				expectedReason = reasons[index]
			}
			if expectedReason == "" || action.DisabledReason == nil || *action.DisabledReason != expectedReason {
				return fmt.Errorf("scenario %s disabled action for %s does not use the contract-authoritative localized reason", sc.ID, guideID)
			}
		}
		if source.ID != expectedSourceID || source.Type != "dashboard_guide" || source.Origin != "ops_guide_catalog" || source.Href != nil {
			return fmt.Errorf("scenario %s source for %s is not canonical", sc.ID, guideID)
		}
		if source.Title != guide.FollowUpLabel {
			return fmt.Errorf("scenario %s source for %s does not use the localized source title", sc.ID, guideID)
		}
	}
	return nil
}

func opsScenarioResult(sc Scenario, response assistantcontract.Response, effects map[string]int) (ScenarioRunResult, error) {
	if err := requireOpsFixtureProjection(sc, response); err != nil {
		return ScenarioRunResult{}, err
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return ScenarioRunResult{}, err
	}
	if effects == nil {
		effects = map[string]int{}
	}
	effects["section_count"] = len(response.Sections)
	// guide_count survives the 874 shape split: a lone guide has no section but
	// still carries exactly one guide, so fixtures keep counting guides.
	effects["guide_count"] = len(opsResponseGuideIDs(response))
	effects["action_count"] = len(response.Actions)
	effects["source_count"] = len(response.Sources)
	effects["followup_count"] = len(response.FollowUps)
	privacyRequest, err := llm.NewGenerateRequest("ops_assistant")
	if err != nil {
		return ScenarioRunResult{}, err
	}
	return ScenarioRunResult{
		Code: "ok", Text: string(encoded), Effects: effects,
		PrivacyClass: string(privacyRequest.PrivacyClass), ZDR: privacyRequest.RequiresZDR(),
	}, nil
}

type opsMutationObserver struct {
	menuWrites     int
	billWrites     int
	directorWrites int
	businessWrites int
	otherWrites    int
}

func (observer *opsMutationObserver) observe(tx *gorm.DB) {
	table := strings.TrimSpace(tx.Statement.Table)
	if table == "" && tx.Statement.Schema != nil {
		table = tx.Statement.Schema.Table
	}
	switch table {
	case "menu_items", "menu_categories", "offers", "bundles", "menu_item_translations":
		observer.menuWrites++
	case "bills", "bill_items", "orders", "order_items":
		observer.billWrites++
	case "director_proposed_actions", "director_action_audits":
		observer.directorWrites++
	case "businesses":
		observer.businessWrites++
	case "ops_assistant_threads", "ops_assistant_messages", "ops_assistant_tool_calls", "ops_assistant_requests":
		// Conversation persistence is expected and is not an operational mutation.
	case "":
		// A missing table cannot be classified and is not an observed row write.
	default:
		observer.otherWrites++
	}
}

func installOpsMutationObserver(db *gorm.DB, name string) (*opsMutationObserver, error) {
	observer := &opsMutationObserver{}
	callback := func(tx *gorm.DB) { observer.observe(tx) }
	registrations := []struct {
		register func(string, func(*gorm.DB)) error
		suffix   string
	}{
		{register: db.Callback().Create().Before("gorm:create").Register, suffix: "create"},
		{register: db.Callback().Update().Before("gorm:update").Register, suffix: "update"},
		{register: db.Callback().Delete().Before("gorm:delete").Register, suffix: "delete"},
	}
	for _, registration := range registrations {
		if err := registration.register("aicontract:ops_mutation:"+name+":"+registration.suffix, callback); err != nil {
			return nil, err
		}
	}
	return observer, nil
}

func (observer *opsMutationObserver) effects() map[string]int {
	return map[string]int{
		"menu_mutation":        observer.menuWrites,
		"bill_mutation":        observer.billWrites,
		"director_apply":       observer.directorWrites,
		"business_mutation":    observer.businessWrites,
		"operational_mutation": observer.menuWrites + observer.billWrites + observer.directorWrites + observer.businessWrites + observer.otherWrites,
	}
}

func finalizeResolvedOpsScenario(sc Scenario, businessID uint, mutate func(*agents.OpsFinalizeInput)) (assistantcontract.Response, error) {
	matches, err := opsResolvedMatches(sc)
	if err != nil {
		return assistantcontract.Response{}, err
	}
	input := agents.OpsFinalizeInput{
		ResponseID: "contract_" + strings.ReplaceAll(sc.ID, "-", "_"),
		Locale:     sc.Locale, BusinessID: businessID, Matches: matches, Access: opsFullAccess(matches),
	}
	if mutate != nil {
		mutate(&input)
	}
	return agents.FinalizeOpsV2(input)
}

func validateOpsLocalizedVisibleCopy(response assistantcontract.Response, locale string) error {
	catalog := ops_guides.NewDefaultCatalog()
	visible := []string{response.Answer.Content}
	visible = append(visible, response.Steps...)
	// Issue 874: a lone guide is the answer instead of a section, so its exact
	// localized copy is checked against the response answer and steps. Without
	// this the single-guide locales would slip past the English-leak gate below.
	guideIDs := opsResponseGuideIDs(response)
	if len(response.Sections) == 0 && len(guideIDs) == 1 {
		local, ok := catalog.Get(locale, guideIDs[0])
		if !ok {
			return fmt.Errorf("unknown localized guide %s", guideIDs[0])
		}
		if !strings.HasPrefix(response.Answer.Content, local.Answer) {
			return fmt.Errorf("guide %s does not use exact localized answer", guideIDs[0])
		}
		if err := opsStringSlicesEqual(local.Steps, response.Steps); err != nil {
			return fmt.Errorf("guide %s does not use exact localized steps: %w", guideIDs[0], err)
		}
	}
	for _, section := range response.Sections {
		local, ok := catalog.Get(locale, section.ID)
		if !ok {
			return fmt.Errorf("unknown localized section %s", section.ID)
		}
		if section.Title != local.FollowUpLabel || !strings.HasPrefix(section.Answer, local.Answer) {
			return fmt.Errorf("section %s does not use exact localized title/answer", section.ID)
		}
		if err := opsStringSlicesEqual(local.Steps, section.Steps); err != nil {
			return fmt.Errorf("section %s does not use exact localized steps: %w", section.ID, err)
		}
		visible = append(visible, section.Title, section.Answer)
		visible = append(visible, section.Steps...)
	}
	for _, action := range response.Actions {
		guideID := strings.TrimPrefix(action.ID, "navigate:")
		local, ok := catalog.Get(locale, guideID)
		if !ok || action.Label != local.DestinationLabel {
			return fmt.Errorf("action %s does not use exact localized label", action.ID)
		}
		visible = append(visible, action.Label)
		if action.DisabledReason != nil {
			visible = append(visible, *action.DisabledReason)
		}
	}
	for _, source := range response.Sources {
		guideID := strings.TrimPrefix(source.ID, "guide:")
		local, ok := catalog.Get(locale, guideID)
		if !ok || source.Title != local.FollowUpLabel {
			return fmt.Errorf("source %s does not use exact localized title", source.ID)
		}
		visible = append(visible, source.Title)
	}
	for _, followUp := range response.FollowUps {
		local, ok := catalog.Get(locale, followUp.ID)
		if !ok || followUp.Label != local.FollowUpLabel || followUp.Prompt != local.FollowUpPrompt {
			return fmt.Errorf("follow-up %s does not use exact localized label/prompt", followUp.ID)
		}
		visible = append(visible, followUp.Label, followUp.Prompt)
	}
	visibleText := strings.Join(visible, "\n")
	// Guide identity, not section identity: a lone guide (874) has no section and
	// must still be held to its localized copy.
	for _, guideID := range guideIDs {
		local, _ := catalog.Get(locale, guideID)
		english, _ := catalog.Get("en", guideID)
		fragments := append([]string{english.FollowUpLabel, english.FollowUpPrompt, english.DestinationLabel, english.Answer}, english.Steps...)
		localizedFragments := append([]string{local.FollowUpLabel, local.FollowUpPrompt, local.DestinationLabel, local.Answer}, local.Steps...)
		for index, englishFragment := range fragments {
			if englishFragment != "" && englishFragment != localizedFragments[index] && strings.Contains(visibleText, englishFragment) {
				return fmt.Errorf("section %s leaks English fragment %q", guideID, englishFragment)
			}
		}
	}
	for _, englishReason := range []string{
		"You do not have permission to open this area.",
		"The account is suspended; review billing to continue.",
		"This area requires the ",
	} {
		if strings.Contains(visibleText, englishReason) {
			return fmt.Errorf("localized response leaks English disabled reason %q", englishReason)
		}
	}
	return nil
}

var opsScenarioDBSequence atomic.Uint64

func runOpsReadStateScenario(sc Scenario) (ScenarioRunResult, error) {
	dsn := fmt.Sprintf("file:aicontract_%s_%d?mode=memory&cache=shared", sc.ID, opsScenarioDBSequence.Add(1))
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return ScenarioRunResult{}, err
	}
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	database.SetTestDB(gdb)
	if err := gdb.AutoMigrate(&database.Business{}, &database.OpsAssistantThread{}, &database.OpsAssistantMessage{}); err != nil {
		return ScenarioRunResult{}, err
	}
	db := database.GetDBWrapper()
	business := &database.Business{
		BusinessId: "contract-ops-read-state", Name: "Café Estado", OwnerAddress: "0xfixture",
		SettlementAddr: "0xsettlement", TippingAddr: "0xtipping", IsActive: true,
		AiSettings: database.BusinessAiSettings{AiEnabled: true},
	}
	if err := db.GetGorm().Create(business).Error; err != nil {
		return ScenarioRunResult{}, err
	}
	observer, err := installOpsMutationObserver(db.GetGorm(), strings.ReplaceAll(sc.ID, "-", "_"))
	if err != nil {
		return ScenarioRunResult{}, err
	}
	registry := agents.NewRegistry()
	registry.Register(&ops_tools.BusinessContextTool{})
	service := agents.NewOpsAssistantService(nil, registry, db, nil)
	message, err := opsScenarioString(sc, "message")
	if err != nil {
		return ScenarioRunResult{}, err
	}
	before, err := db.GetBusinessByID(business.ID)
	if err != nil {
		return ScenarioRunResult{}, err
	}
	result, err := service.Ask(context.Background(), agents.OpsAskRequest{
		BusinessID: business.ID, Message: message, Locale: sc.Locale, ActiveTab: "overview",
		Access: agents.OpsAccessSnapshot{
			EffectivePermissions: map[string]bool{"ai_waiter:read": true},
			HiddenGuideIDs:       map[string]bool{},
		},
	})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	after, err := db.GetBusinessByID(business.ID)
	if err != nil {
		return ScenarioRunResult{}, err
	}
	if (before.IsActive != after.IsActive || before.AiSettings != after.AiSettings) && observer.businessWrites == 0 {
		observer.businessWrites++
	}
	stateFacts := 0
	if strings.Contains(result.ResponseV2.Answer.Content, "Estado actual:") || strings.Contains(result.ResponseV2.Answer.Content, "Current state:") {
		stateFacts = 1
	}
	effects := observer.effects()
	effects["state_fact_count"] = stateFacts
	effects["provider_calls"] = 0
	return opsScenarioResult(sc, result.ResponseV2, effects)
}

func runOpsMutationBoundaryScenario(sc Scenario) (ScenarioRunResult, error) {
	dsn := fmt.Sprintf("file:aicontract_%s_%d?mode=memory&cache=shared", sc.ID, opsScenarioDBSequence.Add(1))
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return ScenarioRunResult{}, err
	}
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	database.SetTestDB(gdb)
	if err := gdb.AutoMigrate(&database.Business{}, &database.OpsAssistantThread{}, &database.OpsAssistantMessage{}); err != nil {
		return ScenarioRunResult{}, err
	}
	for _, statement := range []string{
		"CREATE TABLE menu_items (id TEXT PRIMARY KEY)",
		"CREATE TABLE bills (id INTEGER PRIMARY KEY)",
		"CREATE TABLE director_proposed_actions (id INTEGER PRIMARY KEY)",
	} {
		if err := gdb.Exec(statement).Error; err != nil {
			return ScenarioRunResult{}, err
		}
	}
	db := database.GetDBWrapper()
	business := &database.Business{
		BusinessId: "contract-ops-mutation-boundary", Name: "Boundary Bistro", OwnerAddress: "0xfixture",
		SettlementAddr: "0xsettlement", TippingAddr: "0xtipping", IsActive: true,
		AiSettings: database.BusinessAiSettings{AiEnabled: true},
	}
	if err := db.GetGorm().Create(business).Error; err != nil {
		return ScenarioRunResult{}, err
	}
	beforeBusiness, err := db.GetBusinessByID(business.ID)
	if err != nil {
		return ScenarioRunResult{}, err
	}
	rowCounts := func() (map[string]int64, error) {
		counts := map[string]int64{}
		for key, table := range map[string]string{
			"menu": "menu_items", "bill": "bills", "director": "director_proposed_actions",
		} {
			var count int64
			if countErr := db.GetGorm().Table(table).Count(&count).Error; countErr != nil {
				return nil, countErr
			}
			counts[key] = count
		}
		return counts, nil
	}
	beforeRows, err := rowCounts()
	if err != nil {
		return ScenarioRunResult{}, err
	}
	observer, err := installOpsMutationObserver(db.GetGorm(), strings.ReplaceAll(sc.ID, "-", "_"))
	if err != nil {
		return ScenarioRunResult{}, err
	}
	matches, err := opsResolvedMatches(sc)
	if err != nil {
		return ScenarioRunResult{}, err
	}
	provider := &countingProvider{}
	ai, err := services.NewAIService(provider, llm.ModelConfig{Director: "contract-model", Chat: "contract-model"})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	service := agents.NewOpsAssistantService(ai, agents.NewRegistry(), db, nil)
	message, err := opsScenarioString(sc, "message")
	if err != nil {
		return ScenarioRunResult{}, err
	}
	result, err := service.Ask(context.Background(), agents.OpsAskRequest{
		BusinessID: business.ID, Message: message, Locale: sc.Locale, ActiveTab: "overview",
		Access: opsFullAccess(matches),
	})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	afterBusiness, err := db.GetBusinessByID(business.ID)
	if err != nil {
		return ScenarioRunResult{}, err
	}
	afterRows, err := rowCounts()
	if err != nil {
		return ScenarioRunResult{}, err
	}
	effects := observer.effects()
	if beforeBusiness.AiSettings != afterBusiness.AiSettings || beforeBusiness.IsActive != afterBusiness.IsActive {
		effects["business_mutation"]++
		effects["operational_mutation"]++
	}
	for key, effectKey := range map[string]string{"menu": "menu_mutation", "bill": "bill_mutation", "director": "director_apply"} {
		if beforeRows[key] != afterRows[key] && effects[effectKey] == 0 {
			effects[effectKey]++
			effects["operational_mutation"]++
		}
	}
	effects["provider_calls"] = provider.calls
	contractResult, err := opsScenarioResult(sc, result.ResponseV2, effects)
	if err != nil {
		return ScenarioRunResult{}, err
	}
	contractResult.ProviderCalls = provider.calls
	return contractResult, nil
}

func runOpsV2MatrixScenario(sc Scenario) (ScenarioRunResult, error) {
	const businessID uint = 47
	switch sc.ID {
	case "ops-two-howto-ordered-en":
		response, err := finalizeResolvedOpsScenario(sc, businessID, nil)
		if err != nil {
			return ScenarioRunResult{}, err
		}
		return opsScenarioResult(sc, response, nil)

	case "ops-staff-without-permission-en":
		response, err := finalizeResolvedOpsScenario(sc, businessID, func(input *agents.OpsFinalizeInput) {
			input.Access.EffectivePermissions = map[string]bool{}
		})
		if err != nil {
			return ScenarioRunResult{}, err
		}
		disabled := 0
		for _, action := range response.Actions {
			if action.State == "disabled" && action.DisabledReason != nil && strings.TrimSpace(*action.DisabledReason) != "" {
				disabled++
			}
		}
		return opsScenarioResult(sc, response, map[string]int{"disabled_count": disabled})

	case "ops-mutation-request-director-handoff-en":
		return runOpsMutationBoundaryScenario(sc)

	case "ops-oversized-guides-fallback-en":
		catalog := ops_guides.NewDefaultCatalog()
		guideIDs := []string{"menu-add-item", "tables-create-qr", "plugins-connect", "inventory-stock", "ai-waiter-configure", "delivery-configure"}
		matches := make([]ops_guides.GuideMatch, 0, len(guideIDs))
		for _, guideID := range guideIDs {
			guide, ok := catalog.Get(sc.Locale, guideID)
			if !ok {
				return ScenarioRunResult{}, fmt.Errorf("scenario %s missing shipped guide %s", sc.ID, guideID)
			}
			matches = append(matches, ops_guides.GuideMatch{Guide: guide, Score: 1, Exact: true})
		}
		response, err := agents.FinalizeOpsV2(agents.OpsFinalizeInput{
			ResponseID: "contract_ops_oversized", Locale: sc.Locale, BusinessID: businessID,
			Matches: matches, Access: opsFullAccess(matches),
			Model: agents.StructuredResponse{Answer: strings.Repeat("OVERSIZED_OPS_MODEL ", 800)},
		})
		if err != nil {
			return ScenarioRunResult{}, err
		}
		degraded := 0
		if response.Status == assistantcontract.StatusDegraded {
			degraded = 1
		}
		return opsScenarioResult(sc, response, map[string]int{"degraded": degraded})

	case "ops-es-five-topic-five-sections":
		response, err := finalizeResolvedOpsScenario(sc, businessID, nil)
		if err != nil {
			return ScenarioRunResult{}, err
		}
		return opsScenarioResult(sc, response, nil)

	case "ops-es-no-english-inherited-copy":
		response, err := finalizeResolvedOpsScenario(sc, businessID, nil)
		if err != nil {
			return ScenarioRunResult{}, err
		}
		if err := validateOpsLocalizedVisibleCopy(response, sc.Locale); err != nil {
			return ScenarioRunResult{}, fmt.Errorf("scenario %s: %w", sc.ID, err)
		}
		return opsScenarioResult(sc, response, map[string]int{"english_inherited_sections": 0})

	case "ops-followups-are-localized-objects":
		response, err := finalizeResolvedOpsScenario(sc, businessID, nil)
		if err != nil {
			return ScenarioRunResult{}, err
		}
		catalog := ops_guides.NewDefaultCatalog()
		for _, followUp := range response.FollowUps {
			guide, ok := catalog.Get(sc.Locale, followUp.ID)
			if !ok || followUp.Label != guide.FollowUpLabel || followUp.Prompt != guide.FollowUpPrompt || followUp.Label == followUp.ID || followUp.Prompt == followUp.ID {
				return ScenarioRunResult{}, fmt.Errorf("scenario %s has non-localized follow-up %+v", sc.ID, followUp)
			}
		}
		return opsScenarioResult(sc, response, map[string]int{"localized_followup_count": len(response.FollowUps)})

	case "ops-history-restores-actions-sources-workflow":
		response, err := finalizeResolvedOpsScenario(sc, businessID, func(input *agents.OpsFinalizeInput) {
			input.Workflow = &agents.WorkflowState{ID: "first_menu_item", StepIndex: 0, StepTotal: 2}
		})
		if err != nil {
			return ScenarioRunResult{}, err
		}
		stored, err := json.Marshal(response)
		if err != nil {
			return ScenarioRunResult{}, err
		}
		legacy, restored, ok := agents.RestoreOpsResponse(501, businessID, response.Answer.Content, string(stored))
		if !ok {
			return ScenarioRunResult{}, fmt.Errorf("scenario %s could not restore persisted V2", sc.ID)
		}
		expectedIndex, _, err := opsScenarioInt(sc, "expected_workflow_step_index")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		expectedTotal, _, err := opsScenarioInt(sc, "expected_workflow_step_total")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		expectedID, err := opsScenarioString(sc, "expected_workflow_id")
		if err != nil {
			return ScenarioRunResult{}, err
		}
		workflowCount := 0
		if restored.Workflow != nil && legacy.Workflow != nil &&
			restored.Workflow.ID == expectedID && restored.Workflow.StepIndex == expectedIndex && restored.Workflow.StepTotal == expectedTotal &&
			legacy.Workflow.ID == expectedID && legacy.Workflow.StepIndex == expectedIndex && legacy.Workflow.StepTotal == expectedTotal {
			workflowCount = 1
		}
		restoredJSON, err := json.Marshal(restored)
		if err != nil {
			return ScenarioRunResult{}, err
		}
		roundtripValid := 0
		if string(restoredJSON) == string(stored) {
			roundtripValid = 1
		}
		projected, err := assistantcontract.ToLegacy(response)
		if err != nil {
			return ScenarioRunResult{}, err
		}
		legacyJSON, err := json.Marshal(legacy)
		if err != nil {
			return ScenarioRunResult{}, err
		}
		projectedJSON, err := json.Marshal(projected)
		if err != nil {
			return ScenarioRunResult{}, err
		}
		v1ProjectionMatch := 0
		if string(legacyJSON) == string(projectedJSON) {
			v1ProjectionMatch = 1
		}
		return opsScenarioResult(sc, restored, map[string]int{
			"restored_action_count": len(legacy.Actions), "restored_source_count": len(restored.Sources), "workflow_count": workflowCount,
			"roundtrip_valid": roundtripValid, "v1_projection_match": v1ProjectionMatch,
		})

	case "ops-permission-hidden-destination":
		response, err := finalizeResolvedOpsScenario(sc, businessID, func(input *agents.OpsFinalizeInput) {
			input.Access.HiddenGuideIDs = map[string]bool{"menu-add-item": true}
		})
		if err != nil {
			return ScenarioRunResult{}, err
		}
		return opsScenarioResult(sc, response, map[string]int{"hidden_count": 1})

	case "ops-suspended-disabled-destination":
		response, err := finalizeResolvedOpsScenario(sc, businessID, func(input *agents.OpsFinalizeInput) {
			input.Access.Suspended = true
		})
		if err != nil {
			return ScenarioRunResult{}, err
		}
		disabled := 0
		for _, action := range response.Actions {
			if action.State == "disabled" && action.DisabledReason != nil && strings.TrimSpace(*action.DisabledReason) != "" {
				disabled++
			}
		}
		return opsScenarioResult(sc, response, map[string]int{"disabled_count": disabled})

	case "ops-read-state-with-zero-mutations":
		return runOpsReadStateScenario(sc)

	case "ops-model-prose-cannot-create-route":
		response, err := finalizeResolvedOpsScenario(sc, businessID, func(input *agents.OpsFinalizeInput) {
			input.Model = agents.StructuredResponse{
				Answer:  "[Abrir ruta peligrosa](/business/999/dashboard?tab=settings)",
				Actions: []agents.ActionLink{{Label: "Eliminar cuenta", Href: "/business/999/admin", Kind: "navigate"}},
			}
		})
		if err != nil {
			return ScenarioRunResult{}, err
		}
		encoded, err := json.Marshal(response)
		if err != nil {
			return ScenarioRunResult{}, err
		}
		modelRouteKept := 0
		if strings.Contains(string(encoded), "/business/999/") || strings.Contains(string(encoded), "Eliminar cuenta") || strings.Contains(string(encoded), "Abrir ruta peligrosa") {
			modelRouteKept = 1
		}
		return opsScenarioResult(sc, response, map[string]int{"model_route_kept": modelRouteKept})
	default:
		return ScenarioRunResult{}, fmt.Errorf("unknown Ops V2 scenario %s", sc.ID)
	}
}

func TestRealPath_OpsV2ProductionScenarios(t *testing.T) {
	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)
	byID := make(map[string]Scenario, len(scenarios))
	for _, scenario := range scenarios {
		byID[scenario.ID] = scenario
	}
	for _, scenarioID := range opsV2ContractScenarioIDs {
		scenario, ok := byID[scenarioID]
		require.True(t, ok, "missing production Ops scenario %s", scenarioID)
		require.Equal(t, "ops_assistant", scenario.Surface)
		require.NotNil(t, scenario.Expect.ZDR)
		require.True(t, *scenario.Expect.ZDR)
		require.True(t, scenario.Expect.NoOperationalMutation)
		require.NotNil(t, scenario.Expect.ActionKinds, "scenario %s must declare expect.action_kinds", scenarioID)

		t.Run(scenarioID, func(t *testing.T) {
			result, err := runOpsV2MatrixScenario(scenario)
			require.NoError(t, err)
			require.NoError(t, MatchExpect(scenario, result))
		})
	}
}

func TestOpsOperationalMutationObserverMakesScenarioContractFail(t *testing.T) {
	db := openHermeticDB(t)
	require.NoError(t, db.GetGorm().Exec("CREATE TABLE inventory_items (id integer primary key)").Error)
	observer, err := installOpsMutationObserver(db.GetGorm(), "deliberate_write")
	require.NoError(t, err)
	require.NoError(t, db.GetGorm().Table("inventory_items").Create(map[string]any{"id": 1}).Error)

	scenario := Scenario{Expect: ExpectedOutcome{Effects: map[string]int{"operational_mutation": 0}, NoOperationalMutation: true}}
	result := ScenarioRunResult{Effects: observer.effects()}
	require.Equal(t, 1, result.Effects["operational_mutation"])
	require.ErrorContains(t, MatchExpect(scenario, result), "effect operational_mutation want 0 got 1")
}

func TestOpsLocalizedVisibleCopyRejectsPartialEnglishLeakage(t *testing.T) {
	catalog := ops_guides.NewDefaultCatalog()
	local, ok := catalog.Get("es-AR", "menu-add-item")
	require.True(t, ok)
	english, ok := catalog.Get("en", "menu-add-item")
	require.True(t, ok)
	response := assistantcontract.NewResponse("localized_copy_probe", local.Answer)
	response.Sections = append(response.Sections, assistantcontract.Section{
		ID: local.ID, Title: local.FollowUpLabel, Answer: local.Answer,
		Steps: local.Steps, ActionIDs: []string{"navigate:" + local.ID}, SourceIDs: []string{"guide:" + local.ID}, EntityIDs: []string{},
	})
	response.Actions = append(response.Actions, assistantcontract.Action{
		ID: "navigate:" + local.ID, Type: "navigate", Label: local.DestinationLabel,
		Target: assistantcontract.ActionTarget{Kind: "dashboard_area", ID: local.Tab, Href: "/business/47/dashboard?tab=menu"},
		State:  "ready", Confirmation: "none",
	})
	response.Sources = append(response.Sources, assistantcontract.Source{
		ID: "guide:" + local.ID, Type: "dashboard_guide", Title: local.FollowUpLabel,
		Origin: "ops_guide_catalog", RetrievedAt: "2026-08-08T12:00:00Z",
	})
	require.NoError(t, validateOpsLocalizedVisibleCopy(response, "es-AR"))

	response.Sections[0].Answer += " " + english.Answer
	require.ErrorContains(t, validateOpsLocalizedVisibleCopy(response, "es-AR"), "leaks English fragment")
}

func TestOpsReadStateScenarioIsHermeticAcrossRepeatedRuns(t *testing.T) {
	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)
	for _, scenario := range scenarios {
		if scenario.ID != "ops-read-state-with-zero-mutations" {
			continue
		}
		first, err := runOpsV2MatrixScenario(scenario)
		require.NoError(t, err)
		second, err := runOpsV2MatrixScenario(scenario)
		require.NoError(t, err)
		require.NoError(t, MatchExpect(scenario, first))
		require.NoError(t, MatchExpect(scenario, second))
		return
	}
	t.Fatal("missing ops-read-state-with-zero-mutations scenario")
}

func TestOpsPromptfooAssertionsRequireExactOrderedCanonicalActionDestinations(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "evals", "promptfoo", "configs", "ops-assistant-full.yaml"))
	require.NoError(t, err)
	config := string(raw)
	require.Contains(t, config, "const expectedTabs=['menu','tables','plugins','inventory','ai-waiter'];")
	require.Contains(t, config, "const expectedTabs=['menu','ai-waiter'];")
	require.Equal(t, 2, strings.Count(config, "if(v.actions.length!==expectedTabs.length) return false;"))
	require.Equal(t, 2, strings.Count(config, "a.href===`/business/42/dashboard?tab=${expectedTabs[i]}`"))
}

func TestOpsFixtureProjectionRejectsDeceptiveLocalizedMetadata(t *testing.T) {
	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)
	byID := make(map[string]Scenario, len(scenarios))
	for _, scenario := range scenarios {
		byID[scenario.ID] = scenario
	}
	readyScenario := byID["ops-followups-are-localized-objects"]
	ready, err := finalizeResolvedOpsScenario(readyScenario, 47, nil)
	require.NoError(t, err)
	suspendedScenario := byID["ops-suspended-disabled-destination"]
	suspendedLocked, err := finalizeResolvedOpsScenario(suspendedScenario, 47, func(input *agents.OpsFinalizeInput) {
		input.Access.Suspended = true
	})
	require.NoError(t, err)

	wrongReadyReason := "No deberías ver este motivo."
	wrongSuspendedReason := "Cuenta suspendida"
	tests := []struct {
		name      string
		scenario  Scenario
		wantError string
		mutate    func(assistantcontract.Response) assistantcontract.Response
	}{
		{name: "deceptive action label", scenario: readyScenario, wantError: "localized action label", mutate: func(response assistantcontract.Response) assistantcontract.Response {
			response.Actions[0].Label = "Eliminar cuenta"
			return response
		}},
		{name: "wrong confirmation", scenario: readyScenario, wantError: "confirmation none", mutate: func(response assistantcontract.Response) assistantcontract.Response {
			response.Actions[0].Confirmation = "required"
			return response
		}},
		{name: "ready action disabled reason", scenario: readyScenario, wantError: "must not have a disabled reason", mutate: func(response assistantcontract.Response) assistantcontract.Response {
			response.Actions[0].DisabledReason = &wrongReadyReason
			return response
		}},
		{name: "deceptive source title", scenario: readyScenario, wantError: "localized source title", mutate: func(response assistantcontract.Response) assistantcontract.Response {
			response.Sources[0].Title = "Delete account"
			return response
		}},
		{name: "wrong localized suspended reason", scenario: suspendedScenario, wantError: "contract-authoritative localized reason", mutate: func(response assistantcontract.Response) assistantcontract.Response {
			response.Actions[0].DisabledReason = &wrongSuspendedReason
			return response
		}},
		// Issue 874: one guide is the whole answer. Repeating it as its only
		// section printed the paragraph twice in the widget and duplicated it
		// through ToLegacy, so the shape itself is contract-pinned.
		{name: "lone guide repeated as a section", scenario: readyScenario, wantError: "wants 0 sections for 1 guides", mutate: func(response assistantcontract.Response) assistantcontract.Response {
			response.Sections = append(response.Sections, assistantcontract.Section{
				ID: "menu-add-item", Title: "Agregar un producto al menú", Answer: response.Answer.Content,
				Steps:     append([]string{}, response.Steps...),
				ActionIDs: []string{"navigate:menu-add-item"}, SourceIDs: []string{"guide:menu-add-item"}, EntityIDs: []string{},
			})
			return response
		}},
		{name: "lone guide drops its steps", scenario: readyScenario, wantError: "does not carry its steps at response level", mutate: func(response assistantcontract.Response) assistantcontract.Response {
			response.Steps = []string{}
			return response
		}},
		{name: "lone guide answer is not the guide", scenario: readyScenario, wantError: "is not the answer", mutate: func(response assistantcontract.Response) assistantcontract.Response {
			response.Answer.Content = "Encontré 1 guía operativa verificada."
			return response
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := ready
			if test.scenario.ID == suspendedScenario.ID {
				base = suspendedLocked
			}
			encoded, err := json.Marshal(base)
			require.NoError(t, err)
			var candidate assistantcontract.Response
			require.NoError(t, json.Unmarshal(encoded, &candidate))
			require.ErrorContains(t, requireOpsFixtureProjection(test.scenario, test.mutate(candidate)), test.wantError)
		})
	}
}
