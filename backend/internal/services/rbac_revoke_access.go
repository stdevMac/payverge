package services

import (
	"fmt"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/session"

	"gorm.io/gorm"
)

// staffSessionProviders are the session.Provider values used when minting staff
// JWTs (code login, invite, Google staff OAuth, generic staff). Revocation is
// scoped to these providers so an unrelated user/customer session that happens
// to share the same numeric user_id stays active.
var staffSessionProviders = []string{"google_staff", "staff_code", "staff_invite", "staff", "demo_staff"}

// RevokeStaffAccess is the single chokepoint for invalidating every live access
// channel a staff member holds after an authorization-changing mutation
// (deactivate, role change, permission grant/revoke/deny, remove).
//
// In a DB transaction it:
//  1. atomically increments staff.authz_version (cross-replica JWT invalidation),
//  2. deletes push subscriptions for principal_type=staff / principal_id=staffID.
//
// After commit it:
//  3. revokes scoped staff sessions (provider class),
//  4. publishes a process-local hub event so same-instance SSE streams eject
//     immediately (see DEFERRED note below for multi-replica).
//
// Callers must invoke this from the service methods that perform the mutation
// so a handler cannot change access without going through this path.
//
// DEFERRED (deployment / multi-replica): cross-instance LISTEN/NOTIFY (or an
// equivalent bus) for sub-second SSE eviction on OTHER replicas. Justification:
// authz_version is re-checked on every authenticated request and on every 15s
// SSE heartbeat against the DB (source of truth), so revocation is guaranteed
// within one heartbeat on all replicas — well inside the "dies within one
// minute" bar. LISTEN/NOTIFY is only a latency optimization for the open SSE
// window between heartbeats.
func (s *RBACService) RevokeStaffAccess(staffID uint, reason, changedBy string) error {
	if staffID == 0 {
		return fmt.Errorf("staff id is required")
	}

	var businessID uint
	err := s.db.GetGorm().Transaction(func(tx *gorm.DB) error {
		var staff database.Staff
		if err := tx.Select("id", "business_id").First(&staff, staffID).Error; err != nil {
			return fmt.Errorf("staff member not found: %w", err)
		}
		businessID = staff.BusinessID
		return bumpAuthzVersionTx(tx, staffID)
	})
	if err != nil {
		return err
	}

	s.postAccessRevokeSideEffects(staffID, businessID, reason, changedBy)
	return nil
}

// bumpAuthzVersionTx atomically increments a staff member's authz_version using
// the caller-provided transaction. Concurrent bumps both succeed and leave a
// higher version; any outstanding JWT stays stale. This is the load-bearing
// cross-replica JWT-invalidation signal, so it MUST run inside the same
// transaction as the authorization mutation and its audit record — never as a
// best-effort afterthought.
func bumpAuthzVersionTx(tx *gorm.DB, staffID uint) error {
	if staffID == 0 {
		return fmt.Errorf("staff id is required")
	}
	if err := tx.Model(&database.Staff{}).
		Where("id = ?", staffID).
		UpdateColumn("authz_version", gorm.Expr("authz_version + 1")).Error; err != nil {
		return fmt.Errorf("failed to bump authz_version: %w", err)
	}
	// Dual-write: the staff row is the authoritative JWT anchor, and the
	// membership row's versioning is kept in lockstep so a token revoked here
	// stays revoked if authorization reads ever move to memberships.
	var staff database.Staff
	if err := tx.Select(
		"id", "role", "custom_permissions", "role_level", "is_active", "authz_version",
		"permissions_updated_at", "permissions_updated_by",
	).First(&staff, staffID).Error; err != nil {
		return fmt.Errorf("failed to load staff authorization projection: %w", err)
	}
	if err := tx.Model(&database.StaffMembership{}).
		Where("legacy_staff_id = ?", staffID).
		Updates(map[string]interface{}{
			"role":                   staff.Role,
			"custom_permissions":     staff.CustomPermissions,
			"role_level":             staff.RoleLevel,
			"is_active":              staff.IsActive,
			"authz_version":          staff.AuthzVersion,
			"permissions_updated_at": staff.PermissionsUpdatedAt,
			"permissions_updated_by": staff.PermissionsUpdatedBy,
		}).Error; err != nil {
		return fmt.Errorf("failed to bump membership authz_version: %w", err)
	}
	return nil
}

// applyAuthzMutation runs a permission-changing mutation as ONE atomic unit: it
// loads the staff row, runs mutate (which performs the change and returns the
// audit record to persist), writes the MANDATORY authorization audit record, and
// bumps authz_version — all inside a single transaction. If any step fails
// (including the audit write), the whole mutation rolls back and the error is
// returned, so a permission can never change without a matching audit trail and
// a stale-JWT bump. After the transaction commits, the best-effort live-channel
// eviction side effects run. A precondition returned as an error from mutate
// (e.g. "permission already granted") rolls back an empty transaction.
func (s *RBACService) applyAuthzMutation(
	staffID uint,
	reason, changedBy string,
	mutate func(tx *gorm.DB, staff *database.Staff) (database.RBACAuditLog, error),
) error {
	if staffID == 0 {
		return fmt.Errorf("staff id is required")
	}

	var businessID uint
	err := s.db.GetGorm().Transaction(func(tx *gorm.DB) error {
		var staff database.Staff
		if err := tx.First(&staff, staffID).Error; err != nil {
			return fmt.Errorf("staff member not found: %w", err)
		}
		businessID = staff.BusinessID

		auditLog, err := mutate(tx, &staff)
		if err != nil {
			return err
		}

		// Mandatory audit — a failed audit write rolls back the mutation.
		if err := tx.Create(&auditLog).Error; err != nil {
			return fmt.Errorf("failed to write authorization audit record: %w", err)
		}

		return bumpAuthzVersionTx(tx, staffID)
	})
	if err != nil {
		return err
	}

	s.postAccessRevokeSideEffects(staffID, businessID, reason, changedBy)
	return nil
}

// postAccessRevokeSideEffects evicts the remaining live access channels (push
// subscriptions, scoped sessions, process-local SSE) AFTER the authorization
// transaction has committed. These are best-effort: the in-transaction
// authz_version bump is the authoritative revocation, so a failure here affects
// only eviction latency (bounded by the 15s SSE heartbeat), never correctness.
//
// DEFERRED (deployment / multi-replica): cross-instance LISTEN/NOTIFY for
// sub-second SSE eviction on OTHER replicas. authz_version is re-checked on every
// authenticated request and every 15s SSE heartbeat against the DB (source of
// truth), so revocation is guaranteed within one heartbeat on all replicas.
func (s *RBACService) postAccessRevokeSideEffects(staffID, businessID uint, reason, changedBy string) {
	if err := s.db.GetGorm().
		Where("principal_type = ? AND principal_id = ?", database.PushPrincipalStaff, staffID).
		Delete(&database.PushSubscription{}).Error; err != nil {
		logger.Logger.Warnf("postAccessRevokeSideEffects: push cleanup failed for staff %d: %v (reason=%s changed_by=%s)",
			staffID, err, reason, changedBy)
	}

	if session.GlobalStore != nil {
		if err := session.GlobalStore.RevokeAllForScopedUserWithReason(staffID, session.RevocationReasonAdministrative, staffSessionProviders...); err != nil {
			logger.Logger.Warnf("postAccessRevokeSideEffects: session revoke failed for staff %d: %v (reason=%s changed_by=%s)",
				staffID, err, reason, changedBy)
		}
	}

	// Process-local immediate SSE eviction on this instance.
	events.PublishStaffAccessRevoked(businessID, staffID)
}
