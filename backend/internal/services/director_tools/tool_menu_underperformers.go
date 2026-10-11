package director_tools

import (
	"context"
	"fmt"
	"sort"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// MenuUnderperformersTool surfaces the worst-revenue items inside the
// "premium" price tier — items whose unit price sits above the menu
// median yet underperform on revenue. The model uses this to phrase
// "your $32 wagyu sold only 10 portions" suggestions without
// flagging cheap volume drivers as failures.
//
// Implementation note (deviation from plan §2.8): analytics.ItemStats
// does not carry a `unit_price` field — only AveragePrice (revenue /
// quantity). We treat AveragePrice as the effective unit price and
// expose it as `unit_price` in the response so the model and the UI
// can pivot on a single key.
type MenuUnderperformersTool struct {
	Items PopularItemsProvider
}

// Name is the snake_case function identifier sent to the model.
func (t *MenuUnderperformersTool) Name() string { return "get_menu_underperformers" }

// HumanLabel is the localized pill label the UI shows while the tool runs.
func (t *MenuUnderperformersTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Buscando platos premium con bajo desempeño"
	case "fr":
		return "Recherche d'articles premium peu performants"
	case "ar":
		return "البحث عن العناصر المميزة منخفضة الأداء"
	default:
		return "Reading menu underperformers"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *MenuUnderperformersTool) Description() string {
	return "Returns premium-priced menu items (priced above the menu median) that under-perform on revenue. Call for menu pruning, repricing candidates, or 'which expensive dishes aren't selling'. For top sellers use get_menu_top_items."
}

// Schema declares the argument shape the model sees.
func (t *MenuUnderperformersTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"period": {
				Type:        llm.TypeString,
				Description: "Time window. One of: day, week, month. Default: week.",
				Enum:        []string{"day", "week", "month"},
			},
			"limit": {
				Type:        llm.TypeInteger,
				Description: "Number of underperformers to return (1-50). Default: 5.",
			},
		},
	}
}

// Run validates args, fetches all items, filters to those priced above
// the menu median, sorts ascending by revenue (worst first), and trims
// to the requested limit.
func (t *MenuUnderperformersTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	// Production registers this tool bare (see main.go); the struct field is a
	// test seam. Fall back to the analytics service carried on the env. Guard
	// the concrete-pointer nil so we never wrap a typed nil into the interface.
	provider := t.Items
	if provider == nil && env.Analytics != nil {
		provider = env.Analytics
	}
	if provider == nil {
		return ToolResult{}, fmt.Errorf("get_menu_underperformers: no popular-items provider wired")
	}

	period, err := normalizeMenuPeriod("get_menu_underperformers", args)
	if err != nil {
		return ToolResult{}, err
	}
	limit, err := normalizeMenuLimit("get_menu_underperformers", args)
	if err != nil {
		return ToolResult{}, err
	}

	analyticsPeriod := period
	if period == "day" {
		analyticsPeriod = "today"
	}

	// 0 = no SQL limit: underperformers need all items to compute the median.
	rows, err := provider.GetPopularItems(env.BusinessID, 0, analyticsPeriod, env.Location)
	if err != nil {
		return ToolResult{}, fmt.Errorf("get_menu_underperformers: %w", err)
	}

	median := medianUnitPrice(rows)
	premium := make([]analytics.ItemStats, 0, len(rows))
	for _, r := range rows {
		if r.AveragePrice > median {
			premium = append(premium, r)
		}
	}

	sort.SliceStable(premium, func(i, j int) bool {
		return premium[i].Revenue < premium[j].Revenue
	})
	if len(premium) > limit {
		premium = premium[:limit]
	}

	items := make([]map[string]any, 0, len(premium))
	for _, r := range premium {
		items = append(items, map[string]any{
			"name":       r.ItemName,
			"quantity":   r.TotalSold,
			"revenue":    r.Revenue,
			"unit_price": r.AveragePrice,
		})
	}

	return ToolResult{
		Summary: fmt.Sprintf("%d underperformers vs. price tier", len(items)),
		Data: map[string]any{
			"period": period,
			"limit":  limit,
			"items":  items,
		},
	}, nil
}

// medianUnitPrice returns the median of the AveragePrice field across
// the input rows. Returns 0 for an empty slice.
func medianUnitPrice(rows []analytics.ItemStats) float64 {
	if len(rows) == 0 {
		return 0
	}
	prices := make([]float64, len(rows))
	for i, r := range rows {
		prices[i] = r.AveragePrice
	}
	sort.Float64s(prices)
	mid := len(prices) / 2
	if len(prices)%2 == 1 {
		return prices[mid]
	}
	return (prices[mid-1] + prices[mid]) / 2
}
