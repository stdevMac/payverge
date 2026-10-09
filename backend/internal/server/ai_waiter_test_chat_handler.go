package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

const (
	aiWaiterOperatorTestCtxKey        = "ai_waiter_operator_test"
	aiWaiterOperatorTestMode          = "concierge"
	aiWaiterOperatorTestSessionPrefix = "optest-"
)

type createAIWaiterTestSessionRequest struct {
	Language string `json:"language"`
}

// CreateAIWaiterTestSession issues an operator-only sandbox session for the
// dashboard test chat. It never writes pv_ai_waiter_session (or any guest
// cookie), never counts against the guest per-IP session create limiter, and
// tags the conversation with AiWaiterOperatorTestTableCode so Live Monitor /
// guest quota / public guest routes ignore it.
//
// POST /api/v1/inside/businesses/:id/ai/test-chat/session
func CreateAIWaiterTestSession(c *gin.Context) {
	business, ok := resolveBusinessFromIDParam(c)
	if !ok {
		return
	}
	if RespondIfBusinessLocked(c, business) {
		return
	}
	if !business.AiSettings.AiEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "AI Waiter is not enabled for this business"})
		return
	}

	var req createAIWaiterTestSessionRequest
	_ = c.ShouldBindJSON(&req)

	language := strings.TrimSpace(req.Language)
	if language == "" || !locales.IsGuestLocale(language) {
		language = business.DefaultLanguage
		if language == "" {
			language = "en"
		}
	}

	token := aiWaiterOperatorTestSessionPrefix + newSessionToken()
	if token == aiWaiterOperatorTestSessionPrefix {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}
	conv, err := database.GetOrCreateAiWaiterConversation(
		token,
		business.ID,
		database.AiWaiterOperatorTestTableCode,
		language,
		aiWaiterOperatorTestMode,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	aiName := business.AiSettings.AiName
	if aiName == "" {
		aiName = "Sage"
	}
	// Sandbox is always concierge browse-only — never offers ordering.
	greeting := services.WaiterGreeting(language, aiName, business.Name, aiWaiterOperatorTestMode, true)
	if database.CountAiWaiterMessages(conv.ID) == 0 {
		_ = database.SaveAiWaiterMessage(conv.ID, "assistant", greeting, "")
	}

	// Intentionally no Set-Cookie. Operator test chat carries the token in the
	// JSON body only so it cannot overwrite a real guest's pv_ai_waiter_session.
	c.JSON(http.StatusOK, gin.H{
		"session_token": token,
		"greeting":      greeting,
		"expires_at":    time.Now().Add(aiWaiterSessionTTL).UTC().Format(time.RFC3339),
	})
}

// HandleAIWaiterTestChat runs the live waiter path under an operator sandbox
// session. Requires hybrid auth + ai_waiter:write (wired in main.go). Forces
// concierge + reserved table code and never reads or writes the guest session
// cookie.
//
// POST /api/v1/inside/businesses/:id/ai/test-chat
func HandleAIWaiterTestChat(c *gin.Context) {
	c.Set(aiWaiterOperatorTestCtxKey, true)
	// HandleAIWaiter resolves :businessId; dashboard routes use :id.
	if c.Param("businessId") == "" {
		c.Params = append(c.Params, gin.Param{Key: "businessId", Value: c.Param("id")})
	}
	HandleAIWaiter(c)
}

func isAIWaiterOperatorTestRequest(c *gin.Context) bool {
	v, ok := c.Get(aiWaiterOperatorTestCtxKey)
	if !ok {
		return false
	}
	flag, _ := v.(bool)
	return flag
}
