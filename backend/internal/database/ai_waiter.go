package database

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/stdevmac/payverge/backend/internal/pii"

	"gorm.io/gorm"
)

// SchedulerState tracks idempotency keys for scheduled tasks so that
// jobs that run at fixed hours (e.g., blog ideas at 9 AM) are not
// sent again after a process restart.
type SchedulerState struct {
	ID        uint      `gorm:"primaryKey"`
	Key       string    `gorm:"uniqueIndex;not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// AiWaiterOperatorTestTableCode marks operator sandbox conversations created by
// the dashboard test-chat endpoints. These rows must never participate in guest
// cookie issuance, guest quota, Live Monitor listings, or guest public routes.
const AiWaiterOperatorTestTableCode = "__pv_operator_test__"

// IsAiWaiterOperatorTestTableCode reports whether tableCode is the reserved
// operator sandbox marker.
func IsAiWaiterOperatorTestTableCode(tableCode string) bool {
	return strings.TrimSpace(tableCode) == AiWaiterOperatorTestTableCode
}

// ExcludeAiWaiterOperatorTestConversations scopes a conversation query to
// guest-facing rows only (hides operator sandbox probes from Live Monitor /
// insights / guest quota seeds).
func ExcludeAiWaiterOperatorTestConversations(q *gorm.DB) *gorm.DB {
	return q.Where("table_code <> ?", AiWaiterOperatorTestTableCode)
}

// GetOrCreateAiWaiterConversation retrieves an existing conversation or creates a new one
func GetOrCreateAiWaiterConversation(sessionID string, businessID uint, tableCode, language, mode string) (*AiWaiterConversation, error) {
	var conv AiWaiterConversation
	err := db.Where("session_id = ? AND business_id = ? AND mode = ? AND table_code = ?", sessionID, businessID, mode, tableCode).First(&conv).Error
	if err == nil {
		return &conv, nil
	}

	// Create new if not found
	conv = AiWaiterConversation{
		SessionID:  sessionID,
		BusinessID: businessID,
		TableCode:  tableCode,
		Language:   language,
		Mode:       mode,
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := db.Create(&conv).Error; err != nil {
		return nil, fmt.Errorf("failed to create conversation: %w", err)
	}

	return &conv, nil
}

// GetRecentAiWaiterMessages retrieves the most recent messages for a conversation.
// ToolCalls and StructuredResponse are deliberately not loaded: callers only
// read ID, Role, Content, and CreatedAt, and those payloads can be large.
func GetRecentAiWaiterMessages(convID uint, limit int) ([]AiWaiterMessage, error) {
	var messages []AiWaiterMessage
	if err := db.Select("id", "conversation_id", "role", "content", "created_at").
		Where("conversation_id = ?", convID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get conversation messages: %w", err)
	}
	// Reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

// GetAiWaiterMessagesForGuest returns the same bounded, projected transcript used
// by guest-facing polling.
func GetAiWaiterMessagesForGuest(convID uint, since *time.Time, limit int) ([]AiWaiterMessage, error) {
	return GetAiWaiterMessagesForTranscript(convID, since, limit)
}

// GetAiWaiterMessagesForTranscript intentionally skips ToolCalls because
// transcript UIs only render role/content/timestamp, and tool payloads can grow
// substantially over long conversations.
func GetAiWaiterMessagesForTranscript(convID uint, since *time.Time, limit int) ([]AiWaiterMessage, error) {
	if limit <= 0 {
		limit = 100
	}

	query := db.Select("id", "conversation_id", "role", "content", "structured_response", "author_name", "author_role", "created_at").
		Where("conversation_id = ?", convID)
	if since != nil {
		query = query.Where("created_at > ?", *since).
			Order("created_at ASC, id ASC").
			Limit(limit)
	} else {
		query = query.Order("created_at DESC, id DESC").
			Limit(limit)
	}

	var messages []AiWaiterMessage
	if err := query.Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get guest conversation messages: %w", err)
	}
	if since == nil {
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}
	}
	return messages, nil
}

// SaveAiWaiterMessage saves a single message to a conversation.
func SaveAiWaiterMessage(convID uint, role, content, toolCalls string) error {
	_, _, err := SaveAiWaiterMessageReturningID(convID, role, content, toolCalls)
	return err
}

// SaveAiWaiterMessageReturningID saves a single message and returns its new
// primary key plus its created timestamp. The guest send path uses the returned
// id to tag the optimistically-rendered assistant bubble so the SSE/poll echo
// of the same row dedupes by id instead of fragile content matching (audit E4),
// and uses the timestamp to publish the saved message to the SSE hub without a
// re-query (finding SSE-02).
func SaveAiWaiterMessageReturningID(convID uint, role, content, toolCalls string) (uint, time.Time, error) {
	return SaveAiWaiterMessageReturningIDV2(convID, role, content, toolCalls, "{}")
}

// SaveAiWaiterMessageReturningIDV2 persists the compatibility content and the
// validated structured response atomically with the conversation activity
// update. Blank and JSON null structured values are stored as an empty object.
func SaveAiWaiterMessageReturningIDV2(convID uint, role, content, toolCalls, structured string) (uint, time.Time, error) {
	if db == nil {
		return 0, time.Time{}, gorm.ErrInvalidDB
	}
	if convID == 0 {
		return 0, time.Time{}, fmt.Errorf("conversation id is required")
	}
	normalizedStructured, err := NormalizeAiWaiterStructuredResponse(structured)
	if err != nil {
		return 0, time.Time{}, err
	}
	now := time.Now()
	msg := AiWaiterMessage{
		ConversationID:     convID,
		Role:               role,
		Content:            pii.Redact(content),
		ToolCalls:          toolCalls,
		StructuredResponse: normalizedStructured,
		CreatedAt:          now,
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&msg).Error; err != nil {
			return err
		}
		updates := map[string]any{"updated_at": now}
		if toolCalls != "" && strings.Contains(toolCalls, "add_to_cart") {
			updates["cart_items_added"] = gorm.Expr("cart_items_added + ?", 1)
		}
		result := tx.Model(&AiWaiterConversation{}).Where("id = ?", convID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("ai waiter conversation %d: %w", convID, gorm.ErrRecordNotFound)
		}
		return nil
	})
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to save ai message: %w", err)
	}

	return msg.ID, msg.CreatedAt, nil
}

// NormalizeAiWaiterStructuredResponse validates and PII-redacts a structured
// assistant payload without writing it. Callers use this before deriving wire
// or event compatibility projections so every representation matches the
// storage boundary exactly.
func NormalizeAiWaiterStructuredResponse(structured string) (string, error) {
	return normalizeAiWaiterStructuredResponse(structured)
}

func normalizeAiWaiterStructuredResponse(structured string) (string, error) {
	trimmed := strings.TrimSpace(structured)
	if trimmed == "" || trimmed == "null" {
		return "{}", nil
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(trimmed)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("invalid structured response JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return "", fmt.Errorf("invalid structured response JSON: %w", err)
	}
	redacted, changed, err := redactAiWaiterStructuredJSON(value, "")
	if err != nil {
		return "", err
	}
	if !changed {
		return structured, nil
	}
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return "", fmt.Errorf("encode redacted structured response: %w", err)
	}
	return string(encoded), nil
}

func redactAiWaiterStructuredJSON(value any, fieldName string) (any, bool, error) {
	switch typed := value.(type) {
	case string:
		redacted := redactAiWaiterStructuredString(fieldName, typed)
		return redacted, redacted != typed, nil
	case json.Number:
		redacted := redactAiWaiterStructuredString(fieldName, typed.String())
		if redacted != typed.String() {
			return redacted, true, nil
		}
		return typed, false, nil
	case []any:
		changed := false
		for index, item := range typed {
			redacted, itemChanged, err := redactAiWaiterStructuredJSON(item, fieldName)
			if err != nil {
				return nil, false, err
			}
			typed[index] = redacted
			changed = changed || itemChanged
		}
		return typed, changed, nil
	case map[string]any:
		changed := false
		redactedMap := make(map[string]any, len(typed))
		phoneTaggedValue := aiWaiterStructuredHasPhoneTag(typed)
		for key, item := range typed {
			redactedKey := pii.Redact(key)
			if _, exists := redactedMap[redactedKey]; exists {
				return nil, false, fmt.Errorf("redacted structured response key collision at %q", redactedKey)
			}
			fieldContext := key
			if phoneTaggedValue && normalizeAiWaiterStructuredFieldContext(key) == "value" {
				fieldContext = "phone"
			}
			redacted, itemChanged, err := redactAiWaiterStructuredJSON(item, fieldContext)
			if err != nil {
				return nil, false, err
			}
			redactedMap[redactedKey] = redacted
			changed = changed || redactedKey != key || itemChanged
		}
		return redactedMap, changed, nil
	default:
		return value, false, nil
	}
}

func redactAiWaiterStructuredString(fieldName, value string) string {
	redacted := pii.Redact(value)
	if redacted != value || fieldName == "" {
		return redacted
	}
	prefix := aiWaiterStructuredRedactionContext(fieldName) + ": "
	redactedPrefix := pii.Redact(prefix)
	contextual := pii.Redact(prefix + value)
	if contextual == redactedPrefix+value {
		return value
	}
	if strings.HasPrefix(contextual, redactedPrefix) {
		return strings.TrimPrefix(contextual, redactedPrefix)
	}
	return "[redacted-phone]"
}

func aiWaiterStructuredRedactionContext(fieldName string) string {
	normalized := normalizeAiWaiterStructuredFieldContext(fieldName)
	for _, token := range strings.Fields(normalized) {
		switch token {
		case "phone", "tel", "telefono", "teléfono", "mobile", "movil", "móvil", "cell", "celular", "whatsapp":
			return token
		}
	}
	return normalized
}

func aiWaiterStructuredHasPhoneTag(value map[string]any) bool {
	for key, rawTag := range value {
		normalizedKey := normalizeAiWaiterStructuredFieldContext(key)
		if normalizedKey != "type" && normalizedKey != "kind" {
			continue
		}
		tag, ok := rawTag.(string)
		if !ok {
			continue
		}
		switch strings.ReplaceAll(normalizeAiWaiterStructuredFieldContext(tag), " ", "") {
		case "phone", "telephone", "tel", "telefono", "teléfono", "mobile", "movil", "móvil", "cell", "celular", "whatsapp":
			return true
		}
	}
	return false
}

func normalizeAiWaiterStructuredFieldContext(fieldName string) string {
	runes := []rune(strings.TrimSpace(fieldName))
	var normalized strings.Builder
	writeSpace := func() {
		if normalized.Len() > 0 {
			normalized.WriteByte(' ')
		}
	}
	for index, current := range runes {
		if !unicode.IsLetter(current) && !unicode.IsDigit(current) {
			writeSpace()
			continue
		}
		if unicode.IsUpper(current) && index > 0 {
			previous := runes[index-1]
			nextIsLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
			if unicode.IsLower(previous) || unicode.IsDigit(previous) || (unicode.IsUpper(previous) && nextIsLower) {
				writeSpace()
			}
		}
		normalized.WriteRune(unicode.ToLower(current))
	}

	tokens := strings.Fields(normalized.String())
	canonical := make([]string, 0, len(tokens))
	for index := 0; index < len(tokens); index++ {
		if tokens[index] == "whats" && index+1 < len(tokens) && tokens[index+1] == "app" {
			canonical = append(canonical, "whatsapp")
			index++
			continue
		}
		if tokens[index] == "telephone" {
			canonical = append(canonical, "phone")
			continue
		}
		canonical = append(canonical, tokens[index])
	}
	return strings.Join(canonical, " ")
}

// SaveAiWaiterStaffReply saves a human takeover reply (role=assistant) stamped
// with the acting staff's identity for the operator transcript audit trail.
func SaveAiWaiterStaffReply(convID uint, content string, authorStaffID *uint, authorName, authorRole string) (uint, time.Time, error) {
	msg := AiWaiterMessage{
		ConversationID: convID,
		Role:           "assistant",
		Content:        pii.Redact(content),
		AuthorStaffID:  authorStaffID,
		AuthorName:     authorName,
		AuthorRole:     authorRole,
		CreatedAt:      time.Now(),
	}
	if err := db.Create(&msg).Error; err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to save ai staff reply: %w", err)
	}
	db.Model(&AiWaiterConversation{}).Where("id = ?", convID).Update("updated_at", time.Now())
	return msg.ID, msg.CreatedAt, nil
}

// CountAiWaiterMessages returns the number of stored messages for a conversation.
func CountAiWaiterMessages(convID uint) int64 {
	var n int64
	db.Model(&AiWaiterMessage{}).Where("conversation_id = ?", convID).Count(&n)
	return n
}

// FindAiWaiterConversation returns the conversation for an issued session token
// scoped to business/mode/table, or (nil, false) if no such conversation exists.
func FindAiWaiterConversation(sessionID string, businessID uint, mode, tableCode string) (*AiWaiterConversation, bool) {
	var conv AiWaiterConversation
	err := db.Where("session_id = ? AND business_id = ? AND mode = ? AND table_code = ?", sessionID, businessID, mode, tableCode).First(&conv).Error
	if err != nil {
		return nil, false
	}
	return &conv, true
}

// CountAiWaiterMessagesSince counts a business's AI waiter messages created
// after `since` (joined via conversation). Uses the conversation business_id
// index + messages created_at index. Operator sandbox
// conversations are excluded so dashboard test-chat cannot burn guest quota.
func CountAiWaiterMessagesSince(businessID uint, since time.Time) int64 {
	var n int64
	db.Model(&AiWaiterMessage{}).
		Joins("JOIN ai_waiter_conversations c ON c.id = ai_waiter_messages.conversation_id").
		Where("c.business_id = ? AND c.table_code <> ? AND ai_waiter_messages.created_at >= ?",
			businessID, AiWaiterOperatorTestTableCode, since).
		Count(&n)
	return n
}

// FindAiWaiterConversationBySession returns the conversation for a session token
// scoped to a business, regardless of table/mode. Used to archive a predecessor
// when the guest starts a replacement session.
func FindAiWaiterConversationBySession(sessionID string, businessID uint) (*AiWaiterConversation, bool) {
	if db == nil || strings.TrimSpace(sessionID) == "" || businessID == 0 {
		return nil, false
	}
	var conv AiWaiterConversation
	err := db.Where("session_id = ? AND business_id = ?", strings.TrimSpace(sessionID), businessID).First(&conv).Error
	if err != nil {
		return nil, false
	}
	return &conv, true
}

// CloseAiWaiterConversationBySession archives a predecessor guest conversation
// so a reset or locale switch does not leave an abandoned active Live Monitor
// row. Operator sandbox rows are ignored. A non-empty transition is stored as
// a system message so owners can see the handoff.
func CloseAiWaiterConversationBySession(sessionID string, businessID uint, transition string) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	conv, ok := FindAiWaiterConversationBySession(sessionID, businessID)
	if !ok {
		return false, nil
	}
	if IsAiWaiterOperatorTestTableCode(conv.TableCode) || conv.Status == "closed" {
		return false, nil
	}
	if msg := strings.TrimSpace(transition); msg != "" {
		if err := SaveAiWaiterMessage(conv.ID, "system", msg, ""); err != nil {
			return false, err
		}
	}
	result := db.Model(&AiWaiterConversation{}).
		Where("id = ? AND status <> ?", conv.ID, "closed").
		Updates(map[string]interface{}{
			"status":              "closed",
			"claimed_by_staff_id": nil,
			"claimed_by_name":     "",
			"claimed_by_role":     "",
			"claimed_at":          nil,
			"is_paused":           false,
			"updated_at":          time.Now().UTC(),
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// UpdateAiWaiterConversationLanguage persists a retained guest locale for the
// conversation (WhatsApp multi-turn language continuity).
func UpdateAiWaiterConversationLanguage(convID uint, locale string) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	locale = strings.TrimSpace(locale)
	if convID == 0 || locale == "" {
		return fmt.Errorf("conversation id and locale are required")
	}
	return db.Model(&AiWaiterConversation{}).
		Where("id = ?", convID).
		Updates(map[string]interface{}{
			"language":   locale,
			"updated_at": time.Now().UTC(),
		}).Error
}
