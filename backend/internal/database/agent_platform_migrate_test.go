package database

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAgentPlatformTestDB migrates ONLY the agent-platform models through the
// same AutoMigrate call the production safety net uses, proving the ops
// assistant thread/message tables materialize even when the SQL migration pass
// was skipped (the prod 500 root cause).
func setupAgentPlatformTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file:agent_platform_migrate_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	for _, m := range []any{
		&OpsAssistantToolCall{}, &OpsAssistantMessage{}, &OpsAssistantThread{},
		&Escalation{},
	} {
		_ = gormDB.Migrator().DropTable(m)
	}
	require.NoError(t, gormDB.AutoMigrate(
		&OpsAssistantThread{},
		&OpsAssistantMessage{},
		&OpsAssistantToolCall{},
		&Escalation{},
	))
	db = gormDB
}

func TestAgentPlatformAutoMigrateCreatesTables(t *testing.T) {
	setupAgentPlatformTestDB(t)

	for _, table := range []string{
		"ops_assistant_threads", "ops_assistant_messages", "ops_assistant_tool_calls",
		"escalations",
	} {
		require.Truef(t, GetDB().Migrator().HasTable(table), "table %s must exist after AutoMigrate", table)
	}
}

// TestSetOpsAssistantMessageFeedbackNotFound proves the DB helper reports a
// not-found error (rows-affected == 0) when the message id is unknown OR belongs
// to another business — the cross-tenant scoping is preserved by the business_id
// predicate, so the handler can answer 404 instead of a misleading 200.
func TestSetOpsAssistantMessageFeedbackNotFound(t *testing.T) {
	setupAgentPlatformTestDB(t)

	thread, err := CreateOpsAssistantThread(7, "help", "en")
	require.NoError(t, err)
	msg := &OpsAssistantMessage{
		ThreadID: thread.ID, BusinessID: 7,
		Role: OpsAssistantRoleAssistant, Locale: "en", Content: "answer",
	}
	require.NoError(t, SaveOpsAssistantMessage(msg))

	// Unknown message id -> not found.
	require.ErrorIs(t, SetOpsAssistantMessageFeedback(7, 999999, "up"), ErrOpsAssistantMessageNotFound)

	// Real message id but WRONG business -> not found (cross-tenant blocked).
	require.ErrorIs(t, SetOpsAssistantMessageFeedback(8, msg.ID, "up"), ErrOpsAssistantMessageNotFound)

	// Correct business + id -> success, and the other business still cannot read it.
	require.NoError(t, SetOpsAssistantMessageFeedback(7, msg.ID, "down"))
	var stored OpsAssistantMessage
	require.NoError(t, db.First(&stored, msg.ID).Error)
	require.Equal(t, "down", stored.Feedback)
}

func TestSaveOpsAssistantMessageRoundTrips(t *testing.T) {
	setupAgentPlatformTestDB(t)

	thread, err := CreateOpsAssistantThread(42, "Help with menu", "en")
	require.NoError(t, err)
	require.NotZero(t, thread.ID)

	msg := &OpsAssistantMessage{
		ThreadID:   thread.ID,
		BusinessID: 42,
		Role:       OpsAssistantRoleUser,
		Locale:     "en",
		Content:    "how do I add a table?",
	}
	require.NoError(t, SaveOpsAssistantMessage(msg))
	require.NotZero(t, msg.ID)

	got, err := ListOpsAssistantMessages(42, thread.ID, 100)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "how do I add a table?", got[0].Content)
	require.Equal(t, OpsAssistantRoleUser, got[0].Role)
}

// TestListOpsAssistantMessagesReturnsRecentInChronologicalOrder proves that when
// a thread has more messages than the limit, the MOST RECENT `limit` messages are
// returned (not the oldest), and they are ordered oldest-first for display/context.
func TestListOpsAssistantMessagesReturnsRecentInChronologicalOrder(t *testing.T) {
	setupAgentPlatformTestDB(t)

	thread, err := CreateOpsAssistantThread(42, "Long thread", "en")
	require.NoError(t, err)

	for i := 0; i < 25; i++ {
		msg := &OpsAssistantMessage{
			ThreadID:   thread.ID,
			BusinessID: 42,
			Role:       OpsAssistantRoleUser,
			Locale:     "en",
			Content:    fmt.Sprintf("msg-%02d", i),
		}
		require.NoError(t, SaveOpsAssistantMessage(msg))
	}

	got, err := ListOpsAssistantMessages(42, thread.ID, 20)
	require.NoError(t, err)
	require.Len(t, got, 20)
	// Should be the LAST 20 (msg-05 .. msg-24), oldest-first.
	require.Equal(t, "msg-05", got[0].Content, "oldest of the recent window first")
	require.Equal(t, "msg-24", got[19].Content, "newest message last")
	for i := 1; i < len(got); i++ {
		require.Less(t, got[i-1].ID, got[i].ID, "messages must be chronological (ascending id)")
	}
}

// Prod regression: structured_response and arguments are Postgres jsonb
// columns. The empty string is NOT valid json there (SQLite hides this), so
// the save helpers must normalize "" (and "null" for tool-call args) to "{}".
func TestSaveOpsAssistantMessageNormalizesEmptyJSONB(t *testing.T) {
	setupAgentPlatformTestDB(t)
	thread, err := CreateOpsAssistantThread(1, "help", "en")
	require.NoError(t, err)

	msg := &OpsAssistantMessage{
		ThreadID: thread.ID, BusinessID: 1,
		Role: OpsAssistantRoleUser, Locale: "en", Content: "hi",
	}
	require.NoError(t, SaveOpsAssistantMessage(msg))
	require.Equal(t, "{}", msg.StructuredResponse)

	var stored OpsAssistantMessage
	require.NoError(t, db.First(&stored, msg.ID).Error)
	require.Equal(t, "{}", stored.StructuredResponse)
}

func TestSaveOpsAssistantToolCallNormalizesEmptyJSONB(t *testing.T) {
	setupAgentPlatformTestDB(t)
	thread, err := CreateOpsAssistantThread(1, "help", "en")
	require.NoError(t, err)

	for _, raw := range []string{"", "null", "  "} {
		rec := &OpsAssistantToolCall{
			ThreadID: thread.ID, BusinessID: 1,
			ToolName: "get_pricing_overview", ArgumentsJSON: raw, Status: "completed",
		}
		require.NoError(t, SaveOpsAssistantToolCall(rec))
		require.Equal(t, "{}", rec.ArgumentsJSON, "raw=%q", raw)
	}
}
