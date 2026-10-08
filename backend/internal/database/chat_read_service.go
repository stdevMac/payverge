package database

import (
	"time"

	"gorm.io/gorm/clause"
)

// MarkChannelRead upserts the caller's read marker for a channel on the composite
// PK (channel_id, staff_id), advancing LastReadMessageID + LastReadAt. The marker
// is "last write wins"; the handler resolves an empty body to the channel's
// newest message id, so in practice it only moves forward. LastReadAt is stamped
// UTC so it is comparable across business timezones. Authorization (CanReadChannel)
// is the caller's responsibility — this method does none.
func (d *DB) MarkChannelRead(businessID, channelID, staffID, lastReadMessageID uint) (*ChatRead, error) {
	if businessID == 0 || channelID == 0 || staffID == 0 {
		return nil, ErrChatChannelNotFound
	}
	row := ChatRead{
		ChannelID:         channelID,
		StaffID:           staffID,
		BusinessID:        businessID,
		LastReadMessageID: lastReadMessageID,
		LastReadAt:        time.Now().UTC(),
	}
	err := d.GetGorm().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "staff_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_read_message_id", "last_read_at"}),
	}).Create(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// NewestMessageID returns the id of the most recent live (non-soft-deleted)
// message in a channel, or 0 when the channel is empty. A read POST with no
// explicit id defaults to this ("mark everything read").
func (d *DB) NewestMessageID(businessID, channelID uint) (uint, error) {
	var id uint
	err := d.GetGorm().Model(&ChatMessage{}).
		Where("business_id = ? AND channel_id = ?", businessID, channelID).
		Select("COALESCE(MAX(id), 0)").
		Scan(&id).Error
	return id, err
}

// ChatChannelPreview is the channel-list vitality payload: the newest live
// message per channel as a bounded snippet, so the list reads as a living room
// instead of a bare directory.
type ChatChannelPreview struct {
	Snippet    string    `json:"snippet"`
	SenderName string    `json:"sender_name"`
	CreatedAt  time.Time `json:"created_at"`
}

const previewSnippetRunes = 80

// LatestMessagePreviews returns, for a bounded set of channel ids, each
// channel's newest live (non-soft-deleted) message trimmed to a display snippet.
// It is ONE query (a MAX(id)-per-channel subquery), never an N+1 per channel —
// the channel-list preview source next to UnreadCounts. Channels with no live
// messages are simply absent from the map.
func (d *DB) LatestMessagePreviews(businessID uint, channelIDs []uint) (map[uint]*ChatChannelPreview, error) {
	out := make(map[uint]*ChatChannelPreview, len(channelIDs))
	if len(channelIDs) == 0 {
		return out, nil
	}
	type previewRow struct {
		ChannelID  uint
		Content    string
		SenderName string
		CreatedAt  time.Time
	}
	var rows []previewRow
	err := d.GetGorm().Table("chat_messages AS m").
		Select("m.channel_id AS channel_id, m.content AS content, m.sender_name AS sender_name, m.created_at AS created_at").
		Where("m.business_id = ? AND m.channel_id IN ? AND m.deleted_at IS NULL", businessID, channelIDs).
		Where("m.id IN (?)", d.GetGorm().Table("chat_messages").
			Select("MAX(id)").
			Where("business_id = ? AND channel_id IN ? AND deleted_at IS NULL", businessID, channelIDs).
			Group("channel_id")).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		snippet := []rune(r.Content)
		if len(snippet) > previewSnippetRunes {
			snippet = append(snippet[:previewSnippetRunes], '…')
		}
		out[r.ChannelID] = &ChatChannelPreview{
			Snippet:    string(snippet),
			SenderName: r.SenderName,
			CreatedAt:  r.CreatedAt,
		}
	}
	return out, nil
}

// UnreadCounts returns, for a bounded set of channel ids, how many live messages
// the staff member has NOT yet read (id > their last-read marker) and did not
// send themselves. It is ONE grouped aggregate (a single LEFT JOIN chat_reads),
// never an N+1 per channel — the chat channel-list badge source. The raw-table
// query does not get GORM's automatic soft-delete scope, so deleted_at IS NULL is
// applied explicitly. Channels with zero unread are simply absent from the map.
func (d *DB) UnreadCounts(businessID, staffID uint, channelIDs []uint) (map[uint]int64, error) {
	out := make(map[uint]int64, len(channelIDs))
	if len(channelIDs) == 0 {
		return out, nil
	}
	type unreadRow struct {
		ChannelID uint
		Unread    int64
	}
	var rows []unreadRow
	err := d.GetGorm().Table("chat_messages AS m").
		Select("m.channel_id AS channel_id, COUNT(*) AS unread").
		Joins("LEFT JOIN chat_reads r ON r.channel_id = m.channel_id AND r.staff_id = ? AND r.business_id = ?", staffID, businessID).
		Where("m.business_id = ? AND m.channel_id IN ? AND m.deleted_at IS NULL AND m.sender_staff_id <> ? AND m.id > COALESCE(r.last_read_message_id, 0)",
			businessID, channelIDs, staffID).
		Group("m.channel_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ChannelID] = r.Unread
	}
	return out, nil
}
