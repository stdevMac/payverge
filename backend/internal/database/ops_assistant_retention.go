package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// OpsAssistantRetentionResult reports how many rows one retention pass removed.
type OpsAssistantRetentionResult struct {
	ThreadsDeleted   int64
	MessagesDeleted  int64
	ToolCallsDeleted int64
	RequestsDeleted  int64
}

// DeleteInactiveOpsAssistantThreads deletes up to `limit` ops assistant
// threads whose last message is older than cutoff, across all businesses, plus
// child rows in FK-safe order: tool_calls → messages → requests → threads.
// There is no archive step: a thread nobody has written to since cutoff is
// expired. A thread that keeps receiving messages is never selected.
func DeleteInactiveOpsAssistantThreads(cutoff time.Time, limit int) (OpsAssistantRetentionResult, error) {
	res := OpsAssistantRetentionResult{}
	if db == nil {
		return res, gorm.ErrInvalidDB
	}
	if limit <= 0 {
		limit = 500
	}

	var ids []uint
	if err := inactiveOpsAssistantThreadIDs(db, cutoff, limit).Pluck("id", &ids).Error; err != nil {
		return res, fmt.Errorf("select inactive ops threads: %w", err)
	}
	if len(ids) == 0 {
		return res, nil
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		r := tx.Where("thread_id IN ?", ids).Delete(&OpsAssistantToolCall{})
		if r.Error != nil {
			return fmt.Errorf("delete ops tool calls: %w", r.Error)
		}
		res.ToolCallsDeleted = r.RowsAffected

		r = tx.Where("thread_id IN ?", ids).Delete(&OpsAssistantMessage{})
		if r.Error != nil {
			return fmt.Errorf("delete ops messages: %w", r.Error)
		}
		res.MessagesDeleted = r.RowsAffected

		r = tx.Where("thread_id IN ?", ids).Delete(&OpsAssistantRequest{})
		if r.Error != nil {
			return fmt.Errorf("delete ops requests: %w", r.Error)
		}
		res.RequestsDeleted = r.RowsAffected

		r = tx.Where("id IN ?", ids).Delete(&OpsAssistantThread{})
		if r.Error != nil {
			return fmt.Errorf("delete ops threads: %w", r.Error)
		}
		res.ThreadsDeleted = r.RowsAffected
		return nil
	})
	return res, err
}

// inactiveOpsAssistantThreadIDs is the janitor's oldest-first selection. It is
// served by idx_ops_assistant_threads_last_message_at (migration 000001), so a
// pass reads `limit` index entries instead of scanning every thread.
func inactiveOpsAssistantThreadIDs(tx *gorm.DB, cutoff time.Time, limit int) *gorm.DB {
	return tx.Model(&OpsAssistantThread{}).
		Where("last_message_at < ?", cutoff.UTC()).
		Order("last_message_at ASC").
		Limit(limit)
}
