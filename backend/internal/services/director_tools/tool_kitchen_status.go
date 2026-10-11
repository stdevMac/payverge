package director_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// KitchenStatusTool is a READ of live kitchen tickets and kitchen-role staff.
type KitchenStatusTool struct{}

func (t *KitchenStatusTool) Name() string { return "get_kitchen_status" }

func (t *KitchenStatusTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Revisando la cocina"
	case "fr":
		return "Lecture de la cuisine"
	case "ar":
		return "قراءة حالة المطبخ"
	default:
		return "Reading the kitchen"
	}
}

func (t *KitchenStatusTool) Description() string {
	return "Returns tickets currently in the kitchen or ready to run, plus staff whose role is kitchen. Call for 'who is in the kitchen', 'what's on the board', or kitchen throughput. Does not mark tickets ready. Use get_live_floor for tables and waits."
}

func (t *KitchenStatusTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{Type: llm.TypeObject, Properties: map[string]*llm.JSONSchema{}}
}

func (t *KitchenStatusTool) Run(_ context.Context, _ map[string]any, env ToolEnv) (ToolResult, error) {
	if env.DB == nil {
		return ToolResult{}, fmt.Errorf("get_kitchen_status: nil DB in tool env")
	}

	var inKitchen int64
	var ready int64
	_ = env.DB.GetGorm().Model(&database.Order{}).
		Where("business_id = ? AND status = ?", env.BusinessID, database.OrderStatusInKitchen).
		Count(&inKitchen).Error
	_ = env.DB.GetGorm().Model(&database.Order{}).
		Where("business_id = ? AND status = ?", env.BusinessID, database.OrderStatusOrderReady).
		Count(&ready).Error

	var orders []database.Order
	_ = env.DB.GetGorm().
		Select("id", "order_number", "status", "items", "created_at").
		Where("business_id = ? AND status IN ?", env.BusinessID, []database.OrderStatus{
			database.OrderStatusInKitchen, database.OrderStatusOrderReady,
		}).
		Order("created_at ASC").
		Limit(20).
		Find(&orders).Error

	tickets := make([]map[string]any, 0, len(orders))
	for _, o := range orders {
		tickets = append(tickets, map[string]any{
			"ticket": o.OrderNumber,
			"status": string(o.Status),
			"items":  kitchenItemNames(o.Items),
		})
	}

	var staffRows []database.Staff
	_ = env.DB.GetGorm().
		Select("name").
		Where("business_id = ? AND is_active = ? AND role = ?", env.BusinessID, true, database.StaffRoleKitchen).
		Order("name ASC").
		Find(&staffRows).Error
	staff := make([]string, 0, len(staffRows))
	for _, s := range staffRows {
		if strings.TrimSpace(s.Name) != "" {
			staff = append(staff, s.Name)
		}
	}

	spanish := strings.HasPrefix(strings.ToLower(strings.ReplaceAll(env.Locale, "_", "-")), "es")
	summary := kitchenSummary(spanish, int(inKitchen), int(ready), staff)

	return ToolResult{
		Summary: summary,
		Data: map[string]any{
			"in_kitchen":    int(inKitchen),
			"ready":         int(ready),
			"tickets":       tickets,
			"kitchen_staff": staff,
		},
	}, nil
}

func kitchenSummary(spanish bool, inKitchen, ready int, staff []string) string {
	names := "nadie asignado"
	if !spanish {
		names = "no kitchen staff listed"
	}
	if len(staff) > 0 {
		names = strings.Join(staff, ", ")
	}
	if spanish {
		return fmt.Sprintf("Cocina: %d tickets en preparación, %d listos. En cocina: %s.", inKitchen, ready, names)
	}
	return fmt.Sprintf("Kitchen: %d tickets in progress, %d ready. On kitchen: %s.", inKitchen, ready, names)
}

func kitchenItemNames(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var items []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{}
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if strings.TrimSpace(it.Name) != "" {
			out = append(out, it.Name)
		}
	}
	return out
}
