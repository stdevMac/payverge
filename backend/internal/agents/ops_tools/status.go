package ops_tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// PluginStatusTool exposes only coarse connection health for plugins owned by
// the current business. It deliberately does not select configuration or
// provider payload columns.
type PluginStatusTool struct{}

func (t *PluginStatusTool) Name() string { return "get_plugin_status" }
func (t *PluginStatusTool) HumanLabel(locale string) string {
	return label(locale, "Reading plugin status", "Revisando estado de plugins")
}
func (t *PluginStatusTool) Description() string {
	return "Returns enabled, connected, and coarse status facts for the current business plugins."
}
func (t *PluginStatusTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{Type: llm.TypeObject, Properties: map[string]*llm.JSONSchema{}}
}

func (t *PluginStatusTool) Run(_ context.Context, _ map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	if env.DB == nil || env.BusinessID == 0 {
		return agents.ToolResult{}, fmt.Errorf("get_plugin_status: missing business context")
	}

	type pluginStateRow struct {
		Name       string `gorm:"column:name"`
		IsEnabled  bool   `gorm:"column:is_enabled"`
		LastStatus string `gorm:"column:last_status"`
	}
	var rows []pluginStateRow
	err := env.DB.GetGorm().
		Table("business_plugins AS bp").
		Select("p.name, bp.is_enabled, bp.last_status").
		Joins("JOIN plugins AS p ON p.id = bp.plugin_id").
		Where("bp.business_id = ? AND p.is_active = ?", env.BusinessID, true).
		Order("p.name").
		Scan(&rows).Error
	if err != nil {
		return agents.ToolResult{}, fmt.Errorf("get_plugin_status: %w", err)
	}

	plugins := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		status := strings.ToLower(strings.TrimSpace(row.LastStatus))
		if status != "ok" && status != "error" {
			status = "unknown"
		}
		plugins = append(plugins, map[string]any{
			"name": row.Name, "enabled": row.IsEnabled,
			"connected": row.IsEnabled && status == "ok", "status": status,
		})
	}

	return agents.ToolResult{
		Summary: fmt.Sprintf("Loaded status for %d plugins", len(plugins)),
		Data:    map[string]any{"plugins": plugins},
	}, nil
}
