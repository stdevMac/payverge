package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// AiTranscriptDeletionResult reports how many rows one retention pass removed.
type AiTranscriptDeletionResult struct {
	ConversationsDeleted int64
	MessagesDeleted      int64
}

// DeleteExpiredAiWaiterTranscripts deletes up to `limit` AI waiter conversations
// created before `cutoff`, along with their messages. It selects a bounded batch
// of expired conversation IDs first (cheap, rides idx_ai_waiter_conversations_
// created_at), deletes their messages, then the conversations themselves — so a
// single pass never holds a table-wide lock. Callers loop until a pass deletes 0.
func DeleteExpiredAiWaiterTranscripts(cutoff time.Time, limit int) (AiTranscriptDeletionResult, error) {
	res := AiTranscriptDeletionResult{}
	if db == nil {
		return res, gorm.ErrInvalidDB
	}
	if limit <= 0 {
		limit = 500
	}

	var ids []uint
	if err := db.Model(&AiWaiterConversation{}).
		Where("created_at < ?", cutoff.UTC()).
		Order("created_at ASC").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return res, fmt.Errorf("failed to select expired conversations: %w", err)
	}
	if len(ids) == 0 {
		return res, nil
	}

	msgResult := db.Where("conversation_id IN ?", ids).Delete(&AiWaiterMessage{})
	if msgResult.Error != nil {
		return res, fmt.Errorf("failed to delete expired messages: %w", msgResult.Error)
	}
	res.MessagesDeleted = msgResult.RowsAffected

	convResult := db.Where("id IN ?", ids).Delete(&AiWaiterConversation{})
	if convResult.Error != nil {
		return res, fmt.Errorf("failed to delete expired conversations: %w", convResult.Error)
	}
	res.ConversationsDeleted = convResult.RowsAffected

	return res, nil
}
