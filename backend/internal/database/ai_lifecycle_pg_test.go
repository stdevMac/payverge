package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// PostgreSQL integration: FK-safe permanent delete and archived retention.
// Requires TEST_DATABASE_URL (e.g. postgres://test:test@localhost:5432/test?sslmode=disable).

func openPG(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)

	// Clean slate for focused AI tables (no full business graph).
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS ops_assistant_tool_calls CASCADE`,
		`DROP TABLE IF EXISTS ops_assistant_messages CASCADE`,
		`DROP TABLE IF EXISTS ops_assistant_requests CASCADE`,
		`DROP TABLE IF EXISTS ops_assistant_threads CASCADE`,
		`DROP TABLE IF EXISTS director_action_audits CASCADE`,
		`DROP TABLE IF EXISTS director_proposed_actions CASCADE`,
		`DROP TABLE IF EXISTS director_tool_calls CASCADE`,
		`DROP TABLE IF EXISTS director_console_messages CASCADE`,
		`DROP TABLE IF EXISTS director_console_threads CASCADE`,
		`DROP TABLE IF EXISTS ai_waiter_messages CASCADE`,
		`DROP TABLE IF EXISTS ai_waiter_conversations CASCADE`,
		`DROP TABLE IF EXISTS ai_generated_images CASCADE`,
		`DROP TABLE IF EXISTS menu_wizard_messages CASCADE`,
		`DROP TABLE IF EXISTS menu_wizard_sessions CASCADE`,
		`DROP TABLE IF EXISTS menu_extraction_images CASCADE`,
		`DROP TABLE IF EXISTS menu_extraction_jobs CASCADE`,
	} {
		_ = gdb.Exec(stmt).Error
	}

	require.NoError(t, gdb.AutoMigrate(
		&OpsAssistantThread{},
		&OpsAssistantMessage{},
		&OpsAssistantToolCall{},
		&OpsAssistantRequest{},
		&DirectorConsoleThread{},
		&DirectorConsoleMessage{},
		&DirectorToolCall{},
		&DirectorProposedAction{},
		&DirectorActionAudit{},
		&AiWaiterConversation{},
		&AiWaiterMessage{},
	))
	// Drop any residual FKs GORM still attached.
	for _, stmt := range []string{
		`ALTER TABLE IF EXISTS ai_waiter_conversations DROP CONSTRAINT IF EXISTS fk_ai_waiter_conversations_business`,
		`ALTER TABLE IF EXISTS ops_assistant_threads DROP CONSTRAINT IF EXISTS fk_ops_assistant_threads_business`,
		`ALTER TABLE IF EXISTS director_console_threads DROP CONSTRAINT IF EXISTS fk_director_console_threads_business`,
	} {
		_ = gdb.Exec(stmt).Error
	}
	SetTestDB(gdb)
	return gdb
}

func openHermeticAiWaiterLifecycleDB(t *testing.T) (*gorm.DB, AiWaiterConversation) {
	t.Helper()
	dsn := fmt.Sprintf("file:ai-waiter-structured-%s?mode=memory&cache=shared&_foreign_keys=1", t.Name())
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, gdb.AutoMigrate(&AiWaiterConversation{}, &AiWaiterMessage{}))
	SetTestDB(gdb)
	conversation := AiWaiterConversation{
		SessionID: fmt.Sprintf("structured-%d", time.Now().UnixNano()), BusinessID: 7001,
		TableCode: "T1", Language: "es", Mode: "ordering", Status: "active",
	}
	require.NoError(t, gdb.Create(&conversation).Error)
	return gdb, conversation
}

func TestAiWaiterStructuredResponseGenesisColumn(t *testing.T) {
	require.Contains(t, genesisTableBody(t, "ai_waiter_messages"),
		"structured_response jsonb DEFAULT '{}'::jsonb NOT NULL")
}

func TestAiWaiterStructuredResponsePersistence(t *testing.T) {
	t.Run("valid JSON round trips unchanged", func(t *testing.T) {
		gdb, conversation := openHermeticAiWaiterLifecycleDB(t)
		structured := "{\n  \"version\": 2, \"answer\": {\"content\": \"Hola\"}\n}"
		id, createdAt, err := SaveAiWaiterMessageReturningIDV2(conversation.ID, "assistant", "Hola", "", structured)
		require.NoError(t, err)
		require.NotZero(t, id)
		require.False(t, createdAt.IsZero())
		var stored AiWaiterMessage
		require.NoError(t, gdb.First(&stored, id).Error)
		require.Equal(t, structured, stored.StructuredResponse)
		require.True(t, json.Valid([]byte(stored.StructuredResponse)))
	})

	t.Run("content and structured string values are redacted as valid JSON", func(t *testing.T) {
		gdb, conversation := openHermeticAiWaiterLifecycleDB(t)
		structured := `{"version":2,"answer":{"content":"Email guest@example.com or call +5491155551234"},"count":5491155551234,"entity_id":9007199254740993}`
		id, _, err := SaveAiWaiterMessageReturningIDV2(
			conversation.ID, "assistant", "Email guest@example.com or call +5491155551234", "", structured,
		)
		require.NoError(t, err)
		var stored AiWaiterMessage
		require.NoError(t, gdb.First(&stored, id).Error)
		require.NotContains(t, stored.Content, "guest@example.com")
		require.NotContains(t, stored.Content, "5491155551234")
		require.True(t, json.Valid([]byte(stored.StructuredResponse)))
		require.Contains(t, stored.StructuredResponse, "[redacted-email]")
		require.Contains(t, stored.StructuredResponse, "[redacted-phone]")
		require.Contains(t, stored.StructuredResponse, "5491155551234", "numeric JSON facts must not be corrupted by text redaction")
		require.Contains(t, stored.StructuredResponse, "9007199254740993", "large integer identities must round trip exactly")
	})

	t.Run("structured keys and phone fields are redacted recursively", func(t *testing.T) {
		gdb, conversation := openHermeticAiWaiterLifecycleDB(t)
		structured := `{"guest@example.com":"value","phone +5491155551234":"value","phone":"5491155555678","mobile":5491155552468,"nested":{"backup@example.com":"value","telefono":"5491155559876","tel":5491155551357}}`
		id, _, err := SaveAiWaiterMessageReturningIDV2(conversation.ID, "assistant", "safe", "", structured)
		require.NoError(t, err)
		var stored AiWaiterMessage
		require.NoError(t, gdb.First(&stored, id).Error)
		require.True(t, json.Valid([]byte(stored.StructuredResponse)))
		for _, leaked := range []string{
			"guest@example.com", "5491155551234", "5491155555678", "5491155552468",
			"backup@example.com", "5491155559876", "5491155551357",
		} {
			require.NotContains(t, stored.StructuredResponse, leaked)
		}
		require.Contains(t, stored.StructuredResponse, "[redacted-email]")
		require.Contains(t, stored.StructuredResponse, "[redacted-phone]")
	})

	t.Run("structured phone field variants and tagged values redact without corrupting identities", func(t *testing.T) {
		gdb, conversation := openHermeticAiWaiterLifecycleDB(t)
		structured := `{
			"phoneNumber":"5491155550101",
			"mobileNumber":"5491155550102",
			"whatsappNumber":"5491155550103",
			"numericVariants":{"phoneNumber":5491155550104,"mobileNumber":5491155550105,"whatsappNumber":5491155550106},
			"PHONE_NUMBER":"5491155550107",
			"mobile-number":5491155550108,
			"WhatsApp Number":"5491155550109",
			"nested":{"contactPhoneNumber":"5491155550110","support_mobile_number":5491155550113,"SMSPhoneNumber":"5491155550114"},
			"contacts":[
				{"type":"phone","value":"5491155550115"},
				{"kind":"telefono","value":5491155550116},
				{"type":"menu_item","value":5491155550111}
			],
			"orderId":9007199254740993,
			"entity_id":5491155550112
		}`
		id, _, err := SaveAiWaiterMessageReturningIDV2(conversation.ID, "assistant", "safe", "", structured)
		require.NoError(t, err)
		var stored AiWaiterMessage
		require.NoError(t, gdb.First(&stored, id).Error)
		require.True(t, json.Valid([]byte(stored.StructuredResponse)))

		for _, leakedPhone := range []string{
			"5491155550101", "5491155550102", "5491155550103", "5491155550104",
			"5491155550105", "5491155550106", "5491155550107", "5491155550108",
			"5491155550109", "5491155550110", "5491155550113", "5491155550114",
			"5491155550115", "5491155550116",
		} {
			require.NotContains(t, stored.StructuredResponse, leakedPhone)
		}
		require.Contains(t, stored.StructuredResponse, "5491155550111", "non-phone tagged numeric values must remain exact")
		require.Contains(t, stored.StructuredResponse, "9007199254740993", "large order identities must remain exact")
		require.Contains(t, stored.StructuredResponse, "5491155550112", "arbitrary long entity IDs must not be treated as phones")

		decoder := json.NewDecoder(strings.NewReader(stored.StructuredResponse))
		decoder.UseNumber()
		var decoded map[string]any
		require.NoError(t, decoder.Decode(&decoded))
		for _, originalKey := range []string{
			"phoneNumber", "mobileNumber", "whatsappNumber", "PHONE_NUMBER",
			"numericVariants", "mobile-number", "WhatsApp Number", "nested", "contacts", "orderId", "entity_id",
		} {
			require.Contains(t, decoded, originalKey, "non-PII JSON keys must be preserved")
		}
	})

	t.Run("redacted key collisions reject before insert", func(t *testing.T) {
		gdb, conversation := openHermeticAiWaiterLifecycleDB(t)
		_, _, err := SaveAiWaiterMessageReturningIDV2(
			conversation.ID, "assistant", "safe", "", `{"first@example.com":"one","second@example.com":"two"}`,
		)
		require.ErrorContains(t, err, "redacted structured response key collision")
		var count int64
		require.NoError(t, gdb.Model(&AiWaiterMessage{}).Count(&count).Error)
		require.Zero(t, count)
	})

	t.Run("legacy and blank structured responses store empty object", func(t *testing.T) {
		gdb, conversation := openHermeticAiWaiterLifecycleDB(t)
		_, _, err := SaveAiWaiterMessageReturningID(conversation.ID, "assistant", "legacy", "")
		require.NoError(t, err)
		for _, structured := range []string{"", " \n\t", "null"} {
			_, _, err := SaveAiWaiterMessageReturningIDV2(conversation.ID, "assistant", "blank", "", structured)
			require.NoError(t, err)
		}
		var stored []AiWaiterMessage
		require.NoError(t, gdb.Order("id ASC").Find(&stored).Error)
		require.Len(t, stored, 4)
		for _, message := range stored {
			require.Equal(t, "{}", message.StructuredResponse)
		}
	})

	t.Run("invalid JSON rejects before insert", func(t *testing.T) {
		gdb, conversation := openHermeticAiWaiterLifecycleDB(t)
		_, _, err := SaveAiWaiterMessageReturningIDV2(conversation.ID, "assistant", "invalid", "", `{"version":2`)
		require.ErrorContains(t, err, "invalid structured response")
		var count int64
		require.NoError(t, gdb.Model(&AiWaiterMessage{}).Count(&count).Error)
		require.Zero(t, count)
	})

	t.Run("conversation update failure rolls back message insert", func(t *testing.T) {
		gdb, conversation := openHermeticAiWaiterLifecycleDB(t)
		updateErr := errors.New("forced conversation update failure")
		require.NoError(t, gdb.Callback().Update().Before("gorm:update").Register("test:ai_waiter_update_failure", func(tx *gorm.DB) {
			if tx.Statement.Table == "ai_waiter_conversations" {
				tx.AddError(updateErr)
			}
		}))
		_, _, err := SaveAiWaiterMessageReturningIDV2(conversation.ID, "assistant", "rollback", "", `{}`)
		require.ErrorIs(t, err, updateErr)
		var count int64
		require.NoError(t, gdb.Model(&AiWaiterMessage{}).Count(&count).Error)
		require.Zero(t, count)
	})

	t.Run("zero conversation and nil database reject", func(t *testing.T) {
		_, _ = openHermeticAiWaiterLifecycleDB(t)
		_, _, err := SaveAiWaiterMessageReturningIDV2(0, "assistant", "invalid", "", `{}`)
		require.ErrorContains(t, err, "conversation id")
		previousDB := db
		db = nil
		t.Cleanup(func() { db = previousDB })
		_, _, err = SaveAiWaiterMessageReturningIDV2(1, "assistant", "invalid", "", `{}`)
		require.ErrorIs(t, err, gorm.ErrInvalidDB)
	})
}

func TestIntegration_AiWaiterStructuredResponseJSONBDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres test in -short (repo container-test convention)")
	}
	gdb := startGenesisPostgres(t).DB
	businessID := seedGenesisBusiness(t, gdb, "ai-structured-default")

	var dataType, nullable, defaultValue string
	require.NoError(t, gdb.Raw(`
		SELECT data_type, is_nullable, COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'ai_waiter_messages'
		  AND column_name = 'structured_response'
	`).Row().Scan(&dataType, &nullable, &defaultValue))
	require.Equal(t, "jsonb", dataType)
	require.Equal(t, "NO", nullable)
	require.Contains(t, defaultValue, "{}")

	conversation := AiWaiterConversation{
		SessionID: fmt.Sprintf("pg-structured-%d", time.Now().UnixNano()), BusinessID: businessID,
		TableCode: "T1", Language: "en", Mode: "ordering", Status: "active",
	}
	require.NoError(t, gdb.Create(&conversation).Error)
	require.NoError(t, gdb.Exec(`
		INSERT INTO ai_waiter_messages (conversation_id, role, content, tool_calls, created_at)
		VALUES (?, 'assistant', 'raw insert', '', NOW())
	`, conversation.ID).Error)
	var structured string
	require.NoError(t, gdb.Raw(`
		SELECT structured_response::text
		FROM ai_waiter_messages
		WHERE conversation_id = ? AND content = 'raw insert'
	`, conversation.ID).Scan(&structured).Error)
	require.Equal(t, "{}", strings.TrimSpace(structured))
}

func TestIntegration_InactiveOpsRetention_FKOrder(t *testing.T) {
	gdb := openPG(t)
	biz := uint(9001)
	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	thread := &OpsAssistantThread{BusinessID: biz, Title: "pg-ops", Locale: "en", LastMessageAt: old}
	require.NoError(t, gdb.Create(thread).Error)
	require.NoError(t, gdb.Create(&OpsAssistantMessage{
		ThreadID:           thread.ID,
		BusinessID:         biz,
		Role:               OpsAssistantRoleUser,
		Content:            "hi",
		StructuredResponse: "{}",
	}).Error)
	require.NoError(t, gdb.Create(&OpsAssistantToolCall{
		ThreadID: thread.ID, BusinessID: biz, ToolName: "search_guides", Status: "ok",
		ArgumentsJSON: "{}",
	}).Error)
	fresh := &OpsAssistantThread{BusinessID: biz, Title: "pg-ops-fresh", Locale: "en", LastMessageAt: time.Now().UTC()}
	require.NoError(t, gdb.Create(fresh).Error)

	res, err := DeleteInactiveOpsAssistantThreads(time.Now().UTC().Add(-30*24*time.Hour), 50)
	require.NoError(t, err)
	require.GreaterOrEqual(t, res.ThreadsDeleted, int64(1))
	var n int64
	require.NoError(t, gdb.Model(&OpsAssistantThread{}).Where("id = ?", thread.ID).Count(&n).Error)
	require.Equal(t, int64(0), n)
	require.NoError(t, gdb.Model(&OpsAssistantMessage{}).Where("thread_id = ?", thread.ID).Count(&n).Error)
	require.Equal(t, int64(0), n)
	require.NoError(t, gdb.Model(&OpsAssistantToolCall{}).Where("thread_id = ?", thread.ID).Count(&n).Error)
	require.Equal(t, int64(0), n)
	require.NoError(t, gdb.Model(&OpsAssistantThread{}).Where("id = ?", fresh.ID).Count(&n).Error)
	require.Equal(t, int64(1), n)
}

func TestIntegration_PurgeBusinessAIData_TenantIsolation(t *testing.T) {
	gdb := openPG(t)
	b1 := uint(9101)
	b2 := uint(9102)

	c1 := AiWaiterConversation{SessionID: fmt.Sprintf("pg-s1-%d", time.Now().UnixNano()), BusinessID: b1, TableCode: "T1", Mode: "ordering", Status: "active"}
	c2 := AiWaiterConversation{SessionID: fmt.Sprintf("pg-s2-%d", time.Now().UnixNano()), BusinessID: b2, TableCode: "T2", Mode: "ordering", Status: "active"}
	require.NoError(t, gdb.Create(&c1).Error)
	require.NoError(t, gdb.Create(&c2).Error)
	require.NoError(t, gdb.Create(&AiWaiterMessage{ConversationID: c1.ID, Role: "user", Content: "a"}).Error)
	require.NoError(t, gdb.Create(&AiWaiterMessage{ConversationID: c2.ID, Role: "user", Content: "b"}).Error)

	res, err := PurgeBusinessAIData(b1)
	require.NoError(t, err)
	require.Equal(t, int64(1), res.WaiterConversations)
	var remaining int64
	require.NoError(t, gdb.Model(&AiWaiterConversation{}).Where("business_id = ?", b2).Count(&remaining).Error)
	require.Equal(t, int64(1), remaining)
}
