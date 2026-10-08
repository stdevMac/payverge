package metrics

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

var registerOnce sync.Once

var (
	assistantSurfaces  = map[string]struct{}{"ops": {}, "waiter": {}}
	assistantContracts = map[string]struct{}{"v1": {}, "v2": {}}
	assistantOutcomes  = map[string]struct{}{
		"ok": {}, "fallback": {}, "invalid": {}, "blocked": {}, "timeout": {}, "error": {},
	}
	assistantValidations = map[string]struct{}{
		"schema": {}, "action": {}, "source": {}, "entity": {}, "language": {},
	}
	assistantValidationOutcomes = map[string]struct{}{"verified": {}, "dropped": {}}
)

func validateAssistantResponse(surface, contractVersion, outcome string, seconds, firstContentSeconds float64) error {
	if _, ok := assistantSurfaces[surface]; !ok {
		return fmt.Errorf("unsupported assistant surface")
	}
	if _, ok := assistantContracts[contractVersion]; !ok {
		return fmt.Errorf("unsupported assistant contract version")
	}
	if _, ok := assistantOutcomes[outcome]; !ok {
		return fmt.Errorf("unsupported assistant outcome")
	}
	if !boundedAssistantDuration(seconds) || !boundedAssistantDuration(firstContentSeconds) {
		return fmt.Errorf("assistant duration outside bounded range")
	}
	return nil
}

func boundedAssistantDuration(seconds float64) bool {
	return !math.IsNaN(seconds) && !math.IsInf(seconds, 0) && seconds >= 0 && seconds <= 30*60
}

func validateAssistantValidation(surface, validation, outcome string) error {
	if _, ok := assistantSurfaces[surface]; !ok {
		return fmt.Errorf("unsupported assistant surface")
	}
	if _, ok := assistantValidations[validation]; !ok {
		return fmt.Errorf("unsupported assistant validation")
	}
	if _, ok := assistantValidationOutcomes[outcome]; !ok {
		return fmt.Errorf("unsupported assistant validation outcome")
	}
	return nil
}

// RecordAssistantTerminal records one fully validated terminal event. Every
// dimension is checked before the first metric mutation so malformed runtime
// values cannot create a partial or high-cardinality measurement.
func RecordAssistantTerminal(event llm.AITelemetryEvent) error {
	if err := llm.ValidateTelemetryEvent(event); err != nil {
		return err
	}
	if event.ContractVersion == "" || event.Outcome == "" || event.ActionOutcome == "" ||
		event.SourceOutcome == "" || event.EntityOutcome == "" || event.SchemaOutcome == "" ||
		event.LanguageOutcome == "" || event.Language == "" {
		return fmt.Errorf("assistant terminal event incomplete")
	}

	validations := make([][2]string, 0, 5)
	appendValidation := func(validation, outcome string) {
		if outcome != "none" {
			validations = append(validations, [2]string{validation, outcome})
		}
	}
	appendValidation("schema", event.SchemaOutcome)
	appendValidation("language", event.LanguageOutcome)
	switch event.ActionOutcome {
	case "offered", "accepted":
		appendValidation("action", "verified")
	case "rejected", "failed":
		appendValidation("action", "dropped")
	}
	appendValidation("source", event.SourceOutcome)
	appendValidation("entity", event.EntityOutcome)
	for _, validation := range validations {
		if err := validateAssistantValidation(event.Surface, validation[0], validation[1]); err != nil {
			return err
		}
	}
	if err := validateAssistantResponse(
		event.Surface, event.ContractVersion, event.Outcome,
		float64(event.LatencyMs)/1000, float64(event.TimeToFirstMs)/1000,
	); err != nil {
		return err
	}

	AssistantResponsesTotal.WithLabelValues(event.Surface, event.ContractVersion, event.Outcome).Inc()
	AssistantResponseSeconds.WithLabelValues(event.Surface, event.ContractVersion).Observe(float64(event.LatencyMs) / 1000)
	// Zero means the non-streaming request boundary could not measure first
	// content independently from terminal latency. Do not fabricate an instant
	// TTFC observation; streaming boundaries may populate a positive value.
	if event.TimeToFirstMs > 0 {
		AssistantTimeToFirstContent.WithLabelValues(event.Surface, event.ContractVersion).Observe(float64(event.TimeToFirstMs) / 1000)
	}
	for _, validation := range validations {
		AssistantValidationTotal.WithLabelValues(event.Surface, validation[0], validation[1]).Inc()
	}
	return nil
}

// Module for metrics related functions
var (
	AssistantResponsesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_assistant_responses_total",
			Help: "Terminal assistant responses by bounded surface, contract version, and outcome.",
		},
		[]string{"surface", "contract_version", "outcome"},
	)
	AssistantResponseSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "payverge_assistant_response_seconds",
			Help:    "Assistant request duration by bounded surface and contract version.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"surface", "contract_version"},
	)
	AssistantTimeToFirstContent = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "payverge_assistant_time_to_first_content_seconds",
			Help:    "Time to first assistant content by bounded surface and contract version.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"surface", "contract_version"},
	)
	AssistantValidationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_assistant_validation_total",
			Help: "Trusted assistant validation outcomes by bounded validation stage.",
		},
		[]string{"surface", "validation", "outcome"},
	)

	// HTTP metrics
	TotalRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"path", "method", "status"},
	)

	// FlowRequestDuration tracks request duration in seconds for the high-priority
	// request-shaped flows used in load tests: menu_browse, checkout. SSE is
	// measured via k6's HTTP-level metrics + the broadcast benchmark, not here —
	// the middleware-driven observation would record full connection lifetime.
	// Labeled by flow + HTTP status string ("200", "500"). Buckets cover both
	// fast cache-able reads and slower DB-write paths.
	FlowRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "flow_request_duration_seconds",
			Help: "Request duration in seconds for high-priority flows.",
			Buckets: []float64{
				0.005, 0.01, 0.025, 0.05, 0.1,
				0.2, 0.5, 1, 2, 5,
			},
		},
		[]string{"flow", "status"},
	)

	// Authentication metrics
	AuthOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_operations_total",
			Help: "Total number of authentication operations by type",
		},
		[]string{"operation"},
	)

	// Telegram Bot Metrics
	TelegramCommands = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "telegram_commands_total",
			Help: "Total number of Telegram bot commands by type",
		},
		[]string{"command", "status"},
	)

	TelegramActiveUsers = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "telegram_active_users",
			Help: "Number of users with connected Telegram accounts",
		},
	)

	TelegramMessageLength = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "telegram_message_length_chars",
			Help:    "Distribution of Telegram message lengths in characters",
			Buckets: prometheus.LinearBuckets(50, 50, 20), // 50 to 1000 chars in steps of 50
		},
	)

	TelegramResponseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "telegram_response_time_seconds",
			Help:    "Time taken to process and respond to Telegram commands",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // From 10ms to ~10s
		},
		[]string{"command"},
	)

	TelegramConnectionTokensIssued = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "payverge_telegram_connection_tokens_issued_total",
			Help: "Total number of Telegram connection tokens issued",
		},
	)

	TelegramConnectionTokensConsumed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "payverge_telegram_connection_tokens_consumed_total",
			Help: "Total number of Telegram connection tokens consumed",
		},
	)

	TelegramConnectionFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_telegram_connection_failures_total",
			Help: "Total number of Telegram connection failures by reason",
		},
		[]string{"reason"},
	)

	TelegramWebhookUpdates = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_telegram_webhook_updates_total",
			Help: "Total number of Telegram webhook updates by status",
		},
		[]string{"status"},
	)

	SSEDroppedEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_sse_dropped_events_total",
			Help: "Total number of SSE business events dropped because a subscriber buffer was full",
		},
		[]string{"event_type"},
	)

	SSERejectedSubscriptions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_sse_rejected_subscriptions_total",
			Help: "Total number of SSE subscriptions rejected by a concurrent-connection ceiling (reason: business|ip|guest_business|guest_ip)",
		},
		[]string{"reason"},
	)

	OrderToKitchenSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "payverge_order_to_kitchen_seconds",
			Help:    "Seconds from order creation to the first successful kitchen-ticket print for that order.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 14), // 1s → ~2.3h
		},
	)

	PluginNotificationEnqueued = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_plugin_notification_enqueued_total",
			Help: "Total number of plugin notifications enqueued by plugin and event type",
		},
		[]string{"plugin", "event_type"},
	)

	PluginNotificationDropped = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_plugin_notification_dropped_total",
			Help: "Total number of plugin notifications dropped at enqueue by plugin, event type, and reason",
		},
		[]string{"plugin", "event_type", "reason"},
	)

	TelegramDeliveryAvailable = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "payverge_telegram_delivery_available",
			Help: "1 when the Telegram notification worker is running (bot token configured), else 0",
		},
	)

	PluginNotificationDelivered = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_plugin_notification_delivered_total",
			Help: "Total number of plugin notifications delivered by plugin and event type",
		},
		[]string{"plugin", "event_type"},
	)

	PluginNotificationFailed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_plugin_notification_failed_total",
			Help: "Total number of plugin notifications failed by plugin, event type, and reason",
		},
		[]string{"plugin", "event_type", "reason"},
	)

	PaymentWebhookUnsupportedActions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_payment_webhook_unsupported_actions_total",
			Help: "Total number of unsupported payment webhook actions by plugin and action",
		},
		[]string{"plugin", "action"},
	)

	PaymentReconciliationOutcomes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_payment_reconciliation_outcomes_total",
			Help: "Total authoritative provider reconciliation outcomes by provider and outcome",
		},
		[]string{"provider", "outcome"},
	)

	PaymentReconciliationOldestPendingSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "payverge_payment_reconciliation_oldest_pending_seconds",
			Help: "Age in seconds of the oldest payment currently inspected by provider reconciliation",
		},
		[]string{"provider"},
	)

	PluginNotificationRetried = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_plugin_notification_retried_total",
			Help: "Total number of plugin notifications retried by plugin, event type, and reason",
		},
		[]string{"plugin", "event_type", "reason"},
	)

	TelegramSendAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_telegram_send_attempts_total",
			Help: "Total number of Telegram send attempts by status and reason",
		},
		[]string{"status", "reason"},
	)

	PluginNotificationQueueDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "payverge_plugin_notification_queue_depth",
			Help: "Current due plugin notification queue depth by plugin",
		},
		[]string{"plugin"},
	)

	TelegramConnectedBusinesses = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "payverge_telegram_connected_businesses",
			Help: "Current number of businesses with an active Telegram chat connection",
		},
	)

	TelegramSendDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "payverge_telegram_send_duration_seconds",
			Help:    "Duration of Telegram notification send attempts",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 12),
		},
		[]string{"status", "reason"},
	)

	PluginNotificationDeliveryLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "payverge_plugin_notification_delivery_latency_seconds",
			Help:    "Latency from plugin notification enqueue to final delivery",
			Buckets: prometheus.ExponentialBuckets(1, 2, 16),
		},
		[]string{"plugin", "event_type"},
	)

	// Fiscal worker metrics (AFIP/ARCA job queue)
	FiscalReceiptsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_fiscal_receipts_total",
			Help: "Total fiscal job outcomes by terminal/transition status (authorized, failed_retryable, failed_permanent, rejected)",
		},
		[]string{"status"},
	)

	FiscalJobAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_fiscal_job_attempts_total",
			Help: "Total fiscal job processing attempts by action (issue_receipt, credit_note, status_check)",
		},
		[]string{"action"},
	)

	FiscalJobsQueueDepth = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "payverge_fiscal_jobs_queue_depth",
			Help: "Current number of due, unlocked fiscal jobs awaiting processing",
		},
	)

	// FiscalOrphanedCAETotal counts the worst fiscal failure: AFIP committed a
	// CAE (a legal invoice number was consumed) but the local FiscalReceipt row
	// could not be persisted after the bounded inline retry, so the job goes
	// terminal (failed_permanent) WITHOUT re-issuing. Each increment is an
	// operator-reconcile-by-hand event and should page.
	FiscalOrphanedCAETotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "payverge_fiscal_orphaned_cae_total",
			Help: "Count of authorized AFIP CAEs whose local receipt row failed to persist (orphaned legal invoice number; requires manual reconciliation).",
		},
	)

	// Wave 4 durable fiscal delivery metrics (per-channel tasks).
	FiscalDeliveryPending = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "payverge_fiscal_delivery_pending",
			Help: "Pending + leased fiscal delivery tasks awaiting execution",
		},
	)
	FiscalDeliveryOldestAgeSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "payverge_fiscal_delivery_oldest_age_seconds",
			Help: "Age in seconds of the oldest pending fiscal delivery task",
		},
	)
	FiscalDeliveryAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_fiscal_delivery_attempts_total",
			Help: "Fiscal delivery task attempts by channel",
		},
		[]string{"channel"},
	)
	FiscalDeliverySuccesses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_fiscal_delivery_successes_total",
			Help: "Fiscal delivery task successes by channel",
		},
		[]string{"channel"},
	)
	FiscalDeliveryDeadLetters = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_fiscal_delivery_dead_letters_total",
			Help: "Fiscal delivery tasks dead-lettered by channel",
		},
		[]string{"channel"},
	)
	FiscalDeliveryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "payverge_fiscal_delivery_duration_seconds",
			Help:    "Fiscal delivery task execution duration by channel",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"channel"},
	)

	// PaymentWebhookProcessingFailures counts payment-provider webhook events
	// that failed to process (parse/verify/settle), by plugin and a closed
	// reason. Anything outside that set is recorded as "other" so the label
	// cardinality stays bounded.
	PaymentWebhookProcessingFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_payment_webhook_processing_failures_total",
			Help: "Total payment webhook processing failures by plugin and reason.",
		},
		[]string{"plugin", "reason"},
	)

	// User Operation Metrics
	UserOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "user_operations_total",
			Help: "Total number of user operations by type and status",
		},
		[]string{"operation", "status"},
	)

	UserResponseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "user_response_time_seconds",
			Help:    "Time taken to process user operations",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // From 10ms to ~10s
		},
		[]string{"operation"},
	)

	// Wave 4 browser print agent metrics.
	PrintBrowserClaims = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "payverge_print_browser_claims_total",
			Help: "Total browser print jobs claimed by an operator tab agent.",
		},
	)
	PrintBrowserExpiredLeases = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "payverge_print_browser_expired_leases_total",
			Help: "Total browser print leases reclaimed after expiry (crashed tab recovery).",
		},
	)
	PrintBrowserPresented = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "payverge_print_browser_presented_total",
			Help: "Total times a browser print dialog was presented (window.print).",
		},
	)
	PrintBrowserConfirmed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "payverge_print_browser_confirmed_total",
			Help: "Total browser print jobs explicitly confirmed printed by an operator.",
		},
	)
	PrintBrowserRetry = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "payverge_print_browser_retry_total",
			Help: "Total browser print jobs returned to the queue for retry by an operator.",
		},
	)
	PrintBrowserAbandoned = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "payverge_print_browser_abandoned_total",
			Help: "Total browser print jobs failed after waiting past the abandon SLA with no station claim.",
		},
	)
	PrintBrowserActiveClients = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "payverge_print_browser_active_clients",
			Help: "Businesses with a browser print agent that claimed a job within the activity window.",
		},
		func() float64 { return float64(activePrintBrowserClients()) },
	)

	EmailSendAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_email_send_attempts_total",
			Help: "Email provider send attempts by bounded outcome and failure reason.",
		},
		[]string{"provider", "message_type", "outcome", "reason"},
	)

	EmailSendDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "payverge_email_send_duration_seconds",
			Help:    "Email provider send latency by provider and outcome.",
			Buckets: prometheus.ExponentialBuckets(0.05, 2, 10),
		},
		[]string{"provider", "outcome"},
	)

	EmailSendExhausted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_email_send_exhausted_total",
			Help: "Transactional email sends that exhausted retries without delivery.",
		},
		[]string{"provider", "reason"},
	)
	// AIImageMonthlyAlerts counts businesses crossing the monthly image-usage
	// alert threshold. Deliberately unlabeled: business_id would be unbounded
	// cardinality. The Sentry message carries the identity.
	AIImageMonthlyAlerts = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "payverge_ai_image_monthly_alerts_total",
			Help: "Businesses crossing the monthly AI image usage alert threshold.",
		},
	)

	EmailDeliveryEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_email_delivery_events_total",
			Help: "Authenticated provider delivery events by bounded event type.",
		},
		[]string{"event"},
	)

	// Bounce attribution (#562): delivery events matched back to the entity
	// that triggered the send. A rising "unattributed" share means send sites
	// are queueing without an origin.
	EmailDeliveryAttributed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_email_delivery_attributed_total",
			Help: "Provider delivery events attributed to an originating entity, by event type and entity kind.",
		},
		[]string{"event", "entity"},
	)

	// Transactional email outbox (#551). Mirrors the plugin notification
	// outbox metrics so both outbound channels alert the same way.
	EmailOutboxEnqueued = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_email_outbox_enqueued_total",
			Help: "Transactional emails enqueued on the durable outbox by template.",
		},
		[]string{"template"},
	)

	EmailOutboxDelivered = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_email_outbox_delivered_total",
			Help: "Outbox emails accepted by the provider, by provider and template.",
		},
		[]string{"provider", "template"},
	)

	EmailOutboxRetried = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_email_outbox_retried_total",
			Help: "Outbox emails rescheduled after a retryable failure, by template and reason.",
		},
		[]string{"template", "reason"},
	)

	EmailOutboxFailed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_email_outbox_failed_total",
			Help: "Outbox emails that reached the terminal failed (dead-letter) state, by template and reason.",
		},
		[]string{"template", "reason"},
	)

	EmailOutboxDropped = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "payverge_email_outbox_dropped_total",
			Help: "Outbox emails dropped without delivery (suppressed recipient, TTL expiry), by template and reason.",
		},
		[]string{"template", "reason"},
	)

	EmailOutboxQueueDepth = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "payverge_email_outbox_queue_depth",
			Help: "Transactional emails currently due for delivery on the outbox.",
		},
	)

	EmailOutboxDeliveryLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "payverge_email_outbox_delivery_latency_seconds",
			Help:    "Latency from outbox enqueue to provider acceptance.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 16),
		},
		[]string{"template"},
	)
)

// printBrowserActiveWindow is how long a business stays in
// payverge_print_browser_active_clients after its last browser claim.
const printBrowserActiveWindow = 2 * time.Minute

// printBrowserNow is the clock for the browser-agent activity window.
// Tests replace it and restore the previous value.
var printBrowserNow = time.Now

// printBrowserActivity records the last browser claim time per business.
// Entries older than printBrowserActiveWindow are pruned at scrape time.
var printBrowserActivity = struct {
	mu   sync.Mutex
	last map[uint]time.Time
}{last: map[uint]time.Time{}}

// MarkPrintBrowserClientActive records that a browser print agent for
// businessID claimed a job at the current clock.
func MarkPrintBrowserClientActive(businessID uint) {
	now := printBrowserNow()
	printBrowserActivity.mu.Lock()
	defer printBrowserActivity.mu.Unlock()
	if printBrowserActivity.last == nil {
		printBrowserActivity.last = map[uint]time.Time{}
	}
	printBrowserActivity.last[businessID] = now
	prunePrintBrowserActivityLocked(now)
}

// activePrintBrowserClients is the scrape-time value of PrintBrowserActiveClients.
func activePrintBrowserClients() int {
	now := printBrowserNow()
	printBrowserActivity.mu.Lock()
	defer printBrowserActivity.mu.Unlock()
	prunePrintBrowserActivityLocked(now)
	return len(printBrowserActivity.last)
}

func prunePrintBrowserActivityLocked(now time.Time) {
	for id, seen := range printBrowserActivity.last {
		if now.Sub(seen) > printBrowserActiveWindow {
			delete(printBrowserActivity.last, id)
		}
	}
}

// Closed reasons for PaymentWebhookProcessingFailures. Callers pass these
// constants; RecordPaymentWebhookFailure folds every other string to
// WebhookFailureOther.
const (
	WebhookFailureSignatureInvalid   = "signature_invalid"
	WebhookFailureSecretMissing      = "secret_missing"
	WebhookFailureBusinessUnresolved = "business_unresolved"
	WebhookFailureSettlementFailed   = "settlement_failed"
	WebhookFailureRejected           = "rejected"
	WebhookFailureInternal           = "internal_error"
	// WebhookFailureCapturePending is a deliberate, bounded retry: a refund,
	// dispute or reversal arrived before its capture was recorded. It is not
	// an application failure; alert rules exclude it.
	WebhookFailureCapturePending = "capture_pending"
	WebhookFailureOther          = "other"
)

var paymentWebhookFailurePlugins = []string{"stripe", "paypal", "mercadopago"}

var paymentWebhookFailureReasons = []string{
	WebhookFailureSignatureInvalid,
	WebhookFailureSecretMissing,
	WebhookFailureBusinessUnresolved,
	WebhookFailureSettlementFailed,
	WebhookFailureRejected,
	WebhookFailureInternal,
	WebhookFailureCapturePending,
	WebhookFailureOther,
}

// RecordPaymentWebhookFailure increments the webhook failure counter. A
// reason outside the closed set is stored as "other".
func RecordPaymentWebhookFailure(plugin, reason string) {
	PaymentWebhookProcessingFailures.WithLabelValues(plugin, normalizeWebhookFailureReason(reason)).Inc()
}

func normalizeWebhookFailureReason(reason string) string {
	for _, known := range paymentWebhookFailureReasons {
		if reason == known {
			return reason
		}
	}
	return WebhookFailureOther
}

// pretouchPaymentWebhookFailureSeries publishes every closed reason for the
// first-party payment plugins at zero so a missing series means "no failure",
// not "this reason was never defined".
func pretouchPaymentWebhookFailureSeries() {
	for _, plugin := range paymentWebhookFailurePlugins {
		for _, reason := range paymentWebhookFailureReasons {
			PaymentWebhookProcessingFailures.WithLabelValues(plugin, reason).Add(0)
		}
	}
}

func Init() {
	registerOnce.Do(register)
}

func register() {
	prometheus.MustRegister(AssistantResponsesTotal)
	prometheus.MustRegister(AssistantResponseSeconds)
	prometheus.MustRegister(AssistantTimeToFirstContent)
	prometheus.MustRegister(AssistantValidationTotal)
	// Register HTTP metrics
	prometheus.MustRegister(TotalRequests)
	prometheus.MustRegister(FlowRequestDuration)

	// Register consolidated metrics

	// Register auth metrics
	prometheus.MustRegister(AuthOperations)
	prometheus.MustRegister(refreshOutcomes)
	prometheus.MustRegister(PendingEmailRegistrations)
	prometheus.MustRegister(EmailVerificationLatency)
	prometheus.MustRegister(TenantAuthorizationMismatches)
	prometheus.MustRegister(OwnerlessBusinesses)
	prometheus.MustRegister(alternativePaymentRequestOutcomes)
	prometheus.MustRegister(AlternativePaymentRequestOldestPendingSeconds)

	// Register Telegram metrics
	prometheus.MustRegister(TelegramCommands)
	prometheus.MustRegister(TelegramActiveUsers)
	prometheus.MustRegister(TelegramMessageLength)
	prometheus.MustRegister(TelegramResponseTime)
	prometheus.MustRegister(TelegramConnectionTokensIssued)
	prometheus.MustRegister(TelegramConnectionTokensConsumed)
	prometheus.MustRegister(TelegramConnectionFailures)
	prometheus.MustRegister(TelegramWebhookUpdates)
	prometheus.MustRegister(SSEDroppedEvents)
	prometheus.MustRegister(SSERejectedSubscriptions)
	prometheus.MustRegister(OrderToKitchenSeconds)
	prometheus.MustRegister(PluginNotificationEnqueued)
	prometheus.MustRegister(PluginNotificationDropped)
	prometheus.MustRegister(TelegramDeliveryAvailable)
	prometheus.MustRegister(PluginNotificationDelivered)
	prometheus.MustRegister(PluginNotificationFailed)
	prometheus.MustRegister(PaymentWebhookUnsupportedActions)
	prometheus.MustRegister(PaymentReconciliationOutcomes)
	prometheus.MustRegister(PaymentReconciliationOldestPendingSeconds)
	prometheus.MustRegister(PluginNotificationRetried)
	prometheus.MustRegister(TelegramSendAttempts)
	prometheus.MustRegister(PluginNotificationQueueDepth)
	prometheus.MustRegister(TelegramConnectedBusinesses)
	prometheus.MustRegister(TelegramSendDuration)
	prometheus.MustRegister(PluginNotificationDeliveryLatency)

	// Register fiscal worker metrics
	prometheus.MustRegister(FiscalReceiptsTotal)
	prometheus.MustRegister(FiscalJobAttempts)
	prometheus.MustRegister(FiscalJobsQueueDepth)
	prometheus.MustRegister(FiscalOrphanedCAETotal)
	prometheus.MustRegister(FiscalDeliveryPending)
	prometheus.MustRegister(FiscalDeliveryOldestAgeSeconds)
	prometheus.MustRegister(FiscalDeliveryAttempts)
	prometheus.MustRegister(FiscalDeliverySuccesses)
	prometheus.MustRegister(FiscalDeliveryDeadLetters)
	prometheus.MustRegister(FiscalDeliveryDuration)
	prometheus.MustRegister(PaymentWebhookProcessingFailures)
	pretouchPaymentWebhookFailureSeries()

	// Register User operation metrics
	prometheus.MustRegister(UserOperations)
	prometheus.MustRegister(UserResponseTime)
	prometheus.MustRegister(FileMutationOutcomes)

	// Wave 4 browser print agent
	prometheus.MustRegister(PrintBrowserClaims)
	prometheus.MustRegister(PrintBrowserExpiredLeases)
	prometheus.MustRegister(PrintBrowserPresented)
	prometheus.MustRegister(PrintBrowserConfirmed)
	prometheus.MustRegister(PrintBrowserRetry)
	prometheus.MustRegister(PrintBrowserAbandoned)
	prometheus.MustRegister(PrintBrowserActiveClients)
	prometheus.MustRegister(EmailSendAttempts)
	prometheus.MustRegister(EmailSendDuration)
	prometheus.MustRegister(EmailSendExhausted)
	// Register AI image fair-use metrics
	prometheus.MustRegister(AIImageMonthlyAlerts)
	prometheus.MustRegister(EmailDeliveryEvents)
	prometheus.MustRegister(EmailDeliveryAttributed)

	// Transactional email outbox
	prometheus.MustRegister(EmailOutboxEnqueued)
	prometheus.MustRegister(EmailOutboxDelivered)
	prometheus.MustRegister(EmailOutboxRetried)
	prometheus.MustRegister(EmailOutboxFailed)
	prometheus.MustRegister(EmailOutboxDropped)
	prometheus.MustRegister(EmailOutboxQueueDepth)
	prometheus.MustRegister(EmailOutboxDeliveryLatency)
}
