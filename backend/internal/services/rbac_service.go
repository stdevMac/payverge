package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RBACService handles role-based access control operations
type RBACService struct {
	db *database.DB
}

// NewRBACService creates a new RBAC service instance
func NewRBACService(db *database.DB) *RBACService {
	return &RBACService{db: db}
}

// NOTE: the staff role→permission source of truth lives in
// server.StaffRolePermissions (the enforcement path). A duplicate display-only
// map used to live here and had drifted out of sync (AUTH-3); it was removed.
// Anything needing the canonical set imports server.RolePermissions, or — for
// the staff-permissions display endpoint — handlers.staffDisplayPermissions.

// GrantCustomPermission grants a custom permission to a staff member
func (s *RBACService) GrantCustomPermission(staffID uint, permission string, grantedBy string, reason string) error {
	return s.applyAuthzMutation(staffID, reason, grantedBy, func(tx *gorm.DB, staff *database.Staff) (database.RBACAuditLog, error) {
		// Parse existing custom permissions
		var customPerms []string
		if staff.CustomPermissions != "" {
			if err := json.Unmarshal([]byte(staff.CustomPermissions), &customPerms); err != nil {
				return database.RBACAuditLog{}, fmt.Errorf("failed to parse existing permissions: %w", err)
			}
		}

		// Check if permission already exists
		for _, perm := range customPerms {
			if perm == permission {
				return database.RBACAuditLog{}, fmt.Errorf("permission already granted")
			}
		}

		customPerms = append(customPerms, permission)
		permsJSON, err := json.Marshal(customPerms)
		if err != nil {
			return database.RBACAuditLog{}, fmt.Errorf("failed to marshal permissions: %w", err)
		}

		now := time.Now()
		oldPerms := staff.CustomPermissions
		staff.CustomPermissions = string(permsJSON)
		staff.PermissionsUpdatedAt = &now
		staff.PermissionsUpdatedBy = grantedBy

		if err := tx.Omit(clause.Associations).Save(staff).Error; err != nil {
			return database.RBACAuditLog{}, fmt.Errorf("failed to update staff permissions: %w", err)
		}

		return database.RBACAuditLog{
			StaffID:        staffID,
			BusinessID:     staff.BusinessID,
			Action:         database.RBACActionPermissionGranted,
			OldPermissions: oldPerms,
			NewPermissions: string(permsJSON),
			ChangedBy:      grantedBy,
			Reason:         reason,
			CreatedAt:      now,
		}, nil
	})
}

// DenyPermission upserts an explicit deny for a staff member. Setting a deny
// does NOT remove an existing grant — effective computation subtracts denies.
func (s *RBACService) DenyPermission(staffID uint, permission string, deniedBy string, reason string) error {
	if permission == "" {
		return fmt.Errorf("permission is required")
	}

	return s.applyAuthzMutation(staffID, reason, deniedBy, func(tx *gorm.DB, staff *database.Staff) (database.RBACAuditLog, error) {
		now := time.Now()
		var existing database.StaffPermissionDeny
		err := tx.
			Where("business_id = ? AND staff_id = ? AND permission = ?", staff.BusinessID, staffID, permission).
			First(&existing).Error

		if err == nil {
			// Already denied — refresh metadata (idempotent upsert).
			existing.CreatedBy = deniedBy
			existing.Reason = reason
			existing.UpdatedAt = now
			if err := tx.Omit(clause.Associations).Save(&existing).Error; err != nil {
				return database.RBACAuditLog{}, fmt.Errorf("failed to update permission deny: %w", err)
			}
		} else if err == gorm.ErrRecordNotFound {
			row := database.StaffPermissionDeny{
				BusinessID: staff.BusinessID,
				StaffID:    staffID,
				Permission: permission,
				CreatedBy:  deniedBy,
				Reason:     reason,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if err := tx.Create(&row).Error; err != nil {
				return database.RBACAuditLog{}, fmt.Errorf("failed to create permission deny: %w", err)
			}
		} else {
			return database.RBACAuditLog{}, fmt.Errorf("failed to look up permission deny: %w", err)
		}

		staff.PermissionsUpdatedAt = &now
		staff.PermissionsUpdatedBy = deniedBy
		if err := tx.Model(staff).Updates(map[string]interface{}{
			"permissions_updated_at": now,
			"permissions_updated_by": deniedBy,
		}).Error; err != nil {
			return database.RBACAuditLog{}, fmt.Errorf("failed to update staff permission metadata: %w", err)
		}

		return database.RBACAuditLog{
			StaffID:        staffID,
			BusinessID:     staff.BusinessID,
			Action:         database.RBACActionPermissionDenied,
			OldPermissions: "",
			NewPermissions: permission,
			ChangedBy:      deniedBy,
			Reason:         reason,
			CreatedAt:      now,
		}, nil
	})
}

// RemovePermissionDeny deletes an explicit deny row for a staff member.
func (s *RBACService) RemovePermissionDeny(staffID uint, permission string, removedBy string, reason string) error {
	if permission == "" {
		return fmt.Errorf("permission is required")
	}

	return s.applyAuthzMutation(staffID, reason, removedBy, func(tx *gorm.DB, staff *database.Staff) (database.RBACAuditLog, error) {
		result := tx.
			Where("business_id = ? AND staff_id = ? AND permission = ?", staff.BusinessID, staffID, permission).
			Delete(&database.StaffPermissionDeny{})
		if result.Error != nil {
			return database.RBACAuditLog{}, fmt.Errorf("failed to remove permission deny: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return database.RBACAuditLog{}, fmt.Errorf("permission deny not found")
		}

		now := time.Now()
		if err := tx.Model(staff).Updates(map[string]interface{}{
			"permissions_updated_at": now,
			"permissions_updated_by": removedBy,
		}).Error; err != nil {
			return database.RBACAuditLog{}, fmt.Errorf("failed to update staff permission metadata: %w", err)
		}

		return database.RBACAuditLog{
			StaffID:        staffID,
			BusinessID:     staff.BusinessID,
			Action:         database.RBACActionPermissionDenyRemoved,
			OldPermissions: permission,
			NewPermissions: "",
			ChangedBy:      removedBy,
			Reason:         reason,
			CreatedAt:      now,
		}, nil
	})
}

// ListPermissionDenies returns deny permission strings for a staff member.
func (s *RBACService) ListPermissionDenies(staffID uint) ([]string, error) {
	var denies []string
	if err := s.db.GetGorm().Model(&database.StaffPermissionDeny{}).
		Where("staff_id = ?", staffID).
		Pluck("permission", &denies).Error; err != nil {
		return nil, fmt.Errorf("failed to list permission denies: %w", err)
	}
	if denies == nil {
		denies = []string{}
	}
	return denies, nil
}

// RevokeCustomPermission revokes a custom permission from a staff member
func (s *RBACService) RevokeCustomPermission(staffID uint, permission string, revokedBy string, reason string) error {
	return s.applyAuthzMutation(staffID, reason, revokedBy, func(tx *gorm.DB, staff *database.Staff) (database.RBACAuditLog, error) {
		var customPerms []string
		if staff.CustomPermissions != "" {
			if err := json.Unmarshal([]byte(staff.CustomPermissions), &customPerms); err != nil {
				return database.RBACAuditLog{}, fmt.Errorf("failed to parse existing permissions: %w", err)
			}
		}

		// Find and remove permission
		found := false
		for i, perm := range customPerms {
			if perm == permission {
				customPerms = append(customPerms[:i], customPerms[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return database.RBACAuditLog{}, fmt.Errorf("permission not found")
		}

		permsJSON, err := json.Marshal(customPerms)
		if err != nil {
			return database.RBACAuditLog{}, fmt.Errorf("failed to marshal permissions: %w", err)
		}

		now := time.Now()
		oldPerms := staff.CustomPermissions
		staff.CustomPermissions = string(permsJSON)
		staff.PermissionsUpdatedAt = &now
		staff.PermissionsUpdatedBy = revokedBy

		if err := tx.Omit(clause.Associations).Save(staff).Error; err != nil {
			return database.RBACAuditLog{}, fmt.Errorf("failed to update staff permissions: %w", err)
		}

		return database.RBACAuditLog{
			StaffID:        staffID,
			BusinessID:     staff.BusinessID,
			Action:         database.RBACActionPermissionRevoked,
			OldPermissions: oldPerms,
			NewPermissions: string(permsJSON),
			ChangedBy:      revokedBy,
			Reason:         reason,
			CreatedAt:      now,
		}, nil
	})
}

// ChangeStaffRole changes a staff member's role
func (s *RBACService) ChangeStaffRole(staffID uint, newRole database.StaffRole, changedBy string, reason string) error {
	var staffEmail, staffName string
	var businessID uint
	if err := s.applyAuthzMutation(staffID, reason, changedBy, func(tx *gorm.DB, staff *database.Staff) (database.RBACAuditLog, error) {
		oldRole := staff.Role
		if oldRole == newRole {
			return database.RBACAuditLog{}, fmt.Errorf("staff member already has this role")
		}

		now := time.Now()
		staff.Role = newRole
		staff.PermissionsUpdatedAt = &now
		staff.PermissionsUpdatedBy = changedBy

		if err := tx.Omit(clause.Associations).Save(staff).Error; err != nil {
			return database.RBACAuditLog{}, fmt.Errorf("failed to update staff role: %w", err)
		}

		staffEmail = staff.Email
		staffName = staff.Name
		businessID = staff.BusinessID

		return database.RBACAuditLog{
			StaffID:    staffID,
			BusinessID: staff.BusinessID,
			Action:     database.RBACActionRoleChanged,
			OldRole:    string(oldRole),
			NewRole:    string(newRole),
			ChangedBy:  changedBy,
			Reason:     reason,
			CreatedAt:  now,
		}, nil
	}); err != nil {
		return err
	}

	// Notify the staff member of the role change. Fire-and-forget, post-commit —
	// a send failure must not roll back the already-committed role change.
	role := newRole
	logger.SafeGo(func() {
		if emails.EmailServerInstance == nil || staffEmail == "" {
			return
		}
		var business database.Business
		if err := s.db.GetGorm().First(&business, businessID).Error; err != nil {
			fmt.Printf("Warning: Failed to load business for role update email: %v\n", err)
			return
		}
		language := "en"
		if business.DefaultLanguage != "" {
			language = business.DefaultLanguage
		}
		if err := emails.EmailServerInstance.ForBusiness(business.ID).SendRoleUpdateEmail(
			[]string{staffEmail},
			business.Name,
			staffName,
			string(role),
			language,
		); err != nil {
			fmt.Printf("Warning: Failed to send role update email: %v\n", err)
		}
	})

	return nil
}

// GetStaffAuditLog returns RBAC audit log entries for a specific staff member
func (s *RBACService) GetStaffAuditLog(staffID uint, limit int, offset int) ([]database.RBACAuditLog, error) {
	var logs []database.RBACAuditLog

	query := s.db.GetGorm().Where("staff_id = ?", staffID).
		Preload("Staff", preloadRBACAuditStaffSummary).
		Preload("Business", preloadRBACAuditBusinessSummary).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to get staff audit log: %w", err)
	}

	return logs, nil
}

func preloadRBACAuditStaffSummary(tx *gorm.DB) *gorm.DB {
	return tx.Select("id", "name", "email")
}

func preloadRBACAuditBusinessSummary(tx *gorm.DB) *gorm.DB {
	return tx.Select("id", "business_id", "name")
}

// DeactivateStaff deactivates a staff member
func (s *RBACService) DeactivateStaff(staffID uint, deactivatedBy string, reason string) error {
	return s.applyAuthzMutation(staffID, reason, deactivatedBy, func(tx *gorm.DB, staff *database.Staff) (database.RBACAuditLog, error) {
		if !staff.IsActive {
			return database.RBACAuditLog{}, fmt.Errorf("staff member is already deactivated")
		}

		now := time.Now()
		staff.IsActive = false
		staff.PermissionsUpdatedAt = &now
		staff.PermissionsUpdatedBy = deactivatedBy

		if err := tx.Omit(clause.Associations).Save(staff).Error; err != nil {
			return database.RBACAuditLog{}, fmt.Errorf("failed to deactivate staff: %w", err)
		}

		return database.RBACAuditLog{
			StaffID:    staffID,
			BusinessID: staff.BusinessID,
			Action:     database.RBACActionStaffDeactivated,
			ChangedBy:  deactivatedBy,
			Reason:     reason,
			CreatedAt:  now,
		}, nil
	})
}

// ReactivateStaff reactivates a staff member
func (s *RBACService) ReactivateStaff(staffID uint, reactivatedBy string, reason string) error {
	return s.applyAuthzMutation(staffID, reason, reactivatedBy, func(tx *gorm.DB, staff *database.Staff) (database.RBACAuditLog, error) {
		if staff.IsActive {
			return database.RBACAuditLog{}, fmt.Errorf("staff member is already active")
		}

		now := time.Now()
		staff.IsActive = true
		staff.PermissionsUpdatedAt = &now
		staff.PermissionsUpdatedBy = reactivatedBy

		if err := tx.Omit(clause.Associations).Save(staff).Error; err != nil {
			return database.RBACAuditLog{}, fmt.Errorf("failed to reactivate staff: %w", err)
		}

		return database.RBACAuditLog{
			StaffID:    staffID,
			BusinessID: staff.BusinessID,
			Action:     database.RBACActionStaffReactivated,
			ChangedBy:  reactivatedBy,
			Reason:     reason,
			CreatedAt:  now,
		}, nil
	})
}
