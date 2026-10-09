package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/aiwaiterevents"
	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var jsonStdMarshal = json.Marshal

var aiWaiterStreamKeepalive = 15 * time.Second

// aiWaiterStreamBusinessRecheck bounds how often a held stream re-reads the
// business lock and AI toggle. Overridable in tests.
var aiWaiterStreamBusinessRecheck = 60 * time.Second

func aiWaiterStreamBearer(c *gin.Context) string {
	values := c.Request.Header.Values("Authorization")
	if len(values) != 1 {
		return ""
	}
	const prefix = "Bearer "
	header := values[0]
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	token := strings.TrimPrefix(header, prefix)
	if token == "" || len(token) > 512 || strings.TrimSpace(token) != token {
		return ""
	}
	for _, r := range token {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == '~' {
			continue
		}
		return ""
	}
	return token
}

func HandleAIWaiterStream(c *gin.Context) {
	businessIdentifier := utils.BusinessIdentifierFromParam(c, "businessId")
	if businessIdentifier == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}
	business, err := database.GetBusinessByIdOrBusinessId(businessIdentifier)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found", "code": ErrCodeBusinessNotFound})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		}
		return
	}

	if respondIfAIWaiterBusinessLocked(c, business) {
		return
	}
	if !business.AiSettings.AiEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "AI Waiter is not enabled for this business"})
		return
	}

	// The SSE stream is bound to the tab's in-memory AI-waiter bearer. Never
	// accept the origin-wide cookie or a URL token here: another table opened in
	// the same browser can overwrite that cookie and reconnect to the wrong
	// conversation.
	sessionToken := aiWaiterStreamBearer(c)
	if sessionToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization bearer is required"})
		return
	}

	var conv database.AiWaiterConversation
	if err := database.GetDB().
		Where("session_id = ? AND business_id = ?", sessionToken, business.ID).
		First(&conv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": ErrCodeSessionUnknown, "error": "Unknown session"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate session"})
		return
	}
	if aiWaiterSessionExpired(&conv) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "session_expired", "error": "Session expired"})
		return
	}
	if database.IsAiWaiterOperatorTestTableCode(conv.TableCode) {
		// Operator sandbox sessions are not guest streams.
		c.JSON(http.StatusNotFound, gin.H{"code": ErrCodeSessionUnknown, "error": "Unknown session"})
		return
	}

	hub := aiwaiterevents.GetHub()
	ch, cancel, ok := hub.SubscribeLimited(conv.ID, middleware.ClientRateLimitKey(c))
	if !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many live connections", "code": "sse_connection_limit"})
		return
	}
	defer cancel()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	writeAIWaiterSSE(c, "connected", map[string]any{"session": "ok"})
	c.Writer.Flush()

	ticker := time.NewTicker(aiWaiterStreamKeepalive)
	defer ticker.Stop()

	ctx := c.Request.Context()
	lastBusinessCheck := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, open := <-ch:
			if !open {
				return
			}
			writeAIWaiterSSE(c, "message.created", evt)
			c.Writer.Flush()
		case <-ticker.C:
			// Session expiry is a clock check and runs every tick. The business
			// row is re-read at most once per aiWaiterStreamBusinessRecheck so
			// held streams do not turn into per-tick DB polling.
			checkBusiness := time.Since(lastBusinessCheck) >= aiWaiterStreamBusinessRecheck
			if checkBusiness {
				lastBusinessCheck = time.Now()
			}
			if code, stillAllowed := aiWaiterStreamStillAllowed(&conv, business.ID, checkBusiness); !stillAllowed {
				writeAIWaiterSSE(c, "error", map[string]string{"code": code})
				c.Writer.Flush()
				return
			}
			fmt.Fprint(c.Writer, ": ping\n\n")
			c.Writer.Flush()
		}
	}
}

// aiWaiterStreamStillAllowed re-checks a live SSE stream. code is the terminal
// error frame to send when ok is false. A database error leaves the stream up
// (ok=true) so a blip does not drop the guest.
func aiWaiterStreamStillAllowed(conv *database.AiWaiterConversation, businessID uint, checkBusiness bool) (code string, ok bool) {
	if aiWaiterSessionExpired(conv) {
		return "session_expired", false
	}
	if !checkBusiness {
		return "", true
	}
	var business database.Business
	err := database.GetDB().Model(&database.Business{}).
		Select("id", "is_active", "is_demo", "closed_at", "ai_ai_enabled").
		Where("id = ?", businessID).
		First(&business).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "business_unavailable", false
		}
		return "", true
	}
	if !business.IsActive || !database.IsBusinessOperational(&business) {
		return "business_unavailable", false
	}
	if !business.AiSettings.AiEnabled {
		return "ai_disabled", false
	}
	return "", true
}

func writeAIWaiterSSE(c *gin.Context, event string, payload any) {
	raw, err := jsonMarshalAIWaiter(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, raw)
}

func jsonMarshalAIWaiter(v any) (string, error) {
	b, err := jsonStdMarshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// publishAiWaiterMessage fans the just-saved message to the session's SSE hub.
// It takes the message fields the caller already holds from the save (id, role,
// content, createdAt) and publishes directly with NO DB read. The previous
// implementation re-queried the conversation's newest row, which both wasted a
// query and raced: a concurrent newer message could be fetched and published
// instead of the one the caller just persisted (finding SSE-02).
func publishAiWaiterMessage(conv *database.AiWaiterConversation, id uint, role, content string, createdAt time.Time) {
	var response *assistantcontract.Response
	if role == "assistant" {
		projected := legacyAiWaiterResponseV2(id, content, "en", false)
		content = projected.Answer.Content
		response = &projected
	}
	publishAiWaiterMessageV2(conv, id, role, content, createdAt, response)
}

func publishAiWaiterMessageV2(conv *database.AiWaiterConversation, id uint, role, content string, createdAt time.Time, response *assistantcontract.Response) {
	if conv == nil || conv.SessionID == "" {
		return
	}
	if role != "assistant" {
		response = nil
	}
	aiwaiterevents.GetHub().PublishMessage(conv.ID, aiwaiterevents.MessageEvent{
		ID:         id,
		Role:       role,
		Content:    content,
		CreatedAt:  createdAt.Unix(),
		ResponseV2: response,
	})
}
