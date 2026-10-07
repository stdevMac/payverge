package wastevariance

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
)

// ---- fakes for the five seams ----

type fakeRecipes struct {
	rows []database.RecipeIngredientCost
	err  error
}

func (f *fakeRecipes) GetRecipeCostsForBusiness(businessID uint) ([]database.RecipeIngredientCost, error) {
	return f.rows, f.err
}

type fakeItems struct {
	rows      []analytics.ItemStats
	err       error
	gotPeriod string
	gotLimit  int
	gotBizID  uint
}

func (f *fakeItems) GetPopularItems(businessID uint, limit int, period string, loc *time.Location) ([]analytics.ItemStats, error) {
	f.gotBizID = businessID
	f.gotLimit = limit
	f.gotPeriod = period
	return f.rows, f.err
}

type fakeMovements struct {
	rows []database.MovementAggregate
	err  error
}

func (f *fakeMovements) AggregateInventoryMovements(businessID uint, start, end time.Time) ([]database.MovementAggregate, error) {
	return f.rows, f.err
}

type fakeDims struct {
	rows []database.InventoryItemDim
	err  error
}

func (f *fakeDims) GetInventoryItemDimsForBusiness(businessID uint) ([]database.InventoryItemDim, error) {
	return f.rows, f.err
}

type fakeWindow struct {
	start     time.Time
	end       time.Time
	err       error
	gotPeriod string
}

func (f *fakeWindow) ParsePeriodWindow(period string, loc *time.Location) (time.Time, time.Time, error) {
	f.gotPeriod = period
	return f.start, f.end, f.err
}

func findIngredient(t *testing.T, r Report, id uint) IngredientVariance {
	t.Helper()
	for _, iv := range r.Ingredients {
		if iv.InventoryItemID == id {
			return iv
		}
	}
	t.Fatalf("ingredient #%d not found in report (%+v)", id, r.Ingredients)
	return IngredientVariance{}
}

const delta = 1e-9

func TestCalculator_Analyze(t *testing.T) {
	cases := []struct {
		name    string
		period  string
		recipes []database.RecipeIngredientCost
		stats   []analytics.ItemStats
		aggs    []database.MovementAggregate
		dims    []database.InventoryItemDim
		check   func(t *testing.T, r Report)
	}{
		{
			// Cases 1, 2, 3, 6 (partial): theoretical aggregation across two
			// recipes, movement-bucket signs, variance + cost, loss-by-reason.
			name:   "theoretical+signs+variance+loss",
			period: "week",
			recipes: []database.RecipeIngredientCost{
				{MenuItemID: "burger", InventoryItemID: 1, QuantityRequired: 2, CostPerUnit: 2.0, HasCost: true},
				{MenuItemID: "taco", InventoryItemID: 1, QuantityRequired: 1, CostPerUnit: 2.0, HasCost: true},
			},
			stats: []analytics.ItemStats{
				{ItemID: "burger", TotalSold: 5, RecognizedQuantity: 5},
				{ItemID: "taco", TotalSold: 3, RecognizedQuantity: 3},
			},
			aggs: []database.MovementAggregate{
				{InventoryItemID: 1, MovementType: database.InventoryMovementTypeOrderConsumption, TotalDelta: -10},
				{InventoryItemID: 1, MovementType: database.InventoryMovementTypeOrderRestoration, TotalDelta: 2},
				{InventoryItemID: 1, MovementType: database.InventoryMovementTypeWaste, TotalDelta: -3},
				{InventoryItemID: 1, MovementType: database.InventoryMovementTypeManualAdjustment, TotalDelta: -1},
				{InventoryItemID: 1, MovementType: database.InventoryMovementTypeCorrection, TotalDelta: -4},
				{InventoryItemID: 1, MovementType: database.InventoryMovementTypePurchase, TotalDelta: 50},
				{InventoryItemID: 1, MovementType: database.InventoryMovementTypeRestock, TotalDelta: 20},
			},
			dims: []database.InventoryItemDim{
				{ID: 1, Name: "Beef", Unit: "kg", CostPerUnit: 2.0},
			},
			check: func(t *testing.T, r Report) {
				require.Equal(t, "week", r.Period)
				iv := findIngredient(t, r, 1)
				require.True(t, iv.HasRecipe)
				// theoretical[#1] = 2*5 + 1*3 = 13
				require.InDelta(t, 13, iv.TheoreticalUsage, delta)
				// actual = -(-10 +2 -3 -1 -4) = 16 (purchase + restock excluded)
				require.InDelta(t, 16, iv.ActualUsage, delta)
				// variance = 16 - 13 = 3
				require.InDelta(t, 3, iv.Variance, delta)
				require.InDelta(t, 6, iv.VarianceCost, delta) // 3 * 2
				// tracked loss = (spoilage 3 + count 4 + manual 1) * 2 = 16
				require.InDelta(t, 16, iv.TrackedLossCost, delta)

				require.InDelta(t, 6, r.TotalVarianceCost, delta)
				require.InDelta(t, 26, r.TheoreticalUsageCost, delta) // 13 * 2
				require.InDelta(t, 16, r.TrackedLossCost, delta)
				require.Equal(t, 0, r.ItemsWithoutRecipe)

				// loss-by-reason desc by cost: count_shrink 8, spoilage 6, manual 2
				require.Len(t, r.LossByReason, 3)
				require.Equal(t, "count_shrink", r.LossByReason[0].Reason)
				require.InDelta(t, 8, r.LossByReason[0].Cost, delta)
				require.Equal(t, "spoilage", r.LossByReason[1].Reason)
				require.InDelta(t, 6, r.LossByReason[1].Cost, delta)
				require.Equal(t, "manual", r.LossByReason[2].Reason)
				require.InDelta(t, 2, r.LossByReason[2].Cost, delta)

				require.True(t, r.Sparse) // 1 ingredient < 4
			},
		},
		{
			// Case 4: positive correction adds NO tracked loss but reduces actual.
			name:   "positive_correction_no_loss",
			period: "week",
			aggs: []database.MovementAggregate{
				{InventoryItemID: 1, MovementType: database.InventoryMovementTypeOrderConsumption, TotalDelta: -10},
				{InventoryItemID: 1, MovementType: database.InventoryMovementTypeCorrection, TotalDelta: 5},
			},
			dims: []database.InventoryItemDim{
				{ID: 1, Name: "Beef", Unit: "kg", CostPerUnit: 2.0},
			},
			check: func(t *testing.T, r Report) {
				iv := findIngredient(t, r, 1)
				require.False(t, iv.HasRecipe)
				// actual = -(-10) + -(5) = 10 - 5 = 5
				require.InDelta(t, 5, iv.ActualUsage, delta)
				require.InDelta(t, 0, iv.TrackedLossCost, delta)
				require.InDelta(t, 0, r.TrackedLossCost, delta)
				require.Empty(t, r.LossByReason)
			},
		},
		{
			// Case 5: movement-only ingredient (no recipe).
			name:   "movement_only_no_recipe",
			period: "week",
			aggs: []database.MovementAggregate{
				{InventoryItemID: 2, MovementType: database.InventoryMovementTypeWaste, TotalDelta: -4},
			},
			dims: []database.InventoryItemDim{
				{ID: 2, Name: "Onion", Unit: "kg", CostPerUnit: 3.0},
			},
			check: func(t *testing.T, r Report) {
				iv := findIngredient(t, r, 2)
				require.False(t, iv.HasRecipe)
				require.InDelta(t, 0, iv.Variance, delta)
				require.InDelta(t, 0, iv.VarianceCost, delta)
				require.InDelta(t, 4, iv.ActualUsage, delta)      // -(-4)
				require.InDelta(t, 12, iv.TrackedLossCost, delta) // 4 * 3
				require.Equal(t, 1, r.ItemsWithoutRecipe)
				require.InDelta(t, 0, r.TotalVarianceCost, delta)
			},
		},
		{
			// Case 6: loss-by-reason omits zero buckets.
			name:   "loss_by_reason_omits_zero",
			period: "week",
			aggs: []database.MovementAggregate{
				{InventoryItemID: 1, MovementType: database.InventoryMovementTypeWaste, TotalDelta: -5},
			},
			dims: []database.InventoryItemDim{
				{ID: 1, Name: "Beef", Unit: "kg", CostPerUnit: 2.0},
			},
			check: func(t *testing.T, r Report) {
				require.Len(t, r.LossByReason, 1)
				require.Equal(t, "spoilage", r.LossByReason[0].Reason)
				require.InDelta(t, 10, r.LossByReason[0].Cost, delta) // 5 * 2
			},
		},
		{
			// Case 7: >= 4 ingredients -> Sparse false; also verifies sort order
			// (tracked-loss desc).
			name:   "not_sparse_and_sorted",
			period: "week",
			aggs: []database.MovementAggregate{
				{InventoryItemID: 1, MovementType: database.InventoryMovementTypeWaste, TotalDelta: -1},
				{InventoryItemID: 2, MovementType: database.InventoryMovementTypeWaste, TotalDelta: -2},
				{InventoryItemID: 3, MovementType: database.InventoryMovementTypeWaste, TotalDelta: -3},
				{InventoryItemID: 4, MovementType: database.InventoryMovementTypeWaste, TotalDelta: -4},
			},
			dims: []database.InventoryItemDim{
				{ID: 1, Name: "A", Unit: "kg", CostPerUnit: 1.0},
				{ID: 2, Name: "B", Unit: "kg", CostPerUnit: 1.0},
				{ID: 3, Name: "C", Unit: "kg", CostPerUnit: 1.0},
				{ID: 4, Name: "D", Unit: "kg", CostPerUnit: 1.0},
			},
			check: func(t *testing.T, r Report) {
				require.False(t, r.Sparse)
				require.Len(t, r.Ingredients, 4)
				// tracked loss desc: #4 (4), #3 (3), #2 (2), #1 (1)
				require.Equal(t, uint(4), r.Ingredients[0].InventoryItemID)
				require.Equal(t, uint(3), r.Ingredients[1].InventoryItemID)
				require.Equal(t, uint(2), r.Ingredients[2].InventoryItemID)
				require.Equal(t, uint(1), r.Ingredients[3].InventoryItemID)
			},
		},
		{
			// Case 8: empty inputs -> no panic, empty report.
			name:   "empty_inputs",
			period: "week",
			check: func(t *testing.T, r Report) {
				require.NotNil(t, r.Ingredients, "must serialize as [] not null")
				require.Empty(t, r.Ingredients)
				require.InDelta(t, 0, r.TrackedLossCost, delta)
				require.InDelta(t, 0, r.TotalVarianceCost, delta)
				require.Empty(t, r.LossByReason)
				require.True(t, r.Sparse)
				require.Equal(t, 0, r.ItemsWithoutRecipe)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := NewCalculator(
				&fakeRecipes{rows: tc.recipes},
				&fakeItems{rows: tc.stats},
				&fakeMovements{rows: tc.aggs},
				&fakeDims{rows: tc.dims},
				&fakeWindow{start: time.Unix(0, 0), end: time.Unix(1000, 0)},
			)
			r, err := c.Analyze(1, tc.period, time.UTC)
			require.NoError(t, err)
			tc.check(t, r)
		})
	}
}

// Case 9: period "day" maps to analytics period "today" for BOTH the item
// stats provider and the window provider.
func TestCalculator_Analyze_DayMapsToToday(t *testing.T) {
	items := &fakeItems{}
	window := &fakeWindow{start: time.Unix(0, 0), end: time.Unix(1000, 0)}
	c := NewCalculator(&fakeRecipes{}, items, &fakeMovements{}, &fakeDims{}, window)

	r, err := c.Analyze(7, "day", time.UTC)
	require.NoError(t, err)
	require.Equal(t, "day", r.Period) // report keeps the canonical period label
	require.Equal(t, "today", items.gotPeriod)
	require.Equal(t, "today", window.gotPeriod)
	require.Equal(t, uint(7), items.gotBizID)
	require.Equal(t, 0, items.gotLimit)
}

// Empty period defaults to "week".
func TestCalculator_Analyze_EmptyDefaultsToWeek(t *testing.T) {
	items := &fakeItems{}
	window := &fakeWindow{}
	c := NewCalculator(&fakeRecipes{}, items, &fakeMovements{}, &fakeDims{}, window)

	r, err := c.Analyze(1, "", time.UTC)
	require.NoError(t, err)
	require.Equal(t, "week", r.Period)
	require.Equal(t, "week", items.gotPeriod)
	require.Equal(t, "week", window.gotPeriod)
}

// Case 10: bad period wraps foodcost.ErrUnsupportedPeriod.
func TestCalculator_Analyze_BadPeriod(t *testing.T) {
	c := NewCalculator(&fakeRecipes{}, &fakeItems{}, &fakeMovements{}, &fakeDims{}, &fakeWindow{})
	_, err := c.Analyze(1, "bogus", time.UTC)
	require.Error(t, err)
	require.True(t, errors.Is(err, foodcost.ErrUnsupportedPeriod))
}

// Production analytics keys ItemStats by bill_items.name, not menu-item ID
// (analytics/service.go: "Use name as ID since we aggregated by name"). The
// theoretical-usage join must also match recipes by MenuItemName or every
// real business reports zero theoretical usage.
func TestCalculator_Analyze_MatchesStatsKeyedByItemName(t *testing.T) {
	c := NewCalculator(
		&fakeRecipes{rows: []database.RecipeIngredientCost{
			{MenuItemID: "demo-burger", MenuItemName: "Burger", InventoryItemID: 1, QuantityRequired: 2, CostPerUnit: 2.0, HasCost: true},
		}},
		&fakeItems{rows: []analytics.ItemStats{
			{ItemID: "Burger", ItemName: "Burger", TotalSold: 5, RecognizedQuantity: 5},
		}},
		&fakeMovements{rows: []database.MovementAggregate{
			{InventoryItemID: 1, MovementType: database.InventoryMovementTypeOrderConsumption, TotalDelta: -10},
		}},
		&fakeDims{rows: []database.InventoryItemDim{
			{ID: 1, Name: "Beef", Unit: "kg", CostPerUnit: 2.0},
		}},
		&fakeWindow{start: time.Unix(0, 0), end: time.Unix(1000, 0)},
	)
	r, err := c.Analyze(1, "week", time.UTC)
	require.NoError(t, err)
	iv := findIngredient(t, r, 1)
	require.True(t, iv.HasRecipe)
	require.InDelta(t, 10, iv.TheoreticalUsage, delta)
}

func TestCalculator_TheoreticalUsageUsesRecognizedQuantity(t *testing.T) {
	recipe := []database.RecipeIngredientCost{
		{MenuItemID: "combo-fries", InventoryItemID: 9, QuantityRequired: 2, CostPerUnit: 1, HasCost: true},
	}
	dims := []database.InventoryItemDim{{ID: 9, Name: "X", Unit: "kg", CostPerUnit: 1}}
	window := &fakeWindow{start: time.Unix(0, 0), end: time.Unix(1000, 0)}

	t.Run("component quantity three", func(t *testing.T) {
		c := NewCalculator(
			&fakeRecipes{rows: recipe},
			&fakeItems{rows: []analytics.ItemStats{
				{ItemID: "combo-fries", TotalSold: 3, RecognizedQuantity: 3},
			}},
			&fakeMovements{},
			&fakeDims{rows: dims},
			window,
		)
		r, err := c.Analyze(1, "week", time.UTC)
		require.NoError(t, err)
		iv := findIngredient(t, r, 9)
		require.InDelta(t, 6, iv.TheoreticalUsage, delta)
	})

	t.Run("half portion is not rounded up to a full unit", func(t *testing.T) {
		c := NewCalculator(
			&fakeRecipes{rows: recipe},
			&fakeItems{rows: []analytics.ItemStats{
				// Display count is 1; usage must follow the 0.5 recognized quantity (2 * 0.5 = 1), not 2.
				{ItemID: "combo-fries", TotalSold: 1, RecognizedQuantity: 0.5},
			}},
			&fakeMovements{},
			&fakeDims{rows: dims},
			window,
		)
		r, err := c.Analyze(1, "week", time.UTC)
		require.NoError(t, err)
		iv := findIngredient(t, r, 9)
		require.InDelta(t, 1.0, iv.TheoreticalUsage, delta)
	})
}
