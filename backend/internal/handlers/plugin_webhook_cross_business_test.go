package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
)

func signStripePayload(t *testing.T, payload []byte, secret string) string {
	t.Helper()
	timestamp := time.Now().Unix()
	signed := fmt.Sprintf("%d.%s", timestamp, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	_, err := mac.Write([]byte(signed))
	require.NoError(t, err)
	return fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))
}

func createTestBusinessWithName(t *testing.T, name string) *database.Business {
	t.Helper()
	db := database.GetDB()
	biz := &database.Business{
		BusinessId: fmt.Sprintf("biz-%d", time.Now().UnixNano()),
		Name:       name,
	}
	require.NoError(t, db.Create(biz).Error)
	return biz
}

func createTestBillForBiz(t *testing.T, businessID uint) *database.Bill {
	t.Helper()
	db := database.GetDB()
	bill := &database.Bill{
		BusinessID:     businessID,
		BillNumber:     fmt.Sprintf("BILL-%d", time.Now().UnixNano()),
		Status:         database.BillStatusOpen,
		Items:          "[]",
		Subtotal:       1000,
		TotalAmount:    1000,
		SettlementAddr: "0xSETTLE",
	}
	require.NoError(t, db.Create(bill).Error)
	return bill
}

// TestHandlePaymentWebhook_RejectsCrossBusinessSettlement verifies that a
// webhook crafted with business A's secret but targeting business B's bill is
// rejected and does NOT settle B's bill. The test payment plugin is configured
// to drive a real settlement (Status:"completed", BillID:bB.ID) so that
// removing the PAY-1 guard would actually settle bB's bill. (PAY-1)
//
// Guard-removal proof: temporarily delete the `victimBill.BusinessID != businessID`
// block from handlePaymentWebhook. This test MUST fail — bB settles (Status=Paid,
// PaidAmount>0, Payment row exists). Restore the guard and the test passes again.
func TestHandlePaymentWebhook_RejectsCrossBusinessSettlement(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	bizA := createTestBusinessWithName(t, "AttackerBusiness")
	bizB := createTestBusinessWithName(t, "VictimBusiness")

	// Bill owned by B — this is the target.
	billB := createTestBillForBiz(t, bizB.ID)

	// Register the Stripe plugin row + business plugin config for business A
	// (the attacker, who owns a valid webhook secret).
	stripePlugin := &database.Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(stripePlugin).Error)

	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: bizA.ID,
		PluginID:   stripePlugin.ID,
		IsEnabled:  true,
		Config:     `{"webhook_secret":"whsec_attacker_secret_key"}`,
	}).Error)

	// Register a test payment plugin under "stripe" in the global registry so
	// handlePaymentWebhook can find it via GetPluginByName. The webhook
	// response drives a real settlement attempt on billB (completed status,
	// BillID set to billB's primary key) so that without the PAY-1 guard the
	// bill *would* settle — making the guard load-bearing.
	testPlugin := &testPaymentPlugin{
		name:            "stripe",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: "cs_test_cross_business_123",
			BillID:    billB.ID,
			Amount:    1000,
			Currency:  "usd",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(testPlugin)
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	// Craft a checkout.session.completed payload whose metadata claims
	// business_id = bizA (so signature verification uses A's secret) but
	// bill_id = billB (owned by bizB).
	payloadData := map[string]interface{}{
		"id":   "evt_test_cross_business_123",
		"type": "checkout.session.completed",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":           "cs_test_cross_business_123",
				"amount_total": float64(1000),
				"currency":     "usd",
				"metadata": map[string]interface{}{
					"bill_id":     fmt.Sprintf("%d", billB.ID),
					"business_id": fmt.Sprintf("%d", bizA.ID),
				},
			},
		},
	}
	payloadBytes, err := json.Marshal(payloadData)
	require.NoError(t, err)

	signature := signStripePayload(t, payloadBytes, "whsec_attacker_secret_key")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payloadBytes))
	c.Request.Header.Set("Stripe-Signature", signature)

	ph := &PluginHandlers{}
	ph.handlePaymentWebhook(c, "stripe")

	// Must NOT return 200 — the webhook must be rejected.
	assert.NotEqual(t, http.StatusOK, w.Code,
		"cross-business webhook must not return 200")

	// B's bill must remain unchanged (not paid).
	updatedBill, _, err := database.GetBillByID(billB.ID)
	require.NoError(t, err)
	assert.Equal(t, database.BillStatusOpen, updatedBill.Status,
		"victim bill must remain open, not be settled by cross-business webhook")
	assert.Equal(t, int64(0), updatedBill.PaidAmount,
		"victim bill paid_amount must remain 0")

	// No Payment row (with tx_hash = plugin_cs_test_cross_business_123)
	// should exist for B's bill.
	payment, _ := database.GetPaymentByTxHash("plugin_cs_test_cross_business_123")
	assert.Nil(t, payment, "no payment row must be created for cross-business webhook")
}

// TestHandlePaymentWebhook_AcceptsSameBusinessSettlement verifies the positive
// case: a webhook whose secret and bill belong to the same business continues
// to settle successfully. (PAY-1 companion)
func TestHandlePaymentWebhook_AcceptsSameBusinessSettlement(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	biz := createTestBusiness(t)

	// Bill owned by the same business, with enough remaining balance to pay.
	bill := createTestBillForBiz(t, biz.ID)

	stripePlugin := &database.Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(stripePlugin).Error)

	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: biz.ID,
		PluginID:   stripePlugin.ID,
		IsEnabled:  true,
		Config:     `{"webhook_secret":"whsec_same_biz_secret"}`,
	}).Error)

	testPlugin := &testPaymentPlugin{
		name:            "stripe",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: "cs_test_legitimate_123",
			BillID:    bill.ID,
			Amount:    1000,
			Currency:  "usd",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(testPlugin)
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	payloadData := map[string]interface{}{
		"id":   "evt_test_legitimate_123",
		"type": "checkout.session.completed",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":           "cs_test_legitimate_123",
				"amount_total": float64(1000),
				"currency":     "usd",
				"metadata": map[string]interface{}{
					"bill_id":     fmt.Sprintf("%d", bill.ID),
					"business_id": fmt.Sprintf("%d", biz.ID),
				},
			},
		},
	}
	payloadBytes, err := json.Marshal(payloadData)
	require.NoError(t, err)

	signature := signStripePayload(t, payloadBytes, "whsec_same_biz_secret")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payloadBytes))
	c.Request.Header.Set("Stripe-Signature", signature)

	ph := &PluginHandlers{}
	ph.handlePaymentWebhook(c, "stripe")

	assert.Equal(t, http.StatusOK, w.Code,
		"same-business webhook must succeed")
}
