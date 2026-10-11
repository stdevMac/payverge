package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWizardTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&MenuWizardSession{}, &MenuWizardMessage{}))
	SetTestDB(gormDB)
}

// TestGetRecentWizardMessages_BoundsAndOrder seeds 1 system message + 40
// alternating user/assistant messages and asserts that GetRecentWizardMessages
// returns the system prompt plus the 10 most-recent non-system messages in
// chronological (oldest-first) order.
func TestGetRecentWizardMessages_BoundsAndOrder(t *testing.T) {
	setupWizardTestDB(t)

	sess := MenuWizardSession{
		BusinessID: 1,
		Status:     WizardStatusInProgress,
	}
	require.NoError(t, db.Create(&sess).Error)

	base := time.Now().UTC().Truncate(time.Second)

	// Seed 1 system message first.
	require.NoError(t, db.Create(&MenuWizardMessage{
		SessionID: sess.ID,
		Role:      "system",
		Content:   "You are a menu wizard.",
		CreatedAt: base,
	}).Error)

	// Seed 40 alternating user/assistant messages, each 1 second apart.
	roles := []string{"user", "assistant"}
	for i := 0; i < 40; i++ {
		require.NoError(t, db.Create(&MenuWizardMessage{
			SessionID: sess.ID,
			Role:      roles[i%2],
			Content:   fmt.Sprintf("msg-%02d", i),
			CreatedAt: base.Add(time.Duration(i+1) * time.Second),
		}).Error)
	}

	msgs, err := GetRecentWizardMessages(sess.ID, 10)
	require.NoError(t, err)

	// Must be system + exactly 10 non-system messages (≤11 total).
	if len(msgs) > 11 {
		t.Fatalf("expected <=11 messages, got %d", len(msgs))
	}

	// First message must be the system prompt.
	if msgs[0].Role != "system" {
		t.Fatalf("system prompt must be first, got %q", msgs[0].Role)
	}

	// The 10 non-system messages must be the most-recent 10 in chronological order.
	nonSystem := msgs[1:]
	require.Len(t, nonSystem, 10, "must return exactly 10 non-system messages")

	// The most-recent 10 non-system msgs are msg-30..msg-39. They must be ascending.
	require.Equal(t, "msg-30", nonSystem[0].Content, "first non-system should be msg-30 (oldest of last 10)")
	require.Equal(t, "msg-39", nonSystem[9].Content, "last non-system should be msg-39 (newest)")
	for i := 1; i < len(nonSystem); i++ {
		if !nonSystem[i].CreatedAt.After(nonSystem[i-1].CreatedAt) {
			t.Fatalf("messages out of chronological order at index %d: %v >= %v",
				i, nonSystem[i-1].CreatedAt, nonSystem[i].CreatedAt)
		}
	}
}

// TestGetRecentWizardMessages_DefaultLimit verifies that limit<=0 defaults to 20.
func TestGetRecentWizardMessages_DefaultLimit(t *testing.T) {
	setupWizardTestDB(t)

	sess := MenuWizardSession{BusinessID: 1, Status: WizardStatusInProgress}
	require.NoError(t, db.Create(&sess).Error)

	base := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 30; i++ {
		require.NoError(t, db.Create(&MenuWizardMessage{
			SessionID: sess.ID,
			Role:      "user",
			Content:   fmt.Sprintf("u%02d", i),
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		}).Error)
	}

	msgs, err := GetRecentWizardMessages(sess.ID, 0)
	require.NoError(t, err)
	// default=20, no system message → exactly 20 rows.
	require.Len(t, msgs, 20, "limit 0 should default to 20")
}

// TestGetRecentWizardMessages_EmptySession verifies empty result for unknown session.
func TestGetRecentWizardMessages_EmptySession(t *testing.T) {
	setupWizardTestDB(t)

	msgs, err := GetRecentWizardMessages(99999, 10)
	require.NoError(t, err)
	require.Empty(t, msgs)
}
