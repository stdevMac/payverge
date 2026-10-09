package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"gorm.io/gorm"
)

// requireAdminIdentity resolves the authenticated admin before any mutation.
// Returns false after writing an error response when identity is missing.
// user_id 0 is the synthetic MCP admin context and must not mutate users or businesses.
func requireAdminIdentity(c *gin.Context) (uint, bool) {
	adminUserID, err := server.ExtractAdminUserID(c)
	if err != nil {
		log.Printf("[Admin] Missing admin identity: %v", err)
		server.RespondWithError(c, http.StatusUnauthorized, "", "Admin identity not available")
		return 0, false
	}
	if adminUserID == 0 {
		log.Printf("[Admin] Missing admin identity: user_id is 0")
		server.RespondWithError(c, http.StatusForbidden, "", "Admin identity not available")
		return 0, false
	}
	return adminUserID, true
}

func logAdminActionBestEffort(adminUserID, targetUserID uint, actionType string, details map[string]interface{}) {
	if err := database.CreateAdminAction(adminUserID, targetUserID, actionType, details); err != nil {
		log.Printf("[Admin] Failed to log admin action %s: %v", actionType, err)
	}
}

func mapAdminActionResponses(db *gorm.DB, actions []database.AdminAction) []AdminActionResponse {
	if len(actions) == 0 {
		return []AdminActionResponse{}
	}

	adminIDSet := make(map[uint]struct{})
	for _, a := range actions {
		adminIDSet[a.AdminUserID] = struct{}{}
	}
	adminIDs := make([]uint, 0, len(adminIDSet))
	for id := range adminIDSet {
		adminIDs = append(adminIDs, id)
	}
	adminEmailMap := make(map[uint]string)
	if len(adminIDs) > 0 {
		var adminUsers []database.User
		if err := db.Select("id, email").Where("id IN ?", adminIDs).Find(&adminUsers).Error; err == nil {
			for _, u := range adminUsers {
				adminEmailMap[u.ID] = u.Email
			}
		}
	}

	responses := make([]AdminActionResponse, 0, len(actions))
	for _, a := range actions {
		adminEmail := "unknown"
		if email, ok := adminEmailMap[a.AdminUserID]; ok {
			adminEmail = email
		}
		responses = append(responses, AdminActionResponse{
			ID:         a.ID,
			AdminEmail: adminEmail,
			ActionType: a.ActionType,
			Details:    json.RawMessage(a.Details),
			CreatedAt:  a.CreatedAt,
		})
	}
	return responses
}

func businessBelongsToUser(business database.Business, user database.User) bool {
	if business.UserID != nil && *business.UserID == user.ID {
		return true
	}
	if user.Address != "" && strings.EqualFold(business.OwnerAddress, user.Address) {
		return true
	}
	return false
}
