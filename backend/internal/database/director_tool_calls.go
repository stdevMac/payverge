package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// DirectorToolCall records a single tool-call invocation from the
// Director Console — args, summary, latency, success. Used for post-launch
// analytics on tool value and for support debugging ("show me what Sage
// looked at when this ticket was filed").
//
// Note on ArgsJSON: in Postgres this column is provisioned as `jsonb` by the
// versioned SQL migration. The GORM tag deliberately omits a column
// type so the model also works with SQLite in tests; GORM's AutoMigrate will
// not override an existing column type, so production stays jsonb.
type DirectorToolCall struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	ThreadID   uint           `gorm:"index;not null" json:"thread_id"`
	MessageID  *uint          `gorm:"index" json:"message_id,omitempty"`
	BusinessID uint           `gorm:"index;not null" json:"business_id"`
	ToolName   string         `gorm:"size:128;not null;index" json:"tool_name"`
	ArgsJSON   string         `gorm:"not null;default:'{}'" json:"args_json"`
	Summary    string         `gorm:"not null;default:''" json:"summary"`
	DurationMs int            `gorm:"not null;default:0" json:"duration_ms"`
	Success    bool           `gorm:"not null;default:true" json:"success"`
	Error      string         `json:"error,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName pins the table name for GORM.
func (DirectorToolCall) TableName() string { return "director_tool_calls" }

// SaveDirectorToolCall inserts a log row.
func SaveDirectorToolCall(call *DirectorToolCall) error {
	if call == nil {
		return fmt.Errorf("nil tool call")
	}
	if call.CreatedAt.IsZero() {
		call.CreatedAt = time.Now()
	}
	if err := db.Create(call).Error; err != nil {
		return fmt.Errorf("failed to save director tool call: %w", err)
	}
	return nil
}

// AttachMessageIDToToolCalls back-fills the message_id for a batch of tool
// calls — used after the assistant message is persisted.
func AttachMessageIDToToolCalls(ids []uint, messageID uint) error {
	if len(ids) == 0 {
		return nil
	}
	return db.Model(&DirectorToolCall{}).
		Where("id IN ?", ids).
		Update("message_id", messageID).Error
}
