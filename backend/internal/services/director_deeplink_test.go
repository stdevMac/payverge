package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeepLinkRewrite_SuffixesTitleWithTab(t *testing.T) {
	svc := &DirectorConsoleService{}
	out := DirectorStructuredResponse{
		Summary: "x",
		ActionPlan: []DirectorAction{
			{Title: "Review reservations", Description: "d", DeepLink: "/business/1/dashboard?tab=bogus", Priority: "high"},
		},
	}
	got := svc.normalizeStructuredOutput(1, "en", "how are reservations", out, nil)
	a := got.ActionPlan[0]
	assert.Contains(t, a.DeepLink, "tab=analytics", "bogus tab rewritten to analytics")
	assert.Contains(t, strings.ToLower(a.Title), "analytics", "title must name the actual destination")
}

func TestDeepLinkRewrite_ValidTabUntouched(t *testing.T) {
	svc := &DirectorConsoleService{}
	out := DirectorStructuredResponse{
		Summary: "x",
		ActionPlan: []DirectorAction{
			{Title: "Open menu", Description: "d", DeepLink: "/business/1/dashboard?tab=menu", Priority: "high"},
		},
	}
	got := svc.normalizeStructuredOutput(1, "en", "menu", out, nil)
	assert.Equal(t, "Open menu", got.ActionPlan[0].Title, "valid tab must not suffix the title")
	assert.Contains(t, got.ActionPlan[0].DeepLink, "tab=menu")
}

// #579: inventory ingredients (stock items) must ground to the Inventory
// tab, not get force-rewritten to analytics like a genuinely unknown tab.
// This is the same target the proactive-insight alert card's "Abrir" CTA
// already used (buildProactiveInsightsWithReports) — the LLM chat path was
// the only place still missing "inventory" from the allow-list.
func TestDeepLinkRewrite_InventoryTabAllowed(t *testing.T) {
	svc := &DirectorConsoleService{}
	out := DirectorStructuredResponse{
		Summary: "x",
		ActionPlan: []DirectorAction{
			{Title: "Verify physical inventory", Description: "Confirm Premium Beef stock on hand", DeepLink: "/business/86/dashboard?tab=inventory", Priority: "high"},
		},
	}
	got := svc.normalizeStructuredOutput(86, "en", "premium beef is out of stock", out, nil)
	a := got.ActionPlan[0]
	assert.Equal(t, "Verify physical inventory", a.Title, "valid inventory tab must not suffix the title")
	assert.Equal(t, "/business/86/dashboard?tab=inventory", a.DeepLink)
	assert.Equal(t, buildTabDeepLink("86", "inventory"), a.DeepLink)
}

// #579: es-AR locale must not change the deep-link shape — it stays the
// bare, locale-prefix-free /business/{id}/dashboard?tab=inventory contract;
// the frontend is responsible for preserving any locale prefix on navigation.
func TestDeepLinkRewrite_InventoryTabAllowed_EsAR(t *testing.T) {
	svc := &DirectorConsoleService{}
	out := DirectorStructuredResponse{
		Summary: "x",
		ActionPlan: []DirectorAction{
			{Title: "Verificar inventario físico", Description: "Confirmá el stock de Premium Beef", DeepLink: "/business/86/dashboard?tab=inventory", Priority: "high"},
		},
	}
	got := svc.normalizeStructuredOutput(86, "es_ar", "premium beef sin stock", out, nil)
	a := got.ActionPlan[0]
	assert.Equal(t, "Verificar inventario físico", a.Title, "valid inventory tab must not suffix the title")
	assert.Equal(t, "/business/86/dashboard?tab=inventory", a.DeepLink)
}

// S3-Loop: marketing tab is a first-class Director deep-link target (Library).
func TestDeepLinkRewrite_MarketingTabAllowed(t *testing.T) {
	svc := &DirectorConsoleService{}
	out := DirectorStructuredResponse{
		Summary: "x",
		ActionPlan: []DirectorAction{
			{Title: "Open Marketing Library", Description: "Review posted creatives", DeepLink: "/business/9/dashboard?tab=marketing", Priority: "medium"},
		},
	}
	got := svc.normalizeStructuredOutput(9, "en", "marketing posts", out, nil)
	assert.Equal(t, "Open Marketing Library", got.ActionPlan[0].Title)
	assert.Equal(t, "/business/9/dashboard?tab=marketing", got.ActionPlan[0].DeepLink)
	assert.Equal(t, buildTabDeepLink("9", "marketing"), got.ActionPlan[0].DeepLink)
}

func TestEmptyFields_NotPaddedWithBoilerplate(t *testing.T) {
	svc := &DirectorConsoleService{}
	out := DirectorStructuredResponse{
		Summary:        "Sales are up 5%.",
		Diagnosis:      "",
		Evidence:       []string{},
		ExpectedImpact: "",
		ActionPlan:     []DirectorAction{{Title: "A", Description: "d", DeepLink: "/business/1/dashboard?tab=analytics", Priority: "high"}},
	}
	got := svc.normalizeStructuredOutput(1, "en", "sales", out, nil)
	assert.Empty(t, got.Diagnosis, "empty diagnosis must not be padded")
	assert.Empty(t, got.Evidence, "empty evidence must not be padded")
	assert.Empty(t, got.ExpectedImpact, "empty expected_impact must not be padded")
	assert.Equal(t, "Sales are up 5%.", got.Summary)
}
