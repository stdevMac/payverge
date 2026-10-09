package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMarkChannelReadUpsert proves the read marker upserts on the composite PK
// (channel_id, staff_id): the first call inserts, a second call advances
// last_read_message_id in place (one row, never a duplicate).
func TestMarkChannelReadUpsert(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	r1, err := d.MarkChannelRead(1, 7, 5, 10)
	require.NoError(t, err)
	require.Equal(t, uint(10), r1.LastReadMessageID)
	require.False(t, r1.LastReadAt.IsZero(), "last_read_at is stamped")

	r2, err := d.MarkChannelRead(1, 7, 5, 25)
	require.NoError(t, err)
	require.Equal(t, uint(25), r2.LastReadMessageID, "upsert advances the marker")

	var rows int64
	require.NoError(t, g.Model(&ChatRead{}).Where("channel_id = ? AND staff_id = ?", 7, 5).Count(&rows).Error)
	require.Equal(t, int64(1), rows, "exactly one read row per (channel, staff) — upsert not insert")
}

// TestNewestMessageID returns the max live-message id (0 on an empty channel),
// the default target when a read POST carries no explicit id.
func TestNewestMessageID(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	id, err := d.NewestMessageID(1, 7)
	require.NoError(t, err)
	require.Equal(t, uint(0), id, "empty channel -> 0")

	_, err = d.PostMessage(1, 7, 5, "a", "one")
	require.NoError(t, err)
	m2, err := d.PostMessage(1, 7, 5, "a", "two")
	require.NoError(t, err)

	id, err = d.NewestMessageID(1, 7)
	require.NoError(t, err)
	require.Equal(t, m2.ID, id, "newest message id")
}

// TestLatestMessagePreviews is the channel-list vitality source: ONE query (never
// an N+1 per channel) returning each channel's newest live message as a bounded
// snippet + sender + timestamp. Soft-deleted tails fall back to the previous live
// message; empty channels are simply absent from the map.
func TestLatestMessagePreviews(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	_, err := d.PostMessage(1, 7, 9, "Ana", "older")
	require.NoError(t, err)
	long, err := d.PostMessage(1, 7, 9, "Ana", "This closing-note message is deliberately much longer than the eighty character preview budget allows")
	require.NoError(t, err)
	// A soft-deleted tail must NOT surface — the previous live message wins.
	tail, err := d.PostMessage(1, 8, 9, "Bo", "deleted tail")
	require.NoError(t, err)
	live, err := d.PostMessage(1, 8, 9, "Bo", "still here")
	require.NoError(t, err)
	del, err := d.PostMessage(1, 8, 9, "Bo", "gone")
	require.NoError(t, err)
	require.NoError(t, d.DeleteMessage(1, del.ID))
	_ = tail
	_ = live

	previews, err := d.LatestMessagePreviews(1, []uint{7, 8, 99})
	require.NoError(t, err)

	p7 := previews[7]
	require.NotNil(t, p7)
	require.Equal(t, "Ana", p7.SenderName)
	require.LessOrEqual(t, len([]rune(p7.Snippet)), 81, "snippet is bounded (80 runes + ellipsis)")
	require.Contains(t, p7.Snippet, "This closing-note")
	require.Equal(t, long.CreatedAt.UTC().Truncate(0), p7.CreatedAt.UTC().Truncate(0))

	p8 := previews[8]
	require.NotNil(t, p8)
	require.Equal(t, "still here", p8.Snippet, "soft-deleted tail falls back to the previous live message")

	require.NotContains(t, previews, uint(99), "empty channel has no entry")

	empty, err := d.LatestMessagePreviews(1, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

// TestUnreadCounts is the no-N+1 badge source: one grouped aggregate that counts
// live messages newer than the caller's read marker, excluding the caller's own.
func TestUnreadCounts(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	first, err := d.PostMessage(1, 7, 9, "other", "1")
	require.NoError(t, err)
	_, err = d.PostMessage(1, 7, 9, "other", "2")
	require.NoError(t, err)
	_, err = d.PostMessage(1, 7, 9, "other", "3")
	require.NoError(t, err)
	// The caller's own message must NEVER count as unread for them.
	_, err = d.PostMessage(1, 7, 5, "me", "mine")
	require.NoError(t, err)
	// A second channel the caller has fully read.
	other, err := d.PostMessage(1, 8, 9, "other", "x")
	require.NoError(t, err)

	_, err = d.MarkChannelRead(1, 7, 5, first.ID) // read up to the first message
	require.NoError(t, err)
	_, err = d.MarkChannelRead(1, 8, 5, other.ID) // fully read
	require.NoError(t, err)

	counts, err := d.UnreadCounts(1, 5, []uint{7, 8})
	require.NoError(t, err)
	require.Equal(t, int64(2), counts[7], "2 unread from others after the first (own message excluded)")
	require.Zero(t, counts[8], "fully-read channel has no unread entry")

	// Empty id list short-circuits.
	empty, err := d.UnreadCounts(1, 5, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}
