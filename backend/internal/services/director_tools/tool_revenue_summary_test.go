package director_tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRevenueSummaryTool_Name(t *testing.T) {
	tool := &RevenueSummaryTool{}
	assert.Equal(t, "get_revenue_summary", tool.Name())
}

func TestRevenueSummaryTool_HumanLabel(t *testing.T) {
	tool := &RevenueSummaryTool{}
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
}

func TestRevenueSummaryTool_Schema(t *testing.T) {
	tool := &RevenueSummaryTool{}
	schema := tool.Schema()
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties["period"])
	require.NotNil(t, schema.Properties["compare_to_prior"])
}

func TestRevenueSummaryTool_Run(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Coastal Bistro")

	provider := &stubRevenueProvider{
		byPeriod: map[string]*analytics.PaymentWindowSummary{
			"week": {
				TotalRevenue:     14250.75,
				TotalTips:        1800.25,
				TransactionCount: 142,
				BillCount:        118,
				UniqueCustomers:  91,
				AverageTicket:    120.77,
			},
		},
	}

	tool := &RevenueSummaryTool{Revenue: provider}
	env := ToolEnv{
		BusinessID: bizID,
		Locale:     "en",
		DB:         db,
	}

	result, err := tool.Run(context.Background(), map[string]any{"period": "week"}, env)
	require.NoError(t, err)

	assert.InDelta(t, 14250.75, result.Data["revenue"], 0.001)
	assert.Equal(t, 142, result.Data["transactions"])
	assert.InDelta(t, 120.77, result.Data["average_ticket"], 0.001)
	assert.InDelta(t, 1800.25, result.Data["tips"], 0.001)
	assert.Equal(t, "week", result.Data["period"])
	assert.Contains(t, result.Summary, "14,250.75")
}

func TestRevenueSummaryTool_DefaultsToWeek(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Default Period Bistro")

	provider := &stubRevenueProvider{
		byPeriod: map[string]*analytics.PaymentWindowSummary{
			"week": {
				TotalRevenue:     1000,
				TransactionCount: 10,
				AverageTicket:    100,
			},
		},
	}

	tool := &RevenueSummaryTool{Revenue: provider}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{}, env)
	require.NoError(t, err)
	assert.Equal(t, "week", result.Data["period"])
}

func TestRevenueSummaryTool_ValidatesPeriod(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bad Period Bistro")

	tool := &RevenueSummaryTool{Revenue: &stubRevenueProvider{}}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	_, err := tool.Run(context.Background(), map[string]any{"period": "decade"}, env)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "period"))
}

func TestRevenueSummaryTool_DayIncludesOpenChecks(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Open Check Bistro")

	provider := &stubRevenueProvider{
		byPeriod: map[string]*analytics.PaymentWindowSummary{
			"today": {TotalRevenue: 0, TransactionCount: 0, AverageTicket: 0},
		},
		openRemaining: 18.04,
	}
	tool := &RevenueSummaryTool{Revenue: provider}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"period": "day"}, env)
	require.NoError(t, err)
	assert.InDelta(t, 0.0, result.Data["revenue"], 0.001, "revenue is collected only")
	assert.InDelta(t, 0.0, result.Data["collected_revenue"], 0.001)
	assert.InDelta(t, 18.04, result.Data["floor_remaining"], 0.001)
	assert.Equal(t, false, result.Data["includes_open_checks"], "remaining is not folded into revenue")
	assert.Contains(t, result.Summary, "18.04")
	assert.Contains(t, result.Summary, "Remaining")
}

func TestRevenueSummaryTool_AcceptsDayWeekMonth(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Multi Period Bistro")

	provider := &stubRevenueProvider{
		byPeriod: map[string]*analytics.PaymentWindowSummary{
			"today": {TotalRevenue: 100, TransactionCount: 1, AverageTicket: 100},
			"week":  {TotalRevenue: 500, TransactionCount: 5, AverageTicket: 100},
			"month": {TotalRevenue: 2000, TransactionCount: 20, AverageTicket: 100},
		},
	}
	tool := &RevenueSummaryTool{Revenue: provider}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	for _, period := range []string{"day", "week", "month"} {
		_, err := tool.Run(context.Background(), map[string]any{"period": period}, env)
		require.NoError(t, err, "period %q should be accepted", period)
	}
}

func TestRevenueSummaryTool_CompareToPrior(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Coastal Bistro")

	provider := &stubRevenueProvider{
		byPeriod: map[string]*analytics.PaymentWindowSummary{
			"week": {
				StartDate:        timeDaysAgo(7),
				EndDate:          timeNow(),
				TotalRevenue:     1200,
				TransactionCount: 100,
				AverageTicket:    12,
			},
		},
		window: &analytics.PaymentWindowSummary{
			TotalRevenue:     1000,
			TransactionCount: 80,
			AverageTicket:    12.5,
		},
	}

	tool := &RevenueSummaryTool{Revenue: provider}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	res, err := tool.Run(context.Background(), map[string]any{"period": "week", "compare_to_prior": true}, env)
	require.NoError(t, err)

	require.Equal(t, 1, provider.windowCalls, "prior window must be fetched once")
	prior, ok := res.Data["prior"].(map[string]any)
	require.True(t, ok, "prior block present")
	assert.InDelta(t, 1000.0, prior["revenue"], 0.001)
	delta, ok := res.Data["delta"].(map[string]any)
	require.True(t, ok, "delta block present")
	assert.InDelta(t, 20.0, delta["revenue_pct"], 0.001) // (1200-1000)/1000*100
}

func TestRevenueSummaryTool_NoPriorWhenFlagOff(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Coastal Bistro")
	provider := &stubRevenueProvider{
		byPeriod: map[string]*analytics.PaymentWindowSummary{
			"week": {TotalRevenue: 1200, TransactionCount: 100, AverageTicket: 12},
		},
	}
	tool := &RevenueSummaryTool{Revenue: provider}
	res, err := tool.Run(context.Background(), map[string]any{"period": "week"}, ToolEnv{BusinessID: bizID, DB: db})
	require.NoError(t, err)
	assert.Equal(t, 0, provider.windowCalls, "no prior call when flag absent")
	_, hasPrior := res.Data["prior"]
	assert.False(t, hasPrior)
}

func timeNow() time.Time          { return time.Now() }
func timeDaysAgo(n int) time.Time { return time.Now().AddDate(0, 0, -n) }

func BenchmarkRevenueSummary_WithAndWithoutPrior(b *testing.B) {
	db := newTestDB(&testing.T{})
	bizID := createTestBusiness(&testing.T{}, db, "Bench Bistro")
	provider := &stubRevenueProvider{
		byPeriod: map[string]*analytics.PaymentWindowSummary{
			"week": {StartDate: timeDaysAgo(7), EndDate: timeNow(), TotalRevenue: 1200, TransactionCount: 100, AverageTicket: 12},
		},
		window: &analytics.PaymentWindowSummary{TotalRevenue: 1000, TransactionCount: 80, AverageTicket: 12.5},
	}
	tool := &RevenueSummaryTool{Revenue: provider}
	env := ToolEnv{BusinessID: bizID, DB: db}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tool.Run(context.Background(), map[string]any{"period": "week", "compare_to_prior": true}, env)
	}
}
