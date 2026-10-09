package director_tools

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- stubs for the three extra wastevariance seams ----

type stubMovementProvider struct {
	rows []database.MovementAggregate
}

func (s *stubMovementProvider) AggregateInventoryMovements(_ uint, _, _ time.Time) ([]database.MovementAggregate, error) {
	return s.rows, nil
}

type stubItemDimProvider struct {
	rows []database.InventoryItemDim
}

func (s *stubItemDimProvider) GetInventoryItemDimsForBusiness(_ uint) ([]database.InventoryItemDim, error) {
	return s.rows, nil
}

type stubWindowProvider struct {
	start time.Time
	end   time.Time
}

func (s *stubWindowProvider) ParsePeriodWindow(_ string, _ *time.Location) (time.Time, time.Time, error) {
	return s.start, s.end, nil
}

// ---- tests ----

func TestWasteVarianceTool_Metadata(t *testing.T) {
	tool := &WasteVarianceTool{}
	assert.Equal(t, "get_waste_variance", tool.Name())
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
	assert.NotEmpty(t, tool.HumanLabel("fr"))
	assert.NotEmpty(t, tool.Description())
	schema := tool.Schema()
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties["period"])
	require.NotNil(t, schema.Properties["limit"])
}

// TestWasteVarianceTool_FallsBackToEnvProviders exercises the env.DB /
// env.Analytics fallback path (bare struct, no test seams) against a real
// in-memory DB row. A waste movement dated today falls inside the current-week
// window, so tracked_loss_cost must be non-zero and all required Data fields
// must be present.
func TestWasteVarianceTool_FallsBackToEnvProviders(t *testing.T) {
	db := newTestDB(t)
	fixtureNow := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)

	// InventoryMovement is not in newTestDB's AutoMigrate list — add it here.
	require.NoError(t, db.GetGorm().AutoMigrate(&database.InventoryMovement{}))

	bizID := createTestBusiness(t, db, "Waste Variance Bistro")

	// Seed one active ingredient with a known cost.
	ing := database.InventoryItem{
		BusinessID:  bizID,
		Name:        "tomatoes",
		Unit:        "kg",
		CostPerUnit: 5.0,
		IsActive:    true,
	}
	require.NoError(t, db.GetGorm().Create(&ing).Error)

	// Seed a waste movement dated now — falls within the current week window.
	// QuantityDelta is negative for waste (stock removed).
	require.NoError(t, db.GetGorm().Create(&database.InventoryMovement{
		BusinessID:      bizID,
		InventoryItemID: ing.ID,
		MovementType:    database.InventoryMovementTypeWaste,
		QuantityDelta:   -2.0,
		QuantityBefore:  10.0,
		QuantityAfter:   8.0,
		CreatedAt:       fixtureNow.Add(-time.Minute),
	}).Error)

	tool := &WasteVarianceTool{} // bare → env.DB + env.Analytics fallback
	env := ToolEnv{
		BusinessID: bizID,
		Locale:     "en",
		DB:         db,
		Analytics:  newTestAnalytics(t, db).WithClock(func() time.Time { return fixtureNow }),
		Location:   time.UTC,
	}

	result, err := tool.Run(context.Background(), map[string]any{"period": "week"}, env)
	require.NoError(t, err)
	assert.Equal(t, "week", result.Data["period"])

	// tracked_loss_cost = 2.0 kg × $5.0/kg = $10.0
	lossCost, ok := result.Data["tracked_loss_cost"].(float64)
	require.True(t, ok, "tracked_loss_cost must be float64")
	assert.InDelta(t, 10.0, lossCost, 0.01)

	// Required Data fields the model and UI rely on.
	require.Contains(t, result.Data, "total_variance_cost")
	require.Contains(t, result.Data, "theoretical_usage_cost")
	require.Contains(t, result.Data, "loss_by_reason")
	require.Contains(t, result.Data, "ingredients")
	require.Contains(t, result.Data, "sparse")
	require.Contains(t, result.Data, "items_without_recipe")

	// The single ingredient must surface in the list.
	ingredients, ok := result.Data["ingredients"].([]map[string]any)
	require.True(t, ok, "ingredients must be []map[string]any")
	require.Len(t, ingredients, 1)
	require.Contains(t, ingredients[0], "tracked_loss_cost")
}

// TestWasteVarianceTool_ValidatesPeriod asserts that an unsupported period
// value is rejected before the calculator runs.
func TestWasteVarianceTool_ValidatesPeriod(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bad Period Waste Variance")

	// All five providers wired so the provider check passes before validation.
	tool := &WasteVarianceTool{
		Items:     &stubPopularItemsProvider{},
		Recipes:   &stubRecipeCosts{},
		Movements: &stubMovementProvider{},
		Dims:      &stubItemDimProvider{},
		Window:    &stubWindowProvider{},
	}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	_, err := tool.Run(context.Background(), map[string]any{"period": "fortnight"}, env)
	require.Error(t, err)
}

// ---- #872: the fabricated variance ----
//
// Live repro on 142 Parrilla Quebracho Azul: the Director reported a "varianza
// negativa de 46 kg" on bife while the inventory item read 0 kg. The
// calculator never produced 46; the tool payload did. An ingredient with no
// recipe has UNKNOWN theoretical usage, but the payload emitted
// theoretical_usage: 0 and variance: 0 right next to a real actual_usage
// number — an arithmetic invitation the model accepted. The accounting UI
// (WasteVarianceCard) already renders those cells as "—"; the model was the
// only consumer still shown fabricated zeroes.

// seedRecipelessIngredientWithUsage reproduces the shape: one ingredient with
// real ledger movement and no recipe at all.
func seedRecipelessIngredientWithUsage(t *testing.T, db *database.DB, name string, kg float64) (uint, uint) {
	t.Helper()
	require.NoError(t, db.GetGorm().AutoMigrate(&database.InventoryMovement{}))
	bizID := createTestBusiness(t, db, "Parrilla 872")
	ing := database.InventoryItem{
		BusinessID: bizID, Name: name, Unit: "kg", CostPerUnit: 10.0, IsActive: true,
	}
	require.NoError(t, db.GetGorm().Create(&ing).Error)
	require.NoError(t, db.GetGorm().Create(&database.InventoryMovement{
		BusinessID:      bizID,
		InventoryItemID: ing.ID,
		MovementType:    database.InventoryMovementTypeWaste,
		QuantityDelta:   -kg,
		QuantityBefore:  kg,
		QuantityAfter:   0,
		CreatedAt:       time.Date(2026, time.July, 31, 11, 59, 0, 0, time.UTC),
	}).Error)
	return bizID, ing.ID
}

func TestWasteVarianceTool_OmitsVarianceForIngredientsWithoutARecipe(t *testing.T) {
	db := newTestDB(t)
	fixtureNow := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	bizID, _ := seedRecipelessIngredientWithUsage(t, db, "bife de chorizo", 46.0)

	result, err := (&WasteVarianceTool{}).Run(context.Background(), map[string]any{"period": "week"}, ToolEnv{
		BusinessID: bizID,
		Locale:     "es",
		DB:         db,
		Analytics:  newTestAnalytics(t, db).WithClock(func() time.Time { return fixtureNow }),
		Location:   time.UTC,
	})
	require.NoError(t, err)

	ingredients, ok := result.Data["ingredients"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, ingredients, 1)
	row := ingredients[0]

	require.Equal(t, false, row["has_recipe"], "fixture must have no recipe, otherwise it proves nothing")
	assert.Equal(t, false, row["variance_measurable"])

	// The 46 the model quoted is real consumption — and it must stay, labelled
	// as consumption. What must NOT be there is a variance next to it.
	assert.InDelta(t, 46.0, row["actual_usage"].(float64), 0.001)
	assert.NotContains(t, row, "variance",
		"a recipe-less ingredient has no computable variance; emitting one is the reported invention")
	assert.NotContains(t, row, "variance_cost")
	assert.NotContains(t, row, "theoretical_usage",
		"theoretical usage is UNKNOWN here, not zero — a zero is what the model subtracted from")
	assert.Contains(t, row, "variance_unavailable_reason")
}

func TestWasteVarianceTool_KeepsVarianceForMeasuredIngredients(t *testing.T) {
	tool := &WasteVarianceTool{
		Items: &stubPopularItemsProvider{byPeriod: map[string][]analytics.ItemStats{
			"week": {{ItemID: "m1", TotalSold: 10, RecognizedQuantity: 10}},
		}},
		Recipes: &stubRecipeCosts{rows: []database.RecipeIngredientCost{{
			MenuItemID: "m1", InventoryItemID: 7, QuantityRequired: 1,
		}}},
		Movements: &stubMovementProvider{rows: []database.MovementAggregate{{
			InventoryItemID: 7,
			MovementType:    database.InventoryMovementTypeOrderConsumption,
			TotalDelta:      -12,
		}}},
		Dims:   &stubItemDimProvider{rows: []database.InventoryItemDim{{ID: 7, Name: "bife", Unit: "kg", CostPerUnit: 10}}},
		Window: &stubWindowProvider{start: time.Now().Add(-24 * time.Hour), end: time.Now()},
	}

	result, err := tool.Run(context.Background(), map[string]any{"period": "week"}, ToolEnv{BusinessID: 1, Locale: "en"})
	require.NoError(t, err)

	ingredients := result.Data["ingredients"].([]map[string]any)
	require.Len(t, ingredients, 1)
	row := ingredients[0]

	assert.Equal(t, true, row["variance_measurable"])
	require.Contains(t, row, "variance", "a measured ingredient must still report its real variance")
	assert.InDelta(t, 2.0, row["variance"].(float64), 0.001) // 12 actual − 10 theoretical
	assert.InDelta(t, 10.0, row["theoretical_usage"].(float64), 0.001)
	assert.NotContains(t, row, "variance_unavailable_reason")
	assert.Equal(t, 1, result.Data["measurable_ingredients"])
}

func TestWasteVarianceGuidanceRefusesToLetTheModelInventAQuantity(t *testing.T) {
	// Nothing at all in the window.
	empty := wasteVarianceGuidance(0, 0, 0)
	assert.Contains(t, empty, "NO waste or variance to report")
	assert.Contains(t, empty, "never estimate")

	// The reported case: ingredients exist, none has a recipe.
	none := wasteVarianceGuidance(3, 0, 3)
	assert.Contains(t, none, "NO variance can be computed")
	assert.Contains(t, none, "actual_usage is consumption, not variance")
	assert.Contains(t, none, "never estimate")

	// Mixed: some measurable, some not.
	mixed := wasteVarianceGuidance(5, 2, 3)
	assert.Contains(t, mixed, "ONLY for ingredients where variance_measurable is true")
	assert.Contains(t, mixed, "2 of 5")

	// Fully measurable.
	full := wasteVarianceGuidance(4, 4, 0)
	assert.Contains(t, full, "computable for all 4")
	assert.Contains(t, full, "never estimate")
	assert.NotContains(t, full, "cannot be calculated")
}

func TestWasteVarianceTool_ShipsGuidanceAndMeasurableCount(t *testing.T) {
	db := newTestDB(t)
	fixtureNow := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	bizID, _ := seedRecipelessIngredientWithUsage(t, db, "bife de chorizo", 46.0)

	result, err := (&WasteVarianceTool{}).Run(context.Background(), map[string]any{"period": "week"}, ToolEnv{
		BusinessID: bizID,
		Locale:     "es",
		DB:         db,
		Analytics:  newTestAnalytics(t, db).WithClock(func() time.Time { return fixtureNow }),
		Location:   time.UTC,
	})
	require.NoError(t, err)

	assert.Equal(t, 0, result.Data["measurable_ingredients"])
	guidance, ok := result.Data["guidance"].(string)
	require.True(t, ok, "every waste/variance result must carry the anti-fabrication guidance")
	assert.Contains(t, guidance, "NO variance can be computed")

	// And the Description must tell the model the field exists at all.
	assert.Contains(t, (&WasteVarianceTool{}).Description(), "variance_measurable")
}
