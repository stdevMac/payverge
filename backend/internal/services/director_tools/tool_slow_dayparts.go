package director_tools

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// SlowDaypartsTool identifies hour-of-day buckets that under-perform
// across a rolling day/week/month window. A "slow" hour has revenue
// below 60% of the mean revenue across all hours that saw at least one
// bill, and at least min_orders rows.
//
// We bucket on Bill.CreatedAt (Bill carries the canonical TotalAmount
// in cents) instead of joining through orders — bills are the
// revenue-bearing entity, and the plan's "orders" wording maps cleanly
// onto a customer's bill in the colloquial sense the Director Console
// uses with operators.
type SlowDaypartsTool struct{}

// Name is the snake_case function identifier sent to the model.
func (t *SlowDaypartsTool) Name() string { return "get_slow_dayparts" }

// HumanLabel is the localized pill label the UI shows while the tool runs.
func (t *SlowDaypartsTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Identificando franjas horarias lentas"
	case "fr":
		return "Identification des heures creuses"
	case "ar":
		return "تحديد الفترات البطيئة"
	default:
		return "Reading slow dayparts"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *SlowDaypartsTool) Description() string {
	return "Identifies hours of the day whose revenue falls below 60% of the daily average — the under-performing time windows. Call to find slow/dead hours, when to run promotions, or when to adjust staffing. Returns per-hour revenue and how far below the mean each slow hour sits."
}

// Schema declares the argument shape the model sees.
func (t *SlowDaypartsTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"period": {
				Type:        llm.TypeString,
				Description: "Time window. One of: day, week, month. Default: week.",
				Enum:        []string{"day", "week", "month"},
			},
			"min_orders": {
				Type:        llm.TypeInteger,
				Description: "Minimum number of bills in a bucket for it to count as slow. Default: 1.",
			},
		},
	}
}

// Run validates args, buckets bills by hour-of-day, and returns the
// buckets whose revenue trails the mean and clear min_orders.
func (t *SlowDaypartsTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("get_slow_dayparts: nil DB in tool env")
	}

	period, err := normalizeDaypartsPeriod(args)
	if err != nil {
		return ToolResult{}, err
	}
	minOrders, err := normalizeMinOrders(args)
	if err != nil {
		return ToolResult{}, err
	}

	start := funnelPeriodStart(period, time.Now())

	var bills []database.Bill
	if err := env.DB.GetGorm().
		Model(&database.Bill{}).
		Select("created_at, total_amount").
		Where("business_id = ? AND created_at >= ?", env.BusinessID, start).
		Find(&bills).Error; err != nil {
		return ToolResult{}, fmt.Errorf("get_slow_dayparts: query failed: %w", err)
	}

	type bucket struct {
		orders  int
		revenue float64
	}
	buckets := make(map[int]*bucket, 24)
	for _, b := range bills {
		hr := b.CreatedAt.Hour()
		buck, ok := buckets[hr]
		if !ok {
			buck = &bucket{}
			buckets[hr] = buck
		}
		buck.orders++
		buck.revenue += float64(b.TotalAmount) / 100.0
	}

	if len(buckets) == 0 {
		return ToolResult{
			Summary: "0 slow dayparts identified",
			Data: map[string]any{
				"period": period,
				"slow":   []map[string]any{},
			},
		}, nil
	}

	totalRevenue := 0.0
	for _, b := range buckets {
		totalRevenue += b.revenue
	}
	meanRevenue := totalRevenue / float64(len(buckets))
	threshold := meanRevenue * 0.60

	slow := make([]map[string]any, 0)
	for hr, b := range buckets {
		if b.orders < minOrders {
			continue
		}
		if b.revenue >= threshold {
			continue
		}
		vsMeanPct := 0.0
		if meanRevenue > 0 {
			vsMeanPct = (b.revenue / meanRevenue) * 100.0
		}
		slow = append(slow, map[string]any{
			"hour_start":  hr,
			"hour_end":    (hr + 1) % 24,
			"orders":      b.orders,
			"revenue":     b.revenue,
			"vs_mean_pct": vsMeanPct,
		})
	}

	sort.SliceStable(slow, func(i, j int) bool {
		return slow[i]["hour_start"].(int) < slow[j]["hour_start"].(int)
	})

	return ToolResult{
		Summary: fmt.Sprintf("%d slow dayparts identified", len(slow)),
		Data: map[string]any{
			"period":          period,
			"slow":            slow,
			"mean_revenue":    meanRevenue,
			"threshold_ratio": 0.60,
		},
	}, nil
}

// normalizeDaypartsPeriod accepts the {day, week, month} allow-list,
// defaulting empty/missing to "week".
func normalizeDaypartsPeriod(args map[string]any) (string, error) {
	raw, ok := args["period"]
	if !ok {
		return "week", nil
	}
	str, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("get_slow_dayparts: period must be a string, got %T", raw)
	}
	if str == "" {
		return "week", nil
	}
	switch str {
	case "day", "week", "month":
		return str, nil
	default:
		return "", fmt.Errorf("get_slow_dayparts: unsupported period %q (allowed: day, week, month)", str)
	}
}

// normalizeMinOrders coerces the min_orders arg to a non-negative int.
// JSON delivers numbers as float64; we accept int variants too.
func normalizeMinOrders(args map[string]any) (int, error) {
	raw, ok := args["min_orders"]
	if !ok {
		return 1, nil
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
		return 0, fmt.Errorf("get_slow_dayparts: min_orders must be a number, got %T", raw)
	}
	if n < 0 {
		return 0, fmt.Errorf("get_slow_dayparts: min_orders %d must be non-negative", n)
	}
	return n, nil
}
