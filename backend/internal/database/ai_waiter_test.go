package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAiWaiterTestDB(t testing.TB) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	err = gormDB.AutoMigrate(&AiWaiterConversation{}, &AiWaiterMessage{})
	require.NoError(t, err)

	db = gormDB
}

func TestGetRecentAiWaiterMessages_ReturnsChronologicalOrder(t *testing.T) {
	setupAiWaiterTestDB(t)

	conv := AiWaiterConversation{
		SessionID:  "whatsapp:test123",
		BusinessID: 1,
		TableCode:  "WhatsApp",
		Language:   "Auto",
		Mode:       "ordering",
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, db.Create(&conv).Error)

	// Create messages with different timestamps
	for i := 0; i < 5; i++ {
		msg := AiWaiterMessage{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        fmt.Sprintf("Message %d", i),
			CreatedAt:      time.Now().Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, db.Create(&msg).Error)
	}

	messages, err := GetRecentAiWaiterMessages(conv.ID, 10)
	require.NoError(t, err)
	assert.Len(t, messages, 5)

	// Verify chronological order (oldest first)
	assert.Equal(t, "Message 0", messages[0].Content)
	assert.Equal(t, "Message 4", messages[4].Content)
}

func TestGetRecentAiWaiterMessages_RespectsLimit(t *testing.T) {
	setupAiWaiterTestDB(t)

	conv := AiWaiterConversation{
		SessionID:  "whatsapp:limit_test",
		BusinessID: 1,
		TableCode:  "WhatsApp",
		Language:   "Auto",
		Mode:       "ordering",
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, db.Create(&conv).Error)

	// Create 10 messages
	for i := 0; i < 10; i++ {
		msg := AiWaiterMessage{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        fmt.Sprintf("Message %d", i),
			CreatedAt:      time.Now().Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, db.Create(&msg).Error)
	}

	// Request only 3 most recent
	messages, err := GetRecentAiWaiterMessages(conv.ID, 3)
	require.NoError(t, err)
	assert.Len(t, messages, 3)

	// Should return the 3 most recent in chronological order
	assert.Equal(t, "Message 7", messages[0].Content)
	assert.Equal(t, "Message 8", messages[1].Content)
	assert.Equal(t, "Message 9", messages[2].Content)
}

func TestCloseAiWaiterConversationBySession_ClosesMatchingBusinessRow(t *testing.T) {
	setupAiWaiterTestDB(t)
	conv := AiWaiterConversation{
		SessionID:  "close-me",
		BusinessID: 3,
		TableCode:  "T1",
		Language:   "en",
		Mode:       "ordering",
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, db.Create(&conv).Error)

	closed, err := CloseAiWaiterConversationBySession("close-me", 3, "Guest started a new conversation in Español (Argentina).")
	require.NoError(t, err)
	require.True(t, closed)

	var reloaded AiWaiterConversation
	require.NoError(t, db.First(&reloaded, conv.ID).Error)
	assert.Equal(t, "closed", reloaded.Status)

	messages, err := GetAiWaiterMessagesForTranscript(conv.ID, nil, 10)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "system", messages[0].Role)
	assert.Contains(t, messages[0].Content, "Español (Argentina)")
}

func TestUpdateAiWaiterConversationLanguage_WritesLocale(t *testing.T) {
	setupAiWaiterTestDB(t)
	conv := AiWaiterConversation{
		SessionID:  "lang-me",
		BusinessID: 4,
		TableCode:  "T1",
		Language:   "en",
		Mode:       "ordering",
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, db.Create(&conv).Error)
	require.NoError(t, UpdateAiWaiterConversationLanguage(conv.ID, "es-AR"))

	var reloaded AiWaiterConversation
	require.NoError(t, db.First(&reloaded, conv.ID).Error)
	assert.Equal(t, "es-AR", reloaded.Language)
}

func TestGetRecentAiWaiterMessagesSkipsToolCalls(t *testing.T) {
	setupAiWaiterTestDB(t)

	conv := AiWaiterConversation{
		SessionID:  "skip-blobs",
		BusinessID: 1,
		TableCode:  "T1",
		Language:   "en",
		Mode:       "ordering",
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, db.Create(&conv).Error)

	toolCalls := `{"calls":["` + strings.Repeat("t", 8*1024) + `"]}`
	structured := `{"answer":"` + strings.Repeat("s", 4*1024) + `"}`
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	for i := 0; i < 4; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msg := AiWaiterMessage{
			ConversationID:     conv.ID,
			Role:               role,
			Content:            fmt.Sprintf("Message %d", i),
			ToolCalls:          toolCalls,
			StructuredResponse: structured,
			CreatedAt:          base.Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, db.Create(&msg).Error)
	}

	var sqls []string
	const cb = "ai_waiter_recent_skip_blobs"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(cb, func(tx *gorm.DB) {
		sqls = append(sqls, tx.Statement.SQL.String())
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(cb) })

	messages, err := GetRecentAiWaiterMessages(conv.ID, 4)
	require.NoError(t, err)
	require.Len(t, messages, 4)
	require.NotEmpty(t, sqls)

	for _, sql := range sqls {
		lower := strings.ToLower(sql)
		assert.NotContains(t, lower, "tool_calls")
		assert.NotContains(t, lower, "structured_response")
		assert.NotContains(t, lower, "*")
	}
	assert.Equal(t, "Message 0", messages[0].Content)
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, "Message 3", messages[3].Content)
	assert.Equal(t, "assistant", messages[3].Role)
	for i, msg := range messages {
		assert.Empty(t, msg.ToolCalls, "row %d ToolCalls must not be loaded", i)
		assert.Empty(t, msg.StructuredResponse, "row %d StructuredResponse must not be loaded", i)
		assert.NotZero(t, msg.ID)
		assert.False(t, msg.CreatedAt.IsZero())
	}
}

func BenchmarkGetRecentAiWaiterMessages(b *testing.B) {
	setupAiWaiterTestDB(b)

	conv := AiWaiterConversation{
		SessionID:  fmt.Sprintf("bench-recent-%d", time.Now().UnixNano()),
		BusinessID: 1,
		TableCode:  "T1",
		Language:   "en",
		Mode:       "ordering",
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := db.Create(&conv).Error; err != nil {
		b.Fatal(err)
	}
	toolCalls := `{"calls":["` + strings.Repeat("t", 8*1024) + `"]}`
	structured := `{"answer":"` + strings.Repeat("s", 4*1024) + `"}`
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	msgs := make([]AiWaiterMessage, 40)
	for i := range msgs {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = AiWaiterMessage{
			ConversationID:     conv.ID,
			Role:               role,
			Content:            fmt.Sprintf("Message %d", i),
			ToolCalls:          toolCalls,
			StructuredResponse: structured,
			CreatedAt:          base.Add(time.Duration(i) * time.Second),
		}
	}
	if err := db.Create(&msgs).Error; err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := GetRecentAiWaiterMessages(conv.ID, 20)
		if err != nil {
			b.Fatal(err)
		}
		if len(got) != 20 {
			b.Fatalf("len %d", len(got))
		}
	}
}

func TestGetRecentAiWaiterMessages_EmptyConversation(t *testing.T) {
	setupAiWaiterTestDB(t)

	messages, err := GetRecentAiWaiterMessages(9999, 10)
	require.NoError(t, err)
	assert.Empty(t, messages)
}
