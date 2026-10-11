package server

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

// BenchmarkHandleAIWaiterPausedTurn measures one guest turn into a conversation
// a staff member has taken over and still holds (no model call, no takeover
// alert): the path that now also counts the session rows and takes the
// per-client daily quota before persisting.
func BenchmarkHandleAIWaiterPausedTurn(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDBWithLogger(b, logger.Default.LogMode(logger.Silent))
	business, tableCode := seedAIWaiterChatBusiness(b, db, "bench-paused", "bench-paused-token")
	require.NoError(b, db.Model(&database.AiWaiterConversation{}).
		Where("business_id = ? AND session_id = ?", business.ID, "bench-paused-token").
		Updates(map[string]any{"is_paused": true, "claimed_by_name": "Owner", "claimed_by_role": "owner", "claimed_at": time.Now()}).Error)
	// Every httptest request shares one RemoteAddr; keep the per-network
	// ceiling out of the way (the quota check itself still runs).
	b.Setenv(envAIWaiterDailyPerIP, "100000000")
	b.Cleanup(guestAIQuota.Reset)

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	path := fmt.Sprintf("/ai-waiter/%d", business.ID)
	body := map[string]any{
		"session_token": "bench-paused-token", "mode": "ordering", "table_code": tableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "Is my table's order coming soon?"}},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%(maxAIWaiterMessagesPerSession/2) == 0 {
			b.StopTimer()
			require.NoError(b, db.Exec("DELETE FROM ai_waiter_messages").Error)
			b.StartTimer()
		}
		w := performAIWaiterRequest(b, router, http.MethodPost, path, body)
		if w.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}
