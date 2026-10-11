package database

import (
	"time"

	"gorm.io/gorm"
)

// Chat channel types. Role/dept channels are VIRTUAL — membership is computed
// from Staff.Role + IsActive (+ Position.Department for dept), never stored as
// ChatChannelMember rows. The backing chat_channels row is lazily get-or-created
// on the partial-unique ref_key only so messages have a stable channel_id. Dept
// channels reuse the `role` type with a `dept:` ref-key prefix (no new enum value).
const (
	ChatChannelTypeDirect       = "direct"
	ChatChannelTypeGroup        = "group"
	ChatChannelTypeRole         = "role"
	ChatChannelTypeAnnouncement = "announcement"

	ChatMemberRoleMember = "member"
	ChatMemberRoleAdmin  = "admin"
)

// ChatChannel backs both manual (direct/group) and virtual (role/dept) channels.
// RefKey is non-empty only for virtual channels ("role:server", "dept:BOH") and is
// unique-per-business via the partial unique index idx_chat_channels_refkey.
type ChatChannel struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	BusinessID       uint      `gorm:"not null;index:idx_chat_channels_biz" json:"business_id"`
	Type             string    `gorm:"not null" json:"type"`               // direct|group|role|announcement
	Name             string    `gorm:"not null;default:''" json:"name"`    // display (virtual channels derive it)
	RefKey           string    `gorm:"not null;default:''" json:"ref_key"` // "role:server"/"dept:BOH"; '' for manual
	IsArchived       bool      `gorm:"not null;default:false" json:"is_archived"`
	CreatedByStaffID *uint     `json:"created_by_staff_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (ChatChannel) TableName() string { return "chat_channels" }

// ChatChannelMember rows exist ONLY for manual members (direct/group). Role/dept
// channel membership is virtual and never materialized here. Composite PK
// (channel_id, staff_id).
type ChatChannelMember struct {
	ChannelID  uint       `gorm:"primaryKey;autoIncrement:false" json:"channel_id"`
	StaffID    uint       `gorm:"primaryKey;autoIncrement:false" json:"staff_id"`
	BusinessID uint       `gorm:"not null;index:idx_chat_members_biz" json:"business_id"`
	Role       string     `gorm:"not null;default:'member'" json:"role"` // member|admin
	MutedUntil *time.Time `json:"muted_until"`
	JoinedAt   time.Time  `json:"joined_at"`
}

func (ChatChannelMember) TableName() string { return "chat_channel_members" }

// ChatMessage is soft-deleted (DeletedAt) so ChatRead's last-read message id refs
// stay valid after a moderator delete. The cursor index (business_id, channel_id,
// created_at) backs keyset pagination.
type ChatMessage struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	BusinessID    uint           `gorm:"not null;index:idx_chat_messages_cursor,priority:1" json:"business_id"`
	ChannelID     uint           `gorm:"not null;index:idx_chat_messages_cursor,priority:2" json:"channel_id"`
	SenderStaffID uint           `gorm:"not null" json:"sender_staff_id"`
	SenderName    string         `gorm:"not null;default:''" json:"sender_name"`
	Content       string         `gorm:"not null;default:''" json:"content"`
	ParentID      *uint          `json:"parent_id"` // reserved for future threading (v1 flat)
	AttachmentURL string         `gorm:"not null;default:''" json:"attachment_url"`
	CreatedAt     time.Time      `gorm:"index:idx_chat_messages_cursor,priority:3" json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index:idx_chat_messages_deleted" json:"-"`
}

func (ChatMessage) TableName() string { return "chat_messages" }

// ChatRead tracks the last message a staff member has seen in a channel; unread =
// messages with id > LastReadMessageID. Composite PK (channel_id, staff_id).
type ChatRead struct {
	ChannelID         uint      `gorm:"primaryKey;autoIncrement:false" json:"channel_id"`
	StaffID           uint      `gorm:"primaryKey;autoIncrement:false" json:"staff_id"`
	BusinessID        uint      `gorm:"not null;index:idx_chat_reads_biz" json:"business_id"`
	LastReadMessageID uint      `gorm:"not null;default:0" json:"last_read_message_id"`
	LastReadAt        time.Time `json:"last_read_at"`
}

func (ChatRead) TableName() string { return "chat_reads" }

// Announcement is a one-to-many broadcast with optional proof-of-read (ack).
// AudienceFilter is "all" | "role:server" | "dept:FOH".
type Announcement struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	BusinessID     uint      `gorm:"index:idx_announcements_biz_created,priority:1;not null" json:"business_id"`
	AuthorStaffID  uint      `gorm:"not null" json:"author_staff_id"`
	Title          string    `gorm:"not null" json:"title"`
	Content        string    `gorm:"not null;default:''" json:"content"`
	RequireAck     bool      `gorm:"not null;default:false" json:"require_ack"`
	AudienceFilter string    `gorm:"not null;default:'all'" json:"audience_filter"`
	CreatedAt      time.Time `gorm:"index:idx_announcements_biz_created,priority:2" json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Announcement) TableName() string { return "announcements" }

// AnnouncementAck backs the "X of Y confirmed" aggregate + the unacked list.
// Composite PK (announcement_id, staff_id).
type AnnouncementAck struct {
	AnnouncementID uint      `gorm:"primaryKey;autoIncrement:false" json:"announcement_id"`
	StaffID        uint      `gorm:"primaryKey;autoIncrement:false" json:"staff_id"`
	BusinessID     uint      `gorm:"not null;index:idx_ann_acks_biz" json:"business_id"`
	AcknowledgedAt time.Time `json:"acknowledged_at"`
}

func (AnnouncementAck) TableName() string { return "announcement_acks" }
