package database

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// DirectorProposalStatus is the lifecycle state of a proposed action.
type DirectorProposalStatus string

const (
	DirectorProposalPending   DirectorProposalStatus = "pending"
	DirectorProposalApplied   DirectorProposalStatus = "applied"
	DirectorProposalDismissed DirectorProposalStatus = "dismissed"
	DirectorProposalExpired   DirectorProposalStatus = "expired"
)

// DirectorProposedAction is a staged, not-yet-applied menu change the director
// previewed for the operator. It is NEVER a menu write — apply (a separate
// human-initiated endpoint) recomputes from ParamsJSON against the
// version-matched menu and commits.
type DirectorProposedAction struct {
	ID          uint                   `gorm:"primaryKey" json:"id"`
	PublicID    string                 `gorm:"size:40;uniqueIndex;not null" json:"public_id"`
	BusinessID  uint                   `gorm:"index;not null" json:"business_id"`
	ThreadID    uint                   `gorm:"index;not null" json:"thread_id"`
	MessageID   *uint                  `gorm:"index" json:"message_id,omitempty"`
	Kind        string                 `gorm:"size:32;not null" json:"kind"`
	ParamsJSON  string                 `gorm:"type:text;not null;default:'{}'" json:"params_json"`
	PreviewJSON string                 `gorm:"type:text;not null;default:'{}'" json:"preview_json"`
	MenuVersion uint                   `gorm:"not null;default:0" json:"menu_version"`
	Status      DirectorProposalStatus `gorm:"size:16;not null;default:'pending';index" json:"status"`
	CreatedAt   time.Time              `gorm:"index" json:"created_at"`
	ExpiresAt   time.Time              `gorm:"index" json:"expires_at"`
}

func (DirectorProposedAction) TableName() string { return "director_proposed_actions" }

// DirectorActionAudit is the immutable record of an applied (and optionally
// undone) director action, with full before/after snapshots for undo + audit.
type DirectorActionAudit struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	BusinessID       uint       `gorm:"index;not null" json:"business_id"`
	ThreadID         uint       `gorm:"index;not null" json:"thread_id"`
	ProposedActionID uint       `gorm:"index;not null" json:"proposed_action_id"`
	ActorUserID      uint       `gorm:"index;not null" json:"actor_user_id"`
	Kind             string     `gorm:"size:32;not null" json:"kind"`
	ParamsJSON       string     `gorm:"type:text;not null;default:'{}'" json:"params_json"`
	BeforeJSON       string     `gorm:"type:text;not null;default:'[]'" json:"before_json"`
	AfterJSON        string     `gorm:"type:text;not null;default:'[]'" json:"after_json"`
	AppliedAt        time.Time  `json:"applied_at"`
	UndoneAt         *time.Time `json:"undone_at,omitempty"`
	UndoneBy         *uint      `json:"undone_by,omitempty"`
}

func (DirectorActionAudit) TableName() string { return "director_action_audits" }

// newDirectorActionPublicID returns an unguessable, prefixed proposal id.
func newDirectorActionPublicID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "pa_" + hex.EncodeToString(b[:])
}

// CreateDirectorProposedAction inserts a pending proposal, stamping a public id
// + created_at when unset.
func CreateDirectorProposedAction(p *DirectorProposedAction) error {
	if p == nil {
		return fmt.Errorf("nil proposed action")
	}
	if p.PublicID == "" {
		p.PublicID = newDirectorActionPublicID()
	}
	if p.Status == "" {
		p.Status = DirectorProposalPending
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	if err := db.Create(p).Error; err != nil {
		return fmt.Errorf("failed to create director proposed action: %w", err)
	}
	return nil
}

// GetDirectorProposedActionByPublicID loads a proposal scoped to its business.
func GetDirectorProposedActionByPublicID(businessID uint, publicID string) (*DirectorProposedAction, error) {
	var p DirectorProposedAction
	if err := db.Where("business_id = ? AND public_id = ?", businessID, publicID).First(&p).Error; err != nil {
		return nil, fmt.Errorf("failed to get director proposed action: %w", err)
	}
	return &p, nil
}

// GetDirectorProposedActionsByIDs loads proposals by primary-key id, scoped to
// the business, preserving caller order is NOT guaranteed (caller sorts).
func GetDirectorProposedActionsByIDs(businessID uint, ids []uint) ([]DirectorProposedAction, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []DirectorProposedAction
	if err := db.Where("business_id = ? AND id IN ?", businessID, ids).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list director proposed actions: %w", err)
	}
	return rows, nil
}

// DismissDirectorProposals marks a batch of pending proposals dismissed
// (used to retire superseded proposals after a language regeneration).
func DismissDirectorProposals(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return db.Model(&DirectorProposedAction{}).
		Where("id IN ? AND status = ?", ids, DirectorProposalPending).
		Update("status", DirectorProposalDismissed).Error
}

// AttachMessageIDToProposals back-fills the message_id for a batch of proposals —
// used after the assistant message is persisted (mirrors AttachMessageIDToToolCalls).
func AttachMessageIDToProposals(ids []uint, messageID uint) error {
	if len(ids) == 0 {
		return nil
	}
	return db.Model(&DirectorProposedAction{}).
		Where("id IN ?", ids).
		Update("message_id", messageID).Error
}

// GetDirectorActionAuditByProposalID loads the most recent audit for a proposal.
func GetDirectorActionAuditByProposalID(businessID, proposalID uint) (*DirectorActionAudit, error) {
	var a DirectorActionAudit
	if err := db.Where("business_id = ? AND proposed_action_id = ?", businessID, proposalID).
		Order("id DESC").First(&a).Error; err != nil {
		return nil, fmt.Errorf("failed to get director action audit: %w", err)
	}
	return &a, nil
}

// MarkAuditUndoneTx stamps the undo on an audit row within an existing
// transaction. It sets undone_at=now and undone_by=undoneBy atomically with
// the caller's transaction so the restore-write and undo-stamp succeed or
// roll back together.
func MarkAuditUndoneTx(tx *gorm.DB, auditID, undoneBy uint) error {
	now := time.Now()
	return tx.Model(&DirectorActionAudit{}).
		Where("id = ?", auditID).
		Updates(map[string]interface{}{"undone_at": now, "undone_by": undoneBy}).Error
}

// AppliedDirectorAction is a projected row of the "Applied changes" history:
// one committed director action with the linked proposal's public id (for undo)
// and the proposal's PRE-apply menu version (apply bumps by exactly 1, so the
// undo staleness check is current == ProposalMenuVer+1). IsNoOp flags legacy
// zero-match audits (empty before-snapshot, version never bumped) whose undo is
// a safe heal regardless of version. It is a narrow join projection — no
// BeforeJSON/AfterJSON blobs are materialized for the list; IsNoOp is a cheap
// SQL comparison, not a blob read into Go.
type AppliedDirectorAction struct {
	PublicID         string     `json:"public_id"`
	Kind             string     `json:"kind"`
	AppliedAt        time.Time  `json:"applied_at"`
	UndoneAt         *time.Time `json:"undone_at,omitempty"`
	ProposalMenuVer  uint       `json:"proposal_menu_version"`
	IsNoOp           bool       `json:"-"`
	ProposedActionID uint       `json:"-"`
}

// ListAppliedDirectorActions returns the most recent applied director actions
// for a business (newest first, bounded), each joined to its proposal for the
// public id + menu version. limit is clamped to [1,100] (default 25). The
// projection selects only the columns the history view renders — no snapshot
// blobs — so the list stays cheap even with many audited actions.
func ListAppliedDirectorActions(businessID uint, limit int) ([]AppliedDirectorAction, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	var rows []AppliedDirectorAction
	if err := db.Table("director_action_audits AS a").
		Select("p.public_id AS public_id, a.kind AS kind, a.applied_at AS applied_at, a.undone_at AS undone_at, p.menu_version AS proposal_menu_ver, a.before_json IN ('', '[]', 'null') AS is_no_op, a.proposed_action_id AS proposed_action_id").
		Joins("JOIN director_proposed_actions AS p ON p.id = a.proposed_action_id").
		Where("a.business_id = ?", businessID).
		Order("a.applied_at DESC, a.id DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list applied director actions: %w", err)
	}
	return rows, nil
}
