package handlers

import (
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
	"github.com/stdevmac/payverge/backend/internal/plugins"
)

func TestHandleMercadoPagoReturn_CapturesAndSettlesBill(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.Table{}, &database.Bill{}, &database.Payment{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  fmt.Sprintf("MP-%d", time.Now().UnixNano()),
		Name:       "MP Return",
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	bill := &database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  fmt.Sprintf("B-mp-return-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	pluginRecord := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, map[string]interface{}{
		"access_token":   "APP_USR-1234567890-test-token",
		"public_key":     "APP_USR-1234567890-test-key",
		"webhook_secret": "whsec",
		"base_url":       "https://api.staging.example",
		"environment":    "sandbox",
	}))

	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "mp_tracker_abc",
		ParticipantName: "mercadopago",
		Amount:          2500,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
	}).Error)

	originalPlugin, hadOriginal := plugins.GlobalRegistry.GetPlugin("mercadopago")
	mockPlugin := &testPaymentPlugin{
		name: "mercadopago",
		returnResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: "987654",
			BillID:    bill.ID,
			Amount:    2500,
			Currency:  "USD",
			Metadata: map[string]interface{}{
				"payverge_tracker_id": "mp_tracker_abc",
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mockPlugin)
	defer func() {
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(originalPlugin)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin("mercadopago")
		}
	}()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/mercadopago/return?bill_id=%d&payment_id=987654&status=approved", bill.ID),
		nil,
	)

	NewPluginHandlers(nil, nil).HandleMercadoPagoReturn(c)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, business.ID, mockPlugin.lastBusinessID)
	assert.Equal(t, bill.ID, mockPlugin.lastBillID)
	assert.Equal(t, "987654", mockPlugin.lastReturnOrder)

	location, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "success", location.Query().Get("payment"))
	assert.Equal(t, "987654", location.Query().Get("payment_id"))
	assert.Equal(t, "mercadopago", location.Query().Get("method"))

	var refreshed database.Bill
	require.NoError(t, database.GetDB().First(&refreshed, bill.ID).Error)
	assert.Equal(t, database.BillStatusPaid, refreshed.Status)
}
