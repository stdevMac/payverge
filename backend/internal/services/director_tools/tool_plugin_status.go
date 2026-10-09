package director_tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// PluginStatusTool reports which plugins a business already has on
// plus recommended ones it could still enable. Mirrors the small
// DirectorConsoleService.buildPluginInsights logic but exposes a
// category filter so the model can scope to a single product surface
// without re-implementing the join itself.
type PluginStatusTool struct{}

// pluginStatusCategories lists the allowed `category` values, in
// addition to "all" which is the default.
var pluginStatusCategories = []string{
	"all",
	database.PluginCategoryAnalytics,
	database.PluginCategoryReporting,
	database.PluginCategoryMarketing,
	database.PluginCategoryPayment,
}

// Name is the snake_case function identifier sent to the model.
func (t *PluginStatusTool) Name() string { return "get_plugin_status" }

// HumanLabel is the localized pill label the UI shows while the tool runs.
func (t *PluginStatusTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Revisando estado de plugins"
	case "fr":
		return "Vérification de l'état des plugins"
	case "ar":
		return "مراجعة حالة الإضافات"
	default:
		return "Reading plugin status"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *PluginStatusTool) Description() string {
	return "Returns which integrations/plugins are enabled for the business plus recommended ones it has not enabled yet, optionally filtered by category (analytics, reporting, marketing, payment). Call for integration, automation, reporting-coverage, or growth-tooling questions."
}

// Schema declares the argument shape the model sees.
func (t *PluginStatusTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"category": {
				Type:        llm.TypeString,
				Description: "Optional category filter. One of: all, analytics, reporting, marketing, payment. Default: all.",
				Enum:        pluginStatusCategories,
			},
		},
	}
}

// Run lists enabled plugins for the business and pairs them with up
// to 5 recommended (active, not enabled, matching category) plugins.
func (t *PluginStatusTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("get_plugin_status: nil DB in tool env")
	}

	category, err := normalizePluginCategory(args)
	if err != nil {
		return ToolResult{}, err
	}

	businessPlugins, err := database.GetBusinessPlugins(env.BusinessID)
	if err != nil {
		return ToolResult{}, fmt.Errorf("get_plugin_status: load business plugins: %w", err)
	}

	enabled := make([]map[string]any, 0)
	enabledNames := map[string]struct{}{}
	for _, p := range businessPlugins {
		if !pluginBool(p["is_enabled"]) {
			continue
		}
		name := pluginString(p["name"])
		if name == "" {
			continue
		}
		pluginCategory := pluginString(p["category"])
		if !categoryMatches(category, pluginCategory) {
			enabledNames[name] = struct{}{} // still consider it "taken"
			continue
		}
		enabledNames[name] = struct{}{}
		enabled = append(enabled, map[string]any{
			"name":         name,
			"display_name": pluginString(p["display_name"]),
			"category":     pluginCategory,
		})
	}
	sort.SliceStable(enabled, func(i, j int) bool {
		return strings.ToLower(pluginString(enabled[i]["display_name"])) <
			strings.ToLower(pluginString(enabled[j]["display_name"]))
	})

	recommended := make([]map[string]any, 0)
	platformPlugins, err := database.GetAllPlugins(true)
	if err == nil {
		for _, p := range platformPlugins {
			if _, taken := enabledNames[p.Name]; taken {
				continue
			}
			if p.ComingSoon {
				continue
			}
			if !categoryMatches(category, p.Category) {
				continue
			}
			recommended = append(recommended, map[string]any{
				"name":      p.Name,
				"title":     p.DisplayName,
				"category":  p.Category,
				"deep_link": pluginDeepLink(env.BusinessID),
			})
			if len(recommended) >= 5 {
				break
			}
		}
	}

	suffix := category
	if suffix == "all" {
		suffix = "plugin"
	}
	summary := fmt.Sprintf(
		"%d plugins on · %d %s recommendations",
		len(enabled), len(recommended), suffix,
	)

	return ToolResult{
		Summary: summary,
		Data: map[string]any{
			"category":    category,
			"enabled":     enabled,
			"recommended": recommended,
		},
	}, nil
}

// normalizePluginCategory validates the optional category arg,
// defaulting to "all".
func normalizePluginCategory(args map[string]any) (string, error) {
	raw, ok := args["category"]
	if !ok {
		return "all", nil
	}
	str, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("get_plugin_status: category must be a string, got %T", raw)
	}
	if str == "" {
		return "all", nil
	}
	for _, allowed := range pluginStatusCategories {
		if str == allowed {
			return str, nil
		}
	}
	return "", fmt.Errorf("get_plugin_status: unsupported category %q (allowed: %v)", str, pluginStatusCategories)
}

// categoryMatches returns true when the candidate row should pass the
// filter. "all" passes everything; any other filter matches exactly.
func categoryMatches(filter, candidate string) bool {
	if filter == "all" {
		return true
	}
	return filter == candidate
}

// pluginBool defensively converts the loosely-typed is_enabled field
// returned by database.GetBusinessPlugins. SQLite hands us int64 (0/1),
// Postgres hands us bool, and both run through this package's tests
// and production. Numeric strings ("1") are also tolerated.
func pluginBool(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int:
		return x != 0
	case int32:
		return x != 0
	case int64:
		return x != 0
	case uint:
		return x != 0
	case uint64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x == "1" || strings.EqualFold(x, "true") || strings.EqualFold(x, "t")
	default:
		return false
	}
}

// pluginString defensively converts the loosely-typed map[string]interface{}
// rows returned by database.GetBusinessPlugins into a string.
func pluginString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// pluginDeepLink builds the dashboard deep-link to the plugins tab for
// a business. Kept local to avoid pulling in services/director_console_service.
// The format must match buildTabDeepLink's canonical
// /business/<id>/dashboard?tab=<tab> shape (the prompt mandates it) so the
// model can quote the tool's link verbatim into actions[].deep_link.
func pluginDeepLink(businessID uint) string {
	return fmt.Sprintf("/business/%d/dashboard?tab=plugins", businessID)
}
