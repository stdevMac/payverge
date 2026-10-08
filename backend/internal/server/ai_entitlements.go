package server

import (
	"net/http"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// ErrCodeAINotConfigured is returned (503) by LLM-only routes when no LLM
// provider is configured on this server. The operator action is "configure a
// provider".
//
// Scope (deliberately narrow): AI menu tools (extraction / wizard / image
// generation), marketing image generation, the ops assistant and the director
// ask/ask-stream chat. The guest AI waiter keeps its deterministic
// menu-snapshot fallback, the director's data endpoints (briefing, insights,
// threads, actions) keep working, and the marketing caption endpoint keeps its
// graceful 200 {ai_available:false} answer.
const ErrCodeAINotConfigured = middleware.PublicServerErrorAINotConfigured

// aiNotConfiguredMessage is the operator-facing explanation shipped with
// every ai_not_configured response; ErrorSanitizer allowlists the code and
// writes this fixed message (decision D-3).
const aiNotConfiguredMessage = middleware.AINotConfiguredMessage

// aiProviderConfigured reports whether an LLM provider was wired at startup
// (OPENROUTER_API_KEY or LLM_BASE_URL resolved at boot → SetAIService
// non-nil; see config.AIProviderConfigured).
func aiProviderConfigured() bool {
	return config.AIProviderConfigured()
}

// respondAINotConfigured writes the canonical 503 ai_not_configured body.
func respondAINotConfigured(c *gin.Context) {
	middleware.AllowPublicServerError(c, ErrCodeAINotConfigured)
	c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
		"error":   ErrCodeAINotConfigured,
		"code":    ErrCodeAINotConfigured,
		"message": aiNotConfiguredMessage,
	})
}

// aiWaiterMode tells guest clients which AI waiter they will get: "llm" when
// an LLM provider is configured, "basic" for the deterministic menu-snapshot
// fallback. Additive to ai_available, which stays operational && toggle.
func aiWaiterMode() string {
	if aiProviderConfigured() {
		return "llm"
	}
	return "basic"
}
