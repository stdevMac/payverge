package database

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// OpsAssistantThread stores operator help conversations.
type OpsAssistantThread struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	BusinessID    uint      `gorm:"index;not null" json:"business_id"`
	Title         string    `gorm:"size:200" json:"title"`
	Locale        string    `gorm:"size:16" json:"locale"`
	LastMessageAt time.Time `json:"last_message_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (OpsAssistantThread) TableName() string { return "ops_assistant_threads" }

type OpsAssistantMessageRole string

const (
	OpsAssistantRoleUser      OpsAssistantMessageRole = "user"
	OpsAssistantRoleAssistant OpsAssistantMessageRole = "assistant"
)

// OpsAssistantRequest is the durable claim/ledger row for a client request id.
type OpsAssistantRequest struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	BusinessID         uint       `gorm:"not null;uniqueIndex:uq_ops_req_biz_client" json:"business_id"`
	ThreadID           *uint      `json:"thread_id,omitempty"`
	ClientRequestID    string     `gorm:"size:64;not null;uniqueIndex:uq_ops_req_biz_client" json:"client_request_id"`
	Status             string     `gorm:"size:24;not null;default:'pending'" json:"status"`
	ClaimToken         string     `gorm:"size:64" json:"-"`
	ClaimedAt          *time.Time `json:"claimed_at,omitempty"`
	ErrorCode          string     `gorm:"size:64" json:"error_code,omitempty"`
	UserMessageID      *uint      `json:"user_message_id,omitempty"`
	AssistantMessageID *uint      `json:"assistant_message_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (OpsAssistantRequest) TableName() string { return "ops_assistant_requests" }

// OpsAssistantMessage is one turn in an ops assistant thread.
type OpsAssistantMessage struct {
	ID                 uint                    `gorm:"primaryKey" json:"id"`
	ThreadID           uint                    `gorm:"index;not null" json:"thread_id"`
	BusinessID         uint                    `gorm:"index;not null" json:"business_id"`
	RequestID          *uint                   `json:"request_id,omitempty"`
	Role               OpsAssistantMessageRole `gorm:"size:16;not null" json:"role"`
	Locale             string                  `gorm:"size:16" json:"locale"`
	Content            string                  `gorm:"type:text" json:"content"`
	StructuredResponse string                  `gorm:"type:jsonb" json:"structured_response"`
	ModelName          string                  `gorm:"size:64" json:"model_name"`
	LatencyMs          int64                   `json:"latency_ms"`
	// Feedback is an operator thumbs up/down on an assistant reply: "up"|"down"|"".
	Feedback  string    `gorm:"size:8" json:"feedback"`
	CreatedAt time.Time `json:"created_at"`
}

func (OpsAssistantMessage) TableName() string { return "ops_assistant_messages" }

// ErrOpsAssistantMessageNotFound is returned when a feedback update matches no
// row — the message id is unknown or belongs to another business/thread. The
// business_id predicate keeps cross-tenant writes impossible; reporting 0 rows
// lets the handler answer 404 instead of a misleading 200.
var ErrOpsAssistantMessageNotFound = errors.New("ops assistant message not found")

// SetOpsAssistantMessageFeedback records operator feedback ("up"/"down") on an
// assistant message, scoped to the owning business. It returns
// ErrOpsAssistantMessageNotFound when no row matched (wrong id or another
// business's message), so the caller never reports success for a no-op update.
func SetOpsAssistantMessageFeedback(businessID, messageID uint, feedback string) error {
	res := GetDB().Model(&OpsAssistantMessage{}).
		Where("id = ? AND business_id = ?", messageID, businessID).
		Update("feedback", feedback)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrOpsAssistantMessageNotFound
	}
	return nil
}

// OpsAssistantToolCall records tool invocations during an ops ask.
type OpsAssistantToolCall struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	BusinessID    uint      `gorm:"index;not null" json:"business_id"`
	ThreadID      uint      `gorm:"index;not null" json:"thread_id"`
	MessageID     *uint     `json:"message_id,omitempty"`
	ToolName      string    `gorm:"size:64;not null" json:"tool_name"`
	ArgumentsJSON string    `gorm:"type:jsonb" json:"arguments"`
	ResultSummary string    `gorm:"type:text" json:"result_summary"`
	Status        string    `gorm:"size:16" json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

func (OpsAssistantToolCall) TableName() string { return "ops_assistant_tool_calls" }

// EscalationSource identifies which surface raised an escalation.
type EscalationSource string

const (
	EscalationSourceOps EscalationSource = "ops"
)

// Escalation persists a support/sales escalation raised by a bot surface so the
// owner has a durable record even if Telegram/email delivery fails.
type Escalation struct {
	ID                uint             `gorm:"primaryKey" json:"id"`
	Source            EscalationSource `gorm:"size:16;not null;index" json:"source"`
	BusinessID        *uint            `gorm:"index" json:"business_id,omitempty"`
	SessionRef        string           `gorm:"size:80" json:"session_ref"`
	Issue             string           `gorm:"type:text" json:"issue"`
	TranscriptSummary string           `gorm:"type:text" json:"transcript_summary"`
	ContactEmail      string           `gorm:"size:320" json:"contact_email"`
	Status            string           `gorm:"size:16;not null;default:'open';index" json:"status"`
	AdminNotes        string           `gorm:"type:text" json:"admin_notes"`
	CreatedAt         time.Time        `gorm:"index" json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

func (Escalation) TableName() string { return "escalations" }

func CreateEscalation(e *Escalation) error {
	if e.Status == "" {
		e.Status = "open"
	}
	now := time.Now()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = now
	}
	return GetDB().Create(e).Error
}

// ListEscalations returns escalations newest-first, paginated.
func ListEscalations(limit, offset int, statuses ...string) ([]Escalation, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	statusFilters := normalizeStatusFilters(statuses...)
	var rows []Escalation
	var total int64
	countQuery := GetDB().Model(&Escalation{})
	if len(statusFilters) > 0 {
		countQuery = countQuery.Where("status IN ?", statusFilters)
	}
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	listQuery := GetDB()
	if len(statusFilters) > 0 {
		listQuery = listQuery.Where("status IN ?", statusFilters)
	}
	err := listQuery.Order("created_at DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func normalizeStatusFilters(statuses ...string) []string {
	filters := make([]string, 0, len(statuses))
	for _, status := range statuses {
		for _, part := range strings.Split(status, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				filters = append(filters, trimmed)
			}
		}
	}
	return filters
}

// GetEscalationByID returns one escalation row for admin detail/update flows.
func GetEscalationByID(id uint) (*Escalation, error) {
	var row Escalation
	err := GetDB().First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

var validEscalationStatuses = map[string]struct{}{
	"open": {}, "in_progress": {}, "resolved": {}, "closed": {},
}

// UpdateEscalationAdmin patches status and/or admin notes on an escalation.
func UpdateEscalationAdmin(id uint, status, notes *string) (*Escalation, error) {
	updates := map[string]any{"updated_at": time.Now()}
	if status != nil {
		if _, ok := validEscalationStatuses[*status]; !ok {
			return nil, ErrInvalidEscalationStatus
		}
		updates["status"] = *status
	}
	if notes != nil {
		updates["admin_notes"] = *notes
	}
	if len(updates) == 1 {
		return GetEscalationByID(id)
	}
	res := GetDB().Model(&Escalation{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	return GetEscalationByID(id)
}

// ErrInvalidEscalationStatus is returned when an admin passes an unknown status.
var ErrInvalidEscalationStatus = errors.New("invalid escalation status")

func CreateOpsAssistantThread(businessID uint, title, locale string) (*OpsAssistantThread, error) {
	now := time.Now()
	thread := &OpsAssistantThread{
		BusinessID: businessID, Title: title, Locale: locale,
		LastMessageAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := GetDB().Create(thread).Error; err != nil {
		return nil, err
	}
	return thread, nil
}

func GetOpsAssistantThreadByID(businessID, threadID uint) (*OpsAssistantThread, error) {
	var thread OpsAssistantThread
	err := GetDB().Where("id = ? AND business_id = ?", threadID, businessID).First(&thread).Error
	if err != nil {
		return nil, err
	}
	return &thread, nil
}

func SaveOpsAssistantMessage(msg *OpsAssistantMessage) error {
	// structured_response is a Postgres jsonb column: the empty string is NOT
	// valid json and makes the INSERT fail (user messages carry no structured
	// payload). SQLite tests don't catch this — jsonb degrades to TEXT there.
	// Normalize "null" too (a nil-pointer marshal), matching
	// SaveOpsAssistantToolCall so no caller can reintroduce the jsonb 500.
	if s := strings.TrimSpace(msg.StructuredResponse); s == "" || s == "null" {
		msg.StructuredResponse = "{}"
	}
	if err := GetDB().Create(msg).Error; err != nil {
		return err
	}
	_ = GetDB().Model(&OpsAssistantThread{}).
		Where("id = ? AND business_id = ?", msg.ThreadID, msg.BusinessID).
		Updates(map[string]any{"last_message_at": time.Now(), "updated_at": time.Now()}).Error
	return nil
}

// ListOpsAssistantMessages returns the most recent `limit` messages for a thread
// in chronological (oldest-first) order. Fetching the newest N and then reversing
// ensures long threads keep their recent context — both the LLM prior-turn feed
// and the HTTP thread listing want the latest messages, displayed chronologically.
func ListOpsAssistantMessages(businessID, threadID uint, limit int) ([]OpsAssistantMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	var msgs []OpsAssistantMessage
	err := GetDB().Where("business_id = ? AND thread_id = ?", businessID, threadID).
		Order("id DESC").Limit(limit).Find(&msgs).Error
	if err != nil {
		return nil, err
	}
	// Reverse in place to return chronological (oldest-first) order.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func SaveOpsAssistantToolCall(rec *OpsAssistantToolCall) error {
	// arguments is jsonb — same empty-string trap as structured_response.
	if s := strings.TrimSpace(rec.ArgumentsJSON); s == "" || s == "null" {
		rec.ArgumentsJSON = "{}"
	}
	return GetDB().Create(rec).Error
}

func AttachMessageIDToOpsToolCalls(ids []uint, messageID uint) error {
	if len(ids) == 0 {
		return nil
	}
	return GetDB().Model(&OpsAssistantToolCall{}).Where("id IN ?", ids).Update("message_id", messageID).Error
}
