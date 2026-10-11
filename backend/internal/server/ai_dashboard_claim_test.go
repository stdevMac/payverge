package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAiConversation_ClaimAuthorshipFields_RoundTrip(t *testing.T) {
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "claim-roundtrip", true)
	conv := createAIWaiterConversation(t, biz.ID, "claim-roundtrip-sess")

	staffID := uint(42)
	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).
		Where("id = ?", conv.ID).
		Updates(map[string]interface{}{
			"claimed_by_staff_id": staffID,
			"claimed_by_name":     "Ana",
			"claimed_by_role":     "server",
			"claimed_at":          now,
		}).Error)

	var reloaded database.AiWaiterConversation
	require.NoError(t, database.GetDB().First(&reloaded, conv.ID).Error)
	require.NotNil(t, reloaded.ClaimedByStaffID)
	assert.EqualValues(t, staffID, *reloaded.ClaimedByStaffID)
	assert.Equal(t, "Ana", reloaded.ClaimedByName)
	assert.Equal(t, "server", reloaded.ClaimedByRole)
	require.NotNil(t, reloaded.ClaimedAt)

	msg := database.AiWaiterMessage{
		ConversationID: conv.ID, Role: "assistant", Content: "On my way",
		AuthorStaffID: &staffID, AuthorName: "Ana", AuthorRole: "server", CreatedAt: now,
	}
	require.NoError(t, database.GetDB().Create(&msg).Error)
	var rmsg database.AiWaiterMessage
	require.NoError(t, database.GetDB().First(&rmsg, msg.ID).Error)
	require.NotNil(t, rmsg.AuthorStaffID)
	assert.Equal(t, "server", rmsg.AuthorRole)
}

// aiStaffRouter builds a router that authenticates as the given staff member.
func aiStaffRouter(staffID uint, role database.StaffRole, businessID uint) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_id", staffID)
		c.Set("staff_name", string(role)+"-name")
		c.Set("staff_role", string(role))
		c.Set("staff_business_id", businessID)
		c.Next()
	})
	r.POST("/businesses/:id/ai/conversations/:convId/claim", RoleBasedAccessMiddleware("ai_waiter:reply"), ClaimAiConversation)
	r.POST("/businesses/:id/ai/conversations/:convId/release", RoleBasedAccessMiddleware("ai_waiter:reply"), ReleaseAiConversation)
	r.POST("/businesses/:id/ai/conversations/:convId/reply", RoleBasedAccessMiddleware("ai_waiter:reply"), PostAiReply)
	r.POST("/businesses/:id/ai/conversations/:convId/pause", RoleBasedAccessMiddleware("ai_waiter:reply"), ToggleAiPause)
	r.POST("/businesses/:id/ai/conversations/:convId/close", RoleBasedAccessMiddleware("ai_waiter:reply"), CloseAiConversation)
	r.GET("/businesses/:id/ai/conversations", RoleBasedAccessMiddleware("ai_waiter:read"), GetAiConversations)
	r.GET("/businesses/:id/ai/conversations/:convId/messages", RoleBasedAccessMiddleware("ai_waiter:read"), GetAiConversationMessages)
	return r
}

func claimPath(bizID, convID uint) string {
	return fmt.Sprintf("/businesses/%d/ai/conversations/%d/claim", bizID, convID)
}

// aiOwnerRouter builds a router that authenticates as the business owner
// (no staff_* context values → aiActorIdentity yields the "owner" principal).
func aiOwnerRouter() *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Next()
	})
	r.POST("/businesses/:id/ai/conversations/:convId/claim", ClaimAiConversation)
	return r
}

// A fresh owner claim (NULL claimed_by_staff_id + fresh claimed_at) must not be
// silently stolen by front-line staff: the freshness of claimed_at — not the
// staff id — marks the claim as held.
func TestClaim_FreshOwnerClaimNotStolenByFrontline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "claim-owner-steal", true)
	conv := createAIWaiterConversation(t, biz.ID, "claim-owner-steal-sess")
	a := createAIWaiterStaff(t, biz.ID, database.StaffRoleServer, "a@ex.com")

	// Owner claims the conversation.
	wo := performAIWaiterRequest(t, aiOwnerRouter(), http.MethodPost, claimPath(biz.ID, conv.ID), nil)
	require.Equal(t, http.StatusOK, wo.Code, wo.Body.String())

	var held database.AiWaiterConversation
	require.NoError(t, database.GetDB().First(&held, conv.ID).Error)
	require.Nil(t, held.ClaimedByStaffID, "owner claims carry a NULL staff id")
	require.NotNil(t, held.ClaimedAt)
	require.Equal(t, "owner", held.ClaimedByRole)

	// Front-line staff must NOT steal the fresh owner claim.
	ws := performAIWaiterRequest(t, aiStaffRouter(a.ID, database.StaffRoleServer, biz.ID), http.MethodPost, claimPath(biz.ID, conv.ID), nil)
	require.Equal(t, http.StatusConflict, ws.Code, "fresh owner claim must not be stolen by front-line staff")

	var reloaded database.AiWaiterConversation
	require.NoError(t, database.GetDB().First(&reloaded, conv.ID).Error)
	assert.Nil(t, reloaded.ClaimedByStaffID, "owner claim must survive the steal attempt")
	assert.Equal(t, "owner", reloaded.ClaimedByRole)

	// Once the owner claim goes idle past the TTL, staff may claim it.
	stale := time.Now().Add(-aiClaimIdleTTL - time.Minute)
	require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).Update("claimed_at", stale).Error)
	ws2 := performAIWaiterRequest(t, aiStaffRouter(a.ID, database.StaffRoleServer, biz.ID), http.MethodPost, claimPath(biz.ID, conv.ID), nil)
	require.Equal(t, http.StatusOK, ws2.Code, "stale owner claim is re-claimable")
}

// The lazy TTL sweep in GetAiConversations must also release stale OWNER
// claims (NULL claimed_by_staff_id) — otherwise the conversation stays paused
// forever.
func TestGetConversations_LazyExpiresStaleOwnerClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "lazy-owner-expiry", true)
	conv := createAIWaiterConversation(t, biz.ID, "lazy-owner-expiry-sess")
	a := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "m@ex.com")
	router := aiStaffRouter(a.ID, database.StaffRoleManager, biz.ID)

	stale := time.Now().Add(-aiClaimIdleTTL - time.Minute)
	require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).
		Updates(map[string]interface{}{
			"claimed_by_staff_id": nil,
			"claimed_by_name":     "Owner",
			"claimed_by_role":     "owner",
			"claimed_at":          stale,
			"is_paused":           true,
		}).Error)

	w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/ai/conversations", biz.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp GetAiConversationsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Conversations, 1)
	assert.Empty(t, resp.Conversations[0].ClaimedByRole, "stale owner claim cleared on list")
	assert.Nil(t, resp.Conversations[0].ClaimedAt, "stale owner claimed_at cleared on list")
	assert.False(t, resp.Conversations[0].IsPaused, "AI resumed on lazy expiry of an owner claim")
}

func TestClaim_AtomicSingleWinner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "claim-atomic", true)
	conv := createAIWaiterConversation(t, biz.ID, "claim-atomic-sess")
	a := createAIWaiterStaff(t, biz.ID, database.StaffRoleServer, "a@ex.com")
	b := createAIWaiterStaff(t, biz.ID, database.StaffRoleServer, "b@ex.com")

	wa := performAIWaiterRequest(t, aiStaffRouter(a.ID, database.StaffRoleServer, biz.ID), http.MethodPost, claimPath(biz.ID, conv.ID), nil)
	require.Equal(t, http.StatusOK, wa.Code)

	wb := performAIWaiterRequest(t, aiStaffRouter(b.ID, database.StaffRoleServer, biz.ID), http.MethodPost, claimPath(biz.ID, conv.ID), nil)
	require.Equal(t, http.StatusConflict, wb.Code, "second claimer loses")

	var reloaded database.AiWaiterConversation
	require.NoError(t, database.GetDB().First(&reloaded, conv.ID).Error)
	require.NotNil(t, reloaded.ClaimedByStaffID)
	assert.EqualValues(t, a.ID, *reloaded.ClaimedByStaffID)
	assert.True(t, reloaded.IsPaused, "claim pauses the AI")
}

func TestClaim_IdleExpiryAllowsReclaim(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "claim-idle", true)
	conv := createAIWaiterConversation(t, biz.ID, "claim-idle-sess")
	a := createAIWaiterStaff(t, biz.ID, database.StaffRoleServer, "a@ex.com")
	b := createAIWaiterStaff(t, biz.ID, database.StaffRoleServer, "b@ex.com")

	require.Equal(t, http.StatusOK,
		performAIWaiterRequest(t, aiStaffRouter(a.ID, database.StaffRoleServer, biz.ID), http.MethodPost, claimPath(biz.ID, conv.ID), nil).Code)

	// Backdate the claim past the idle TTL.
	stale := time.Now().Add(-aiClaimIdleTTL - time.Minute)
	require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).Update("claimed_at", stale).Error)

	wb := performAIWaiterRequest(t, aiStaffRouter(b.ID, database.StaffRoleServer, biz.ID), http.MethodPost, claimPath(biz.ID, conv.ID), nil)
	require.Equal(t, http.StatusOK, wb.Code, "stale claim is re-claimable")
}

func TestManagerCanOverrideClaim(t *testing.T) {
	// Decision #4: managers no longer silently seize — they must pass steal=true.
	// Without the flag the claim stays with the original holder (409 steal_required).
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "claim-override", true)
	conv := createAIWaiterConversation(t, biz.ID, "claim-override-sess")
	a := createAIWaiterStaff(t, biz.ID, database.StaffRoleServer, "a@ex.com")
	mgr := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "m@ex.com")

	require.Equal(t, http.StatusOK,
		performAIWaiterRequest(t, aiStaffRouter(a.ID, database.StaffRoleServer, biz.ID), http.MethodPost, claimPath(biz.ID, conv.ID), nil).Code)

	// Silent claim attempt → 409 steal required, holder unchanged.
	wm := performAIWaiterRequest(t, aiStaffRouter(mgr.ID, database.StaffRoleManager, biz.ID), http.MethodPost, claimPath(biz.ID, conv.ID), nil)
	require.Equal(t, http.StatusConflict, wm.Code, "manager cannot silently seize")
	var held database.AiWaiterConversation
	require.NoError(t, database.GetDB().First(&held, conv.ID).Error)
	require.NotNil(t, held.ClaimedByStaffID)
	assert.EqualValues(t, a.ID, *held.ClaimedByStaffID)

	// Explicit steal → 200 + audit.
	wm2 := performAIWaiterRequest(t, aiStaffRouter(mgr.ID, database.StaffRoleManager, biz.ID), http.MethodPost, claimPath(biz.ID, conv.ID), map[string]interface{}{"steal": true})
	require.Equal(t, http.StatusOK, wm2.Code, "manager steals with explicit flag")
	require.NoError(t, database.GetDB().First(&held, conv.ID).Error)
	require.NotNil(t, held.ClaimedByStaffID)
	assert.EqualValues(t, mgr.ID, *held.ClaimedByStaffID)

	var audits int64
	require.NoError(t, database.GetDB().Model(&database.RBACAuditLog{}).
		Where("business_id = ? AND action = ?", biz.ID, database.RBACActionClaimStolen).
		Count(&audits).Error)
	assert.EqualValues(t, 1, audits)
}

func TestReply_RecordsAuthorship_AndRequiresClaim(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "reply-auth", true)
	conv := createAIWaiterConversation(t, biz.ID, "reply-auth-sess")
	a := createAIWaiterStaff(t, biz.ID, database.StaffRoleServer, "a@ex.com")
	replyPath := fmt.Sprintf("/businesses/%d/ai/conversations/%d/reply", biz.ID, conv.ID)
	router := aiStaffRouter(a.ID, database.StaffRoleServer, biz.ID)

	// Reply without claim → 409.
	w1 := performAIWaiterRequest(t, router, http.MethodPost, replyPath, map[string]string{"content": "hi"})
	require.Equal(t, http.StatusConflict, w1.Code, "front-line must claim before replying")

	// Claim, then reply → 200 + authorship.
	require.Equal(t, http.StatusOK,
		performAIWaiterRequest(t, router, http.MethodPost, claimPath(biz.ID, conv.ID), nil).Code)
	w2 := performAIWaiterRequest(t, router, http.MethodPost, replyPath, map[string]string{"content": "hi there"})
	require.Equal(t, http.StatusOK, w2.Code)

	var msgs []database.AiWaiterMessage
	require.NoError(t, database.GetDB().Where("conversation_id = ? AND content = ?", conv.ID, "hi there").Find(&msgs).Error)
	require.Len(t, msgs, 1)
	require.NotNil(t, msgs[0].AuthorStaffID)
	assert.EqualValues(t, a.ID, *msgs[0].AuthorStaffID)
	assert.Equal(t, "server", msgs[0].AuthorRole)
	assert.Equal(t, "assistant", msgs[0].Role)
}

// Managers also must claim before replying — no silent bypass (decision #4).
func TestReply_ManagerRequiresClaim(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "reply-mgr-claim", true)
	conv := createAIWaiterConversation(t, biz.ID, "reply-mgr-claim-sess")
	mgr := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "mgr@ex.com")
	replyPath := fmt.Sprintf("/businesses/%d/ai/conversations/%d/reply", biz.ID, conv.ID)
	router := aiStaffRouter(mgr.ID, database.StaffRoleManager, biz.ID)

	w1 := performAIWaiterRequest(t, router, http.MethodPost, replyPath, map[string]string{"content": "hi"})
	require.Equal(t, http.StatusConflict, w1.Code, "manager must claim before replying")

	require.Equal(t, http.StatusOK,
		performAIWaiterRequest(t, router, http.MethodPost, claimPath(biz.ID, conv.ID), nil).Code)
	w2 := performAIWaiterRequest(t, router, http.MethodPost, replyPath, map[string]string{"content": "claimed"})
	require.Equal(t, http.StatusOK, w2.Code)
}

// TestReply_RejectsOverlongContent guards the staff-reply byte cap: an
// operator-authored reply is persisted and pushed over SSE to the guest, so it
// must honor the same maxAIWaiterMessageBytes ceiling as the guest ingest path
// rather than storing/broadcasting an unbounded blob.
func TestReply_RejectsOverlongContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "reply-cap", true)
	conv := createAIWaiterConversation(t, biz.ID, "reply-cap-sess")
	// Manager must claim first (decision #4); isolate the length check after.
	a := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "cap@ex.com")
	replyPath := fmt.Sprintf("/businesses/%d/ai/conversations/%d/reply", biz.ID, conv.ID)
	router := aiStaffRouter(a.ID, database.StaffRoleManager, biz.ID)
	require.Equal(t, http.StatusOK,
		performAIWaiterRequest(t, router, http.MethodPost, claimPath(biz.ID, conv.ID), nil).Code)

	overlong := strings.Repeat("x", maxAIWaiterMessageBytes+1)
	w := performAIWaiterRequest(t, router, http.MethodPost, replyPath, map[string]string{"content": overlong})
	require.Equal(t, http.StatusBadRequest, w.Code, "overlong staff reply must be rejected")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.AiWaiterMessage{}).
		Where("conversation_id = ?", conv.ID).Count(&count).Error)
	assert.Zero(t, count, "no message should be persisted when the reply is rejected")
}

func TestRelease_ResumesAI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "release", true)
	conv := createAIWaiterConversation(t, biz.ID, "release-sess")
	a := createAIWaiterStaff(t, biz.ID, database.StaffRoleServer, "a@ex.com")
	router := aiStaffRouter(a.ID, database.StaffRoleServer, biz.ID)
	releasePath := fmt.Sprintf("/businesses/%d/ai/conversations/%d/release", biz.ID, conv.ID)

	require.Equal(t, http.StatusOK,
		performAIWaiterRequest(t, router, http.MethodPost, claimPath(biz.ID, conv.ID), nil).Code)
	require.Equal(t, http.StatusOK,
		performAIWaiterRequest(t, router, http.MethodPost, releasePath, nil).Code)

	var reloaded database.AiWaiterConversation
	require.NoError(t, database.GetDB().First(&reloaded, conv.ID).Error)
	assert.Nil(t, reloaded.ClaimedByStaffID)
	assert.False(t, reloaded.IsPaused, "release resumes the AI")
}

func TestGetConversations_LazyExpiresStaleClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "lazy-expiry", true)
	conv := createAIWaiterConversation(t, biz.ID, "lazy-expiry-sess")
	a := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "m@ex.com")
	router := aiStaffRouter(a.ID, database.StaffRoleManager, biz.ID)

	require.Equal(t, http.StatusOK,
		performAIWaiterRequest(t, router, http.MethodPost, claimPath(biz.ID, conv.ID), nil).Code)
	stale := time.Now().Add(-aiClaimIdleTTL - time.Minute)
	require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).Update("claimed_at", stale).Error)

	w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/ai/conversations", biz.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp GetAiConversationsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Conversations, 1)
	assert.Nil(t, resp.Conversations[0].ClaimedByStaffID, "stale claim cleared on list")
	assert.False(t, resp.Conversations[0].IsPaused, "AI resumed on lazy expiry")
}
