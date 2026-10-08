package handlers

import (
	"bytes"
	"context"
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

// driveMercadoPagoCompletedWebhook drives a signature-verified "completed"
// MercadoPago webhook for the given bill/payment against handlePaymentWebhook,
// returning the recorder. The plugin double is registered under the real
// "mercadopago" name (in pluginWebhookConfigs) so verification passes with the
// seeded webhook_secret.
func driveMercadoPagoCompletedWebhook(t *testing.T, businessID, billID uint, paymentID string, billCents, tipCents int64, refundErr error) (*httptest.ResponseRecorder, *testPaymentPlugin) {
	t.Helper()
	mock := &testPaymentPlugin{
		name:            "mercadopago",
		verifySignature: true,
		refundErr:       refundErr,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    billID,
			PaymentID: paymentID,
			Amount:    billCents + tipCents,
			Currency:  "USD",
			Metadata: map[string]interface{}{
				"bill_amount_cents": fmt.Sprintf("%d", billCents),
				"tip_amount_cents":  fmt.Sprintf("%d", tipCents),
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mock)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/?bill_id=%d&business_id=%d", billID, businessID),
		bytes.NewBufferString(fmt.Sprintf(`{"id":"evt-%s","action":"payment.updated","data":{"id":"%s"}}`, paymentID, paymentID)),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")
	return w, mock
}

// TestHandlePaymentWebhook_TerminalBill_AutoRefundsAndAcks (H2 + F7): a late
// plugin capture that lands on an already-terminal (paid) bill must be
// auto-refunded, raise a refund-review alert, and — on refund success — ack 200
// rather than 409 (a 4xx would make the PSP retry and re-refund).
func TestHandlePaymentWebhook_TerminalBill_AutoRefundsAndAcks(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-terminal-%d", time.Now().UnixNano()),
		Status:      database.BillStatusPaid, // terminal — paid by cash meanwhile
		Items:       "[]",
		TotalAmount: 2000,
		PaidAmount:  2000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	w, mock := driveMercadoPagoCompletedWebhook(t, business.ID, bill.ID, "mp-terminal-1", 2000, 0, nil)

	assert.Equal(t, http.StatusOK, w.Code, "successful auto-refund of a terminal-bill capture must ack 200")
	assert.Contains(t, w.Body.String(), "auto-refunded")

	require.Equal(t, 1, mock.refundCalls, "terminal-bill capture must be auto-refunded")
	assert.Equal(t, "mp-terminal-1", mock.lastRefundPaymentID)
	assert.Equal(t, business.ID, mock.lastRefundBizID)

	// Bill must be untouched — the late capture never lands.
	refreshed, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, database.BillStatusPaid, refreshed.Status)
	assert.Equal(t, int64(2000), refreshed.PaidAmount)

	// No settlement Payment row was created for the refunded capture.
	_, lookupErr := database.GetPaymentByTxHash("plugin_mp-terminal-1")
	assert.Error(t, lookupErr, "terminal-bill capture must not produce a settlement payment")

	// A refund-review operational alert was raised for manual visibility.
	assert.Equal(t, int64(1), countRefundReviewAlerts(t, business.ID))
}

// TestHandlePaymentWebhook_TerminalBill_RefundFailureRetries (H2): when the
// auto-refund of a terminal-bill capture FAILS, keep the non-2xx response so the
// PSP retries, and raise a manual-action alert.
func TestHandlePaymentWebhook_TerminalBill_RefundFailureRetries(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-terminal-fail-%d", time.Now().UnixNano()),
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 2000,
		PaidAmount:  2000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	w, mock := driveMercadoPagoCompletedWebhook(t, business.ID, bill.ID, "mp-terminal-fail-1", 2000, 0, fmt.Errorf("psp refund unavailable"))

	assert.Equal(t, http.StatusConflict, w.Code, "failed auto-refund must keep a non-2xx so the PSP retries")
	require.Equal(t, 1, mock.refundCalls)
	assert.Equal(t, int64(1), countRefundReviewAlerts(t, business.ID))
}

// TestGetBusinessPaymentPlugins_OmitsOperatorHealthFields (FIX 7): the public
// guest endpoint must NOT leak operator health/error columns.
func TestGetBusinessPaymentPlugins_OmitsOperatorHealthFields(t *testing.T) {
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
		"enabled":      true,
		"access_token": "sk_test_guest_visible_xxxxxxxx",
	}))

	// Simulate a prior webhook failure that stamped the health/error columns.
	require.NoError(t, database.RecordBusinessPluginWebhookFailure(business.ID, "stripe", "SECRET provider webhook error text"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewPluginHandlers(nil, nil).GetBusinessPaymentPlugins(c)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Plugins, 1)

	for _, leaked := range []string{"last_status", "last_error", "last_error_at", "last_success_at", "config_schema"} {
		_, present := resp.Plugins[0][leaked]
		assert.Falsef(t, present, "guest payment-plugins response must not expose %q", leaked)
	}
	assert.Equal(t, "stripe", resp.Plugins[0]["name"])
	assert.Equal(t, true, resp.Plugins[0]["is_enabled"])
}

// TestVerifyPluginWebhookSignature_FirstPartyMissingConfigFailsClosed (FIX 8):
// a first-party PSP with no signature config must be rejected (fail closed),
// while an ad-hoc/non-first-party plugin keeps the tolerant path.
func TestVerifyPluginWebhookSignature_FirstPartyMissingConfigFailsClosed(t *testing.T) {
	setupHandlerTestDB(t)

	mock := &testPaymentPlugin{name: "stripe", verifySignature: true}

	// Temporarily remove stripe's webhook config so resolvePluginWebhookSignature
	// returns errPluginWebhookConfigUnknown for a FIRST-PARTY provider.
	saved, had := pluginWebhookConfigs["stripe"]
	delete(pluginWebhookConfigs, "stripe")
	t.Cleanup(func() {
		if had {
			pluginWebhookConfigs["stripe"] = saved
		}
	})

	err := verifyPluginWebhookSignature(mock, "stripe", 0, []byte("{}"), map[string]string{}, nil)
	require.Error(t, err, "a first-party PSP with no signature config must fail closed")

	// A non-first-party ad-hoc plugin keeps the tolerant (skip) path.
	adhoc := &testPaymentPlugin{name: "adhoc-provider"}
	require.NoError(t, verifyPluginWebhookSignature(adhoc, "adhoc-provider", 0, []byte("{}"), map[string]string{}, nil),
		"non-first-party plugins keep the tolerant path")
}

// TestHandlePaymentWebhook_RefundWithoutBillRefStillReverses (FIX 10): a
// signature-verified refund webhook whose payload carries NO bill reference must
// still reverse the local payment (resolved by txHash) instead of 200-acking a
// no-op and leaving the bill silently paid.
func TestHandlePaymentWebhook_RefundWithoutBillRefStillReverses(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-norefund-ref-%d", time.Now().UnixNano()),
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 1500,
		PaidAmount:  1500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	paymentID := "pay-no-billref-1"
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "plugin",
		Amount:        1500,
		TxHash:        "plugin_" + paymentID,
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "plugin",
	}).Error)

	// Register a refund webhook whose response carries BillID:0 (provider dropped
	// the bill reference on the refund object).
	pluginName := fmt.Sprintf("norefundref-%d", time.Now().UnixNano())
	mock := &testPaymentPlugin{
		name: pluginName,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "refunded",
			BillID:    0, // no bill reference
			PaymentID: paymentID,
			Amount:    1500,
			Currency:  "USD",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mock)
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin(pluginName) })

	originalSupport := guestBillPaymentPluginSupported
	guestBillPaymentPluginSupported = func(name string) bool { return name == pluginName }
	t.Cleanup(func() { guestBillPaymentPluginSupported = originalSupport })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"id":"evt-norefund-ref-1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

	assert.Equal(t, http.StatusOK, w.Code)

	// The payment must be reversed even though the webhook carried no bill id.
	payment, err := database.GetPaymentByTxHash("plugin_" + paymentID)
	require.NoError(t, err)
	assert.Equal(t, database.PaymentStatusReversed, payment.Status, "refund with no bill ref must still reverse via txHash")

	refreshed, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), refreshed.PaidAmount, "reversal must decrement the bill's paid amount")
}

// TestCreatePluginPayment_ClosedBusinessIsRejected (FIX 11): plugin checkout
// must enforce the same administrator-lifecycle gate as the native crypto
// path — a closed business cannot take plugin payments. (A suspended
// is_active=false venue already 404s at the bill-token resolver.)
func TestCreatePluginPayment_ClosedBusinessIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, bill, pluginName := setupPluginCheckoutAccessDB(t, nil)

	// Close the business (server administrator lifecycle lock).
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		UpdateColumn("closed_at", time.Now().Add(-time.Hour)).Error)

	body, err := json.Marshal(map[string]interface{}{
		"plugin_id": pluginName,
		"amount":    50,
		"currency":  "USD",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).CreatePluginPayment(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "business_unavailable")
}

// TestEnableBusinessPlugin_PreservesMaskedSecret (coordinator #1): enabling an
// already-configured plugin with the masked ••••••••  sentinel must NOT overwrite
// the real stored secret.
func TestEnableBusinessPlugin_PreservesMaskedSecret(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	business := createTestBusiness(t)
	plugin := &database.Plugin{
		Name:        "mercadopago",
		DisplayName: "MercadoPago",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	// Seed a stored config with a real secret.
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"access_token":   "APP_USR-real-secret-token",
		"base_url":       "https://api.mercadopago.com",
		"webhook_secret": "real-webhook-secret",
	}))

	// Re-enable carrying the masked sentinel for the secret fields.
	reqBody, err := json.Marshal(map[string]interface{}{
		"config": map[string]interface{}{
			"access_token":   "••••••••",
			"base_url":       "https://api.mercadopago.com",
			"webhook_secret": "••••••••",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "plugin_id", Value: fmt.Sprintf("%d", plugin.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).EnableBusinessPlugin(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	stored, err := database.GetBusinessPluginConfig(business.ID, "mercadopago")
	require.NoError(t, err)
	assert.Equal(t, "APP_USR-real-secret-token", stored["access_token"], "masked enable must not overwrite the stored access_token")
	assert.Equal(t, "real-webhook-secret", stored["webhook_secret"], "masked enable must not overwrite the stored webhook_secret")
}

// TestDeactivatePlugin_ActuallyDeactivates (coordinator #2): the admin Deactivate
// endpoint must persist is_active=false (GORM's Updates(struct) previously
// skipped the zero value, making it a no-op).
func TestDeactivatePlugin_ActuallyDeactivates(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}))

	plugin := &database.Plugin{
		Name:        "deactivate-me",
		DisplayName: "Deactivate Me",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(plugin).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", plugin.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	NewPluginHandlers(nil, nil).DeactivatePlugin(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	reloaded, err := database.GetPluginByID(plugin.ID)
	require.NoError(t, err)
	assert.False(t, reloaded.IsActive, "DeactivatePlugin must persist is_active=false")
}

// TestSuccessfulWebhook_ClearsHealthError (FIX 9): a successfully processed
// webhook clears a previously-stamped health error (sticky "Webhook error"
// badge). This locks the existing production behavior.
func TestSuccessfulWebhook_ClearsHealthError(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)

	// Stamp a prior failure.
	require.NoError(t, database.RecordBusinessPluginWebhookFailure(business.ID, "mercadopago", "prior boom"))

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-healthclear-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2000,
		PaidAmount:  0,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	w, _ := driveMercadoPagoCompletedWebhook(t, business.ID, bill.ID, "mp-healthclear-1", 2000, 0, nil)
	require.Equal(t, http.StatusOK, w.Code)

	health := mustLoadBusinessPluginHealth(t, business.ID, "mercadopago")
	assert.Equal(t, "ok", health["last_status"])
	if lastError, ok := health["last_error"]; ok && lastError != nil {
		assert.Equal(t, "", fmt.Sprintf("%v", lastError), "successful webhook must clear last_error")
	}
}

func mustLoadBusinessPluginHealth(t *testing.T, businessID uint, name string) map[string]interface{} {
	t.Helper()
	rows, err := database.GetBusinessPlugins(businessID)
	require.NoError(t, err)
	for _, row := range rows {
		if fmt.Sprintf("%v", row["name"]) == name {
			return row
		}
	}
	t.Fatalf("business plugin %q not found", name)
	return nil
}

// TestReconcilePendingPluginPayments (F9): the sweep settles stranded pending
// trackers that the provider reports terminal-completed, expires terminal-failed
// ones, and leaves recent / still-pending trackers untouched.
func TestReconcilePendingPluginPayments(t *testing.T) {
	setupHandlerTestDB(t)
	migrateSplitPluginTables(t)

	business := createTestBusiness(t)
	ph := &PluginHandlers{}
	old := time.Now().Add(-2 * time.Hour)

	// A stranded, provider-completed tracker → should settle.
	completedBill := &database.Bill{
		BusinessID: business.ID, BillNumber: fmt.Sprintf("B-recon-ok-%d", time.Now().UnixNano()),
		Status: database.BillStatusOpen, Items: "[]", Subtotal: 2000, TotalAmount: 2000,
	}
	require.NoError(t, database.GetDB().Create(completedBill).Error)
	seedReconTracker(t, completedBill.ID, "recon-completed", 2000, old)
	registerReconPlugin(t, "recon-completed", "completed")

	// A provider-failed tracker → should be expired (marked failed).
	failedBill := &database.Bill{
		BusinessID: business.ID, BillNumber: fmt.Sprintf("B-recon-fail-%d", time.Now().UnixNano()),
		Status: database.BillStatusOpen, Items: "[]", Subtotal: 2000, TotalAmount: 2000,
	}
	require.NoError(t, database.GetDB().Create(failedBill).Error)
	seedReconTracker(t, failedBill.ID, "recon-failed", 1500, old)
	registerReconPlugin(t, "recon-failed", "failed")

	// A RECENT completed tracker → must NOT be touched (age gate).
	recentBill := &database.Bill{
		BusinessID: business.ID, BillNumber: fmt.Sprintf("B-recon-recent-%d", time.Now().UnixNano()),
		Status: database.BillStatusOpen, Items: "[]", Subtotal: 2000, TotalAmount: 2000,
	}
	require.NoError(t, database.GetDB().Create(recentBill).Error)
	seedReconTracker(t, recentBill.ID, "recon-recent", 2000, time.Now())
	registerReconPlugin(t, "recon-recent", "completed")

	ph.ReconcilePendingPluginPayments(context.Background(), 50)

	// Completed tracker settled the bill.
	settledBill, _, err := database.GetBillByID(completedBill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2000), settledBill.PaidAmount, "reconciliation must settle a provider-completed stranded tracker")
	_, err = database.GetPaymentByTxHash("plugin_recon-completed")
	assert.NoError(t, err)

	// Failed tracker expired.
	var failedTracker database.AlternativePayment
	require.NoError(t, database.GetDB().Where("participant_addr = ?", "recon-failed").First(&failedTracker).Error)
	assert.Equal(t, database.AltPaymentStatusFailed, failedTracker.Status)

	// Recent tracker untouched.
	var recentTracker database.AlternativePayment
	require.NoError(t, database.GetDB().Where("participant_addr = ?", "recon-recent").First(&recentTracker).Error)
	assert.Equal(t, database.AltPaymentStatusPending, recentTracker.Status, "recent tracker must be left for a later sweep")
}

func seedReconTracker(t *testing.T, billID uint, providerPaymentID string, amountCents int64, createdAt time.Time) {
	t.Helper()
	tracker := &database.AlternativePayment{
		BillID:          billID,
		ParticipantAddr: providerPaymentID,
		ParticipantName: providerPaymentID,
		Amount:          amountCents,
		BillAmountCents: amountCents,
		PaymentMethod:   database.AlternativePaymentMethod(providerPaymentID),
		Status:          database.AltPaymentStatusPending,
	}
	require.NoError(t, database.GetDB().Create(tracker).Error)
	// Force created_at to the desired age (GORM stamps now on create).
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Where("id = ?", tracker.ID).
		UpdateColumn("created_at", createdAt).Error)
}

func registerReconPlugin(t *testing.T, name, status string) {
	t.Helper()
	plugin := &testPaymentPlugin{name: name, status: status}
	plugins.GlobalRegistry.RegisterPlugin(plugin)
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin(name) })
}

// migrateSplitPluginTables migrates the tables the split-share plugin flow
// touches and creates the alternative-payment idempotency index (F5) so the
// dedup guard is exercised the way it is in production.
func migrateSplitPluginTables(t *testing.T) {
	t.Helper()
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.AlternativePayment{},
		&database.BillSplitShare{},
	))
	require.NoError(t, database.GetDB().Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_alt_payments_bill_idem
			ON alternative_payments (bill_id, idempotency_key)
			WHERE idempotency_key <> ''`).Error)
}

// holdSplitShare creates a held custom-amount split share on a bill.
func holdSplitShare(t *testing.T, billID uint, guestSession string, amountCents int64) *database.BillSplitShare {
	t.Helper()
	share, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         billID,
		GuestSessionID: guestSession,
		DisplayName:    guestSession,
		Mode:           database.BillSplitModeCustom,
		AmountCents:    amountCents,
		HoldTTL:        10 * time.Minute,
		Now:            time.Now().UTC(),
	})
	require.NoError(t, err)
	return share
}

// TestUpdateBillPaymentStatus_DoublePaidSplitShare_ReversesSecondCapture (H3):
// a second confirmed plugin payment targeting an already-settled split share
// must be reversed (never bleed its surplus into the other guests' shares) and
// signal the double-pay sentinel so the webhook handler auto-refunds it.
func TestUpdateBillPaymentStatus_DoublePaidSplitShare_ReversesSecondCapture(t *testing.T) {
	setupHandlerTestDB(t)
	migrateSplitPluginTables(t)

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-doublepay-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		Subtotal:    2000,
		TotalAmount: 2000,
		PaidAmount:  0,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	share1 := holdSplitShare(t, bill.ID, "guest-A", 1000)
	holdSplitShare(t, bill.ID, "guest-B", 1000) // share2 stays owed

	ph := &PluginHandlers{}

	// First payment settles share1 (participant name encodes the share id).
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "pay1",
		ParticipantName: pluginTrackerParticipantName("mercadopago", &share1.ID),
		Amount:          1000,
		BillAmountCents: 1000,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
	}).Error)
	_, applied, err := ph.updateBillPaymentStatus(bill.ID, "pay1", 1000, 0, "usd", "stripe", nil)
	require.NoError(t, err)
	require.True(t, applied)

	// Second payment targets the SAME share (concurrent double-intent).
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "pay2",
		ParticipantName: pluginTrackerParticipantName("mercadopago", &share1.ID),
		Amount:          1000,
		BillAmountCents: 1000,
		PaymentMethod:   database.AlternativePaymentMethod("mercadopago"),
		Status:          database.AltPaymentStatusPending,
	}).Error)
	_, _, err = ph.updateBillPaymentStatus(bill.ID, "pay2", 1000, 0, "usd", "stripe", nil)
	require.ErrorIs(t, err, errPluginSplitShareDoublePaid, "second payment for a settled share must surface the double-pay sentinel")

	// The second capture must be reversed — it must NOT bleed into share2.
	pay2, err := database.GetPaymentByTxHash("plugin_pay2")
	require.NoError(t, err)
	assert.Equal(t, database.PaymentStatusReversed, pay2.Status)

	refreshed, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1000), refreshed.PaidAmount, "only share1's single payment should remain applied")

	// share2 must still be owed (unpaid), not silently paid down by the surplus.
	var share2 database.BillSplitShare
	require.NoError(t, database.GetDB().Where("bill_id = ? AND guest_session_id = ?", bill.ID, "guest-B").First(&share2).Error)
	assert.NotEqual(t, database.BillSplitShareStatusSettled, share2.Status, "the other guest's share must remain owed")

	// The double-paid tracker was flipped to failed.
	var tracker2 database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ? AND participant_addr = ?", bill.ID, "pay2").First(&tracker2).Error)
	assert.Equal(t, database.AltPaymentStatusFailed, tracker2.Status)
}

// TestHasPendingPluginTrackerForSplitShare (H3 intent guard): the pre-check
// detects an existing live pending tracker for the same split share.
func TestHasPendingPluginTrackerForSplitShare(t *testing.T) {
	setupHandlerTestDB(t)
	migrateSplitPluginTables(t)

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	shareID := uint(42)
	otherShareID := uint(7)

	// No trackers yet.
	has, err := hasPendingPluginTrackerForSplitShare(bill.ID, shareID)
	require.NoError(t, err)
	assert.False(t, has)

	// A pending tracker for a DIFFERENT share must not match.
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "payX",
		ParticipantName: pluginTrackerParticipantName("stripe", &otherShareID),
		Amount:          1000,
		PaymentMethod:   database.AlternativePaymentMethod("stripe"),
		Status:          database.AltPaymentStatusPending,
	}).Error)
	has, err = hasPendingPluginTrackerForSplitShare(bill.ID, shareID)
	require.NoError(t, err)
	assert.False(t, has)

	// A pending tracker for THIS share must match.
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "payY",
		ParticipantName: pluginTrackerParticipantName("stripe", &shareID),
		Amount:          1000,
		PaymentMethod:   database.AlternativePaymentMethod("stripe"),
		Status:          database.AltPaymentStatusPending,
	}).Error)
	has, err = hasPendingPluginTrackerForSplitShare(bill.ID, shareID)
	require.NoError(t, err)
	assert.True(t, has)

	// A CONFIRMED (non-pending) tracker for this share must not count as live.
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Where("participant_addr = ?", "payY").
		Update("status", database.AltPaymentStatusConfirmed).Error)
	has, err = hasPendingPluginTrackerForSplitShare(bill.ID, shareID)
	require.NoError(t, err)
	assert.False(t, has)
}

// TestStorePluginPaymentRecord_DedupesConcurrentInserts (F5): concurrent creates
// for the same provider payment id must collapse to a single tracker row via the
// deterministic idempotency key + partial unique index.
func TestStorePluginPaymentRecord_DedupesConcurrentInserts(t *testing.T) {
	setupHandlerTestDB(t)
	migrateSplitPluginTables(t)

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	ph := &PluginHandlers{}

	const workers = 6
	start := make(chan struct{})
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			<-start
			_, err := ph.storePluginPaymentRecord(bill.ID, business.ID, "stripe", "provider-pay-dedup", 1500, "USD", 1500, 0, nil)
			errs <- err
		}()
	}
	close(start)
	for i := 0; i < workers; i++ {
		require.NoError(t, <-errs)
	}

	var count int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Where("bill_id = ? AND participant_addr = ?", bill.ID, "provider-pay-dedup").
		Count(&count).Error)
	assert.Equal(t, int64(1), count, "concurrent plugin tracker inserts must dedupe to a single row")
}

// TestIsPartialPluginRefund_UsesFullCapturedBaseline (H9): the partial-vs-full
// classification must measure the refunded amount against the FULL capture
// (bill + tip), not the bill-only payment.Amount.
func TestIsPartialPluginRefund_UsesFullCapturedBaseline(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	txHash := "plugin_h9-refund"
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "plugin",
		Amount:        2000, // bill portion
		TipAmount:     300,  // tip portion; full capture = 2300
		TxHash:        txHash,
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "plugin",
	}).Error)

	ph := &PluginHandlers{}
	cents := func(v int64) *int64 { return &v }

	// Full refund of the whole capture (bill + tip) -> NOT partial (full reversal).
	assert.False(t, ph.isPartialPluginRefund(&plugins.WebhookResponse{
		Status: "refunded", PaymentID: "h9-refund", RefundedAmountCents: cents(2300),
	}, txHash), "refund of the full capture (bill+tip) must classify as full")

	// Refund of the bill only (tip retained) -> PARTIAL (2000 < 2300).
	assert.True(t, ph.isPartialPluginRefund(&plugins.WebhookResponse{
		Status: "refunded", PaymentID: "h9-refund", RefundedAmountCents: cents(2000),
	}, txHash), "a bill-only refund of a tip-inclusive capture must classify as partial")

	// Refund that exceeds the bill but is less than the full capture -> PARTIAL.
	assert.True(t, ph.isPartialPluginRefund(&plugins.WebhookResponse{
		Status: "refunded", PaymentID: "h9-refund", RefundedAmountCents: cents(2200),
	}, txHash), "a refund above bill-only but below full capture must classify as partial")

	// Legacy Amount fallback (no RefundedAmountCents): bill-only refund -> PARTIAL.
	assert.True(t, ph.isPartialPluginRefund(&plugins.WebhookResponse{
		Status: "refunded", PaymentID: "h9-refund", Amount: 2000,
	}, txHash), "Amount-fallback bill-only refund must classify as partial against full capture")

	// Legacy Amount fallback full capture -> NOT partial.
	assert.False(t, ph.isPartialPluginRefund(&plugins.WebhookResponse{
		Status: "refunded", PaymentID: "h9-refund", Amount: 2300,
	}, txHash), "Amount-fallback full-capture refund must classify as full")
}
