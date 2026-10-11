package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MenuWizardSessionStatus represents the status of a wizard session
type MenuWizardSessionStatus string

const (
	WizardStatusInProgress MenuWizardSessionStatus = "in_progress"
	WizardStatusCompleted  MenuWizardSessionStatus = "completed"
	WizardStatusAbandoned  MenuWizardSessionStatus = "abandoned"
)

// MenuExtractionStatus represents the status of an extraction job
type MenuExtractionStatus string

const (
	ExtractionStatusPending    MenuExtractionStatus = "pending"
	ExtractionStatusUploading  MenuExtractionStatus = "uploading"
	ExtractionStatusProcessing MenuExtractionStatus = "processing"
	ExtractionStatusCompleted  MenuExtractionStatus = "completed"
	ExtractionStatusFailed     MenuExtractionStatus = "failed"
)

// MenuWizardSession stores a menu generation wizard session
type MenuWizardSession struct {
	ID            uint                    `gorm:"primaryKey" json:"id"`
	BusinessID    uint                    `gorm:"index;not null" json:"business_id"`
	Status        MenuWizardSessionStatus `gorm:"default:'in_progress'" json:"status"`
	GeneratedMenu string                  `gorm:"type:text" json:"generated_menu"`  // JSON of generated menu
	Config        string                  `gorm:"type:text" json:"config"`          // JSON of extracted config (business type, cuisine, etc.)
	Language      string                  `gorm:"type:varchar(16)" json:"language"` // resolved prompt locale for output-language fidelity
	CreatedAt     time.Time               `json:"created_at"`
	UpdatedAt     time.Time               `json:"updated_at"`

	// Relationships
	Business Business            `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
	Messages []MenuWizardMessage `gorm:"foreignKey:SessionID" json:"messages,omitempty"`
}

// MenuWizardMessage stores a single message in the wizard conversation
type MenuWizardMessage struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SessionID uint      `gorm:"index;index:idx_menu_wizard_messages_session_history,priority:1;not null" json:"session_id"`
	Role      string    `gorm:"not null" json:"role"` // "user", "assistant", "system"
	Content   string    `gorm:"type:text;not null" json:"content"`
	CreatedAt time.Time `gorm:"index:idx_menu_wizard_messages_session_history,priority:2,sort:desc" json:"created_at"`

	// Relationships
	Session MenuWizardSession `gorm:"foreignKey:SessionID" json:"session,omitempty"`
}

// MenuExtractionJob stores the state of a PDF/image menu extraction job
type MenuExtractionJob struct {
	ID            uint                 `gorm:"primaryKey" json:"id"`
	BusinessID    uint                 `gorm:"index;not null" json:"business_id"`
	Status        MenuExtractionStatus `gorm:"default:'pending'" json:"status"`
	ErrorMessage  string               `gorm:"type:text" json:"error_message"`
	ExtractedMenu string               `gorm:"type:text" json:"extracted_menu"` // JSON of extracted menu
	ImageCount    int                  `json:"image_count"`                     // Total expected images
	// Durable worker claim fields. ClaimToken is never exposed
	// on the wire; it gates complete/fail transitions so only the owning worker
	// can finalize a job after a restart or concurrent claim race.
	ClaimedAt     *time.Time `json:"claimed_at,omitempty"`
	ClaimToken    string     `gorm:"size:64" json:"-"`
	AttemptCount  int64      `json:"attempt_count"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// Relationships
	Business Business              `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
	Images   []MenuExtractionImage `gorm:"foreignKey:JobID" json:"images,omitempty"`
}

// MenuExtractionImage stores an individual menu page image for extraction
type MenuExtractionImage struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	JobID      uint      `gorm:"index;not null" json:"job_id"`
	FilePath   string    `gorm:"type:text;not null" json:"file_path"` // Legacy local path; new rows use StorageKey
	PageOrder  int       `json:"page_order"`                          // Order of the page
	MIMEType   string    `gorm:"size:32;not null;default:'image/jpeg'" json:"mime_type"`
	StorageKey string    `gorm:"type:text" json:"-"` // Protected-object key for restart-safe reads
	CreatedAt  time.Time `json:"created_at"`

	// Relationships
	Job MenuExtractionJob `gorm:"foreignKey:JobID" json:"job,omitempty"`
}

// TableName methods
func (MenuWizardSession) TableName() string {
	return "menu_wizard_sessions"
}

func (MenuWizardMessage) TableName() string {
	return "menu_wizard_messages"
}

func (MenuExtractionJob) TableName() string {
	return "menu_extraction_jobs"
}

func (MenuExtractionImage) TableName() string {
	return "menu_extraction_images"
}

// --- CRUD Operations ---

// CreateWizardSession creates a new wizard session
func CreateWizardSession(session *MenuWizardSession) error {
	return db.Create(session).Error
}

// GetWizardSessionByID retrieves a session by ID
func GetWizardSessionByID(id uint) (*MenuWizardSession, error) {
	var session MenuWizardSession
	if err := db.First(&session, id).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

// UpdateWizardSession updates a session
func UpdateWizardSession(session *MenuWizardSession) error {
	session.UpdatedAt = time.Now()
	return db.Omit(clause.Associations).Save(session).Error
}

// AddWizardMessage adds a message to a session
func AddWizardMessage(message *MenuWizardMessage) error {
	return db.Create(message).Error
}

// GetWizardMessagesForClient returns the latest bounded wizard messages in
// chronological order, projected to the fields rendered by the UI transcript.
func GetWizardMessagesForClient(sessionID uint, limit int) ([]MenuWizardMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}

	var messages []MenuWizardMessage
	if err := db.Select("role", "content", "created_at").
		Where("session_id = ?", sessionID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

// GetRecentWizardMessages returns the session's system prompt (if any) plus the
// most recent `limit` non-system messages, in chronological order, projected to
// role+content+created_at. Bounds the LLM context to O(limit) DB bytes / tokens
// per turn instead of O(total history). limit<=0 defaults to 20, capped at 100.
func GetRecentWizardMessages(sessionID uint, limit int) ([]MenuWizardMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var system []MenuWizardMessage
	if err := db.Select("role", "content", "created_at").
		Where("session_id = ? AND role = ?", sessionID, "system").
		Order("created_at ASC").Limit(1).
		Find(&system).Error; err != nil {
		return nil, err
	}

	var recent []MenuWizardMessage
	if err := db.Select("role", "content", "created_at").
		Where("session_id = ? AND role <> ?", sessionID, "system").
		Order("created_at DESC, id DESC").Limit(limit).
		Find(&recent).Error; err != nil {
		return nil, err
	}
	// Reverse to restore chronological (oldest-first) order for the LLM prompt.
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	return append(system, recent...), nil
}

// CreateExtractionJob creates a new extraction job
func CreateExtractionJob(job *MenuExtractionJob) error {
	return db.Create(job).Error
}

// GetExtractionJobByID retrieves an extraction job by ID
func GetExtractionJobByID(id uint) (*MenuExtractionJob, error) {
	var job MenuExtractionJob
	// Use new session to ensure fresh read (no caching)
	if err := db.Session(&gorm.Session{}).Preload("Images", func(db *gorm.DB) *gorm.DB {
		return db.Order("page_order ASC")
	}).First(&job, id).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// UpdateExtractionJob updates an extraction job
func UpdateExtractionJob(job *MenuExtractionJob) error {
	job.UpdatedAt = time.Now()
	return db.Omit(clause.Associations).Save(job).Error
}

// AddExtractionImage adds an image to an extraction job
func AddExtractionImage(image *MenuExtractionImage) error {
	return db.Create(image).Error
}

// MenuExtractionClaim is the result of an atomic claim attempt.
type MenuExtractionClaim struct {
	Job     MenuExtractionJob
	Claimed bool
}

// DefaultMenuExtractionClaimLease is how long a processing claim is exclusive
// before a restart/stale worker may re-claim the row.
const DefaultMenuExtractionClaimLease = 10 * time.Minute

// ClaimMenuExtractionJob atomically claims a job for processing. Exactly one
// concurrent caller receives Claimed=true. Fresh processing/completed rows are
// returned without a claim. Stale processing (claimed_at older than lease) is
// re-claimable and increments attempt_count.
func ClaimMenuExtractionJob(id uint, claimToken string, now time.Time, lease time.Duration) (MenuExtractionClaim, error) {
	if db == nil {
		return MenuExtractionClaim{}, gorm.ErrInvalidDB
	}
	if claimToken == "" {
		return MenuExtractionClaim{}, fmt.Errorf("claim token is required")
	}
	if lease <= 0 {
		lease = DefaultMenuExtractionClaimLease
	}
	staleBefore := now.Add(-lease)

	// Conditional update is the claim arbiter — never use Save for transitions.
	result := db.Model(&MenuExtractionJob{}).
		Where("id = ?", id).
		Where(
			"(status IN ? OR (status = ? AND (claimed_at IS NULL OR claimed_at < ?)))",
			[]MenuExtractionStatus{
				ExtractionStatusPending,
				ExtractionStatusUploading,
				ExtractionStatusFailed,
			},
			ExtractionStatusProcessing,
			staleBefore,
		).
		Updates(map[string]interface{}{
			"status":          ExtractionStatusProcessing,
			"claimed_at":      now,
			"claim_token":     claimToken,
			"attempt_count":   gorm.Expr("attempt_count + ?", 1),
			"next_attempt_at": nil,
			"updated_at":      now,
			"error_message":   "",
		})
	if result.Error != nil {
		return MenuExtractionClaim{}, result.Error
	}

	job, err := GetExtractionJobByID(id)
	if err != nil {
		return MenuExtractionClaim{}, err
	}
	if result.RowsAffected == 1 {
		return MenuExtractionClaim{Job: *job, Claimed: true}, nil
	}
	return MenuExtractionClaim{Job: *job, Claimed: false}, nil
}

// CompleteMenuExtractionJob marks a job completed only when claim_token still matches.
func CompleteMenuExtractionJob(id uint, claimToken, extractedMenu string, now time.Time) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	result := db.Model(&MenuExtractionJob{}).
		Where("id = ? AND claim_token = ? AND status = ?", id, claimToken, ExtractionStatusProcessing).
		Updates(map[string]interface{}{
			"status":          ExtractionStatusCompleted,
			"extracted_menu":  extractedMenu,
			"error_message":   "",
			"next_attempt_at": nil,
			"updated_at":      now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// FailMenuExtractionJob marks a job failed only when claim_token still matches.
// publicError is a stable safe code; nextAttempt schedules retry when non-nil.
func FailMenuExtractionJob(id uint, claimToken, publicError string, nextAttempt *time.Time, now time.Time) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	result := db.Model(&MenuExtractionJob{}).
		Where("id = ? AND claim_token = ? AND status = ?", id, claimToken, ExtractionStatusProcessing).
		Updates(map[string]interface{}{
			"status":          ExtractionStatusFailed,
			"error_message":   publicError,
			"next_attempt_at": nextAttempt,
			"updated_at":      now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ReleaseMenuExtractionJob hands a claimed job back as pending and refunds the
// attempt the claim spent, only while claimToken still owns it. The worker
// calls it when the process shuts down mid-job, which is not the job's fault.
func ReleaseMenuExtractionJob(id uint, claimToken string, now time.Time) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	result := db.Model(&MenuExtractionJob{}).
		Where("id = ? AND claim_token = ? AND status = ?", id, claimToken, ExtractionStatusProcessing).
		Updates(map[string]interface{}{
			"status":          ExtractionStatusPending,
			"claim_token":     "",
			"claimed_at":      nil,
			"attempt_count":   gorm.Expr("CASE WHEN attempt_count > 0 THEN attempt_count - 1 ELSE 0 END"),
			"next_attempt_at": nil,
			"updated_at":      now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ListMenuExtractionJobsNeedingWork returns job IDs that should be enqueued at
// startup or by a recovery sweep: pending/uploading, failed with due retry, or
// stale processing. Bounded by limit.
func ListMenuExtractionJobsNeedingWork(now time.Time, lease time.Duration, limit int) ([]uint, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if lease <= 0 {
		lease = DefaultMenuExtractionClaimLease
	}
	if limit <= 0 {
		limit = 100
	}
	staleBefore := now.Add(-lease)
	var ids []uint
	err := db.Model(&MenuExtractionJob{}).
		Where(
			`status IN ? OR (status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)) OR (status = ? AND (claimed_at IS NULL OR claimed_at < ?))`,
			[]MenuExtractionStatus{ExtractionStatusPending, ExtractionStatusUploading},
			ExtractionStatusFailed,
			now,
			ExtractionStatusProcessing,
			staleBefore,
		).
		Order("id ASC").
		Limit(limit).
		Pluck("id", &ids).Error
	return ids, err
}
