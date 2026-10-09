package ops_tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

func strArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func label(locale, en, es string) string {
	switch locale {
	case "es", "es_ar", "es-ar", "es-AR":
		return es
	default:
		return en
	}
}

func dashboardHref(businessID uint, tab string) string {
	return fmt.Sprintf("/business/%d/dashboard?tab=%s", businessID, tab)
}

type BusinessContextTool struct{}

func (t *BusinessContextTool) Name() string { return "get_business_context" }
func (t *BusinessContextTool) HumanLabel(locale string) string {
	return label(locale, "Loading business", "Cargando negocio")
}
func (t *BusinessContextTool) Description() string {
	return "Returns business name, currency, timezone, kitchen/orders flags, AI enabled state."
}
func (t *BusinessContextTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{Type: llm.TypeObject, Properties: map[string]*llm.JSONSchema{}}
}
func (t *BusinessContextTool) Run(_ context.Context, _ map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	if env.DB == nil || env.BusinessID == 0 {
		return agents.ToolResult{}, fmt.Errorf("missing business context")
	}
	biz, err := env.DB.GetBusinessByID(env.BusinessID)
	if err != nil {
		return agents.ToolResult{}, err
	}
	data := map[string]any{
		"id": biz.ID, "name": biz.Name, "currency": biz.DefaultCurrency,
		"timezone":        biz.Timezone,
		"kitchen_enabled": biz.KitchenEnabled, "orders_enabled": biz.OrdersEnabled,
		"ai_enabled":  database.IsAIWaiterAvailable(biz),
		"operational": database.IsBusinessOperational(biz),
		"active_tab":  env.ActiveTab, "staff_role": env.StaffRole,
	}
	return agents.ToolResult{Summary: "Loaded business context", Data: data}, nil
}

type SetupStatusTool struct{}

func (t *SetupStatusTool) Name() string { return "get_setup_status" }
func (t *SetupStatusTool) HumanLabel(locale string) string {
	return label(locale, "Checking setup", "Revisando configuración")
}
func (t *SetupStatusTool) Description() string {
	return "Returns onboarding setup steps: profile, tables, menu, staff, payment completion."
}
func (t *SetupStatusTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{Type: llm.TypeObject, Properties: map[string]*llm.JSONSchema{}}
}
func (t *SetupStatusTool) Run(_ context.Context, _ map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	if env.DB == nil || env.BusinessID == 0 {
		return agents.ToolResult{}, fmt.Errorf("missing business context")
	}
	status, err := computeSetupStatusForBusiness(env.DB.GetGorm(), env.BusinessID)
	if err != nil {
		return agents.ToolResult{}, err
	}
	return agents.ToolResult{Summary: "Setup status loaded", Data: status}, nil
}

type ActiveTabHelpTool struct{}

func (t *ActiveTabHelpTool) Name() string { return "get_active_tab_help" }
func (t *ActiveTabHelpTool) HumanLabel(locale string) string {
	return label(locale, "Loading tab help", "Cargando ayuda")
}
func (t *ActiveTabHelpTool) Description() string {
	return "Returns curated playbook help for the active dashboard tab."
}
func (t *ActiveTabHelpTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"tab": {Type: llm.TypeString, Description: "Optional tab override; defaults to active tab"},
		},
	}
}
func (t *ActiveTabHelpTool) Run(_ context.Context, args map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	tab := strArg(args, "tab")
	if tab == "" {
		tab = env.ActiveTab
	}
	body, err := agents.LoadOpsPlaybook(tab)
	if err != nil {
		return agents.ToolResult{Summary: "No playbook for tab", Data: map[string]any{"tab": tab, "found": false}}, nil
	}
	return agents.ToolResult{Summary: "Tab help loaded", Data: map[string]any{"tab": tab, "found": true, "body": body}}, nil
}

type NavigateToTabTool struct{}

func (t *NavigateToTabTool) Name() string { return "navigate_to_tab" }
func (t *NavigateToTabTool) HumanLabel(locale string) string {
	return label(locale, "Finding tab", "Buscando pestaña")
}
func (t *NavigateToTabTool) Description() string {
	return "Returns deep link metadata for a dashboard tab including lock state."
}
func (t *NavigateToTabTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"tab": {Type: llm.TypeString, Description: "Dashboard tab key"},
		},
		Required: []string{"tab"},
	}
}
func (t *NavigateToTabTool) Run(_ context.Context, args map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	tab := strings.TrimSpace(strArg(args, "tab"))
	meta := tabLockMeta(tab, env)
	href := dashboardHref(env.BusinessID, tab)
	return agents.ToolResult{
		Summary: "Navigation resolved",
		Data: map[string]any{
			"label": tab, "href": href, "locked": meta.Locked,
			"hidden": meta.Hidden,
		},
	}, nil
}

type NavigateToSettingsTool struct{}

func (t *NavigateToSettingsTool) Name() string { return "navigate_to_settings" }
func (t *NavigateToSettingsTool) HumanLabel(locale string) string {
	return label(locale, "Opening settings", "Abriendo ajustes")
}
func (t *NavigateToSettingsTool) Description() string {
	return "Returns settings sub-route: general, printers, fiscal, plugins."
}
func (t *NavigateToSettingsTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"section": {Type: llm.TypeString, Enum: []string{"general", "printers", "fiscal", "plugins"}},
		},
	}
}
func (t *NavigateToSettingsTool) Run(_ context.Context, args map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	section := strArg(args, "section")
	href := dashboardHref(env.BusinessID, "settings")
	if section != "" && section != "general" {
		href += "&section=" + section
	}
	return agents.ToolResult{Summary: "Settings link", Data: map[string]any{"href": href, "section": section}}, nil
}

type ListAccessibleTabsTool struct{}

func (t *ListAccessibleTabsTool) Name() string { return "list_accessible_tabs" }
func (t *ListAccessibleTabsTool) HumanLabel(locale string) string {
	return label(locale, "Listing tabs", "Listando pestañas")
}
func (t *ListAccessibleTabsTool) Description() string {
	return "Lists dashboard tabs the caller can access with lock metadata."
}
func (t *ListAccessibleTabsTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{Type: llm.TypeObject, Properties: map[string]*llm.JSONSchema{}}
}
func (t *ListAccessibleTabsTool) Run(_ context.Context, _ map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	tabs := allTabKeys()
	out := make([]map[string]any, 0, len(tabs))
	for _, tab := range tabs {
		meta := tabLockMeta(tab, env)
		if meta.Hidden {
			continue
		}
		out = append(out, map[string]any{
			"tab": tab, "href": dashboardHref(env.BusinessID, tab),
			"locked": meta.Locked,
		})
	}
	return agents.ToolResult{Summary: "Listed tabs", Data: map[string]any{"tabs": out}}, nil
}

type ExplainLockedFeatureTool struct{}

func (t *ExplainLockedFeatureTool) Name() string { return "explain_locked_feature" }
func (t *ExplainLockedFeatureTool) HumanLabel(locale string) string {
	return label(locale, "Checking access", "Verificando acceso")
}
func (t *ExplainLockedFeatureTool) Description() string {
	return "Explain why a tab or feature is locked (suspended, RBAC)."
}
func (t *ExplainLockedFeatureTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"tab": {Type: llm.TypeString},
		},
		Required: []string{"tab"},
	}
}
func (t *ExplainLockedFeatureTool) Run(_ context.Context, args map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	tab := strArg(args, "tab")
	meta := tabLockMeta(tab, env)
	reason := "available"
	if meta.Locked {
		if env.IsSuspended {
			reason = "Business suspended — contact the server administrator"
		} else {
			reason = "Locked for your role"
		}
	}
	return agents.ToolResult{Summary: reason, Data: map[string]any{"tab": tab, "locked": meta.Locked, "reason": reason}}, nil
}

type SearchOperatorHelpTool struct{}

func (t *SearchOperatorHelpTool) Name() string { return "search_operator_help" }
func (t *SearchOperatorHelpTool) HumanLabel(locale string) string {
	return label(locale, "Searching help", "Buscando ayuda")
}
func (t *SearchOperatorHelpTool) Description() string {
	return "Search operator FAQ and playbooks for procedural answers."
}
func (t *SearchOperatorHelpTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"query": {Type: llm.TypeString},
			"tab":   {Type: llm.TypeString},
		},
		Required: []string{"query"},
	}
}
func (t *SearchOperatorHelpTool) Run(_ context.Context, args map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	query := strArg(args, "query")
	tab := strArg(args, "tab")
	if tab == "" {
		tab = env.ActiveTab
	}
	// Locale-aware catalog search when available; falls back to English.
	locale := env.Locale
	matches := opsGuideCatalog.Search(query, locale, tab, 5)
	hits := make([]map[string]any, 0, len(matches))
	for _, m := range matches {
		g := m.Guide
		hits = append(hits, map[string]any{
			"id": g.ID, "tab": g.Tab, "answer": g.Answer,
			"steps": g.Steps, "destination": g.Destination,
			"required_permission": g.RequiredPermission,
			"score":               m.Score, "exact": m.Exact,
		})
	}
	return agents.ToolResult{Summary: "Help search done", Data: map[string]any{"results": hits}}, nil
}
