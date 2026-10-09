package director_actions

import (
	"fmt"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// largeSampleMenu returns a realistic menu with n items spread across multiple
// categories. It is used by benchmarks to measure compute throughput at scale.
// Items alternate between two categories (Mains, Drinks) to exercise category
// scoping without hot-spotting a single category path.
func largeSampleMenu(n int) []database.MenuCategory {
	mains := database.MenuCategory{
		ID:          "cat-mains",
		Name:        "Mains",
		Description: "Hot dishes from the grill",
		SortOrder:   0,
	}
	drinks := database.MenuCategory{
		ID:          "cat-drinks",
		Name:        "Drinks",
		Description: "Beverages and cocktails",
		SortOrder:   1,
	}

	for i := 0; i < n; i++ {
		item := database.MenuItem{
			ID:          fmt.Sprintf("item-%d", i),
			Name:        fmt.Sprintf("Item %d", i),
			Price:       float64(5 + i%15), // $5–$19 range — no item starts at $0
			IsAvailable: true,
			DietaryTags: []string{"mild"},
			Allergens:   []string{"gluten"},
		}
		if i%2 == 0 {
			mains.Items = append(mains.Items, item)
		} else {
			drinks.Items = append(drinks.Items, item)
		}
	}
	return []database.MenuCategory{mains, drinks}
}

// BenchmarkComputePriceChange measures the pure-compute path over a 50-item
// menu (scope=all, percent +20%). No I/O; allocations reflect deep-copy cost.
func BenchmarkComputePriceChange(b *testing.B) {
	cats := largeSampleMenu(50)
	p := PriceChangeParams{Scope: "all", Mode: "percent", Value: 20, Direction: "up"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _, _ = ComputePriceChange(cats, p)
	}
}
