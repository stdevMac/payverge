package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
)

const (
	aiWaiterSessionTTL    = 24 * time.Hour
	aiWaiterSessionHeader = "X-AI-Waiter-Session"
)

// aiWaiterSessionExpired reports whether a conversation is older than the
// advertised session TTL. Enforces the expires_at we already return to clients.
func aiWaiterSessionExpired(conv *database.AiWaiterConversation) bool {
	return time.Since(conv.CreatedAt) > aiWaiterSessionTTL
}

type createSessionRequest struct {
	TableCode           string `json:"table_code"`
	Mode                string `json:"mode"`
	Language            string `json:"language"`
	ReplaceSessionToken string `json:"replace_session_token"`
}

// resolveAiWaiterSessionToken returns the explicit session token when the
// client supplies one, else falls back to the HttpOnly pv_ai_waiter_session
// cookie. This lets the guest UI authenticate AI-waiter chat/poll via the
// HttpOnly cookie instead of holding a replayable bearer token in JS-readable
// storage (localStorage/sessionStorage) that any XSS could dump and replay to
// impersonate the guest's session, read its history, or place orders. The
// explicit token still wins during the deploy window (backend-first rollout).
func resolveAiWaiterSessionToken(c *gin.Context, explicit string) string {
	if t := strings.TrimSpace(explicit); t != "" {
		return t
	}
	if h := strings.TrimSpace(c.GetHeader(aiWaiterSessionHeader)); h != "" {
		return h
	}
	if ck, err := c.Cookie("pv_ai_waiter_session"); err == nil {
		return strings.TrimSpace(ck)
	}
	return ""
}

// writeAIWaiterSessionCookie mirrors the auth-cookie convention: HttpOnly,
// SameSite=Lax (example.com and api.example.com are same-site), Domain from
// COOKIE_DOMAIN, Secure only in production (so dev http works). Carries the
// session token for the SSE stream, which cannot send a body or auth header.
func writeAIWaiterSessionCookie(c *gin.Context, token string) {
	parts := []string{
		fmt.Sprintf("pv_ai_waiter_session=%s", token),
		"Path=/",
		fmt.Sprintf("Max-Age=%d", int(aiWaiterSessionTTL.Seconds())),
		"HttpOnly",
		"SameSite=Lax",
	}
	if d := strings.TrimSpace(os.Getenv("COOKIE_DOMAIN")); d != "" {
		parts = append(parts, "Domain="+d)
	}
	if utils.IsProduction() {
		parts = append(parts, "Secure")
	}
	c.Writer.Header().Add("Set-Cookie", strings.Join(parts, "; "))
}

func newSessionToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// CreateAIWaiterSession issues a server-side crypto session token (contract C4).
// POST /api/v1/ai-waiter/:businessId/session
func CreateAIWaiterSession(c *gin.Context) {
	identifier := utils.BusinessIdentifierFromParam(c, "businessId")
	if identifier == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}
	business, err := database.GetBusinessByIdOrBusinessId(identifier)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found", "code": ErrCodeBusinessNotFound})
		return
	}
	if respondIfAIWaiterBusinessLocked(c, business) {
		return
	}
	if !business.AiSettings.AiEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "AI Waiter is not enabled for this business"})
		return
	}
	var req createSessionRequest
	_ = c.ShouldBindJSON(&req)

	// Operator sandbox table codes are reserved for authenticated dashboard
	// test-chat endpoints. Reject them on the public guest write path so a
	// guest cannot mint Live-Monitor-hidden sessions or share that namespace.
	if database.IsAiWaiterOperatorTestTableCode(req.TableCode) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid table code"})
		return
	}

	if !allowSessionCreate(middleware.ClientRateLimitKey(c)) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many sessions", "code": ErrCodeRateLimited})
		return
	}

	mode, tableCode, _, scopeErr := validateAIWaiterPublicScope(business, req.Mode, req.TableCode)
	if scopeErr != nil {
		respondAIWaiterPublicScopeError(c, scopeErr)
		return
	}

	language := strings.TrimSpace(req.Language)
	if language == "" || !locales.IsGuestLocale(language) {
		language = business.DefaultLanguage
		if language == "" {
			language = "en"
		}
	}

	if predecessor := strings.TrimSpace(req.ReplaceSessionToken); predecessor != "" {
		_, _ = database.CloseAiWaiterConversationBySession(
			predecessor,
			business.ID,
			services.WaiterNewConversationTransition(language),
		)
	}

	token := newSessionToken()
	if token == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}
	conv, err := database.GetOrCreateAiWaiterConversation(token, business.ID, tableCode, language, mode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	aiName := business.AiSettings.AiName
	if aiName == "" {
		aiName = "Sage"
	}
	// Closed Mode honesty: when the venue is closed for hours or guest
	// ordering is toggled off, the greeting must not offer "help ordering".
	// Concierge mode never takes orders; browseOnly only applies to ordering.
	db := database.GetDB()
	businessOpen := services.BusinessOpenAt(db, business, time.Now())
	guestOrderingOn := business.KitchenEnabled && business.OrdersEnabled && database.IsBusinessOperational(business)
	browseOnly := !businessOpen || !guestOrderingOn
	greeting := services.WaiterGreeting(language, aiName, business.Name, mode, browseOnly)
	if !aiProviderConfigured() {
		// No model: set replies from the menu snapshot. Do not claim AI.
		greeting = services.WaiterScriptedGreeting(language, aiName, business.Name)
	}
	if database.CountAiWaiterMessages(conv.ID) == 0 {
		_ = database.SaveAiWaiterMessage(conv.ID, "assistant", greeting, "")
	}

	writeAIWaiterSessionCookie(c, token)
	// Signed per-device id for the guest AI daily per-device cap.
	ensureAIDeviceCookie(c)

	c.JSON(http.StatusOK, gin.H{
		"session_token": token,
		"greeting":      greeting,
		"expires_at":    time.Now().Add(aiWaiterSessionTTL).UTC().Format(time.RFC3339),
	})
}
