package database

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// chatMessageListMax bounds a single message page (perf gate). The keyset cursor
// (WHERE id < ?) drives older pages, so callers never need a larger window.
const chatMessageListMax = 50

// ErrChatMessageNotFound is returned when a message does not exist within the
// caller's business (tenant-scoped moderation).
var ErrChatMessageNotFound = errors.New("chat message not found")

// ListMessages returns a newest-first page of live messages for a channel via a
// keyset cursor on the monotonic id (id tracks created_at), tenant-scoped. The
// soft-delete scope (deleted_at IS NULL) is applied automatically, so moderator
// deletes drop out without breaking last-read refs. The projection is NARROW
// (explicit Select — no SELECT *, the deleted_at column never reaches the wire)
// and the result is bounded by LIMIT min(limit, chatMessageListMax).
func (d *DB) ListMessages(businessID, channelID, cursorID uint, limit int) ([]ChatMessage, error) {
	if limit <= 0 || limit > chatMessageListMax {
		limit = chatMessageListMax
	}
	out := make([]ChatMessage, 0, limit)
	q := d.GetGorm().Model(&ChatMessage{}).
		Select("id, business_id, channel_id, sender_staff_id, sender_name, content, parent_id, attachment_url, created_at, updated_at").
		Where("business_id = ? AND channel_id = ?", businessID, channelID)
	if cursorID > 0 {
		q = q.Where("id < ?", cursorID)
	}
	err := q.Order("id DESC").Limit(limit).Find(&out).Error
	return out, err
}

// PostMessage appends a flat message to a channel. Callers must have already
// passed CanReadChannel; this method does no authorization. CreatedAt is stamped
// UTC so the keyset cursor (id ↔ created_at) stays monotonic.
func (d *DB) PostMessage(businessID, channelID, senderStaffID uint, senderName, content string) (*ChatMessage, error) {
	msg := ChatMessage{
		BusinessID:    businessID,
		ChannelID:     channelID,
		SenderStaffID: senderStaffID,
		SenderName:    senderName,
		Content:       content,
		CreatedAt:     time.Now().UTC(),
	}
	if err := d.GetGorm().Create(&msg).Error; err != nil {
		return nil, err
	}
	return &msg, nil
}

// MessageChannelType returns the TYPE of the channel a live message belongs to
// (tenant-scoped), so moderation can refuse to touch private direct messages —
// a manager must not be able to delete a 1:1 DM between two other staff. Returns
// ErrChatMessageNotFound when no live message matches in the business.
func (d *DB) MessageChannelType(businessID, messageID uint) (string, error) {
	if businessID == 0 || messageID == 0 {
		return "", ErrChatMessageNotFound
	}
	var typ string
	err := d.GetGorm().Table("chat_messages AS m").
		Joins("JOIN chat_channels c ON c.id = m.channel_id").
		Where("m.id = ? AND m.business_id = ? AND m.deleted_at IS NULL", messageID, businessID).
		Select("c.type").Scan(&typ).Error
	if err != nil {
		return "", err
	}
	if typ == "" {
		return "", ErrChatMessageNotFound
	}
	return typ, nil
}

// DeleteMessage soft-deletes a message tenant-scoped (moderation). The row is
// preserved (gorm.DeletedAt) so ChatRead last-read refs stay valid; the message
// simply drops out of ListMessages' default non-deleted scope. Returns
// ErrChatMessageNotFound when no live message matches in the business.
func (d *DB) DeleteMessage(businessID, messageID uint) error {
	if businessID == 0 || messageID == 0 {
		return ErrChatMessageNotFound
	}
	res := d.GetGorm().Where("id = ? AND business_id = ?", messageID, businessID).Delete(&ChatMessage{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrChatMessageNotFound
	}
	return nil
}

// dmRefKey builds the stable, order-independent ref_key for a staff pair so the
// DM is idempotent on the partial-unique idx_chat_channels_refkey.
func dmRefKey(staffA, staffB uint) string {
	lo, hi := staffA, staffB
	if lo > hi {
		lo, hi = hi, lo
	}
	return fmt.Sprintf("dm:%d:%d", lo, hi)
}

// GetOrCreateDM resolves the single direct channel for a staff pair, creating it
// (and both ChatChannelMember rows) on first use. Idempotent: the canonical row is
// keyed by the order-independent ref_key "dm:<lo>:<hi>" against the partial-unique
// index, and the membership inserts ride ON CONFLICT DO NOTHING — so repeated calls
// converge on one channel with exactly two members.
func (d *DB) GetOrCreateDM(businessID, staffA, staffB uint) (*ChatChannel, error) {
	if businessID == 0 || staffA == 0 || staffB == 0 || staffA == staffB {
		return nil, ErrChatChannelNotFound
	}
	refKey := dmRefKey(staffA, staffB)
	var out ChatChannel
	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		ch := ChatChannel{BusinessID: businessID, Type: ChatChannelTypeDirect, RefKey: refKey}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ch).Error; err != nil {
			return err
		}
		if err := tx.Where("business_id = ? AND ref_key = ?", businessID, refKey).First(&out).Error; err != nil {
			return err
		}
		for _, sid := range []uint{staffA, staffB} {
			m := ChatChannelMember{
				ChannelID: out.ID, StaffID: sid, BusinessID: businessID,
				Role: ChatMemberRoleMember, JoinedAt: time.Now().UTC(),
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&m).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
