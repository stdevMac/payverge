package director_tools

import (
	"context"
	"fmt"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// RevenueProvider is the slice of the analytics service the revenue tool
// actually depends on. The interface seam lets unit tests stub canned
// PaymentWindowSummary values without seeding the (very chunky) ledger
// fixture chain the real SQL pipeline requires.
//
// Production wiring uses the concrete *analytics.AnalyticsService, which
// satisfies this interface — see DirectorConsoleService startup.
type RevenueProvider interface {
	GetPaymentPeriodSummary(businessID uint, period string, loc *time.Location) (*analytics.PaymentWindowSummary, error)
	GetPaymentWindowSummary(businessID uint, startDate, endDate time.Time) (*analytics.PaymentWindowSummary, error)
	// LiveOpenCheckRemaining is remaining due on every open/partial check.
	// Overview "today sales" adds this to collected revenue; day-period
	// summaries must do the same so Sage does not report $0 tonight (#730).
	LiveOpenCheckRemaining(businessID uint) (float64, error)
}

// RevenueSummaryTool answers "how did the last day/week/month go" — the
// Director Console's bread-and-butter glance. Returns revenue, tips,
// transaction count, average ticket, and a human-readable summary the
// model can quote verbatim.
//
// When compare_to_prior=true the tool also fetches the previous equal-length
// window via GetPaymentWindowSummary and appends "prior" and "delta" blocks
// to the Data map so the model can answer trend or "vs last week" questions.
type RevenueSummaryTool struct {
	// Revenue is the analytics seam. In production this is the real
	// *analytics.AnalyticsService; tests inject a stub.
	Revenue RevenueProvider
}

// Name is the snake_case function identifier sent to the model.
func (t *RevenueSummaryTool) Name() string { return "get_revenue_summary" }

// HumanLabel is the localized pill label the UI shows while the tool runs.
func (t *RevenueSummaryTool) HumanLabel(locale string) string {
	switch locale {
	case "es":
		return "Calculando resumen de ingresos"
	case "fr":
		return "Calcul du résumé des revenus"
	case "ar":
		return "حساب ملخص الإيرادات"
	default:
		return "Reading revenue summary"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *RevenueSummaryTool) Description() string {
	return "Returns total revenue, tips, transaction count, bill count, unique guests, and average ticket for a day/week/month window. Call for sales totals, 'how much did we make', revenue, or average-check questions. Set compare_to_prior=true to also get the previous equal-length window and the deltas for trend/'vs last week' questions."
}

// Schema declares the argument shape the model sees.
func (t *RevenueSummaryTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"period": {
				Type:        llm.TypeString,
				Description: "Time window to summarize. One of: day, week, month. Default: week.",
				Enum:        []string{"day", "week", "month"},
			},
			"compare_to_prior": {
				Type:        llm.TypeBoolean,
				Description: "If true, also fetch the previous equal-length window and return prior values plus percent deltas (revenue_pct, transactions_pct, average_ticket_pct). Use for trend or 'vs last week' questions.",
			},
		},
	}
}

// Run validates args, dispatches to the analytics provider, and shapes
// the response.
func (t *RevenueSummaryTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	// Production registers this tool bare (see main.go); the struct field is a
	// test seam. Fall back to the analytics service carried on the env. Guard
	// the concrete-pointer nil so we never wrap a typed nil into the interface.
	provider := t.Revenue
	if provider == nil && env.Analytics != nil {
		provider = env.Analytics
	}
	if provider == nil {
		return ToolResult{}, fmt.Errorf("get_revenue_summary: no revenue provider wired")
	}

	period, err := normalizeRevenuePeriod(args)
	if err != nil {
		return ToolResult{}, err
	}

	// Map the public period taxonomy {day, week, month} to the analytics
	// service's internal vocabulary ({today, week, month}).
	analyticsPeriod := period
	if period == "day" {
		analyticsPeriod = "today"
	}

	summary, err := provider.GetPaymentPeriodSummary(env.BusinessID, analyticsPeriod, env.Location)
	if err != nil {
		return ToolResult{}, fmt.Errorf("get_revenue_summary: %w", err)
	}

	revenue := summary.TotalRevenue
	floorRemaining := 0.0
	if period == "day" {
		if remaining, openErr := provider.LiveOpenCheckRemaining(env.BusinessID); openErr == nil {
			floorRemaining = remaining
		}
	}

	humanSummary := fmt.Sprintf(
		"Revenue: $%s across %d transactions (avg ticket $%s)",
		humanizeMoney(revenue),
		summary.TransactionCount,
		humanizeMoney(summary.AverageTicket),
	)
	if period == "day" && floorRemaining != 0 {
		humanSummary = fmt.Sprintf(
			"Collected today: $%s. Remaining on open checks: $%s. %d transactions (avg ticket $%s)",
			humanizeMoney(summary.TotalRevenue),
			humanizeMoney(floorRemaining),
			summary.TransactionCount,
			humanizeMoney(summary.AverageTicket),
		)
	}

	data := map[string]any{
		"period":         period,
		"revenue":        revenue,
		"tips":           summary.TotalTips,
		"transactions":   summary.TransactionCount,
		"bills":          summary.BillCount,
		"unique_guests":  summary.UniqueCustomers,
		"average_ticket": summary.AverageTicket,
	}
	if period == "day" {
		data["collected_revenue"] = summary.TotalRevenue
		data["floor_remaining"] = floorRemaining
		data["includes_open_checks"] = false
	}

	if comparePriorRequested(args) && !summary.StartDate.IsZero() && summary.EndDate.After(summary.StartDate) {
		length := summary.EndDate.Sub(summary.StartDate)
		priorEnd := summary.StartDate
		priorStart := summary.StartDate.Add(-length)
		if prior, perr := provider.GetPaymentWindowSummary(env.BusinessID, priorStart, priorEnd); perr == nil && prior != nil {
			data["prior"] = map[string]any{
				"revenue":        prior.TotalRevenue,
				"transactions":   prior.TransactionCount,
				"average_ticket": prior.AverageTicket,
			}
			data["delta"] = map[string]any{
				"revenue_pct":        pctDelta(summary.TotalRevenue, prior.TotalRevenue),
				"transactions_pct":   pctDelta(float64(summary.TransactionCount), float64(prior.TransactionCount)),
				"average_ticket_pct": pctDelta(summary.AverageTicket, prior.AverageTicket),
			}
		}
	}

	return ToolResult{Summary: humanSummary, Data: data}, nil
}

// comparePriorRequested reads the optional compare_to_prior bool arg.
func comparePriorRequested(args map[string]any) bool {
	v, ok := args["compare_to_prior"].(bool)
	return ok && v
}

// pctDelta is the percent change from prior to current; 0 when prior is 0.
func pctDelta(current, prior float64) float64 {
	if prior == 0 {
		return 0
	}
	return (current - prior) / prior * 100
}

// normalizeRevenuePeriod pulls the period arg from the function-call
// payload, validating against the {day, week, month} allow-list. An
// empty/missing arg defaults to "week".
func normalizeRevenuePeriod(args map[string]any) (string, error) {
	raw, ok := args["period"]
	if !ok {
		return "week", nil
	}
	str, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("get_revenue_summary: period must be a string, got %T", raw)
	}
	if str == "" {
		return "week", nil
	}
	switch str {
	case "day", "week", "month":
		return str, nil
	default:
		return "", fmt.Errorf("get_revenue_summary: unsupported period %q (allowed: day, week, month)", str)
	}
}
