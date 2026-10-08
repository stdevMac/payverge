package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type pluginCheckoutSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *pluginCheckoutSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *pluginCheckoutSQLRecorder) selectCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

func (r *pluginCheckoutSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

func setupPluginCheckoutAccessDB(t testing.TB, rec logger.Interface) (*database.Business, *database.Bill, string) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if rec != nil {
		cfg.Logger = rec
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
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
	))

	business := &database.Business{
		BusinessId:      fmt.Sprintf("plugin-checkout-%d", time.Now().UnixNano()),
		Name:            "Plugin Checkout Restaurant",
		OwnerAddress:    "0xPluginCheckoutOwner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		IsActive:        true,
	}
	require.NoError(t, gormDB.Create(business).Error)

	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  fmt.Sprintf("PLUGIN-%d", time.Now().UnixNano()),
		Name:       "Plugin Checkout Table",
		Capacity:   4,
		QRCode:     strings.Repeat("qr-payload", 64),
		IsActive:   true,
	}
	require.NoError(t, gormDB.Create(table).Error)

	bill := &database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  fmt.Sprintf("PLUGIN-BILL-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 5000,
		PaidAmount:  0,
	}
	require.NoError(t, gormDB.Create(bill).Error)

	require.NoError(t, gormDB.Create(&database.Payment{
		BillID:    bill.ID,
		PayerAddr: "0xpayer",
		Amount:    100,
		TxHash:    "0xexisting-payment",
	}).Error)

	pluginName := fmt.Sprintf("checkout-shape-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "Checkout Shape",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, gormDB.Create(pluginRecord).Error)
	require.NoError(t, gormDB.Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   pluginRecord.ID,
		IsEnabled:  true,
		Config:     `{"mode":"test"}`,
	}).Error)

	plugins.GlobalRegistry.RegisterPlugin(&testPaymentPlugin{name: pluginName})
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin(pluginName) })

	originalSupport := guestBillPaymentPluginSupported
	guestBillPaymentPluginSupported = func(name string) bool { return name == pluginName }
	t.Cleanup(func() { guestBillPaymentPluginSupported = originalSupport })

	return business, bill, pluginName
}

func TestCreatePluginPayment_UsesLeanBillAndBusinessLoads(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := &pluginCheckoutSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	_, bill, pluginName := setupPluginCheckoutAccessDB(t, recorder)

	body, err := json.Marshal(map[string]interface{}{
		"plugin_id":  pluginName,
		"amount":     50,
		"currency":   "USD",
		"tip_amount": 0,
	})
	require.NoError(t, err)

	recorder.statements = nil
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).CreatePluginPayment(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	assert.Equal(t, 1, recorder.selectCount("bills"), "lean checkout should use one bill join")
	assert.Equal(t, 1, recorder.selectCount("tables"), "lean checkout should use one table_code lookup")
	assert.Equal(t, 1, recorder.selectCount("businesses"))
	assert.Zero(t, recorder.selectCount("payments"), "guest checkout must not preload payments")
	assert.Zero(t, recorder.selectStarCount("bills"))
	assert.Zero(t, recorder.selectStarCount("businesses"))
}
