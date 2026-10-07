package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// CreateDirectorConsoleThread creates a new persistent director console thread.
func CreateDirectorConsoleThread(businessID uint, title, locale string) (*DirectorConsoleThread, error) {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = "New strategy thread"
	}

	if locale == "" {
		locale = "en"
	}

	now := time.Now()
	thread := DirectorConsoleThread{
		BusinessID:    businessID,
		Title:         cleanTitle,
		Locale:        locale,
		LastMessageAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := db.Create(&thread).Error; err != nil {
		return nil, fmt.Errorf("failed to create director thread: %w", err)
	}

	return &thread, nil
}

// FindReusableDirectorConsoleThread returns the most recent active thread that
// carries the exact same question (title match AND an identical user message),
// or nil when none qualifies. Used by the Ask path (#820) so a verbatim re-ask
// — a client retry after a dropped SSE stream (the client only learns the
// thread_id from the terminal complete event) or a scripted repeat prompt —
// appends to the existing conversation instead of minting one more identical
// sidebar row per attempt. Pinned (seeded) and archived threads are never
// reused, and `since` bounds reuse to a recency window so the same question
// asked on a later service day stays a separate conversation (#730). The
// lookup is a single narrow query: indexed thread columns plus one correlated
// EXISTS on the messages table.
func FindReusableDirectorConsoleThread(businessID uint, title, message string, since time.Time) (*DirectorConsoleThread, error) {
	var thread DirectorConsoleThread
	err := db.
		Where("business_id = ? AND title = ? AND archived_at IS NULL AND pinned = ? AND last_message_at > ?",
			businessID, strings.TrimSpace(title), false, since).
		Where("EXISTS (SELECT 1 FROM director_console_messages m WHERE m.thread_id = director_console_threads.id AND m.business_id = ? AND m.role = ? AND m.content = ?)",
			businessID, DirectorMessageRoleUser, strings.TrimSpace(message)).
		Order("last_message_at DESC").
		First(&thread).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find reusable director thread: %w", err)
	}
	return &thread, nil
}

// LatestDirectorConsoleMessage returns the newest message row for a thread
// (narrow projection: no structured_response blob), or nil when the thread has
// no messages. The Ask retry-reuse path (#820) uses it to detect a trailing
// unanswered user turn so a verbatim retry does not stack a duplicate bubble.
func LatestDirectorConsoleMessage(businessID, threadID uint) (*DirectorConsoleMessage, error) {
	var msg DirectorConsoleMessage
	err := db.Select("id", "thread_id", "business_id", "role", "locale", "content", "created_at").
		Where("business_id = ? AND thread_id = ?", businessID, threadID).
		Order("created_at DESC, id DESC").
		First(&msg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load latest director message: %w", err)
	}
	return &msg, nil
}

// GetDirectorConsoleThreadByID retrieves a thread by ID and business.
func GetDirectorConsoleThreadByID(businessID uint, threadID uint) (*DirectorConsoleThread, error) {
	var thread DirectorConsoleThread
	if err := db.Where("id = ? AND business_id = ?", threadID, businessID).First(&thread).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("director thread not found")
		}
		return nil, fmt.Errorf("failed to get director thread: %w", err)
	}
	return &thread, nil
}

// ListDirectorConsoleThreads returns recent threads for a business ordered by activity.
func ListDirectorConsoleThreads(businessID uint, limit int) ([]DirectorConsoleThread, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	var threads []DirectorConsoleThread
	if err := db.Where("business_id = ? AND archived_at IS NULL", businessID).
		Order("pinned DESC, updated_at desc").
		Limit(limit).
		Find(&threads).Error; err != nil {
		return nil, fmt.Errorf("failed to list director threads: %w", err)
	}

	return threads, nil
}

// ListDirectorConsoleThreadsPaged returns a bounded page of threads for a
// business plus the total count for the requested filter. When archived is
// false it returns active threads (archived_at IS NULL) ordered pinned-first;
// when true it returns archived threads (archived_at IS NOT NULL) ordered by
// most-recently-archived. limit is clamped to [1,200] (default 50); offset is
// floored at 0. The narrow COUNT + Offset/Limit shape keeps the sidebar honest
// without hydrating the whole table.
func ListDirectorConsoleThreadsPaged(businessID uint, archived bool, limit, offset int) ([]DirectorConsoleThread, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	base := db.Model(&DirectorConsoleThread{}).Where("business_id = ?", businessID)
	if archived {
		base = base.Where("archived_at IS NOT NULL")
	} else {
		base = base.Where("archived_at IS NULL")
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count director threads: %w", err)
	}

	order := "pinned DESC, updated_at desc"
	if archived {
		order = "updated_at desc"
	}

	var threads []DirectorConsoleThread
	if err := base.Order(order).Limit(limit).Offset(offset).Find(&threads).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list director threads: %w", err)
	}
	return threads, total, nil
}

// SaveDirectorConsoleMessage stores a thread message and updates thread activity metadata.
func SaveDirectorConsoleMessage(message *DirectorConsoleMessage) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}

	if message.Locale == "" {
		message.Locale = "en"
	}

	now := time.Now()
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	message.UpdatedAt = now

	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}

	if err := tx.Create(message).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to save director message: %w", err)
	}

	if err := tx.Model(&DirectorConsoleThread{}).
		Where("id = ? AND business_id = ?", message.ThreadID, message.BusinessID).
		Updates(map[string]interface{}{
			"last_message_at": now,
			"updated_at":      now,
		}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update thread activity: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit message save: %w", err)
	}

	return nil
}

// ReplaceDirectorConsoleTrailingAssistant swaps the superseded trailing
// assistant rows for a freshly generated one inside a SINGLE transaction, so a
// regenerate can never leave the thread without an answer (L4-15). Callers must
// only reach this once a genuine replacement exists — a failed or canned
// generation has to keep the previous answer instead of destroying it.
//
// Thread activity metadata is stamped exactly like SaveDirectorConsoleMessage.
func ReplaceDirectorConsoleTrailingAssistant(supersededIDs []uint, message *DirectorConsoleMessage) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}

	if message.Locale == "" {
		message.Locale = "en"
	}

	now := time.Now()
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	message.UpdatedAt = now

	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}

	if len(supersededIDs) > 0 {
		if err := tx.Where(
			"id IN ? AND business_id = ? AND thread_id = ?",
			supersededIDs, message.BusinessID, message.ThreadID,
		).Delete(&DirectorConsoleMessage{}).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to delete superseded director messages: %w", err)
		}
	}

	if err := tx.Create(message).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to save director message: %w", err)
	}

	if err := tx.Model(&DirectorConsoleThread{}).
		Where("id = ? AND business_id = ?", message.ThreadID, message.BusinessID).
		Updates(map[string]interface{}{
			"last_message_at": now,
			"updated_at":      now,
		}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update thread activity: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit regenerate replacement: %w", err)
	}

	return nil
}

// ListDirectorConsoleMessages returns the latest bounded messages for a thread
// in chronological order. It projects only fields rendered by the Director
// Console transcript; relations and UpdatedAt are intentionally not hydrated.
func ListDirectorConsoleMessages(businessID uint, threadID uint, limit int) ([]DirectorConsoleMessage, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}

	var messages []DirectorConsoleMessage
	if err := db.Select(
		"id",
		"thread_id",
		"business_id",
		"role",
		"locale",
		"content",
		"structured_response",
		"model_name",
		"latency_ms",
		"feedback_vote",
		"feedback_at",
		"created_at",
	).
		Where("business_id = ? AND thread_id = ?", businessID, threadID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to list director messages: %w", err)
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// ListDirectorConsoleMessageSummaries is the memory-path projection of
// ListDirectorConsoleMessages. It deliberately omits structured_response and
// other blob/metrics columns (model_name, latency_ms, feedback_vote,
// feedback_at) that buildPriorTurns never reads — it only uses ID, Role, and
// Content. Callers that need the full transcript (e.g. ListThreadMessages in
// the director-console handler) must keep using ListDirectorConsoleMessages.
//
// Projection: id, thread_id, business_id, role, locale, content, created_at.
// Order and limit clamping are identical to ListDirectorConsoleMessages.
func ListDirectorConsoleMessageSummaries(businessID uint, threadID uint, limit int) ([]DirectorConsoleMessage, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}

	var messages []DirectorConsoleMessage
	if err := db.Select(
		"id",
		"thread_id",
		"business_id",
		"role",
		"locale",
		"content",
		"created_at",
	).
		Where("business_id = ? AND thread_id = ?", businessID, threadID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to list director message summaries: %w", err)
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// UpdateDirectorConsoleMessageFeedback stores owner feedback for KPI tracking.
func UpdateDirectorConsoleMessageFeedback(businessID uint, messageID uint, vote DirectorFeedbackVote) (*DirectorConsoleMessage, error) {
	now := time.Now()
	var message DirectorConsoleMessage

	if err := db.Where("id = ? AND business_id = ? AND role = ?", messageID, businessID, DirectorMessageRoleAssistant).
		First(&message).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("director assistant message not found")
		}
		return nil, fmt.Errorf("failed to get director message: %w", err)
	}

	message.FeedbackVote = &vote
	message.FeedbackAt = &now
	message.UpdatedAt = now

	if err := db.Model(&message).Updates(map[string]interface{}{
		"feedback_vote": message.FeedbackVote,
		"feedback_at":   message.FeedbackAt,
		"updated_at":    message.UpdatedAt,
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to update director feedback: %w", err)
	}

	return &message, nil
}
