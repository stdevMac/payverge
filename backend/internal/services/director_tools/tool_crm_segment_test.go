package director_tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCRMSegmentTool_Name(t *testing.T) {
	tool := &CRMSegmentTool{}
	assert.Equal(t, "get_crm_segment", tool.Name())
}

func TestCRMSegmentTool_HumanLabel(t *testing.T) {
	tool := &CRMSegmentTool{}
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
}

func TestCRMSegmentTool_Schema(t *testing.T) {
	tool := &CRMSegmentTool{}
	schema := tool.Schema()
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties["filter"])
}

func seedCRMRows(t *testing.T, db *database.DB, bizID uint) {
	t.Helper()
	gormDB := db.GetGorm()
	now := time.Now()

	rows := []database.CustomerBusiness{
		// VIPs (top decile by total_spent). With 10 rows, top decile = 1.
		{CustomerID: 1, BusinessID: bizID, TotalSpent: 5000, VisitCount: 10, FirstVisitAt: now.AddDate(-1, 0, 0), CreatedAt: now.AddDate(0, -3, 0)},
		{CustomerID: 2, BusinessID: bizID, TotalSpent: 4000, VisitCount: 9, FirstVisitAt: now.AddDate(-1, 0, 0), CreatedAt: now.AddDate(0, -3, 0)},
		{CustomerID: 3, BusinessID: bizID, TotalSpent: 1000, VisitCount: 5, FirstVisitAt: now.AddDate(-1, 0, 0), CreatedAt: now.AddDate(0, -3, 0)},

		// Churning: last_visit_at < 30d ago AND visit_count >= 2.
		{
			CustomerID: 4, BusinessID: bizID, TotalSpent: 500, VisitCount: 4,
			FirstVisitAt: now.AddDate(-1, 0, 0),
			LastVisitAt:  ptrTime(now.AddDate(0, 0, -45)),
			CreatedAt:    now.AddDate(0, -2, 0),
		},
		{
			CustomerID: 5, BusinessID: bizID, TotalSpent: 300, VisitCount: 3,
			FirstVisitAt: now.AddDate(-1, 0, 0),
			LastVisitAt:  ptrTime(now.AddDate(0, 0, -50)),
			CreatedAt:    now.AddDate(0, -2, 0),
		},

		// Recent visitor — NOT churning despite the older date because visit_count = 1.
		{
			CustomerID: 6, BusinessID: bizID, TotalSpent: 100, VisitCount: 1,
			FirstVisitAt: now.AddDate(0, 0, -60),
			LastVisitAt:  ptrTime(now.AddDate(0, 0, -55)),
			CreatedAt:    now.AddDate(0, 0, -60),
		},

		// New: created within last 14 days.
		{CustomerID: 7, BusinessID: bizID, TotalSpent: 50, VisitCount: 1, FirstVisitAt: now.AddDate(0, 0, -5), CreatedAt: now.AddDate(0, 0, -5)},
		{CustomerID: 8, BusinessID: bizID, TotalSpent: 80, VisitCount: 1, FirstVisitAt: now.AddDate(0, 0, -2), CreatedAt: now.AddDate(0, 0, -2)},
		{CustomerID: 9, BusinessID: bizID, TotalSpent: 120, VisitCount: 2, FirstVisitAt: now.AddDate(0, 0, -10), CreatedAt: now.AddDate(0, 0, -10)},

		// Opt-in marketing.
		{CustomerID: 10, BusinessID: bizID, TotalSpent: 200, VisitCount: 3, OptInMarketing: true, FirstVisitAt: now.AddDate(0, -3, 0), CreatedAt: now.AddDate(0, -3, 0)},
	}
	for _, r := range rows {
		require.NoError(t, gormDB.Create(&r).Error)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestCRMSegmentTool_FilterVIP(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "CRM VIP Bistro")
	seedCRMRows(t, db, bizID)

	tool := &CRMSegmentTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"filter": "vip"}, env)
	require.NoError(t, err)
	assert.Equal(t, "vip", result.Data["filter"])
	// Top decile of 10 rows = 1; top spender = $5000.
	assert.Equal(t, 1, result.Data["size"])
	assert.InDelta(t, 5000.0, result.Data["avg_spend"].(float64), 0.001)
	assert.Contains(t, strings.ToLower(result.Summary), "vip")
}

func TestCRMSegmentTool_FilterChurning(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "CRM Churn Bistro")
	seedCRMRows(t, db, bizID)

	tool := &CRMSegmentTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"filter": "churning"}, env)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Data["size"])
}

func TestCRMSegmentTool_FilterNew(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "CRM New Bistro")
	seedCRMRows(t, db, bizID)

	tool := &CRMSegmentTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"filter": "new"}, env)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Data["size"])
}

func TestCRMSegmentTool_FilterOptIn(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "CRM Opt-in Bistro")
	seedCRMRows(t, db, bizID)

	tool := &CRMSegmentTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"filter": "opt_in_marketing"}, env)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Data["size"])
}

func TestCRMSegmentTool_ValidatesFilter(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "CRM Bad Filter")
	tool := &CRMSegmentTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	_, err := tool.Run(context.Background(), map[string]any{"filter": "bogus"}, env)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "filter"))
}

// #876: the Director told the operator that "Camila" on Mesa 9 was a lapsing
// regular. No CRM row says that — the tool returns aggregates only, and the
// one thing in the result that looked like evidence was a wall of zeroes for
// an empty segment. An empty segment must read as "no customers", never as
// "customers who spend $0 and last came today".
func TestCRMSegmentTool_EmptySegmentOmitsZeroAggregates(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "CRM Empty Bistro")

	tool := &CRMSegmentTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	for _, filter := range crmSegmentFilters {
		t.Run(filter, func(t *testing.T) {
			result, err := tool.Run(context.Background(), map[string]any{"filter": filter}, env)
			require.NoError(t, err)
			assert.Equal(t, 0, result.Data["size"])
			assert.Equal(t, false, result.Data["segment_measurable"])
			assert.NotContains(t, result.Data, "avg_spend")
			assert.NotContains(t, result.Data, "avg_visits")
			assert.NotContains(t, result.Data, "last_visit_median_days_ago")
			reason, ok := result.Data["no_data_reason"].(string)
			require.True(t, ok, "empty segment must state why there is nothing to report")
			assert.NotEmpty(t, reason)
			assert.NotContains(t, result.Summary, "$0")
		})
	}
}

// A segment with rows but no recorded visit must not report "0 days ago",
// which reads as "they were here today".
func TestCRMSegmentTool_RecencyOmittedWhenNoVisitRecorded(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "CRM No Visit Bistro")
	gormDB := db.GetGorm()
	now := time.Now()
	require.NoError(t, gormDB.Create(&database.CustomerBusiness{
		CustomerID: 41, BusinessID: bizID, TotalSpent: 90, VisitCount: 1,
		OptInMarketing: true, FirstVisitAt: now.AddDate(0, 0, -3), CreatedAt: now.AddDate(0, 0, -3),
	}).Error)

	tool := &CRMSegmentTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}
	result, err := tool.Run(context.Background(), map[string]any{"filter": "opt_in_marketing"}, env)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Data["size"])
	assert.Equal(t, true, result.Data["segment_measurable"])
	assert.Contains(t, result.Data, "avg_spend")
	assert.NotContains(t, result.Data, "last_visit_median_days_ago",
		"no row carries a visit date, so there is no recency to report")
}

// Every result must state that it carries no customer identities, so naming a
// person, a table, or a contact detail is visibly an invention.
func TestCRMSegmentTool_GuidanceForbidsInventedIdentities(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "CRM Identity Bistro")
	seedCRMRows(t, db, bizID)

	tool := &CRMSegmentTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	for _, filter := range crmSegmentFilters {
		t.Run(filter, func(t *testing.T) {
			result, err := tool.Run(context.Background(), map[string]any{"filter": filter}, env)
			require.NoError(t, err)
			assert.Equal(t, false, result.Data["contains_customer_identities"])
			guidance, ok := result.Data["guidance"].(string)
			require.True(t, ok, "every CRM result must carry anti-fabrication guidance")
			lower := strings.ToLower(guidance)
			assert.Contains(t, lower, "name")
			assert.Contains(t, lower, "table")
			for key, value := range result.Data {
				if str, isString := value.(string); isString && key != "guidance" && key != "no_data_reason" {
					assert.NotContains(t, strings.ToLower(str), "camila")
				}
			}
		})
	}
}

// The model-facing description must say the result is identity-free, since
// that is the only place the model learns it before choosing the tool.
func TestCRMSegmentTool_DescriptionStatesNoIdentities(t *testing.T) {
	desc := strings.ToLower((&CRMSegmentTool{}).Description())
	assert.Contains(t, desc, "no customer names")
}
