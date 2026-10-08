package fiscal

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DeliveryChannelSpec describes one channel to enqueue for a receipt.
type DeliveryChannelSpec struct {
	Channel string
	// Locale is optional non-PII locale for email/print execution.
	Locale string
}

// EnqueueDeliveryTasks inserts pending delivery tasks for the given channels
// inside tx (caller's transaction). Idempotent on (receipt_id, channel):
// existing rows are left unchanged (ON CONFLICT DO NOTHING). baseKey is a
// stable prefix; each channel gets baseKey+":"+channel as idempotency_key.
func (r *Repository) EnqueueDeliveryTasks(
	tx *gorm.DB,
	receiptID, businessID uint,
	channels []DeliveryChannelSpec,
	baseKey string,
	now time.Time,
) error {
	if tx == nil {
		tx = r.db
	}
	if len(channels) == 0 {
		return nil
	}
	now = now.UTC()
	baseKey = strings.TrimSpace(baseKey)
	if baseKey == "" {
		baseKey = fmt.Sprintf("receipt:%d", receiptID)
	}

	for _, ch := range channels {
		channel := strings.TrimSpace(ch.Channel)
		if channel == "" {
			continue
		}
		if !isValidDeliveryChannel(channel) {
			return fmt.Errorf("invalid delivery channel %q", channel)
		}
		task := database.FiscalDeliveryTask{
			BusinessID:     businessID,
			ReceiptID:      receiptID,
			Channel:        channel,
			Status:         database.FiscalDeliveryStatusPending,
			Attempts:       0,
			MaxAttempts:    database.DefaultFiscalDeliveryMaxAttempts,
			NextAttemptAt:  &now,
			IdempotencyKey: fmt.Sprintf("%s:%s", baseKey, channel),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if loc := strings.TrimSpace(ch.Locale); loc != "" {
			task.Locale = &loc
		}
		// Clauses.OnConflict DoNothing keeps (receipt_id, channel) unique without
		// erroring on re-enqueue after a crash between persist and mark.
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "receipt_id"}, {Name: "channel"}},
			DoNothing: true,
		}).Create(&task).Error; err != nil {
			return fmt.Errorf("enqueue delivery task %s: %w", channel, err)
		}
	}
	return nil
}

// ClaimDueDeliveryTasks leases up to limit due tasks for workerID.
// Due = status pending with next_attempt_at <= now, OR status leased with
// expired lease. Uses FOR UPDATE SKIP LOCKED on Postgres.
func (r *Repository) ClaimDueDeliveryTasks(
	workerID string,
	now time.Time,
	leaseDur time.Duration,
	limit int,
) ([]database.FiscalDeliveryTask, error) {
	if limit <= 0 {
		return nil, nil
	}
	if leaseDur <= 0 {
		leaseDur = 2 * time.Minute
	}
	now = now.UTC()
	leaseExp := now.Add(leaseDur)
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "fiscal-delivery-worker"
	}

	var tasks []database.FiscalDeliveryTask
	err := r.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&database.FiscalDeliveryTask{}).
			Where(`(
				(status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?))
				OR (status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at < ?)
			)`,
				database.FiscalDeliveryStatusPending, now,
				database.FiscalDeliveryStatusLeased, now,
			).
			Order("next_attempt_at ASC NULLS FIRST, id ASC").
			Limit(limit)

		if fiscalDeliveryClaimUsesSkipLocked(tx.Name()) {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}

		var ids []uint
		if err := query.Pluck("id", &ids).Error; err != nil {
			return fmt.Errorf("delivery claim select: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}

		result := tx.Model(&database.FiscalDeliveryTask{}).
			Where("id IN ?", ids).
			Where(`(
				(status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?))
				OR (status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at < ?)
			)`,
				database.FiscalDeliveryStatusPending, now,
				database.FiscalDeliveryStatusLeased, now,
			).
			Updates(map[string]interface{}{
				"status":           database.FiscalDeliveryStatusLeased,
				"lease_owner":      workerID,
				"lease_expires_at": leaseExp,
				"updated_at":       now,
			})
		if result.Error != nil {
			return fmt.Errorf("delivery claim update: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}

		if err := tx.Where("id IN ? AND status = ? AND lease_owner = ?",
			ids, database.FiscalDeliveryStatusLeased, workerID).
			Order("next_attempt_at ASC NULLS FIRST, id ASC").
			Find(&tasks).Error; err != nil {
			return fmt.Errorf("delivery claim reload: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ClaimDueDeliveryTasks: %w", err)
	}
	return tasks, nil
}

// MarkDeliverySucceeded transitions a leased task owned by workerID to succeeded.
// providerMessageID is optional (email provider message id).
func (r *Repository) MarkDeliverySucceeded(taskID uint, workerID string, providerMessageID string, now time.Time) error {
	now = now.UTC()
	updates := map[string]interface{}{
		"status":           database.FiscalDeliveryStatusSucceeded,
		"succeeded_at":     now,
		"lease_owner":      "",
		"lease_expires_at": nil,
		"last_error":       "",
		"updated_at":       now,
	}
	if mid := strings.TrimSpace(providerMessageID); mid != "" {
		updates["provider_message_id"] = mid
	}
	res := r.db.Model(&database.FiscalDeliveryTask{}).
		Where("id = ? AND status = ? AND lease_owner = ?",
			taskID, database.FiscalDeliveryStatusLeased, workerID).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("MarkDeliverySucceeded: task %d not leased by %s", taskID, workerID)
	}
	return nil
}

// MarkDeliveryDispatched records that an at-least-once side effect (the receipt
// email) was accepted by the provider, BEFORE the task is finalized as
// succeeded. It stamps provider_message_id only while the task is still leased by
// workerID and the marker is unset, so it is idempotent and lease-scoped. This
// closes the duplicate-send window: if the process dies (or the lease expires)
// after the provider accepted the email but before MarkDeliverySucceeded, a
// re-claim sees the persisted marker and skips the resend. A missing/absent
// marker (RowsAffected == 0) is not an error — the caller already sent, and a
// concurrent finalizer may have cleared the lease.
func (r *Repository) MarkDeliveryDispatched(taskID uint, workerID string, providerMessageID string, now time.Time) error {
	mid := strings.TrimSpace(providerMessageID)
	if mid == "" {
		return nil
	}
	now = now.UTC()
	return r.db.Model(&database.FiscalDeliveryTask{}).
		Where("id = ? AND status = ? AND lease_owner = ? AND (provider_message_id IS NULL OR provider_message_id = ?)",
			taskID, database.FiscalDeliveryStatusLeased, workerID, "").
		Updates(map[string]interface{}{
			"provider_message_id": mid,
			"updated_at":          now,
		}).Error
}

// MarkDeliveryFailed records a failed attempt. When permanent or attempts
// exhaust max_attempts, status becomes dead; otherwise pending with next_attempt_at.
func (r *Repository) MarkDeliveryFailed(
	taskID uint,
	workerID string,
	errMsg string,
	nextAttemptAt time.Time,
	permanent bool,
	now time.Time,
) error {
	now = now.UTC()
	errMsg = boundDeliveryError(errMsg)

	var task database.FiscalDeliveryTask
	if err := r.db.Select("id", "attempts", "max_attempts", "status", "lease_owner").
		Where("id = ?", taskID).First(&task).Error; err != nil {
		return err
	}
	if task.Status != database.FiscalDeliveryStatusLeased || task.LeaseOwner != workerID {
		return fmt.Errorf("MarkDeliveryFailed: task %d not leased by %s", taskID, workerID)
	}

	attempts := task.Attempts + 1
	maxAttempts := task.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = database.DefaultFiscalDeliveryMaxAttempts
	}

	updates := map[string]interface{}{
		"attempts":         attempts,
		"last_error":       errMsg,
		"lease_owner":      "",
		"lease_expires_at": nil,
		"updated_at":       now,
	}

	if permanent || attempts >= maxAttempts {
		updates["status"] = database.FiscalDeliveryStatusDead
		updates["dead_at"] = now
		updates["next_attempt_at"] = nil
	} else {
		na := nextAttemptAt.UTC()
		updates["status"] = database.FiscalDeliveryStatusPending
		updates["next_attempt_at"] = na
	}

	return r.db.Model(&database.FiscalDeliveryTask{}).
		Where("id = ? AND status = ? AND lease_owner = ?",
			taskID, database.FiscalDeliveryStatusLeased, workerID).
		Updates(updates).Error
}

// DeferDelivery returns a leased task to pending until nextAttemptAt without
// incrementing attempts: the side effect was refused by a quota that resets on
// its own (DeliveryDeferredError), so the refusal must not count toward
// max_attempts.
func (r *Repository) DeferDelivery(taskID uint, workerID string, errMsg string, nextAttemptAt time.Time, now time.Time) error {
	now = now.UTC()
	res := r.db.Model(&database.FiscalDeliveryTask{}).
		Where("id = ? AND status = ? AND lease_owner = ?",
			taskID, database.FiscalDeliveryStatusLeased, workerID).
		Updates(map[string]interface{}{
			"status":           database.FiscalDeliveryStatusPending,
			"next_attempt_at":  nextAttemptAt.UTC(),
			"last_error":       boundDeliveryError(errMsg),
			"lease_owner":      "",
			"lease_expires_at": nil,
			"updated_at":       now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("DeferDelivery: task %d not leased by %s", taskID, workerID)
	}
	return nil
}

// MarkDeliveryDead force-dead-letters a leased task (e.g. permanent validation).
func (r *Repository) MarkDeliveryDead(taskID uint, workerID string, errMsg string, now time.Time) error {
	now = now.UTC()
	res := r.db.Model(&database.FiscalDeliveryTask{}).
		Where("id = ? AND status = ? AND lease_owner = ?",
			taskID, database.FiscalDeliveryStatusLeased, workerID).
		Updates(map[string]interface{}{
			"status":           database.FiscalDeliveryStatusDead,
			"dead_at":          now,
			"last_error":       boundDeliveryError(errMsg),
			"lease_owner":      "",
			"lease_expires_at": nil,
			"next_attempt_at":  nil,
			"updated_at":       now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("MarkDeliveryDead: task %d not leased by %s", taskID, workerID)
	}
	return nil
}

// RequeueDeliveryTask moves a dead (or failed-pending with exhausted attempts)
// task back to pending for operator retry. Bumps idempotency_key with a salt so
// provider-side dedup keys are unique for the new attempt cycle. Only dead
// tasks (and optionally pending with last_error) are requeued; succeeded is a no-op.
func (r *Repository) RequeueDeliveryTask(taskID, businessID uint, actor string, now time.Time) (*database.FiscalDeliveryTask, error) {
	now = now.UTC()
	var task database.FiscalDeliveryTask
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND business_id = ?", taskID, businessID).
			First(&task).Error; err != nil {
			return err
		}
		if task.Status == database.FiscalDeliveryStatusSucceeded {
			return nil // already done — idempotent no-op for double-click
		}
		if task.Status != database.FiscalDeliveryStatusDead &&
			task.Status != database.FiscalDeliveryStatusPending {
			// Leased tasks cannot be operator-requeued mid-flight.
			return fmt.Errorf("task %d status %q is not requeueable", taskID, task.Status)
		}
		// Only requeue dead (or pending that is effectively stuck with error).
		if task.Status == database.FiscalDeliveryStatusPending && task.LastError == "" {
			return nil
		}
		salt := fmt.Sprintf("retry:%d:%s", now.UnixNano(), strings.TrimSpace(actor))
		newKey := task.IdempotencyKey
		if idx := strings.Index(newKey, "|retry:"); idx >= 0 {
			newKey = newKey[:idx]
		}
		newKey = newKey + "|" + salt
		if len(newKey) > 200 {
			newKey = newKey[:200]
		}
		updates := map[string]interface{}{
			"status":           database.FiscalDeliveryStatusPending,
			"next_attempt_at":  now,
			"dead_at":          nil,
			"succeeded_at":     nil,
			"lease_owner":      "",
			"lease_expires_at": nil,
			"last_error":       "",
			"attempts":         0,
			"idempotency_key":  newKey,
			"updated_at":       now,
		}
		if err := tx.Model(&database.FiscalDeliveryTask{}).
			Where("id = ? AND business_id = ?", taskID, businessID).
			Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", taskID).First(&task).Error
	})
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// ListDeliveryTasksForReceipt returns all channel tasks for a receipt, scoped
// to businessID (cross-tenant denial).
func (r *Repository) ListDeliveryTasksForReceipt(businessID, receiptID uint) ([]database.FiscalDeliveryTask, error) {
	var tasks []database.FiscalDeliveryTask
	err := r.db.Where("business_id = ? AND receipt_id = ?", businessID, receiptID).
		Order("channel ASC").
		Find(&tasks).Error
	return tasks, err
}

// CountPendingDeliveryTasks returns pending + leased (non-terminal) task count.
func (r *Repository) CountPendingDeliveryTasks() (int64, error) {
	var n int64
	err := r.db.Model(&database.FiscalDeliveryTask{}).
		Where("status IN ?", []string{
			database.FiscalDeliveryStatusPending,
			database.FiscalDeliveryStatusLeased,
		}).
		Count(&n).Error
	return n, err
}

// OldestPendingDeliveryAge returns age of the oldest due pending task, or 0.
func (r *Repository) OldestPendingDeliveryAge(now time.Time) (time.Duration, error) {
	now = now.UTC()
	var task database.FiscalDeliveryTask
	err := r.db.Select("id", "next_attempt_at", "created_at").
		Where("status = ?", database.FiscalDeliveryStatusPending).
		Order("next_attempt_at ASC NULLS FIRST, id ASC").
		First(&task).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}
	anchor := task.CreatedAt
	if task.NextAttemptAt != nil {
		anchor = *task.NextAttemptAt
	}
	if anchor.After(now) {
		return 0, nil
	}
	return now.Sub(anchor), nil
}

// CountLegacyUndeliveredAuthorizedReceipts counts authorized receipts still
// missing delivered_at (legacy coarse-sweep backlog). Used to gate retirement
// of SweepUndeliveredReceipts.
func (r *Repository) CountLegacyUndeliveredAuthorizedReceipts() (int64, error) {
	var n int64
	err := r.db.Model(&database.FiscalReceipt{}).
		Where("status = ? AND delivered_at IS NULL", database.FiscalStatusAuthorized).
		Count(&n).Error
	return n, err
}

// BackfillDeliveryTasksForAuthorizedReceipts creates missing channel tasks for
// authorized receipts whose aggregate delivery state shows pending work.
// Idempotent. Does not remove the coarse sweep (Task 5 gates that).
func (r *Repository) BackfillDeliveryTasksForAuthorizedReceipts(now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	now = now.UTC()
	var receipts []database.FiscalReceipt
	// provider=demo receipts are simulated (seeded or sandbox-issued) and never
	// get real artifacts; enqueuing transport work for them only dead-letters.
	err := r.db.
		Where("status = ? AND provider <> ? AND (delivered_at IS NULL OR pdf_path IS NULL OR pdf_path = '')",
			database.FiscalStatusAuthorized, demoProviderName).
		Order("id ASC").
		Limit(limit).
		Find(&receipts).Error
	if err != nil {
		return 0, err
	}
	created := 0
	for i := range receipts {
		rec := receipts[i]
		before, _ := r.countTasksForReceipt(rec.ID)
		channels := []DeliveryChannelSpec{
			{Channel: database.FiscalDeliveryChannelArtifact},
			{Channel: database.FiscalDeliveryChannelEmail},
			{Channel: database.FiscalDeliveryChannelPrint},
		}
		if err := r.EnqueueDeliveryTasks(r.db, rec.ID, rec.BusinessID, channels,
			fmt.Sprintf("backfill:receipt:%d:channel", rec.ID), now); err != nil {
			return created, err
		}
		after, _ := r.countTasksForReceipt(rec.ID)
		if after > before {
			created += int(after - before)
		}
	}
	return created, nil
}

func (r *Repository) countTasksForReceipt(receiptID uint) (int64, error) {
	var n int64
	err := r.db.Model(&database.FiscalDeliveryTask{}).
		Where("receipt_id = ?", receiptID).
		Count(&n).Error
	return n, err
}

func isValidDeliveryChannel(ch string) bool {
	switch ch {
	case database.FiscalDeliveryChannelArtifact,
		database.FiscalDeliveryChannelEmail,
		database.FiscalDeliveryChannelPrint:
		return true
	}
	return false
}

func boundDeliveryError(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	if utf8.RuneCountInString(msg) <= database.FiscalDeliveryLastErrorMaxLen {
		return msg
	}
	runes := []rune(msg)
	return string(runes[:database.FiscalDeliveryLastErrorMaxLen])
}

// fiscalDeliveryClaimUsesSkipLocked mirrors fiscalJobsClaimUsesSkipLocked.
func fiscalDeliveryClaimUsesSkipLocked(dialect string) bool {
	return dialect == "postgres"
}
