package director_tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTools_ResolveEnvAnalyticsWhenStructProviderNil proves the production
// wiring path. main.go registers these analytics-backed tools bare
// (&RevenueSummaryTool{}, &MenuTopItemsTool{}, &MenuUnderperformersTool{}),
// so their struct-field provider seam is nil at runtime. The tools MUST
// resolve the analytics service from env.Analytics (which the Director
// Console populates) instead of bailing with "no ... provider wired".
//
// The analytics SQL may still return zero rows or an error on the empty
// in-memory DB — that is fine. The assertion is narrow: the failure must
// NOT be the "provider wired" sentinel, i.e. resolution happened.
func TestTools_ResolveEnvAnalyticsWhenStructProviderNil(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Wiring Bistro")
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db, Analytics: newTestAnalytics(t, db)}

	cases := []struct {
		name string
		run  func() (ToolResult, error)
	}{
		{"get_revenue_summary", func() (ToolResult, error) {
			return (&RevenueSummaryTool{}).Run(context.Background(), map[string]any{"period": "week"}, env)
		}},
		{"get_menu_top_items", func() (ToolResult, error) {
			return (&MenuTopItemsTool{}).Run(context.Background(), map[string]any{"period": "week"}, env)
		}},
		{"get_menu_underperformers", func() (ToolResult, error) {
			return (&MenuUnderperformersTool{}).Run(context.Background(), map[string]any{"period": "week"}, env)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.run()
			if err != nil {
				require.NotContains(t, err.Error(), "provider wired",
					"%s must resolve env.Analytics, not bail on the nil struct-field provider", c.name)
			}
		})
	}
}

// TestTools_ErrorWhenNoProviderAnywhere confirms the nil-guard still fires
// when neither the struct field nor env.Analytics is set.
func TestTools_ErrorWhenNoProviderAnywhere(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "No Provider Bistro")
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db} // Analytics nil, struct field nil

	runs := []struct {
		name string
		run  func() (ToolResult, error)
	}{
		{"get_revenue_summary", func() (ToolResult, error) {
			return (&RevenueSummaryTool{}).Run(context.Background(), map[string]any{"period": "week"}, env)
		}},
		{"get_menu_top_items", func() (ToolResult, error) {
			return (&MenuTopItemsTool{}).Run(context.Background(), map[string]any{"period": "week"}, env)
		}},
		{"get_menu_underperformers", func() (ToolResult, error) {
			return (&MenuUnderperformersTool{}).Run(context.Background(), map[string]any{"period": "week"}, env)
		}},
	}
	for _, c := range runs {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.run()
			require.Error(t, err)
			require.Contains(t, err.Error(), "provider wired")
		})
	}
}
