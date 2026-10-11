package metrics

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLaunchReadinessStateMetricsReflectDatabaseState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE businesses (
		id INTEGER PRIMARY KEY,
		owner_address TEXT,
		user_id INTEGER
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE user_auths (
		id INTEGER PRIMARY KEY,
		provider TEXT NOT NULL,
		email_verified BOOLEAN NOT NULL
	)`).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO businesses (id, owner_address, user_id) VALUES (?, ?, NULL)`,
		2, " \t\n",
	).Error)

	for _, statement := range []string{
		`INSERT INTO businesses (id, owner_address, user_id) VALUES (1, '', NULL)`,
		`INSERT INTO businesses (id, owner_address, user_id) VALUES (3, '0xowner', NULL)`,
		`INSERT INTO businesses (id, owner_address, user_id) VALUES (4, '', 42)`,
		`INSERT INTO user_auths (id, provider, email_verified) VALUES (1, 'email', FALSE)`,
		`INSERT INTO user_auths (id, provider, email_verified) VALUES (2, 'email', FALSE)`,
		`INSERT INTO user_auths (id, provider, email_verified) VALUES (3, 'email', TRUE)`,
		`INSERT INTO user_auths (id, provider, email_verified) VALUES (4, 'google', FALSE)`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}

	require.NoError(t, RefreshLaunchReadinessState(db))
	require.Equal(t, float64(2), testutil.ToFloat64(OwnerlessBusinesses))
	require.Equal(t, float64(2), testutil.ToFloat64(PendingEmailRegistrations))

	OwnerlessBusinesses.Set(0)
	PendingEmailRegistrations.Set(0)
}

func TestAuthorizationAndVerificationMetricsAreBoundedAndCollectable(t *testing.T) {
	TenantAuthorizationMismatches.WithLabelValues("business_access").Inc()
	EmailVerificationLatency.Observe((23 * time.Minute).Seconds())

	mismatch := collect(t, TenantAuthorizationMismatches)
	require.Equal(t, "payverge_tenant_authorization_mismatches_total", mismatch.GetName())
	require.Len(t, mismatch.Metric, 1)
	require.Len(t, mismatch.Metric[0].Label, 1)
	require.Equal(t, "boundary", mismatch.Metric[0].Label[0].GetName())

	latency := collect(t, EmailVerificationLatency)
	require.Equal(t, "payverge_email_verification_latency_seconds", latency.GetName())
	require.Len(t, latency.Metric, 1)
}

func TestAlternativePaymentRequestMetricsUseClosedOutcomes(t *testing.T) {
	for _, outcome := range AlternativePaymentRequestOutcomeValues() {
		before := CurrentAlternativePaymentRequestOutcome(outcome)
		RecordAlternativePaymentRequestOutcome(outcome)
		require.Equal(t, float64(1), CurrentAlternativePaymentRequestOutcome(outcome)-before)
	}

	mf := collect(t, alternativePaymentRequestOutcomes)
	require.Equal(t, "payverge_alternative_payment_request_outcomes_total", mf.GetName())
	require.Len(t, mf.Metric, len(AlternativePaymentRequestOutcomeValues()))

	require.Panics(t, func() {
		RecordAlternativePaymentRequestOutcome(AlternativePaymentRequestOutcome("tenant-42"))
	})
}

func TestRefreshAlternativePaymentRequestStatePublishesOldestPendingAge(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE alternative_payments (
		id INTEGER PRIMARY KEY,
		status TEXT NOT NULL,
		created_at DATETIME NOT NULL
	)`).Error)

	now := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec(
		`INSERT INTO alternative_payments (id, status, created_at) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)`,
		1, "pending", now.Add(-11*time.Minute),
		2, "pending", now.Add(-2*time.Minute),
		3, "confirmed", now.Add(-30*time.Minute),
	).Error)

	require.NoError(t, RefreshAlternativePaymentRequestState(db, now))
	require.Equal(t, float64(11*time.Minute/time.Second), testutil.ToFloat64(AlternativePaymentRequestOldestPendingSeconds))

	require.NoError(t, db.Exec(`UPDATE alternative_payments SET status = 'confirmed'`).Error)
	require.NoError(t, RefreshAlternativePaymentRequestState(db, now))
	require.Zero(t, testutil.ToFloat64(AlternativePaymentRequestOldestPendingSeconds))
}

func TestRefreshCollectorIsNotExported(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "refresh.go", nil, 0)
	if err != nil {
		t.Fatalf("parse refresh metrics source: %v", err)
	}
	for _, declaration := range file.Decls {
		vars, ok := declaration.(*ast.GenDecl)
		if !ok || vars.Tok != token.VAR {
			continue
		}
		for _, spec := range vars.Specs {
			values, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range values.Names {
				if ast.IsExported(name.Name) {
					t.Fatalf("refresh metric variable %s must stay private; callers must use the typed API", name.Name)
				}
			}
		}
	}
}

func TestRefreshOutcomeCounterUsesClosedLabels(t *testing.T) {
	Init()
	for _, realm := range RefreshRealms() {
		for _, outcome := range RefreshOutcomeValues() {
			before := CurrentRefreshOutcome(realm, outcome)
			RecordRefreshOutcome(realm, outcome)
			if got := CurrentRefreshOutcome(realm, outcome) - before; got != 1 {
				t.Fatalf("want one increment for %s/%s, got %v", realm, outcome, got)
			}
		}
	}

	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather refresh outcomes: %v", err)
	}
	var mf *dto.MetricFamily
	for _, candidate := range mfs {
		if candidate.GetName() == "payverge_auth_refresh_outcomes_total" {
			mf = candidate
			break
		}
	}
	if mf == nil {
		t.Fatal("registered refresh outcome metric not found")
	}
	if got, want := len(mf.Metric), len(RefreshRealms())*len(RefreshOutcomeValues()); got != want {
		t.Fatalf("want %d bounded label combinations, got %d", want, got)
	}
	for _, sample := range mf.Metric {
		labelNames := map[string]bool{}
		for _, pair := range sample.Label {
			labelNames[pair.GetName()] = true
		}
		if len(labelNames) != 2 || !labelNames["realm"] || !labelNames["outcome"] {
			t.Fatalf("want only realm+outcome labels, got %v", labelNames)
		}
	}

	assertPanics := func(name string, fn func()) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("want invalid label value to panic before creating a series")
				}
			}()
			fn()
		})
	}
	assertPanics("realm", func() {
		RecordRefreshOutcome(RefreshRealm("tenant-123"), RefreshOutcomeSuccess)
	})
	assertPanics("outcome", func() {
		RecordRefreshOutcome(RefreshRealmOperator, RefreshOutcome("database: timeout for user 42"))
	})
	assertPanics("read realm", func() {
		CurrentRefreshOutcome(RefreshRealm("tenant-123"), RefreshOutcomeSuccess)
	})
	assertPanics("read outcome", func() {
		CurrentRefreshOutcome(RefreshRealmOperator, RefreshOutcome("database: timeout for user 42"))
	})
}

func TestRefreshOutcomeCounterRegisteredByInit(t *testing.T) {
	Init()
	RecordRefreshOutcome(RefreshRealmOperator, RefreshOutcomeSuccess)

	count, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "payverge_auth_refresh_outcomes_total")
	if err != nil {
		t.Fatalf("gather registered refresh metric: %v", err)
	}
	if count == 0 {
		t.Fatal("want registered refresh metric samples")
	}
}

func TestFlowRequestDurationLabelsAndBuckets(t *testing.T) {
	if FlowRequestDuration == nil {
		t.Fatal("FlowRequestDuration is nil")
	}
	FlowRequestDuration.WithLabelValues("menu_browse", "200").Observe(0.01)
	FlowRequestDuration.WithLabelValues("checkout", "500").Observe(1.2)
	FlowRequestDuration.WithLabelValues("sse_kitchen", "200").Observe(0.001)

	mf := collect(t, FlowRequestDuration)
	if got := len(mf.Metric); got != 3 {
		t.Fatalf("want 3 label combinations, got %d", got)
	}
	m, ok := FlowRequestDuration.WithLabelValues("menu_browse", "200").(prometheus.Metric)
	if !ok {
		t.Fatalf("expected histogram observer to implement prometheus.Metric")
	}
	desc := m.Desc().String()
	if !strings.Contains(desc, "flow") || !strings.Contains(desc, "status") {
		t.Fatalf("expected labels flow+status in desc, got %s", desc)
	}
}

func TestFiscalOrphanedCAETotalRegistered(t *testing.T) {
	if FiscalOrphanedCAETotal == nil {
		t.Fatal("FiscalOrphanedCAETotal is nil")
	}
	FiscalOrphanedCAETotal.Inc()
	mf := collect(t, FiscalOrphanedCAETotal)
	if mf.GetName() != "payverge_fiscal_orphaned_cae_total" {
		t.Fatalf("want metric name payverge_fiscal_orphaned_cae_total, got %s", mf.GetName())
	}
	if got := len(mf.Metric); got != 1 {
		t.Fatalf("want 1 metric sample, got %d", got)
	}
}

func TestEmailMetricsRegisteredByInit(t *testing.T) {
	Init()
	EmailSendAttempts.WithLabelValues("resend", "transactional", "success", "none").Add(0)
	EmailSendDuration.WithLabelValues("resend", "success").Observe(0)
	EmailSendExhausted.WithLabelValues("resend", "timeout").Add(0)
	EmailDeliveryEvents.WithLabelValues("email.delivered").Add(0)
	for _, name := range []string{
		"payverge_email_send_attempts_total",
		"payverge_email_send_duration_seconds",
		"payverge_email_send_exhausted_total",
		"payverge_email_delivery_events_total",
	} {
		count, err := testutil.GatherAndCount(prometheus.DefaultGatherer, name)
		if err != nil {
			t.Fatalf("gather %s: %v", name, err)
		}
		if count != 1 {
			t.Fatalf("want %s registered once, got %d", name, count)
		}
	}
}

// TestAIImageMonthlyAlertsRegisteredByInit pins the half of the fair-use alert
// that scraping depends on. The counter increments in memory whether or not it
// is registered, so the server-side alert test stays green even if the
// MustRegister line is dropped — only the default gatherer can tell the
// difference, and an unregistered counter never reaches /metrics.
func TestAIImageMonthlyAlertsRegisteredByInit(t *testing.T) {
	Init()
	AIImageMonthlyAlerts.Add(0)

	const name = "payverge_ai_image_monthly_alerts_total"
	count, err := testutil.GatherAndCount(prometheus.DefaultGatherer, name)
	if err != nil {
		t.Fatalf("gather %s: %v", name, err)
	}
	if count != 1 {
		t.Fatalf("want %s registered once, got %d", name, count)
	}
}

func TestNoDeadCarFleetMetricsRegistered(t *testing.T) {
	Init() // idempotent registerOnce
	// Force one observation so CounterVec surfaces in Gather output.
	PaymentWebhookProcessingFailures.WithLabelValues("_test", "_test").Add(0)
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	dead := map[string]bool{
		"car_metrics_total": true, "fleet_investment_amount": true,
		"fleet_rental_duration_days": true, "financial_metrics_total": true,
		"user_metrics_total": true, "fleet_operations_total": true,
		"car_operations_total": true, "kyc_operations_total": true,
		"nft_operations_total": true, "nft_response_time_seconds": true,
		"reward_operations_total": true, "reward_amounts_distribution": true,
		"reward_response_time_seconds": true, "fleet_metrics_distribution": true,
		"car_operation_duration_days_distribution": true,
		"user_referrals_total":                     true, "active_users_total": true,
	}
	have := map[string]bool{}
	for _, mf := range mfs {
		if dead[mf.GetName()] {
			t.Errorf("dead car/fleet metric still registered: %s", mf.GetName())
		}
		have[mf.GetName()] = true
	}
	if !have["payverge_payment_webhook_processing_failures_total"] {
		t.Error("missing real signal: payverge_payment_webhook_processing_failures_total")
	}
}

func TestPaymentReconciliationMetricsRegistered(t *testing.T) {
	Init()
	PaymentReconciliationOutcomes.WithLabelValues("_test", "pending").Add(0)
	PaymentReconciliationOldestPendingSeconds.WithLabelValues("_test").Set(0)
	for _, name := range []string{
		"payverge_payment_reconciliation_outcomes_total",
		"payverge_payment_reconciliation_oldest_pending_seconds",
	} {
		count, err := testutil.GatherAndCount(prometheus.DefaultGatherer, name)
		if err != nil {
			t.Fatalf("gather %s: %v", name, err)
		}
		if count != 1 {
			t.Fatalf("want %s registered once, got %d", name, count)
		}
	}
}

func collect(t *testing.T, c prometheus.Collector) *dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(mfs) != 1 {
		t.Fatalf("want 1 metric family, got %d", len(mfs))
	}
	return mfs[0]
}

func TestRecordPaymentWebhookFailure_FoldsUnknownReasonToOther(t *testing.T) {
	before := testutil.ToFloat64(PaymentWebhookProcessingFailures.WithLabelValues("_reason_test", WebhookFailureOther))
	RecordPaymentWebhookFailure("_reason_test", "http_418")
	require.Equal(t, before+1, testutil.ToFloat64(PaymentWebhookProcessingFailures.WithLabelValues("_reason_test", WebhookFailureOther)))
	require.Equal(t, 0.0, testutil.ToFloat64(PaymentWebhookProcessingFailures.WithLabelValues("_reason_test", "http_418")))
}

func TestPrintBrowserActiveClients_ReturnsToZeroAfterWindow(t *testing.T) {
	printBrowserActivity.mu.Lock()
	previousNow := printBrowserNow
	printBrowserActivity.last = map[uint]time.Time{}
	printBrowserActivity.mu.Unlock()

	current := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	printBrowserNow = func() time.Time { return current }
	t.Cleanup(func() {
		printBrowserActivity.mu.Lock()
		printBrowserNow = previousNow
		printBrowserActivity.last = map[uint]time.Time{}
		printBrowserActivity.mu.Unlock()
	})

	MarkPrintBrowserClientActive(1)
	MarkPrintBrowserClientActive(2)
	require.Equal(t, float64(2), testutil.ToFloat64(PrintBrowserActiveClients))

	MarkPrintBrowserClientActive(1)
	require.Equal(t, float64(2), testutil.ToFloat64(PrintBrowserActiveClients))

	current = current.Add(printBrowserActiveWindow + time.Nanosecond)
	require.Equal(t, float64(0), testutil.ToFloat64(PrintBrowserActiveClients))

	printBrowserActivity.mu.Lock()
	defer printBrowserActivity.mu.Unlock()
	require.Empty(t, printBrowserActivity.last)
}
