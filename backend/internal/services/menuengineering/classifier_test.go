package menuengineering

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
)

func item(id string, fcPct float64, qty int, price, cost float64) foodcost.ItemMargin {
	return foodcost.ItemMargin{
		MenuItemID: id, MenuItemName: "n-" + id,
		FoodCostPct: fcPct, QtySold: qty, AvgPrice: price, UnitCost: cost,
		MarginPerUnit: price - cost, HasCompleteCost: true,
	}
}

func byID(r Report) map[string]DishClass {
	m := map[string]DishClass{}
	for _, d := range r.Dishes {
		m[d.MenuItemID] = d
	}
	return m
}

func TestClassify_Quadrants(t *testing.T) {
	in := foodcost.Report{Period: "week", Items: []foodcost.ItemMargin{
		item("star", 0.20, 40, 10, 2),
		item("plow", 0.40, 50, 10, 4),
		item("puzz", 0.20, 5, 12, 2.4),
		item("dog", 0.45, 4, 8, 3.6),
		item("mid", 0.30, 20, 10, 3), // exactly on both medians -> favorable -> star
	}}
	got := byID(Classify(in))
	for id, want := range map[string]Quadrant{
		"star": "star", "plow": "plowhorse", "puzz": "puzzle", "dog": "dog", "mid": "star",
	} {
		if got[id].Quadrant != want {
			t.Errorf("%s: quadrant=%q want %q", id, got[id].Quadrant, want)
		}
	}
	if got["star"].Action != "protect" || got["plow"].Action != "reprice_up" ||
		got["puzz"].Action != "promote" || got["dog"].Action != "cut" {
		t.Errorf("action mapping wrong: %+v", got)
	}
}

func TestClassify_SuggestedPriceOnlyForPlowhorse(t *testing.T) {
	in := foodcost.Report{Period: "week", Items: []foodcost.ItemMargin{
		item("a", 0.20, 40, 10, 2),
		item("plow", 0.50, 50, 10, 5),
		item("b", 0.20, 5, 12, 2.4),
		item("c", 0.50, 4, 8, 4),
	}}
	got := byID(Classify(in))
	// median food-cost% = 0.35; suggested = max(10, 5/0.35=14.29)
	if math.Abs(got["plow"].SuggestedPrice-14.29) > 0.01 {
		t.Errorf("plow suggested=%v want ~14.29", got["plow"].SuggestedPrice)
	}
	if got["a"].SuggestedPrice != 0 || got["b"].SuggestedPrice != 0 {
		t.Errorf("non-plowhorse should have suggested 0")
	}
}

// TestClassify_SuggestedPriceClampedAtMaxUplift pins the uplift cap: when an
// item's food-cost % sits far above the menu median, the naive target
// (unitCost/medianFoodCostPct) would blow past 1.5× the current price. The
// clamp must hold the suggestion at avgPrice*maxUpliftMultiplier.
func TestClassify_SuggestedPriceClampedAtMaxUplift(t *testing.T) {
	in := foodcost.Report{Period: "week", Items: []foodcost.ItemMargin{
		item("a", 0.20, 40, 10, 2),
		item("b", 0.22, 30, 10, 2.2),
		item("c", 0.24, 20, 10, 2.4),
		// extreme plowhorse: FC%=0.90, cost=$9 on a $10 price. median≈0.24,
		// naive target = 9/0.24 = 37.5 → clamp to 10*1.5 = 15.
		item("plow", 0.90, 50, 10, 9),
	}}
	got := byID(Classify(in))
	plow := got["plow"]
	if plow.Quadrant != QuadrantPlowhorse {
		t.Fatalf("plow: quadrant=%q want plowhorse", plow.Quadrant)
	}
	upliftCap := plow.AvgPrice * maxUpliftMultiplier
	if plow.SuggestedPrice > upliftCap+0.01 {
		t.Errorf("suggested=%v exceeds cap %v (avgPrice*maxUpliftMultiplier)", plow.SuggestedPrice, upliftCap)
	}
	if math.Abs(plow.SuggestedPrice-15.0) > 0.01 {
		t.Errorf("suggested=%v want clamped to 15.00", plow.SuggestedPrice)
	}
}

func TestClassify_IncompleteCostBucketed(t *testing.T) {
	in := foodcost.Report{Period: "week", Items: []foodcost.ItemMargin{
		item("ok", 0.30, 10, 10, 3),
		{MenuItemID: "missing", QtySold: 99, AvgPrice: 10, HasCompleteCost: false},
	}}
	r := Classify(in)
	if r.ItemsNeedingCost != 1 {
		t.Errorf("ItemsNeedingCost=%d want 1", r.ItemsNeedingCost)
	}
	for _, d := range r.Dishes {
		if d.MenuItemID == "missing" {
			t.Errorf("incomplete-cost item must not be classified")
		}
	}
}

func TestClassify_SparseAndEmpty(t *testing.T) {
	if r := Classify(foodcost.Report{Period: "day"}); len(r.Dishes) != 0 || !r.Sparse {
		t.Errorf("empty menu: want 0 dishes + sparse")
	}
	in := foodcost.Report{Items: []foodcost.ItemMargin{item("x", 0.3, 1, 5, 1.5)}}
	if !Classify(in).Sparse {
		t.Errorf("single-item menu must be sparse")
	}
}

func TestClassify_RevenueShareSumsToOne(t *testing.T) {
	in := foodcost.Report{Items: []foodcost.ItemMargin{
		item("a", 0.20, 40, 10, 2), item("b", 0.40, 50, 10, 4),
		item("c", 0.20, 5, 12, 2.4), item("d", 0.45, 4, 8, 3.6),
	}}
	var sum float64
	for _, rr := range Classify(in).Rollups {
		sum += rr.RevenueShare
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("revenue shares sum=%v want 1", sum)
	}
}

// TestClassify_SingleAxisTies pins each axis tie independently. Five items give
// odd-count medians that land exactly on a chosen item per axis:
//   - food-cost% set {0.20,0.25,0.30,0.40,0.50} -> median 0.30
//   - qty set        {5,10,20,30,40}            -> median 20
//
// "puzz_tie" sits exactly at median food-cost% (favorable margin via <=) but
// below median qty -> puzzle. "plow_tie" sits exactly at median qty (favorable
// pop via >=) but above median food-cost% -> plowhorse.
func TestClassify_SingleAxisTies(t *testing.T) {
	in := foodcost.Report{Period: "week", Items: []foodcost.ItemMargin{
		item("puzz_tie", 0.30, 10, 10, 3), // FC==median, qty<median -> puzzle
		item("plow_tie", 0.40, 20, 10, 4), // qty==median, FC>median  -> plowhorse
		item("a", 0.20, 40, 10, 2),
		item("b", 0.25, 30, 10, 2.5),
		item("c", 0.50, 5, 10, 5),
	}}
	r := Classify(in)
	if r.MedianFoodCostPct != 0.30 {
		t.Fatalf("median food-cost%% = %v want 0.30", r.MedianFoodCostPct)
	}
	if r.MedianQtySold != 20 {
		t.Fatalf("median qty = %v want 20", r.MedianQtySold)
	}
	got := byID(r)
	if got["puzz_tie"].Quadrant != QuadrantPuzzle {
		t.Errorf("puzz_tie: quadrant=%q want puzzle (FC==median, low pop)", got["puzz_tie"].Quadrant)
	}
	if got["plow_tie"].Quadrant != QuadrantPlowhorse {
		t.Errorf("plow_tie: quadrant=%q want plowhorse (qty==median, low margin)", got["plow_tie"].Quadrant)
	}
}

// TestClassify_SparseBoundary asserts the sparse flag in BOTH directions around
// the threshold of 4 complete items.
func TestClassify_SparseBoundary(t *testing.T) {
	three := foodcost.Report{Items: []foodcost.ItemMargin{
		item("a", 0.20, 40, 10, 2), item("b", 0.40, 50, 10, 4), item("c", 0.20, 5, 12, 2.4),
	}}
	if !Classify(three).Sparse {
		t.Errorf("3 complete items: Sparse=false want true")
	}
	four := foodcost.Report{Items: []foodcost.ItemMargin{
		item("a", 0.20, 40, 10, 2), item("b", 0.40, 50, 10, 4),
		item("c", 0.20, 5, 12, 2.4), item("d", 0.45, 4, 8, 3.6),
	}}
	if Classify(four).Sparse {
		t.Errorf("4 complete items: Sparse=true want false")
	}
}

// TestClassify_ZeroRevenueGuard exercises the divide-by-zero guard: when every
// complete item has zero units sold, total classified revenue is 0. Classify
// must not panic, rollups must still carry counts, and every RevenueShare is 0.
// Qty=0 is never high-popularity, so an all-zero menu lands in puzzle/dog
// rather than minting stars from 0>=0 (#858).
func TestClassify_ZeroRevenueGuard(t *testing.T) {
	in := foodcost.Report{Items: []foodcost.ItemMargin{
		item("a", 0.20, 0, 10, 2), item("b", 0.20, 0, 10, 2),
		item("c", 0.50, 0, 10, 5), item("d", 0.50, 0, 10, 5),
	}}
	r := Classify(in)
	if len(r.Rollups) == 0 {
		t.Fatalf("expected populated rollups even with zero revenue")
	}
	var totalCount int
	for _, rr := range r.Rollups {
		if rr.Count <= 0 {
			t.Errorf("rollup %q count=%d want > 0", rr.Quadrant, rr.Count)
		}
		if rr.RevenueShare != 0 {
			t.Errorf("rollup %q RevenueShare=%v want 0 (zero revenue)", rr.Quadrant, rr.RevenueShare)
		}
		if rr.Quadrant == QuadrantStar || rr.Quadrant == QuadrantPlowhorse {
			t.Errorf("rollup %q: zero-sales menu must not be high-pop", rr.Quadrant)
		}
		totalCount += rr.Count
	}
	if totalCount != 4 {
		t.Errorf("classified count=%d want 4", totalCount)
	}
	for _, d := range r.Dishes {
		if d.Quadrant == QuadrantStar || d.Quadrant == QuadrantPlowhorse {
			t.Errorf("%s: quadrant=%q want puzzle/dog (qty=0 is low pop)", d.MenuItemID, d.Quadrant)
		}
	}
}

// TestClassify_DoesNotMutateInput guards purity: Classify must not mutate the
// caller's Report.Items (the three downstream consumers share one Report).
func TestClassify_DoesNotMutateInput(t *testing.T) {
	in := foodcost.Report{Items: []foodcost.ItemMargin{
		item("a", 0.20, 40, 10, 2), item("b", 0.40, 50, 10, 4),
		item("c", 0.20, 5, 12, 2.4), item("d", 0.45, 4, 8, 3.6),
	}}
	type snap struct {
		id    string
		fcPct float64
		qty   int
		price float64
		cost  float64
	}
	before := make([]snap, len(in.Items))
	for i, it := range in.Items {
		before[i] = snap{it.MenuItemID, it.FoodCostPct, it.QtySold, it.AvgPrice, it.UnitCost}
	}
	_ = Classify(in)
	if len(in.Items) != len(before) {
		t.Fatalf("input slice length changed: %d want %d", len(in.Items), len(before))
	}
	for i, it := range in.Items {
		b := before[i]
		if it.MenuItemID != b.id || it.FoodCostPct != b.fcPct || it.QtySold != b.qty ||
			it.AvgPrice != b.price || it.UnitCost != b.cost {
			t.Errorf("input mutated at %d: got %+v want %+v", i, it, b)
		}
	}
}

// O2 topology (#834): a business whose window has items missing costs but ZERO
// recorded sales must not be told "sales are recorded". HasSales passes through
// from the food-cost report so consumers can key empty-state copy honestly.
func TestClassify_HasSalesPassthrough(t *testing.T) {
	noSales := foodcost.Report{Period: "week", HasSales: false, Items: []foodcost.ItemMargin{
		{MenuItemID: "a", MenuItemName: "n-a", HasCompleteCost: false},
		{MenuItemID: "b", MenuItemName: "n-b", HasCompleteCost: false},
	}}
	r := Classify(noSales)
	if r.HasSales {
		t.Fatal("zero-sales window must not report has_sales=true")
	}
	if r.ItemsNeedingCost != 2 {
		t.Fatalf("ItemsNeedingCost = %d, want 2", r.ItemsNeedingCost)
	}

	withSales := foodcost.Report{Period: "week", HasSales: true, Items: []foodcost.ItemMargin{item("x", 0.3, 5, 10, 3)}}
	if got := Classify(withSales); !got.HasSales {
		t.Fatal("sold window must carry has_sales=true through")
	}
}

// unsoldZero matches foodcost's recipe-mapped unsold row: qty/price/FC%/margin
// stay 0 when AvgPrice==0, but HasCompleteCost is true so Classify still places it.
func unsoldZero(id, name string) foodcost.ItemMargin {
	return foodcost.ItemMargin{
		MenuItemID: id, MenuItemName: name,
		QtySold: 0, AvgPrice: 0, FoodCostPct: 0, MarginPerUnit: 0,
		UnitCost: 1.5, HasCompleteCost: true,
	}
}

// #858: a week WITH sales must not treat unsold (qty=0, price=0, FC%=0) rows as
// stars. Including those zeros in both medians pulls MedianQtySold and
// MedianFoodCostPct to 0, so 0>=0 and 0<=0 classifies every ghost as a star
// and every real seller as a plowhorse. Medians must come from sold items;
// zero-sales rows are low-popularity (puzzle/dog), never star/plowhorse.
func TestClassify_ZeroSalesRowsAreNotStarsOnASalesWeek(t *testing.T) {
	in := foodcost.Report{Period: "week", HasSales: true, Items: []foodcost.ItemMargin{
		item("star", 0.20, 40, 10, 2),
		item("plow", 0.40, 50, 10, 4),
		item("puzz", 0.20, 5, 12, 2.4),
		item("dog", 0.45, 4, 8, 3.6),
		unsoldZero("choripan", "Choripán"),
		unsoldZero("ensalada", "Ensalada"),
		unsoldZero("provoleta", "Provoleta"),
		unsoldZero("ghost4", "Ghost 4"),
		unsoldZero("ghost5", "Ghost 5"),
	}}

	r := Classify(in)
	require.True(t, r.HasSales, "week has sold units")
	got := byID(r)

	require.InDelta(t, 0.30, r.MedianFoodCostPct, 1e-9,
		"median food-cost%% must ignore unsold zeros (sold set 0.20,0.20,0.40,0.45)")
	require.InDelta(t, 22.5, r.MedianQtySold, 1e-9,
		"median qty must ignore unsold zeros (sold set 4,5,40,50)")

	for _, id := range []string{"choripan", "ensalada", "provoleta", "ghost4", "ghost5"} {
		d, ok := got[id]
		require.True(t, ok, "unsold %s must still be classified", id)
		assert.NotEqual(t, QuadrantStar, d.Quadrant, "%s qty=0 must not be a star", id)
		assert.NotEqual(t, QuadrantPlowhorse, d.Quadrant, "%s qty=0 must not be a plowhorse", id)
		assert.Contains(t, []Quadrant{QuadrantPuzzle, QuadrantDog}, d.Quadrant,
			"%s qty=0 must land low-pop (puzzle/dog), got %q", id, d.Quadrant)
		assert.NotEqual(t, "protect", d.Action, "%s qty=0 must not get protect", id)
		assert.Zero(t, d.SuggestedPrice, "%s qty=0 suggested_price must stay 0", id)
		assert.Equal(t, 0, d.QtySold)
		assert.Zero(t, d.AvgPrice)
		assert.Zero(t, d.FoodCostPct)
	}

	for id, want := range map[string]Quadrant{
		"star": QuadrantStar, "plow": QuadrantPlowhorse,
		"puzz": QuadrantPuzzle, "dog": QuadrantDog,
	} {
		assert.Equal(t, want, got[id].Quadrant, "%s: sold-only medians", id)
	}
}
