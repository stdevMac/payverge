package director_tools

import (
	"context"
	"fmt"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// OrderFunnelTool aggregates order counts by status across a day/week/month
// window and reports the cancellation rate. Replaces the hand-rolled
// all-statuses summary the previous prompt embedded inline.
//
// Status taxonomy is the canonical OrderStatus enum from the database
// package: pending, approved, in_kitchen, ready, delivered, cancelled.
// Statuses with zero rows in the window are still reported as 0 so the
// model can phrase "no cancellations this week" with confidence.
type OrderFunnelTool struct{}

// orderFunnelStatuses is the full list of statuses we always surface,
// in display order (top of funnel to bottom + the failure case last).
var orderFunnelStatuses = []database.OrderStatus{
	database.OrderStatusPending,
	database.OrderStatusApproved,
	database.OrderStatusInKitchen,
	database.OrderStatusOrderReady,
	database.OrderStatusOrderDelivered,
	database.OrderStatusOrderCancelled,
}

// Name is the snake_case function identifier sent to the model.
func (t *OrderFunnelTool) Name() string { return "get_order_funnel" }

// HumanLabel is the localized pill label the UI shows while the tool runs.
func (t *OrderFunnelTool) HumanLabel(locale string) string {
	switch locale {
	case "es":
		return "Analizando embudo de pedidos"
	case "fr":
		return "Analyse de l'entonnoir de commandes"
	case "ar":
		return "تحليل قمع الطلبات"
	default:
		return "Reading order funnel"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *OrderFunnelTool) Description() string {
	return "Returns order counts by status (pending, approved, in_kitchen, ready, delivered, cancelled) and the cancellation rate over a day/week/month window. Call for order-flow, kitchen throughput, cancellations, or 'where are orders stalling'. Use get_revenue_summary for money totals."
}

// Schema declares the argument shape the model sees.
func (t *OrderFunnelTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"period": {
				Type:        llm.TypeString,
				Description: "Time window to summarize. One of: day, week, month. Default: week.",
				Enum:        []string{"day", "week", "month"},
			},
		},
	}
}

// Run validates args, queries the orders table grouped by status, and
// returns the funnel plus cancellation rate.
func (t *OrderFunnelTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("get_order_funnel: nil DB in tool env")
	}

	period, err := normalizeFunnelPeriod(args)
	if err != nil {
		return ToolResult{}, err
	}

	start := funnelPeriodStart(period, time.Now())

	type row struct {
		Status string
		Count  int
	}
	var rows []row

	gormDB := env.DB.GetGorm()
	if err := gormDB.
		Model(&database.Order{}).
		Select("status, COUNT(*) AS count").
		Where("business_id = ? AND created_at >= ?", env.BusinessID, start).
		Group("status").
		Scan(&rows).Error; err != nil {
		return ToolResult{}, fmt.Errorf("get_order_funnel: query failed: %w", err)
	}

	counts := make(map[string]int, len(orderFunnelStatuses))
	for _, s := range orderFunnelStatuses {
		counts[string(s)] = 0
	}
	total := 0
	for _, r := range rows {
		counts[r.Status] = r.Count
		total += r.Count
	}

	cancelled := counts[string(database.OrderStatusOrderCancelled)]
	cancellationRate := 0.0
	if total > 0 {
		cancellationRate = (float64(cancelled) / float64(total)) * 100.0
	}

	summary := fmt.Sprintf(
		"%d orders this %s · %d cancelled (%.1f%%)",
		total, period, cancelled, cancellationRate,
	)

	return ToolResult{
		Summary: summary,
		Data: map[string]any{
			"period":            period,
			"statuses":          counts,
			"total":             total,
			"cancellation_rate": cancellationRate,
		},
	}, nil
}

// normalizeFunnelPeriod pulls the period arg from the function-call
// payload, validating against the {day, week, month} allow-list. Empty
// or missing defaults to "week".
func normalizeFunnelPeriod(args map[string]any) (string, error) {
	raw, ok := args["period"]
	if !ok {
		return "week", nil
	}
	str, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("get_order_funnel: period must be a string, got %T", raw)
	}
	if str == "" {
		return "week", nil
	}
	switch str {
	case "day", "week", "month":
		return str, nil
	default:
		return "", fmt.Errorf("get_order_funnel: unsupported period %q (allowed: day, week, month)", str)
	}
}

// funnelPeriodStart returns the inclusive lower bound for the requested
// rolling window. "day" → 24 hours ago, "week" → 7 days, "month" → 30 days.
func funnelPeriodStart(period string, now time.Time) time.Time {
	switch period {
	case "day":
		return now.AddDate(0, 0, -1)
	case "week":
		return now.AddDate(0, 0, -7)
	case "month":
		return now.AddDate(0, 0, -30)
	default:
		return now.AddDate(0, 0, -7)
	}
}
