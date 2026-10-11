package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guestsession"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

type testPaymentPlugin struct {
	name            string
	lastBusinessID  uint
	lastBillID      uint
	lastAmount      int64
	lastCurrency    string
	lastMetadata    map[string]interface{}
	lastStatusID    string
	lastStatusBizID uint
	lastReturnOrder string
	status          string
	statusCalls     int
	paymentResponse *plugins.PaymentResponse
	webhookResponse *plugins.WebhookResponse
	returnResponse  *plugins.WebhookResponse
	// refund recording (CR-7)
	refundCalls         int
	lastRefundBizID     uint
	lastRefundPaymentID string
	lastRefundAmount    int64
	refundErr           error // when non-nil, RefundPayment records the call then returns this
	refundHook          func()
	webhookPanic        any   // when non-nil, HandleWebhook panics with it
	statusErr           error // when non-nil, GetPaymentStatus returns this (models Stripe Checkout, which has no queryable status)
	// verifySignature, when true, makes VerifyWebhookSignature return true so the
	// double can be registered under a real provider name (e.g. "mercadopago")
	// and pass signature verification with a stub secret/header.
	verifySignature bool
}

func (p *testPaymentPlugin) GetName() string                               { return p.name }
func (p *testPaymentPlugin) GetDisplayName() string                        { return p.name }
func (p *testPaymentPlugin) GetDescription() string                        { return "test payment plugin" }
func (p *testPaymentPlugin) GetCategory() string                           { return database.PluginCategoryPayment }
func (p *testPaymentPlugin) GetVersion() string                            { return "1.0.0" }
func (p *testPaymentPlugin) GetFeatures() string                           { return "[]" }
func (p *testPaymentPlugin) GetConfigSchema() string                       { return `{"type":"object"}` }
func (p *testPaymentPlugin) IsActive() bool                                { return true }
func (p *testPaymentPlugin) ValidateConfig(map[string]interface{}) error   { return nil }
func (p *testPaymentPlugin) Initialize(uint, map[string]interface{}) error { return nil }
func (p *testPaymentPlugin) Cleanup(uint) error                            { return nil }
func (p *testPaymentPlugin) ProcessPayment(uint, int64, string, map[string]interface{}) (string, error) {
	return "test-payment", nil
}
func (p *testPaymentPlugin) RefundPayment(businessID uint, paymentID string, amount int64) error {
	p.refundCalls++
	p.lastRefundBizID = businessID
	p.lastRefundPaymentID = paymentID
	p.lastRefundAmount = amount
	if p.refundHook != nil {
		p.refundHook()
	}
	return p.refundErr
}
func (p *testPaymentPlugin) GetPaymentStatus(businessID uint, paymentID string) (string, error) {
	p.statusCalls++
	p.lastStatusBizID = businessID
	p.lastStatusID = paymentID
	if p.statusErr != nil {
		return "", p.statusErr
	}
	if p.status != "" {
		return p.status, nil
	}
	return "pending", nil
}
func (p *testPaymentPlugin) CreateBillPayment(businessID uint, billID uint, amount int64, currency string, metadata map[string]interface{}) (*plugins.PaymentResponse, error) {
	p.lastBusinessID = businessID
	p.lastBillID = billID
	p.lastAmount = amount
	p.lastCurrency = currency
	p.lastMetadata = metadata

	if p.paymentResponse != nil {
		return p.paymentResponse, nil
	}

	return &plugins.PaymentResponse{
		PaymentID:   "test-payment-id",
		Status:      "pending",
		PaymentURL:  "https://example.com/pay",
		RedirectURL: "https://example.com/pay",
		Metadata:    map[string]interface{}{},
	}, nil
}
func (p *testPaymentPlugin) HandleWebhook(uint, []byte, map[string]string) (*plugins.WebhookResponse, error) {
	if p.webhookPanic != nil {
		panic(p.webhookPanic)
	}
	if p.webhookResponse != nil {
		return p.webhookResponse, nil
	}
	return &plugins.WebhookResponse{Success: true, Message: "ok"}, nil
}
func (p *testPaymentPlugin) GetWebhookEndpoint() string { return "/test" }
func (p *testPaymentPlugin) VerifyWebhookSignature([]byte, string, string) bool {
	return p.verifySignature
}
func (p *testPaymentPlugin) CapturePaymentReturn(businessID uint, billID uint, orderID string) (*plugins.WebhookResponse, error) {
	p.lastBusinessID = businessID
	p.lastBillID = billID
	p.lastReturnOrder = orderID
	if p.returnResponse != nil {
		return p.returnResponse, nil
	}
	return &plugins.WebhookResponse{Success: true, Status: "completed", PaymentID: "capture-test", BillID: billID, Amount: 2500, Currency: "USD"}, nil
}

func TestTestPaymentPluginRecordsRefundInvocations(t *testing.T) {
	p := &testPaymentPlugin{name: "rec"}
	err := p.RefundPayment(77, "pay-xyz", 0)
	require.NoError(t, err)
	require.Equal(t, 1, p.refundCalls)
	require.Equal(t, uint(77), p.lastRefundBizID)
	require.Equal(t, "pay-xyz", p.lastRefundPaymentID)
	require.Equal(t, int64(0), p.lastRefundAmount)

	p.refundErr = errors.New("psp boom")
	err = p.RefundPayment(77, "pay-xyz", 0)
	require.Error(t, err)
	require.Equal(t, 2, p.refundCalls)
}

type lifecycleTestPlugin struct {
	name          string
	validateErr   error
	initErr       error
	cleanupErr    error
	validateCalls int
	initCalls     int
	cleanupCalls  int
	lastBusiness  uint
	lastConfig    map[string]interface{}
}

func (p *lifecycleTestPlugin) GetName() string         { return p.name }
func (p *lifecycleTestPlugin) GetDisplayName() string  { return p.name }
func (p *lifecycleTestPlugin) GetDescription() string  { return "test lifecycle plugin" }
func (p *lifecycleTestPlugin) GetCategory() string     { return database.PluginCategoryIntegration }
func (p *lifecycleTestPlugin) GetVersion() string      { return "1.0.0" }
func (p *lifecycleTestPlugin) GetFeatures() string     { return "[]" }
func (p *lifecycleTestPlugin) GetConfigSchema() string { return `{"type":"object"}` }
func (p *lifecycleTestPlugin) IsActive() bool          { return true }
func (p *lifecycleTestPlugin) ValidateConfig(config map[string]interface{}) error {
	p.validateCalls++
	p.lastConfig = config
	return p.validateErr
}
func (p *lifecycleTestPlugin) Initialize(businessID uint, config map[string]interface{}) error {
	p.initCalls++
	p.lastBusiness = businessID
	p.lastConfig = config
	return p.initErr
}
func (p *lifecycleTestPlugin) Cleanup(businessID uint) error {
	p.cleanupCalls++
	p.lastBusiness = businessID
	return p.cleanupErr
}

type detailedTestPaymentPlugin struct {
	testPaymentPlugin
	details                      *plugins.PaymentStatusDetails
	detailErr                    error
	detailCalls                  int
	attachedBusinessID           uint
	attachedPaymentID            string
	attachedAlternativePaymentID uint
	attachErr                    error
}

func (p *detailedTestPaymentPlugin) GetPaymentStatusDetails(businessID uint, paymentID string) (*plugins.PaymentStatusDetails, error) {
	p.detailCalls++
	p.lastStatusBizID = businessID
	p.lastStatusID = paymentID
	if p.detailErr != nil {
		return nil, p.detailErr
	}
	if p.details != nil {
		return p.details, nil
	}
	return &plugins.PaymentStatusDetails{
		Status: "awaiting_payment",
		Metadata: map[string]interface{}{
			"address": "TCsHYKC27np7cGAxJEq55DnsGysejpFF11",
			"network": "tron",
			"asset":   "USDT",
			"amount":  "28.00",
		},
	}, nil
}

func (p *detailedTestPaymentPlugin) AttachAlternativePayment(businessID uint, paymentID string, alternativePaymentID uint) error {
	p.attachedBusinessID = businessID
	p.attachedPaymentID = paymentID
	p.attachedAlternativePaymentID = alternativePaymentID
	return p.attachErr
}

type configHookTestPlugin struct {
	lifecycleTestPlugin
	normalizeCalls int
	publicCalls    int
}

func (p *configHookTestPlugin) NormalizeConfig(businessID uint, config map[string]interface{}) (map[string]interface{}, error) {
	p.normalizeCalls++
	p.lastBusiness = businessID

	normalized := make(map[string]interface{}, len(config)+2)
	for key, value := range config {
		if key == "secret" {
			continue
		}
		normalized[key] = value
	}
	if secret, ok := config["secret"].(string); ok && secret != "" {
		normalized["secret_encrypted"] = "v1:" + secret
	}
	normalized["normalized"] = true
	return normalized, nil
}

func (p *configHookTestPlugin) PublicConfig(config map[string]interface{}) map[string]interface{} {
	p.publicCalls++
	public := make(map[string]interface{}, len(config)+1)
	for key, value := range config {
		if key == "secret_encrypted" {
			public["secret_masked"] = true
			continue
		}
		public[key] = value
	}
	return public
}

func registerPluginForHandlerTest(t *testing.T, plugin plugins.Plugin) {
	t.Helper()
	previous, hadPrevious := plugins.GlobalRegistry.GetPlugin(plugin.GetName())
	plugins.GlobalRegistry.RegisterPlugin(plugin)
	t.Cleanup(func() {
		if hadPrevious {
			plugins.GlobalRegistry.RegisterPlugin(previous)
			return
		}
		plugins.GlobalRegistry.UnregisterPlugin(plugin.GetName())
	})
}

func TestGetBusinessPaymentPlugins_UsesBusinessIDRouteParam(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)

	paymentPlugin := &database.Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	analyticsPlugin := &database.Plugin{
		Name:        "reporting",
		DisplayName: "Reporting",
		Category:    database.PluginCategoryAnalytics,
		IsActive:    true,
	}
	disabledPaymentPlugin := &database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(paymentPlugin).Error)
	require.NoError(t, database.GetDB().Create(analyticsPlugin).Error)
	require.NoError(t, database.GetDB().Create(disabledPaymentPlugin).Error)

	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   paymentPlugin.ID,
		IsEnabled:  true,
		Config:     `{"access_token":"sk_test_guest_visible_xxxxxxxx"}`,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   analyticsPlugin.ID,
		IsEnabled:  true,
		Config:     "{}",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   disabledPaymentPlugin.ID,
		IsEnabled:  false,
		Config:     "{}",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Plugins, 1)
	assert.Equal(t, "payment", resp.Plugins[0]["category"])
	assert.Equal(t, true, resp.Plugins[0]["is_enabled"])
	assert.EqualValues(t, paymentPlugin.ID, resp.Plugins[0]["plugin_id"])
}

func TestGetBusinessPaymentPlugins_DemoHouseRailDoesNotOpenCashSession(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.CashRegisterSession{},
		&database.CashRegisterMovement{},
	))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Updates(map[string]any{
		"is_demo": true,
		"kind":    database.BusinessKindDemo,
	}).Error)
	closedAt := time.Date(2026, 8, 21, 4, 0, 0, 0, time.UTC)
	require.NoError(t, database.GetDB().Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 20000,
		OpenedByLabel:     "demo-manager",
		OpenedAt:          closedAt.Add(-2 * time.Hour),
		ClosedByLabel:     "demo-manager",
		ClosedAt:          &closedAt,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Plugins                []map[string]interface{} `json:"plugins"`
		CounterSettlementReady bool                     `json:"counter_settlement_ready"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Empty(t, resp.Plugins)
	assert.False(t, resp.CounterSettlementReady)

	open, err := database.FindOpenCashRegisterSessionForBusinessTx(database.GetDB(), business.ID)
	require.NoError(t, err)
	require.Nil(t, open)

	var sessionCount int64
	require.NoError(t, database.GetDB().Model(&database.CashRegisterSession{}).
		Where("business_id = ?", business.ID).
		Count(&sessionCount).Error)
	require.Equal(t, int64(1), sessionCount)
}

func TestGetBusinessPaymentPlugins_RealVenueDoesNotOpenCashSession(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.CashRegisterSession{},
		&database.CashRegisterMovement{},
	))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Updates(map[string]any{
		"is_demo": true,
		"kind":    database.BusinessKindReal,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Plugins                []map[string]interface{} `json:"plugins"`
		CounterSettlementReady bool                     `json:"counter_settlement_ready"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Empty(t, resp.Plugins)
	assert.False(t, resp.CounterSettlementReady)

	open, err := database.FindOpenCashRegisterSessionForBusinessTx(database.GetDB(), business.ID)
	require.NoError(t, err)
	require.Nil(t, open)

	var sessionCount int64
	require.NoError(t, database.GetDB().Model(&database.CashRegisterSession{}).
		Where("business_id = ?", business.ID).
		Count(&sessionCount).Error)
	require.Zero(t, sessionCount)
}

func TestGetBusinessPaymentPlugins_ReportsReadyWhenDrawerAlreadyOpen(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.CashRegisterSession{},
		&database.CashRegisterMovement{},
	))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusOpen,
		OpeningFloatCents: 20000,
		OpenedByLabel:     "manager",
		OpenedAt:          time.Date(2026, 8, 21, 18, 0, 0, 0, time.UTC),
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		CounterSettlementReady bool `json:"counter_settlement_ready"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.CounterSettlementReady)

	var sessionCount int64
	require.NoError(t, database.GetDB().Model(&database.CashRegisterSession{}).
		Where("business_id = ?", business.ID).
		Count(&sessionCount).Error)
	require.Equal(t, int64(1), sessionCount)
}

// TestGetBusinessPaymentPlugins_ListsHouseCounterRailWhenDrawerOpen (issue 894):
// a venue with no processor credentials still has one live, settleable rail —
// the open cash drawer. Reporting it only through the counter_settlement_ready
// boolean left the `plugins` rail list empty, so a client reading the list
// concluded "nothing to tap" while cash was being taken at the counter. The
// house rail must appear in the list itself, carrying no credentials and no
// plugin_id, marked as counter-settled so nobody routes it to a processor.
func TestGetBusinessPaymentPlugins_ListsHouseCounterRailWhenDrawerOpen(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.CashRegisterSession{},
		&database.CashRegisterMovement{},
	))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusOpen,
		OpeningFloatCents: 20000,
		OpenedByLabel:     "manager",
		OpenedAt:          time.Date(2026, 8, 21, 18, 0, 0, 0, time.UTC),
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Plugins                []map[string]interface{} `json:"plugins"`
		CounterSettlementReady bool                     `json:"counter_settlement_ready"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.CounterSettlementReady)
	require.Len(t, resp.Plugins, 1)

	rail := resp.Plugins[0]
	assert.Equal(t, "counter_cash", rail["name"])
	assert.Equal(t, "payment", rail["category"])
	assert.Equal(t, true, rail["is_enabled"])
	assert.Equal(t, "counter", rail["settlement"])
	// It is not a plugin: no catalog id, and never any credential material.
	assert.NotContains(t, rail, "plugin_id")
	assert.NotContains(t, rail, "config")
}

// TestGetBusinessPaymentPlugins_OmitsHouseCounterRailWhenDrawerClosed (issue
// 894): the honest rail list must stay honest in the other direction too — a
// closed drawer cannot settle cash, so it must not be advertised. An empty
// rail list is still an empty JSON array, never null.
func TestGetBusinessPaymentPlugins_OmitsHouseCounterRailWhenDrawerClosed(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.CashRegisterSession{},
		&database.CashRegisterMovement{},
	))

	business := createTestBusiness(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Plugins                []map[string]interface{} `json:"plugins"`
		CounterSettlementReady bool                     `json:"counter_settlement_ready"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.CounterSettlementReady)
	assert.Empty(t, resp.Plugins)

	// `plugins` is advertised as a list and clients iterate it directly.
	assert.Contains(t, w.Body.String(), `"plugins":[]`)
	assert.NotContains(t, w.Body.String(), `"plugins":null`)
}

func TestGetBusinessPaymentPlugins_ExposesEnabledFirstPartyGuestPaymentProviders(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	stripePlugin := &database.Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	paypalPlugin := &database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	mercadoPagoPlugin := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(stripePlugin).Error)
	require.NoError(t, database.GetDB().Create(paypalPlugin).Error)
	require.NoError(t, database.GetDB().Create(mercadoPagoPlugin).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   stripePlugin.ID,
		IsEnabled:  true,
		Config:     `{"access_token":"sk_test_guest_visible_xxxxxxxx"}`,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   paypalPlugin.ID,
		IsEnabled:  true,
		Config:     `{"client_id":"paypal-public-client"}`,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   mercadoPagoPlugin.ID,
		IsEnabled:  true,
		Config:     `{"access_token":"TEST-123456789012345678901"}`,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Plugins, 3)
	names := []string{
		resp.Plugins[0]["name"].(string),
		resp.Plugins[1]["name"].(string),
		resp.Plugins[2]["name"].(string),
	}
	assert.ElementsMatch(t, []string{"stripe", "paypal", "mercadopago"}, names)
}

func TestGetBusinessPaymentPlugins_ExposesEnabledCryptoPlugins(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	usdcPlugin := &database.Plugin{
		Name:        "usdc_payment",
		DisplayName: "USDC",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	crossChainPlugin := &database.Plugin{
		Name:        "cross_chain_payment",
		DisplayName: "Cross-chain",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(usdcPlugin).Error)
	require.NoError(t, database.GetDB().Create(crossChainPlugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, usdcPlugin.ID, map[string]interface{}{"enabled": true}))
	require.NoError(t, database.EnableBusinessPlugin(business.ID, crossChainPlugin.ID, map[string]interface{}{"enabled": true}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// cross_chain_payment stays enabled for the business but is not offered to
	// guests while the rail cannot be payer-bound.
	require.Len(t, resp.Plugins, 1)
	assert.Equal(t, "usdc_payment", resp.Plugins[0]["name"])

	enableGuestCrossChainSettlementForTest(t)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Plugins, 2)
	assert.Equal(t, "usdc_payment", resp.Plugins[0]["name"])
	assert.Equal(t, "cross_chain_payment", resp.Plugins[1]["name"])
}

func TestGetBusinessPaymentPlugins_OrdersMercadoPagoBeforeCrypto(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	usdcPlugin := &database.Plugin{
		Name:        "usdc_payment",
		DisplayName: "USDC",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	mpPlugin := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(usdcPlugin).Error)
	require.NoError(t, database.GetDB().Create(mpPlugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, usdcPlugin.ID, map[string]interface{}{"enabled": true}))
	require.NoError(t, database.EnableBusinessPlugin(business.ID, mpPlugin.ID, map[string]interface{}{
		"enabled":      true,
		"access_token": "TEST-123456789012345678901",
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Plugins, 2)
	assert.Equal(t, "mercadopago", resp.Plugins[0]["name"])
	assert.Equal(t, "usdc_payment", resp.Plugins[1]["name"])
}

func TestGetBusinessPaymentPlugins_HidesCryptoPluginWhenConfigDisabled(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "usdc_payment",
		DisplayName: "USDC",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{"enabled": false}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Plugins)
}

func TestGetBusinessPaymentPlugins_ExposesEnabledPayPal(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"enabled":                  true,
		"client_secret_encrypted":  "v1:abc",
		"webhook_secret_encrypted": "v1:def",
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Plugins, 1)
	assert.Equal(t, "paypal", resp.Plugins[0]["name"])
	assert.Equal(t, "payment", resp.Plugins[0]["category"])
	assert.Equal(t, true, resp.Plugins[0]["is_enabled"])
}

func TestGetBusinessPaymentPlugins_LangQueryReturnsTranslatedDisplayName(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.PluginTranslation{},
	))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "usdc_payment",
		DisplayName: "USDC Payment",
		Description: "Pay with USDC",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{"enabled": true}))
	require.NoError(t, database.CreateOrUpdatePluginTranslation(
		plugin.ID, "es", "display_name", "Pago con USDC",
	))
	require.NoError(t, database.CreateOrUpdatePluginTranslation(
		plugin.ID, "es", "description", "Acepta pagos USDC",
	))

	// Spanish request returns translated catalog copy.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/?lang=es", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Plugins, 1)
	assert.Equal(t, "Pago con USDC", resp.Plugins[0]["display_name"])
	assert.Equal(t, "Acepta pagos USDC", resp.Plugins[0]["description"])
	// Still guest-safe: no operator health fields.
	_, hasLastError := resp.Plugins[0]["last_error"]
	assert.False(t, hasLastError)

	// Unsupported lang degrades to English catalog names (no 400).
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c2.Request = httptest.NewRequest(http.MethodGet, "/?lang=xx-ZZ", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c2)

	assert.Equal(t, http.StatusOK, w2.Code)
	var resp2 struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	require.Len(t, resp2.Plugins, 1)
	assert.Equal(t, "USDC Payment", resp2.Plugins[0]["display_name"])
}

func TestGetBusinessPaymentPlugins_LangQueryFallsBackToBaseLanguage(t *testing.T) {
	enableGuestCrossChainSettlementForTest(t) // fixture uses the cross-chain plugin row
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.PluginTranslation{},
	))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "cross_chain_payment",
		DisplayName: "Pay with Any Token",
		Description: "Cross-chain crypto",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{"enabled": true}))
	// Only base "es" rows — no es-AR — so es-AR must fall back to es.
	require.NoError(t, database.CreateOrUpdatePluginTranslation(
		plugin.ID, "es", "display_name", "Pago con Cualquier Token",
	))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/?lang=es-AR", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Plugins, 1)
	assert.Equal(t, "Pago con Cualquier Token", resp.Plugins[0]["display_name"])
}

func TestEnableBusinessPlugin_ReturnsBusinessPlugin(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "telegram",
		DisplayName: "Telegram",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	body, err := json.Marshal(map[string]interface{}{
		"config": map[string]interface{}{
			"chat_id": "1234",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", plugin.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil).EnableBusinessPlugin(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Message        string                  `json:"message"`
		BusinessPlugin database.BusinessPlugin `json:"business_plugin"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Plugin enabled successfully", resp.Message)
	assert.Equal(t, business.ID, resp.BusinessPlugin.BusinessID)
	assert.Equal(t, plugin.ID, resp.BusinessPlugin.PluginID)
	assert.True(t, resp.BusinessPlugin.IsEnabled)
	assert.Equal(t, plugin.ID, resp.BusinessPlugin.Plugin.ID)
}

func TestEnableBusinessPlugin_WarnsWhenTelegramDeliveryDisabled(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "telegram",
		DisplayName: "Telegram",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	services.SetPluginDeliveryEnabled("telegram", false)
	t.Cleanup(func() { services.SetPluginDeliveryEnabled("telegram", true) })

	body, err := json.Marshal(map[string]interface{}{
		"config": map[string]interface{}{
			"chat_id": "1234",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", plugin.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil).EnableBusinessPlugin(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["delivery_available"])
	assert.NotEmpty(t, resp["warning"])
	assert.Contains(t, resp["warning"], "bot token")
}

func TestEnableBusinessPlugin_RejectsComingSoonPlugin(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "future_integration",
		DisplayName: "Future Integration",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
		ComingSoon:  true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	pluginService := services.NewPluginService(database.GetDBWrapper())

	body, err := json.Marshal(map[string]interface{}{
		"config": map[string]interface{}{},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", plugin.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(pluginService, nil).EnableBusinessPlugin(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "coming soon")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.BusinessPlugin{}).Where("plugin_id = ?", plugin.ID).Count(&count).Error)
	assert.Zero(t, count)
}

// publicConfigTestPlugin implements plugins.PublicConfigProvider so the
// handler's provider-controlled masking branch (publicPluginConfig /
// pluginManagesOwnSecrets) stays covered now that no shipped plugin
// implements the interface.
type publicConfigTestPlugin struct {
	testPaymentPlugin
}

func (p *publicConfigTestPlugin) PublicConfig(config map[string]interface{}) map[string]interface{} {
	public := map[string]interface{}{}
	for key, value := range config {
		switch key {
		case "api_key", "api_key_encrypted", "webhook_secret", "webhook_secret_encrypted":
			continue
		}
		public[key] = value
	}
	_, hasKey := config["api_key"]
	_, hasKeyEnc := config["api_key_encrypted"]
	public["has_api_key"] = hasKey || hasKeyEnc
	_, hasSecret := config["webhook_secret"]
	_, hasSecretEnc := config["webhook_secret_encrypted"]
	public["has_webhook_secret"] = hasSecret || hasSecretEnc
	return public
}

func TestEnableBusinessPlugin_MasksPublicConfigProviderResponse(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "maskedpay",
		DisplayName: "Masked Pay",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	pluginService := services.NewPluginService(database.GetDBWrapper())
	registerPluginForHandlerTest(t, &publicConfigTestPlugin{testPaymentPlugin: testPaymentPlugin{name: "maskedpay"}})

	body, err := json.Marshal(map[string]interface{}{
		"config": map[string]interface{}{
			"enabled":        true,
			"api_key":        "mp_secret_value",
			"webhook_secret": "whsec_secret",
			"fee_mode":       "business_absorbs",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", plugin.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(pluginService, nil).EnableBusinessPlugin(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		BusinessPlugin database.BusinessPlugin `json:"business_plugin"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotContains(t, resp.BusinessPlugin.Config, "api_key_encrypted")
	require.NotContains(t, resp.BusinessPlugin.Config, "webhook_secret_encrypted")
	require.NotContains(t, resp.BusinessPlugin.Config, "mp_secret_value")
	require.NotContains(t, resp.BusinessPlugin.Config, "whsec_secret")
	require.Contains(t, resp.BusinessPlugin.Config, `"has_api_key":true`)
	require.Contains(t, resp.BusinessPlugin.Config, `"has_webhook_secret":true`)
}

func TestUpdateBusinessPluginConfig_ReturnsUpdatedBusinessPlugin(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{"mode": "test"}))

	body, err := json.Marshal(map[string]interface{}{
		"config": map[string]interface{}{
			"mode":   "live",
			"secret": "sk_test",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", plugin.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil).UpdateBusinessPluginConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Message        string                  `json:"message"`
		BusinessPlugin database.BusinessPlugin `json:"business_plugin"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Plugin configuration updated successfully", resp.Message)
	assert.Equal(t, business.ID, resp.BusinessPlugin.BusinessID)
	assert.Equal(t, plugin.ID, resp.BusinessPlugin.PluginID)

	config, err := database.GetBusinessPluginConfig(business.ID, plugin.Name)
	require.NoError(t, err)
	assert.Equal(t, "live", config["mode"])
	assert.Equal(t, "sk_test", config["secret"])
}

func TestEnableBusinessPlugin_UsesRuntimePluginLifecycle(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	pluginName := fmt.Sprintf("lifecycle-enable-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "Lifecycle Enable",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)

	mockPlugin := &lifecycleTestPlugin{name: pluginName}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	body, err := json.Marshal(map[string]interface{}{
		"config": map[string]interface{}{
			"business_name": "Test Business",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", pluginRecord.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil).EnableBusinessPlugin(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, mockPlugin.validateCalls)
	assert.Equal(t, 1, mockPlugin.initCalls)
	assert.Equal(t, 0, mockPlugin.cleanupCalls)
	assert.Equal(t, business.ID, mockPlugin.lastBusiness)
	assert.Equal(t, "Test Business", mockPlugin.lastConfig["business_name"])
}

func TestUpdateBusinessPluginConfig_ReturnsRuntimeValidationError(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	pluginName := fmt.Sprintf("lifecycle-update-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "Lifecycle Update",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{
		"business_name": "Old Name",
	}))

	mockPlugin := &lifecycleTestPlugin{
		name:        pluginName,
		validateErr: fmt.Errorf("invalid runtime configuration"),
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	body, err := json.Marshal(map[string]interface{}{
		"config": map[string]interface{}{
			"business_name": "",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", pluginRecord.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil).UpdateBusinessPluginConfig(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid runtime configuration")
	assert.Equal(t, 1, mockPlugin.validateCalls)
	assert.Equal(t, 0, mockPlugin.initCalls)
}

func TestEnableBusinessPlugin_NormalizesRuntimePluginConfig(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	pluginName := fmt.Sprintf("config-normalize-enable-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "Config Normalize Enable",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)

	mockPlugin := &configHookTestPlugin{lifecycleTestPlugin: lifecycleTestPlugin{name: pluginName}}
	registerPluginForHandlerTest(t, mockPlugin)

	body, err := json.Marshal(map[string]interface{}{
		"config": map[string]interface{}{
			"enabled": true,
			"secret":  "plain-secret",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", pluginRecord.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil).EnableBusinessPlugin(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, mockPlugin.normalizeCalls)
	assert.Equal(t, 1, mockPlugin.validateCalls)
	assert.Equal(t, 1, mockPlugin.initCalls)
	assert.Equal(t, business.ID, mockPlugin.lastBusiness)
	assert.Equal(t, true, mockPlugin.lastConfig["normalized"])
	assert.Equal(t, "v1:plain-secret", mockPlugin.lastConfig["secret_encrypted"])
	assert.NotContains(t, mockPlugin.lastConfig, "secret")

	config, err := database.GetBusinessPluginConfig(business.ID, pluginName)
	require.NoError(t, err)
	assert.Equal(t, true, config["normalized"])
	assert.Equal(t, "v1:plain-secret", config["secret_encrypted"])
	assert.NotContains(t, config, "secret")
}

func TestUpdateBusinessPluginConfig_NormalizesRuntimePluginConfig(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	pluginName := fmt.Sprintf("config-normalize-update-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "Config Normalize Update",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{
		"enabled":          true,
		"secret_encrypted": "v1:old",
	}))

	mockPlugin := &configHookTestPlugin{lifecycleTestPlugin: lifecycleTestPlugin{name: pluginName}}
	registerPluginForHandlerTest(t, mockPlugin)

	body, err := json.Marshal(map[string]interface{}{
		"config": map[string]interface{}{
			"enabled": true,
			"secret":  "new-secret",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", pluginRecord.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil).UpdateBusinessPluginConfig(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, mockPlugin.normalizeCalls)
	assert.Equal(t, 1, mockPlugin.validateCalls)
	assert.Equal(t, 1, mockPlugin.initCalls)
	assert.Equal(t, true, mockPlugin.lastConfig["normalized"])
	assert.Equal(t, "v1:new-secret", mockPlugin.lastConfig["secret_encrypted"])
	assert.NotContains(t, mockPlugin.lastConfig, "secret")

	config, err := database.GetBusinessPluginConfig(business.ID, pluginName)
	require.NoError(t, err)
	assert.Equal(t, true, config["normalized"])
	assert.Equal(t, "v1:new-secret", config["secret_encrypted"])
	assert.NotContains(t, config, "secret")
}

func TestDisableBusinessPlugin_CallsRuntimeCleanup(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	pluginName := fmt.Sprintf("lifecycle-disable-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "Lifecycle Disable",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{
		"business_name": "Cleanup Me",
	}))

	mockPlugin := &lifecycleTestPlugin{name: pluginName}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", pluginRecord.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil).DisableBusinessPlugin(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, mockPlugin.cleanupCalls)
	assert.Equal(t, business.ID, mockPlugin.lastBusiness)
}

func TestEnableBusinessPlugin_SyncsDailyReportSchedule(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.ReportSchedule{}))

	business := createTestBusiness(t)
	pluginRecord := &database.Plugin{
		Name:        "daily_email_report",
		DisplayName: "Daily Email Report",
		Category:    database.PluginCategoryAnalytics,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)

	body, err := json.Marshal(map[string]interface{}{
		"config": map[string]interface{}{
			"enabled":  true,
			"hour":     7,
			"timezone": "America/New_York",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", pluginRecord.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	reportScheduler := services.NewReportScheduler(database.GetDBWrapper(), nil, nil)
	NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), reportScheduler).EnableBusinessPlugin(c)

	assert.Equal(t, http.StatusOK, w.Code)

	schedule, err := database.GetDBWrapper().GetReportScheduleByBusinessAndFrequency(business.ID, database.ReportFrequencyDaily)
	require.NoError(t, err)
	require.NotNil(t, schedule)
	assert.True(t, schedule.IsActive)
	assert.Equal(t, 7, schedule.Hour)
	assert.Equal(t, "America/New_York", schedule.Timezone)
}

func TestDisableBusinessPlugin_DisablesWeeklyReportSchedule(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.ReportSchedule{}))

	business := createTestBusiness(t)
	pluginRecord := &database.Plugin{
		Name:        "weekly_email_report",
		DisplayName: "Weekly Email Report",
		Category:    database.PluginCategoryAnalytics,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{
		"enabled":     true,
		"day_of_week": 1,
		"hour":        9,
		"timezone":    "UTC",
	}))

	reportScheduler := services.NewReportScheduler(database.GetDBWrapper(), nil, nil)
	require.NoError(t, reportScheduler.SyncWeeklySchedule(business.ID, map[string]interface{}{
		"enabled":     true,
		"day_of_week": 1,
		"hour":        9,
		"timezone":    "UTC",
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", pluginRecord.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), reportScheduler).DisableBusinessPlugin(c)

	assert.Equal(t, http.StatusOK, w.Code)

	schedule, err := database.GetDBWrapper().GetReportScheduleByBusinessAndFrequency(business.ID, database.ReportFrequencyWeekly)
	require.NoError(t, err)
	require.NotNil(t, schedule)
	assert.False(t, schedule.IsActive)
}

func TestGetBusinessPlugins_OmitsStoredConfig(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"secret_key": "sk_live_should_not_leak",
		"mode":       "live",
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Plugins, 1)
	_, hasConfig := resp.Plugins[0]["config"]
	assert.False(t, hasConfig)
}

// Issue #795: the dashboard advertised payment-readiness while every rail was
// off. The list endpoint must expose the enabled payment-rail count
// server-side so the frontend derives readiness from backend truth instead of
// hardcoding it.
func TestGetBusinessPlugins_ReportsEnabledPaymentRailCount(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	mkPlugin := func(name, category string) *database.Plugin {
		p := &database.Plugin{
			Name:        name,
			DisplayName: name,
			Category:    category,
			IsActive:    true,
		}
		require.NoError(t, database.GetDB().Create(p).Error)
		return p
	}
	mp := mkPlugin("mercadopago", database.PluginCategoryPayment)
	stripe := mkPlugin("stripe", database.PluginCategoryPayment)
	telegram := mkPlugin("telegram", "integration")

	// mercadopago enabled, stripe subscribed-but-disabled, telegram enabled
	// but not a payment rail.
	require.NoError(t, database.EnableBusinessPlugin(business.ID, mp.ID, map[string]interface{}{}))
	require.NoError(t, database.EnableBusinessPlugin(business.ID, stripe.ID, map[string]interface{}{}))
	require.NoError(t, database.DisableBusinessPlugin(business.ID, stripe.ID))
	require.NoError(t, database.EnableBusinessPlugin(business.ID, telegram.ID, map[string]interface{}{}))

	fetch := func() (int, bool) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		NewPluginHandlers(nil, nil).GetBusinessPlugins(c)
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			EnabledPaymentCount *int  `json:"enabled_payment_count"`
			PaymentsReady       *bool `json:"payments_ready"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.EnabledPaymentCount,
			"list response must carry enabled_payment_count (#795)")
		require.NotNil(t, resp.PaymentsReady,
			"list response must carry payments_ready (#795)")
		return *resp.EnabledPaymentCount, *resp.PaymentsReady
	}

	count, ready := fetch()
	assert.Equal(t, 1, count, "only mercadopago is an enabled payment rail")
	assert.True(t, ready)

	// Disable the last live rail: the endpoint must say 0 / not ready.
	require.NoError(t, database.DisableBusinessPlugin(business.ID, mp.ID))
	count, ready = fetch()
	assert.Equal(t, 0, count)
	assert.False(t, ready, "no enabled rail must never read as payments-ready (#795)")
}

func TestGetBusinessPlugins_PaymentRailsMercadoPagoFirst(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	for _, name := range []string{"usdc_payment", "mercadopago", "stripe"} {
		plugin := &database.Plugin{
			Name:        name,
			DisplayName: name,
			Category:    database.PluginCategoryPayment,
			IsActive:    true,
		}
		require.NoError(t, database.GetDB().Create(plugin).Error)
		require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{}))
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPlugins(c)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.GreaterOrEqual(t, len(resp.Plugins), 3)
	names := make([]string, 0, len(resp.Plugins))
	for _, p := range resp.Plugins {
		names = append(names, fmt.Sprint(p["name"]))
	}
	mp := -1
	crypto := -1
	for i, n := range names {
		if n == "mercadopago" {
			mp = i
		}
		if n == "usdc_payment" {
			crypto = i
		}
	}
	require.GreaterOrEqual(t, mp, 0)
	require.GreaterOrEqual(t, crypto, 0)
	assert.Less(t, mp, crypto, "Mercado Pago must precede crypto in the operator plugins API")
}

func TestGetBusinessPluginConfig_RequiresPluginConfigPermissionForStaff(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"secret_key": "sk_test_123",
	}))

	handler := NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleServer))
		c.Next()
	})
	router.GET("/inside/businesses/:id/plugins/:plugin_id/config", server.RoleBasedAccessMiddleware(string(server.PermPluginsConfig)), handler.GetBusinessPluginConfig)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/businesses/%d/plugins/%d/config", business.ID, plugin.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetBusinessPluginConfig_MasksSecretsForManager(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"client_secret": "paypal-secret",
	}))

	handler := NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Next()
	})
	router.GET("/inside/businesses/:id/plugins/:plugin_id/config", server.RoleBasedAccessMiddleware(string(server.PermPluginsConfig)), handler.GetBusinessPluginConfig)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/businesses/%d/plugins/%d/config", business.ID, plugin.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Config map[string]interface{} `json:"config"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// SEC-1: a Manager may load the config form but must NOT receive the raw
	// processor secret. paypal does not implement PublicConfigProvider, so the
	// fail-closed mask applies.
	assert.Equal(t, maskedSecretValue, resp.Config["client_secret"])
}

func TestUpdateBusinessPluginConfig_PreservesMaskedSecretOnSave(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"secret_key":      "sk_live_REAL",
		"publishable_key": "pk_live_x",
	}))

	handler := NewPluginHandlers(nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Next()
	})
	router.PUT("/inside/businesses/:id/plugins/:plugin_id/config", server.RoleBasedAccessMiddleware(string(server.PermPluginsConfig)), handler.UpdateBusinessPluginConfig)

	// The client round-trips the masked secret it was given on GET.
	body, err := json.Marshal(map[string]interface{}{"config": map[string]interface{}{
		"secret_key":      maskedSecretValue,
		"publishable_key": "pk_live_x",
	}})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/inside/businesses/%d/plugins/%d/config", business.ID, plugin.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	// The stored secret must be preserved, not clobbered by the sentinel.
	stored, err := database.GetBusinessPluginConfig(business.ID, "stripe")
	require.NoError(t, err)
	assert.Equal(t, "sk_live_REAL", stored["secret_key"])
	assert.Equal(t, "pk_live_x", stored["publishable_key"])
}

func TestGetBusinessPluginConfig_UsesPublicConfigProvider(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	pluginName := fmt.Sprintf("config-public-%d", time.Now().UnixNano())
	plugin := &database.Plugin{
		Name:        pluginName,
		DisplayName: "Public Config",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"enabled":          true,
		"secret_encrypted": "v1:hidden",
		"mode":             "live",
	}))

	mockPlugin := &configHookTestPlugin{lifecycleTestPlugin: lifecycleTestPlugin{name: pluginName}}
	registerPluginForHandlerTest(t, mockPlugin)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", plugin.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil).GetBusinessPluginConfig(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, mockPlugin.publicCalls)

	var resp struct {
		Config map[string]interface{} `json:"config"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "live", resp.Config["mode"])
	assert.Equal(t, true, resp.Config["secret_masked"])
	assert.NotContains(t, resp.Config, "secret_encrypted")
}

func TestGetBusinessPluginConfig_ReturnsNotFoundWhenPluginNotEnabled(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	handler := NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Next()
	})
	router.GET("/inside/businesses/:id/plugins/:plugin_id/config", server.RoleBasedAccessMiddleware(string(server.PermPluginsConfig)), handler.GetBusinessPluginConfig)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/businesses/%d/plugins/%d/config", business.ID, plugin.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreatePluginPayment_UsesOutstandingBillAmountAndTip(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	pluginName := fmt.Sprintf("testpay-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "TestPay",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-%d", time.Now().UnixNano()),
		Status:      "partial",
		Items:       "[]",
		TotalAmount: 5000,
		PaidAmount:  2000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	mockPlugin := &testPaymentPlugin{name: pluginName}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	body, err := json.Marshal(map[string]interface{}{
		"plugin_id":  pluginName,
		"amount":     1,
		"currency":   "JPY",
		"tip_amount": 3.5,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).CreatePluginPayment(c)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, bill.BusinessID, mockPlugin.lastBusinessID)
	assert.Equal(t, bill.ID, mockPlugin.lastBillID)
	assert.EqualValues(t, 3350, mockPlugin.lastAmount)
	assert.Equal(t, "USD", mockPlugin.lastCurrency)
	// Public metadata values are in DOLLARS (matching the JSON wire contract);
	// *_cents keys expose the authoritative integer-cent values.
	assert.Equal(t, 3.5, mockPlugin.lastMetadata["tip_amount"])
	assert.Equal(t, 33.5, mockPlugin.lastMetadata["amount"])
	assert.Equal(t, int64(3350), mockPlugin.lastMetadata["amount_cents"])
	assert.Equal(t, int64(3000), mockPlugin.lastMetadata["bill_amount_cents"])
}

func TestCreatePluginPayment_UsesHeldSplitShareAmountWhenProvided(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}, &database.BillSplitShare{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	pluginName := fmt.Sprintf("splitpay-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "SplitPay",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-SPLIT-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 10000,
		PaidAmount:  0,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	now := time.Now().UTC()
	share, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    2500,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.NotNil(t, share.HoldExpiresAt)
	originalExpiry := *share.HoldExpiresAt

	mockPlugin := &testPaymentPlugin{name: pluginName}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	body, err := json.Marshal(map[string]interface{}{
		"plugin_id":  pluginName,
		"amount":     100,
		"currency":   "USD",
		"tip_amount": 1.25,
		"metadata": map[string]interface{}{
			"split_share_id": share.ID,
		},
	})
	require.NoError(t, err)

	router := gin.New()
	router.POST("/guest/bill/:bill_token/plugin-payment", NewPluginHandlers(nil, nil).CreatePluginPayment)
	req := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/plugin-payment", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{
		Name:  guestSplitSessionCookie,
		Value: "guest-1." + guestsession.Sign("guest-1"),
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.EqualValues(t, 2625, mockPlugin.lastAmount)
	assert.Equal(t, int64(2500), mockPlugin.lastMetadata["bill_amount_cents"])
	assert.Equal(t, int64(125), mockPlugin.lastMetadata["tip_amount_cents"])
	assert.Equal(t, share.ID, mockPlugin.lastMetadata["split_share_id"])
	var refreshedShare database.BillSplitShare
	require.NoError(t, database.GetDB().Select("hold_expires_at").First(&refreshedShare, share.ID).Error)
	require.NotNil(t, refreshedShare.HoldExpiresAt)
	assert.True(t, refreshedShare.HoldExpiresAt.After(originalExpiry.Add(5*time.Minute)))

	var tracker database.AlternativePayment
	require.NoError(t, database.GetDB().
		Where("bill_id = ? AND participant_addr = ?", bill.ID, "test-payment-id").
		First(&tracker).Error)
	assert.Contains(t, tracker.ParticipantName, fmt.Sprintf("split_share_id=%d", share.ID))
}

func TestCreatePluginPayment_AttachesAlternativePaymentForTracker(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	pluginRecord := &database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"enabled": true}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-td-attach-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	mockPlugin := &detailedTestPaymentPlugin{testPaymentPlugin: testPaymentPlugin{name: "paypal"}}
	registerPluginForHandlerTest(t, mockPlugin)

	body, err := json.Marshal(map[string]interface{}{
		"plugin_id":  "paypal",
		"amount":     25.00,
		"currency":   "USD",
		"tip_amount": 3.00,
		"metadata": map[string]interface{}{
			"provider_network": "tron",
			"provider_asset":   "USDT",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).CreatePluginPayment(c)

	require.Equal(t, http.StatusCreated, w.Code)
	require.NotZero(t, mockPlugin.attachedAlternativePaymentID)
	assert.Equal(t, business.ID, mockPlugin.attachedBusinessID)
	assert.Equal(t, "test-payment-id", mockPlugin.attachedPaymentID)

	var tracked database.AlternativePayment
	require.NoError(t, database.GetDB().First(&tracked, mockPlugin.attachedAlternativePaymentID).Error)
	assert.Equal(t, bill.ID, tracked.BillID)
	assert.Equal(t, int64(2800), tracked.Amount)
	assert.Equal(t, database.AlternativePaymentMethod("paypal"), tracked.PaymentMethod)
}

func TestCreatePluginPayment_StoresProviderGrossAmountForTracker(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	pluginRecord := &database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"enabled": true}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-td-gross-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	mockPlugin := &detailedTestPaymentPlugin{testPaymentPlugin: testPaymentPlugin{
		name: "paypal",
		paymentResponse: &plugins.PaymentResponse{
			PaymentID: "td-gross-payment",
			Status:    "awaiting_payment",
			Metadata: map[string]interface{}{
				"amount":       "28.12",
				"amount_cents": int64(2812),
				"fee_breakdown": map[string]interface{}{
					"bill_amount_cents": int64(2500),
					"gross_due_cents":   int64(2812),
				},
			},
		},
	}}
	registerPluginForHandlerTest(t, mockPlugin)

	body, err := json.Marshal(map[string]interface{}{
		"plugin_id":  "paypal",
		"amount":     25.00,
		"currency":   "USD",
		"tip_amount": 0,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).CreatePluginPayment(c)

	require.Equal(t, http.StatusCreated, w.Code)
	var tracked database.AlternativePayment
	require.NoError(t, database.GetDB().
		Where("bill_id = ? AND participant_addr = ?", bill.ID, "td-gross-payment").
		First(&tracked).Error)
	assert.Equal(t, int64(2812), tracked.Amount)
	assert.Equal(t, tracked.ID, mockPlugin.attachedAlternativePaymentID)

	var resp struct {
		Metadata map[string]interface{} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "28.12", resp.Metadata["amount"])
	assert.Equal(t, float64(2812), resp.Metadata["amount_cents"])
	assert.Equal(t, float64(2500), resp.Metadata["bill_amount_cents"])
}

func TestCreatePluginPayment_RejectsUnsafeTrackerGrossAmountMetadata(t *testing.T) {
	cases := []struct {
		name        string
		metadata    map[string]interface{}
		wantMessage string
	}{
		{
			name:        "lower_than_authoritative",
			metadata:    map[string]interface{}{"amount_cents": int64(1)},
			wantMessage: "Payment processing failed",
		},
		{
			name:        "fractional_cents",
			metadata:    map[string]interface{}{"amount_cents": 28.12},
			wantMessage: "Payment processing failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupHandlerTestDB(t)
			require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

			business := createTestBusiness(t)
			require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
			pluginRecord := &database.Plugin{
				Name:        "paypal",
				DisplayName: "PayPal",
				Category:    database.PluginCategoryPayment,
				IsActive:    true,
			}
			require.NoError(t, database.GetDB().Create(pluginRecord).Error)
			require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"enabled": true}))

			bill := &database.Bill{
				BusinessID:  business.ID,
				BillNumber:  fmt.Sprintf("B-td-unsafe-gross-%s-%d", tc.name, time.Now().UnixNano()),
				Status:      database.BillStatusOpen,
				Items:       "[]",
				TotalAmount: 2500,
			}
			require.NoError(t, database.GetDB().Create(bill).Error)

			mockPlugin := &detailedTestPaymentPlugin{testPaymentPlugin: testPaymentPlugin{
				name: "paypal",
				paymentResponse: &plugins.PaymentResponse{
					PaymentID: "td-unsafe-gross",
					Status:    "awaiting_payment",
					Metadata:  tc.metadata,
				},
			}}
			registerPluginForHandlerTest(t, mockPlugin)

			body, err := json.Marshal(map[string]interface{}{
				"plugin_id": "paypal",
				"amount":    25.00,
				"currency":  "USD",
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
			c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			NewPluginHandlers(nil, nil).CreatePluginPayment(c)

			require.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Contains(t, w.Body.String(), tc.wantMessage)
			var count int64
			require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
				Where("bill_id = ?", bill.ID).
				Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestCreatePluginPayment_MarksTrackedPaymentFailedWhenTrackerAttachFails(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	pluginRecord := &database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"enabled": true}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-td-attach-fail-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	mockPlugin := &detailedTestPaymentPlugin{
		testPaymentPlugin: testPaymentPlugin{
			name: "paypal",
			paymentResponse: &plugins.PaymentResponse{
				PaymentID: "td-attach-fail",
				Status:    "awaiting_payment",
				Metadata:  map[string]interface{}{"amount_cents": int64(2500)},
			},
		},
		attachErr: errors.New("attach failed"),
	}
	registerPluginForHandlerTest(t, mockPlugin)

	body, err := json.Marshal(map[string]interface{}{
		"plugin_id": "paypal",
		"amount":    25.00,
		"currency":  "USD",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).CreatePluginPayment(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)

	var tracked database.AlternativePayment
	require.NoError(t, database.GetDB().
		Where("bill_id = ? AND participant_addr = ?", bill.ID, "td-attach-fail").
		First(&tracked).Error)
	assert.Equal(t, database.AltPaymentStatusFailed, tracked.Status)
}

func TestStorePluginPaymentRecordReusesExistingPendingProviderIntent(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}))
	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	existing := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "tdpi_existing",
		ParticipantName: "paypal",
		Amount:          2812,
		PaymentMethod:   database.AlternativePaymentMethod("paypal"),
		Status:          database.AltPaymentStatusPending,
	}
	require.NoError(t, database.GetDB().Create(existing).Error)

	tracked, err := NewPluginHandlers(nil, nil).storePluginPaymentRecord(
		bill.ID,
		business.ID,
		"paypal",
		"tdpi_existing",
		2812,
		"USD",
		2812,
		0,
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, existing.ID, tracked.ID)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Where("bill_id = ? AND participant_addr = ?", bill.ID, "tdpi_existing").
		Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestCreatePluginPayment_RejectsInvalidRedirectURLsBeforeProviderCall(t *testing.T) {
	cases := []struct {
		name       string
		field      string
		value      string
		wantBody   string
		otherField string
		otherValue string
	}{
		{
			name:       "return_url",
			field:      "return_url",
			value:      "https://evil.example/payment/success",
			wantBody:   "Invalid return URL",
			otherField: "cancel_url",
			otherValue: "/payment/cancelled",
		},
		{
			name:       "cancel_url",
			field:      "cancel_url",
			value:      "https://evil.example/payment/cancelled",
			wantBody:   "Invalid cancel URL",
			otherField: "return_url",
			otherValue: "/payment/success",
		},
		{
			name:       "backslash_relative_return_url",
			field:      "return_url",
			value:      `/\evil.example/path`,
			wantBody:   "Invalid return URL",
			otherField: "cancel_url",
			otherValue: "/payment/cancelled",
		},
		{
			name:       "slash_backslash_relative_cancel_url",
			field:      "cancel_url",
			value:      `/\/evil.example`,
			wantBody:   "Invalid cancel URL",
			otherField: "return_url",
			otherValue: "/payment/success",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ALLOWED_REDIRECT_DOMAINS", "payverge.io,www.payverge.io")
			setupHandlerTestDB(t)
			require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

			business := createTestBusiness(t)
			require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
			table := &database.Table{
				BusinessID: business.ID,
				TableCode:  fmt.Sprintf("TR-%d", time.Now().UnixNano()),
				Name:       "Redirect Test",
			}
			require.NoError(t, database.GetDB().Create(table).Error)

			pluginName := fmt.Sprintf("testpay-redirect-%s-%d", tc.name, time.Now().UnixNano())
			pluginRecord := &database.Plugin{
				Name:        pluginName,
				DisplayName: "TestPay",
				Category:    database.PluginCategoryPayment,
				IsActive:    true,
			}
			require.NoError(t, database.GetDB().Create(pluginRecord).Error)
			require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

			bill := &database.Bill{
				BusinessID:  business.ID,
				TableID:     table.ID,
				BillNumber:  fmt.Sprintf("B-redirect-%s-%d", tc.name, time.Now().UnixNano()),
				Status:      database.BillStatusOpen,
				Items:       "[]",
				TotalAmount: 2500,
			}
			require.NoError(t, database.GetDB().Create(bill).Error)

			mockPlugin := &testPaymentPlugin{name: pluginName}
			plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

			originalSupport := guestBillPaymentPluginSupported
			defer func() {
				guestBillPaymentPluginSupported = originalSupport
			}()
			guestBillPaymentPluginSupported = func(name string) bool {
				return name == pluginName
			}

			bodyMap := map[string]interface{}{
				"plugin_id":   pluginName,
				"amount":      25,
				"currency":    "USD",
				"tip_amount":  0,
				tc.field:      tc.value,
				tc.otherField: tc.otherValue,
			}
			body, err := json.Marshal(bodyMap)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
			c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			NewPluginHandlers(nil, nil).CreatePluginPayment(c)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), tc.wantBody)
			assert.Zero(t, mockPlugin.lastBillID)

			var trackingCount int64
			require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).Where("bill_id = ?", bill.ID).Count(&trackingCount).Error)
			assert.Zero(t, trackingCount)
		})
	}
}

func TestCreatePluginPayment_RejectsLocalhostDefaultRedirectsInProduction(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("PUBLIC_URL", "")
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  fmt.Sprintf("TP-%d", time.Now().UnixNano()),
		Name:       "Production Redirect",
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	pluginName := fmt.Sprintf("testpay-prod-redirect-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "TestPay",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  fmt.Sprintf("B-prod-redirect-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	mockPlugin := &testPaymentPlugin{name: pluginName}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	body, err := json.Marshal(map[string]interface{}{
		"plugin_id": pluginName,
		"amount":    25,
		"currency":  "USD",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).CreatePluginPayment(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid return URL")
	assert.Zero(t, mockPlugin.lastBillID)

	var trackingCount int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).Where("bill_id = ?", bill.ID).Count(&trackingCount).Error)
	assert.Zero(t, trackingCount)
}

func TestCreatePluginPayment_RejectsSubCentNegativeTipWithoutTrackingPayment(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	pluginName := fmt.Sprintf("testpay-neg-tip-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "TestPay",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-neg-tip-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	mockPlugin := &testPaymentPlugin{name: pluginName}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	body, err := json.Marshal(map[string]interface{}{
		"plugin_id":  pluginName,
		"amount":     25,
		"currency":   "USD",
		"tip_amount": -0.004,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).CreatePluginPayment(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "tip amount cannot be negative")
	assert.Zero(t, mockPlugin.lastBillID)

	var trackingCount int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).Where("bill_id = ?", bill.ID).Count(&trackingCount).Error)
	assert.Zero(t, trackingCount)
}

func TestCreatePluginPayment_RejectsTipOnlyCheckoutForNonPayableBills(t *testing.T) {
	cases := []struct {
		name       string
		status     database.BillStatus
		total      int64
		paid       int64
		wantBody   string
		wantStatus int
	}{
		{
			name:       "paid",
			status:     database.BillStatusPaid,
			total:      2500,
			paid:       2500,
			wantBody:   "bill is not open",
			wantStatus: http.StatusConflict,
		},
		{
			name:       "zero_outstanding",
			status:     database.BillStatusPartial,
			total:      2500,
			paid:       2500,
			wantBody:   "bill is already fully paid",
			wantStatus: http.StatusConflict,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupHandlerTestDB(t)
			require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

			business := createTestBusiness(t)
			require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
			pluginName := fmt.Sprintf("testpay-nonpayable-%s-%d", tc.name, time.Now().UnixNano())
			pluginRecord := &database.Plugin{
				Name:        pluginName,
				DisplayName: "TestPay",
				Category:    database.PluginCategoryPayment,
				IsActive:    true,
			}
			require.NoError(t, database.GetDB().Create(pluginRecord).Error)
			require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

			bill := &database.Bill{
				BusinessID:  business.ID,
				BillNumber:  fmt.Sprintf("B-nonpayable-%s-%d", tc.name, time.Now().UnixNano()),
				Status:      tc.status,
				Items:       "[]",
				TotalAmount: tc.total,
				PaidAmount:  tc.paid,
			}
			require.NoError(t, database.GetDB().Create(bill).Error)

			mockPlugin := &testPaymentPlugin{name: pluginName}
			plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

			originalSupport := guestBillPaymentPluginSupported
			defer func() {
				guestBillPaymentPluginSupported = originalSupport
			}()
			guestBillPaymentPluginSupported = func(name string) bool {
				return name == pluginName
			}

			body, err := json.Marshal(map[string]interface{}{
				"plugin_id":  pluginName,
				"amount":     25,
				"currency":   "USD",
				"tip_amount": 2.5,
			})
			require.NoError(t, err)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
			c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			NewPluginHandlers(nil, nil).CreatePluginPayment(c)

			assert.Equal(t, tc.wantStatus, w.Code)
			assert.Contains(t, w.Body.String(), tc.wantBody)
			var response server.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, server.ErrCodeConflict, response.Code)
			assert.Zero(t, mockPlugin.lastBillID)

			var trackingCount int64
			require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).Where("bill_id = ?", bill.ID).Count(&trackingCount).Error)
			assert.Zero(t, trackingCount)
		})
	}
}

func TestCreatePluginPayment_UsesOpaqueBillNumberRouteParam(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)

	pluginName := fmt.Sprintf("testpay-route-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "TestPay",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 25,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	mockPlugin := &testPaymentPlugin{name: pluginName}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	body, err := json.Marshal(map[string]interface{}{
		"plugin_id": pluginName,
		"amount":    25,
		"currency":  "USD",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).CreatePluginPayment(c)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, bill.ID, mockPlugin.lastBillID)
	assert.Equal(t, business.ID, mockPlugin.lastBusinessID)

	var payments []database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ?", bill.ID).Find(&payments).Error)
	require.Len(t, payments, 1)
	assert.Equal(t, database.AltPaymentStatusPending, payments[0].Status)
	assert.Equal(t, pluginName, string(payments[0].PaymentMethod))
	assert.Nil(t, payments[0].ConfirmedAt)
}

func TestGetPluginPaymentStatus_UsesOpaqueBillNumberRouteParam(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	pluginName := fmt.Sprintf("teststatus-route-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "TestStatus",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 25,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "pay_123",
		ParticipantName: pluginName,
		Amount:          25,
		PaymentMethod:   database.AlternativePaymentMethod(pluginName),
		Status:          database.AltPaymentStatusConfirmed,
	}).Error)

	mockPlugin := &testPaymentPlugin{
		name:   pluginName,
		status: "paid",
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "bill_token", Value: bill.PublicToken},
		{Key: "payment_id", Value: "pay_123"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/?plugin="+pluginName, nil)

	NewPluginHandlers(nil, nil).GetPluginPaymentStatus(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"paid","metadata":null}`, w.Body.String())
	assert.Equal(t, business.ID, mockPlugin.lastStatusBizID)
	assert.Equal(t, "pay_123", mockPlugin.lastStatusID)
}

func TestGetPluginPaymentStatus_UsesDetailedProvider(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	pluginRecord := &database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"enabled": true}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-td-status-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2800,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "td-status-payment",
		ParticipantName: "paypal",
		Amount:          2800,
		PaymentMethod:   database.AlternativePaymentMethod("paypal"),
		Status:          database.AltPaymentStatusPending,
	}).Error)

	mockPlugin := &detailedTestPaymentPlugin{
		testPaymentPlugin: testPaymentPlugin{name: "paypal", status: "paid"},
		details: &plugins.PaymentStatusDetails{
			Status: "awaiting_payment",
			Metadata: map[string]interface{}{
				"address": "TCsHYKC27np7cGAxJEq55DnsGysejpFF11",
				"network": "tron",
				"asset":   "USDT",
				"amount":  "28.00",
			},
		},
	}
	registerPluginForHandlerTest(t, mockPlugin)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "bill_token", Value: bill.PublicToken},
		{Key: "payment_id", Value: "td-status-payment"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/?plugin=paypal", nil)

	NewPluginHandlers(nil, nil).GetPluginPaymentStatus(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, mockPlugin.detailCalls)
	assert.Equal(t, 0, mockPlugin.statusCalls)
	assert.Equal(t, business.ID, mockPlugin.lastStatusBizID)
	assert.Equal(t, "td-status-payment", mockPlugin.lastStatusID)
	assert.JSONEq(t, `{"status":"awaiting_payment","metadata":{"address":"TCsHYKC27np7cGAxJEq55DnsGysejpFF11","network":"tron","asset":"USDT","amount":"28.00"}}`, w.Body.String())
}

func TestGetPluginPaymentStatus_DetailedProviderReturnsProcessingUntilLocalSettlementRecorded(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	pluginRecord := &database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"enabled": true}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-td-status-processing-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2800,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "td-status-processing",
		ParticipantName: "paypal",
		Amount:          2800,
		PaymentMethod:   database.AlternativePaymentMethod("paypal"),
		Status:          database.AltPaymentStatusPending,
	}).Error)

	mockPlugin := &detailedTestPaymentPlugin{
		testPaymentPlugin: testPaymentPlugin{name: "paypal"},
		details: &plugins.PaymentStatusDetails{
			Status: "paid",
			Metadata: map[string]interface{}{
				"amount": "28.00",
			},
		},
	}
	registerPluginForHandlerTest(t, mockPlugin)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "bill_token", Value: bill.PublicToken},
		{Key: "payment_id", Value: "td-status-processing"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/?plugin=paypal", nil)

	NewPluginHandlers(nil, nil).GetPluginPaymentStatus(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, mockPlugin.detailCalls)
	assert.JSONEq(t, `{"status":"processing","metadata":{"amount":28,"awaiting_settlement":true}}`, w.Body.String())
}

func TestGetPluginPaymentStatus_IncludesRecordedPaymentAmountsWhenSettled(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}, &database.Payment{}))

	business := createTestBusiness(t)
	pluginName := fmt.Sprintf("teststatus-settled-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "TestStatus",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-settled-%d", time.Now().UnixNano()),
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 25,
		PaidAmount:  25,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	splitShareID := uint(42)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "pay_settled",
		ParticipantName: pluginTrackerParticipantName(pluginName, &splitShareID),
		Amount:          25,
		PaymentMethod:   database.AlternativePaymentMethod(pluginName),
		Status:          database.AltPaymentStatusPending,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "plugin",
		Amount:        20,
		TipAmount:     5,
		TxHash:        "plugin_pay_settled",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "usd",
	}).Error)

	mockPlugin := &testPaymentPlugin{
		name:   pluginName,
		status: "paid",
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "bill_token", Value: bill.PublicToken},
		{Key: "payment_id", Value: "pay_settled"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/?plugin="+pluginName, nil)

	NewPluginHandlers(nil, nil).GetPluginPaymentStatus(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"paid","metadata":{"amount":0.2,"tip_amount":0.05,"total_amount":0.25,"split_share_id":42}}`, w.Body.String())
	assert.Equal(t, business.ID, mockPlugin.lastStatusBizID)
	assert.Equal(t, "pay_settled", mockPlugin.lastStatusID)
}

func TestGetPluginPaymentStatus_ReturnsProcessingUntilLocalSettlementIsRecorded(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	pluginName := fmt.Sprintf("teststatus-pending-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "TestStatus",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-pending-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 25,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "pay_pending",
		ParticipantName: pluginName,
		Amount:          25,
		PaymentMethod:   database.AlternativePaymentMethod(pluginName),
		Status:          database.AltPaymentStatusPending,
	}).Error)

	mockPlugin := &testPaymentPlugin{
		name:   pluginName,
		status: "paid",
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "bill_token", Value: bill.PublicToken},
		{Key: "payment_id", Value: "pay_pending"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/?plugin="+pluginName, nil)

	NewPluginHandlers(nil, nil).GetPluginPaymentStatus(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"processing","metadata":{"amount":0.25,"awaiting_settlement":true}}`, w.Body.String())
	assert.Equal(t, business.ID, mockPlugin.lastStatusBizID)
	assert.Equal(t, "pay_pending", mockPlugin.lastStatusID)
}

// Regression: a plugin whose GetPaymentStatus always errors (Stripe Checkout has
// no queryable status and settles only via webhook) must NOT turn every guest
// status poll into a 500 "Payment processing failed" — that surfaced as
// "Payment failed" for a card payment that in fact succeeded. With no local
// settlement yet, the endpoint reports the payment as still pending so the poller
// keeps going until the webhook lands.
func TestGetPluginPaymentStatus_UnqueryablePluginReportsPendingNotError(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	pluginName := fmt.Sprintf("teststatus-webhookonly-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "TestStatus",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-webhookonly-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 25,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "pay_webhookonly",
		ParticipantName: pluginName,
		Amount:          25,
		PaymentMethod:   database.AlternativePaymentMethod(pluginName),
		Status:          database.AltPaymentStatusPending,
	}).Error)

	mockPlugin := &testPaymentPlugin{
		name:      pluginName,
		statusErr: errors.New("generic GetPaymentStatus is unsupported; rely on webhooks"),
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() { guestBillPaymentPluginSupported = originalSupport }()
	guestBillPaymentPluginSupported = func(name string) bool { return name == pluginName }

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "bill_token", Value: bill.PublicToken},
		{Key: "payment_id", Value: "pay_webhookonly"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/?plugin="+pluginName, nil)

	NewPluginHandlers(nil, nil).GetPluginPaymentStatus(c)

	assert.Equal(t, http.StatusOK, w.Code, "must not 500 when the plugin cannot report status")
	assert.JSONEq(t, `{"status":"pending","metadata":{"amount":0.25,"awaiting_settlement":true}}`, w.Body.String())
}

func TestGetPluginPaymentStatus_RejectsPaymentIDsFromOtherBills(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	pluginName := fmt.Sprintf("teststatus-mismatch-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "TestStatus",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-main-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 25,
	}
	otherBill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-other-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 12,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NoError(t, database.GetDB().Create(otherBill).Error)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          otherBill.ID,
		ParticipantAddr: "pay_other_bill",
		ParticipantName: pluginName,
		Amount:          12,
		PaymentMethod:   database.AlternativePaymentMethod(pluginName),
		Status:          database.AltPaymentStatusConfirmed,
	}).Error)

	mockPlugin := &testPaymentPlugin{
		name:   pluginName,
		status: "paid",
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "bill_token", Value: bill.PublicToken},
		{Key: "payment_id", Value: "pay_other_bill"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/?plugin="+pluginName, nil)

	NewPluginHandlers(nil, nil).GetPluginPaymentStatus(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Payment not found")
	assert.Empty(t, mockPlugin.lastStatusID)
}

func TestCreatePluginPayment_FailsWhenPaymentTrackingCannotBeStored(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)

	pluginName := fmt.Sprintf("testpay-track-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "TestPay",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{"mode": "test"}))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-track-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 25,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	mockPlugin := &testPaymentPlugin{name: pluginName}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	body, err := json.Marshal(map[string]interface{}{
		"plugin_id": pluginName,
		"amount":    25,
		"currency":  "USD",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).CreatePluginPayment(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Payment processing failed")
}

func TestUpdateBillPaymentStatus_MarksTrackedPluginPaymentConfirmed(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.AlternativePayment{},
		&database.Payment{},
		&database.Bill{},
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
		&database.Plugin{},
		&database.BusinessPlugin{},
	))

	business := createTestBusiness(t)
	// The Telegram enqueue now passes the ShouldEnqueueTelegramNotification
	// eligibility gate — connect Telegram for this business so the delivery
	// row assertion below exercises the eligible path.
	telegramPlugin := database.Plugin{Name: "telegram", DisplayName: "Telegram", IsActive: true, Category: "integration"}
	require.NoError(t, database.GetDB().Where("name = ?", "telegram").FirstOrCreate(&telegramPlugin, database.Plugin{Name: "telegram"}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   telegramPlugin.ID,
		IsEnabled:  true,
		Config:     `{"is_connected":true,"chat_id":"12345"}`,
	}).Error)
	services.ResetTelegramNotificationEligibilityCache()
	t.Cleanup(services.ResetTelegramNotificationEligibilityCache)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-settle-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	tracked := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "pay_confirm",
		ParticipantName: "paypal",
		Amount:          2500,
		PaymentMethod:   database.AlternativePaymentMethod("paypal"),
		Status:          database.AltPaymentStatusPending,
	}
	require.NoError(t, database.GetDB().Create(tracked).Error)

	handler := NewPluginHandlers(nil, nil)
	_, _, err := handler.updateBillPaymentStatus(bill.ID, "pay_confirm", 2500, 0, "USD", "stripe", nil)
	require.NoError(t, err)

	var refreshed database.AlternativePayment
	require.NoError(t, database.GetDB().First(&refreshed, tracked.ID).Error)
	assert.Equal(t, database.AltPaymentStatusConfirmed, refreshed.Status)
	assert.Equal(t, "plugin_webhook", refreshed.ConfirmedBy)
	assert.NotNil(t, refreshed.ConfirmedAt)

	var delivery database.PluginNotificationDelivery
	require.NoError(t, database.GetDB().Where("business_id = ? AND plugin_name = ? AND event_type = ?", business.ID, "telegram", services.PluginEventPaymentReceived).First(&delivery).Error)
	assert.Equal(t, "payment:pay_confirm", delivery.EventID)
	assert.Equal(t, bill.ID, uint(delivery.Payload["bill_id"].(float64)))
	assert.Equal(t, bill.BillNumber, delivery.Payload["bill_number"])
	assert.Equal(t, float64(2500), delivery.Payload["amount_cents"])
}

func TestHandlePayPalReturn_RedirectsToGuestBillRoute(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Table{}, &database.Bill{}))

	business := createTestBusiness(t)
	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  fmt.Sprintf("TT-%d", time.Now().UnixNano()),
		Name:       "Patio",
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	bill := &database.Bill{
		BusinessID: business.ID,
		TableID:    table.ID,
		BillNumber: fmt.Sprintf("B-paypal-%d", time.Now().UnixNano()),
		Status:     database.BillStatusOpen,
		Items:      "[]",
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/paypal/return?bill_id=%d&token=order-token-1&PayerID=payer-1", bill.ID),
		nil,
	)

	NewPluginHandlers(nil, nil).HandlePayPalReturn(c)

	assert.Equal(t, http.StatusFound, w.Code)
	location, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("/t/%s/bill", table.TableCode), location.Path)
	assert.Equal(t, "success", location.Query().Get("payment"))
	assert.Equal(t, "paypal", location.Query().Get("method"))
	assert.Equal(t, "order-token-1", location.Query().Get("payment_id"))
	assert.Equal(t, "paypal", location.Query().Get("payment_method"))
	assert.Equal(t, bill.BillNumber, location.Query().Get("bill_number"))
}

func TestHandlePayPalReturn_CapturesOrderAndSettlesBill(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.Table{}, &database.Bill{}, &database.Payment{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  fmt.Sprintf("TR-%d", time.Now().UnixNano()),
		Name:       "PayPal Return",
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	bill := &database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  fmt.Sprintf("B-paypal-capture-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	pluginRecord := &database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{
		"client_id":     "client-id-12345678901234567890",
		"client_secret": "client-secret-12345678901234567890",
		"webhook_id":    "WH123456789",
	}))

	// Seed the pending PayPal tracker that hasPendingPayPalTracker requires before
	// making an outbound capture call. This mirrors what CreatePluginPayment writes
	// when the guest initiates the payment — the tracker proves this order was
	// legitimately started by this server.
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORDER-1",
		ParticipantName: "paypal",
		Amount:          2500,
		PaymentMethod:   database.AlternativePaymentMethod("paypal"),
		Status:          database.AltPaymentStatusPending,
	}).Error)

	originalPlugin, hadOriginal := plugins.GlobalRegistry.GetPlugin("paypal")
	mockPlugin := &testPaymentPlugin{
		name: "paypal",
		returnResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: "capture-return-1",
			BillID:    bill.ID,
			Amount:    2500,
			Currency:  "USD",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)
	defer func() {
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(originalPlugin)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin("paypal")
		}
	}()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/paypal/return?bill_id=%d&token=ORDER-1&PayerID=payer-1", bill.ID),
		nil,
	)

	NewPluginHandlers(nil, nil).HandlePayPalReturn(c)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, business.ID, mockPlugin.lastBusinessID)
	assert.Equal(t, bill.ID, mockPlugin.lastBillID)
	assert.Equal(t, "ORDER-1", mockPlugin.lastReturnOrder)

	location, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "success", location.Query().Get("payment"))
	assert.Equal(t, "capture-return-1", location.Query().Get("payment_id"))

	var refreshed database.Bill
	require.NoError(t, database.GetDB().First(&refreshed, bill.ID).Error)
	assert.Equal(t, database.BillStatusPaid, refreshed.Status)

	payment, err := database.GetPaymentByTxHash("plugin_capture-return-1")
	require.NoError(t, err)
	assert.Equal(t, bill.ID, payment.BillID)
	assert.EqualValues(t, 2500, payment.Amount)
}

func TestHandlePayPalReturn_SettlesTrackedSplitShareAfterCaptureIDSwap(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.Table{}, &database.Bill{}, &database.Payment{}, &database.AlternativePayment{}, &database.BillSplitShare{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  fmt.Sprintf("TS-%d", time.Now().UnixNano()),
		Name:       "PayPal Split Return",
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	bill := &database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  fmt.Sprintf("B-paypal-split-capture-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	share, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-paypal",
		DisplayName:    "Sara",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    2500,
		HoldTTL:        5 * time.Minute,
		Now:            time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORDER-SPLIT-1",
		ParticipantName: pluginTrackerParticipantName("paypal", &share.ID),
		Amount:          2500,
		PaymentMethod:   database.AlternativePaymentMethod("paypal"),
		Status:          database.AltPaymentStatusPending,
	}).Error)

	pluginRecord := &database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{
		"client_id":     "client-id-12345678901234567890",
		"client_secret": "client-secret-12345678901234567890",
		"webhook_id":    "WH123456789",
	}))

	originalPlugin, hadOriginal := plugins.GlobalRegistry.GetPlugin("paypal")
	mockPlugin := &testPaymentPlugin{
		name: "paypal",
		returnResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: "CAPTURE-SPLIT-1",
			BillID:    bill.ID,
			Amount:    2500,
			Currency:  "USD",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)
	defer func() {
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(originalPlugin)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin("paypal")
		}
	}()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/paypal/return?bill_id=%d&token=ORDER-SPLIT-1&PayerID=payer-1", bill.ID),
		nil,
	)

	NewPluginHandlers(nil, nil).HandlePayPalReturn(c)

	assert.Equal(t, http.StatusFound, w.Code)
	payment, err := database.GetPaymentByTxHash("plugin_CAPTURE-SPLIT-1")
	require.NoError(t, err)

	var refreshedShare database.BillSplitShare
	require.NoError(t, database.GetDB().First(&refreshedShare, share.ID).Error)
	require.Equal(t, database.BillSplitShareStatusSettled, refreshedShare.Status)
	require.NotNil(t, refreshedShare.PaymentID)
	require.Equal(t, payment.ID, *refreshedShare.PaymentID)
	require.Nil(t, refreshedShare.HoldExpiresAt)

	var tracker database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ? AND participant_addr = ?", bill.ID, "CAPTURE-SPLIT-1").First(&tracker).Error)
	require.Equal(t, database.AltPaymentStatusConfirmed, tracker.Status)
	require.Equal(t, pluginTrackerParticipantName("paypal", &share.ID), tracker.ParticipantName)
}

func TestHandleMercadoPagoWebhook_SettlesTrackedSplitShareAfterTrackerIDSwap(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.Table{}, &database.Bill{}, &database.Payment{}, &database.AlternativePayment{}, &database.BillSplitShare{}))

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  fmt.Sprintf("TS-MP-%d", time.Now().UnixNano()),
		Name:       "MercadoPago Split Webhook",
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	bill := &database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  fmt.Sprintf("B-mercadopago-split-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	share, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-mp",
		DisplayName:    "Sara",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    2500,
		HoldTTL:        5 * time.Minute,
		Now:            time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "mp_tracker_123",
		ParticipantName: pluginTrackerParticipantName("mercadopago", &share.ID),
		Amount:          2500,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
	}).Error)

	mockPlugin := &testPaymentPlugin{
		name:            "mercadopago",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: "MP-PAYMENT-SPLIT-1",
			BillID:    bill.ID,
			Amount:    2500,
			Currency:  "USD",
			Metadata: map[string]interface{}{
				"payverge_tracker_id": "mp_tracker_123",
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/?bill_id=%d&business_id=%d", bill.ID, business.ID),
		bytes.NewBufferString(`{"id":"evt-mp-split-1","action":"payment.updated","data":{"id":"MP-PAYMENT-SPLIT-1"}}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	payment, err := database.GetPaymentByTxHash("plugin_MP-PAYMENT-SPLIT-1")
	require.NoError(t, err)

	var refreshedShare database.BillSplitShare
	require.NoError(t, database.GetDB().First(&refreshedShare, share.ID).Error)
	require.Equal(t, database.BillSplitShareStatusSettled, refreshedShare.Status)
	require.NotNil(t, refreshedShare.PaymentID)
	require.Equal(t, payment.ID, *refreshedShare.PaymentID)
	require.Nil(t, refreshedShare.HoldExpiresAt)

	var tracker database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ? AND participant_addr = ?", bill.ID, "MP-PAYMENT-SPLIT-1").First(&tracker).Error)
	require.Equal(t, database.AltPaymentStatusConfirmed, tracker.Status)
	require.Contains(t, tracker.ParticipantName, fmt.Sprintf("split_share_id=%d", share.ID))
	require.Contains(t, tracker.ParticipantName, "provider_tracker_id=mp_tracker_123")
}

func TestGetPluginPaymentStatus_ResolvesOriginalTrackerAfterProviderRekey(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.Bill{}, &database.Payment{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-mp-status-%d", time.Now().UnixNano()),
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 2500,
		PaidAmount:  2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "plugin",
		Amount:        2500,
		TxHash:        "plugin_MP-PAYMENT-SPLIT-1",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "USD",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "MP-PAYMENT-SPLIT-1",
		ParticipantName: "mercadopago|split_share_id=7|provider_tracker_id=mp_tracker_123",
		Amount:          2500,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusConfirmed,
	}).Error)

	mockPlugin := &testPaymentPlugin{
		name:            "mercadopago",
		verifySignature: true,
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "bill_token", Value: bill.PublicToken},
		{Key: "payment_id", Value: "mp_tracker_123"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/?plugin=mercadopago", nil)

	NewPluginHandlers(nil, nil).GetPluginPaymentStatus(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"status":"completed"`)
	require.Contains(t, w.Body.String(), `"split_share_id":7`)
	require.Zero(t, mockPlugin.statusCalls, "locally settled provider-rekeyed trackers must not call provider status with the old opaque ID")
}

func TestGetPluginPaymentStatus_ReturnsLocalPendingForMercadoPagoTracker(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.Bill{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-mp-pending-status-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "mp_tracker_pending",
		ParticipantName: "mercadopago|split_share_id=7",
		Amount:          2500,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
	}).Error)

	mockPlugin := &testPaymentPlugin{
		name:            "mercadopago",
		verifySignature: true,
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "bill_token", Value: bill.PublicToken},
		{Key: "payment_id", Value: "mp_tracker_pending"},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/?plugin=mercadopago", nil)

	NewPluginHandlers(nil, nil).GetPluginPaymentStatus(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"status":"pending"`)
	require.Contains(t, w.Body.String(), `"awaiting_settlement":true`)
	require.Zero(t, mockPlugin.statusCalls, "pending MercadoPago trackers are opaque local IDs, not provider payment IDs")
}

func TestHandlePayPalCancel_RedirectsToGuestBillRoute(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Table{}, &database.Bill{}))

	business := createTestBusiness(t)
	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  fmt.Sprintf("TT-%d", time.Now().UnixNano()),
		Name:       "Garden",
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	bill := &database.Bill{
		BusinessID: business.ID,
		TableID:    table.ID,
		BillNumber: fmt.Sprintf("B-paypal-%d", time.Now().UnixNano()),
		Status:     database.BillStatusOpen,
		Items:      "[]",
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/paypal/cancel?bill_id=%d", bill.ID),
		nil,
	)

	NewPluginHandlers(nil, nil).HandlePayPalCancel(c)

	assert.Equal(t, http.StatusFound, w.Code)
	location, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("/t/%s/bill", table.TableCode), location.Path)
	assert.Equal(t, "cancelled", location.Query().Get("payment"))
	assert.Equal(t, "paypal", location.Query().Get("method"))
	assert.Equal(t, bill.BillNumber, location.Query().Get("bill_number"))
}

func TestHandlePaymentWebhook_SettlesTipSeparatelyAndAllowsPartialBill(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-%d", time.Now().UnixNano()),
		Status:      "partial",
		Items:       "[]",
		TotalAmount: 3000,
		PaidAmount:  1000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	pluginName := fmt.Sprintf("testsettle-%d", time.Now().UnixNano())
	mockPlugin := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    bill.ID,
			PaymentID: "settlement-1",
			Amount:    2350,
			Currency:  "USD",
			Metadata: map[string]interface{}{
				"bill_amount_cents": "2000",
				"tip_amount_cents":  "350",
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"evt_1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

	assert.Equal(t, http.StatusOK, w.Code)

	updatedBill, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3000), updatedBill.PaidAmount)
	assert.Equal(t, int64(350), updatedBill.TipAmount)
	assert.Equal(t, database.BillStatusPaid, updatedBill.Status)
}

func TestHandlePaymentWebhookRecordsCRMVisitFromBillCustomer(t *testing.T) {
	setupHandlerTestDB(t)

	customer := createPaymentRegressionCustomer(t, "plugin-crm-payer@example.com")
	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("crm_enabled", true).Error)
	connectPaymentRegressionCustomer(t, customer.ID, business.ID)
	bill := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     fmt.Sprintf("B-plugin-crm-%d", time.Now().UnixNano()),
		Status:         database.BillStatusOpen,
		Items:          "[]",
		TotalAmount:    4000,
		CRMCustomerID:  &customer.ID,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	pluginName := fmt.Sprintf("testplugincrm-%d", time.Now().UnixNano())
	mockPlugin := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    bill.ID,
			PaymentID: "plugin-payment-crm-1",
			Amount:    4000,
			Currency:  "USD",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	handler := NewPluginHandlers(nil, nil)
	requestBody := `{"id":"evt_plugin_crm_1"}`
	first := httptest.NewRecorder()
	firstContext, _ := gin.CreateTestContext(first)
	firstContext.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(requestBody))
	firstContext.Request.Header.Set("Content-Type", "application/json")
	handler.handlePaymentWebhook(firstContext, pluginName)
	require.Equal(t, http.StatusOK, first.Code)
	assertSinglePaymentCRMVisitForCustomer(t, bill.ID, customer.ID)

	second := httptest.NewRecorder()
	secondContext, _ := gin.CreateTestContext(second)
	secondContext.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(requestBody))
	secondContext.Request.Header.Set("Content-Type", "application/json")
	handler.handlePaymentWebhook(secondContext, pluginName)
	require.Equal(t, http.StatusOK, second.Code)
	assertSinglePaymentCRMVisitForCustomer(t, bill.ID, customer.ID)
}

func TestHandlePaymentWebhook_DoesNotDuplicateReceiptPrintJobsForPaidSettlement(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.PrintJob{}))

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-print-dedupe-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 1000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	pluginName := fmt.Sprintf("testplugindedupe-%d", time.Now().UnixNano())
	mockPlugin := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    bill.ID,
			PaymentID: "plugin-payment-dedupe-1",
			Amount:    1000,
			Currency:  "USD",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"evt_plugin_dedupe_1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

	require.Equal(t, http.StatusOK, w.Code)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.PrintJob{}).
		Where("business_id = ? AND source_type = ? AND source_id = ? AND kind = ?",
			business.ID, "bill", bill.ID, database.PrintJobKindReceipt).
		Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestHandlePaymentWebhook_ReturnsConflictWhenSettlementExceedsRemaining(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-%d", time.Now().UnixNano()),
		Status:      "partial",
		Items:       "[]",
		TotalAmount: 3000,
		PaidAmount:  2900,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	pluginName := fmt.Sprintf("testsettlefail-%d", time.Now().UnixNano())
	mockPlugin := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    bill.ID,
			PaymentID: "settlement-fail-1",
			Amount:    2000,
			Currency:  "USD",
			Metadata: map[string]interface{}{
				"bill_amount_cents": "2000",
				"tip_amount_cents":  "0",
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"evt_fail_1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "Payment exceeds remaining bill balance")

	updatedBill, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2900), updatedBill.PaidAmount)
	assert.Equal(t, int64(0), updatedBill.TipAmount)
	assert.Equal(t, database.BillStatus("partial"), updatedBill.Status)

	var webhookEvent database.WebhookEvent
	require.NoError(t, database.GetDB().Where("provider = ? AND webhook_id = ?", "plugin_"+pluginName, "evt_fail_1").First(&webhookEvent).Error)
	assert.Equal(t, "failed", webhookEvent.Status)
	assert.Contains(t, webhookEvent.Error, "payment amount exceeds remaining balance")
}

// enableTestMercadoPagoPlugin migrates the plugin tables and enables a
// "mercadopago" business plugin carrying a webhook_secret so the handler's
// resolvePluginWebhookSignature succeeds. GetBusinessPluginConfig requires an
// enabled row and errors when none exists, so the env-var secret fallback never
// runs without this seed — the config-supplied secret is the realistic path.
func enableTestMercadoPagoPlugin(t *testing.T, businessID uint) {
	t.Helper()
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	plugin := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(businessID, plugin.ID, map[string]interface{}{
		"webhook_secret": "mp-test-secret",
	}))

	// The plugin registry is a process-global map. Tests below register their
	// mock under the real "mercadopago" name; without this cleanup the leaked
	// mock makes a later test (TestHandleMercadoPagoWebhook_ReturnsNotFound...)
	// resolve a plugin it expects to be absent. Restore the registry to its
	// pre-test state when the test finishes.
	originalPlugin, hadOriginal := plugins.GlobalRegistry.GetPlugin("mercadopago")
	t.Cleanup(func() {
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(originalPlugin)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin("mercadopago")
		}
	})
}

func TestHandlePaymentWebhook_AutoRefundsOnOverpayment(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-autorefund-%d", time.Now().UnixNano()),
		Status:      "partial",
		Items:       "[]",
		TotalAmount: 3000,
		PaidAmount:  2900,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	// Register the double under the real "mercadopago" name so the handler
	// resolves business_id from the query string and runs the auto-refund branch.
	mockPlugin := &testPaymentPlugin{
		name:            "mercadopago",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    bill.ID,
			PaymentID: "mp-overpay-1",
			Amount:    2000,
			Currency:  "USD",
			Metadata: map[string]interface{}{
				"bill_amount_cents": "2000",
				"tip_amount_cents":  "0",
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/?bill_id=%d&business_id=%d", bill.ID, business.ID),
		// extractPluginWebhookID reads data.id for mercadopago (the stable payment
		// id), so the idempotency WebhookEvent row is keyed on mp-overpay-1.
		bytes.NewBufferString(`{"id":"evt-mp-overpay-1","action":"payment.updated","data":{"id":"mp-overpay-1"}}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")

	// F7: a SUCCESSFUL auto-refund acks 200 — a 4xx would make the PSP retry the
	// webhook and re-attempt the refund. The overpayment never lands on the bill.
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "auto-refunded")

	// The auto-refund branch ran exactly once with the webhook's paymentID and a
	// zero amount (full refund).
	require.Equal(t, 1, mockPlugin.refundCalls)
	assert.Equal(t, business.ID, mockPlugin.lastRefundBizID)
	assert.Equal(t, "mp-overpay-1", mockPlugin.lastRefundPaymentID)
	assert.Equal(t, int64(0), mockPlugin.lastRefundAmount)

	// Ledger untouched: the overpayment never landed.
	updatedBill, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2900), updatedBill.PaidAmount)
	assert.Equal(t, int64(0), updatedBill.TipAmount)
	assert.Equal(t, database.BillStatus("partial"), updatedBill.Status)
}

func TestHandlePaymentWebhook_RekeysTrackerBeforeBreakdownLookup(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}))

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)
	// TotalAmount is the bill balance (2500¢). Without rekey-before-lookup the
	// webhook treats the full 3000¢ capture as bill payment; with a 3000¢ total
	// that path still settles (tip 0), which is the bug this test catches.
	// The guest-chosen split on the tracker is 2500¢ bill + 500¢ tip.
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-rekey-breakdown-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 3000,
		PaidAmount:  0,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	// Pending tracker row keyed by the provider ORDER id, carrying the
	// guest-chosen split: 2500¢ bill + 500¢ tip.
	// AlternativePayment has no BusinessID column — bill ownership is via BillID.
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORDER-9",
		ParticipantName: "PayPal (pending)",
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
		Amount:          3000,
		BillAmountCents: 2500,
		TipAmountCents:  500,
	}).Error)

	// Provider confirms under the CAPTURE id, with only the tracker pointer —
	// no bill/tip metadata (that lives on the tracker row).
	mockPlugin := &testPaymentPlugin{
		name:            "mercadopago",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    bill.ID,
			PaymentID: "CAPTURE-1",
			Amount:    3000,
			Currency:  "USD",
			Metadata:  map[string]interface{}{"provider_tracker_id": "ORDER-9"},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/?bill_id=%d&business_id=%d", bill.ID, business.ID),
		bytes.NewBufferString(`{"id":"evt-rekey-1","action":"payment.updated","data":{"id":"CAPTURE-1"}}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")

	assert.Equal(t, http.StatusOK, w.Code)

	// The settled payment must carry the tracker row's tip split.
	var payment database.Payment
	require.NoError(t, database.GetDB().Where("bill_id = ?", bill.ID).
		Order("id DESC").First(&payment).Error)
	require.EqualValues(t, 500, payment.TipAmount,
		"tip split from the tracker row must survive the provider's capture-id rename")
	require.EqualValues(t, 2500, payment.Amount)
}

func TestHandlePaymentWebhook_OverpaymentRefundErrorStillReturnsConflict(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-refunderr-%d", time.Now().UnixNano()),
		Status:      "partial",
		Items:       "[]",
		TotalAmount: 3000,
		PaidAmount:  2900,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	mockPlugin := &testPaymentPlugin{
		name:            "mercadopago",
		verifySignature: true,
		refundErr:       errors.New("psp refund endpoint unavailable"),
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    bill.ID,
			PaymentID: "mp-overpay-refunderr-1",
			Amount:    2000,
			Currency:  "USD",
			Metadata: map[string]interface{}{
				"bill_amount_cents": "2000",
				"tip_amount_cents":  "0",
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/?bill_id=%d&business_id=%d", bill.ID, business.ID),
		// data.id → stable WebhookEvent.webhook_id for the assertion below
		// (mercadopago dedup keys on the payment id, not the per-delivery id).
		bytes.NewBufferString(`{"id":"evt-mp-overpay-refunderr-1","action":"payment.updated","data":{"id":"mp-overpay-refunderr-1"}}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")

	// Must NOT panic even though RefundPayment errors.
	require.NotPanics(t, func() {
		NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")
	})

	// Refund was attempted exactly once and the error was swallowed.
	require.Equal(t, 1, mockPlugin.refundCalls)
	assert.Equal(t, "mp-overpay-refunderr-1", mockPlugin.lastRefundPaymentID)

	// Same 409 as the happy-refund case — a failed refund does not change the
	// response contract.
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "Payment exceeds remaining bill balance")

	// Ledger unchanged.
	updatedBill, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2900), updatedBill.PaidAmount)
	assert.Equal(t, int64(0), updatedBill.TipAmount)
	assert.Equal(t, database.BillStatus("partial"), updatedBill.Status)

	// The webhook event is marked failed (settlement failed before the refund
	// branch — MarkWebhookEventFailed runs at plugin_handlers.go:1468-1470).
	// webhook_id is action-scoped (extractPluginWebhookID/mercadopago).
	var webhookEvent database.WebhookEvent
	require.NoError(t, database.GetDB().
		Where("provider = ? AND webhook_id = ?", "plugin_mercadopago", "payment.updated:mp-overpay-refunderr-1").
		First(&webhookEvent).Error)
	assert.Equal(t, "failed", webhookEvent.Status)
}

func TestHandlePaymentWebhook_ReversesAppliedAmountsOnRefundOrProviderReversal(t *testing.T) {
	for _, status := range []string{"refunded", "reversed"} {
		t.Run(status, func(t *testing.T) {
			setupHandlerTestDB(t)

			business := createTestBusiness(t)
			bill := &database.Bill{
				BusinessID:  business.ID,
				BillNumber:  fmt.Sprintf("B-reverse-%s-%d", status, time.Now().UnixNano()),
				Status:      database.BillStatusPaid,
				Items:       "[]",
				TotalAmount: 2000,
				PaidAmount:  2000,
				TipAmount:   300,
			}
			require.NoError(t, database.GetDB().Create(bill).Error)

			paymentID := fmt.Sprintf("plugin-reverse-%s-1", status)
			payment := &database.Payment{
				BillID:        bill.ID,
				PayerAddr:     "test-payer",
				Amount:        2000,
				TipAmount:     300,
				TxHash:        "plugin_" + paymentID, // handler computes plugin_<paymentID>
				Status:        database.PaymentStatusConfirmed,
				PaymentMethod: "plugin",
			}
			require.NoError(t, database.GetDB().Create(payment).Error)

			pluginName := fmt.Sprintf("testreverse-%s-%d", status, time.Now().UnixNano())
			mockPlugin := &testPaymentPlugin{
				name: pluginName,
				webhookResponse: &plugins.WebhookResponse{
					Success:   true,
					Status:    status,
					BillID:    bill.ID,
					PaymentID: paymentID,
					// Full refund of the FULL capture (bill 2000 + tip 300). H9:
					// the refunded baseline is bill+tip, so a full reversal must
					// report the whole captured amount, not just the bill portion.
					Amount:   2300,
					Currency: "USD",
				},
			}
			plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

			originalSupport := guestBillPaymentPluginSupported
			defer func() { guestBillPaymentPluginSupported = originalSupport }()
			guestBillPaymentPluginSupported = func(name string) bool { return name == pluginName }

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(
				http.MethodPost, "/",
				bytes.NewBufferString(fmt.Sprintf(`{"id":"evt_reverse_%s_1"}`, status)),
			)
			c.Request.Header.Set("Content-Type", "application/json")

			NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

			// Reversal succeeded → 200 OK.
			assert.Equal(t, http.StatusOK, w.Code)

			// Bill amounts reversed to zero; status recomputed to open.
			updatedBill, _, err := database.GetBillByID(bill.ID)
			require.NoError(t, err)
			assert.Equal(t, int64(0), updatedBill.PaidAmount)
			assert.Equal(t, int64(0), updatedBill.TipAmount)
			assert.Equal(t, database.BillStatusOpen, updatedBill.Status)

			// Payment row marked reversed.
			var reloaded database.Payment
			require.NoError(t, database.GetDB().First(&reloaded, payment.ID).Error)
			assert.Equal(t, database.PaymentStatusReversed, reloaded.Status)
			assert.NotNil(t, reloaded.ReversedAt)
		})
	}
}

func TestHandlePaymentWebhook_ReturnsBadRequestWhenBreakdownHasNegativeTip(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 3000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	pluginName := fmt.Sprintf("testsettlenegtip-%d", time.Now().UnixNano())
	mockPlugin := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    bill.ID,
			PaymentID: "settlement-negative-tip-1",
			Amount:    2000,
			Currency:  "USD",
			Metadata: map[string]interface{}{
				"bill_amount_cents": "2000",
				"tip_amount_cents":  "-1",
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"evt_negative_tip_1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Tip amount cannot be negative")

	var paymentCount int64
	require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
	assert.Zero(t, paymentCount)

	updatedBill, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), updatedBill.PaidAmount)
	assert.Equal(t, int64(0), updatedBill.TipAmount)
	assert.Equal(t, database.BillStatusOpen, updatedBill.Status)

	var webhookEvent database.WebhookEvent
	require.NoError(t, database.GetDB().Where("provider = ? AND webhook_id = ?", "plugin_"+pluginName, "evt_negative_tip_1").First(&webhookEvent).Error)
	assert.Equal(t, "failed", webhookEvent.Status)
	assert.Contains(t, webhookEvent.Error, "tip amount cannot be negative")
}

func TestHandlePaymentWebhook_ReturnsBadRequestWhenBreakdownHasNegativeBillAmount(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 3000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	pluginName := fmt.Sprintf("testsettlenegbill-%d", time.Now().UnixNano())
	mockPlugin := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    bill.ID,
			PaymentID: "settlement-negative-bill-1",
			Amount:    2000,
			Currency:  "USD",
			Metadata: map[string]interface{}{
				"bill_amount_cents": "-1",
				"tip_amount_cents":  "2001",
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"evt_negative_bill_1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Payment amount must be greater than zero")

	var paymentCount int64
	require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
	assert.Zero(t, paymentCount)

	var webhookEvent database.WebhookEvent
	require.NoError(t, database.GetDB().Where("provider = ? AND webhook_id = ?", "plugin_"+pluginName, "evt_negative_bill_1").First(&webhookEvent).Error)
	assert.Equal(t, "failed", webhookEvent.Status)
	assert.Contains(t, webhookEvent.Error, "payment amount must be greater than zero")
}

func TestHandlePaymentWebhook_ReturnsBadRequestWhenBreakdownMismatchesAmount(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 3000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	pluginName := fmt.Sprintf("testsettlemismatch-%d", time.Now().UnixNano())
	mockPlugin := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    bill.ID,
			PaymentID: "settlement-mismatch-1",
			Amount:    2000,
			Currency:  "USD",
			Metadata: map[string]interface{}{
				"bill_amount_cents": "1500",
				"tip_amount_cents":  "200",
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"evt_mismatch_1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Payment breakdown does not match webhook amount")

	var paymentCount int64
	require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
	assert.Zero(t, paymentCount)

	var webhookEvent database.WebhookEvent
	require.NoError(t, database.GetDB().Where("provider = ? AND webhook_id = ?", "plugin_"+pluginName, "evt_mismatch_1").First(&webhookEvent).Error)
	assert.Equal(t, "failed", webhookEvent.Status)
	assert.Contains(t, webhookEvent.Error, "payment breakdown does not match webhook amount")
}

func TestHandlePaymentWebhook_ReturnsConflictWhenPaymentIDBelongsToAnotherBill(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	existingBill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-existing-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 3000,
	}
	require.NoError(t, database.GetDB().Create(existingBill).Error)
	targetBill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-target-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 3000,
	}
	require.NoError(t, database.GetDB().Create(targetBill).Error)
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        existingBill.ID,
		PayerAddr:     "plugin",
		Amount:        1000,
		TxHash:        "plugin_settlement-conflict-1",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "USD",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}).Error)

	pluginName := fmt.Sprintf("testsettleconflict-%d", time.Now().UnixNano())
	mockPlugin := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    targetBill.ID,
			PaymentID: "settlement-conflict-1",
			Amount:    1000,
			Currency:  "USD",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"evt_conflict_1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "Payment transaction already recorded")

	updatedBill, _, err := database.GetBillByID(targetBill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), updatedBill.PaidAmount)
	assert.Equal(t, database.BillStatusOpen, updatedBill.Status)

	var webhookEvent database.WebhookEvent
	require.NoError(t, database.GetDB().Where("provider = ? AND webhook_id = ?", "plugin_"+pluginName, "evt_conflict_1").First(&webhookEvent).Error)
	assert.Equal(t, "failed", webhookEvent.Status)
	assert.Contains(t, webhookEvent.Error, "payment transaction hash conflict")
}

func TestHandlePaymentWebhook_ReturnsConflictWhenTerminalBillPaymentIDBelongsToAnotherBill(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	existingBill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-existing-terminal-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 3000,
	}
	require.NoError(t, database.GetDB().Create(existingBill).Error)
	targetBill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-target-terminal-%d", time.Now().UnixNano()),
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 3000,
		PaidAmount:  3000,
	}
	require.NoError(t, database.GetDB().Create(targetBill).Error)
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        existingBill.ID,
		PayerAddr:     "plugin",
		Amount:        1000,
		TxHash:        "plugin_settlement-terminal-conflict-1",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "USD",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}).Error)

	pluginName := fmt.Sprintf("testsettleterminalconflict-%d", time.Now().UnixNano())
	mockPlugin := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    targetBill.ID,
			PaymentID: "settlement-terminal-conflict-1",
			Amount:    1000,
			Currency:  "USD",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"evt_terminal_conflict_1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "Payment transaction already recorded")

	updatedBill, _, err := database.GetBillByID(targetBill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3000), updatedBill.PaidAmount)
	assert.Equal(t, database.BillStatusPaid, updatedBill.Status)

	var webhookEvent database.WebhookEvent
	require.NoError(t, database.GetDB().Where("provider = ? AND webhook_id = ?", "plugin_"+pluginName, "evt_terminal_conflict_1").First(&webhookEvent).Error)
	assert.Equal(t, "failed", webhookEvent.Status)
	assert.Contains(t, webhookEvent.Error, "payment transaction hash conflict")
}

func TestHandlePaymentWebhook_ReturnsConflictWhenTerminalBillHasNoRecordedPayment(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}))

	business := createTestBusiness(t)
	targetBill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-target-terminal-new-%d", time.Now().UnixNano()),
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 3000,
		PaidAmount:  3000,
	}
	require.NoError(t, database.GetDB().Create(targetBill).Error)

	pluginName := fmt.Sprintf("testsettleterminalnew-%d", time.Now().UnixNano())
	paymentID := "settlement-terminal-new-1"
	trackedPayment := &database.AlternativePayment{
		BillID:          targetBill.ID,
		ParticipantAddr: paymentID,
		ParticipantName: pluginName,
		Amount:          1000,
		PaymentMethod:   database.AlternativePaymentMethod(pluginName),
		Status:          database.AltPaymentStatusPending,
	}
	require.NoError(t, database.GetDB().Create(trackedPayment).Error)

	mockPlugin := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    targetBill.ID,
			PaymentID: paymentID,
			Amount:    1000,
			Currency:  "USD",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"evt_terminal_new_1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "Bill is already paid or closed")

	_, err := database.GetPaymentByTxHash("plugin_" + paymentID)
	require.True(t, errors.Is(err, database.ErrPaymentNotFound), "expected no settlement payment row, got %v", err)

	var refreshedTracked database.AlternativePayment
	require.NoError(t, database.GetDB().First(&refreshedTracked, trackedPayment.ID).Error)
	assert.Equal(t, database.AltPaymentStatusPending, refreshedTracked.Status)
	assert.Empty(t, refreshedTracked.ConfirmedBy)
	assert.Nil(t, refreshedTracked.ConfirmedAt)

	var webhookEvent database.WebhookEvent
	require.NoError(t, database.GetDB().Where("provider = ? AND webhook_id = ?", "plugin_"+pluginName, "evt_terminal_new_1").First(&webhookEvent).Error)
	assert.Equal(t, "failed", webhookEvent.Status)
	assert.NotEqual(t, "processed", webhookEvent.Status)
	assert.Contains(t, webhookEvent.Error, "bill is not open for payment")
}

func TestHandlePaymentWebhook_IdempotentTerminalBillPaymentConfirmsTrackedRecord(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}))

	business := createTestBusiness(t)
	targetBill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-target-terminal-idempotent-%d", time.Now().UnixNano()),
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 3000,
		PaidAmount:  3000,
	}
	require.NoError(t, database.GetDB().Create(targetBill).Error)

	pluginName := fmt.Sprintf("testsettleterminalidem-%d", time.Now().UnixNano())
	paymentID := "settlement-terminal-idempotent-1"
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        targetBill.ID,
		PayerAddr:     "plugin",
		Amount:        1000,
		TipAmount:     0,
		TxHash:        "plugin_" + paymentID,
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "USD",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}).Error)
	trackedPayment := &database.AlternativePayment{
		BillID:          targetBill.ID,
		ParticipantAddr: paymentID,
		ParticipantName: pluginName,
		Amount:          1000,
		PaymentMethod:   database.AlternativePaymentMethod(pluginName),
		Status:          database.AltPaymentStatusPending,
	}
	require.NoError(t, database.GetDB().Create(trackedPayment).Error)

	mockPlugin := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    targetBill.ID,
			PaymentID: paymentID,
			Amount:    1000,
			Currency:  "USD",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"evt_terminal_idempotent_1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

	assert.Equal(t, http.StatusOK, w.Code)

	var paymentCount int64
	require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("tx_hash = ?", "plugin_"+paymentID).Count(&paymentCount).Error)
	assert.EqualValues(t, 1, paymentCount)

	var refreshedTracked database.AlternativePayment
	require.NoError(t, database.GetDB().First(&refreshedTracked, trackedPayment.ID).Error)
	assert.Equal(t, database.AltPaymentStatusConfirmed, refreshedTracked.Status)
	assert.Equal(t, "plugin_webhook", refreshedTracked.ConfirmedBy)
	assert.NotNil(t, refreshedTracked.ConfirmedAt)

	var webhookEvent database.WebhookEvent
	require.NoError(t, database.GetDB().Where("provider = ? AND webhook_id = ?", "plugin_"+pluginName, "evt_terminal_idempotent_1").First(&webhookEvent).Error)
	assert.Equal(t, "processed", webhookEvent.Status)
	assert.Empty(t, webhookEvent.Error)
}

func TestHandlePaymentWebhook_RetriesPreviouslyFailedWebhook(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-%d", time.Now().UnixNano()),
		Status:      "partial",
		Items:       "[]",
		TotalAmount: 3000,
		PaidAmount:  2900,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	pluginName := fmt.Sprintf("testsettleretry-%d", time.Now().UnixNano())
	mockPlugin := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    bill.ID,
			PaymentID: "settlement-retry-1",
			Amount:    2000,
			Currency:  "USD",
			Metadata: map[string]interface{}{
				"bill_amount_cents": "2000",
				"tip_amount_cents":  "0",
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	requestBody := `{"id":"evt_retry_1"}`
	handler := NewPluginHandlers(nil, nil)

	firstWriter := httptest.NewRecorder()
	firstContext, _ := gin.CreateTestContext(firstWriter)
	firstContext.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(requestBody))
	firstContext.Request.Header.Set("Content-Type", "application/json")
	handler.handlePaymentWebhook(firstContext, pluginName)

	assert.Equal(t, http.StatusConflict, firstWriter.Code)

	mockPlugin.webhookResponse = &plugins.WebhookResponse{
		Success:   true,
		Status:    "completed",
		BillID:    bill.ID,
		PaymentID: "settlement-retry-1",
		Amount:    100,
		Currency:  "USD",
		Metadata: map[string]interface{}{
			"bill_amount_cents": "100",
			"tip_amount_cents":  "0",
		},
	}

	secondWriter := httptest.NewRecorder()
	secondContext, _ := gin.CreateTestContext(secondWriter)
	secondContext.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(requestBody))
	secondContext.Request.Header.Set("Content-Type", "application/json")
	handler.handlePaymentWebhook(secondContext, pluginName)

	assert.Equal(t, http.StatusOK, secondWriter.Code)

	updatedBill, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3000), updatedBill.PaidAmount)
	assert.Equal(t, database.BillStatusPaid, updatedBill.Status)

	var webhookEvent database.WebhookEvent
	require.NoError(t, database.GetDB().Where("provider = ? AND webhook_id = ?", "plugin_"+pluginName, "evt_retry_1").First(&webhookEvent).Error)
	assert.Equal(t, "processed", webhookEvent.Status)
}

func TestCreatePluginPayment_RejectsInactivePlatformPlugin(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	pluginName := fmt.Sprintf("inactivepay-%d", time.Now().UnixNano())
	pluginRecord := &database.Plugin{
		Name:        pluginName,
		DisplayName: "InactivePay",
		Category:    database.PluginCategoryPayment,
		IsActive:    false,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   pluginRecord.ID,
		IsEnabled:  true,
		Config:     "{}",
	}).Error)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 25,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	originalSupport := guestBillPaymentPluginSupported
	defer func() {
		guestBillPaymentPluginSupported = originalSupport
	}()
	guestBillPaymentPluginSupported = func(name string) bool {
		return name == pluginName
	}

	body, err := json.Marshal(map[string]interface{}{
		"plugin_id": pluginName,
		"amount":    25,
		"currency":  "USD",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).CreatePluginPayment(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandlePayPalWebhook_ReturnsNotFoundWhenPluginUnregistered(t *testing.T) {
	setupHandlerTestDB(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).HandlePayPalWebhook(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleMercadoPagoWebhook_ReturnsNotFoundWhenPluginUnregistered(t *testing.T) {
	setupHandlerTestDB(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).HandleMercadoPagoWebhook(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestHandleMercadoPagoWebhook_OrderTopicResolvesBusinessViaTracker asserts that
// an order.processed webhook without business_id query still settles when a
// pending AlternativePayment tracker maps data.id → bill → business.
func TestHandleMercadoPagoWebhook_OrderTopicResolvesBusinessViaTracker(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{}, &database.Table{},
		&database.Bill{}, &database.Payment{}, &database.AlternativePayment{},
		&database.WebhookEvent{},
	))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	enableTestMercadoPagoPlugin(t, business.ID)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-mp-order-tracker-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 3350,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORD123",
		ParticipantName: "mercadopago",
		Amount:          3350,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
	}).Error)

	mockPlugin := &testPaymentPlugin{
		name:            "mercadopago",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: "ORD123",
			BillID:    bill.ID,
			Amount:    3350,
			Currency:  "USD",
			Metadata: map[string]interface{}{
				"payverge_tracker_id": "ORD123",
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)

	payload := `{"id":"notif-ord-1","action":"order.processed","type":"order","data":{"id":"ORD123","status":"processed"}}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// No business_id query — resolution must use the tracker.
	c.Request = httptest.NewRequest(http.MethodPost, "/?data.id=ORD123", bytes.NewBufferString(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	payment, err := database.GetPaymentByTxHash("plugin_ORD123")
	require.NoError(t, err)
	require.Equal(t, bill.ID, payment.BillID)
	require.Equal(t, int64(3350), payment.Amount)

	var tracker database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ? AND participant_addr = ?", bill.ID, "ORD123").First(&tracker).Error)
	require.Equal(t, database.AltPaymentStatusConfirmed, tracker.Status)
}

// TestHandleMercadoPagoWebhook_OrderTopicNoTrackerReturns500 (F5) asserts that
// a verified order.* notification without a matching tracker returns non-2xx so
// MercadoPago retries (covers the create-order-before-tracker-insert race) and
// does not settle any bill.
func TestHandleMercadoPagoWebhook_OrderTopicNoTrackerReturns500(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{}, &database.Bill{},
		&database.Payment{}, &database.AlternativePayment{},
	))

	t.Setenv("MERCADOPAGO_WEBHOOK_SECRET", "mp-env-secret")

	// Register a double that passes signature verification so the unresolved
	// path exercises the retry response after verify.
	originalPlugin, hadOriginal := plugins.GlobalRegistry.GetPlugin("mercadopago")
	t.Cleanup(func() {
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(originalPlugin)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin("mercadopago")
		}
	})
	plugins.GlobalRegistry.RegisterPlugin(&testPaymentPlugin{
		name:            "mercadopago",
		verifySignature: true,
	})

	payload := `{"id":"notif-ord-miss","action":"order.processed","type":"order","data":{"id":"ORD-NO-TRACKER"}}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/?data.id=ORD-NO-TRACKER", bytes.NewBufferString(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Order tracker not ready")
	_, err := database.GetPaymentByTxHash("plugin_ORD-NO-TRACKER")
	require.Error(t, err)
}

// TestHandleMercadoPagoWebhook_PaymentUpdatedStillRequiresBusiness ensures
// Checkout Pro payment.updated webhooks without business_id (and no order
// tracker path) still fail closed rather than accepting unverified settlement.
func TestHandleMercadoPagoWebhook_PaymentUpdatedStillRequiresBusiness(t *testing.T) {
	setupHandlerTestDB(t)

	originalPlugin, hadOriginal := plugins.GlobalRegistry.GetPlugin("mercadopago")
	t.Cleanup(func() {
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(originalPlugin)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin("mercadopago")
		}
	})
	plugins.GlobalRegistry.RegisterPlugin(&testPaymentPlugin{
		name:            "mercadopago",
		verifySignature: true,
	})

	payload := `{"id":"notif-pay-1","action":"payment.updated","type":"payment","data":{"id":"999888"}}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Unable to resolve business")
}

func TestGetBusinessIDByPluginPaymentTracker(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Bill{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-tracker-lookup-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 1000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "ORD-LOOKUP-1",
		ParticipantName: "mercadopago",
		Amount:          1000,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
	}).Error)

	got, err := database.GetBusinessIDByPluginPaymentTracker("ORD-LOOKUP-1", "mercadopago")
	require.NoError(t, err)
	require.Equal(t, business.ID, got)

	_, err = database.GetBusinessIDByPluginPaymentTracker("ORD-MISSING", "mercadopago")
	require.Error(t, err)
}

func TestPaymentWebhookRequiresResolvedBusiness_MercadoPagoExempt(t *testing.T) {
	// MP implements WebhookRequestVerifier in production; the exemption must
	// hold even when the registered plugin satisfies that interface pattern.
	require.False(t, paymentWebhookRequiresResolvedBusiness("mercadopago", &testPaymentPlugin{name: "mercadopago"}))
	require.True(t, paymentWebhookRequiresResolvedBusiness("stripe", &testPaymentPlugin{name: "stripe"}))
}

// TestResolvePluginWebhookSignature_MissingSecret verifies that
// resolvePluginWebhookSignature returns errPluginWebhookSecretMissing when no
// secret is configured in the business plugin config or environment.
func TestResolvePluginWebhookSignature_MissingSecret(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)

	for _, pluginName := range []string{"paypal", "mercadopago", "stripe"} {
		t.Run(pluginName, func(t *testing.T) {
			// Register the plugin with an empty config (no webhook_secret).
			plugin := &database.Plugin{
				Name:        pluginName,
				DisplayName: pluginName,
				Category:    database.PluginCategoryPayment,
				IsActive:    true,
			}
			require.NoError(t, database.GetDB().Create(plugin).Error)
			require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
				BusinessID: business.ID,
				PluginID:   plugin.ID,
				IsEnabled:  true,
				Config:     "{}",
			}).Error)

			// Ensure the env-var fallbacks are not set.
			for _, cfg := range pluginWebhookConfigs[pluginName].envVars {
				t.Setenv(cfg, "")
			}

			headers := map[string]string{
				pluginWebhookConfigs[pluginName].signatureHeader: "some-signature-value",
			}
			_, _, err := resolvePluginWebhookSignature(pluginName, business.ID, headers)
			require.ErrorIs(t, err, errPluginWebhookSecretMissing,
				"expected errPluginWebhookSecretMissing for plugin %q with no configured secret", pluginName)
		})
	}
}

// TestResolvePluginWebhookSignature_MissingHeader verifies that
// resolvePluginWebhookSignature returns an error when the expected signature
// header is absent from the request.
func TestResolvePluginWebhookSignature_MissingHeader(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)

	for _, pluginName := range []string{"paypal", "mercadopago"} {
		t.Run(pluginName, func(t *testing.T) {
			plugin := &database.Plugin{
				Name:        pluginName + "_hdr",
				DisplayName: pluginName + "_hdr",
				Category:    database.PluginCategoryPayment,
				IsActive:    true,
			}
			require.NoError(t, database.GetDB().Create(plugin).Error)
			// Store a real secret so we get past the secret-resolution step.
			require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
				BusinessID: business.ID,
				PluginID:   plugin.ID,
				IsEnabled:  true,
				Config:     `{"webhook_secret":"test-secret"}`,
			}).Error)

			// Pass an empty headers map — no signature header present.
			_, _, err := resolvePluginWebhookSignature(pluginName, business.ID, map[string]string{})
			require.Error(t, err, "expected error for missing signature header for plugin %q", pluginName)
			require.NotErrorIs(t, err, errPluginWebhookSecretMissing,
				"error should be about missing header, not missing secret")
		})
	}
}

// TestVerifyPluginWebhookSignature_NonStripeRejected verifies that payloads
// delivered without a valid signature are rejected for PayPal and MercadoPago,
// not just Stripe. It calls the verification function directly with a plugin
// stub that always returns false from VerifyWebhookSignature.
func TestVerifyPluginWebhookSignature_NonStripeRejected(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	for _, pluginName := range []string{"paypal", "mercadopago"} {
		t.Run(pluginName, func(t *testing.T) {
			// Use a unique business per sub-test so DB lookups are isolated.
			business := createTestBusiness(t)

			// Register the plugin using its canonical name so GetBusinessPluginConfig
			// finds it via the name-based JOIN.
			plugin := &database.Plugin{
				Name:        pluginName,
				DisplayName: pluginName,
				Category:    database.PluginCategoryPayment,
				IsActive:    true,
			}
			require.NoError(t, database.GetDB().Create(plugin).Error)
			require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
				BusinessID: business.ID,
				PluginID:   plugin.ID,
				IsEnabled:  true,
				Config:     `{"webhook_secret":"test-secret"}`,
			}).Error)

			// testPaymentPlugin.VerifyWebhookSignature always returns false.
			stub := &testPaymentPlugin{name: pluginName}

			sigHeader := pluginWebhookConfigs[pluginName].signatureHeader
			headers := map[string]string{sigHeader: "bad-signature"}

			err := verifyPluginWebhookSignature(stub, pluginName, business.ID, []byte(`{}`), headers, nil)
			require.Error(t, err, "expected signature verification to fail for plugin %q with wrong signature", pluginName)
			assert.Contains(t, err.Error(), "signature verification failed")
		})
	}
}

// ── PAY-2: pluginWebhookSettlementBreakdownCents with local row ──

func TestPluginWebhookSettlementBreakdownCents_NilMetadataWithLocalRow(t *testing.T) {
	// Metadata nil, local (bill=9000, tip=1000), Amount=10000 → (9000, 1000)
	response := &plugins.WebhookResponse{
		Amount: 10000,
	}
	bill, tip, err := pluginWebhookSettlementBreakdownCents(response, 9000, 1000, true)
	require.NoError(t, err)
	assert.Equal(t, int64(9000), bill)
	assert.Equal(t, int64(1000), tip)
}

func TestPluginWebhookSettlementBreakdownCents_NilMetadataNoLocal(t *testing.T) {
	// Metadata nil, no local → legacy (10000, 0)
	response := &plugins.WebhookResponse{
		Amount: 10000,
	}
	bill, tip, err := pluginWebhookSettlementBreakdownCents(response, 0, 0, false)
	require.NoError(t, err)
	assert.Equal(t, int64(10000), bill)
	assert.Equal(t, int64(0), tip)
}

func TestPluginWebhookSettlementBreakdownCents_MetadataHonoredWhenPresent(t *testing.T) {
	// Metadata has tip=1000, bill=9000, Amount=10000 → (9000, 1000)
	response := &plugins.WebhookResponse{
		Amount: 10000,
		Metadata: map[string]interface{}{
			"bill_amount_cents": "9000",
			"tip_amount_cents":  "1000",
		},
	}
	bill, tip, err := pluginWebhookSettlementBreakdownCents(response, 0, 0, false)
	require.NoError(t, err)
	assert.Equal(t, int64(9000), bill)
	assert.Equal(t, int64(1000), tip)
}

func TestPluginWebhookSettlementBreakdownCents_LocalRowSumMismatchIsRejected(t *testing.T) {
	// The persisted checkout tracker is the amount binding. Reinterpreting a
	// mismatched provider capture as an untracked legacy payment would accept the
	// wrong amount and silently discard the locally committed bill/tip split.
	response := &plugins.WebhookResponse{
		Amount: 10000,
	}
	_, _, err := pluginWebhookSettlementBreakdownCents(response, 8000, 1000, true)
	require.ErrorIs(t, err, errPluginPaymentBreakdownMismatch)
}

func TestPluginWebhookSettlementBreakdownCents_NegativeLocalBillRejected(t *testing.T) {
	response := &plugins.WebhookResponse{
		Amount: 5000,
	}
	bill, tip, err := pluginWebhookSettlementBreakdownCents(response, -100, 5100, true)
	require.NoError(t, err)
	assert.Equal(t, int64(5000), bill, "negative local bill should fall back to legacy")
	assert.Equal(t, int64(0), tip)
}

func TestPluginWebhookSettlementBreakdownCents_NegativeLocalTipRejected(t *testing.T) {
	response := &plugins.WebhookResponse{
		Amount: 5000,
	}
	bill, tip, err := pluginWebhookSettlementBreakdownCents(response, 5100, -100, true)
	require.NoError(t, err)
	assert.Equal(t, int64(5000), bill, "negative local tip should fall back to legacy")
	assert.Equal(t, int64(0), tip)
}

func TestPluginWebhookSettlementBreakdownCents_NilResponse(t *testing.T) {
	_, _, err := pluginWebhookSettlementBreakdownCents(nil, 0, 0, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing webhook response")
}

// ── PAY-2: storePluginPaymentRecord persists breakdown ──

func TestStorePluginPaymentRecord_PersistsBreakdown(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}))

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 10000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	payment, err := NewPluginHandlers(nil, nil).storePluginPaymentRecord(
		bill.ID,
		business.ID,
		"paypal",
		"pay-tip-split-1",
		10000,
		"USD",
		9000, // bill amount
		1000, // tip amount
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, payment)
	assert.Equal(t, int64(9000), payment.BillAmountCents)
	assert.Equal(t, int64(1000), payment.TipAmountCents)

	// Re-fetch and verify persistence
	var row database.AlternativePayment
	require.NoError(t, database.GetDB().First(&row, payment.ID).Error)
	assert.Equal(t, int64(9000), row.BillAmountCents)
	assert.Equal(t, int64(1000), row.TipAmountCents)
}

// TestCapturePayPalReturn_NoTracker_SkipsCapture verifies that capturePayPalReturnPayment
// returns "" and does NOT invoke the outbound capture API when no locally-initiated
// pending PayPal tracker exists for the (bill, orderID) pair. Without this guard,
// an unauthenticated caller could drive outbound PayPal capture calls for arbitrary
// (bill, token) pairs — an amplification / PayPal rate-limit burn vector. (P1.9)
func TestCapturePayPalReturn_NoTracker_SkipsCapture(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{}, &database.BusinessPlugin{},
		&database.Bill{}, &database.AlternativePayment{},
	))

	business := createTestBusiness(t)
	bill := createTestBillForBiz(t, business.ID)

	// Seed a "paypal" Plugin row + BusinessPlugin config so GetBusinessPluginConfig
	// passes (the check that precedes the tracker guard).
	paypalNamedPlugin := &database.Plugin{
		Name:        "paypal",
		DisplayName: "PayPal",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	// Ignore duplicate-name error — another test may have created it already.
	_ = database.GetDB().Create(paypalNamedPlugin)
	var paypalDBPlugin database.Plugin
	require.NoError(t, database.GetDB().Where("name = ?", "paypal").First(&paypalDBPlugin).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   paypalDBPlugin.ID,
		IsEnabled:  true,
		Config:     `{"client_id":"test-client","client_secret":"test-secret"}`,
	}).Error)

	// Register a mock PayPal plugin; we will verify its CapturePaymentReturn is never called.
	trackingPlugin := &testPaymentPlugin{
		name: "paypal",
		returnResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: "no-tracker-capture",
			BillID:    bill.ID,
			Amount:    1000,
			Currency:  "USD",
		},
	}
	originalPlugin, hadOriginal := plugins.GlobalRegistry.GetPlugin("paypal")
	plugins.GlobalRegistry.RegisterPlugin(trackingPlugin)
	defer func() {
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(originalPlugin)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin("paypal")
		}
	}()

	// NO AlternativePayment tracker row for this bill + token — this is the key condition.
	// (There is no row with bill_id=bill.ID, participant_addr="ORDER-NO-TRACKER-XYZ", payment_method="paypal")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/paypal/return?bill_id=%d&token=ORDER-NO-TRACKER-XYZ&PayerID=payer-1", bill.ID),
		nil,
	)

	got, _ := NewPluginHandlers(nil, nil).capturePayPalReturnPayment(c, fmt.Sprintf("%d", bill.ID), "ORDER-NO-TRACKER-XYZ")

	require.Equal(t, "", got,
		"capturePayPalReturnPayment must return \"\" when no local pending tracker exists for the bill/order pair")
	require.Equal(t, "", trackingPlugin.lastReturnOrder,
		"CapturePaymentReturn must NOT be invoked when no local pending tracker exists (amplification guard)")
}

func TestExtractPluginWebhookID_MercadoPagoScopesByAction(t *testing.T) {
	withAction := map[string]interface{}{
		"id":     "evt-1",
		"action": "payment.updated",
		"data":   map[string]interface{}{"id": "12345"},
	}
	if got := extractPluginWebhookID("mercadopago", withAction); got != "payment.updated:12345" {
		t.Fatalf("expected action-scoped id payment.updated:12345, got %q", got)
	}

	// Without an action the stable data.id remains the key.
	withoutAction := map[string]interface{}{
		"id":   "evt-2",
		"data": map[string]interface{}{"id": "12345"},
	}
	if got := extractPluginWebhookID("mercadopago", withoutAction); got != "12345" {
		t.Fatalf("expected fallback data id 12345, got %q", got)
	}
}
