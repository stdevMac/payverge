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

func TestAIWaiterPerformanceTool_Name(t *testing.T) {
	tool := &AIWaiterPerformanceTool{}
	assert.Equal(t, "get_ai_waiter_performance", tool.Name())
}

func TestAIWaiterPerformanceTool_HumanLabel(t *testing.T) {
	tool := &AIWaiterPerformanceTool{}
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
}

func TestAIWaiterPerformanceTool_Schema(t *testing.T) {
	tool := &AIWaiterPerformanceTool{}
	schema := tool.Schema()
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties["period"])
}

func TestAIWaiterPerformanceTool_Run(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "AI Waiter Bistro")
	gormDB := db.GetGorm()

	now := time.Now()
	// 4 conversations total: 2 with upsell (cart_items_added > 0),
	// 1 active. All within the past week.
	convs := []database.AiWaiterConversation{
		{SessionID: "s1", BusinessID: bizID, TableCode: "T1", Mode: "ordering", Status: "active", CartItemsAdded: 2, CreatedAt: now.AddDate(0, 0, -1), UpdatedAt: now},
		{SessionID: "s2", BusinessID: bizID, TableCode: "T2", Mode: "ordering", Status: "closed", CartItemsAdded: 1, CreatedAt: now.AddDate(0, 0, -3), UpdatedAt: now},
		{SessionID: "s3", BusinessID: bizID, TableCode: "T3", Mode: "ordering", Status: "closed", CartItemsAdded: 0, CreatedAt: now.AddDate(0, 0, -4), UpdatedAt: now},
		{SessionID: "s4", BusinessID: bizID, TableCode: "T4", Mode: "concierge", Status: "closed", CartItemsAdded: 0, CreatedAt: now.AddDate(0, 0, -6), UpdatedAt: now},
	}
	for i := range convs {
		require.NoError(t, gormDB.Create(&convs[i]).Error)
	}

	// 5 messages across conversations.
	msgs := []database.AiWaiterMessage{
		{ConversationID: convs[0].ID, Role: "user", Content: "menu?", CreatedAt: now},
		{ConversationID: convs[0].ID, Role: "assistant", Content: "Sure", CreatedAt: now},
		{ConversationID: convs[1].ID, Role: "user", Content: "wine?", CreatedAt: now},
		{ConversationID: convs[1].ID, Role: "assistant", Content: "Try the red", CreatedAt: now},
		{ConversationID: convs[2].ID, Role: "user", Content: "hello", CreatedAt: now},
	}
	for i := range msgs {
		require.NoError(t, gormDB.Create(&msgs[i]).Error)
	}

	tool := &AIWaiterPerformanceTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"period": "week"}, env)
	require.NoError(t, err)

	assert.Equal(t, "week", result.Data["period"])
	assert.Equal(t, int64(4), result.Data["conversations"])
	assert.Equal(t, int64(5), result.Data["messages"])
	assert.Equal(t, int64(1), result.Data["active"])
	assert.InDelta(t, 50.0, result.Data["upsell_rate_pct"].(float64), 0.001)
	assert.Contains(t, result.Summary, "4")
}

func TestAIWaiterPerformanceTool_DefaultsToWeek(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Default AI Waiter Bistro")
	tool := &AIWaiterPerformanceTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{}, env)
	require.NoError(t, err)
	assert.Equal(t, "week", result.Data["period"])
}

func TestAIWaiterPerformanceTool_ValidatesPeriod(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bad Period AI Waiter")
	tool := &AIWaiterPerformanceTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	_, err := tool.Run(context.Background(), map[string]any{"period": "year"}, env)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "period"))
}
