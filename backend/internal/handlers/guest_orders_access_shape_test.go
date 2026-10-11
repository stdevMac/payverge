package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type guestOrdersSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *guestOrdersSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *guestOrdersSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table+" ") {
			count++
		}
	}
	return count
}

func (r *guestOrdersSQLRecorder) mentionsTable(table string) bool {
	for _, statement := range r.statements {
		normalized := strings.ToLower(statement)
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "join `"+table+"`") ||
			strings.Contains(normalized, "join "+table) ||
			strings.Contains(normalized, "from "+table) {
			return true
		}
	}
	return false
}

func setupGuestOrdersAccessDB(t testing.TB, gormLogger logger.Interface) *gorm.DB {
	t.Helper()
	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.Order{},
	))
	return gormDB
}

func seedGuestOrdersVenue(t testing.TB, display, def string) (*database.Business, *database.Bill, *database.Order) {
	t.Helper()
	biz := &database.Business{
		BusinessId:      fmt.Sprintf("guest-orders-%d", time.Now().UnixNano()),
		Name:            "Guest Orders Venue",
		OwnerAddress:    "0xGUESTORDEROWNER",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		IsActive:        true,
		DisplayCurrency: display,
		DefaultCurrency: def,
		OnboardingState: database.JSONRawMessage(`{"secret":true}`),
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	bill := &database.Bill{
		BusinessID:     biz.ID,
		BillNumber:     fmt.Sprintf("GO-%d", time.Now().UnixNano()),
		Items:          "[]",
		Status:         database.BillStatusOpen,
		SettlementAddr: biz.SettlementAddr,
		TippingAddr:    biz.TippingAddr,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NotEmpty(t, bill.PublicToken)
	order := &database.Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: fmt.Sprintf("O-%d", time.Now().UnixNano()),
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest",
		Items:       `[{"id":"item-1","menu_item_name":"Empanada","quantity":1,"price":5,"subtotal":5}]`,
	}
	require.NoError(t, database.GetDB().Create(order).Error)
	return biz, bill, order
}

func invokeGuestOrders(billToken string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: billToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/"+billToken+"/orders", nil)
	GetGuestOrdersByBillNumber(c)
	return w
}

// operatorOrdersJSON renders the bill's orders through the operator wire
// shape (Order.MarshalJSON via OrdersResponse), which every operator order
// list route emits.
func operatorOrdersJSON(t *testing.T, billID uint) []byte {
	t.Helper()
	orders, err := database.GetOrdersByBillID(billID)
	require.NoError(t, err)
	body, err := json.Marshal(OrdersResponse{Orders: orders, Total: int64(len(orders))})
	require.NoError(t, err)
	return body
}

type guestOrdersWire struct {
	Orders []struct {
		ID       uint   `json:"id"`
		Currency string `json:"currency"`
	} `json:"orders"`
	Total int64 `json:"total"`
}

// TestGetGuestOrdersByBillNumber_EmitsResolvedCurrencyMatchingOperator is
// the #560 money-display gate: guestOrderView must not copy the empty
// gorm:"-" Currency field. It has to emit the same resolved currency the
// operator MarshalJSON path returns (display → default → USD).
func TestGetGuestOrdersByBillNumber_EmitsResolvedCurrencyMatchingOperator(t *testing.T) {
	setupGuestOrdersAccessDB(t, nil)
	_, bill, order := seedGuestOrdersVenue(t, "ARS", "USD")

	guestW := invokeGuestOrders(bill.PublicToken)
	require.Equal(t, http.StatusOK, guestW.Code, guestW.Body.String())
	var guest guestOrdersWire
	require.NoError(t, json.Unmarshal(guestW.Body.Bytes(), &guest))
	require.Len(t, guest.Orders, 1)
	require.Equal(t, order.ID, guest.Orders[0].ID)
	require.NotEmpty(t, guest.Orders[0].Currency, "guest orders must not emit currency:\"\"")
	assert.Equal(t, "ARS", guest.Orders[0].Currency, "guest view must resolve display currency")

	var op OrdersResponse
	require.NoError(t, json.Unmarshal(operatorOrdersJSON(t, bill.ID), &op))
	require.Len(t, op.Orders, 1)
	assert.Equal(t, op.Orders[0].Currency, guest.Orders[0].Currency,
		"guest and operator must emit the same resolved currency for the same bill")
}

func TestGetGuestOrdersByBillNumber_FallsBackToDefaultThenUSD(t *testing.T) {
	setupGuestOrdersAccessDB(t, nil)
	_, bill, _ := seedGuestOrdersVenue(t, "", "MXN")
	guestW := invokeGuestOrders(bill.PublicToken)
	require.Equal(t, http.StatusOK, guestW.Code, guestW.Body.String())
	var guest guestOrdersWire
	require.NoError(t, json.Unmarshal(guestW.Body.Bytes(), &guest))
	require.Len(t, guest.Orders, 1)
	assert.Equal(t, "MXN", guest.Orders[0].Currency)

	setupGuestOrdersAccessDB(t, nil)
	_, bill2, _ := seedGuestOrdersVenue(t, "", "")
	guestW2 := invokeGuestOrders(bill2.PublicToken)
	require.Equal(t, http.StatusOK, guestW2.Code, guestW2.Body.String())
	var guest2 guestOrdersWire
	require.NoError(t, json.Unmarshal(guestW2.Body.Bytes(), &guest2))
	require.Len(t, guest2.Orders, 1)
	assert.Equal(t, "USD", guest2.Orders[0].Currency)
}

// TestGetGuestOrdersByBillNumberAccessShape proves the guest poll either
// skips businesses entirely or consumes only the narrow currency
// projection — never a full Business aggregate / SELECT *.
func TestGetGuestOrdersByBillNumberAccessShape(t *testing.T) {
	recorder := &guestOrdersSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupGuestOrdersAccessDB(t, recorder)
	biz, bill, _ := seedGuestOrdersVenue(t, "AED", "USD")
	require.NoError(t, database.GetDB().Model(biz).Update("onboarding_state", `{"secret":"GUEST_ORDER_LEAK"}`).Error)

	recorder.statements = nil
	w := invokeGuestOrders(bill.PublicToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var guest guestOrdersWire
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &guest))
	require.Len(t, guest.Orders, 1)
	assert.Equal(t, "AED", guest.Orders[0].Currency)
	assert.NotContains(t, w.Body.String(), "GUEST_ORDER_LEAK")
	assert.NotContains(t, w.Body.String(), `"business"`)
	assert.Zero(t, recorder.selectStarCount("businesses"),
		"guest orders must not SELECT * the businesses table")

	// If the path still touches businesses, it must be the 3-column currency
	// projection (or the lean is_active join on the token resolver) — never a
	// full-row hydrate of onboarding columns.
	if recorder.mentionsTable("businesses") {
		for _, stmt := range recorder.statements {
			normalized := strings.ToLower(stmt)
			if !strings.Contains(normalized, "business") {
				continue
			}
			assert.NotContains(t, normalized, "onboarding_state")
			assert.NotContains(t, normalized, "about_story")
		}
	}
}

func BenchmarkGetGuestOrdersByBillNumber(b *testing.B) {
	setupGuestOrdersAccessDB(b, logger.Default.LogMode(logger.Silent))
	_, bill, _ := seedGuestOrdersVenue(b, "ARS", "USD")
	for i := 0; i < 24; i++ {
		require.NoError(b, database.GetDB().Create(&database.Order{
			BillID:      bill.ID,
			BusinessID:  bill.BusinessID,
			OrderNumber: fmt.Sprintf("O-BENCH-%d", i),
			Status:      database.OrderStatusApproved,
			CreatedBy:   "guest",
			Items:       `[{"id":"item","menu_item_name":"Bench","quantity":1,"price":12.5,"subtotal":12.5}]`,
		}).Error)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := invokeGuestOrders(bill.PublicToken)
		if w.Code != http.StatusOK {
			b.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
	}
}
