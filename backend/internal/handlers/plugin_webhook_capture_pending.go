package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
)

// Refund, dispute and reversal webhooks that arrive BEFORE their capture is in
// the ledger (decision D3, 2026-10-07).
//
// Providers do not guarantee delivery order: a merchant can refund from the PSP
// dashboard, or a guest can open a dispute, while the capture webhook is still
// being retried. Acknowledging such an event drops it for good: the reversal
// keys on the recorded payment, finds nothing, and the bill later settles as
// paid with the refunded money already gone. Instead the handler answers 503
// (Retry-After) and parks the webhook_events row in retry_pending so the
// provider's redelivery is claimed and run once the capture lands.
//
// The retry is bounded by pluginCapturePendingMaxAge. Provider redelivery
// windows end at about three days for Stripe and PayPal and sooner for
// MercadoPago, so the bound sits safely inside every one of them:
//   - a delivery that arrives after the bound is acknowledged with a warning
//     log, a capture_pending.expired counter and a payment-review operational
//     alert so an operator reconciles it by hand;
//   - a row that stays in retry_pending past the bound because the provider
//     stopped redelivering is expired the same way by
//     SweepExpiredPluginCapturePendingWebhooks, which runs on a ticker.
const (
	pluginCapturePendingMaxAge = 48 * time.Hour
	// pluginCapturePendingRetryAfterSeconds is advisory: providers schedule
	// their own backoff, but a client that honours Retry-After waits this long.
	pluginCapturePendingRetryAfterSeconds = 300
	// pluginCapturePendingReasonPrefix starts the webhook_events.error of a
	// parked row; the JSON context after it is what the sweeper needs to
	// raise the alert without re-parsing the provider payload.
	pluginCapturePendingReasonPrefix = "capture_pending: "
	// pluginCapturePendingExpiredPrefix replaces it once the row is expired.
	pluginCapturePendingExpiredPrefix = "capture_pending_expired: "

	pluginCapturePendingSweepInterval  = 15 * time.Minute
	pluginCapturePendingSweepBatchSize = 100
)

// pluginCapturePendingContext is stored with a parked row so the sweeper can
// raise the same alert the in-request path would.
type pluginCapturePendingContext struct {
	BusinessID uint   `json:"business_id"`
	BillID     uint   `json:"bill_id,omitempty"`
	PaymentID  string `json:"provider_payment_id"`
	Status     string `json:"provider_status"`
}

func (pc pluginCapturePendingContext) encode(prefix string) string {
	raw, err := json.Marshal(pc)
	if err != nil {
		return strings.TrimSpace(prefix)
	}
	return prefix + string(raw)
}

func parsePluginCapturePendingContext(stored string) (pluginCapturePendingContext, bool) {
	var pc pluginCapturePendingContext
	if !strings.HasPrefix(stored, pluginCapturePendingReasonPrefix) {
		return pc, false
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(stored, pluginCapturePendingReasonPrefix)), &pc); err != nil {
		return pc, false
	}
	return pc, pc.BusinessID > 0
}

// pluginWebhookStatusNeedsRecordedCapture reports the webhook statuses that act
// on an existing capture: refunds, dispute withdrawals and reinstatements, and
// dispute openings.
func pluginWebhookStatusNeedsRecordedCapture(status string) bool {
	switch status {
	case "refunded", "reversed", "dispute_reinstated", "disputed":
		return true
	default:
		return false
	}
}

// pluginWebhookStatusIsDispute reports the dispute statuses. A provider
// dispute object does not carry the merchant's bill reference (a Stripe
// Dispute's metadata is its own, not the charge's), so these are deferred for
// a verified tenant even when the event names no bill. Refunds are not: a
// Payverge charge always carries its bill id, so a bill-less refund belongs to
// a charge Payverge did not create and is acknowledged as before.
func pluginWebhookStatusIsDispute(status string) bool {
	switch status {
	case "reversed", "dispute_reinstated", "disputed":
		return true
	default:
		return false
	}
}

// extractPluginWebhookCreatedAt returns when the provider created the event,
// from a field the provider's signature covers: Stripe "created" (unix
// seconds, inside the signed body) and PayPal "create_time" (RFC 3339, the
// body PayPal verifies). MercadoPago is deliberately absent: its x-signature
// manifest signs only data.id, the request id and ts, not the body, so a
// replayed fresh header with an old body "date_created" could force an early
// stale acknowledgement. MercadoPago events are bounded by the row's first-seen
// created_at alone.
func extractPluginWebhookCreatedAt(pluginName string, payload map[string]interface{}) (time.Time, bool) {
	switch pluginName {
	case "stripe":
		switch v := payload["created"].(type) {
		case float64:
			if v > 0 && v < math.MaxInt64 {
				return time.Unix(int64(v), 0), true
			}
		case string:
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && n > 0 {
				return time.Unix(n, 0), true
			}
		}
	case "paypal":
		return parsePluginWebhookTimestamp(stringFromAny(payload["create_time"]))
	}
	return time.Time{}, false
}

func parsePluginWebhookTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// pluginCaptureAutoRefundMarkerProvider namespaces the webhook_events marker
// row written when Payverge itself refunds a capture it could not settle.
func pluginCaptureAutoRefundMarkerProvider(pluginName string) string {
	return "plugin_" + pluginName + ":capture_auto_refunded"
}

// recordPluginCaptureAutoRefunded leaves a durable marker that this capture
// was refunded by Payverge without ever reaching the ledger. The provider's
// follow-up refund webhook for it must then be acknowledged as a no-op, not
// retried for three days as an early refund. Best effort: a missing marker
// only costs bounded retries and an operator alert.
func recordPluginCaptureAutoRefunded(pluginName, paymentID string) {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return
	}
	now := time.Now()
	marker := &database.WebhookEvent{
		Provider:    pluginCaptureAutoRefundMarkerProvider(pluginName),
		WebhookID:   paymentID,
		EventType:   "capture_auto_refunded",
		Status:      "processed",
		ReceivedAt:  now,
		ProcessedAt: &now,
	}
	if _, err := database.GetDBWrapper().CreateWebhookEventIfNotExists(marker); err != nil {
		log.Printf("Failed to record auto-refund marker for %s capture %s: %v", pluginName, paymentID, err)
	}
}

func pluginCaptureWasAutoRefunded(pluginName, paymentID string) (bool, error) {
	event, err := database.GetDBWrapper().GetWebhookEvent(pluginCaptureAutoRefundMarkerProvider(pluginName), strings.TrimSpace(paymentID))
	if err != nil {
		return false, err
	}
	return event != nil, nil
}

// pluginWebhookOldestKnownAt is the earliest time this event is known to have
// existed: the provider's own creation time, or the first time Payverge stored
// it (webhook_events.created_at survives retry claims, received_at does not).
func pluginWebhookOldestKnownAt(pluginName string, payload map[string]interface{}, providerKey, webhookID string, now time.Time) time.Time {
	oldest := now
	if created, ok := extractPluginWebhookCreatedAt(pluginName, payload); ok && created.Before(oldest) {
		oldest = created
	}
	if webhookID != "" {
		if event, err := database.GetDBWrapper().GetWebhookEvent(providerKey, webhookID); err == nil && event != nil &&
			!event.CreatedAt.IsZero() && event.CreatedAt.Before(oldest) {
			oldest = event.CreatedAt
		}
	}
	return oldest
}

// deferPluginWebhookUntilCaptureRecorded applies D3. It runs after signature
// verification, the dedup claim and the bill ownership check, for a status that
// acts on an existing capture. businessID is the tenant whose secret (or, for
// Stripe Connect, whose connected account) verified the event. billID is the
// bill the signed event names, already verified to belong to businessID, or 0.
//
// A dispute is deferred even with billID 0, because provider dispute objects
// do not carry the bill reference; retrying is harmless and needs no bill.
//
// It returns true when it wrote the response (retry or stale ack) and the
// caller must stop. It returns false to let the normal path continue: the
// capture is recorded, the lookup failed (the reversal path fails closed on
// its own), the capture was auto-refunded by Payverge, or the event is a
// bill-less refund (a charge Payverge did not create, acknowledged as before).
func (ph *PluginHandlers) deferPluginWebhookUntilCaptureRecorded(
	c *gin.Context,
	pluginName string,
	businessID, billID uint,
	payload map[string]interface{},
	response *plugins.WebhookResponse,
	providerKey, webhookID string,
) bool {
	if response == nil || businessID == 0 || !pluginWebhookStatusNeedsRecordedCapture(response.Status) {
		return false
	}
	if billID == 0 && !pluginWebhookStatusIsDispute(response.Status) {
		return false
	}
	paymentID := strings.TrimSpace(response.PaymentID)
	if paymentID == "" {
		return false
	}
	txHash := fmt.Sprintf("plugin_%s", paymentID)
	if _, err := database.GetPaymentByTxHash(txHash); err == nil || !errors.Is(err, database.ErrPaymentNotFound) {
		return false
	}

	autoRefunded, markerErr := pluginCaptureWasAutoRefunded(pluginName, paymentID)
	if markerErr != nil {
		log.Printf("Failed to check auto-refund marker for %s payment %s: %v", pluginName, paymentID, markerErr)
		if webhookID != "" {
			_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(markerErr.Error(), 1000))
		}
		server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
		return true
	}
	if autoRefunded {
		log.Printf("%s %s for payment %s matches a capture Payverge auto-refunded; nothing to reverse", pluginName, response.Status, paymentID)
		return false
	}

	now := time.Now()
	age := now.Sub(pluginWebhookOldestKnownAt(pluginName, payload, providerKey, webhookID, now))
	pending := pluginCapturePendingContext{BusinessID: businessID, BillID: billID, PaymentID: paymentID, Status: response.Status}
	if age > pluginCapturePendingMaxAge {
		var webhookEventID uint
		if webhookID != "" {
			if event, err := database.GetDBWrapper().GetWebhookEvent(providerKey, webhookID); err == nil && event != nil {
				webhookEventID = event.ID
			}
			if err := database.GetDBWrapper().MarkWebhookEventProcessed(providerKey, webhookID); err != nil {
				log.Printf("Failed to mark webhook as processed for %s: %v", pluginName, err)
			}
		}
		raisePluginCapturePendingExpired(context.Background(), pluginName, pending, webhookID, webhookEventID, age, "webhook_delivery")
		c.JSON(http.StatusOK, gin.H{"message": "Acknowledged without a recorded capture; operator alert raised"})
		return true
	}

	c.Set(pluginWebhookFailureReasonKey, metrics.WebhookFailureCapturePending)
	if webhookID != "" {
		if err := database.GetDBWrapper().MarkWebhookEventRetryPending(providerKey, webhookID, truncateWebhookError(pending.encode(pluginCapturePendingReasonPrefix), 1000)); err != nil {
			log.Printf("Failed to mark webhook retry-pending for %s: %v", pluginName, err)
		}
	}
	log.Printf("Deferring %s %s webhook for payment %s on bill %d: capture not recorded yet (age %s)", pluginName, response.Status, paymentID, billID, age.Truncate(time.Second))
	c.Header("Retry-After", strconv.Itoa(pluginCapturePendingRetryAfterSeconds))
	server.RespondWithError(c, http.StatusServiceUnavailable, "", "Capture not recorded yet; retry later")
	return true
}

// raisePluginCapturePendingExpired is the single exit for an early refund,
// dispute or reversal whose capture never landed within the bound: a warning
// log, the capture_pending.expired counter and a payment-review operational
// alert. The alert is bill-scoped when the event named a bill, and scoped to
// the webhook_events row otherwise.
func raisePluginCapturePendingExpired(ctx context.Context, pluginName string, pc pluginCapturePendingContext, webhookID string, webhookEventID uint, age time.Duration, source string) {
	metrics.PaymentWebhookUnsupportedActions.WithLabelValues(pluginName, "capture_pending.expired").Inc()
	ageHours := int64(age / time.Hour)
	slog.Warn("payment webhook acknowledged without a recorded capture",
		"plugin", pluginName,
		"webhook_id", webhookID,
		"business_id", pc.BusinessID,
		"bill_id", pc.BillID,
		"provider_payment_id", pc.PaymentID,
		"provider_status", pc.Status,
		"event_age_hours", ageHours,
		"expired_by", source,
	)
	metadata := map[string]any{
		"provider":            pluginName,
		"provider_payment_id": pc.PaymentID,
		"provider_status":     pc.Status,
		"settlement_source":   "plugin_webhook",
		"ledger_applied":      false,
		"event_age_hours":     ageHours,
		"expired_by":          source,
		"reconciliation_note": fmt.Sprintf("provider refund/dispute/reversal arrived for a capture that was never recorded in the ledger; acknowledged after %dh without it — reconcile manually against the provider dashboard", int64(pluginCapturePendingMaxAge/time.Hour)),
	}
	if pc.BillID > 0 {
		bill, _, billErr := database.GetBillByIDLean(pc.BillID)
		if billErr == nil && bill != nil {
			createPaymentRefundReviewOperationalAlert(ctx, bill, "plugin_"+pc.PaymentID, metadata)
			return
		}
		log.Printf("Failed to load bill %d for capture-pending alert: %v", pc.BillID, billErr)
	}
	if pc.BusinessID == 0 || webhookEventID == 0 {
		log.Printf("capture-pending alert skipped for %s payment %s: no business or webhook row to scope it", pluginName, pc.PaymentID)
		return
	}
	metadata["transaction_hash"] = "plugin_" + pc.PaymentID
	if err := operational_alerts.NewService(database.GetDB()).CreatePaymentWebhookReviewAlert(ctx, pc.BusinessID, webhookEventID, metadata); err != nil {
		log.Printf("failed to create capture-pending operational alert: business_id=%d webhook_event_id=%d error=%v", pc.BusinessID, webhookEventID, err)
	}
}

// SweepExpiredPluginCapturePendingWebhooks expires retry_pending rows the
// provider stopped redelivering. A row first stored more than
// pluginCapturePendingMaxAge before now is moved to processed (only while it
// is still retry_pending, so an in-flight redelivery wins) and raises the same
// alert as a stale delivery. It returns how many rows it expired.
func SweepExpiredPluginCapturePendingWebhooks(ctx context.Context, now time.Time) (int, error) {
	db := database.GetDBWrapper()
	rows, err := db.ListRetryPendingWebhookEventsBefore(now.Add(-pluginCapturePendingMaxAge), pluginCapturePendingSweepBatchSize)
	if err != nil {
		return 0, err
	}
	expired := 0
	for _, row := range rows {
		if ctx.Err() != nil {
			return expired, ctx.Err()
		}
		pluginName, ok := strings.CutPrefix(row.Provider, "plugin_")
		if !ok || pluginName == "" {
			continue
		}
		pending, parsed := parsePluginCapturePendingContext(row.Error)
		note := pending.encode(pluginCapturePendingExpiredPrefix)
		if !parsed {
			note = pluginCapturePendingExpiredPrefix + truncateWebhookError(row.Error, 900)
		}
		moved, err := db.ExpireRetryPendingWebhookEvent(row.ID, truncateWebhookError(note, 1000))
		if err != nil {
			return expired, err
		}
		if !moved {
			continue
		}
		expired++
		if !parsed {
			metrics.PaymentWebhookUnsupportedActions.WithLabelValues(pluginName, "capture_pending.expired").Inc()
			log.Printf("Expired retry-pending %s webhook %s without a readable context; no alert raised", pluginName, row.WebhookID)
			continue
		}
		raisePluginCapturePendingExpired(ctx, pluginName, pending, row.WebhookID, row.ID, now.Sub(row.CreatedAt), "sweeper")
	}
	return expired, nil
}

// StartPluginCapturePendingSweeper runs SweepExpiredPluginCapturePendingWebhooks
// on a ticker until ctx is cancelled; the returned channel closes on exit.
func StartPluginCapturePendingSweeper(ctx context.Context, interval time.Duration) <-chan struct{} {
	if interval <= 0 {
		interval = pluginCapturePendingSweepInterval
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := func() {
			logger.SafeTick("plugin-capture-pending-sweeper", func() {
				n, err := SweepExpiredPluginCapturePendingWebhooks(ctx, time.Now())
				if err != nil && ctx.Err() == nil {
					logger.Logger.Warnf("capture-pending webhook sweep failed after %d rows: %v", n, err)
					return
				}
				if n > 0 {
					logger.Logger.Warnf("capture-pending webhook sweep expired %d rows; payment-review alerts raised", n)
				}
			})
		}
		tick()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tick()
			}
		}
	}()
	return done
}
