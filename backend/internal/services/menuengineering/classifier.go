// Package menuengineering classifies menu items into the classic
// menu-engineering quadrants (Star/Plowhorse/Puzzle/Dog) from a food-cost
// Report. It is a pure, side-effect-free consumer of foodcost.Report — no DB,
// no I/O — so the same classification feeds the endpoint, the Sage tool, and
// any future digest without duplicated math.
package menuengineering

import (
	"math"
	"sort"

	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
)

type Quadrant string

const (
	QuadrantStar      Quadrant = "star"
	QuadrantPlowhorse Quadrant = "plowhorse"
	QuadrantPuzzle    Quadrant = "puzzle"
	QuadrantDog       Quadrant = "dog"
)

// sparseThreshold is the minimum count of complete-cost items for the median
// splits to be meaningful; below it the UI warns the split is not informative.
const sparseThreshold = 4

// maxUpliftMultiplier caps a plowhorse's suggested price at 1.5× its current
// average price. The naive target (unitCost / medianFoodCostPct) explodes to
// 2–2.4× when an item's food-cost % sits far above the menu median, producing
// suggestions no operator would trust (e.g. $42 → $99.54). Capping at 1.5×
// keeps the nudge aggressive-but-plausible — a single reprice pass an operator
// can defend to guests — while the median-target formula still applies below
// the cap. 1.5 (not 1.25) leaves room for genuinely underpriced plowhorses.
const maxUpliftMultiplier = 1.5

type DishClass struct {
	MenuItemID     string   `json:"menu_item_id"`
	MenuItemName   string   `json:"menu_item_name"`
	FoodCostPct    float64  `json:"food_cost_pct"` // 0..1
	QtySold        int      `json:"qty_sold"`
	AvgPrice       float64  `json:"avg_price"`       // dollars
	UnitCost       float64  `json:"unit_cost"`       // dollars
	MarginPerUnit  float64  `json:"margin_per_unit"` // dollars
	Quadrant       Quadrant `json:"quadrant"`
	Action         string   `json:"action"`          // protect | reprice_up | promote | cut
	SuggestedPrice float64  `json:"suggested_price"` // dollars; 0 when N/A
}

type QuadrantRollup struct {
	Quadrant     Quadrant `json:"quadrant"`
	Count        int      `json:"count"`
	RevenueShare float64  `json:"revenue_share"` // 0..1 of classified revenue
}

type Report struct {
	Period            string           `json:"period"`
	MedianFoodCostPct float64          `json:"median_food_cost_pct"`
	MedianQtySold     float64          `json:"median_qty_sold"`
	Dishes            []DishClass      `json:"dishes"`
	Rollups           []QuadrantRollup `json:"rollups"`
	ItemsNeedingCost  int              `json:"items_needing_cost"`
	Sparse            bool             `json:"sparse"`
	// HasSales passes through foodcost.Report.HasSales: true only when the
	// window recorded at least one sold unit. ItemsNeedingCost > 0 alone does
	// NOT imply sales happened — consumers must key any "sales are recorded"
	// copy on this flag, not on ItemsNeedingCost (#834).
	HasSales bool `json:"has_sales"`
}

func actionFor(q Quadrant) string {
	switch q {
	case QuadrantStar:
		return "protect"
	case QuadrantPlowhorse:
		return "reprice_up"
	case QuadrantPuzzle:
		return "promote"
	default:
		return "cut"
	}
}

// Classify partitions the food-cost Report's complete-cost items by the median
// food-cost% (margin axis) and median units sold (popularity axis). Medians are
// taken over sold items only (QtySold > 0 and AvgPrice > 0) so unsold zeros
// cannot collapse both splits to 0 and mint ghost stars (#858). Items at a
// median land on the favorable side (>= popularity, <= food-cost%). QtySold==0
// is never high-popularity (0 >= 0 must not count), so unsold rows land in
// puzzle/dog. Items missing complete cost cannot be placed on the margin axis
// and are counted in ItemsNeedingCost.
//
// Dishes preserves the input Report.Items ordering (food-cost% desc, then
// MenuItemID, as emitted by foodcost.Analyze); callers must not assume any
// other order.
func Classify(r foodcost.Report) Report {
	out := Report{Period: r.Period, Dishes: []DishClass{}, Rollups: []QuadrantRollup{}, HasSales: r.HasSales}

	complete := make([]foodcost.ItemMargin, 0, len(r.Items))
	for _, it := range r.Items {
		if it.HasCompleteCost {
			complete = append(complete, it)
		} else {
			out.ItemsNeedingCost++
		}
	}

	out.Sparse = len(complete) < sparseThreshold
	if len(complete) == 0 {
		return out
	}

	sold := make([]foodcost.ItemMargin, 0, len(complete))
	for _, it := range complete {
		if it.QtySold > 0 && it.AvgPrice > 0 {
			sold = append(sold, it)
		}
	}
	if len(sold) > 0 {
		out.MedianFoodCostPct = medianFloat(mapFloat(sold, func(i foodcost.ItemMargin) float64 { return i.FoodCostPct }))
		out.MedianQtySold = medianFloat(mapFloat(sold, func(i foodcost.ItemMargin) float64 { return float64(i.QtySold) }))
	}

	var totalRev float64
	counts := map[Quadrant]int{}
	rev := map[Quadrant]float64{}

	for _, it := range complete {
		highMargin := it.FoodCostPct <= out.MedianFoodCostPct
		highPop := it.QtySold > 0 && float64(it.QtySold) >= out.MedianQtySold
		var q Quadrant
		switch {
		case highMargin && highPop:
			q = QuadrantStar
		case !highMargin && highPop:
			q = QuadrantPlowhorse
		case highMargin && !highPop:
			q = QuadrantPuzzle
		default:
			q = QuadrantDog
		}

		dc := DishClass{
			MenuItemID: it.MenuItemID, MenuItemName: it.MenuItemName,
			FoodCostPct: it.FoodCostPct, QtySold: it.QtySold,
			AvgPrice: it.AvgPrice, UnitCost: it.UnitCost, MarginPerUnit: it.MarginPerUnit,
			Quadrant: q, Action: actionFor(q),
		}
		if q == QuadrantPlowhorse && it.UnitCost > 0 && out.MedianFoodCostPct > 0 {
			// Clamp the median-target uplift so the suggestion never exceeds
			// maxUpliftMultiplier× the current price (see the const doc).
			target := math.Min(it.UnitCost/out.MedianFoodCostPct, it.AvgPrice*maxUpliftMultiplier)
			dc.SuggestedPrice = round2(math.Max(it.AvgPrice, target))
		}
		out.Dishes = append(out.Dishes, dc)

		itemRev := it.AvgPrice * float64(it.QtySold)
		totalRev += itemRev
		counts[q]++
		rev[q] += itemRev
	}

	for _, q := range []Quadrant{QuadrantStar, QuadrantPlowhorse, QuadrantPuzzle, QuadrantDog} {
		if counts[q] == 0 {
			continue
		}
		share := 0.0
		if totalRev > 0 {
			share = rev[q] / totalRev
		}
		out.Rollups = append(out.Rollups, QuadrantRollup{Quadrant: q, Count: counts[q], RevenueShare: share})
	}
	return out
}

func mapFloat(items []foodcost.ItemMargin, f func(foodcost.ItemMargin) float64) []float64 {
	out := make([]float64, len(items))
	for i, it := range items {
		out[i] = f(it)
	}
	return out
}

func medianFloat(v []float64) float64 {
	n := len(v)
	if n == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }
