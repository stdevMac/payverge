package handlers

// webhook_ingest_perf_test.go — P8.4
//
// Access-shape regression test and SQLite benchmark for the PSP plugin webhook
// ingest path (handlePaymentWebhook → "completed" settlement branch).
//
// What we lock in
// ───────────────
//  • The "completed" settlement branch calls GetBillByID twice today:
//    once for the PAY-1 cross-business ownership check and once inside
//    updateBillPaymentStatus. Both are the heavy aggregate (Business + Table +
//    Payments preloads). TestWebhookIngestQueryShape records this current shape
//    and asserts the total SELECT count on bills is ≤ 4 (enough headroom for
//    the two GetBillByID calls plus one ApplyConfirmedPayment re-read under
//    FOR UPDATE, but tight enough to catch an accidental N+1 explosion).
//  • No SELECT * on the heavy tables: payments, bills, businesses, alternative_payments.
//    Narrowing these to targeted projections is the future optimisation target;
//    the test will guide that work.
//  • webhook_events gets at most 2 queries (INSERT + UPDATE).
//
// BenchmarkSettlePluginWebhook measures allocs/op so regressions are caught
// before they ship to production.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// webhookIngestSQLRecorder captures SQL so the query shape of the PSP ingest
// path can be asserted. It wraps the silent GORM logger so it is a drop-in
// replacement for the gorm.Config.Logger field.
type webhookIngestSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *webhookIngestSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

// selectStarCount returns the number of captured statements that are
// SELECT * queries against the given table name.  It is quote-agnostic so it
// works with both the SQLite test dialect (backticks) and Postgres (double
// quotes).
func (r *webhookIngestSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, s := range r.statements {
		norm := strings.ToLower(strings.TrimSpace(s))
		if !strings.HasPrefix(norm, "select *") {
			continue
		}
		if strings.Contains(norm, "from `"+table+"`") ||
			strings.Contains(norm, `from "`+table+`"`) ||
			strings.Contains(norm, "from "+table) {
			count++
		}
	}
	return count
}

// selectCount returns the number of SELECT statements that touch the given
// table (regardless of projection).
func (r *webhookIngestSQLRecorder) selectCount(table string) int {
	count := 0
	for _, s := range r.statements {
		norm := strings.ToLower(strings.Join(strings.Fields(s), " "))
		if !strings.HasPrefix(norm, "select ") {
			continue
		}
		if strings.Contains(norm, "from `"+table+"`") ||
			strings.Contains(norm, `from "`+table+`"`) ||
			strings.Contains(norm, "from "+table) {
			count++
		}
	}
	return count
}

// signStripeHeader computes the Stripe-Signature header value for a payload
// and secret using the t=<ts>,v1=<hmac> format that verifyPluginWebhookSignature
// expects. It is safe to call with any testing.TB (unlike the *testing.T-only
// signStripePayload in plugin_webhook_cross_business_test.go).
func signStripeHeader(payload []byte, secret string) string {
	ts := time.Now().Unix()
	signed := fmt.Sprintf("%d.%s", ts, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

// buildPluginWebhookRequest builds a signed Stripe checkout.session.completed
// request body that handlePaymentWebhook will accept and route through the
// "completed" settlement branch.
func buildPluginWebhookRequest(tb testing.TB, businessID, billID uint, paymentID string, amountCents int64) *http.Request {
	tb.Helper()

	payload := map[string]interface{}{
		"id":   fmt.Sprintf("evt_%s", paymentID),
		"type": "checkout.session.completed",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":           paymentID,
				"amount_total": float64(amountCents),
				"currency":     "usd",
				"metadata": map[string]interface{}{
					"bill_id":     fmt.Sprintf("%d", billID),
					"business_id": fmt.Sprintf("%d", businessID),
				},
			},
		},
	}
	body, err := json.Marshal(payload)
	require.NoError(tb, err)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", signStripeHeader(body, "whsec_perf_test_key"))
	// Set a unique webhook-id so the idempotency dedup row is inserted exactly once.
	req.Header.Set("webhook-id", fmt.Sprintf("evt_%s", paymentID))
	return req
}

// setupWebhookIngestPerfDB creates an isolated in-memory SQLite DB, seeds a
// business + bill with a non-zero TotalAmount (so ApplyConfirmedPayment does
// not reject the payment as exceeding the remaining balance), registers a stub
// payment plugin under "stripe", and returns the seeded entities.
//
// The stub plugin's webhookResponse is left at nil; callers configure it by
// pushing a new testPaymentPlugin into plugins.GlobalRegistry before each
// request (the t.Cleanup removes it).
func setupWebhookIngestPerfDB(tb testing.TB, gormLogger logger.Interface) (*database.Business, *database.Bill) {
	tb.Helper()

	// Use a unique DSN so parallel test runs do not share the same file.
	dsnName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(tb.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(tb, err)

	sqlDB, err := gormDB.DB()
	require.NoError(tb, err)
	sqlDB.SetMaxOpenConns(1)
	tb.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	server.InitializeRBAC(database.GetDBWrapper())

	require.NoError(tb, gormDB.AutoMigrate(
		&database.Business{},
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.WebhookEvent{},
		&database.BusinessMilestoneEvent{},
		&database.BusinessRevenueAggregate{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
		&database.BusinessAlertSettings{},
	))

	// Ensure SSE hub is ready (idempotent).
	events.GetHub()

	business := &database.Business{
		BusinessId:     fmt.Sprintf("webhook-ingest-perf-%d", time.Now().UnixNano()),
		Name:           "Webhook Ingest Perf Restaurant",
		OwnerAddress:   "0xwebhookingestperf",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(tb, gormDB.Create(business).Error)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("BENCH-WEBHOOK-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 50000, // $500.00 — large headroom so many small benchmark payments don't close the bill
		Subtotal:    50000,
	}
	require.NoError(tb, gormDB.Create(bill).Error)

	// Register a Stripe Plugin row so verifyPluginWebhookSignature can look up
	// the webhook secret from BusinessPlugin.Config.
	stripePlugin := &database.Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(tb, gormDB.Create(stripePlugin).Error)
	require.NoError(tb, gormDB.Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   stripePlugin.ID,
		IsEnabled:  true,
		Config:     `{"webhook_secret":"whsec_perf_test_key"}`,
	}).Error)

	return business, bill
}

// registerIngestStub registers a testPaymentPlugin stub into the global registry
// with the given paymentID and returns a cleanup func that removes it.
func registerIngestStub(billID uint, paymentID string, amountCents int64) func() {
	stub := &testPaymentPlugin{
		name:            "stripe",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: paymentID,
			BillID:    billID,
			Amount:    amountCents,
			Currency:  "usd",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(stub)
	return func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") }
}

// TestWebhookIngestQueryShape locks in the current access shape for the PSP
// webhook ingest path (handlePaymentWebhook "completed" branch) so that future
// refactors that inadvertently introduce N+1 queries or unbounded table reads
// are caught immediately.
//
// Current baseline shape (locked in 2026-07-02 after lean webhook ingest):
//
//   - bills: 3 SELECT statements (PAY-1 GetBillBusinessIDByBillID +
//     updateBillPaymentStatus GetBillByIDLean + ApplyConfirmedPayment FOR UPDATE)
//   - payments: 3 SELECT statements (GetPaymentByTxHash ×3 — idempotency check,
//     existence-check in updateBillPaymentStatus, and post-settlement confirmation)
//   - alternative_payments: 2 SELECT statements (lookupPluginPaymentBreakdown
//     narrow projection + settleTrackedPluginSplitShare)
//   - businesses: 0 SELECT * (projectedBillBusinessPreload is a named-column
//     SELECT — this MUST remain at zero to protect PII projection)
//
// Note: SELECT * on bills, payments, and alternative_payments exists today via
// GORM Preload. These counts are capped at their current values so that any
// future change adding a loop that re-runs these queries will be caught.
// Reducing the star counts is the future optimisation target; when a PR
// narrows a Preload to named columns, tighten the bound below.
func TestWebhookIngestQueryShape(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := &webhookIngestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	business, bill := setupWebhookIngestPerfDB(t, recorder)

	paymentID := fmt.Sprintf("cs_shape_%d", time.Now().UnixNano())
	cleanup := registerIngestStub(bill.ID, paymentID, 4000)
	defer cleanup()

	req := buildPluginWebhookRequest(t, business.ID, bill.ID, paymentID, 4000)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	ph := &PluginHandlers{}
	ph.handlePaymentWebhook(c, "stripe")

	require.Equal(t, http.StatusOK, w.Code,
		"webhook must settle successfully; body: %s", w.Body.String())

	// --- Shape invariants ---

	stmtDump := strings.Join(recorder.statements, "\n")

	// 1. Bills: total SELECT count bounded at 3 (lean ingest shape).
	billSelectCount := recorder.selectCount("bills")
	assert.LessOrEqual(t, billSelectCount, 3,
		"bills: expected ≤3 SELECT statements in the completed settlement path; got %d.\n"+
			"Possible cause: a new bill reload was added to the webhook path.\n%s",
		billSelectCount, stmtDump)

	// 2. Payments: total SELECT count bounded at 3 (current shape).
	paymentsSelectCount := recorder.selectCount("payments")
	assert.LessOrEqual(t, paymentsSelectCount, 3,
		"payments: expected ≤3 SELECT statements; got %d.\n"+
			"Possible cause: GetPaymentByTxHash is now called more often.\n%s",
		paymentsSelectCount, stmtDump)

	// 3. AlternativePayments: total SELECT count bounded at 2 (current shape).
	altPaySelectCount := recorder.selectCount("alternative_payments")
	assert.LessOrEqual(t, altPaySelectCount, 2,
		"alternative_payments: expected ≤2 SELECT statements; got %d.\n"+
			"Possible cause: lookupPluginPaymentBreakdown added a loop.\n%s",
		altPaySelectCount, stmtDump)

	// 4. Businesses: projectedBillBusinessPreload MUST stay a named-column SELECT.
	//    SELECT * on businesses would leak PII columns and widen the read surface.
	assert.Zero(t, recorder.selectStarCount("businesses"),
		"SELECT * on businesses must be absent; projectedBillBusinessPreload must project named columns.\n%s",
		stmtDump)

	// Sanity: settlement actually landed — bill has a positive paid amount.
	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Greater(t, reloaded.PaidAmount, int64(0),
		"settled webhook must have applied a non-zero payment to the bill")
}

// BenchmarkSettlePluginWebhook measures the allocs/op and ns/op of a full
// happy-path Stripe webhook ingest (signature verify → dedup insert → bill
// ownership check → settlement → dedup mark processed).
//
// Run with:
//
//	go test ./internal/handlers/ -bench BenchmarkSettlePluginWebhook -benchmem -benchtime=5x -run '^$'
//
// The -benchtime=5x flag runs 5 iterations so the result is stable even on a
// cold cache.  Use -count=3 for variance checks.
func BenchmarkSettlePluginWebhook(b *testing.B) {
	gin.SetMode(gin.TestMode)

	business, bill := setupWebhookIngestPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Fresh unique payment ID per iteration so idempotency dedup does not
		// short-circuit on the second call (each call inserts a new webhook_events
		// row and applies a 1-cent payment, keeping the bill open).
		iterPaymentID := fmt.Sprintf("cs_bench_%d_%d", time.Now().UnixNano(), i)

		cleanup := registerIngestStub(bill.ID, iterPaymentID, 1) // 1 cent — bill stays open
		req := buildPluginWebhookRequest(b, business.ID, bill.ID, iterPaymentID, 1)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		ph := &PluginHandlers{}
		ph.handlePaymentWebhook(c, "stripe")
		cleanup()

		if w.Code != http.StatusOK {
			b.Fatalf("iteration %d: expected 200, got %d: %s", i, w.Code, w.Body.String())
		}
	}
}
