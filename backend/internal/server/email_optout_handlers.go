package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"

	"github.com/gin-gonic/gin"
)

// UnsubscribeMarketingEmail is the tokenized, no-auth marketing opt-out
// (P2-10). POST target of the RFC-8058 List-Unsubscribe header AND of the
// public /unsubscribe page's confirm button. It flips users.email_enabled off
// for EVERY account matching the token's email case-insensitively —
// conservative and deterministic (see marketingEmailOptedOut). Idempotent;
// 200 regardless of whether an account exists (no account-existence oracle).
func UnsubscribeMarketingEmail(c *gin.Context) {
	email, err := emails.ParseUnsubscribeToken(c.Query("token"), time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired unsubscribe link"})
		return
	}
	if err := database.GetDB().Model(&database.User{}).
		Where("LOWER(email) = ?", strings.ToLower(email)).
		Update("email_enabled", false).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not update preferences"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"unsubscribed": true, "email": maskEmailForDisplay(email)})
}

// GetUnsubscribeStatus validates a token for the public /unsubscribe page and
// returns the masked address it would opt out. Read-only.
func GetUnsubscribeStatus(c *gin.Context) {
	email, err := emails.ParseUnsubscribeToken(c.Query("token"), time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired unsubscribe link"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"email": maskEmailForDisplay(email)})
}

// maskEmailForDisplay keeps the first rune + domain: "o***@example.com".
func maskEmailForDisplay(email string) string {
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}
