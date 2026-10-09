package director_tools

import (
	"context"
	"fmt"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// PopularItemsProvider is the analytics-service slice the menu top/under
// tools depend on. The seam lets unit tests inject canned ItemStats
// without seeding the recognized-events CTE the real GetPopularItems
// SQL pipeline drives — see RevenueProvider for the same pattern.
//
// Production wiring satisfies this with the concrete *analytics.AnalyticsService.
type PopularItemsProvider interface {
	GetPopularItems(businessID uint, limit int, period string, loc *time.Location) ([]analytics.ItemStats, error)
}

// MenuTopItemsTool returns the highest-revenue menu items inside a
// day/week/month window. The Director Console uses this to answer
// "what's selling best lately?" without hand-rolling SQL in the prompt.
type MenuTopItemsTool struct {
	Items PopularItemsProvider
}

// Name is the snake_case function identifier sent to the model.
func (t *MenuTopItemsTool) Name() string { return "get_menu_top_items" }

// HumanLabel is the localized pill label the UI shows while the tool runs.
func (t *MenuTopItemsTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Buscando los platos más vendidos"
	case "fr":
		return "Recherche des plats les plus vendus"
	case "ar":
		return "البحث عن أفضل العناصر مبيعًا"
	default:
		return "Reading top menu items"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *MenuTopItemsTool) Description() string {
	return "Returns the best-selling menu items by revenue and quantity for a window. Call for 'what's selling best', popular items, or menu winners. For expensive items that are NOT selling, use get_menu_underperformers instead."
}

// Schema declares the argument shape the model sees.
func (t *MenuTopItemsTool) Schema() *llm.JSONSchema {
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
				Description: "Number of items to return (1-50). Default: 10.",
			},
		},
	}
}

// Run validates args, fetches popular items, sorts by revenue desc, and
// trims to the requested limit.
func (t *MenuTopItemsTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	// Production registers this tool bare (see main.go); the struct field is a
	// test seam. Fall back to the analytics service carried on the env. Guard
	// the concrete-pointer nil so we never wrap a typed nil into the interface.
	provider := t.Items
	if provider == nil && env.Analytics != nil {
		provider = env.Analytics
	}
	if provider == nil {
		return ToolResult{}, fmt.Errorf("get_menu_top_items: no popular-items provider wired")
	}

	period, err := normalizeMenuPeriod("get_menu_top_items", args)
	if err != nil {
		return ToolResult{}, err
	}
	limit, err := normalizeMenuLimit("get_menu_top_items", args)
	if err != nil {
		return ToolResult{}, err
	}

	analyticsPeriod := period
	if period == "day" {
		analyticsPeriod = "today"
	}

	// Pass limit into the SQL so only the requested rows are fetched.
	// The service returns them already sorted by revenue DESC.
	rows, err := provider.GetPopularItems(env.BusinessID, limit, analyticsPeriod, env.Location)
	if err != nil {
		return ToolResult{}, fmt.Errorf("get_menu_top_items: %w", err)
	}

	items := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		items = append(items, map[string]any{
			"name":     r.ItemName,
			"quantity": r.TotalSold,
			"revenue":  r.Revenue,
		})
	}

	return ToolResult{
		Summary: fmt.Sprintf("Top %d items in the %s", len(items), period),
		Data: map[string]any{
			"period": period,
			"limit":  limit,
			"items":  items,
		},
	}, nil
}

// normalizeMenuPeriod is the shared period validator used by the menu
// top/under tools. Empty/missing defaults to "week".
func normalizeMenuPeriod(toolName string, args map[string]any) (string, error) {
	raw, ok := args["period"]
	if !ok {
		return "week", nil
	}
	str, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s: period must be a string, got %T", toolName, raw)
	}
	if str == "" {
		return "week", nil
	}
	switch str {
	case "day", "week", "month":
		return str, nil
	default:
		return "", fmt.Errorf("%s: unsupported period %q (allowed: day, week, month)", toolName, str)
	}
}

// normalizeMenuLimit validates the limit arg as an integer in [1, 50].
// The model sends numbers as float64 via JSON; we coerce both int and
// float to a clean int.
func normalizeMenuLimit(toolName string, args map[string]any) (int, error) {
	raw, ok := args["limit"]
	if !ok {
		if toolName == "get_menu_top_items" {
			return 10, nil
		}
		return 5, nil
	}

	var n int
	switch v := raw.(type) {
	case int:
		n = v
	case int64:
		n = int(v)
	case float64:
		n = int(v)
	case float32:
		n = int(v)
	default:
		return 0, fmt.Errorf("%s: limit must be a number, got %T", toolName, raw)
	}

	if n < 1 || n > 50 {
		return 0, fmt.Errorf("%s: limit %d out of range (1-50)", toolName, n)
	}
	return n, nil
}
