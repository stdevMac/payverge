package server

import (
	"net/http"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type pushSubscriptionRequest struct {
	BusinessID uint   `json:"business_id" binding:"required"`
	Endpoint   string `json:"endpoint" binding:"required"`
	P256dhKey  string `json:"p256dh_key" binding:"required"`
	AuthKey    string `json:"auth_key" binding:"required"`
	UserAgent  string `json:"user_agent"`
}

// resolvePushPrincipal derives the explicit push principal from auth context.
// Staff JWTs supply staff_id (no user_id required); owners/users supply user_id.
func resolvePushPrincipal(c *gin.Context) (principalType string, principalID uint, userID uint, ok bool) {
	if staffIDVal, exists := c.Get("staff_id"); exists {
		if sid, parsed := extractContextUint(staffIDVal); parsed && sid > 0 {
			return database.PushPrincipalStaff, sid, 0, true
		}
	}
	if userIDVal, exists := c.Get("user_id"); exists {
		if uid, parsed := extractContextUint(userIDVal); parsed && uid > 0 {
			return database.PushPrincipalOwnerUser, uid, uid, true
		}
	}
	return "", 0, 0, false
}

// CreatePushSubscription registers a new Web Push subscription for the
// authenticated principal (owner_user or staff). Upsert is keyed on
// (principal_type, principal_id, endpoint) so resubscribe refreshes keys/business.
func CreatePushSubscription(c *gin.Context) {
	principalType, principalID, userID, ok := resolvePushPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req pushSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if err := services.ValidatePushEndpoint(req.Endpoint); err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeFieldInvalid, err.Error())
		return
	}

	business, err := database.GetBusinessByID(req.BusinessID)
	if err != nil || business == nil || !business.IsActive {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}
	if !CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have access to this business"})
		return
	}
	if RespondIfBusinessLocked(c, business) {
		return
	}

	sub := database.PushSubscription{
		UserID:        userID,
		BusinessID:    req.BusinessID,
		PrincipalType: principalType,
		PrincipalID:   principalID,
		Endpoint:      req.Endpoint,
		P256dhKey:     req.P256dhKey,
		AuthKey:       req.AuthKey,
		UserAgent:     req.UserAgent,
	}

	// Upsert keyed (principal, endpoint): a resubscribe must REFRESH business_id
	// and the browser's rotated p256dh/auth keys.
	db := database.GetDB()
	result := db.Where(
		"principal_type = ? AND principal_id = ? AND endpoint = ?",
		sub.PrincipalType, sub.PrincipalID, sub.Endpoint,
	).
		Assign(map[string]interface{}{
			"business_id":    req.BusinessID,
			"user_id":        userID,
			"principal_type": principalType,
			"principal_id":   principalID,
			"p256dh_key":     req.P256dhKey,
			"auth_key":       req.AuthKey,
			"user_agent":     req.UserAgent,
		}).
		FirstOrCreate(&sub)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save subscription"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"subscription": sub})
}

// ListPushSubscriptions returns all Web Push subscriptions for the
// authenticated principal.
func ListPushSubscriptions(c *gin.Context) {
	principalType, principalID, _, ok := resolvePushPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var subs []database.PushSubscription
	db := database.GetDB()
	if err := db.Where("principal_type = ? AND principal_id = ?", principalType, principalID).
		Find(&subs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list subscriptions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"subscriptions": subs})
}

// DeletePushSubscription removes a Web Push subscription by ID, scoped to the
// authenticated principal so callers cannot delete each other's subscriptions.
func DeletePushSubscription(c *gin.Context) {
	principalType, principalID, _, ok := resolvePushPrincipal(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	subID := c.Param("subscriptionId")
	db := database.GetDB()
	result := db.Where(
		"id = ? AND principal_type = ? AND principal_id = ?",
		subID, principalType, principalID,
	).Delete(&database.PushSubscription{})
	if result.Error != nil || result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subscription not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Subscription deleted"})
}
