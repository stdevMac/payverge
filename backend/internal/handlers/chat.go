package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// ChatHandler serves the staff-chat read/send surface. Stage 2 covers channels,
// messages and the privacy gate; read-state, announcements, SSE and route wiring
// land in Stage 3 (these handlers are not wired into main.go yet).
type ChatHandler struct{ db *database.DB }

func NewChatHandler(db *database.DB) *ChatHandler { return &ChatHandler{db: db} }

// callerStaffRole reads the acting staff role from context ("" for an owner
// authenticating by wallet, who has no staff role). Mirrors coverage's
// callerStaffID/callerActor context reads.
func callerStaffRole(c *gin.Context) string {
	if v, ok := c.Get("staff_role"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// callerSenderName is the display name stamped on a posted message, best-effort
// from context; empty is acceptable (Stage 3 may enrich from the staff row).
func callerSenderName(c *gin.Context) string {
	if v, ok := c.Get("staff_name"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// chatError maps service sentinels to HTTP responses.
func (h *ChatHandler) chatError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, database.ErrChatChannelNotFound):
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Channel not found")
	default:
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Chat operation failed")
	}
}

// authorizeChannel loads the channel tenant-scoped and runs CanReadChannel,
// writing the 403/404/500 response on failure. It returns the channel and true
// only when the caller may read it.
func (h *ChatHandler) authorizeChannel(c *gin.Context, businessID, channelID uint) (*database.ChatChannel, bool) {
	ch, err := h.db.GetChannel(businessID, channelID)
	if err != nil {
		h.chatError(c, err)
		return nil, false
	}
	ok, err := h.db.CanReadChannel(businessID, ch, callerStaffID(c), callerStaffRole(c))
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to authorize channel")
		return nil, false
	}
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "You do not have access to this channel")
		return nil, false
	}
	return ch, true
}

type chatPostDTO struct {
	Content string `json:"content"`
}

type chatReadDTO struct {
	LastReadMessageID uint `json:"last_read_message_id"`
}

// isChatOperator mirrors the FE's canManageCommunication gate: an owner (no
// staff principal, staffID 0) or a manager staff member. Operators list and
// read/post every role & department channel; DMs stay membership-only.
func isChatOperator(c *gin.Context) bool {
	return callerStaffID(c) == 0 || callerStaffRole(c) == string(database.StaffRoleManager)
}

// GetChannels lists the caller's channels (their role + dept channels + DMs).
// Operators (owner/manager) get the full role & department surface instead of
// just their own slice. Gated by chat:read at the route layer (Stage 3).
func (h *ChatHandler) GetChannels(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID := callerStaffID(c)
	var channels []database.ChatChannel
	var err error
	if isChatOperator(c) {
		channels, err = h.db.ListChannelsForOperator(businessID, staffID)
	} else {
		channels, err = h.db.ListChannelsForStaff(businessID, staffID, callerStaffRole(c))
	}
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list channels")
		return
	}
	// Attach the per-channel unread badge source in ONE grouped aggregate (no N+1).
	// Channels with zero unread are simply absent from the map; the FE treats a
	// missing key as 0. Owners (staffID 0) have no read markers, so this is empty.
	ids := make([]uint, len(channels))
	for i := range channels {
		ids[i] = channels[i].ID
	}
	unread, err := h.db.UnreadCounts(businessID, staffID, ids)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to resolve unread counts")
		return
	}
	// Same no-N+1 contract for the vitality strip: one query resolves every
	// channel's newest live message as a bounded preview.
	previews, err := h.db.LatestMessagePreviews(businessID, ids)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to resolve channel previews")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": channels, "unread": unread, "previews": previews})
}

// GetMessages returns a cursor page of a channel's messages, but only after
// CanReadChannel — a non-member gets 403. Query params: cursor, limit.
func (h *ChatHandler) GetMessages(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	channelID, ok := parseParamUint(c, "channelId")
	if !ok {
		return
	}
	if _, ok := h.authorizeChannel(c, businessID, channelID); !ok {
		return
	}
	var cursor uint
	if cv := c.Query("cursor"); cv != "" {
		if n, err := strconv.ParseUint(cv, 10, 64); err == nil {
			cursor = uint(n)
		}
	}
	var limit int
	if lv := c.Query("limit"); lv != "" {
		if n, err := strconv.Atoi(lv); err == nil {
			limit = n
		}
	}
	msgs, err := h.db.ListMessages(businessID, channelID, cursor, limit)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list messages")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": msgs})
}

// PostMessage appends a message to a channel after CanReadChannel — a non-member
// gets 403. Gated by chat:send at the route layer (Stage 3).
func (h *ChatHandler) PostMessage(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	channelID, ok := parseParamUint(c, "channelId")
	if !ok {
		return
	}
	staffID := callerStaffID(c)
	var in chatPostDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	in.Content = strings.TrimSpace(in.Content)
	if in.Content == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "content is required")
		return
	}
	// Owners (staffID 0) may post too — CanReadChannel admits them to role/dept
	// channels only, so a DM post still 403s below.
	if _, ok := h.authorizeChannel(c, businessID, channelID); !ok {
		return
	}
	senderName := callerSenderName(c)
	if staffID == 0 && senderName == "" {
		// Owner posts speak as the business — there is no staff row to name them.
		if biz, bizErr := database.GetBusinessByID(businessID); bizErr == nil && biz != nil {
			senderName = biz.Name
		}
	}
	msg, err := h.db.PostMessage(businessID, channelID, staffID, senderName, in.Content)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to post message")
		return
	}
	// Content-free realtime nudge: ids only, NEVER the message body. chat:read is
	// universal and the hub fans per-business, so a body here would leak DMs to
	// every staff member; clients refetch via the authorized read path.
	events.GetHub().PublishJSON(businessID, "chat.message", gin.H{"channel_id": channelID, "message_id": msg.ID})
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": msg})
}

// DeleteMessage soft-deletes a message (moderation). Gated by chat:moderate at
// the route layer (manager/owner). Tenant-scoped by business; the underlying row
// is preserved so read markers stay valid.
func (h *ChatHandler) DeleteMessage(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	messageID, ok := parseParamUint(c, "messageId")
	if !ok {
		return
	}
	// Moderation covers team channels only. A chat:moderate holder must not be able
	// to delete a 1:1 direct message between two other staff, so refuse DM deletes
	// up front (the id is unguessable without read access, but close it anyway).
	typ, err := h.db.MessageChannelType(businessID, messageID)
	if err != nil {
		if errors.Is(err, database.ErrChatMessageNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Message not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to delete message")
		return
	}
	if typ == database.ChatChannelTypeDirect {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Direct messages cannot be moderated")
		return
	}
	if err := h.db.DeleteMessage(businessID, messageID); err != nil {
		if errors.Is(err, database.ErrChatMessageNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Message not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to delete message")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type announcementCreateDTO struct {
	Title          string `json:"title"`
	Content        string `json:"content"`
	RequireAck     bool   `json:"require_ack"`
	AudienceFilter string `json:"audience_filter"`
}

// announcementError maps announcement service sentinels to HTTP responses.
func (h *ChatHandler) announcementError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, database.ErrAnnouncementNotFound):
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Announcement not found")
	case errors.Is(err, database.ErrAnnouncementTitleRequired):
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "title is required")
	case errors.Is(err, database.ErrInvalidAudienceFilter):
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "audience_filter must be 'all', 'role:<role>' or 'dept:<dept>'")
	case errors.Is(err, database.ErrAnnouncementNotEligible):
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "You are not in this announcement's audience")
	default:
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Announcement operation failed")
	}
}

// CreateAnnouncement broadcasts a notice. Gated by chat:announce at the route
// layer (manager/owner only). The author is the acting staff id (0 for an owner).
func (h *ChatHandler) CreateAnnouncement(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	var in announcementCreateDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	ann, err := h.db.CreateAnnouncement(businessID, callerStaffID(c), in.Title, in.Content, in.RequireAck, in.AudienceFilter)
	if err != nil {
		h.announcementError(c, err)
		return
	}
	// Content-free realtime nudge: announcement id only, never the title/content;
	// clients refetch via the audience-filtered list.
	events.GetHub().PublishJSON(businessID, "chat.announcement", gin.H{"announcement_id": ann.ID})
	audience, _ := h.db.ListAnnouncementAudienceStaffIDs(businessID, ann.ID)
	notifyStaff(h.db, businessID, excludeStaffID(audience, callerStaffID(c)), "chat.announcement",
		services.PushKeyAnnouncement, services.PushArgs{Title: ann.Title}, "/staff/home?tab=chat")
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": ann})
}

// ListAnnouncements returns announcements for the feed. Managers/owners who can
// chat:announce see every notice (so role:/dept: broadcasts remain manageable);
// line staff see only audience-matched notices. Gated by chat:read at the route.
func (h *ChatHandler) ListAnnouncements(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID := callerStaffID(c)
	// Manager/owner path: same bypass as documents/polls (chat:announce roles).
	anns, err := h.db.ListAnnouncements(businessID, staffID, callerStaffRole(c), callerIsManager(c))
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list announcements")
		return
	}
	// Attach the caller's per-announcement ack state (one query, no N+1) so the feed
	// can hide the Acknowledge button on notices this staff member already confirmed.
	// Acked ids are present->true; a missing id means not-yet-acked.
	ids := make([]uint, len(anns))
	for i := range anns {
		ids[i] = anns[i].ID
	}
	acked, err := h.db.AnnouncementsAckedBy(businessID, staffID, ids)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to resolve ack state")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": anns, "acked": acked})
}

// AckAnnouncement records the caller's acknowledgment (idempotent). Gated by
// chat:read at the route layer; owners (no staff row) cannot ack.
func (h *ChatHandler) AckAnnouncement(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	announcementID, ok := parseParamUint(c, "announcementId")
	if !ok {
		return
	}
	staffID := callerStaffID(c)
	if staffID == 0 {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Only staff can acknowledge announcements")
		return
	}
	ack, err := h.db.AckAnnouncement(businessID, announcementID, staffID)
	if err != nil {
		h.announcementError(c, err)
		return
	}
	// Content-free nudge (id only) so open "X of Y confirmed" rosters tick up
	// live instead of waiting on a manual refresh.
	events.GetHub().PublishJSON(businessID, "chat.announcement_ack", gin.H{"announcement_id": announcementID})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ack})
}

// GetAnnouncementAcks returns the "X of Y confirmed" aggregate + the unacked
// roster. Gated by chat:announce at the route layer — only announce-holders
// (manager/owner) see who has and hasn't read.
func (h *ChatHandler) GetAnnouncementAcks(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	announcementID, ok := parseParamUint(c, "announcementId")
	if !ok {
		return
	}
	status, err := h.db.GetAnnouncementAckStatus(businessID, announcementID)
	if err != nil {
		h.announcementError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": status})
}

// announcementAckBatchMax bounds the ?ids= list a single ack-summary batch call
// may resolve, matching the feed's own bounded page (chatAnnouncementListMax in
// the service).
const announcementAckBatchMax = 200

// GetAnnouncementAckSummaries returns the compact {acked,total_eligible,first
// acker names} aggregate for a comma-separated ?ids= list — the batch endpoint
// that replaces the per-announcement AckRoster poll (the audit's one true HTTP
// N+1). Gated by chat:announce at the route layer — only announce-holders
// (manager/owner) see confirmation counts. The feed calls this ONCE per refresh
// for its VISIBLE require_ack announcements; the full unacked roster still loads
// on expand via GetAnnouncementAcks.
func (h *ChatHandler) GetAnnouncementAckSummaries(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	raw := strings.TrimSpace(c.Query("ids"))
	ids := make([]uint, 0, 16)
	if raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if n, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64); err == nil && n > 0 {
				ids = append(ids, uint(n))
				if len(ids) >= announcementAckBatchMax {
					break
				}
			}
		}
	}
	summaries, err := h.db.GetAnnouncementAckSummaries(businessID, ids, 3)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load ack summaries")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summaries})
}

// UpdateAnnouncement edits a broadcast's title/content/require_ack/audience in
// place (UpdatedAt bumps; the id and ack rows are preserved). Gated by
// chat:announce at the route layer. A content-free nudge re-fans the feed so
// open viewers pick up the edit.
func (h *ChatHandler) UpdateAnnouncement(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	announcementID, ok := parseParamUint(c, "announcementId")
	if !ok {
		return
	}
	var in announcementCreateDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	ann, err := h.db.UpdateAnnouncement(businessID, announcementID, in.Title, in.Content, in.RequireAck, in.AudienceFilter)
	if err != nil {
		h.announcementError(c, err)
		return
	}
	events.GetHub().PublishJSON(businessID, "chat.announcement", gin.H{"announcement_id": ann.ID})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ann})
}

// DeleteAnnouncement removes a broadcast and its ack rows (the announcements
// table has no soft-delete column, so this is a hard delete + an RBAC audit
// entry recording who removed what — see database.DeleteAnnouncement). Deleting
// also stops its ack tracking (the acks cascade in the same transaction). Gated
// by chat:announce at the route layer.
func (h *ChatHandler) DeleteAnnouncement(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	announcementID, ok := parseParamUint(c, "announcementId")
	if !ok {
		return
	}
	if err := h.db.DeleteAnnouncement(businessID, announcementID, callerStaffID(c)); err != nil {
		h.announcementError(c, err)
		return
	}
	events.GetHub().PublishJSON(businessID, "chat.announcement", gin.H{"announcement_id": announcementID})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// MarkRead advances the caller's read marker for a channel. The body
// {last_read_message_id} is optional — an empty/zero id defaults to the channel's
// newest live message ("mark everything read"). Gated by chat:read at the route
// layer plus CanReadChannel (a non-member gets 403).
func (h *ChatHandler) MarkRead(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	channelID, ok := parseParamUint(c, "channelId")
	if !ok {
		return
	}
	staffID := callerStaffID(c)
	if staffID == 0 {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Only staff can mark channels read")
		return
	}
	if _, ok := h.authorizeChannel(c, businessID, channelID); !ok {
		return
	}
	var in chatReadDTO
	_ = c.ShouldBindJSON(&in)
	// Resolve the channel's newest live id and clamp the marker to it: an empty/zero
	// body means "mark everything read", and a client must not be able to push its
	// own marker past the real tail (which would suppress all future unread counts).
	newest, err := h.db.NewestMessageID(businessID, channelID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to resolve read marker")
		return
	}
	lastRead := in.LastReadMessageID
	if lastRead == 0 || lastRead > newest {
		lastRead = newest
	}
	rd, err := h.db.MarkChannelRead(businessID, channelID, staffID, lastRead)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to mark channel read")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rd})
}

// PostDM resolves (or creates) the direct channel between the caller and the path
// staff id, then posts a message if the body carries content — otherwise it just
// returns the channel. Gated by chat:send at the route layer (Stage 3).
func (h *ChatHandler) PostDM(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	targetStaffID, ok := parseParamUint(c, "staffId")
	if !ok {
		return
	}
	staffID := callerStaffID(c)
	if staffID == 0 {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Only staff can send direct messages")
		return
	}
	if targetStaffID == staffID {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Cannot DM yourself")
		return
	}
	ch, err := h.db.GetOrCreateDM(businessID, staffID, targetStaffID)
	if err != nil {
		h.chatError(c, err)
		return
	}
	var in chatPostDTO
	_ = c.ShouldBindJSON(&in)
	in.Content = strings.TrimSpace(in.Content)
	if in.Content == "" {
		// No content: ensure-and-return the channel only.
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"channel": ch}})
		return
	}
	msg, err := h.db.PostMessage(businessID, ch.ID, staffID, callerSenderName(c), in.Content)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to post message")
		return
	}
	// Content-free realtime nudge (ids only, never the DM body) — see PostMessage.
	events.GetHub().PublishJSON(businessID, "chat.message", gin.H{"channel_id": ch.ID, "message_id": msg.ID})
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"channel": ch, "message": msg}})
}
