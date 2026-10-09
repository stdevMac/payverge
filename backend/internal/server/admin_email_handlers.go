package server

import (
	"log"
	"net/http"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

// SendOperationalUpdateRequest represents a request to send operational updates
type SendOperationalUpdateRequest struct {
	Recipients  []string `json:"recipients" binding:"required"` // List of email addresses or "all_businesses"
	UpdateTitle string   `json:"update_title" binding:"required"`
	UpdateIntro string   `json:"update_intro" binding:"required"`
	UpdateBody  string   `json:"update_body" binding:"required"`
	Language    string   `json:"language"`
}

// SendPlatformUpdateRequest represents a request to send platform updates
type SendPlatformUpdateRequest struct {
	Recipients  []string `json:"recipients" binding:"required"` // List of email addresses or "all_businesses"
	UpdateTitle string   `json:"update_title" binding:"required"`
	UpdateIntro string   `json:"update_intro" binding:"required"`
	UpdateBody  string   `json:"update_body" binding:"required"`
	Language    string   `json:"language"`
}

// SendOperationalUpdate sends operational updates to specified recipients
func SendOperationalUpdate(c *gin.Context) {
	var req SendOperationalUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	if emails.EmailServerInstance == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Email service not available"})
		return
	}

	// Default language
	if req.Language == "" {
		req.Language = "en"
	}

	// Resolve via the shared broadcast recipient resolver (Task 13): kind=real
	// only + non-deliverable domains excluded so fixtures never reach Resend.
	recipientEmails, err := services.ResolveBroadcastRecipients(req.Recipients)
	if err != nil {
		// FIND-060: do not leak lookup/driver text to admin clients.
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not resolve email recipients")
		return
	}

	if len(recipientEmails) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No valid recipients found"})
		return
	}

	// Send emails
	successCount := 0
	failedCount := 0
	dashboardURL := config.PublicURL() + "/dashboard"

	for email, name := range recipientEmails {
		if err := emails.EmailServerInstance.SendOperationalUpdatesEmail(
			[]string{email},
			name,
			req.UpdateTitle,
			req.UpdateIntro,
			req.UpdateBody,
			dashboardURL,
			req.Language,
		); err != nil {
			log.Printf("Failed to send operational update to %s: %v", logger.RedactEmail(email), err)
			failedCount++
		} else {
			successCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Operational update emails sent",
		"success_count": successCount,
		"failed_count":  failedCount,
		"total":         len(recipientEmails),
	})
}

// SendPlatformUpdate sends platform updates to specified recipients
func SendPlatformUpdate(c *gin.Context) {
	var req SendPlatformUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	if emails.EmailServerInstance == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Email service not available"})
		return
	}

	// Default language
	if req.Language == "" {
		req.Language = "en"
	}

	// Resolve via the shared broadcast recipient resolver (Task 13).
	recipientEmails, err := services.ResolveBroadcastRecipients(req.Recipients)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not resolve email recipients")
		return
	}

	if len(recipientEmails) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No valid recipients found"})
		return
	}

	// Send emails
	successCount := 0
	failedCount := 0
	dashboardURL := config.PublicURL() + "/dashboard"

	for email, name := range recipientEmails {
		if err := emails.EmailServerInstance.SendPayvergeUpdateEmail(
			[]string{email},
			name,
			req.UpdateTitle,
			req.UpdateIntro,
			req.UpdateBody,
			dashboardURL,
			req.Language,
		); err != nil {
			log.Printf("Failed to send platform update to %s: %v", logger.RedactEmail(email), err)
			failedCount++
		} else {
			successCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Platform update emails sent",
		"success_count": successCount,
		"failed_count":  failedCount,
		"total":         len(recipientEmails),
	})
}

// GetBusinessEmails returns deliverable kind=real business emails for the admin
// compose/confirm UI. Demo/test fixtures and reserved domains (.test/.local/
// .invalid/.example) are excluded by the shared broadcast recipient resolver
// (Tasks 12 + 13) so the displayed count matches what a send will resolve to.
func GetBusinessEmails(c *gin.Context) {
	rows, total, sample, err := services.ListBroadcastableBusinessEmails()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch businesses"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"businesses": rows,
		// total / with_email are the deliverable recipient count (kind=real + domain-safe).
		"total":           total,
		"with_email":      total,
		"recipient_count": total,
		"sample":          sample,
		"kind":            "real",
	})
}
