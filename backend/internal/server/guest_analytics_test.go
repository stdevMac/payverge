package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

func TestGuestScanThrottle_AllowDenyExpire(t *testing.T) {
	st := &scanThrottle{seen: make(map[uint]time.Time)}
	now := time.Now()

	assert.True(t, st.Allow(1, now), "first scan of a table emits")
	assert.False(t, st.Allow(1, now.Add(time.Minute)), "re-scan inside the TTL is throttled")
	assert.True(t, st.Allow(2, now), "another table is independent")
	assert.True(t, st.Allow(1, now.Add(guestScanThrottleTTL+time.Second)), "after the TTL the table emits again")
}

func TestGuestScanThrottle_BoundedMemory(t *testing.T) {
	st := &scanThrottle{seen: make(map[uint]time.Time)}
	now := time.Now()
	for i := uint(0); i < guestScanThrottleMaxEntries+100; i++ {
		st.Allow(i, now)
	}
	assert.LessOrEqual(t, len(st.seen), guestScanThrottleMaxEntries+1,
		"throttle map must stay bounded even under pathological table churn")
}

// P2-19 + perf gate: the scan emit must (a) fire exactly once for rapid repeat
// loads of the same table, (b) add ZERO SQL to the hot route — the SQL recorder
// bounds here are identical to TestGetTableByCodePublicUsesSharedMenuCache,
// proving the instrumentation changed nothing about the route's access shape.
func TestGetTableByCodePublic_EmitsThrottledScanEventWithNoExtraQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicGuestTableHandlerTestDBWithLogger(t, recorder)
	services.ResetPricingCache()
	resetGuestScanThrottleForTest()

	origHook := guestAnalyticsTrackHook
	defer func() { guestAnalyticsTrackHook = origHook }()
	var mu sync.Mutex
	var events []string
	var distinctIDs []string
	guestAnalyticsTrackHook = func(distinctID, event string, _ map[string]interface{}) error {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
		distinctIDs = append(distinctIDs, distinctID)
		return nil
	}

	business := createSensitiveGuestTableBusiness(t, "SCANEVT1")
	table := createGuestPublicTable(t, business.ID, "SCANEVT1")

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
		c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s", table.TableCode), nil)
		GetTableByCodePublic(c)
		require.Equal(t, http.StatusOK, w.Code)
	}

	// Async emit — wait for it, then assert the throttle collapsed 2 loads → 1 event.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) == 1
	}, 2*time.Second, 10*time.Millisecond, "two rapid scans must emit exactly one guest_table_scanned")
	mu.Lock()
	assert.Equal(t, "guest_table_scanned", events[0])
	assert.Equal(t, business.BusinessId, distinctIDs[0], "distinct_id must be the BusinessId string")
	mu.Unlock()

	// Access shape unchanged: identical bounds to the pre-existing cache test.
	assert.LessOrEqual(t, recorder.selectCount("tables"), 1)
	assert.LessOrEqual(t, recorder.selectCount("businesses"), 2)
}

func TestCreateGuestOrder_EmitsOrderPlacedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)
	// OrderItem is JSON-on-Order, not a table — only Order needs AutoMigrate.
	require.NoError(t, database.GetDB().AutoMigrate(&database.Order{}))
	services.ResetPricingCache()
	services.ResetTelegramNotificationEligibilityCache()

	origHook := guestAnalyticsTrackHook
	defer func() { guestAnalyticsTrackHook = origHook }()
	var mu sync.Mutex
	var gotEvent string
	var gotProps map[string]interface{}
	guestAnalyticsTrackHook = func(_ string, event string, props map[string]interface{}) error {
		mu.Lock()
		defer mu.Unlock()
		gotEvent, gotProps = event, props
		return nil
	}

	// Mirror TestCreateGuestOrderUsesSharedMenuCache fixture shape (menu JSON
	// with is_available, open bill, kitchen+orders enabled).
	business := createSensitiveGuestTableBusiness(t, "ORDEVT01")
	table := createGuestPublicTable(t, business.ID, "ORDEVT01")
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID, IsActive: true,
		Categories: `[{"id":"cat1","name":"Drinks","items":[{"id":"item1","name":"Coffee","price":3.5,"is_available":true}]}]`,
	}).Error)
	bill := database.Bill{
		BusinessID: business.ID, TableID: table.ID, Status: database.BillStatusOpen,
		BillNumber: "B-EVT-1", SettlementAddr: business.SettlementAddr, TippingAddr: business.TippingAddr,
	}
	require.NoError(t, database.GetDB().Create(&bill).Error)

	body := fmt.Sprintf(`{"bill_id":%d,"items":[{"menu_item_id":"item1","menu_item_name":"Coffee","quantity":2,"price":3.5}]}`, bill.ID)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/order", table.TableCode), strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Request-Id", "guest-analytics-order-1")

	CreateGuestOrder(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return gotEvent == "guest_order_placed"
	}, 2*time.Second, 10*time.Millisecond)
	mu.Lock()
	assert.Equal(t, 1, gotProps["item_count"], "one order line (qty folds into the line)")
	mu.Unlock()
}
