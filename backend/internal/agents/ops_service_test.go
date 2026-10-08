package agents

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type opsReadStateStub struct {
	name  string
	data  map[string]any
	calls int
}

func (t *opsReadStateStub) Name() string             { return t.name }
func (t *opsReadStateStub) HumanLabel(string) string { return t.name }
func (t *opsReadStateStub) Description() string      { return t.name }
func (t *opsReadStateStub) Schema() *llm.JSONSchema  { return &llm.JSONSchema{Type: llm.TypeObject} }
func (t *opsReadStateStub) Run(context.Context, map[string]any, ToolEnv) (ToolResult, error) {
	t.calls++
	return ToolResult{Summary: "read state", Data: t.data}, nil
}

func TestOpsReadOnlyStateEnrichesDeterministicGuides(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		permission string
		toolName   string
		data       map[string]any
		contains   []string
	}{
		{
			name: "ai waiter enabled state", query: "How do I configure AI Waiter?",
			permission: "ai_waiter:read", toolName: "get_business_context",
			data:     map[string]any{"ai_enabled": true},
			contains: []string{"AI Waiter is enabled"},
		},
		{
			name: "plugin connection state", query: "How do I connect and test a plugin?",
			permission: "plugins:write", toolName: "get_plugin_status",
			data:     map[string]any{"plugins": []map[string]any{{"name": "stripe", "enabled": true, "connected": true, "status": "ok"}}},
			contains: []string{"1 plugin enabled", "1 connected", "ok"},
		},
		{
			name: "setup completion state", query: "How do I finish the initial setup?",
			permission: "overview:read", toolName: "get_setup_status",
			data:     map[string]any{"completed_count": 3, "total_count": 5, "required_done": true, "all_done": false},
			contains: []string{"3 of 5 setup steps are complete"},
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			biz := setupOpsReadOnlyStateBusiness(t, index)
			tool := &opsReadStateStub{name: tt.toolName, data: tt.data}
			registry := NewRegistry()
			registry.Register(tool)
			svc := NewOpsAssistantService(nil, registry, database.GetDBWrapper(), nil)

			result, err := svc.Ask(context.Background(), OpsAskRequest{
				BusinessID: biz.ID, Message: tt.query, Locale: "en", ActiveTab: "overview",
				Access: OpsAccessSnapshot{
					EffectivePermissions: map[string]bool{tt.permission: true},
				},
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, "deterministic-guide", result.Usage.Model, "read state must not invoke the LLM")
			assert.Equal(t, 1, tool.calls)
			for _, want := range tt.contains {
				assert.Contains(t, result.Response.Answer, want)
			}
		})
	}
}

func TestOpsReadOnlyStateRequiresEffectivePermission(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		toolName string
	}{
		{name: "ai waiter", query: "How do I configure AI Waiter?", toolName: "get_business_context"},
		{name: "plugins", query: "How do I connect and test a plugin?", toolName: "get_plugin_status"},
		{name: "setup", query: "How do I finish the initial setup?", toolName: "get_setup_status"},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			biz := setupOpsReadOnlyStateBusiness(t, index+10)
			tool := &opsReadStateStub{name: tt.toolName, data: map[string]any{
				"ai_enabled":      true,
				"completed_count": 5, "total_count": 5,
				"plugins": []map[string]any{{"name": "stripe", "enabled": true, "connected": true, "status": "ok"}},
			}}
			registry := NewRegistry()
			registry.Register(tool)
			svc := NewOpsAssistantService(nil, registry, database.GetDBWrapper(), nil)

			result, err := svc.Ask(context.Background(), OpsAskRequest{
				BusinessID: biz.ID, Message: tt.query, Locale: "en", ActiveTab: "overview",
				Access: OpsAccessSnapshot{EffectivePermissions: map[string]bool{}},
			})
			require.NoError(t, err)
			assert.Equal(t, 0, tool.calls, "permission denial must prevent even a read-tool call")
			assert.NotContains(t, result.Response.Answer, "Current state:")
		})
	}
}

func TestOpsReadOnlyStateSkipsToolForSuspension(t *testing.T) {
	tests := []struct {
		name   string
		access OpsAccessSnapshot
	}{
		{
			name: "suspension",
			access: OpsAccessSnapshot{
				EffectivePermissions: map[string]bool{"ai_waiter:read": true}, Suspended: true,
			},
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			biz := setupOpsReadOnlyStateBusiness(t, index+20)
			tool := &opsReadStateStub{name: "get_business_context", data: map[string]any{
				"ai_enabled": true,
			}}
			registry := NewRegistry()
			registry.Register(tool)
			svc := NewOpsAssistantService(nil, registry, database.GetDBWrapper(), nil)

			result, err := svc.Ask(context.Background(), OpsAskRequest{
				BusinessID: biz.ID, Message: "How do I configure AI Waiter?", Locale: "en", ActiveTab: "overview",
				Access: tt.access,
			})
			require.NoError(t, err)
			assert.Equal(t, 0, tool.calls)
			assert.NotContains(t, result.Response.Answer, "Current state:")
		})
	}
}

func TestOpsNoMutationToolRunsForDeterministicReadState(t *testing.T) {
	biz := setupOpsReadOnlyStateBusiness(t, 30)
	readTool := &opsReadStateStub{name: "get_business_context", data: map[string]any{
		"ai_enabled": true, "currency": "USD",
	}}
	mutationTrap := &opsReadStateStub{name: "enable_ai_waiter", data: map[string]any{"mutated": true}}
	registry := NewRegistry()
	registry.Register(readTool)
	registry.Register(mutationTrap)
	svc := NewOpsAssistantService(nil, registry, database.GetDBWrapper(), nil)

	before, err := database.GetBusinessByID(biz.ID)
	require.NoError(t, err)
	result, err := svc.Ask(context.Background(), OpsAskRequest{
		BusinessID: biz.ID, Message: "How do I configure AI Waiter?", Locale: "en", ActiveTab: "overview",
		Access: OpsAccessSnapshot{
			EffectivePermissions: map[string]bool{"ai_waiter:read": true},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	after, err := database.GetBusinessByID(biz.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, readTool.calls)
	assert.Equal(t, 0, mutationTrap.calls)
	assert.Equal(t, before.AiSettings, after.AiSettings)
	assert.Equal(t, before.IsActive, after.IsActive)
}

func setupOpsReadOnlyStateBusiness(t *testing.T, suffix int) *database.Business {
	t.Helper()
	setupOpsServiceTestDB(t)
	biz := &database.Business{
		BusinessId: fmt.Sprintf("ops-state-%d", suffix), Name: "State Cafe",
		OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2",
		IsActive:  true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	return biz
}

// The retention janitor deletes threads with no message for N days while the
// dashboard widget keeps the old thread id in localStorage. Asking on that id
// (or on another tenant's thread id) must open a fresh thread, not fail.
func TestOpsAskWithMissingThreadStartsNewThread(t *testing.T) {
	biz := setupOpsReadOnlyStateBusiness(t, 40)
	other, err := database.CreateOpsAssistantThread(biz.ID+1000, "other tenant", "en")
	require.NoError(t, err)
	svc := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)

	for _, staleID := range []uint{987654, other.ID} {
		id := staleID
		result, err := svc.Ask(context.Background(), OpsAskRequest{
			BusinessID: biz.ID, ThreadID: &id, Message: "How do I add a menu item?", Locale: "en", ActiveTab: "overview",
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotEqual(t, id, result.Thread.ID)
		require.Equal(t, biz.ID, result.Thread.BusinessID)
		_, err = database.GetOpsAssistantThreadByID(biz.ID, result.Thread.ID)
		require.NoError(t, err)
	}

	var otherMsgs int64
	require.NoError(t, database.GetDB().Model(&database.OpsAssistantMessage{}).
		Where("thread_id = ?", other.ID).Count(&otherMsgs).Error)
	require.Zero(t, otherMsgs, "another tenant's thread must not receive messages")
}
