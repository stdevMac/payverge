package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A paused (taken-over) conversation makes no model call, but before this
// regression every guest turn was still persisted and could raise a takeover
// alert with no per-session cap and no per-client daily ceiling, so one
// script could grow ai_waiter_messages and alert events without limit.

func seedAIWaiterMessages(t *testing.T, db *gorm.DB, convID uint, n int) {
	t.Helper()
	roles := []string{"user", "assistant", "staff"}
	rows := make([]database.AiWaiterMessage, n)
	for i := range rows {
		rows[i] = database.AiWaiterMessage{
			ConversationID:     convID,
			Role:               roles[i%len(roles)],
			Content:            fmt.Sprintf("seed-%d", i),
			StructuredResponse: "{}",
		}
	}
	require.NoError(t, db.CreateInBatches(rows, 50).Error)
}

func countGuestTurn(t *testing.T, db *gorm.DB, convID uint, content string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&database.AiWaiterMessage{}).
		Where("conversation_id = ? AND role = ? AND content = ?", convID, "user", content).
		Count(&n).Error)
	return n
}

func pausedGuestTurn(t *testing.T, convSession, tableCode, content string) map[string]any {
	t.Helper()
	return map[string]any{
		"session_token": convSession, "mode": "ordering", "table_code": tableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": content}},
	}
}

func TestHandleAIWaiter_PausedSessionCapRefusesBeforePersistOrAlert(t *testing.T) {
	guestAIQuota.Reset()
	t.Cleanup(guestAIQuota.Reset)
	db, business, conv, table := setupAITakeoverAlertTest(t)
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
		Update("is_paused", true).Error)
	router := guestAIWaiterRouter(t)
	path := fmt.Sprintf("/ai-waiter/%d", business.ID)

	// A full AI session (60 rows) can still reach the human who took it over.
	seedAIWaiterMessages(t, db, conv.ID, maxAIWaiterMessagesPerSession)
	w := performAIWaiterRequest(t, router, http.MethodPost, path,
		pausedGuestTurn(t, conv.SessionID, table.TableCode, "still there?"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.EqualValues(t, 1, countGuestTurn(t, db, conv.ID, "still there?"))
	require.Len(t, openTakeoverAlerts(t, db, business.ID, conv.ID), 1)

	// Fill the paused cap: the next turn is refused before it is persisted
	// and before the takeover alert is touched.
	require.NoError(t, db.Where("business_id = ?", business.ID).Delete(&database.OperationalAlert{}).Error)
	seedAIWaiterMessages(t, db, conv.ID, maxPausedAIWaiterMessagesPerSession-maxAIWaiterMessagesPerSession-2)
	require.EqualValues(t, maxPausedAIWaiterMessagesPerSession-1, database.CountAiWaiterMessages(conv.ID))

	w = performAIWaiterRequest(t, router, http.MethodPost, path,
		pausedGuestTurn(t, conv.SessionID, table.TableCode, "spam-over-cap"))
	requireOverBudget(t, w)
	require.Zero(t, countGuestTurn(t, db, conv.ID, "spam-over-cap"), "refused paused turn must not be persisted")
	require.Empty(t, openTakeoverAlerts(t, db, business.ID, conv.ID), "refused paused turn must not raise a takeover alert")
	require.EqualValues(t, maxPausedAIWaiterMessagesPerSession-1, database.CountAiWaiterMessages(conv.ID))
}

func TestHandleAIWaiter_PausedTurnsDrawFromPerClientDailyQuota(t *testing.T) {
	guestAIQuota.Reset()
	t.Cleanup(guestAIQuota.Reset)
	t.Setenv(envAIWaiterDailyPerIP, "2")
	db, business, conv, table := setupAITakeoverAlertTest(t)
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
		Update("is_paused", true).Error)
	router := guestAIWaiterRouter(t)
	path := fmt.Sprintf("/ai-waiter/%d", business.ID)

	for i := 0; i < 2; i++ {
		content := fmt.Sprintf("hello-%d", i)
		w := performAIWaiterRequest(t, router, http.MethodPost, path,
			pausedGuestTurn(t, conv.SessionID, table.TableCode, content))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.EqualValues(t, 1, countGuestTurn(t, db, conv.ID, content))
	}

	w := performAIWaiterRequest(t, router, http.MethodPost, path,
		pausedGuestTurn(t, conv.SessionID, table.TableCode, "hello-over-quota"))
	requireOverBudget(t, w)
	require.Zero(t, countGuestTurn(t, db, conv.ID, "hello-over-quota"),
		"a paused turn past the per-network daily ceiling must not be persisted")
}
