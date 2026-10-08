package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// staffPermissionBreakdown returns the display permission lists for a staff
// member, split into role defaults, custom grants, and explicit denies:
//
//   - roleOnly: canonical role permissions (server.RolePermissions) as strings
//   - custom:   parsed CustomPermissions JSON only (may include keys that
//     already exist on the role if mis-granted; listed as-is). Empty slice
//     when missing or malformed ("Malformed custom JSON is ignored").
//   - denied:   explicit deny permission strings (may be empty).
//   - effective: (role ∪ custom) − denies − owner-only — same policy as
//     enforcement, minus check-time wildcard composition.
//
// Sourcing role perms from the canonical enforcement set prevents the UI from
// drifting out of sync with what is actually enforced (AUTH-3).
func staffPermissionBreakdown(role database.StaffRole, customPermsJSON string, denies []string) (effective, roleOnly, custom, denied []string) {
	rolePerms := server.RolePermissions(role)
	roleOnly = make([]string, 0, len(rolePerms))
	for _, p := range rolePerms {
		roleOnly = append(roleOnly, string(p))
	}
	if customPermsJSON != "" {
		_ = json.Unmarshal([]byte(customPermsJSON), &custom)
	}
	// On missing/malformed JSON, Unmarshal leaves custom nil — force empty
	// slice so JSON encodes as [] rather than null.
	if custom == nil {
		custom = []string{}
	}
	if denies == nil {
		denied = []string{}
	} else {
		denied = append([]string{}, denies...)
	}

	// Build effective = (role ∪ custom) minus denies and owner-only.
	// Deny matching honors wildcards (menu:* denies menu:read).
	union := append([]string{}, roleOnly...)
	union = append(union, custom...)
	effective = make([]string, 0, len(union))
	for _, perm := range union {
		if server.IsOwnerOnlyPermission(perm) {
			continue
		}
		if permissionDeniedBy(denied, perm) {
			continue
		}
		effective = append(effective, perm)
	}
	return effective, roleOnly, custom, denied
}

// permissionDeniedBy mirrors server denyMatches for display filtering without
// exporting the private helper (same exact / category-wildcard / *:* rules).
func permissionDeniedBy(denies []string, required string) bool {
	for _, d := range denies {
		if d == required {
			return true
		}
		if strings.HasSuffix(d, ":*") {
			category := strings.TrimSuffix(d, ":*")
			if strings.HasPrefix(required, category+":") {
				return true
			}
		}
		if d == "*:*" {
			return true
		}
	}
	return false
}

// staffDisplayPermissions returns the effective permission list to DISPLAY for a
// staff member (role + custom − denies − owner-only). Thin wrapper over
// staffPermissionBreakdown for callers that only need the union.
func staffDisplayPermissions(role database.StaffRole, customPermsJSON string) []string {
	effective, _, _, _ := staffPermissionBreakdown(role, customPermsJSON, nil)
	return effective
}

// RBACHandlers handles RBAC-related API endpoints
type RBACHandlers struct {
	rbacService *services.RBACService
	db          *database.DB
}

// NewRBACHandlers creates a new RBAC handlers instance
func NewRBACHandlers(db *database.DB) *RBACHandlers {
	return &RBACHandlers{
		rbacService: services.NewRBACService(db),
		db:          db,
	}
}

func rbacBusinessFromRoute(c *gin.Context) (*database.Business, bool) {
	business, err := database.GetBusinessByIdOrBusinessId(c.Param("id"))
	if err != nil || business == nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
		return nil, false
	}
	if !server.CheckBusinessAccess(c, business) {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied")
		return nil, false
	}
	return business, true
}

// verifyStaffAccess checks that the staff member exists and belongs to the caller's business.
// Returns the staff record on success, or writes an error response and returns nil.
//
// When the route carries a business id (:id) — as all staff-scoped routes do —
// the target staff's BusinessID must also match that route business. This
// mirrors GetStaffAuditLog: without it, a multi-business owner could mutate
// Business B's staff through a /businesses/A/... URL, mis-attributing the audit
// trail to the wrong business. (Authorization itself is still enforced via
// CheckBusinessAccess against the target's real business.)
func (h *RBACHandlers) verifyStaffAccess(c *gin.Context, staffID uint) *database.Staff {
	var staffRecord database.Staff
	if err := h.db.GetGorm().First(&staffRecord, staffID).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Staff member not found")
		return nil
	}
	business, err := database.GetBusinessByID(staffRecord.BusinessID)
	if err != nil || !server.CheckBusinessAccess(c, business) {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied")
		return nil
	}
	if c.Param("id") != "" {
		routeBusiness, ok := rbacBusinessFromRoute(c)
		if !ok {
			return nil
		}
		if staffRecord.BusinessID != routeBusiness.ID {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Staff member not found")
			return nil
		}
	}
	return &staffRecord
}

// guardStaffMutation enforces three invariants for any handler that mutates
// a specific staff record by ID:
//  1. Cross-business: target staff belongs to the business the actor is in
//     (delegated to verifyStaffAccess).
//  2. Self-modification: actor cannot modify their own staff row via these
//     handlers (use a different flow for self-changes).
//  3. Hierarchy: a staff actor cannot manage a target whose role is at or
//     above the actor's role. Owner Web3/OAuth callers (no staff_role on
//     context) skip the hierarchy check; they outrank every staff role.
//
// On any failure, writes the appropriate status and returns nil. On success,
// returns the target staff record (already loaded by verifyStaffAccess).
func (h *RBACHandlers) guardStaffMutation(c *gin.Context, staffID uint) *database.Staff {
	if actor := server.ExtractStaffIDFromContext(c); actor != nil && *actor == staffID {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Cannot modify yourself")
		return nil
	}
	target := h.verifyStaffAccess(c, staffID)
	if target == nil {
		return nil
	}
	if actorRoleVal, ok := c.Get("staff_role"); ok {
		actorRoleStr, _ := actorRoleVal.(string)
		if !server.CanManageRole(database.StaffRole(actorRoleStr), target.Role) {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Cannot manage a staff member at or above your role")
			return nil
		}
	}
	return target
}

// GetStaffPermissions returns all permissions for a staff member
func (h *RBACHandlers) GetStaffPermissions(c *gin.Context) {
	staffIDStr := c.Param("staffId")
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid staff ID")
		return
	}

	// Verify staff belongs to the requesting user's business (also loads the row).
	staff := h.verifyStaffAccess(c, uint(staffID))
	if staff == nil {
		return
	}

	// Display the canonical role permissions + custom grants + denies (AUTH-3):
	// the same sources the enforcement path uses, so the UI never drifts from
	// reality. Expose inherited / granted / denied for the permission editor.
	denies, err := h.rbacService.ListPermissionDenies(staff.ID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to load permission denies")
		return
	}
	effective, roleOnly, custom, denied := staffPermissionBreakdown(staff.Role, staff.CustomPermissions, denies)

	c.JSON(http.StatusOK, gin.H{
		"permissions":      effective,
		"role_permissions": roleOnly,
		"custom_grants":    custom,
		"custom_denies":    denied,
	})
}

// ChangeStaffRole changes a staff member's role
func (h *RBACHandlers) ChangeStaffRole(c *gin.Context) {
	staffIDStr := c.Param("staffId")
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid staff ID")
		return
	}

	if h.guardStaffMutation(c, uint(staffID)) == nil {
		return
	}

	var request struct {
		NewRole string `json:"new_role" binding:"required"`
		Reason  string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Convert string role to StaffRole enum
	var newRole database.StaffRole
	switch request.NewRole {
	case "kitchen":
		newRole = database.StaffRoleKitchen
	case "host":
		newRole = database.StaffRoleHost
	case "server":
		newRole = database.StaffRoleServer
	case "manager":
		newRole = database.StaffRoleManager
	default:
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid role")
		return
	}

	// Additional gate beyond the helper's target-role check: the actor cannot
	// grant a role at or above their own. Owner Web3/OAuth callers (no
	// staff_role on context) skip this check.
	if actorRoleVal, ok := c.Get("staff_role"); ok {
		actorRoleStr, _ := actorRoleVal.(string)
		if !server.CanManageRole(database.StaffRole(actorRoleStr), newRole) {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Cannot assign a role at or above your own")
			return
		}
	}

	// Get the user making the change
	changedBy := "unknown"
	if address, exists := c.Get("address"); exists {
		if s, ok := address.(string); ok {
			changedBy = s
		}
	} else if email, exists := c.Get("staff_email"); exists {
		if s, ok := email.(string); ok {
			changedBy = s
		}
	}

	err = h.rbacService.ChangeStaffRole(uint(staffID), newRole, changedBy, request.Reason)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to change staff role")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Staff role updated successfully"})
}

// GrantCustomPermission grants a custom permission to a staff member
func (h *RBACHandlers) GrantCustomPermission(c *gin.Context) {
	staffIDStr := c.Param("staffId")
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid staff ID")
		return
	}

	if h.guardStaffMutation(c, uint(staffID)) == nil {
		return
	}

	var request struct {
		Permission string `json:"permission" binding:"required"`
		Reason     string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Allowlist: a staff actor cannot grant a permission outside their own
	// role's perm set. This blocks future owner-only perms (e.g. a
	// hypothetical financial:withdraw) from being self- or peer-granted via
	// this handler. Owner Web3/OAuth callers skip the check.
	if actorRoleVal, ok := c.Get("staff_role"); ok {
		actorRoleStr, _ := actorRoleVal.(string)
		actorPerms := server.RolePermissions(database.StaffRole(actorRoleStr))
		if actorPerms == nil {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Unknown actor role")
			return
		}
		grantable := false
		for _, p := range actorPerms {
			if string(p) == request.Permission {
				grantable = true
				break
			}
		}
		if !grantable {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Cannot grant a permission outside your own role's perm set")
			return
		}
	}

	// Get the user making the change
	grantedBy := "unknown"
	if address, exists := c.Get("address"); exists {
		if s, ok := address.(string); ok {
			grantedBy = s
		}
	} else if email, exists := c.Get("staff_email"); exists {
		if s, ok := email.(string); ok {
			grantedBy = s
		}
	}

	err = h.rbacService.GrantCustomPermission(uint(staffID), request.Permission, grantedBy, request.Reason)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to grant permission")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Permission granted successfully"})
}

// RevokeCustomPermission revokes a custom permission from a staff member
func (h *RBACHandlers) RevokeCustomPermission(c *gin.Context) {
	staffIDStr := c.Param("staffId")
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid staff ID")
		return
	}

	if h.guardStaffMutation(c, uint(staffID)) == nil {
		return
	}

	var request struct {
		Permission string `json:"permission" binding:"required"`
		Reason     string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Get the user making the change
	revokedBy := "unknown"
	if address, exists := c.Get("address"); exists {
		if s, ok := address.(string); ok {
			revokedBy = s
		}
	} else if email, exists := c.Get("staff_email"); exists {
		if s, ok := email.(string); ok {
			revokedBy = s
		}
	}

	err = h.rbacService.RevokeCustomPermission(uint(staffID), request.Permission, revokedBy, request.Reason)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to revoke permission")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Permission revoked successfully"})
}

// DenyPermission sets an explicit deny for a staff member + permission.
func (h *RBACHandlers) DenyPermission(c *gin.Context) {
	staffIDStr := c.Param("staffId")
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid staff ID")
		return
	}

	if h.guardStaffMutation(c, uint(staffID)) == nil {
		return
	}

	var request struct {
		Permission string `json:"permission" binding:"required"`
		Reason     string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Staff actors cannot deny permissions outside their own role set (mirrors grant allowlist).
	// Owners (no staff_role) skip the check.
	if !actorMayTouchPermission(c, request.Permission, "deny") {
		return
	}

	deniedBy := rbacActorIdentity(c)
	err = h.rbacService.DenyPermission(uint(staffID), request.Permission, deniedBy, request.Reason)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to deny permission")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Permission denied successfully"})
}

// RemovePermissionDeny clears an explicit deny for a staff member + permission.
func (h *RBACHandlers) RemovePermissionDeny(c *gin.Context) {
	staffIDStr := c.Param("staffId")
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid staff ID")
		return
	}

	if h.guardStaffMutation(c, uint(staffID)) == nil {
		return
	}

	var request struct {
		Permission string `json:"permission" binding:"required"`
		Reason     string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Wildcard denies (including *:* and category wildcards) can only be lifted
	// by an owner. A staff actor is rejected even when the pattern is somehow
	// present on their role allowlist.
	if strings.Contains(request.Permission, "*") {
		if _, isStaff := c.Get("staff_role"); isStaff {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Cannot lift a wildcard permission")
			return
		}
	}
	if !actorMayTouchPermission(c, request.Permission, "lift") {
		return
	}

	removedBy := rbacActorIdentity(c)
	err = h.rbacService.RemovePermissionDeny(uint(staffID), request.Permission, removedBy, request.Reason)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to remove permission deny")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Permission deny removed successfully"})
}

// actorMayTouchPermission reports whether the caller may deny or lift perm.
// Owners (no staff_role) may. A staff actor may only touch permissions in
// their own role set. On denial it writes 403 and returns false. verb is the
// action word in the outside-role message ("deny", "lift").
func actorMayTouchPermission(c *gin.Context, perm string, verb string) bool {
	actorRoleVal, ok := c.Get("staff_role")
	if !ok {
		return true
	}
	actorRoleStr, _ := actorRoleVal.(string)
	actorPerms := server.RolePermissions(database.StaffRole(actorRoleStr))
	if actorPerms == nil {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Unknown actor role")
		return false
	}
	for _, p := range actorPerms {
		if string(p) == perm {
			return true
		}
	}
	server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Cannot "+verb+" a permission outside your own role's perm set")
	return false
}

func rbacActorIdentity(c *gin.Context) string {
	if address, exists := c.Get("address"); exists {
		if s, ok := address.(string); ok && s != "" {
			return s
		}
	}
	if email, exists := c.Get("staff_email"); exists {
		if s, ok := email.(string); ok && s != "" {
			return s
		}
	}
	return "unknown"
}

// GetStaffAuditLog returns RBAC audit log for a specific staff member
func (h *RBACHandlers) GetStaffAuditLog(c *gin.Context) {
	staffIDStr := c.Param("staffId")
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid staff ID")
		return
	}

	// Verify staff belongs to one of the requesting user's businesses.
	staff := h.verifyStaffAccess(c, uint(staffID))
	if staff == nil {
		return
	}
	if c.Param("id") != "" {
		business, ok := rbacBusinessFromRoute(c)
		if !ok {
			return
		}
		if staff.BusinessID != business.ID {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Staff member not found")
			return
		}
	}

	// Parse query parameters.
	// Clamp limit so a caller can't request arbitrarily many rows.
	limit := ClampLimit(c.Query("limit"), 20, 200)

	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}

	logs, err := h.rbacService.GetStaffAuditLog(uint(staffID), limit, offset)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to retrieve audit log")
		return
	}

	c.JSON(http.StatusOK, gin.H{"audit_logs": logs})
}

// DeactivateStaff deactivates a staff member
func (h *RBACHandlers) DeactivateStaff(c *gin.Context) {
	staffIDStr := c.Param("staffId")
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid staff ID")
		return
	}

	if h.guardStaffMutation(c, uint(staffID)) == nil {
		return
	}

	var request struct {
		Reason string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Get the user making the change
	deactivatedBy := "unknown"
	if address, exists := c.Get("address"); exists {
		if s, ok := address.(string); ok {
			deactivatedBy = s
		}
	} else if email, exists := c.Get("staff_email"); exists {
		if s, ok := email.(string); ok {
			deactivatedBy = s
		}
	}

	err = h.rbacService.DeactivateStaff(uint(staffID), deactivatedBy, request.Reason)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to deactivate staff")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Staff deactivated successfully"})
}

// ReactivateStaff reactivates a staff member
func (h *RBACHandlers) ReactivateStaff(c *gin.Context) {
	staffIDStr := c.Param("staffId")
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid staff ID")
		return
	}

	if h.guardStaffMutation(c, uint(staffID)) == nil {
		return
	}

	var request struct {
		Reason string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Get the user making the change
	reactivatedBy := "unknown"
	if address, exists := c.Get("address"); exists {
		if s, ok := address.(string); ok {
			reactivatedBy = s
		}
	} else if email, exists := c.Get("staff_email"); exists {
		if s, ok := email.(string); ok {
			reactivatedBy = s
		}
	}

	err = h.rbacService.ReactivateStaff(uint(staffID), reactivatedBy, request.Reason)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to reactivate staff")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Staff reactivated successfully"})
}
