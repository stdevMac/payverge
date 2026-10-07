package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newChatTestServer(t *testing.T, staffID uint, role string) (func(method, url string, body interface{}) *httptest.ResponseRecorder, *gorm.DB, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}, &database.Position{},
		&database.StaffPosition{}, &database.ChatChannel{}, &database.ChatChannelMember{}, &database.ChatMessage{},
		&database.ChatRead{}, &database.Announcement{}, &database.AnnouncementAck{}))
	// Partial-unique ref_key — mirrors db_config.go so virtual/DM channels stay idempotent.
	require.NoError(t, gormDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_channels_refkey ON chat_channels (business_id, ref_key) WHERE ref_key <> ''").Error)
	database.SetTestDB(gormDB)
	h := NewChatHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if staffID != 0 {
			c.Set("staff_id", staffID)
			c.Set("staff_role", role)
		}
		c.Next()
	})
	r.GET("/b/:id/chat/channels", h.GetChannels)
	r.GET("/b/:id/chat/channels/:channelId/messages", h.GetMessages)
	r.POST("/b/:id/chat/channels/:channelId/messages", h.PostMessage)
	r.POST("/b/:id/chat/dm/:staffId", h.PostDM)
	r.POST("/b/:id/chat/channels/:channelId/read", h.MarkRead)
	r.POST("/b/:id/announcements", h.CreateAnnouncement)
	r.GET("/b/:id/announcements", h.ListAnnouncements)
	r.POST("/b/:id/announcements/:announcementId/ack", h.AckAnnouncement)
	r.GET("/b/:id/announcements/:announcementId/acks", h.GetAnnouncementAcks)
	r.DELETE("/b/:id/chat/messages/:messageId", h.DeleteMessage)
	do := func(method, url string, body interface{}) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			bb, _ := json.Marshal(body)
			rdr = bytes.NewReader(bb)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, url, rdr)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	return do, gormDB, func() { _ = sqlDB.Close() }
}

// chatRouterAs builds a request runner bound to the CURRENT test DB (set via
// SetTestDB by newChatTestServer) but acting as a different staff identity — used
// to drive the same chat endpoints from a second staff member without re-seeding.
func chatRouterAs(staffID uint, role string) func(method, url string, body interface{}) *httptest.ResponseRecorder {
	h := NewChatHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("staff_id", staffID)
		c.Set("staff_role", role)
		c.Next()
	})
	r.POST("/b/:id/announcements/:announcementId/ack", h.AckAnnouncement)
	r.GET("/b/:id/announcements", h.ListAnnouncements)
	r.POST("/b/:id/chat/channels/:channelId/messages", h.PostMessage)
	r.DELETE("/b/:id/chat/messages/:messageId", h.DeleteMessage)
	return func(method, url string, body interface{}) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			bb, _ := json.Marshal(body)
			rdr = bytes.NewReader(bb)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, url, rdr)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
}

// TestChatNonMemberForbidden is the REQUIRED privacy test: a non-member GET and
// POST on a group channel both return 403.
func TestChatNonMemberForbidden(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 5, "server") // caller is staff 5
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	// A group channel that staff 5 is NOT a member of (only staff 7 is).
	group := database.ChatChannel{ID: 30, BusinessID: 1, Type: database.ChatChannelTypeGroup, Name: "Shift A"}
	require.NoError(t, g.Create(&group).Error)
	require.NoError(t, g.Create(&database.ChatChannelMember{ChannelID: 30, StaffID: 7, BusinessID: 1, Role: database.ChatMemberRoleMember}).Error)

	w := do(http.MethodGet, "/b/1/chat/channels/30/messages", nil)
	require.Equal(t, http.StatusForbidden, w.Code, "non-member GET must be 403")

	w = do(http.MethodPost, "/b/1/chat/channels/30/messages", map[string]interface{}{"content": "sneaking in"})
	require.Equal(t, http.StatusForbidden, w.Code, "non-member POST must be 403")
}

// TestChatRoleChannelPrivacyHTTP confirms role channels are readable only by the
// matching role: a server may read role:server but not role:host.
func TestChatRoleChannelPrivacyHTTP(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 5, "server")
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.ChatChannel{ID: 40, BusinessID: 1, Type: database.ChatChannelTypeRole, RefKey: "role:server"}).Error)
	require.NoError(t, g.Create(&database.ChatChannel{ID: 41, BusinessID: 1, Type: database.ChatChannelTypeRole, RefKey: "role:host"}).Error)

	w := do(http.MethodGet, "/b/1/chat/channels/40/messages", nil)
	require.Equal(t, http.StatusOK, w.Code, "matching role can read its role channel")

	w = do(http.MethodGet, "/b/1/chat/channels/41/messages", nil)
	require.Equal(t, http.StatusForbidden, w.Code, "a server cannot read role:host")
}

// TestChatDMCreateAndReadHTTP drives the DM happy path: create via POST, then the
// member reads it back; a non-member is rejected.
func TestChatDMCreateAndReadHTTP(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 5, "server") // caller staff 5
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)

	w := do(http.MethodPost, "/b/1/chat/dm/6", map[string]interface{}{"content": "hey 6"})
	require.Equal(t, http.StatusCreated, w.Code)
	var resp struct {
		Data struct {
			Channel database.ChatChannel `json:"channel"`
			Message database.ChatMessage `json:"message"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotZero(t, resp.Data.Channel.ID)
	require.Equal(t, "dm:5:6", resp.Data.Channel.RefKey)
	chID := resp.Data.Channel.ID

	// The member (staff 5) can read the DM back.
	w = do(http.MethodGet, fmt.Sprintf("/b/1/chat/channels/%d/messages", chID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	// A no-content DM call just returns the (same) channel with 200.
	w = do(http.MethodPost, "/b/1/chat/dm/6", nil)
	require.Equal(t, http.StatusOK, w.Code)
}

// TestChatDMNonMemberForbiddenHTTP: a staff member outside a DM pair cannot read it.
func TestChatDMNonMemberForbiddenHTTP(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 9, "server") // caller staff 9 (outsider)
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	// A DM between 5 and 6; staff 9 is not a member.
	dm := database.ChatChannel{ID: 50, BusinessID: 1, Type: database.ChatChannelTypeDirect, RefKey: "dm:5:6"}
	require.NoError(t, g.Create(&dm).Error)
	require.NoError(t, g.Create(&database.ChatChannelMember{ChannelID: 50, StaffID: 5, BusinessID: 1}).Error)
	require.NoError(t, g.Create(&database.ChatChannelMember{ChannelID: 50, StaffID: 6, BusinessID: 1}).Error)

	w := do(http.MethodGet, "/b/1/chat/channels/50/messages", nil)
	require.Equal(t, http.StatusForbidden, w.Code, "DM outsider must be 403")
}

// TestChatSSEFramesAreContentFree is the load-bearing privacy assertion for the
// realtime layer: posting a message and an announcement publishes ids-only frames
// (channel_id+message_id / announcement_id) with NO message body or content. The
// hub fans per-business and chat:read is universal, so any body in the frame would
// leak a DM to every staff member.
func TestChatSSEFramesAreContentFree(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 5, "server")
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.ChatChannel{ID: 90, BusinessID: 1, Type: database.ChatChannelTypeRole, RefKey: "role:server"}).Error)

	// Subscribe to the chat topics BEFORE posting so the live frames arrive.
	msgCh, _, cancelMsg := events.GetHub().SubscribeWithReplayTopic(1, 0, "chat.message")
	defer cancelMsg()
	annCh, _, cancelAnn := events.GetHub().SubscribeWithReplayTopic(1, 0, "chat.announcement")
	defer cancelAnn()

	w := do(http.MethodPost, "/b/1/chat/channels/90/messages", map[string]interface{}{"content": "top secret DM body"})
	require.Equal(t, http.StatusCreated, w.Code)

	frame := readFrame(t, msgCh)
	require.Equal(t, "chat.message", frame.Type)
	payload := decodeFrame(t, frame.Data)
	require.Contains(t, payload, "channel_id")
	require.Contains(t, payload, "message_id")
	require.NotContains(t, payload, "content", "frame must NOT carry the message body")
	require.NotContains(t, payload, "body")
	require.NotContains(t, payload, "sender_name")
	// And nowhere in the raw bytes does the secret text appear.
	require.NotContains(t, string(frame.Data), "top secret DM body")

	w = do(http.MethodPost, "/b/1/announcements", map[string]interface{}{"title": "secret title", "content": "secret body", "audience_filter": "all"})
	// caller role server lacks chat:announce at the route layer, but the test
	// harness has no RBAC middleware, so the handler runs and publishes.
	require.Equal(t, http.StatusCreated, w.Code)

	frame = readFrame(t, annCh)
	require.Equal(t, "chat.announcement", frame.Type)
	payload = decodeFrame(t, frame.Data)
	require.Contains(t, payload, "announcement_id")
	require.NotContains(t, payload, "title", "frame must NOT carry the announcement title")
	require.NotContains(t, payload, "content")
	require.NotContains(t, string(frame.Data), "secret")
}

func readFrame(t *testing.T, ch <-chan events.BusinessEvent) events.BusinessEvent {
	t.Helper()
	select {
	case f := <-ch:
		return f
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE frame")
		return events.BusinessEvent{}
	}
}

func decodeFrame(t *testing.T, raw json.RawMessage) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// TestChatModerateDeleteHTTP drives the moderator delete: a deleted message drops
// from the channel list while the row physically survives (read-ref integrity);
// a missing/foreign message is 404.
func TestChatModerateDeleteHTTP(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 5, "server")
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.ChatChannel{ID: 80, BusinessID: 1, Type: database.ChatChannelTypeRole, RefKey: "role:server"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 200, BusinessID: 1, ChannelID: 80, SenderStaffID: 9, Content: "keep"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 201, BusinessID: 1, ChannelID: 80, SenderStaffID: 9, Content: "spam"}).Error)

	w := do(http.MethodDelete, "/b/1/chat/messages/201", nil)
	require.Equal(t, http.StatusOK, w.Code)

	w = do(http.MethodGet, "/b/1/chat/channels/80/messages", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []database.ChatMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1, "deleted message excluded from the list")
	require.Equal(t, uint(200), resp.Data[0].ID)

	var withDeleted int64
	require.NoError(t, g.Unscoped().Model(&database.ChatMessage{}).Where("id = ?", 201).Count(&withDeleted).Error)
	require.Equal(t, int64(1), withDeleted, "soft-deleted row preserved")

	w = do(http.MethodDelete, "/b/1/chat/messages/999999", nil)
	require.Equal(t, http.StatusNotFound, w.Code, "missing message -> 404")
}

// TestChatAnnouncementsHTTP drives the announcement lifecycle: a manager creates
// a role-targeted announcement, a server in that role sees it and acks (idempotently),
// and the acks roster reports the correct X-of-Y aggregate.
func TestChatAnnouncementsHTTP(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 1, "manager") // caller staff 1 (manager/author)
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.Staff{ID: 5, BusinessID: 1, Email: "a@x.co", Name: "Ana", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	require.NoError(t, g.Create(&database.Staff{ID: 6, BusinessID: 1, Email: "b@x.co", Name: "Bo", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)

	w := do(http.MethodPost, "/b/1/announcements", map[string]interface{}{
		"title": "Shift change", "content": "read up", "require_ack": true, "audience_filter": "role:server",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct {
		Data database.Announcement `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	annID := created.Data.ID
	require.NotZero(t, annID)
	require.Equal(t, "role:server", created.Data.AudienceFilter)

	// Invalid audience -> 400.
	w = do(http.MethodPost, "/b/1/announcements", map[string]interface{}{"title": "x", "audience_filter": "team:lol"})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// Acks roster before anyone confirms: 0 of 2.
	w = do(http.MethodGet, fmt.Sprintf("/b/1/announcements/%d/acks", annID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var status struct {
		Data database.AnnouncementAckStatus `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	require.Equal(t, 2, status.Data.TotalEligible)
	require.Equal(t, 0, status.Data.Acked)
	require.Len(t, status.Data.Unacked, 2)

	// Server 5 acks (twice -> idempotent). A second router bound to the SAME test
	// DB but acting as staff 5 lets us exercise the ack from the audience member.
	doServer := chatRouterAs(5, "server")
	w = doServer(http.MethodPost, fmt.Sprintf("/b/1/announcements/%d/ack", annID), map[string]interface{}{})
	require.Equal(t, http.StatusOK, w.Code)
	w = doServer(http.MethodPost, fmt.Sprintf("/b/1/announcements/%d/ack", annID), map[string]interface{}{})
	require.Equal(t, http.StatusOK, w.Code)

	// Roster now 1 of 2.
	w = do(http.MethodGet, fmt.Sprintf("/b/1/announcements/%d/acks", annID), nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	require.Equal(t, 1, status.Data.Acked)
	require.Len(t, status.Data.Unacked, 1)
	require.Equal(t, uint(6), status.Data.Unacked[0].StaffID, "only the non-acking server remains")

	// Server 5 sees the role:server announcement in their feed, flagged acked (they
	// confirmed it above) so the feed can hide the Acknowledge button.
	w = doServer(http.MethodGet, "/b/1/announcements", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var feed struct {
		Data  []database.Announcement `json:"data"`
		Acked map[uint]bool           `json:"acked"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &feed))
	require.Len(t, feed.Data, 1)
	require.Equal(t, annID, feed.Data[0].ID)
	require.True(t, feed.Acked[annID], "the caller's prior ack is reflected in the feed")

	// Server 6 (who never acked) sees the same announcement but NOT flagged acked.
	// Decode into a FRESH struct — Unmarshal merges into an existing map.
	doServer6 := chatRouterAs(6, "server")
	w = doServer6(http.MethodGet, "/b/1/announcements", nil)
	var feed6 struct {
		Acked map[uint]bool `json:"acked"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &feed6))
	require.False(t, feed6.Acked[annID], "a non-acking caller's feed leaves the notice unacked")
}

// TestChatMarkReadHTTP drives the read-marker upsert: the matching role marks a
// role channel read (defaulting to the newest message), and a non-member is 403.
func TestChatMarkReadHTTP(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 5, "server") // caller staff 5 (server)
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.ChatChannel{ID: 70, BusinessID: 1, Type: database.ChatChannelTypeRole, RefKey: "role:server"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 100, BusinessID: 1, ChannelID: 70, SenderStaffID: 9, Content: "hi"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 101, BusinessID: 1, ChannelID: 70, SenderStaffID: 9, Content: "again"}).Error)

	// No explicit id -> defaults to the newest message (101).
	w := do(http.MethodPost, "/b/1/chat/channels/70/read", map[string]interface{}{})
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data database.ChatRead `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, uint(101), resp.Data.LastReadMessageID, "empty body defaults to newest message id")

	var rows int64
	require.NoError(t, g.Model(&database.ChatRead{}).Where("channel_id = ? AND staff_id = ?", 70, 5).Count(&rows).Error)
	require.Equal(t, int64(1), rows, "single upserted read row")

	// A server cannot mark a role:host channel read (non-member -> 403).
	require.NoError(t, g.Create(&database.ChatChannel{ID: 71, BusinessID: 1, Type: database.ChatChannelTypeRole, RefKey: "role:host"}).Error)
	w = do(http.MethodPost, "/b/1/chat/channels/71/read", map[string]interface{}{"last_read_message_id": 5})
	require.Equal(t, http.StatusForbidden, w.Code, "non-member read marker must be 403")
}

// TestChatChannelsListHTTP confirms the channels endpoint returns the caller's
// role channel and DM, lazily materialized.
func TestChatChannelsListHTTP(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 5, "server")
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, database.GetDBWrapper().GetGorm().Create(&database.ChatChannel{ID: 60, BusinessID: 1, Type: database.ChatChannelTypeDirect, RefKey: "dm:5:8"}).Error)
	require.NoError(t, g.Create(&database.ChatChannelMember{ChannelID: 60, StaffID: 5, BusinessID: 1}).Error)

	w := do(http.MethodGet, "/b/1/chat/channels", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []database.ChatChannel `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	refKeys := map[string]bool{}
	for _, ch := range resp.Data {
		refKeys[ch.RefKey] = true
	}
	require.True(t, refKeys["role:server"], "role channel materialized in the list")
	require.True(t, refKeys["dm:5:8"], "caller's DM present in the list")
}

// TestChatChannelsListUnreadCounts confirms GetChannels attaches the per-channel
// unread badge map in one aggregate: messages from others count, the caller's own
// do not, and a channel the caller has fully read drops out of the map.
func TestChatChannelsListUnreadCounts(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 5, "server") // caller staff 5 (server)
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	// Pre-seed the role channel id so the lazy resolver re-uses it by ref_key.
	require.NoError(t, g.Create(&database.ChatChannel{ID: 60, BusinessID: 1, Type: database.ChatChannelTypeRole, RefKey: "role:server"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 300, BusinessID: 1, ChannelID: 60, SenderStaffID: 9, Content: "a"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 301, BusinessID: 1, ChannelID: 60, SenderStaffID: 9, Content: "b"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 302, BusinessID: 1, ChannelID: 60, SenderStaffID: 5, Content: "mine"}).Error) // own -> not unread

	w := do(http.MethodGet, "/b/1/chat/channels", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data   []database.ChatChannel `json:"data"`
		Unread map[uint]int64         `json:"unread"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, int64(2), resp.Unread[60], "two messages from others are unread; the caller's own is excluded")

	// After marking read, the channel drops out of the unread map entirely. Decode
	// into a FRESH struct — json.Unmarshal merges into an existing map rather than
	// replacing it, which would otherwise leave the stale count in place.
	w = do(http.MethodPost, "/b/1/chat/channels/60/read", map[string]interface{}{})
	require.Equal(t, http.StatusOK, w.Code)
	w = do(http.MethodGet, "/b/1/chat/channels", nil)
	var after struct {
		Unread map[uint]int64 `json:"unread"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &after))
	_, present := after.Unread[60]
	require.False(t, present, "fully-read channel is absent from the unread map")
}

// TestChatChannelsListPreviews confirms GetChannels attaches each channel's
// newest live message as a bounded preview (snippet + sender + timestamp) in one
// aggregate — the channel-list vitality source next to the unread map.
func TestChatChannelsListPreviews(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 5, "server")
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.ChatChannel{ID: 60, BusinessID: 1, Type: database.ChatChannelTypeRole, RefKey: "role:server"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 300, BusinessID: 1, ChannelID: 60, SenderStaffID: 9, SenderName: "Ana", Content: "older"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 301, BusinessID: 1, ChannelID: 60, SenderStaffID: 9, SenderName: "Ana", Content: "see you at the pass"}).Error)

	w := do(http.MethodGet, "/b/1/chat/channels", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Previews map[uint]struct {
			Snippet    string `json:"snippet"`
			SenderName string `json:"sender_name"`
			CreatedAt  string `json:"created_at"`
		} `json:"previews"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	p, ok := resp.Previews[60]
	require.True(t, ok, "channel with messages carries a preview")
	require.Equal(t, "see you at the pass", p.Snippet, "newest message wins")
	require.Equal(t, "Ana", p.SenderName)
	require.NotEmpty(t, p.CreatedAt)
}

// TestChatAnnouncementAckSSE proves acking an announcement publishes a
// content-free realtime nudge so open "X of Y confirmed" rosters tick up live
// instead of waiting on a manual refresh.
func TestChatAnnouncementAckSSE(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 5, "server")
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	// Ack eligibility requires an active staff row in the audience.
	require.NoError(t, g.Create(&database.Staff{ID: 5, BusinessID: 1, Email: "s5@x.co", Name: "Sam", Role: "server", IsActive: true, InvitedBy: "o"}).Error)
	require.NoError(t, g.Create(&database.Announcement{ID: 40, BusinessID: 1, Title: "secret title", Content: "secret body", AudienceFilter: "all", AuthorStaffID: 1}).Error)

	ackCh, _, cancelAck := events.GetHub().SubscribeWithReplayTopic(1, 0, "chat.announcement_ack")
	defer cancelAck()

	w := do(http.MethodPost, "/b/1/announcements/40/ack", nil)
	require.Equal(t, http.StatusOK, w.Code)

	frame := readFrame(t, ackCh)
	require.Equal(t, "chat.announcement_ack", frame.Type)
	payload := decodeFrame(t, frame.Data)
	require.Contains(t, payload, "announcement_id")
	require.NotContains(t, payload, "title")
	require.NotContains(t, string(frame.Data), "secret", "ack frame is content-free")
}

// TestChatModerateDMRefused proves moderation covers team channels only: a
// chat:moderate holder can delete a role-channel message but is refused (403) on a
// private direct message between two other staff.
func TestChatModerateDMRefused(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 1, "manager") // caller staff 1 (moderator)
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	// A role channel (moderatable) and a DM between staff 7 and 8 (NOT the caller).
	require.NoError(t, g.Create(&database.ChatChannel{ID: 80, BusinessID: 1, Type: database.ChatChannelTypeRole, RefKey: "role:server"}).Error)
	require.NoError(t, g.Create(&database.ChatChannel{ID: 90, BusinessID: 1, Type: database.ChatChannelTypeDirect, RefKey: "dm:7:8"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 400, BusinessID: 1, ChannelID: 80, SenderStaffID: 9, Content: "team spam"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 401, BusinessID: 1, ChannelID: 90, SenderStaffID: 7, Content: "private"}).Error)

	w := do(http.MethodDelete, "/b/1/chat/messages/400", nil)
	require.Equal(t, http.StatusOK, w.Code, "team-channel message is moderatable")

	w = do(http.MethodDelete, "/b/1/chat/messages/401", nil)
	require.Equal(t, http.StatusForbidden, w.Code, "a DM between two other staff must not be moderatable")

	var live int64
	require.NoError(t, g.Model(&database.ChatMessage{}).Where("id = ?", 401).Count(&live).Error)
	require.Equal(t, int64(1), live, "the DM message survived the refused delete")
}

// TestChatMarkReadClampedToNewest proves a client cannot push its read marker past
// the channel's real tail (which would suppress all future unread counts).
func TestChatMarkReadClampedToNewest(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 5, "server") // caller staff 5 (server)
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.ChatChannel{ID: 70, BusinessID: 1, Type: database.ChatChannelTypeRole, RefKey: "role:server"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 100, BusinessID: 1, ChannelID: 70, SenderStaffID: 9, Content: "hi"}).Error)
	require.NoError(t, g.Create(&database.ChatMessage{ID: 101, BusinessID: 1, ChannelID: 70, SenderStaffID: 9, Content: "again"}).Error)

	w := do(http.MethodPost, "/b/1/chat/channels/70/read", map[string]interface{}{"last_read_message_id": 999999})
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data database.ChatRead `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, uint(101), resp.Data.LastReadMessageID, "oversized marker clamped down to the newest live id")
}

// TestChatOperatorChannelsAndPosting: an owner (staffID 0, no staff principal)
// lists the full role/dept channel surface, posts into a role channel (message
// stamped with the business name as sender), and is still denied on staff DMs.
// A manager gets the same operator listing.
func TestChatOperatorChannelsAndPosting(t *testing.T) {
	do, g, cleanup := newChatTestServer(t, 0, "") // owner caller
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1", Name: "Cafe Uno"}).Error)
	require.NoError(t, g.Create(&database.Staff{ID: 5, BusinessID: 1, Email: "a@x.com", Name: "A", Role: "server", InvitedBy: "w"}).Error)
	require.NoError(t, g.Create(&database.Staff{ID: 6, BusinessID: 1, Email: "b@x.com", Name: "B", Role: "kitchen", InvitedBy: "w"}).Error)
	require.NoError(t, g.Create(&database.Position{ID: 9, BusinessID: 1, Name: "Line Cook", Department: "BOH", IsActive: true}).Error)
	// A DM between two staff members the owner must NOT be able to read or post to.
	dm := database.ChatChannel{ID: 40, BusinessID: 1, Type: database.ChatChannelTypeDirect, RefKey: "dm:5:6"}
	require.NoError(t, g.Create(&dm).Error)
	require.NoError(t, g.Create(&database.ChatChannelMember{ChannelID: 40, StaffID: 5, BusinessID: 1, Role: database.ChatMemberRoleMember}).Error)
	require.NoError(t, g.Create(&database.ChatChannelMember{ChannelID: 40, StaffID: 6, BusinessID: 1, Role: database.ChatMemberRoleMember}).Error)

	// Owner listing: every role + dept channel, never the staff DM.
	w := do("GET", "/b/1/chat/channels", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Data []database.ChatChannel `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	refKeys := map[string]uint{}
	for _, ch := range listResp.Data {
		refKeys[ch.RefKey] = ch.ID
	}
	require.Contains(t, refKeys, "role:server")
	require.Contains(t, refKeys, "role:kitchen")
	require.Contains(t, refKeys, "dept:BOH")
	require.NotContains(t, refKeys, "dm:5:6", "owner listing must not expose staff DMs")

	// Owner posts into the server role channel; sender falls back to the business name.
	roleChID := refKeys["role:server"]
	w = do("POST", fmt.Sprintf("/b/1/chat/channels/%d/messages", roleChID), map[string]string{"content": "Team meeting at 4pm"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var postResp struct {
		Data database.ChatMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &postResp))
	require.Equal(t, uint(0), postResp.Data.SenderStaffID)
	require.Equal(t, "Cafe Uno", postResp.Data.SenderName)

	// Owner is still shut out of the staff DM — read and write.
	w = do("GET", "/b/1/chat/channels/40/messages", nil)
	require.Equal(t, http.StatusForbidden, w.Code)
	w = do("POST", "/b/1/chat/channels/40/messages", map[string]string{"content": "hi"})
	require.Equal(t, http.StatusForbidden, w.Code)

	// A manager gets the operator listing too (and may post to any role channel).
	asManager := chatRouterAs(9, "manager")
	w = asManager("POST", fmt.Sprintf("/b/1/chat/channels/%d/messages", roleChID), map[string]string{"content": "noted"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}
