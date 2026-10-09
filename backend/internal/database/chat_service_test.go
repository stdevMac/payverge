package database

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var chatTestDBSeq atomic.Int64

// newChatTestDB builds an isolated in-memory DB with the chat tables materialized
// from struct tags (the genesis/autoMigrate path) PLUS the partial-unique ref_key
// index ensured exactly as db_config.go does on a fresh DB.
func newChatTestDB(t testing.TB) (*DB, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), chatTestDBSeq.Add(1))
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(
		&Business{}, &Staff{}, &Position{}, &StaffPosition{},
		&ChatChannel{}, &ChatChannelMember{}, &ChatMessage{}, &ChatRead{}, &Announcement{}, &AnnouncementAck{},
	))
	// Genesis-safe partial-unique (GORM tags can't express WHERE) — mirrors db_config.go.
	require.NoError(t, g.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_channels_refkey ON chat_channels (business_id, ref_key) WHERE ref_key <> ''").Error)
	prev := db
	SetTestDB(g)
	t.Cleanup(func() { SetTestDB(prev) })
	return GetDBWrapper(), g
}

// TestChatGenesisShape asserts the chat tables + EVERY index (incl. the partial
// unique ref_key) materialize on a genesis (force-baselined) DB from struct tags,
// not just columns — the genesis-safety guard the prior slices established.
func TestChatGenesisShape(t *testing.T) {
	_, g := newChatTestDB(t)
	m := g.Migrator()

	for _, tbl := range []interface{}{
		&ChatChannel{}, &ChatChannelMember{}, &ChatMessage{}, &ChatRead{}, &Announcement{}, &AnnouncementAck{},
	} {
		require.Truef(t, m.HasTable(tbl), "table for %T must exist on genesis", tbl)
	}

	idxChecks := []struct {
		model interface{}
		index string
	}{
		{&ChatChannel{}, "idx_chat_channels_biz"},
		{&ChatChannel{}, "idx_chat_channels_refkey"},
		{&ChatChannelMember{}, "idx_chat_members_biz"},
		{&ChatMessage{}, "idx_chat_messages_cursor"},
		{&ChatMessage{}, "idx_chat_messages_deleted"},
		{&ChatRead{}, "idx_chat_reads_biz"},
		{&Announcement{}, "idx_announcements_biz_created"},
		{&AnnouncementAck{}, "idx_ann_acks_biz"},
	}
	for _, c := range idxChecks {
		require.Truef(t, m.HasIndex(c.model, c.index), "missing genesis index %s on %T", c.index, c.model)
	}
}

// TestChatChannelRefKeyPartialUniqueEnforced proves the virtual-channel idempotency
// guard: two non-empty ref_keys collide, but multiple empty ref_keys (manual
// direct/group channels) are allowed.
func TestChatChannelRefKeyPartialUniqueEnforced(t *testing.T) {
	_, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	require.NoError(t, g.Create(&ChatChannel{BusinessID: 1, Type: ChatChannelTypeRole, RefKey: "role:server"}).Error)
	// Duplicate virtual ref_key for the same business must be rejected.
	require.Error(t, g.Create(&ChatChannel{BusinessID: 1, Type: ChatChannelTypeRole, RefKey: "role:server"}).Error,
		"duplicate (business_id, ref_key) virtual channel must violate the partial unique index")
	// A different business may reuse the same ref_key.
	require.NoError(t, g.Create(&Business{ID: 2, BusinessId: "biz-2"}).Error)
	require.NoError(t, g.Create(&ChatChannel{BusinessID: 2, Type: ChatChannelTypeRole, RefKey: "role:server"}).Error)
	// Multiple manual channels (empty ref_key) are allowed — the partial WHERE excludes them.
	require.NoError(t, g.Create(&ChatChannel{BusinessID: 1, Type: ChatChannelTypeDirect, RefKey: ""}).Error)
	require.NoError(t, g.Create(&ChatChannel{BusinessID: 1, Type: ChatChannelTypeDirect, RefKey: ""}).Error)
}

// TestChatMessageSoftDeletePreservesRow confirms a moderator delete soft-deletes
// (so ChatRead last-read refs survive) rather than hard-deleting.
func TestChatMessageSoftDeletePreservesRow(t *testing.T) {
	_, g := newChatTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	msg := ChatMessage{BusinessID: 1, ChannelID: 5, SenderStaffID: 7, Content: "hi", CreatedAt: time.Now().UTC()}
	require.NoError(t, g.Create(&msg).Error)
	require.NoError(t, g.Delete(&msg).Error) // soft delete

	var live int64
	require.NoError(t, g.Model(&ChatMessage{}).Where("id = ?", msg.ID).Count(&live).Error)
	require.Equal(t, int64(0), live, "soft-deleted message excluded from default scope")
	var withDeleted int64
	require.NoError(t, g.Unscoped().Model(&ChatMessage{}).Where("id = ?", msg.ID).Count(&withDeleted).Error)
	require.Equal(t, int64(1), withDeleted, "row physically preserved for read-ref integrity")
}
