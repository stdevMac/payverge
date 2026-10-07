package handlers

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/plugins"
)

// pluginReconciliationMinAge is how long a pending plugin tracker must sit before
// the reconciliation sweep will poll the provider for it. It gives the normal
// webhook path time to settle first, so the sweep only picks up genuinely
// stranded (lost/dropped-webhook) captures.
const pluginReconciliationMinAge = 30 * time.Minute

// pluginReconciliationExpiredORDLookback is how long after a MercadoPago ORD
// tracker is marked expired we still poll the provider. Without this, the
// sweeper flips status=expired and the pending-only reconciler permanently
// drops captured-but-untracked money.
const pluginReconciliationExpiredORDLookback = 48 * time.Hour

// pluginTerminalCompletedStatuses are the provider status strings we treat as a
// confirmed, money-captured terminal state that should settle the bill.
var pluginTerminalCompletedStatuses = map[string]struct{}{
	"completed": {},
	"confirmed": {},
	"succeeded": {},
	"success":   {},
	"paid":      {},
	"approved":  {},
}

// pluginTerminalFailedStatuses are the provider status strings we treat as a
// terminal failure/expiry, so the pending tracker can be marked failed.
var pluginTerminalFailedStatuses = map[string]struct{}{
	"failed":    {},
	"cancelled": {},
	"canceled":  {},
	"expired":   {},
	"declined":  {},
	"voided":    {},
	"refunded":  {},
}

// ReconcilePendingPluginPayments is a conservative background sweep that finds
// long-pending plugin payment trackers (F9) and reconciles them against the
// provider's authoritative status, so a dropped/misconfigured webhook self-heals
// instead of stranding a captured payment off-ledger forever.
//
// It is bounded (limit rows per run), age-gated (only trackers older than
// pluginReconciliationMinAge), and errors on any single tracker are logged and
// skipped so one bad row cannot stall the batch. Settlement flows through the
// same idempotent updateBillPaymentStatus (plugin_<id> txHash) the webhook path
// uses, so a webhook that lands concurrently cannot double-credit.
//
// Q3: also inspects MercadoPago ORD trackers that the TTL sweeper marked
// expired within the last 48h, so a capture that landed after ExpiresAt is not
// permanently stranded.
//
// This method is intended to be armed on a cron (see cmd/app/main.go). It is a
// no-op when there are no eligible trackers.
func (ph *PluginHandlers) ReconcilePendingPluginPayments(ctx context.Context, limit int) {
	if limit <= 0 {
		limit = 50
	}
	// Clear stale gauges first so a successfully drained queue resolves the
	// backlog alert on the next sweep instead of preserving yesterday's age.
	for _, providerContract := range plugins.ProductionPaymentProviderContracts() {
		metrics.PaymentReconciliationOldestPendingSeconds.WithLabelValues(providerContract.Name).Set(0)
	}
	// Use wall-clock Now() (not UTC()) so SQLite datetime comparisons stay
	// consistent with GORM's default local-time storage for created_at/expires_at.
	now := time.Now()
	cutoff := now.Add(-pluginReconciliationMinAge)
	expiredORDSince := now.Add(-pluginReconciliationExpiredORDLookback)

	var trackers []database.AlternativePayment
	if err := database.GetDB().
		Where("status = ? AND created_at < ?", database.AltPaymentStatusPending, cutoff).
		Order("created_at ASC").
		Limit(limit).
		Find(&trackers).Error; err != nil {
		log.Printf("Plugin reconciliation: failed to load pending trackers: %v", err)
		return
	}

	// Q3 self-heal: recently-expired ORD trackers (status flipped by the TTL
	// sweeper) are otherwise permanently dropped from the pending-only filter.
	remaining := limit - len(trackers)
	if remaining > 0 {
		var expiredORD []database.AlternativePayment
		if err := database.GetDB().
			Where(
				"status = ? AND payment_method = ? AND expires_at IS NOT NULL AND expires_at > ? AND expires_at <= ? AND UPPER(participant_addr) LIKE ?",
				database.AltPaymentStatusExpired,
				"mercadopago",
				expiredORDSince,
				now,
				"ORD%",
			).
			Order("expires_at ASC").
			Limit(remaining).
			Find(&expiredORD).Error; err != nil {
			log.Printf("Plugin reconciliation: failed to load recently-expired ORD trackers: %v", err)
		} else if len(expiredORD) > 0 {
			seen := make(map[uint]struct{}, len(trackers))
			for _, t := range trackers {
				seen[t.ID] = struct{}{}
			}
			for _, t := range expiredORD {
				if _, ok := seen[t.ID]; ok {
					continue
				}
				trackers = append(trackers, t)
				log.Printf("Plugin reconciliation: including recently-expired ORD tracker %d payment %s (expired_at=%s) for self-heal",
					t.ID, t.ParticipantAddr, t.ExpiresAt.UTC().Format(time.RFC3339))
			}
		}
	}

	checked, settled, expired, errored := 0, 0, 0, 0
	oldestRecorded := make(map[string]bool)
	for i := range trackers {
		select {
		case <-ctx.Done():
			log.Printf("Plugin reconciliation: context cancelled after %d checked", checked)
			return
		default:
		}

		tracker := trackers[i]
		pluginName := strings.TrimSpace(string(tracker.PaymentMethod))
		providerPaymentID := strings.TrimSpace(tracker.ParticipantAddr)
		if pluginName == "" || providerPaymentID == "" {
			continue
		}

		// Only reconcile trackers whose method resolves to a registered payment
		// plugin — cash/card/venmo/other trackers are not provider-pollable.
		registered, exists := plugins.GetPluginByName(pluginName)
		if !exists {
			continue
		}
		paymentPlugin, ok := registered.(plugins.PaymentPlugin)
		if !ok {
			continue
		}
		businessID, err := database.GetBillBusinessIDByBillID(tracker.BillID)
		if err != nil || businessID == 0 {
			metrics.PaymentReconciliationOutcomes.WithLabelValues(pluginName, "binding_error").Inc()
			log.Printf("Plugin reconciliation: cannot resolve business for bill %d tracker %d: %v", tracker.BillID, tracker.ID, err)
			continue
		}

		checked++
		if !oldestRecorded[pluginName] {
			age := time.Since(tracker.CreatedAt).Seconds()
			if age < 0 {
				age = 0
			}
			metrics.PaymentReconciliationOldestPendingSeconds.WithLabelValues(pluginName).Set(age)
			oldestRecorded[pluginName] = true
		}
		status, err := paymentPlugin.GetPaymentStatus(businessID, providerPaymentID)
		if err != nil {
			// Provider poll failed (network, unsupported, transient) — back off and
			// leave the tracker pending for a later sweep.
			errored++
			metrics.PaymentReconciliationOutcomes.WithLabelValues(pluginName, "provider_error").Inc()
			log.Printf("Plugin reconciliation: GetPaymentStatus failed for %s payment %s (bill %d): %v", pluginName, providerPaymentID, tracker.BillID, err)
			continue
		}

		normalized := strings.ToLower(strings.TrimSpace(status))
		switch {
		case isPluginTerminalCompletedStatus(normalized):
			billAmountCents, tipAmountCents := reconciliationTrackerBreakdown(tracker)
			currency := reconciliationTrackerCurrency(businessID)
			if _, _, settleErr := ph.updateBillPaymentStatus(tracker.BillID, providerPaymentID, billAmountCents, tipAmountCents, currency, pluginName, nil); settleErr != nil {
				errored++
				metrics.PaymentReconciliationOutcomes.WithLabelValues(pluginName, "settlement_error").Inc()
				log.Printf("Plugin reconciliation: settle failed for %s payment %s (bill %d): %v", pluginName, providerPaymentID, tracker.BillID, settleErr)
				continue
			}
			settled++
			metrics.PaymentReconciliationOutcomes.WithLabelValues(pluginName, "settled").Inc()
			log.Printf("Plugin reconciliation: settled stranded %s payment %s on bill %d", pluginName, providerPaymentID, tracker.BillID)
		case isPluginTerminalFailedStatus(normalized):
			if failErr := ph.markPluginPaymentRecordFailed(tracker.ID); failErr != nil {
				errored++
				metrics.PaymentReconciliationOutcomes.WithLabelValues(pluginName, "expiration_error").Inc()
				log.Printf("Plugin reconciliation: failed to expire tracker %d (%s payment %s): %v", tracker.ID, pluginName, providerPaymentID, failErr)
				continue
			}
			expired++
			metrics.PaymentReconciliationOutcomes.WithLabelValues(pluginName, "expired").Inc()
			log.Printf("Plugin reconciliation: expired terminal-failed %s payment %s on bill %d (status=%s)", pluginName, providerPaymentID, tracker.BillID, normalized)
		default:
			// Still pending at the provider — leave it for a later sweep.
			metrics.PaymentReconciliationOutcomes.WithLabelValues(pluginName, "pending").Inc()
		}
	}

	if checked > 0 {
		log.Printf("Plugin reconciliation: checked=%d settled=%d expired=%d errored=%d", checked, settled, expired, errored)
	}
}

func isPluginTerminalCompletedStatus(status string) bool {
	_, ok := pluginTerminalCompletedStatuses[status]
	return ok
}

func isPluginTerminalFailedStatus(status string) bool {
	_, ok := pluginTerminalFailedStatuses[status]
	return ok
}

// reconciliationTrackerBreakdown returns the bill/tip breakdown recorded on the
// tracker, falling back to Amount-as-bill when the breakdown columns were never
// populated (legacy rows).
func reconciliationTrackerBreakdown(tracker database.AlternativePayment) (billCents, tipCents int64) {
	if tracker.BillAmountCents > 0 || tracker.TipAmountCents > 0 {
		return tracker.BillAmountCents, tracker.TipAmountCents
	}
	return tracker.Amount, 0
}

func reconciliationTrackerCurrency(businessID uint) string {
	var business database.Business
	if err := database.GetDB().Select("id", "default_currency").First(&business, businessID).Error; err != nil {
		return "USD"
	}
	return authoritativeGuestPluginCurrency(&business)
}
