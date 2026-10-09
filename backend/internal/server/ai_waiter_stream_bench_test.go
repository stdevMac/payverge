package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormlogger "gorm.io/gorm/logger"
)

type countingLogger struct {
	gormlogger.Interface
	count *int64
}

func (l countingLogger) Trace(ctx context.Context, begin time.Time,
	fc func() (string, int64), err error) {
	atomic.AddInt64(l.count, 1)
}

func TestAIWaiterStream_NoPerTickDBPolling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var queries int64
	setupAIWaiterTestDBWithLogger(t, countingLogger{
		Interface: gormlogger.Default.LogMode(gormlogger.Silent),
		count:     &queries,
	})
	business := createAIWaiterBusiness(t, "noplly", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-noplly")

	prev := aiWaiterStreamKeepalive
	aiWaiterStreamKeepalive = 10 * time.Millisecond
	t.Cleanup(func() { aiWaiterStreamKeepalive = prev })

	atomic.StoreInt64(&queries, 0)

	req := httptest.NewRequest(http.MethodGet,
		"/ai-waiter/"+strconv.Itoa(int(business.ID))+"/stream", nil)
	req.Header.Set("Authorization", "Bearer "+conv.SessionID)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "businessId", Value: strconv.Itoa(int(business.ID))}}
	c.Request = req

	HandleAIWaiterStream(c)

	total := atomic.LoadInt64(&queries)
	require.Equal(t, http.StatusOK, w.Code)
	assert.LessOrEqual(t, total, int64(8),
		"stream must validate once and then poll the hub, not the DB (got %d queries)", total)
}

func BenchmarkAiWaiterMessagesPoll(b *testing.B) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(b)
	business := createAIWaiterBusiness(b, "bench-poll", true)
	conv := createAIWaiterConversation(b, business.ID, "tok-bench")
	for i := 0; i < 20; i++ {
		require.NoError(b, database.SaveAiWaiterMessage(conv.ID, "assistant", "reply", ""))
	}
	since := time.Now().Add(-time.Hour)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := database.GetAiWaiterMessagesForGuest(conv.ID, &since, 100)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAiWaiterStreamPublish(b *testing.B) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(b)
	business := createAIWaiterBusiness(b, "bench-pub", true)
	conv := createAIWaiterConversation(b, business.ID, "tok-bench-pub")
	require.NoError(b, database.SaveAiWaiterMessage(conv.ID, "assistant", "reply", ""))
	pubConv := &database.AiWaiterConversation{ID: conv.ID, SessionID: conv.SessionID}
	now := time.Now()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		publishAiWaiterMessage(pubConv, 1, "assistant", "reply", now)
	}
}
