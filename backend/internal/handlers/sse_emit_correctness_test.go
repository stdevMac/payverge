package handlers

// SSE emit-correctness regression tests for the payments flow.
//
// Contract under test:
//   - A guest payment REQUEST (pending, unconfirmed) must NOT emit
//     payment.received — operators were celebrating cash that never arrived.
//     It emits payment.pending instead.
//   - Confirmation paths (MarkAlternativePayment, the legacy payment webhook,
//     plugin settlements) MUST emit payment.received once the payment row is
//     actually recorded, plus bill.updated so dashboards refetch.
//   - Void/refund mutate the bill and must emit bill.updated.
//   - Plugin Telegram enqueues must pass ShouldEnqueueTelegramNotification
//     like every other notification source.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// collectHubEventTypes drains buffered hub events (plus any arriving within
// the grace window) and returns their types alongside raw payloads by type.
func collectHubEventTypes(ch <-chan events.BusinessEvent, grace time.Duration) ([]string, map[string][]string) {
	var types []string
	payloads := make(map[string][]string)
	for {
		select {
		case ev := <-ch:
			types = append(types, ev.Type)
			payloads[ev.Type] = append(payloads[ev.Type], string(ev.Data))
		case <-time.After(grace):
			return types, payloads
		}
	}
}

func TestRequestAlternativePayment_EmitsPendingNotReceived(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "alt-req-sse", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 30)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/request-alternative-payment", handler.RequestAlternativePayment)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/request-alternative-payment", bill.PublicToken), map[string]any{
			"amount":           "30.00",
			"payment_method":   "cash",
			"participant_name": "Guest",
		})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	types, _ := collectHubEventTypes(ch, 250*time.Millisecond)
	require.NotContains(t, types, "payment.received",
		"a pending guest payment request must not announce a received payment")
	require.Contains(t, types, "payment.pending",
		"the request should still surface as a distinct payment.pending event")
}

func TestMarkAlternativePayment_ManualConfirm_EmitsPaymentReceived(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(9)
	business := createPaymentRegressionBusiness(t, "alt-confirm-sse", &ownerID, "owner@example.com")
	openPaymentRegressionCashSession(t, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 20)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
			"participant_address":   "guest",
			"participant_name":      "Jane",
			"amount":                "20.00",
			"payment_method":        "cash",
			"business_confirmation": true,
		})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	types, _ := collectHubEventTypes(ch, 250*time.Millisecond)
	require.Contains(t, types, "payment.received",
		"manual alternative-payment confirmation records money and must emit payment.received")
	require.Contains(t, types, "bill.updated",
		"confirmed payment changes bill state and must emit bill.updated")
}

func TestMarkAlternativePayment_RequestConfirm_EmitsPaymentReceived(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(11)
	business := createPaymentRegressionBusiness(t, "alt-reqconfirm-sse", &ownerID, "owner2@example.com")
	openPaymentRegressionCashSession(t, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 15)

	pending := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Sam",
		Amount:          1500,
		BillAmountCents: 1500,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusPending,
	}
	require.NoError(t, database.GetDB().Create(pending).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner2@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
			"request_id":            pending.ID,
			"participant_address":   "guest",
			"amount":                "15.00",
			"payment_method":        "cash",
			"business_confirmation": true,
		})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	types, _ := collectHubEventTypes(ch, 250*time.Millisecond)
	require.Contains(t, types, "payment.received",
		"pending-request confirmation records money and must emit payment.received")
	require.Contains(t, types, "bill.updated")
}

func TestVoidBill_EmitsBillUpdated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)
	// VoidBill cancels the bill's pending orders in the same transaction.
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Order{},
		&database.InventorySettings{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.InventoryMovement{},
	))

	business := createPaymentRegressionBusiness(t, "void-sse", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 25)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", business.OwnerAddress)
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/void", handler.VoidBill)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/inside/bills/%d/void", bill.ID), map[string]any{
			"reason": "entered in error",
		})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	types, _ := collectHubEventTypes(ch, 250*time.Millisecond)
	require.Contains(t, types, "bill.updated", "voiding a bill must emit bill.updated")
}

func TestRefundBillAlternativePayment_EmitsBillUpdated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "refund-sse", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 25)
	now := time.Now()
	altPayment := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Jane",
		Amount:          2500,
		BillAmountCents: 2500,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusConfirmed,
		ConfirmedBy:     "owner",
		ConfirmedAt:     &now,
	}
	require.NoError(t, database.GetDB().Create(altPayment).Error)
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", bill.ID).Updates(map[string]any{
		"paid_amount": 2500,
		"status":      database.BillStatusPaid,
	}).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", business.OwnerAddress)
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/refund", handler.RefundBillPayment)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/inside/bills/%d/refund", bill.ID), map[string]any{
			"alternative_payment_id": altPayment.ID,
			"reason":                 "guest asked for refund",
		})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	types, _ := collectHubEventTypes(ch, 250*time.Millisecond)
	require.Contains(t, types, "bill.updated", "refunding a payment must emit bill.updated")
}

func TestPluginPayment_EmitsPaymentReceivedAndAmountedBillUpdated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "plugin-sse", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 30)

	ph := &PluginHandlers{}

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	// Partial payment: 10.00 of 30.00 — must announce the payment.
	_, applied, err := ph.updateBillPaymentStatus(bill.ID, "prov-partial-1", 1000, 0, "usd", "stripe", nil)
	require.NoError(t, err)
	require.True(t, applied)

	types, _ := collectHubEventTypes(ch, 250*time.Millisecond)
	require.Contains(t, types, "payment.received",
		"a recorded partial plugin payment must emit payment.received")
	require.Contains(t, types, "bill.updated",
		"a recorded partial plugin payment must emit bill.updated")

	// Full payment: remaining 20.00 — bill.updated payload must carry amounts.
	_, applied, err = ph.updateBillPaymentStatus(bill.ID, "prov-full-1", 2000, 0, "usd", "stripe", nil)
	require.NoError(t, err)
	require.True(t, applied)

	types, payloads := collectHubEventTypes(ch, 250*time.Millisecond)
	require.Contains(t, types, "payment.received")
	require.Contains(t, types, "bill.updated")
	var paidPayload string
	for _, p := range payloads["bill.updated"] {
		if strings.Contains(p, `"paid"`) {
			paidPayload = p
		}
	}
	require.NotEmpty(t, paidPayload, "expected a bill.updated payload for the paid transition")
	require.Contains(t, paidPayload, `"total_amount":30`,
		"paid bill.updated payload must include the bill amounts (dollars via MarshalJSON)")
	require.Contains(t, paidPayload, `"paid_amount":30`,
		"paid bill.updated payload must include the paid amount")
}

func TestEnqueueTelegramPluginPaymentNotification_RespectsEligibilityGate(t *testing.T) {
	setupPaymentRegressionDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
	))
	services.SetPluginDeliveryEnabled("telegram", true)
	services.ResetTelegramNotificationEligibilityCache()
	t.Cleanup(services.ResetTelegramNotificationEligibilityCache)

	business := createPaymentRegressionBusiness(t, "tg-gate-sse", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 30)

	countDeliveries := func() int64 {
		var n int64
		require.NoError(t, database.GetDB().Model(&database.PluginNotificationDelivery{}).Count(&n).Error)
		return n
	}

	// No telegram plugin configured → the enqueue must be skipped entirely.
	enqueueTelegramPaymentReceivedNotification(bill, "gate-pay-1", 1000, 0, "usd")
	require.EqualValues(t, 0, countDeliveries(),
		"business without a connected Telegram config must not get plugin payment outbox rows")

	// Connect Telegram → the enqueue goes through.
	plugin := database.Plugin{Name: "telegram", DisplayName: "Telegram", IsActive: true, Category: "integration"}
	require.NoError(t, database.GetDB().Where("name = ?", "telegram").FirstOrCreate(&plugin, database.Plugin{Name: "telegram"}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   plugin.ID,
		IsEnabled:  true,
		Config:     `{"is_connected":true,"chat_id":"12345"}`,
	}).Error)
	services.ResetTelegramNotificationEligibilityCache()

	enqueueTelegramPaymentReceivedNotification(bill, "gate-pay-2", 1000, 0, "usd")
	require.EqualValues(t, 1, countDeliveries(), "eligible business enqueues the plugin payment notification")
}
