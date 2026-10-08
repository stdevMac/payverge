package director_tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// defaultMarginPreviewBumpDollars is the advertised "small price adjustment"
// used when the owner asks for a new-margin preview without naming a price.
const defaultMarginPreviewBumpDollars = 1.50

// PreviewMarginChangeTool is a read-only before/after calculator. It never
// writes the menu and never stages a proposal — that is why the model can
// call it on "show me the new margin before anything goes live" asks, which
// the prompt forbids from using propose_* tools.
type PreviewMarginChangeTool struct{}

func (t *PreviewMarginChangeTool) Name() string { return "preview_margin_change" }

func (t *PreviewMarginChangeTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Calculando el margen nuevo"
	case "fr":
		return "Calcul de la nouvelle marge"
	case "ar":
		return "حساب الهامش الجديد"
	default:
		return "Previewing new margin"
	}
}

func (t *PreviewMarginChangeTool) Description() string {
	return "Read-only preview of current and new plate margin and food-cost % after a small price change. Defaults to a +$1.50 flat increase when the owner does not name a bump. If they name a dollar amount (+$2, two dollars), pass that as value. Does not apply, queue, or stage anything. Call when the owner asks for a new-margin / new food-cost walkthrough, 'before anything goes live', or a price preview without applying. If no item is named, returns the two thinnest-margin bestsellers that have a known plate cost. Never invent a plate cost; never ask the owner what price to consider."
}

func (t *PreviewMarginChangeTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"item_name": {
				Type:        llm.TypeString,
				Description: "Optional single dish name as the owner said it.",
			},
			"item_names": {
				Type:        llm.TypeArray,
				Description: "Optional list of dish names. When omitted, the two thinnest-margin bestsellers with a known plate cost are chosen.",
				Items:       &llm.JSONSchema{Type: llm.TypeString},
			},
			"value": {
				Type:        llm.TypeNumber,
				Description: "Positive flat dollar increase. Default: 1.50.",
			},
			"period": {
				Type:        llm.TypeString,
				Description: "Sales window used to pick bestsellers. One of: day, week, month. Default: week.",
				Enum:        []string{"day", "week", "month"},
			},
		},
	}
}

func (t *PreviewMarginChangeTool) Run(ctx context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("preview_margin_change: nil DB in tool env")
	}
	bump := defaultMarginPreviewBumpDollars
	if raw, ok := args["value"]; ok {
		switch v := raw.(type) {
		case int:
			bump = float64(v)
		case int64:
			bump = float64(v)
		case float32:
			bump = float64(v)
		case float64:
			bump = v
		default:
			return ToolResult{}, fmt.Errorf("preview_margin_change: value must be a number, got %T", raw)
		}
		if bump <= 0 {
			return ToolResult{}, fmt.Errorf("preview_margin_change: value must be > 0")
		}
	}

	period, err := normalizeMenuPeriod("preview_margin_change", args)
	if err != nil {
		return ToolResult{}, err
	}

	candidates := collectMarginPreviewCandidates(ctx, env, period)
	wanted := parsePreviewItemNames(args)
	selected := selectMarginPreviewDishes(candidates, wanted, 2)
	if len(selected) == 0 {
		return ToolResult{
			Summary: "No plate cost on file for a new-margin preview",
			Data: map[string]any{
				"applied":        false,
				"price_increase": bump,
				"items":          []map[string]any{},
				"note":           "Preview only. Nothing was applied or queued.",
			},
		}, nil
	}

	rows := make([]map[string]any, 0, len(selected))
	parts := make([]string, 0, len(selected))
	for _, d := range selected {
		row := marginPreviewRow(d, bump)
		rows = append(rows, row)
		if known, _ := row["unit_cost_known"].(bool); known {
			parts = append(parts, fmt.Sprintf("%s current margin $%s, new margin $%s",
				d.Name, humanizeMoney(anyFloat(row["current_margin"])), humanizeMoney(anyFloat(row["new_margin"]))))
		} else {
			parts = append(parts, fmt.Sprintf("%s $%s → $%s (plate cost not on file)",
				d.Name, humanizeMoney(d.Price), humanizeMoney(d.Price+bump)))
		}
	}

	return ToolResult{
		Summary: strings.Join(parts, ". ") + ". Preview only. Nothing is queued.",
		Data: map[string]any{
			"applied":        false,
			"price_increase": bump,
			"period":         period,
			"items":          rows,
			"note":           "Preview only. Nothing was applied or queued. Do not ask the owner for a price.",
		},
	}, nil
}

// marginPreviewDish is a card+sales+cost row used to pick the advertised
// two-dish new-margin walkthrough. It is never serialized as wire JSON.
type marginPreviewDish struct {
	Name     string
	ID       string
	Price    float64
	Cost     float64
	Units    int
	Revenue  float64
	HasCost  bool
	HasSales bool
}

func collectMarginPreviewCandidates(ctx context.Context, env ToolEnv, period string) []marginPreviewDish {
	byName := map[string]int{}
	out := make([]marginPreviewDish, 0)

	upsert := func(d marginPreviewDish) {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			return
		}
		d.Name = name
		key := strings.ToLower(name)
		if idx, ok := byName[key]; ok {
			cur := out[idx]
			if d.ID != "" {
				cur.ID = d.ID
			}
			if d.Price > 0 {
				cur.Price = d.Price
			}
			if d.HasCost {
				cur.HasCost = true
				cur.Cost = d.Cost
			}
			if d.HasSales {
				cur.HasSales = true
				cur.Units = d.Units
				cur.Revenue = d.Revenue
			}
			out[idx] = cur
			return
		}
		byName[key] = len(out)
		out = append(out, d)
	}

	if _, cats, err := database.GetMenuByBusinessID(env.BusinessID); err == nil {
		for _, cat := range cats {
			for _, item := range cat.Items {
				d := marginPreviewDish{Name: item.Name, ID: item.ID, Price: item.Price}
				if item.Cogs > 0 {
					d.HasCost = true
					d.Cost = item.Cogs
				}
				upsert(d)
			}
		}
	}

	for i := range out {
		if cost, ok := plateCostForItem(env, out[i].ID, out[i].Name); ok {
			out[i].HasCost = true
			out[i].Cost = cost
		}
	}

	top := &MenuTopItemsTool{}
	if res, err := top.Run(ctx, map[string]any{"period": period, "limit": float64(10)}, env); err == nil {
		for _, row := range anyItemMaps(res.Data["items"]) {
			name, _ := row["name"].(string)
			if strings.TrimSpace(name) == "" {
				continue
			}
			units := anyInt(row["quantity"])
			if units == 0 {
				units = anyInt(row["qty_sold"])
			}
			price := anyFloat(row["avg_price"])
			upsert(marginPreviewDish{
				Name:     name,
				HasSales: units > 0,
				Units:    units,
				Revenue:  anyFloat(row["revenue"]),
				Price:    price,
			})
		}
	}

	return out
}

func parsePreviewItemNames(args map[string]any) []string {
	names := make([]string, 0, 2)
	if raw, ok := args["item_names"]; ok {
		switch v := raw.(type) {
		case []string:
			for _, n := range v {
				if n = strings.TrimSpace(n); n != "" {
					names = append(names, n)
				}
			}
		case []any:
			for _, row := range v {
				if n, ok := row.(string); ok {
					if n = strings.TrimSpace(n); n != "" {
						names = append(names, n)
					}
				}
			}
		case string:
			if n := strings.TrimSpace(v); n != "" {
				names = append(names, n)
			}
		}
	}
	if n, _ := args["item_name"].(string); strings.TrimSpace(n) != "" {
		names = append(names, strings.TrimSpace(n))
	}
	return names
}

// selectMarginPreviewDishes picks up to limit dishes for the advertised
// walkthrough. Named items win (even without cost). Otherwise bestsellers
// that have plate cost come first, then remaining costed card items.
func selectMarginPreviewDishes(all []marginPreviewDish, wanted []string, limit int) []marginPreviewDish {
	if limit <= 0 {
		limit = 2
	}
	if len(wanted) > 0 {
		out := make([]marginPreviewDish, 0, len(wanted))
		for _, name := range wanted {
			for _, d := range all {
				if strings.EqualFold(d.Name, name) {
					out = append(out, d)
					break
				}
			}
		}
		return out
	}

	costedSold := make([]marginPreviewDish, 0)
	costed := make([]marginPreviewDish, 0)
	for _, d := range all {
		if !d.HasCost || d.Cost <= 0 || d.Price <= 0 {
			continue
		}
		costed = append(costed, d)
		if d.HasSales && d.Units > 0 {
			costedSold = append(costedSold, d)
		}
	}
	sort.SliceStable(costedSold, func(i, j int) bool {
		if costedSold[i].Units != costedSold[j].Units {
			return costedSold[i].Units > costedSold[j].Units
		}
		return previewFoodCostPct(costedSold[i]) > previewFoodCostPct(costedSold[j])
	})
	sort.SliceStable(costed, func(i, j int) bool {
		if previewFoodCostPct(costed[i]) != previewFoodCostPct(costed[j]) {
			return previewFoodCostPct(costed[i]) > previewFoodCostPct(costed[j])
		}
		return costed[i].Margin() < costed[j].Margin()
	})

	seen := map[string]bool{}
	out := make([]marginPreviewDish, 0, limit)
	add := func(list []marginPreviewDish) {
		for _, d := range list {
			if len(out) >= limit {
				return
			}
			key := strings.ToLower(d.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, d)
		}
	}
	add(costedSold)
	add(costed)
	return out
}

func (d marginPreviewDish) Margin() float64 {
	if !d.HasCost {
		return 0
	}
	return d.Price - d.Cost
}

func previewFoodCostPct(d marginPreviewDish) float64 {
	if d.Price <= 0 || !d.HasCost {
		return 0
	}
	return d.Cost / d.Price
}

func marginPreviewRow(d marginPreviewDish, bump float64) map[string]any {
	proposed := d.Price + bump
	row := map[string]any{
		"name":           d.Name,
		"current_price":  d.Price,
		"proposed_price": proposed,
		"price_increase": bump,
		"applied":        false,
	}
	if d.HasSales && d.Units > 0 {
		row["units_sold"] = d.Units
	}
	if d.HasCost && d.Cost > 0 && d.Price > 0 && proposed > 0 {
		row["unit_cost"] = d.Cost
		row["current_margin"] = d.Price - d.Cost
		row["new_margin"] = proposed - d.Cost
		row["current_food_cost_pct"] = d.Cost / d.Price
		row["new_food_cost_pct"] = d.Cost / proposed
		row["unit_cost_known"] = true
		return row
	}
	row["unit_cost_known"] = false
	return row
}

func anyItemMaps(raw any) []map[string]any {
	switch v := raw.(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, row := range v {
			if m, ok := row.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func anyInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	default:
		return 0
	}
}

func anyFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}
