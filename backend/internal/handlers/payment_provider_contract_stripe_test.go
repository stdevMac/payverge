package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/paymentcontract"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/stripe"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func stripeContractAdapter() paymentcontract.Adapter {
	return providerContractAdapter{name: "stripe", verify: verifyStripeContractSignature, behavior: verifyStripeContractBehavior, rotation: verifyStripeContractRotation}
}

type providerContractWebhookDriver struct {
	name       string
	businessID uint
	send       func(*testing.T, string, string, string, uint, uint, int64, string, int64) *httptest.ResponseRecorder
	statusErr  func(*testing.T) error
	reconcile  func(*testing.T, uint, string) error
	trackerID  func(string) string
	refunded   func(string) error
}

func verifyStripeContractBehavior(t *testing.T, testCase paymentcontract.CaseSpec) error {
	t.Helper()
	return verifyProviderContractBehavior(t, testCase, newStripeContractWebhookDriver(t))
}

func newStripeContractWebhookDriver(t *testing.T) providerContractWebhookDriver {
	t.Helper()
	businessID := prepareProviderContractDatabase(t, "stripe", map[string]interface{}{
		"secret_key":      "sk_test_contract_123456789",
		"publishable_key": "pk_test_contract_123456789",
		"webhook_secret":  "whsec_contract_behavior",
	})
	plugins.GlobalRegistry.RegisterPlugin(stripe.NewStripePlugin(services.NewPluginService(database.GetDBWrapper())))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })
	var refundedPaymentID string
	originalTransport := http.DefaultTransport
	http.DefaultTransport = providerContractRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "api.stripe.com" && request.URL.Path == "/v1/refunds" {
			if err := request.ParseForm(); err != nil {
				return nil, err
			}
			refundedPaymentID = request.Form.Get("payment_intent")
			return providerContractHTTPResponse(http.StatusOK, `{"id":"re_contract_compensation","status":"succeeded"}`), nil
		}
		return originalTransport.RoundTrip(request)
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	return providerContractWebhookDriver{
		name:       "stripe",
		businessID: businessID,
		send:       sendStripeContractWebhook,
		trackerID:  func(paymentID string) string { return "cs_" + paymentID },
		refunded: func(paymentID string) error {
			if refundedPaymentID != paymentID {
				return fmt.Errorf("Stripe compensation refunded %q, want %q", refundedPaymentID, paymentID)
			}
			return nil
		},
		statusErr: func(t *testing.T) error {
			originalTransport := http.DefaultTransport
			http.DefaultTransport = providerContractRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Host != "api.stripe.com" {
					return nil, fmt.Errorf("unexpected Stripe timeout request %s", request.URL.String())
				}
				return nil, errors.New("simulated Stripe provider timeout")
			})
			defer func() { http.DefaultTransport = originalTransport }()
			returnStatus, err := stripe.NewStripePlugin(services.NewPluginService(database.GetDBWrapper())).GetPaymentStatus(businessID, "cs_pay-timeout")
			if returnStatus != "" {
				return fmt.Errorf("Stripe empty payment id returned status %q", returnStatus)
			}
			return err
		},
		reconcile: func(t *testing.T, billID uint, paymentID string) error {
			originalTransport := http.DefaultTransport
			http.DefaultTransport = providerContractRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Host != "api.stripe.com" || request.URL.Path != "/v1/checkout/sessions/"+paymentID {
					return nil, fmt.Errorf("unexpected Stripe reconciliation request %s", request.URL.String())
				}
				return providerContractHTTPResponse(http.StatusOK, `{"payment_status":"paid","status":"complete"}`), nil
			})
			t.Cleanup(func() { http.DefaultTransport = originalTransport })
			plugins.GlobalRegistry.RegisterPlugin(stripe.NewStripePlugin(services.NewPluginService(database.GetDBWrapper())))
			NewPluginHandlers(nil, nil).ReconcilePendingPluginPayments(t.Context(), 10)
			return nil
		},
	}
}

type providerContractRoundTripFunc func(*http.Request) (*http.Response, error)

func (f providerContractRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func providerContractHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func sendStripeContractWebhook(t *testing.T, kind, eventID, paymentID string, signingBusinessID, billID uint, amount int64, currency string, expectedBillAmount int64) *httptest.ResponseRecorder {
	t.Helper()
	eventType := "checkout.session.completed"
	paymentStatus := "paid"
	objectID := "cs_" + paymentID
	switch kind {
	case "pending":
		paymentStatus = "unpaid"
	case "failed":
		eventType = "payment_intent.payment_failed"
		objectID = paymentID
	case "reversal":
		eventType = "charge.dispute.funds_withdrawn"
		objectID = "dp_" + paymentID
	case "refund":
		eventType = "charge.refunded"
		objectID = "ch_" + paymentID
	case "dispute":
		eventType = "charge.dispute.created"
		objectID = "dp_" + paymentID
	case "cancellation":
		eventType = "checkout.session.async_payment_failed"
	case "expiration":
		eventType = "checkout.session.expired"
	}
	metadata := map[string]interface{}{
		"bill_id":     fmt.Sprintf("%d", billID),
		"business_id": fmt.Sprintf("%d", signingBusinessID),
	}
	if expectedBillAmount >= 0 {
		metadata["bill_amount_cents"] = fmt.Sprintf("%d", expectedBillAmount)
		metadata["tip_amount_cents"] = "0"
	}
	payload, err := json.Marshal(map[string]interface{}{
		"id":   eventID,
		"type": eventType,
		"data": map[string]interface{}{"object": map[string]interface{}{
			"id": objectID, "payment_intent": paymentID, "amount_total": amount, "currency": strings.ToLower(currency),
			"payment_status": paymentStatus, "metadata": metadata,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	signingSecret := "whsec_contract_behavior"
	if kind == "wrong_account" {
		signingSecret = "whsec_different_provider_account"
	}
	c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, signingSecret))
	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")
	return w
}

func prepareProviderContractDatabase(t *testing.T, provider string, config map[string]interface{}) uint {
	t.Helper()
	setupHandlerTestDB(t)
	db := database.GetDB()
	if err := db.AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}); err != nil {
		t.Fatal(err)
	}
	business := createTestBusiness(t)
	if err := db.Model(business).Update("default_currency", "USD").Error; err != nil {
		t.Fatal(err)
	}
	plugin := &database.Plugin{Name: provider, DisplayName: provider, Category: database.PluginCategoryPayment, IsActive: true}
	if err := db.Create(plugin).Error; err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.BusinessPlugin{BusinessID: business.ID, PluginID: plugin.ID, IsEnabled: true, Config: string(encoded)}).Error; err != nil {
		t.Fatal(err)
	}
	return business.ID
}

func newProviderContractBill(t *testing.T, businessID uint, total int64) *database.Bill {
	t.Helper()
	bill := &database.Bill{
		BusinessID: businessID, BillNumber: fmt.Sprintf("CONTRACT-%d", time.Now().UnixNano()),
		Status: database.BillStatusOpen, Items: "[]", Subtotal: total, TotalAmount: total,
	}
	if err := database.GetDB().Create(bill).Error; err != nil {
		t.Fatal(err)
	}
	return bill
}

func loadProviderContractBill(id uint) (*database.Bill, error) {
	bill, _, err := database.GetBillByIDLean(id)
	return bill, err
}

func verifyProviderContractBehavior(t *testing.T, testCase paymentcontract.CaseSpec, driver providerContractWebhookDriver) error {
	t.Helper()
	bill := newProviderContractBill(t, driver.businessID, 1000)
	send := func(kind, eventID, paymentID string, businessID, billID uint, amount int64, currency string, expected int64) *httptest.ResponseRecorder {
		return driver.send(t, kind, eventID, paymentID, businessID, billID, amount, currency, expected)
	}
	assertBill := func(id uint, status database.BillStatus, paid int64) error {
		current, err := loadProviderContractBill(id)
		if err != nil {
			return err
		}
		if current.Status != status || current.PaidAmount != paid {
			return fmt.Errorf("bill %d state = (%s,%d), want (%s,%d)", id, current.Status, current.PaidAmount, status, paid)
		}
		return nil
	}
	countPayment := func(paymentID string) (int64, error) {
		var count int64
		err := database.GetDB().Model(&database.Payment{}).Where("tx_hash = ?", "plugin_"+paymentID).Count(&count).Error
		return count, err
	}
	assertUnsettledCompensated := func(paymentID string) error {
		if err := assertBill(bill.ID, database.BillStatusOpen, 0); err != nil {
			return err
		}
		count, err := countPayment(paymentID)
		if err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("unsettleable capture %q wrote %d local payments", paymentID, count)
		}
		if driver.refunded == nil {
			return errors.New("provider driver cannot observe capture compensation")
		}
		return driver.refunded(paymentID)
	}

	switch testCase.ID {
	case "replay/duplicate_event":
		first := send("completed", "evt-duplicate", "pay-duplicate", driver.businessID, bill.ID, 1000, "USD", 1000)
		second := send("completed", "evt-duplicate", "pay-duplicate", driver.businessID, bill.ID, 1000, "USD", 1000)
		if first.Code != http.StatusOK || second.Code != http.StatusOK {
			return fmt.Errorf("duplicate delivery status = %d/%d", first.Code, second.Code)
		}
		if err := assertBill(bill.ID, database.BillStatusPaid, 1000); err != nil {
			return err
		}
		count, err := countPayment("pay-duplicate")
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("duplicate delivery payment count = %d, want 1", count)
		}
		return nil
	case "replay/replayed_payment":
		first := send("completed", "evt-replay-1", "pay-replay", driver.businessID, bill.ID, 1000, "USD", 1000)
		second := send("completed", "evt-replay-2", "pay-replay", driver.businessID, bill.ID, 1000, "USD", 1000)
		if first.Code != http.StatusOK || second.Code != http.StatusOK {
			return fmt.Errorf("payment replay status = %d/%d", first.Code, second.Code)
		}
		count, err := countPayment("pay-replay")
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("payment replay count = %d, want 1", count)
		}
		return assertBill(bill.ID, database.BillStatusPaid, 1000)
	case "replay/delayed_completion":
		pending := send("pending", "evt-delayed-1", "pay-delayed", driver.businessID, bill.ID, 1000, "USD", 1000)
		if pending.Code != http.StatusOK {
			return fmt.Errorf("pending delivery status = %d", pending.Code)
		}
		if err := assertBill(bill.ID, database.BillStatusOpen, 0); err != nil {
			return err
		}
		completed := send("completed", "evt-delayed-2", "pay-delayed", driver.businessID, bill.ID, 1000, "USD", 1000)
		if completed.Code != http.StatusOK {
			return fmt.Errorf("delayed completion status = %d", completed.Code)
		}
		return assertBill(bill.ID, database.BillStatusPaid, 1000)
	case "replay/out_of_order":
		if got := send("completed", "evt-order-2", "pay-order", driver.businessID, bill.ID, 1000, "USD", 1000); got.Code != http.StatusOK {
			return fmt.Errorf("completion status = %d", got.Code)
		}
		if got := send("pending", "evt-order-1", "pay-order", driver.businessID, bill.ID, 1000, "USD", 1000); got.Code != http.StatusOK {
			return fmt.Errorf("late pending status = %d", got.Code)
		}
		return assertBill(bill.ID, database.BillStatusPaid, 1000)
	case "binding/wrong_business":
		other := createTestBusinessWithName(t, "contract-other-business")
		otherBill := newProviderContractBill(t, other.ID, 1000)
		got := send("completed", "evt-wrong-business", "pay-wrong-business", driver.businessID, otherBill.ID, 1000, "USD", 1000)
		if got.Code != http.StatusForbidden {
			return fmt.Errorf("wrong-business status = %d, want 403", got.Code)
		}
		return assertBill(otherBill.ID, database.BillStatusOpen, 0)
	case "binding/wrong_bill":
		other := newProviderContractBill(t, driver.businessID, 1000)
		if got := send("completed", "evt-bill-1", "pay-wrong-bill", driver.businessID, bill.ID, 1000, "USD", 1000); got.Code != http.StatusOK {
			return fmt.Errorf("first bill status = %d", got.Code)
		}
		_ = send("completed", "evt-bill-2", "pay-wrong-bill", driver.businessID, other.ID, 1000, "USD", 1000)
		if err := assertBill(other.ID, database.BillStatusOpen, 0); err != nil {
			return err
		}
		if err := assertBill(bill.ID, database.BillStatusPaid, 1000); err != nil {
			return err
		}
		count, err := countPayment("pay-wrong-bill")
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("wrong-bill replay payment count = %d, want 1", count)
		}
		return nil
	case "binding/wrong_provider_account":
		// The signature/webhook-id is the provider-account binding in both real adapters.
		got := driver.send(t, "wrong_account", "evt-wrong-account", "pay-wrong-account", driver.businessID, bill.ID, 1000, "USD", 1000)
		if got.Code != http.StatusUnauthorized {
			return fmt.Errorf("wrong-provider-account status = %d, want 401", got.Code)
		}
		return assertBill(bill.ID, database.BillStatusOpen, 0)
	case "amount_currency/wrong_amount":
		// PayPal does not echo the order's requested amount as metadata, so the
		// production handler binds it through the persisted checkout tracker.
		// Stripe echoes the same value, but seeding the tracker also proves both
		// adapters use the locally committed amount as the source of truth.
		seedProviderContractTracker(t, bill.ID, driver.name, "pay-wrong-amount", 1000, database.AltPaymentStatusPending)
		got := send("completed", "evt-wrong-amount", "pay-wrong-amount", driver.businessID, bill.ID, 900, "USD", 1000)
		if got.Code != http.StatusOK {
			return fmt.Errorf("wrong-amount compensation status = %d", got.Code)
		}
		return assertUnsettledCompensated("pay-wrong-amount")
	case "amount_currency/wrong_currency":
		got := send("completed", "evt-wrong-currency", "pay-wrong-currency", driver.businessID, bill.ID, 1000, "EUR", 1000)
		if got.Code != http.StatusOK {
			return fmt.Errorf("wrong-currency compensation status = %d", got.Code)
		}
		return assertUnsettledCompensated("pay-wrong-currency")
	case "under_overpayment/underpayment":
		got := send("completed", "evt-under", "pay-under", driver.businessID, bill.ID, 400, "USD", -1)
		if got.Code != http.StatusOK {
			return fmt.Errorf("underpayment status = %d", got.Code)
		}
		return assertBill(bill.ID, database.BillStatusPartial, 400)
	case "under_overpayment/overpayment":
		got := send("completed", "evt-over", "pay-over", driver.businessID, bill.ID, 1100, "USD", -1)
		if got.Code != http.StatusOK {
			return fmt.Errorf("overpayment compensation status = %d", got.Code)
		}
		return assertUnsettledCompensated("pay-over")
	case "lifecycle/refund":
		if got := send("completed", "evt-refund-capture", "pay-refund", driver.businessID, bill.ID, 1000, "USD", 1000); got.Code != http.StatusOK {
			return fmt.Errorf("capture status = %d", got.Code)
		}
		got := send("refund", "evt-refund", "pay-refund", driver.businessID, bill.ID, 1000, "USD", -1)
		if got.Code != http.StatusOK {
			return fmt.Errorf("refund status = %d", got.Code)
		}
		return assertBill(bill.ID, database.BillStatusOpen, 0)
	case "lifecycle/reversal":
		if got := send("completed", "evt-reverse-capture", "pay-reverse", driver.businessID, bill.ID, 1000, "USD", 1000); got.Code != http.StatusOK {
			return fmt.Errorf("capture status = %d", got.Code)
		}
		if got := send("reversal", "evt-reverse", "pay-reverse", driver.businessID, bill.ID, 1000, "USD", -1); got.Code != http.StatusOK {
			return fmt.Errorf("reversal status = %d", got.Code)
		}
		return assertBill(bill.ID, database.BillStatusOpen, 0)
	case "lifecycle/dispute":
		if got := send("completed", "evt-dispute-capture", "pay-dispute", driver.businessID, bill.ID, 1000, "USD", 1000); got.Code != http.StatusOK {
			return fmt.Errorf("capture status = %d", got.Code)
		}
		if got := send("dispute", "evt-dispute", "pay-dispute", driver.businessID, bill.ID, 1000, "USD", -1); got.Code != http.StatusOK {
			return fmt.Errorf("dispute status = %d", got.Code)
		}
		if err := assertBill(bill.ID, database.BillStatusPaid, 1000); err != nil {
			return fmt.Errorf("dispute-created must preserve captured money until loss: %w", err)
		}
		var alerts int64
		if err := database.GetDB().Model(&database.OperationalAlert{}).
			Where("business_id = ? AND alert_type = ?", driver.businessID, database.OperationalAlertTypePaymentRefundReview).
			Count(&alerts).Error; err != nil {
			return err
		}
		if alerts == 0 {
			return errors.New("dispute-created produced no durable operator alert")
		}
		return nil
	case "lifecycle/cancellation":
		cancelTrackerID := "pay-cancel"
		if driver.trackerID != nil {
			cancelTrackerID = driver.trackerID(cancelTrackerID)
		}
		seedProviderContractTracker(t, bill.ID, driver.name, cancelTrackerID, 1000, database.AltPaymentStatusPending)
		if got := send("cancellation", "evt-cancel", "pay-cancel", driver.businessID, bill.ID, 1000, "USD", -1); got.Code != http.StatusOK {
			return fmt.Errorf("cancellation status = %d", got.Code)
		}
		if err := assertProviderContractTrackerTerminal(bill.ID, cancelTrackerID); err != nil {
			return err
		}
		paidBill := newProviderContractBill(t, driver.businessID, 1000)
		if got := send("completed", "evt-cancel-paid", "pay-cancel-paid", driver.businessID, paidBill.ID, 1000, "USD", 1000); got.Code != http.StatusOK {
			return fmt.Errorf("capture status = %d", got.Code)
		}
		if got := send("cancellation", "evt-cancel-late", "pay-cancel-paid", driver.businessID, paidBill.ID, 1000, "USD", -1); got.Code != http.StatusOK {
			return fmt.Errorf("late cancellation status = %d", got.Code)
		}
		return assertBill(paidBill.ID, database.BillStatusPaid, 1000)
	case "lifecycle/expiration":
		expiryTrackerID := "pay-expire"
		if driver.trackerID != nil {
			expiryTrackerID = driver.trackerID(expiryTrackerID)
		}
		seedProviderContractTracker(t, bill.ID, driver.name, expiryTrackerID, 1000, database.AltPaymentStatusPending)
		if got := send("expiration", "evt-expire", "pay-expire", driver.businessID, bill.ID, 1000, "USD", -1); got.Code != http.StatusOK {
			return fmt.Errorf("expiration status = %d", got.Code)
		}
		if err := assertProviderContractTrackerTerminal(bill.ID, expiryTrackerID); err != nil {
			return err
		}
		paidBill := newProviderContractBill(t, driver.businessID, 1000)
		if got := send("completed", "evt-expire-paid", "pay-expire-paid", driver.businessID, paidBill.ID, 1000, "USD", 1000); got.Code != http.StatusOK {
			return fmt.Errorf("capture status = %d", got.Code)
		}
		if got := send("expiration", "evt-expire-late", "pay-expire-paid", driver.businessID, paidBill.ID, 1000, "USD", -1); got.Code != http.StatusOK {
			return fmt.Errorf("late expiration status = %d", got.Code)
		}
		return assertBill(paidBill.ID, database.BillStatusPaid, 1000)
	case "lifecycle/provider_timeout":
		timeoutTrackerID := "pay-timeout"
		if driver.trackerID != nil {
			timeoutTrackerID = driver.trackerID(timeoutTrackerID)
		}
		seedProviderContractTracker(t, bill.ID, driver.name, timeoutTrackerID, 1000, database.AltPaymentStatusPending)
		if err := driver.statusErr(t); err == nil {
			return errors.New("provider timeout/error did not fail closed")
		}
		if err := assertBill(bill.ID, database.BillStatusOpen, 0); err != nil {
			return err
		}
		var tracker database.AlternativePayment
		if err := database.GetDB().Where("bill_id = ? AND participant_addr = ?", bill.ID, timeoutTrackerID).First(&tracker).Error; err != nil {
			return err
		}
		if tracker.Status != database.AltPaymentStatusPending {
			return fmt.Errorf("provider timeout changed pending tracker to %s", tracker.Status)
		}
		return nil
	case "lifecycle/local_timeout_after_provider_success":
		trackerID := "pay-late-success"
		if driver.trackerID != nil {
			trackerID = driver.trackerID(trackerID)
		}
		seedProviderContractTracker(t, bill.ID, driver.name, trackerID, 1000, database.AltPaymentStatusFailed)
		if got := send("completed", "evt-late-success", "pay-late-success", driver.businessID, bill.ID, 1000, "USD", 1000); got.Code != http.StatusOK {
			return fmt.Errorf("late success status = %d", got.Code)
		}
		if err := assertBill(bill.ID, database.BillStatusPaid, 1000); err != nil {
			return err
		}
		var tracker database.AlternativePayment
		if err := database.GetDB().Where("bill_id = ? AND participant_addr = ?", bill.ID, "pay-late-success").First(&tracker).Error; err != nil {
			return err
		}
		if tracker.Status != database.AltPaymentStatusConfirmed {
			return fmt.Errorf("late-success tracker remained %s", tracker.Status)
		}
		return nil
	case "reconciliation/mismatch_repaired":
		seedProviderContractTracker(t, bill.ID, driver.name, "pay-repair", 1000, database.AltPaymentStatusPending)
		if err := database.GetDB().Model(&database.AlternativePayment{}).
			Where("bill_id = ? AND participant_addr = ?", bill.ID, "pay-repair").
			UpdateColumn("created_at", time.Now().Add(-time.Hour)).Error; err != nil {
			return err
		}
		if driver.reconcile == nil {
			return errors.New("provider has no reconciliation probe")
		}
		if err := driver.reconcile(t, bill.ID, "pay-repair"); err != nil {
			return err
		}
		if err := assertBill(bill.ID, database.BillStatusPaid, 1000); err != nil {
			return err
		}
		var tracker database.AlternativePayment
		if err := database.GetDB().Where("bill_id = ? AND participant_addr = ?", bill.ID, "pay-repair").First(&tracker).Error; err != nil {
			return err
		}
		if tracker.Status != database.AltPaymentStatusConfirmed {
			return fmt.Errorf("tracker remained %s", tracker.Status)
		}
		count, err := countPayment("pay-repair")
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("repaired payment count = %d, want 1", count)
		}
		return nil
	default:
		return fmt.Errorf("unknown %s behavior case %q", driver.name, testCase.ID)
	}
}

func seedProviderContractTracker(t *testing.T, billID uint, provider, paymentID string, amount int64, status database.AlternativePaymentStatus) {
	t.Helper()
	tracker := &database.AlternativePayment{
		BillID: billID, ParticipantAddr: paymentID, ParticipantName: provider,
		Amount: amount, BillAmountCents: amount, PaymentMethod: database.AlternativePaymentMethod(provider), Status: status,
	}
	if err := database.GetDB().Create(tracker).Error; err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatal(err)
	}
}

func assertProviderContractTrackerTerminal(billID uint, paymentID string) error {
	var tracker database.AlternativePayment
	if err := database.GetDB().Where("bill_id = ? AND participant_addr = ?", billID, paymentID).First(&tracker).Error; err != nil {
		return err
	}
	switch tracker.Status {
	case database.AltPaymentStatusFailed, database.AltPaymentStatusCancelled, database.AltPaymentStatusExpired:
		return nil
	default:
		return fmt.Errorf("pending attempt %s remained %s", paymentID, tracker.Status)
	}
}

func verifyStripeContractSignature(t *testing.T, testCase paymentcontract.SignatureCase) error {
	t.Helper()
	payload := []byte(`{"id":"evt_contract","type":"checkout.session.completed"}`)
	secret := "whsec_contract_current"
	signature := ""
	switch testCase {
	case paymentcontract.SignatureMissing:
	case paymentcontract.SignatureInvalid:
		signature = fmt.Sprintf("t=%d,v1=deadbeef", time.Now().Unix())
	case paymentcontract.SignatureValid:
		timestamp := time.Now().Unix()
		signed := fmt.Sprintf("%d.%s", timestamp, payload)
		signature = fmt.Sprintf("t=%d,v1=%s", timestamp, contractHMACHex(secret, []byte(signed)))
	}
	if stripe.NewStripePlugin(nil).VerifyWebhookSignature(payload, signature, secret) {
		return nil
	}
	return errors.New("stripe signature rejected")
}

func verifyStripeContractRotation(t *testing.T) paymentcontract.SecretRotationObservation {
	t.Helper()
	payload := []byte(`{"id":"evt_rotation"}`)
	previous := "whsec_contract_previous"
	timestamp := time.Now().Unix()
	signature := fmt.Sprintf("t=%d,v1=%s", timestamp, contractHMACHex(previous, []byte(fmt.Sprintf("%d.%s", timestamp, payload))))
	plugin := stripe.NewStripePlugin(nil)
	return paymentcontract.SecretRotationObservation{
		PreviousConfigured: verifyWebhookSignatureWithSecrets(plugin, payload, signature, []string{"whsec_contract_current", previous}),
		PreviousRetired:    verifyWebhookSignatureWithSecrets(plugin, payload, signature, []string{"whsec_contract_current"}),
	}
}

func contractHMACHex(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
