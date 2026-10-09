package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGuestTableEventPayloadContainsLifecycleOnly(t *testing.T) {
	raw, err := json.Marshal(events.TableBillChanged{HasActiveBill: false})
	require.NoError(t, err)
	require.JSONEq(t, `{"has_active_bill":false}`, string(raw))
	for _, forbidden := range []string{"bill_id", "customer", "payment", "amount", "items"} {
		require.NotContains(t, string(raw), forbidden)
	}
}

func TestGuestTableEventsOverCapacityWritesSSETerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)
	business := createSensitiveGuestTableBusiness(t, "CAPSSE01")
	require.NoError(t, database.GetDB().Model(business).Update("is_active", true).Error)
	table := createGuestPublicTable(t, business.ID, "CAPSSE01")

	hub := events.NewTableHub(50, 200, 10, 1)
	_, cancel, err := hub.Subscribe(table.ID, business.ID, "203.0.113.9")
	require.NoError(t, err)
	t.Cleanup(cancel)
	t.Cleanup(events.SetTableHubForTest(hub))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/table/CAPSSE01/events", nil)
	c.Params = gin.Params{{Key: "code", Value: "CAPSSE01"}}

	GuestTableEvents(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	body := w.Body.String()
	require.Contains(t, body, "retry: 60000")
	require.Contains(t, body, `"code":"capacity"`)
	require.Contains(t, body, "Too many live connections")
}

func TestGuestTableEventsLockedBusinessWritesSSETerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)
	business := createSensitiveGuestTableBusiness(t, "LOCKSSE1")
	require.NoError(t, database.GetDB().Model(business).Updates(map[string]any{"is_active": true, "closed_at": time.Now()}).Error)
	createGuestPublicTable(t, business.ID, "LOCKSSE1")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/table/LOCKSSE1/events", nil)
	c.Params = gin.Params{{Key: "code", Value: "LOCKSSE1"}}

	GuestTableEvents(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	body := w.Body.String()
	require.Contains(t, body, "retry: 60000")
	require.Contains(t, body, "event: error")
	require.Contains(t, body, database.BusinessLockCodeClosed)
	require.NotContains(t, body, `"code":"capacity"`)
}
