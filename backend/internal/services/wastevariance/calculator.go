// Package wastevariance computes a read-only theoretical-vs-actual ingredient
// usage variance + tracked loss, from recipes×sales (theoretical) and the
// inventory-movement ledger (actual). Persists nothing; does not touch the P&L.
package wastevariance

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
)

// Seams (all satisfied by concrete services in production).
type MovementProvider interface {
	AggregateInventoryMovements(businessID uint, start, end time.Time) ([]database.MovementAggregate, error)
}
type ItemDimProvider interface {
	GetInventoryItemDimsForBusiness(businessID uint) ([]database.InventoryItemDim, error)
}
type WindowProvider interface {
	ParsePeriodWindow(period string, loc *time.Location) (time.Time, time.Time, error)
}

type IngredientVariance struct {
	InventoryItemID  uint    `json:"inventory_item_id"`
	Name             string  `json:"name"`
	Unit             string  `json:"unit"`
	CostPerUnit      float64 `json:"cost_per_unit"`
	TheoreticalUsage float64 `json:"theoretical_usage"`
	ActualUsage      float64 `json:"actual_usage"`
	Variance         float64 `json:"variance"`
	VarianceCost     float64 `json:"variance_cost"`
	TrackedLossCost  float64 `json:"tracked_loss_cost"`
	HasRecipe        bool    `json:"has_recipe"`
}

type LossByReason struct {
	Reason string  `json:"reason"`
	Cost   float64 `json:"cost"`
}

type Report struct {
	Period               string               `json:"period"`
	TrackedLossCost      float64              `json:"tracked_loss_cost"`
	TotalVarianceCost    float64              `json:"total_variance_cost"`
	TheoreticalUsageCost float64              `json:"theoretical_usage_cost"`
	LossByReason         []LossByReason       `json:"loss_by_reason"`
	Ingredients          []IngredientVariance `json:"ingredients"`
	ItemsWithoutRecipe   int                  `json:"items_without_recipe"`
	Sparse               bool                 `json:"sparse"`
}

const sparseThreshold = 4

type Calculator struct {
	recipes   foodcost.RecipeCostProvider
	items     foodcost.ItemStatsProvider
	movements MovementProvider
	dims      ItemDimProvider
	window    WindowProvider
}

func NewCalculator(recipes foodcost.RecipeCostProvider, items foodcost.ItemStatsProvider,
	movements MovementProvider, dims ItemDimProvider, window WindowProvider) *Calculator {
	return &Calculator{recipes: recipes, items: items, movements: movements, dims: dims, window: window}
}

func (c *Calculator) Analyze(businessID uint, period string, loc *time.Location) (Report, error) {
	switch period {
	case "":
		period = "week"
	case "day", "week", "month":
	default:
		return Report{}, fmt.Errorf("%w %q (allowed: day, week, month)", foodcost.ErrUnsupportedPeriod, period)
	}
	analyticsPeriod := period
	if period == "day" {
		analyticsPeriod = "today"
	}

	start, end, err := c.window.ParsePeriodWindow(analyticsPeriod, loc)
	if err != nil {
		return Report{}, fmt.Errorf("wastevariance: window: %w", err)
	}
	stats, err := c.items.GetPopularItems(businessID, 0, analyticsPeriod, loc)
	if err != nil {
		return Report{}, fmt.Errorf("wastevariance: popular items: %w", err)
	}
	recipeRows, err := c.recipes.GetRecipeCostsForBusiness(businessID)
	if err != nil {
		return Report{}, fmt.Errorf("wastevariance: recipe costs: %w", err)
	}
	aggs, err := c.movements.AggregateInventoryMovements(businessID, start, end)
	if err != nil {
		return Report{}, fmt.Errorf("wastevariance: movements: %w", err)
	}
	dims, err := c.dims.GetInventoryItemDimsForBusiness(businessID)
	if err != nil {
		return Report{}, fmt.Errorf("wastevariance: item dims: %w", err)
	}

	// analytics keys ItemStats by bill_items.name, recipes by MenuItemID —
	// match by ID first, then by the recipe's MenuItemName (same fallback as
	// foodcost.Analyze) so theoretical usage isn't silently zero in production.
	// Quantity is the unrounded recognized amount, so a half-paid line and a
	// zero-price bundle component both contribute fractional usage.
	sold := make(map[string]float64, len(stats))
	for _, s := range stats {
		sold[s.ItemID] = s.RecognizedQuantity
	}
	soldQty := func(r database.RecipeIngredientCost) float64 {
		if q, ok := sold[r.MenuItemID]; ok {
			return q
		}
		return sold[r.MenuItemName]
	}
	theoretical := make(map[uint]float64)
	for _, r := range recipeRows {
		if q := soldQty(r); q > 0 {
			theoretical[r.InventoryItemID] += r.QuantityRequired * q
		}
	}
	type ledger struct {
		actual    float64
		spoilage  float64
		countDown float64
		manDown   float64
	}
	led := make(map[uint]*ledger)
	getLed := func(id uint) *ledger {
		l := led[id]
		if l == nil {
			l = &ledger{}
			led[id] = l
		}
		return l
	}
	for _, a := range aggs {
		switch a.MovementType {
		case database.InventoryMovementTypePurchase, database.InventoryMovementTypeRestock:
			// procurement — excluded from usage
		default:
			getLed(a.InventoryItemID).actual += -a.TotalDelta
		}
		switch a.MovementType {
		case database.InventoryMovementTypeWaste:
			getLed(a.InventoryItemID).spoilage += math.Abs(a.TotalDelta)
		case database.InventoryMovementTypeCorrection:
			if a.TotalDelta < 0 {
				getLed(a.InventoryItemID).countDown += -a.TotalDelta
			}
		case database.InventoryMovementTypeManualAdjustment:
			if a.TotalDelta < 0 {
				getLed(a.InventoryItemID).manDown += -a.TotalDelta
			}
		}
	}

	dimByID := make(map[uint]database.InventoryItemDim, len(dims))
	for _, d := range dims {
		dimByID[d.ID] = d
	}
	idset := make(map[uint]struct{})
	for id := range theoretical {
		idset[id] = struct{}{}
	}
	for id := range led {
		idset[id] = struct{}{}
	}

	var report Report
	report.Period = period
	report.Ingredients = []IngredientVariance{} // serialize as [] not null (FE maps over it)
	var spoilTot, countTot, manTot float64
	for id := range idset {
		d := dimByID[id]
		l := led[id]
		if l == nil {
			l = &ledger{}
		}
		theo, hasRecipe := theoretical[id]
		iv := IngredientVariance{
			InventoryItemID:  id,
			Name:             d.Name,
			Unit:             d.Unit,
			CostPerUnit:      d.CostPerUnit,
			TheoreticalUsage: theo,
			ActualUsage:      l.actual,
			HasRecipe:        hasRecipe,
			TrackedLossCost:  (l.spoilage + l.countDown + l.manDown) * d.CostPerUnit,
		}
		if hasRecipe {
			iv.Variance = l.actual - theo
			iv.VarianceCost = iv.Variance * d.CostPerUnit
			report.TotalVarianceCost += iv.VarianceCost
			report.TheoreticalUsageCost += theo * d.CostPerUnit
		} else {
			report.ItemsWithoutRecipe++
		}
		spoilTot += l.spoilage * d.CostPerUnit
		countTot += l.countDown * d.CostPerUnit
		manTot += l.manDown * d.CostPerUnit
		report.Ingredients = append(report.Ingredients, iv)
	}
	report.TrackedLossCost = spoilTot + countTot + manTot
	report.LossByReason = nonZeroReasons(spoilTot, countTot, manTot)
	report.Sparse = len(report.Ingredients) < sparseThreshold

	sort.SliceStable(report.Ingredients, func(i, j int) bool {
		a, b := report.Ingredients[i], report.Ingredients[j]
		if a.TrackedLossCost != b.TrackedLossCost {
			return a.TrackedLossCost > b.TrackedLossCost
		}
		if math.Abs(a.VarianceCost) != math.Abs(b.VarianceCost) {
			return math.Abs(a.VarianceCost) > math.Abs(b.VarianceCost)
		}
		return a.InventoryItemID < b.InventoryItemID
	})
	return report, nil
}

func nonZeroReasons(spoilage, countShrink, manual float64) []LossByReason {
	all := []LossByReason{
		{Reason: "spoilage", Cost: spoilage},
		{Reason: "count_shrink", Cost: countShrink},
		{Reason: "manual", Cost: manual},
	}
	out := make([]LossByReason, 0, 3)
	for _, r := range all {
		if r.Cost > 0 {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost > out[j].Cost
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}
