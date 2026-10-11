package database

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// chatReadDBSeq makes each in-memory DSN unique per harness call so a benchmark
// run with -count=N gets a fresh shared-cache DB instead of colliding on prior rows.
var chatReadDBSeq atomic.Int64

// newChatReadTestDB builds a recorder-backed chat DB (reusing laborSQLRecorder /
// selectStmts from schedule_labor_read_test.go) so the message-read access shape
// can be asserted.
func newChatReadTestDB(t testing.TB) (*DB, *gorm.DB, *laborSQLRecorder) {
	t.Helper()
	rec := &laborSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), chatReadDBSeq.Add(1))
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: rec})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&Business{}, &Staff{}, &ChatChannel{}, &ChatChannelMember{}, &ChatMessage{}))
	require.NoError(t, g.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_channels_refkey ON chat_channels (business_id, ref_key) WHERE ref_key <> ''").Error)
	prev := db
	SetTestDB(g)
	t.Cleanup(func() { SetTestDB(prev) })
	return GetDBWrapper(), g, rec
}

// TestListMessagesAccessShape asserts the message page is a single narrow SELECT
// (no SELECT *), is bounded by LIMIT, excludes soft-deleted rows, and applies the
// keyset cursor — the perf/data-shape guard for the chat read path.
func TestListMessagesAccessShape(t *testing.T) {
	d, g, rec := newChatReadTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	base := time.Now().UTC()
	for i := 0; i < 80; i++ {
		require.NoError(t, g.Create(&ChatMessage{BusinessID: 1, ChannelID: 7, SenderStaffID: 5,
			Content: fmt.Sprintf("m%d", i), CreatedAt: base.Add(time.Duration(i) * time.Second)}).Error)
	}
	// Soft-delete one message; it must not appear in the page.
	require.NoError(t, g.Where("business_id = ? AND channel_id = ? AND content = ?", 1, 7, "m0").Delete(&ChatMessage{}).Error)

	rec.stmts = nil
	msgs, err := d.ListMessages(1, 7, 0, 0) // limit 0 -> capped at chatMessageListMax
	require.NoError(t, err)
	require.Len(t, msgs, chatMessageListMax, "page bounded to the max")
	require.Greater(t, msgs[0].ID, msgs[len(msgs)-1].ID, "newest-first ordering")

	sels := selectStmts(rec)
	require.Len(t, sels, 1, "exactly one SELECT, no N+1")
	low := strings.ToLower(sels[0])
	require.NotContains(t, low, "select *", "must be a narrow projection")
	require.Contains(t, low, "limit", "result set must be bounded")
	for _, col := range []string{"sender_staff_id", "content", "created_at"} {
		require.Contains(t, low, col)
	}

	// Cursor page: only messages with id < cursor come back.
	cursor := msgs[len(msgs)-1].ID
	older, err := d.ListMessages(1, 7, cursor, 10)
	require.NoError(t, err)
	for _, m := range older {
		require.Less(t, m.ID, cursor, "keyset cursor excludes id >= cursor")
	}

	// An oversized client limit is clamped DOWN to the server cap — a caller cannot
	// widen the page beyond chatMessageListMax.
	wide, err := d.ListMessages(1, 7, 0, 10_000)
	require.NoError(t, err)
	require.Len(t, wide, chatMessageListMax, "limit above the cap is clamped to chatMessageListMax")
}

// TestGetOrCreateDMIdempotent proves a stable per-pair direct channel with exactly
// two members, order-independent on the staff ids.
func TestGetOrCreateDMIdempotent(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	a, err := d.GetOrCreateDM(1, 6, 5)
	require.NoError(t, err)
	require.NotZero(t, a.ID)
	require.Equal(t, ChatChannelTypeDirect, a.Type)
	require.Equal(t, "dm:5:6", a.RefKey, "ref_key is order-independent (lo:hi)")

	b, err := d.GetOrCreateDM(1, 5, 6) // reversed args -> same channel
	require.NoError(t, err)
	require.Equal(t, a.ID, b.ID, "DM is idempotent for the pair regardless of arg order")

	var members int64
	require.NoError(t, g.Model(&ChatChannelMember{}).Where("channel_id = ?", a.ID).Count(&members).Error)
	require.Equal(t, int64(2), members, "exactly two membership rows, no duplicates")

	_, err = d.GetOrCreateDM(1, 5, 5)
	require.Error(t, err, "a self-DM is rejected")
}

// TestPostMessageRoundTrip confirms a posted message is readable back in the page.
func TestPostMessageRoundTrip(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	msg, err := d.PostMessage(1, 7, 5, "Alice", "hello team")
	require.NoError(t, err)
	require.NotZero(t, msg.ID)

	page, err := d.ListMessages(1, 7, 0, 10)
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Equal(t, "hello team", page[0].Content)
	require.Equal(t, "Alice", page[0].SenderName)
}

// TestPostMessageContentCap pins the ChatContentMaxRunes boundary in runes (a
// multibyte rune counts once), so every caller of PostMessage is bounded.
func TestPostMessageContentCap(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	atCap := strings.Repeat("é", ChatContentMaxRunes)
	msg, err := d.PostMessage(1, 7, 5, "Alice", atCap)
	require.NoError(t, err, "content at the cap is accepted")
	require.NotZero(t, msg.ID)

	_, err = d.PostMessage(1, 7, 5, "Alice", atCap+"x")
	require.ErrorIs(t, err, ErrChatMessageTooLong)

	var n int64
	require.NoError(t, g.Model(&ChatMessage{}).Where("business_id = ?", 1).Count(&n).Error)
	require.Equal(t, int64(1), n, "the over-long message is not stored")
}

// TestDeleteMessageSoftDeleteExcludedFromList proves a moderator delete drops the
// message from ListMessages but preserves the underlying row (read-ref integrity),
// and that cross-tenant / missing deletes are not-found.
func TestDeleteMessageSoftDeleteExcludedFromList(t *testing.T) {
	d, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	m1, err := d.PostMessage(1, 7, 5, "a", "keep")
	require.NoError(t, err)
	m2, err := d.PostMessage(1, 7, 5, "a", "delete me")
	require.NoError(t, err)

	require.NoError(t, d.DeleteMessage(1, m2.ID))

	page, err := d.ListMessages(1, 7, 0, 50)
	require.NoError(t, err)
	require.Len(t, page, 1, "soft-deleted message excluded from the list")
	require.Equal(t, m1.ID, page[0].ID)

	var withDeleted int64
	require.NoError(t, g.Unscoped().Model(&ChatMessage{}).Where("id = ?", m2.ID).Count(&withDeleted).Error)
	require.Equal(t, int64(1), withDeleted, "row preserved for read-ref integrity")

	require.ErrorIs(t, d.DeleteMessage(2, m1.ID), ErrChatMessageNotFound, "cross-tenant delete is not-found")
	require.ErrorIs(t, d.DeleteMessage(1, 999999), ErrChatMessageNotFound, "missing message is not-found")
}

func BenchmarkListMessages(b *testing.B) {
	d, g, _ := newChatReadTestDB(b)
	require.NoError(b, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	base := time.Now().UTC()
	for i := 0; i < 200; i++ {
		require.NoError(b, g.Create(&ChatMessage{BusinessID: 1, ChannelID: 7, SenderStaffID: 5,
			Content: fmt.Sprintf("m%d", i), CreatedAt: base.Add(time.Duration(i) * time.Second)}).Error)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.ListMessages(1, 7, 0, chatMessageListMax)
	}
}
