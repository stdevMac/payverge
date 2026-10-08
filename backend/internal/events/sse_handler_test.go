package events

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupSSEHandlerTestDB(t *testing.T) {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}))
}

func createSSETestBusiness(t *testing.T) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:     "biz-sse-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Name:           "SSE Test Business",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		TippingAddr:    "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
	}

	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func TestSSEHandler_AllowsStaffContextWithUintBusinessID(t *testing.T) {
	setupSSEHandlerTestDB(t)
	business := createSSETestBusiness(t)

	req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(business.ID))+"/events", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(business.ID))}}
	c.Request = req
	c.Set("token_type", "staff")
	c.Set("staff_business_id", business.ID)

	SSEHandler(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "event: connected")
}

func TestWriteBusinessSSEEmitsIDNamedEventAndGenericFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writeBusinessSSE(c, BusinessEvent{
		ID:         17,
		BusinessID: 42,
		Type:       "inventory.low",
		Data:       json.RawMessage(`{"item_id":12}`),
		Timestamp:  time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
	})

	idStr := "id: " + GetHub().Epoch() + ":17"
	chunks := strings.Split(strings.TrimSpace(w.Body.String()), "\n\n")
	require.Len(t, chunks, 2)
	assert.Equal(t, idStr+"\nevent: inventory.low\ndata: {\"item_id\":12}", chunks[0])
	assert.NotContains(t, chunks[1], "event:")
	assert.Contains(t, chunks[1], idStr)
	assert.Contains(t, chunks[1], `"type":"inventory.low"`)
	assert.Contains(t, chunks[1], `"data":{"item_id":12}`)
}

func TestSSEHandler_SuspendedBusinessClosesStreamCleanly(t *testing.T) {
	setupSSEHandlerTestDB(t)
	business := createSSETestBusiness(t)
	// Force the gate to deny: the administrator suspended the business.
	require.NoError(t, database.GetDB().Model(business).UpdateColumn("is_active", false).Error)

	req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(business.ID))+"/events", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(business.ID))}}
	c.Request = req
	c.Set("token_type", "staff")
	c.Set("staff_business_id", business.ID)

	SSEHandler(c)

	// EventSource-readable: a 200 SSE stream, not a 403 it cannot parse.
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	body := w.Body.String()
	assert.Contains(t, body, "event: error")
	assert.Contains(t, body, `"code":"business_suspended"`)
	// Must NOT have opened a live stream.
	assert.NotContains(t, body, "event: connected")
}

func TestSSEHandler_AccessDeniedClosesStreamCleanly(t *testing.T) {
	setupSSEHandlerTestDB(t)
	business := createSSETestBusiness(t) // active business, owner 0xowner

	req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(business.ID))+"/events", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(business.ID))}}
	c.Request = req
	// Identity that does NOT match the business owner or staff binding.
	c.Set("address", "0xsomeone-else")

	SSEHandler(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	body := w.Body.String()
	assert.Contains(t, body, "event: error")
	assert.Contains(t, body, `"code":"access_denied"`)
	assert.NotContains(t, body, "event: connected")
}

func TestSSEHandler_PlatformAdminCanStreamListedDemo(t *testing.T) {
	setupSSEHandlerTestDB(t)
	business := createSSETestBusiness(t)
	business.IsDemo = true
	business.Kind = database.BusinessKindDemo
	require.NoError(t, database.GetDB().Save(business).Error)

	req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(business.ID))+"/events", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(business.ID))}}
	c.Request = req
	c.Set("role", "admin")
	c.Set("user_id", uint(99))

	SSEHandler(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "event: connected")
	assert.NotContains(t, w.Body.String(), `"code":"access_denied"`)
}

func TestSSEHandler_PlatformAdminDeniedOnLiveTenant(t *testing.T) {
	setupSSEHandlerTestDB(t)
	business := createSSETestBusiness(t) // Kind defaults to real, IsDemo false

	req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(business.ID))+"/events", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(business.ID))}}
	c.Request = req
	c.Set("role", "admin")
	c.Set("user_id", uint(99))

	SSEHandler(c)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"code":"access_denied"`)
	assert.NotContains(t, body, "event: connected")
}

func TestSSEHandler_DemoOwnerCanStream(t *testing.T) {
	setupSSEHandlerTestDB(t)
	owner := uint(44)
	business := createSSETestBusiness(t)
	business.IsDemo = true
	business.Kind = database.BusinessKindDemo
	business.DemoOwnerUserID = &owner
	require.NoError(t, database.GetDB().Save(business).Error)

	req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(business.ID))+"/events", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(business.ID))}}
	c.Request = req
	c.Set("user_id", owner)

	SSEHandler(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "event: connected")
}
