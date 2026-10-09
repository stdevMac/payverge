package director_tools

import (
	"context"
	"fmt"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// AIWaiterPerformanceTool reports on the AI Waiter feature's traffic
// inside a day/week/month window: how many conversations the bot held,
// how many messages flowed through, what share of conversations led
// to an upsell (cart_items_added > 0), and how many sessions remain
// active right now.
//
// Mirrors the small DirectorConsoleService.buildAIWaiterStats logic
// but adds a period filter so the model can ask "this week" or
// "today" without scoping at the prompt level.
type AIWaiterPerformanceTool struct{}

// Name is the snake_case function identifier sent to the model.
func (t *AIWaiterPerformanceTool) Name() string { return "get_ai_waiter_performance" }

// HumanLabel is the localized pill label the UI shows while the tool runs.
func (t *AIWaiterPerformanceTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Revisando desempeño del AI Waiter"
	case "fr":
		return "Vérification des performances du serveur IA"
	case "ar":
		return "مراجعة أداء النادل الذكي"
	default:
		return "Reading AI Waiter performance"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *AIWaiterPerformanceTool) Description() string {
	return "Returns AI Waiter activity for a window: conversation count, message volume, upsell success rate (share of chats that added to cart), and active sessions. Call for questions about the AI waiter / assisted-selling performance or chat engagement."
}

// Schema declares the argument shape the model sees.
func (t *AIWaiterPerformanceTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"period": {
				Type:        llm.TypeString,
				Description: "Time window. One of: day, week, month. Default: week.",
				Enum:        []string{"day", "week", "month"},
			},
		},
	}
}

// Run queries ai_waiter_conversations and ai_waiter_messages for the
// window and returns the four headline metrics.
func (t *AIWaiterPerformanceTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("get_ai_waiter_performance: nil DB in tool env")
	}

	period, err := normalizeWaiterPeriod(args)
	if err != nil {
		return ToolResult{}, err
	}

	start := funnelPeriodStart(period, time.Now())
	gormDB := env.DB.GetGorm()

	var conversations int64
	if err := gormDB.Model(&database.AiWaiterConversation{}).
		Where("business_id = ? AND created_at >= ?", env.BusinessID, start).
		Count(&conversations).Error; err != nil {
		return ToolResult{}, fmt.Errorf("get_ai_waiter_performance: count conversations: %w", err)
	}

	var messages int64
	if err := gormDB.Model(&database.AiWaiterMessage{}).
		Joins("JOIN ai_waiter_conversations ON ai_waiter_messages.conversation_id = ai_waiter_conversations.id").
		Where("ai_waiter_conversations.business_id = ? AND ai_waiter_conversations.created_at >= ?", env.BusinessID, start).
		Count(&messages).Error; err != nil {
		return ToolResult{}, fmt.Errorf("get_ai_waiter_performance: count messages: %w", err)
	}

	var upsells int64
	if err := gormDB.Model(&database.AiWaiterConversation{}).
		Where("business_id = ? AND created_at >= ? AND cart_items_added > 0", env.BusinessID, start).
		Count(&upsells).Error; err != nil {
		return ToolResult{}, fmt.Errorf("get_ai_waiter_performance: count upsells: %w", err)
	}

	var active int64
	if err := gormDB.Model(&database.AiWaiterConversation{}).
		Where("business_id = ? AND status = ?", env.BusinessID, "active").
		Count(&active).Error; err != nil {
		return ToolResult{}, fmt.Errorf("get_ai_waiter_performance: count active: %w", err)
	}

	upsellRate := 0.0
	if conversations > 0 {
		upsellRate = float64(upsells) / float64(conversations) * 100.0
	}

	summary := fmt.Sprintf(
		"AI Waiter: %d chats this %s, %.0f%% upsell",
		conversations, period, upsellRate,
	)

	return ToolResult{
		Summary: summary,
		Data: map[string]any{
			"period":          period,
			"conversations":   conversations,
			"messages":        messages,
			"upsell_rate_pct": upsellRate,
			"active":          active,
		},
	}, nil
}

// normalizeWaiterPeriod validates the period arg against the
// {day, week, month} allow-list, defaulting to "week".
func normalizeWaiterPeriod(args map[string]any) (string, error) {
	raw, ok := args["period"]
	if !ok {
		return "week", nil
	}
	str, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("get_ai_waiter_performance: period must be a string, got %T", raw)
	}
	if str == "" {
		return "week", nil
	}
	switch str {
	case "day", "week", "month":
		return str, nil
	default:
		return "", fmt.Errorf("get_ai_waiter_performance: unsupported period %q (allowed: day, week, month)", str)
	}
}
