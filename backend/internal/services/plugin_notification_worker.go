package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

type PluginNotificationSender interface {
	SendPluginNotification(ctx context.Context, delivery database.PluginNotificationDelivery) PluginNotificationSendResult
}

type PluginNotificationSendResult struct {
	ProviderMessageID string
	PermanentFailure  bool
	Drop              bool
	RetryAfter        time.Duration
	ErrorCode         string
	ErrorMessage      string
	Err               error
}

type PluginNotificationWorker struct {
	PluginName               string
	Sender                   PluginNotificationSender
	BatchSize                int
	MaxAttempts              int
	QueueDepthSampleInterval time.Duration
	// ReclaimStaleAfter is how long a delivery may sit in "processing" before it
	// is presumed orphaned (worker crashed/restarted between claim and terminal
	// write) and reclaimed. Must exceed any real send duration so an actively
	// sending row is never reclaimed mid-flight. Defaults to 2 minutes.
	ReclaimStaleAfter time.Duration
	Now               func() time.Time

	lastQueueDepthAt time.Time
}

func NewPluginNotificationWorker(pluginName string, sender PluginNotificationSender) *PluginNotificationWorker {
	return &PluginNotificationWorker{
		PluginName:  pluginName,
		Sender:      sender,
		BatchSize:   25,
		MaxAttempts: 5,
	}
}

func (w *PluginNotificationWorker) ProcessDue(ctx context.Context) (int, error) {
	if w.Sender == nil {
		return 0, errors.New("plugin notification sender is required")
	}
	now := w.now()
	batchSize := w.BatchSize
	if batchSize <= 0 {
		batchSize = 25
	}

	// Reclaim deliveries orphaned in "processing" by a crashed/restarted worker
	// before claiming, so a deploy/OOM/panic mid-send doesn't strand the row
	// forever (the claim ignores "processing" and the outbox janitor skips it).
	// Best-effort self-heal: a reclaim failure must not block normal delivery.
	if reclaimed, rErr := database.ReclaimStaleProcessingPluginNotificationDeliveries(w.PluginName, w.reclaimStaleAfter(), w.maxAttempts(), now); rErr != nil {
		logger.Logger.Warnf("Plugin notification reclaim (%s) failed: %v", w.PluginName, rErr)
	} else if reclaimed > 0 {
		logger.Logger.Infof("Plugin notification worker (%s) reclaimed %d stranded processing deliveries", w.PluginName, reclaimed)
	}

	deliveries, err := database.ClaimPluginNotificationDeliveries(w.PluginName, batchSize, now)
	if err != nil {
		return 0, err
	}
	w.recordQueueDepthIfDue(now)

	for _, delivery := range deliveries {
		result := w.Sender.SendPluginNotification(ctx, delivery)
		if successfulPluginNotificationResult(result) {
			// Mark THIS delivery delivered immediately, in its own transaction,
			// right after its successful send (N-4). A batched all-or-nothing mark
			// meant one failed mark tx redelivered up to BatchSize already-sent
			// messages after the stale-processing reclaim; per-message marking
			// caps that blast radius at the single row whose mark failed.
			logPluginNotificationMarkFailure(w.PluginName, "delivered", []uint{delivery.ID},
				database.MarkPluginNotificationDeliveryDeliveredAttempt(delivery.ID, delivery.AttemptCount, result.ProviderMessageID, now))
			w.recordDeliveredMetrics(delivery, now)
			continue
		}
		w.recordNonDeliveredResult(delivery, result, now)
	}
	w.recordQueueDepthIfDue(w.now())

	return len(deliveries), nil
}

func successfulPluginNotificationResult(result PluginNotificationSendResult) bool {
	return !result.Drop && !result.PermanentFailure && result.Err == nil
}

func (w *PluginNotificationWorker) recordNonDeliveredResult(delivery database.PluginNotificationDelivery, result PluginNotificationSendResult, now time.Time) {
	code := result.ErrorCode
	message := result.ErrorMessage
	if message == "" && result.Err != nil {
		message = result.Err.Error()
	}

	switch {
	case result.Drop:
		logPluginNotificationMarkFailure(delivery.PluginName, "dropped", []uint{delivery.ID},
			database.MarkPluginNotificationDeliveryDropped(delivery.ID, delivery.AttemptCount, code, message, now))
		metrics.PluginNotificationFailed.WithLabelValues(delivery.PluginName, delivery.EventType, reasonLabel(code)).Inc()
	case result.PermanentFailure:
		logPluginNotificationMarkFailure(delivery.PluginName, "failed", []uint{delivery.ID},
			database.MarkPluginNotificationDeliveryFailed(delivery.ID, delivery.AttemptCount, code, message, now))
		metrics.PluginNotificationFailed.WithLabelValues(delivery.PluginName, delivery.EventType, reasonLabel(code)).Inc()
	case result.Err != nil:
		nextAttempt := now.Add(w.backoffForAttempt(delivery.AttemptCount))
		if result.RetryAfter > 0 {
			nextAttempt = now.Add(result.RetryAfter)
		}
		if delivery.AttemptCount >= w.maxAttempts() {
			logPluginNotificationMarkFailure(delivery.PluginName, "failed", []uint{delivery.ID},
				database.MarkPluginNotificationDeliveryFailed(delivery.ID, delivery.AttemptCount, code, message, now))
			metrics.PluginNotificationFailed.WithLabelValues(delivery.PluginName, delivery.EventType, reasonLabel(code)).Inc()
			return
		}
		logPluginNotificationMarkFailure(delivery.PluginName, "retry", []uint{delivery.ID},
			database.MarkPluginNotificationDeliveryRetry(delivery.ID, delivery.AttemptCount, code, message, nextAttempt, now))
		metrics.PluginNotificationRetried.WithLabelValues(delivery.PluginName, delivery.EventType, reasonLabel(code)).Inc()
	}
}

// logPluginNotificationMarkFailure logs a failed outbox mark write LOUDLY.
// A silently-failed mark leaves the row in "processing": after the stale
// reclaim it is redelivered — for the batched delivered-mark that means up to
// BatchSize duplicate Telegram messages. At-least-once is by design; invisible
// mark failures are not.
func logPluginNotificationMarkFailure(pluginName, op string, ids []uint, err error) {
	if err == nil {
		return
	}
	logger.Logger.Errorf(
		"Plugin notification worker (%s): failed to mark %d deliveries %s (ids=%v): %v — rows will be redelivered after reclaim",
		pluginName, len(ids), op, ids, err)
}

func (w *PluginNotificationWorker) recordDeliveredMetrics(delivery database.PluginNotificationDelivery, now time.Time) {
	metrics.PluginNotificationDelivered.WithLabelValues(delivery.PluginName, delivery.EventType).Inc()
	if !delivery.CreatedAt.IsZero() && now.After(delivery.CreatedAt) {
		metrics.PluginNotificationDeliveryLatency.WithLabelValues(delivery.PluginName, delivery.EventType).Observe(now.Sub(delivery.CreatedAt).Seconds())
	}
}

func (w *PluginNotificationWorker) recordQueueDepthIfDue(now time.Time) {
	if !w.lastQueueDepthAt.IsZero() && now.Sub(w.lastQueueDepthAt) < w.queueDepthSampleInterval() {
		return
	}
	count, err := database.CountPluginNotificationBacklog(w.PluginName, now)
	if err != nil {
		return
	}
	metrics.PluginNotificationQueueDepth.WithLabelValues(w.PluginName).Set(float64(count))
	w.lastQueueDepthAt = now
}

func reasonLabel(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return "unknown"
	}
	return code
}

func (w *PluginNotificationWorker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w *PluginNotificationWorker) maxAttempts() int {
	if w.MaxAttempts <= 0 {
		return 5
	}
	return w.MaxAttempts
}

func (w *PluginNotificationWorker) reclaimStaleAfter() time.Duration {
	if w.ReclaimStaleAfter <= 0 {
		return 2 * time.Minute
	}
	return w.ReclaimStaleAfter
}

func (w *PluginNotificationWorker) queueDepthSampleInterval() time.Duration {
	if w.QueueDepthSampleInterval <= 0 {
		return 30 * time.Second
	}
	return w.QueueDepthSampleInterval
}

func (w *PluginNotificationWorker) backoffForAttempt(attempt int) time.Duration {
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
