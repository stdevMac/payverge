package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/agents/ops_guides"
	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"github.com/gin-gonic/gin"
)

// OpsAssistantServiceAPI is the handler-facing Ops Assistant slice.
type OpsAssistantServiceAPI interface {
	Ask(ctx context.Context, req agents.OpsAskRequest) (*agents.OpsAskResult, error)
	ListMessages(businessID, threadID uint) ([]database.OpsAssistantMessage, error)
}

var opsAssistantService OpsAssistantServiceAPI

func SetOpsAssistantService(s OpsAssistantServiceAPI) { opsAssistantService = s }

func ensureOpsAssistantEnabled(c *gin.Context) bool {
	if opsAssistantService == nil {
		// The ops assistant is only constructed when an LLM provider is
		// configured (see main.go), so nil means "no provider".
		respondAINotConfigured(c)
		return false
	}
	return true
}

type opsAskBody struct {
	Message         string `json:"message" binding:"required"`
	ThreadID        *uint  `json:"thread_id"`
	Locale          string `json:"locale"`
	ActiveTab       string `json:"active_tab"`
	ClientRequestID string `json:"client_request_id"`
}

// AskOpsAssistant POST /api/v1/inside/businesses/:id/assistant/ask
func AskOpsAssistant(c *gin.Context) {
	if !ensureOpsAssistantEnabled(c) {
		return
	}
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}
	var body opsAskBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "invalid_request")
		return
	}

	// Per-business daily USD ceiling backstop (each ask fans out to several model
	// calls). Keyed on the real business id.
	if aiOverDollarBudget(business.ID) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "AI assistant is busy, please try again later", "code": "over_budget"})
		return
	}

	env := buildOpsToolEnv(c, business)
	res, err := opsAssistantService.Ask(c.Request.Context(), agents.OpsAskRequest{
		BusinessID:      business.ID,
		Message:         body.Message,
		ThreadID:        body.ThreadID,
		Locale:          body.Locale,
		ActiveTab:       body.ActiveTab,
		ClientRequestID: body.ClientRequestID,
		Env:             env,
		Access:          buildOpsAccessSnapshot(c, business),
	})
	if err != nil {
		if errors.Is(err, database.ErrOpsAssistantRequestInFlight) {
			c.JSON(http.StatusConflict, gin.H{"error": "Request already in progress", "code": "ai_job_conflict"})
			return
		}
		logger.Logger.Warnf("ops assistant ask failed (business %d): %v", business.ID, err)
		RespondAIError(c, err)
		return
	}
	assistantMessage := res.AssistantMessage
	assistantMessage.StructuredResponse = ""
	payload := gin.H{
		"thread":            res.Thread,
		"assistant_message": assistantMessage,
		"response":          res.Response,
		"usage":             res.Usage,
	}
	payload["response_v2"] = res.ResponseV2
	c.JSON(http.StatusOK, payload)
}

func buildOpsToolEnv(c *gin.Context, business *database.Business) agents.ToolEnv {
	isStaff := false
	staffRole := ""
	if tokenType, exists := c.Get("token_type"); exists && tokenType == "staff" {
		isStaff = true
		if r, ok := c.Get("role"); ok {
			staffRole, _ = r.(string)
		}
	}
	canWrite := callerHasPermission(c, string(PermAssistantWrite))
	return agents.ToolEnv{
		StaffRole:   staffRole,
		IsStaffUser: isStaff,
		CanWrite:    canWrite,
		IsSuspended: !database.IsBusinessOperational(business),
	}
}

func buildOpsAccessSnapshot(c *gin.Context, business *database.Business) agents.OpsAccessSnapshot {
	checker, allAccess := ResolveContextEventPermissions(c)
	effective := make(map[string]bool, len(ops_guides.RequiredGuideIDs)+1)
	catalog := ops_guides.NewDefaultCatalog()
	requiredPermissions := make(map[string]struct{}, len(ops_guides.RequiredGuideIDs)+1)
	for _, guideID := range ops_guides.RequiredGuideIDs {
		guide, ok := catalog.Get("en", guideID)
		if !ok || strings.TrimSpace(guide.RequiredPermission) == "" {
			continue
		}
		requiredPermissions[guide.RequiredPermission] = struct{}{}
	}
	requiredPermissions["plugins:read"] = struct{}{}
	for permission := range requiredPermissions {
		if allAccess || (checker != nil && checker(permission)) {
			effective[permission] = true
		}
	}
	suspended := !database.IsBusinessOperational(business)
	return agents.OpsAccessSnapshot{
		EffectivePermissions: effective,
		HiddenGuideIDs:       buildOpsHiddenGuideIDs(c, business, catalog, checker, allAccess),
		Suspended:            suspended,
	}
}

func buildOpsHiddenGuideIDs(c *gin.Context, business *database.Business, catalog *ops_guides.Catalog, checker func(string) bool, allAccess bool) map[string]bool {
	hidden := map[string]bool{}
	tokenType, _ := c.Get("token_type")
	if tokenType != "staff" || catalog == nil {
		return hidden
	}
	suspended := !database.IsBusinessOperational(business)
	for _, guideID := range ops_guides.RequiredGuideIDs {
		guide, ok := catalog.Get("en", guideID)
		if !ok {
			continue
		}
		tab, ok := ops_guides.LookupTab(guide.Tab)
		if !ok {
			hidden[guide.ID] = true
			continue
		}
		if !allAccess && (checker == nil || !checker(tab.RequiredPermission)) {
			hidden[guide.ID] = true
			continue
		}
		if suspended && tab.StaffLockBehavior == "hidden" && guide.Tab != "settings" {
			hidden[guide.ID] = true
		}
	}
	return hidden
}

// GetOpsAssistantThreadMessages GET .../assistant/threads/:threadId/messages
func GetOpsAssistantThreadMessages(c *gin.Context) {
	if !ensureOpsAssistantEnabled(c) {
		return
	}
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}
	threadID, err := strconv.ParseUint(c.Param("threadId"), 10, 64)
	if err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "invalid_thread")
		return
	}
	msgs, err := opsAssistantService.ListMessages(business.ID, uint(threadID))
	if err != nil {
		logger.Logger.Warnf("ops assistant list messages failed (business %d, thread %d): %v", business.ID, threadID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load messages"})
		return
	}
	history := make([]opsAssistantHistoryMessage, 0, len(msgs))
	for _, message := range msgs {
		item := opsAssistantHistoryMessage{
			ID: message.ID, ThreadID: message.ThreadID, BusinessID: message.BusinessID,
			RequestID: message.RequestID, Role: message.Role, Locale: message.Locale,
			Content: message.Content, ModelName: message.ModelName, LatencyMs: message.LatencyMs,
			Feedback: message.Feedback, CreatedAt: message.CreatedAt,
		}
		if message.Role == database.OpsAssistantRoleAssistant {
			legacy, responseV2, restored := agents.RestoreOpsResponse(message.ID, business.ID, message.Content, message.StructuredResponse)
			if restored {
				item.StructuredResponse = &legacy
				item.ResponseV2 = &responseV2
			}
		}
		history = append(history, item)
	}
	c.JSON(http.StatusOK, gin.H{"messages": history})
}

type opsAssistantHistoryMessage struct {
	ID                 uint                             `json:"id"`
	ThreadID           uint                             `json:"thread_id"`
	BusinessID         uint                             `json:"business_id"`
	RequestID          *uint                            `json:"request_id,omitempty"`
	Role               database.OpsAssistantMessageRole `json:"role"`
	Locale             string                           `json:"locale"`
	Content            string                           `json:"content"`
	StructuredResponse *agents.StructuredResponse       `json:"structured_response,omitempty"`
	ResponseV2         *assistantcontract.Response      `json:"response_v2,omitempty"`
	ModelName          string                           `json:"model_name"`
	LatencyMs          int64                            `json:"latency_ms"`
	Feedback           string                           `json:"feedback"`
	CreatedAt          time.Time                        `json:"created_at"`
}

type opsFeedbackBody struct {
	Feedback string `json:"feedback" binding:"required"`
}

// SubmitOpsAssistantFeedback POST .../assistant/messages/:messageId/feedback
func SubmitOpsAssistantFeedback(c *gin.Context) {
	if !ensureOpsAssistantEnabled(c) {
		return
	}
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}
	messageID, err := strconv.ParseUint(c.Param("messageId"), 10, 64)
	if err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "invalid_message")
		return
	}
	var body opsFeedbackBody
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "invalid_request")
		return
	}
	fb := strings.ToLower(strings.TrimSpace(body.Feedback))
	if fb != "up" && fb != "down" {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "invalid_feedback")
		return
	}
	if err := database.SetOpsAssistantMessageFeedback(business.ID, uint(messageID), fb); err != nil {
		if errors.Is(err, database.ErrOpsAssistantMessageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
			return
		}
		logger.Logger.Warnf("ops assistant feedback persist failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not record feedback"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "feedback_recorded"})
}
