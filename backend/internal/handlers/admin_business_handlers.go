package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/utils"
	"gorm.io/gorm"
)

// AdminBusinessHandler handles business management operations.
type AdminBusinessHandler struct {
	DB *gorm.DB
}

// NewAdminBusinessHandler creates a new AdminBusinessHandler.
func NewAdminBusinessHandler(db *gorm.DB) *AdminBusinessHandler {
	return &AdminBusinessHandler{DB: db}
}

// --- Response types ---

// AdminBusinessDetailBusiness is the admin detail projection of a business.
// Wallet addresses, contact details, and settings stay off this payload.
type AdminBusinessDetailBusiness struct {
	ID                   uint                     `json:"id"`
	BusinessID           string                   `json:"business_id"`
	Slug                 string                   `json:"slug"`
	Name                 string                   `json:"name"`
	Address              database.BusinessAddress `json:"address"`
	Kind                 string                   `json:"kind"`
	IsActive             bool                     `json:"is_active"`
	ClosedAt             *time.Time               `json:"closed_at"`
	CreatedAt            time.Time                `json:"created_at"`
	HasSettlementAddress bool                     `json:"has_settlement_address"`
	HasTippingAddress    bool                     `json:"has_tipping_address"`
}

// AdminBusinessDetailOwner is the admin detail projection of the owning user.
// The full email never leaves the handler; only the domain does. Nil when the
// business has no owner user.
type AdminBusinessDetailOwner struct {
	ID          uint      `json:"id"`
	EmailDomain string    `json:"email_domain"`
	CreatedAt   time.Time `json:"created_at"`
}

// AdminBusinessDetailStaff represents a staff member in the business detail response.
type AdminBusinessDetailStaff struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	EmailDomain string `json:"email_domain"`
	Role        string `json:"role"`
}

// adminDetailEmailDomain is the part after the last "@", lower-cased.
// It is empty when the address has no domain.
func adminDetailEmailDomain(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at < 0 || at+1 >= len(email) {
		return ""
	}
	return strings.ToLower(email[at+1:])
}

func projectAdminBusinessDetail(business *database.Business) AdminBusinessDetailBusiness {
	return AdminBusinessDetailBusiness{
		ID:                   business.ID,
		BusinessID:           business.BusinessId,
		Slug:                 business.CustomURL,
		Name:                 business.Name,
		Address:              business.Address,
		Kind:                 string(business.Kind),
		IsActive:             business.IsActive,
		ClosedAt:             business.ClosedAt,
		CreatedAt:            business.CreatedAt,
		HasSettlementAddress: strings.TrimSpace(business.SettlementAddr) != "",
		HasTippingAddress:    strings.TrimSpace(business.TippingAddr) != "",
	}
}

// AdminBusinessDetailActivity represents recent activity stats.
type AdminBusinessDetailActivity struct {
	RecentOrderCount   int64   `json:"recent_order_count"`
	RecentPaymentCount int64   `json:"recent_payment_count"`
	TotalRevenue       float64 `json:"total_revenue"`
	TotalTips          float64 `json:"total_tips"`
}

// --- Request types ---

// AdminBusinessActionRequest represents a suspend/reactivate request body.
type AdminBusinessActionRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// --- Handlers ---

// GetBusinessDetail returns detailed information about a single business.
// GET /api/v1/admin/businesses/:id/detail
func (h *AdminBusinessHandler) GetBusinessDetail(c *gin.Context) {
	// Fetch business
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Business not found")
		return
	}

	// Fetch owner user. Only id, email, and created_at are loaded; the response
	// keeps the domain and drops the local part.
	var owner *AdminBusinessDetailOwner
	if business.UserID != nil {
		var user database.User
		if err := h.DB.Select("id, email, created_at").First(&user, *business.UserID).Error; err == nil {
			owner = &AdminBusinessDetailOwner{
				ID:          user.ID,
				EmailDomain: adminDetailEmailDomain(user.Email),
				CreatedAt:   user.CreatedAt,
			}
		}
	}

	// Fetch staff members associated with this business
	var staffMembers []AdminBusinessDetailStaff
	var staffRecords []database.Staff
	if err := h.DB.Where("business_id = ?", business.ID).Find(&staffRecords).Error; err == nil {
		for _, s := range staffRecords {
			staffMembers = append(staffMembers, AdminBusinessDetailStaff{
				ID:          s.ID,
				Name:        s.Name,
				EmailDomain: adminDetailEmailDomain(s.Email),
				Role:        string(s.Role),
			})
		}
	}
	if staffMembers == nil {
		staffMembers = []AdminBusinessDetailStaff{}
	}

	// Fetch recent activity (last 30 days)
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	activity := AdminBusinessDetailActivity{}

	// Recent orders count
	h.DB.Model(&database.Order{}).
		Where("business_id = ? AND created_at >= ?", business.ID, thirtyDaysAgo).
		Count(&activity.RecentOrderCount)

	// Recent payments count and total revenue (payments are linked via bills)
	summary, err := database.GetRecognizedPaymentSummary(business.ID, thirtyDaysAgo, time.Now())
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to calculate business payment activity")
		return
	}
	activity.RecentPaymentCount = summary.PositiveEventCount
	activity.TotalRevenue = float64(summary.TotalRevenueCents) / 100.0
	activity.TotalTips = float64(summary.TotalTipCents) / 100.0

	// Fetch admin actions for this business (owner-scoped + business_id in details)
	var adminActions []AdminActionResponse
	ownerUserID := uint(0)
	if business.UserID != nil {
		ownerUserID = *business.UserID
	}
	actions, _ := database.GetAdminActionsForBusinessContext(business.ID, ownerUserID, 50)
	adminActions = mapAdminActionResponses(h.DB, actions)
	if adminActions == nil {
		adminActions = []AdminActionResponse{}
	}

	c.JSON(http.StatusOK, gin.H{
		"business":      projectAdminBusinessDetail(business),
		"owner":         owner,
		"staff":         staffMembers,
		"activity":      activity,
		"admin_actions": adminActions,
	})
}

// SuspendBusiness sets is_active=false and logs an admin action.
// POST /api/v1/admin/businesses/:id/suspend
func (h *AdminBusinessHandler) SuspendBusiness(c *gin.Context) {
	var req AdminBusinessActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Reason is required")
		return
	}

	adminUserID, ok := requireAdminIdentity(c)
	if !ok {
		return
	}

	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Business not found")
		return
	}

	h.setBusinessActive(c, business, adminUserID, req.Reason, false)
}

// ReactivateBusiness sets is_active=true and logs an admin action.
// POST /api/v1/admin/businesses/:id/reactivate
func (h *AdminBusinessHandler) ReactivateBusiness(c *gin.Context) {
	var req AdminBusinessActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Reason is required")
		return
	}

	adminUserID, ok := requireAdminIdentity(c)
	if !ok {
		return
	}

	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Business not found")
		return
	}

	h.setBusinessActive(c, business, adminUserID, req.Reason, true)
}

// setBusinessActive is the admin suspend/reactivate: the admin lock is
// is_active (closed_at is the separate, permanent closure).
func (h *AdminBusinessHandler) setBusinessActive(c *gin.Context, business *database.Business, adminUserID uint, reason string, active bool) {
	action, verb, already := "suspend_business", "suspend", "Business is already suspended"
	if active {
		action, verb, already = "reactivate_business", "reactivate", "Business is already active"
	}
	result := h.DB.Model(&database.Business{}).
		Where("id = ? AND is_active = ?", business.ID, !active).
		Update("is_active", active)
	if result.Error != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to "+verb+" business")
		return
	}
	if result.RowsAffected == 0 {
		server.RespondWithError(c, http.StatusConflict, "", already)
		return
	}

	targetUserID := uint(0)
	if business.UserID != nil {
		targetUserID = *business.UserID
	}
	logAdminActionBestEffort(adminUserID, targetUserID, action, map[string]interface{}{
		"business_id":   business.ID,
		"business_name": business.Name,
		"reason":        reason,
	})

	// A closed account stays locked after reactivation: closure is permanent
	// and is not reversed by toggling is_active.
	if active && business.ClosedAt != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"warning": "Business reactivated, but its account is closed, so access stays locked.",
			"status":  database.BusinessStatusClosed,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
