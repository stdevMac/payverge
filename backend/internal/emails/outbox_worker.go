package emails

import (
	"context"
	"errors"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

const (
	defaultOutboxBatchSize         = 25
	defaultOutboxMaxAttempts       = 5
	defaultOutboxReclaimStaleAfter = 2 * time.Minute
	defaultOutboxTickInterval      = 5 * time.Second
	// DefaultOutboxTTL bounds how long an undelivered row may sit in the queue
	// before the janitor drops it (a worker that is down for a day should not
	// let the queue grow without limit).
	DefaultOutboxTTL = 24 * time.Hour
	// DefaultOutboxRetention bounds how long terminal rows are kept. The payload
	// carries the guest's plaintext address and body, so it is deliberately
	// short — long enough for delivery webhooks and post-incident forensics.
	DefaultOutboxRetention = 14 * 24 * time.Hour
)

// OutboxSender delivers one queued message. EmailServer implements it.
type OutboxSender interface {
	DeliverOutboxMessage(ctx context.Context, msg EmailMessage) (SendResult, error)
	RecordOutboxFailure(ctx context.Context, msg EmailMessage, sendErr error)
	ProviderLabel() string
}

// OutboxWorker drains the email outbox. It is the email twin of
// services.PluginNotificationWorker: bounded batch, injectable clock, stale
// claim reclamation, exponential backoff, terminal failed/dropped states.
type OutboxWorker struct {
	Store       OutboxStore
	Sender      OutboxSender
	BatchSize   int
	MaxAttempts int
	// ReclaimStaleAfter is how long a row may sit in "processing" before it is
	// presumed orphaned by a crashed worker. Must exceed any real send.
	ReclaimStaleAfter        time.Duration
	QueueDepthSampleInterval time.Duration
	Now                      func() time.Time

	lastQueueDepthAt time.Time
}

func NewOutboxWorker(store OutboxStore, sender OutboxSender) *OutboxWorker {
	return &OutboxWorker{
		Store:       store,
		Sender:      sender,
		BatchSize:   defaultOutboxBatchSize,
		MaxAttempts: defaultOutboxMaxAttempts,
	}
}

// ProcessDue reclaims stranded rows, claims a batch of due sends, and drives
// each to a terminal or retry state. Returns the number of rows claimed.
func (w *OutboxWorker) ProcessDue(ctx context.Context) (int, error) {
	if w == nil || w.Store == nil || w.Sender == nil {
		return 0, errors.New("email outbox worker requires a store and a sender")
	}
	now := w.now()

	if reclaimed, err := w.Store.ReclaimStale(ctx, w.reclaimStaleAfter(), w.maxAttempts(), now); err != nil {
		logger.Logger.Warnf("Email outbox reclaim failed: %v", err)
	} else if reclaimed > 0 {
		logger.Logger.Infof("Email outbox worker reclaimed %d stranded rows", reclaimed)
	}

	rows, err := w.Store.Claim(ctx, w.batchSize(), now)
	if err != nil {
		return 0, err
	}
	w.recordQueueDepthIfDue(ctx, now)

	for i := range rows {
		w.deliver(ctx, rows[i])
	}
	w.recordQueueDepthIfDue(ctx, w.now())
	return len(rows), nil
}

func (w *OutboxWorker) deliver(ctx context.Context, row EmailOutbox) {
	result, err := w.Sender.DeliverOutboxMessage(ctx, row.Payload)
	now := w.now()
	if err == nil {
		if markErr := w.Store.MarkSent(ctx, row.ID, w.Sender.ProviderLabel(), result.ProviderMessageID, now); markErr != nil {
			// A failed mark leaves the row in "processing" and the stale
			// reclaim will send it again. Resend deduplicates on the
			// idempotency key; Postmark and SMTP do not (SMTP reuses a
			// Message-ID derived from the key, which some receivers collapse),
			// so a duplicate is possible there. Provider-side failures follow
			// isRetryableEmailFailure: an SMTP send whose final reply was lost
			// is "outcome_unknown" and is not retried.
			logger.Logger.Errorf("Email outbox: failed to mark row %d sent: %v — it will be retried after reclaim", row.ID, markErr)
			return
		}
		metrics.EmailOutboxDelivered.WithLabelValues(w.Sender.ProviderLabel(), row.TemplateName).Inc()
		if !row.CreatedAt.IsZero() && now.After(row.CreatedAt) {
			metrics.EmailOutboxDeliveryLatency.WithLabelValues(row.TemplateName).Observe(now.Sub(row.CreatedAt).Seconds())
		}
		return
	}

	reason := classifyEmailFailure(err)
	switch {
	case errors.Is(err, ErrRecipientSuppressed):
		w.markTerminal(ctx, row, OutboxStatusDropped, "suppressed", err.Error(), now)
		metrics.EmailOutboxDropped.WithLabelValues(row.TemplateName, "suppressed").Inc()
	case !isRetryableEmailFailure(err), row.AttemptCount >= w.maxAttempts():
		w.markTerminal(ctx, row, OutboxStatusFailed, reason, err.Error(), now)
		metrics.EmailOutboxFailed.WithLabelValues(row.TemplateName, reason).Inc()
		// Mirrors the synchronous path: the redacted outbound ledger row and the
		// EmailSendExhausted counter are both written by RecordOutboxFailure.
		w.Sender.RecordOutboxFailure(ctx, row.Payload, err)
		logrus.WithFields(logrus.Fields{
			"outbox_id":     row.ID,
			"template_name": row.TemplateName,
			"attempts":      row.AttemptCount,
			"reason":        reason,
		}).WithError(err).Error("email outbox send failed permanently")
	default:
		next := now.Add(w.backoffForAttempt(row.AttemptCount))
		if markErr := w.Store.MarkRetry(ctx, row.ID, reason, err.Error(), next, now); markErr != nil {
			logger.Logger.Errorf("Email outbox: failed to mark row %d for retry: %v", row.ID, markErr)
			return
		}
		metrics.EmailOutboxRetried.WithLabelValues(row.TemplateName, reason).Inc()
	}
}

func (w *OutboxWorker) markTerminal(ctx context.Context, row EmailOutbox, status, code, message string, now time.Time) {
	if err := w.Store.MarkTerminal(ctx, row.ID, status, code, message, now); err != nil {
		logger.Logger.Errorf("Email outbox: failed to mark row %d %s: %v", row.ID, status, err)
	}
}

func (w *OutboxWorker) recordQueueDepthIfDue(ctx context.Context, now time.Time) {
	if !w.lastQueueDepthAt.IsZero() && now.Sub(w.lastQueueDepthAt) < w.queueDepthSampleInterval() {
		return
	}
	count, err := w.Store.CountBacklog(ctx, now)
	if err != nil {
		return
	}
	metrics.EmailOutboxQueueDepth.Set(float64(count))
	w.lastQueueDepthAt = now
}

// backoffForAttempt mirrors the Telegram outbox schedule: 1m, 1m, 5m, 15m, 1h,
// then 6h. attempt is the post-claim attempt count of the row that just failed.
func (w *OutboxWorker) backoffForAttempt(attempt int) time.Duration {
	switch attempt {
	case 0, 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	case 3:
		return 15 * time.Minute
	case 4:
		return time.Hour
	default:
		return 6 * time.Hour
	}
}

func (w *OutboxWorker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w *OutboxWorker) batchSize() int {
	if w.BatchSize > 0 {
		return w.BatchSize
	}
	return defaultOutboxBatchSize
}

func (w *OutboxWorker) maxAttempts() int {
	if w.MaxAttempts > 0 {
		return w.MaxAttempts
	}
	return defaultOutboxMaxAttempts
}

func (w *OutboxWorker) reclaimStaleAfter() time.Duration {
	if w.ReclaimStaleAfter > 0 {
		return w.ReclaimStaleAfter
	}
	return defaultOutboxReclaimStaleAfter
}

func (w *OutboxWorker) queueDepthSampleInterval() time.Duration {
	if w.QueueDepthSampleInterval > 0 {
		return w.QueueDepthSampleInterval
	}
	return 30 * time.Second
}

// StartEmailOutboxWorker runs w.ProcessDue on interval until ctx is cancelled.
// A panic in one tick is contained by logger.SafeTick, exactly like the
// Telegram notification worker.
func StartEmailOutboxWorker(ctx context.Context, w *OutboxWorker, interval time.Duration) {
	if w == nil {
		return
	}
	if interval <= 0 {
		interval = defaultOutboxTickInterval
	}
	logger.SafeGo(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logger.SafeTick("email-outbox-worker", func() {
					if processed, err := w.ProcessDue(ctx); err != nil {
						logger.Logger.Warnf("Email outbox worker failed: %v", err)
					} else if processed > 0 {
						logger.Logger.Debugf("Email outbox worker processed %d sends", processed)
					}
				})
			}
		}
	})
}

// StartEmailOutboxJanitor expires stale pending rows (TTL) and purges terminal
// rows past the retention window, so the queue stays bounded and the plaintext
// payloads do not linger.
func StartEmailOutboxJanitor(ctx context.Context, store OutboxStore, ttl, retention, interval time.Duration) {
	if store == nil {
		return
	}
	if ttl <= 0 {
		ttl = DefaultOutboxTTL
	}
	if retention <= 0 {
		retention = DefaultOutboxRetention
	}
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	logger.SafeGo(func() {
		runEmailOutboxMaintenance(ctx, store, ttl, retention)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runEmailOutboxMaintenance(ctx, store, ttl, retention)
			}
		}
	})
}

func runEmailOutboxMaintenance(ctx context.Context, store OutboxStore, ttl, retention time.Duration) {
	logger.SafeTick("email-outbox-janitor", func() {
		now := time.Now().UTC()
		if expired, err := store.ExpirePending(ctx, now.Add(-ttl), now); err != nil {
			logger.Logger.Warnf("Email outbox janitor expiry failed: %v", err)
		} else if expired > 0 {
			logger.Logger.Warnf("Email outbox janitor dropped %d undelivered emails older than %s", expired, ttl)
		}
		if purged, err := store.PurgeTerminal(ctx, now.Add(-retention)); err != nil {
			logger.Logger.Warnf("Email outbox janitor purge failed: %v", err)
		} else if purged > 0 {
			logger.Logger.Infof("Email outbox janitor purged %d terminal rows older than %s", purged, retention)
		}
	})
}
