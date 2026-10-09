package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BusinessService provides business-specific operations
type BusinessService struct {
	repo *Repository[Business]
}

// NewBusinessService creates a new business service
func NewBusinessService() *BusinessService {
	return &BusinessService{
		repo: NewRepository[Business](db),
	}
}

func (s *BusinessService) Create(business *Business) error {
	return s.repo.Create(business)
}

func (s *BusinessService) GetByID(id uint) (*Business, error) {
	return s.repo.GetByID(id)
}

func (s *BusinessService) Update(business *Business) error {
	return s.repo.Update(business)
}

func (s *BusinessService) SoftDelete(id uint) error {
	return s.repo.UpdateWhere("id = ?", map[string]interface{}{"is_active": false}, id)
}

// TableService provides table-specific operations
type TableService struct {
	repo *Repository[Table]
}

func NewTableService() *TableService {
	return &TableService{
		repo: NewRepository[Table](db),
	}
}

func (s *TableService) Create(table *Table) error {
	return s.repo.Create(table)
}

func (s *TableService) GetByID(id uint) (*Table, error) {
	return s.repo.GetByID(id)
}

func (s *TableService) GetByBusinessID(businessID uint) ([]Table, error) {
	return s.repo.GetWhere("business_id = ? AND is_active = ?", businessID, true)
}

func (s *TableService) Update(table *Table) error {
	return s.repo.Update(table)
}

func (s *TableService) SoftDelete(id uint) error {
	return s.repo.UpdateWhere("id = ?", map[string]interface{}{"is_active": false}, id)
}

// BillService provides bill-specific operations
type BillService struct {
	repo *Repository[Bill]
}

func NewBillService() *BillService {
	return &BillService{
		repo: NewRepository[Bill](db),
	}
}

func (s *BillService) Create(bill *Bill) error {
	return s.repo.Create(bill)
}

func (s *BillService) GetByID(id uint) (*Bill, error) {
	return s.repo.GetByID(id)
}

func (s *BillService) GetByDateRange(businessID uint, startDate, endDate time.Time) ([]Bill, error) {
	return s.repo.GetByDateRange("created_at", startDate, endDate, "business_id = ?", businessID)
}

func (s *BillService) Update(bill *Bill) error {
	return s.repo.Update(bill)
}

// PaymentService provides payment-specific operations
type PaymentService struct {
	repo *Repository[Payment]
}

func NewPaymentService() *PaymentService {
	return &PaymentService{
		repo: NewRepository[Payment](db),
	}
}

func (s *PaymentService) Create(payment *Payment) error {
	return s.repo.Create(payment)
}

func (s *PaymentService) GetByID(id uint) (*Payment, error) {
	return s.repo.GetByID(id)
}

func (s *PaymentService) GetByBillID(billID uint) ([]Payment, error) {
	return s.repo.GetWhere("bill_id = ?", billID)
}

func (s *PaymentService) GetByDateRange(businessID uint, startDate, endDate time.Time) ([]Payment, error) {
	var payments []Payment
	err := db.Joins("JOIN bills ON payments.bill_id = bills.id").
		Where("bills.business_id = ? AND payments.created_at >= ? AND payments.created_at < ?",
			businessID, startDate, endDate).
		Find(&payments).Error
	return payments, err
}

func (s *PaymentService) Update(payment *Payment) error {
	return s.repo.Update(payment)
}

// AlternativePaymentService provides alternative payment-specific operations
type AlternativePaymentService struct {
	repo *Repository[AlternativePayment]
}

var billManagedAlternativePaymentMethods = []AlternativePaymentMethod{
	PaymentMethodCash,
	PaymentMethodCard,
	PaymentMethodVenmo,
	PaymentMethodOther,
}

func NewAlternativePaymentService() *AlternativePaymentService {
	return &AlternativePaymentService{
		repo: NewRepository[AlternativePayment](db),
	}
}

func (s *AlternativePaymentService) Create(payment *AlternativePayment) error {
	return s.repo.Create(payment)
}

func (s *AlternativePaymentService) GetByID(id uint) (*AlternativePayment, error) {
	return s.repo.GetByID(id)
}

func (s *AlternativePaymentService) GetByBillID(billID uint) ([]AlternativePayment, error) {
	return s.repo.GetWhere("bill_id = ?", billID)
}

func (s *AlternativePaymentService) GetPendingByBillID(billID uint) ([]AlternativePayment, error) {
	payments, err := s.repo.GetWhere(
		"bill_id = ? AND status = ? AND payment_method IN ? AND (expires_at IS NULL OR expires_at > ?)",
		billID,
		AltPaymentStatusPending,
		billManagedAlternativePaymentMethods,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	for i := range payments {
		exp := AlternativePaymentRequestExpiresAt(payments[i])
		payments[i].ExpiresAt = &exp
	}
	return payments, nil
}

func (s *AlternativePaymentService) GetConfirmedByBillID(billID uint) ([]AlternativePayment, error) {
	return s.repo.GetWhere(
		"bill_id = ? AND status = ? AND payment_method IN ?",
		billID,
		AltPaymentStatusConfirmed,
		billManagedAlternativePaymentMethods,
	)
}

func (s *AlternativePaymentService) GetConfirmedTotalByBillID(billID uint) (int64, error) {
	var total int64
	err := s.repo.db.Model(&AlternativePayment{}).
		Select("COALESCE(SUM(amount), 0)").
		Where(
			"bill_id = ? AND status = ? AND payment_method IN ?",
			billID,
			AltPaymentStatusConfirmed,
			billManagedAlternativePaymentMethods,
		).
		Scan(&total).Error
	return total, err
}

func (s *AlternativePaymentService) Update(payment *AlternativePayment) error {
	return s.repo.Update(payment)
}

func (s *AlternativePaymentService) GetByDateRange(businessID uint, startDate, endDate time.Time) ([]AlternativePayment, error) {
	var payments []AlternativePayment
	err := s.repo.db.Where(`
		bill_id IN (
			SELECT id FROM bills 
			WHERE business_id = ? 
			AND created_at >= ? 
			AND created_at < ?
		) AND status = ? AND payment_method IN ?
	`, businessID, startDate, endDate, AltPaymentStatusConfirmed, billManagedAlternativePaymentMethods).Limit(1000).Find(&payments).Error

	return payments, err
}

// MenuService provides menu-specific operations with JSON handling
type MenuService struct {
	repo *Repository[Menu]
}

func NewMenuService() *MenuService {
	return &MenuService{
		repo: NewRepository[Menu](db),
	}
}

func (s *MenuService) Create(menu *Menu, categories []MenuCategory) error {
	ensureMenuEntityIDs(categories)
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		return err
	}
	menu.Categories = string(categoriesJSON)
	return s.repo.Create(menu)
}

func (s *MenuService) GetByBusinessID(businessID uint) (*Menu, []MenuCategory, error) {
	menu, err := s.repo.GetFirstWhere("business_id = ? AND is_active = ?", businessID, true)
	if err != nil {
		return nil, nil, err
	}

	var categories []MenuCategory
	if err := json.Unmarshal([]byte(menu.Categories), &categories); err != nil {
		return nil, nil, err
	}

	return menu, categories, nil
}

func (s *MenuService) Update(menu *Menu, categories []MenuCategory) error {
	ensureMenuEntityIDs(categories)
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		return err
	}
	menu.Categories = string(categoriesJSON)
	return s.repo.Update(menu)
}

// StaffService provides staff-specific operations
type StaffService struct {
	repo *Repository[Staff]
}

var (
	ErrStaffMembershipActive          = errors.New("staff membership is already active")
	ErrStaffMembershipSelectionNeeded = errors.New("staff membership selection is required")
)

type StaffInvitationAcceptance struct {
	Staff         *Staff
	Created       bool
	PreviousStaff *Staff
	AuditLogID    uint
}

var staffMembershipSchemaState struct {
	sync.Mutex
	db        *gorm.DB
	checked   bool
	available bool
}

func staffMembershipSchemaAvailable(conn *gorm.DB) bool {
	staffMembershipSchemaState.Lock()
	defer staffMembershipSchemaState.Unlock()
	if staffMembershipSchemaState.db == conn && staffMembershipSchemaState.checked {
		return staffMembershipSchemaState.available
	}
	available := conn != nil && conn.Migrator().HasTable(&StaffIdentity{}) && conn.Migrator().HasTable(&StaffMembership{})
	staffMembershipSchemaState.db = conn
	staffMembershipSchemaState.checked = true
	staffMembershipSchemaState.available = available
	return available
}

func NewStaffService() *StaffService {
	return &StaffService{
		repo: NewRepository[Staff](db),
	}
}

func (s *StaffService) Create(staff *Staff) error {
	staff.Email = normalizeStaffEmail(staff.Email)
	return s.repo.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(staff).Error; err != nil {
			return fmt.Errorf("failed to create record: %w", err)
		}
		return syncStaffMembershipTx(tx, staff)
	})
}

func (s *StaffService) GetByID(id uint) (*Staff, error) {
	staff, err := s.repo.GetByID(id)
	if err != nil || !staffMembershipSchemaAvailable(s.repo.db) {
		return staff, err
	}
	var membership StaffMembership
	if membershipErr := s.repo.db.Where("legacy_staff_id = ?", id).Take(&membership).Error; membershipErr != nil {
		if errors.Is(membershipErr, gorm.ErrRecordNotFound) {
			return staff, nil
		}
		return nil, fmt.Errorf("failed to load staff membership: %w", membershipErr)
	}
	staff.BusinessID = membership.BusinessID
	staff.Name = membership.Name
	staff.Role = membership.Role
	staff.CustomPermissions = membership.CustomPermissions
	staff.RoleLevel = membership.RoleLevel
	staff.IsActive = staff.IsActive && membership.IsActive
	if membership.AuthzVersion > staff.AuthzVersion {
		staff.AuthzVersion = membership.AuthzVersion
	}
	staff.InvitedBy = membership.InvitedBy
	staff.PermissionsUpdatedAt = membership.PermissionsUpdatedAt
	staff.PermissionsUpdatedBy = membership.PermissionsUpdatedBy
	return staff, nil
}

func (s *StaffService) GetByEmail(email string) (*Staff, error) {
	// Match case-insensitively and tolerate stray whitespace. Login/OAuth callers
	// already pass a normalized (lower-cased, trimmed) address, but rows created
	// before email normalization existed — or via any path that skipped it — may
	// be stored with mixed case. A plain `email = ?` is case-sensitive on
	// Postgres, so those rows are invisible to the lookup and the staff member is
	// silently locked out of email-code and Google login (request-login-code then
	// reports "record not found" while still returning 200). Normalizing both
	// sides of the comparison closes that gap.
	normalized := strings.ToLower(strings.TrimSpace(email))
	active, err := s.GetActiveByEmail(normalized)
	if err != nil {
		return nil, err
	}
	if len(active) > 1 {
		return nil, ErrStaffMembershipSelectionNeeded
	}
	if len(active) == 1 {
		return &active[0], nil
	}
	var matches []Staff
	if err := s.repo.db.Where("LOWER(TRIM(email)) = ?", normalized).Order("id").Find(&matches).Error; err != nil {
		return nil, fmt.Errorf("failed to get record: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("record not found")
	}
	return s.GetByID(matches[0].ID)
}

func (s *StaffService) GetByBusinessAndEmail(businessID uint, email string) (*Staff, error) {
	normalized := normalizeStaffEmail(email)
	if staffMembershipSchemaAvailable(s.repo.db) {
		var identity StaffIdentity
		identityErr := s.repo.db.Where("normalized_email = ?", normalized).Take(&identity).Error
		if identityErr == nil {
			var membership StaffMembership
			if err := s.repo.db.Where("identity_id = ? AND business_id = ?", identity.ID, businessID).Take(&membership).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, fmt.Errorf("record not found")
				}
				return nil, fmt.Errorf("failed to get staff membership: %w", err)
			}
			return s.GetByID(membership.LegacyStaffID)
		}
		if !errors.Is(identityErr, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to get staff identity: %w", identityErr)
		}
	}
	return s.repo.GetFirstWhere("business_id = ? AND LOWER(TRIM(email)) = ?", businessID, normalized)
}

func (s *StaffService) GetActiveByEmail(email string) ([]Staff, error) {
	normalized := normalizeStaffEmail(email)
	if staffMembershipSchemaAvailable(s.repo.db) {
		var identity StaffIdentity
		identityErr := s.repo.db.Where("normalized_email = ?", normalized).Take(&identity).Error
		if identityErr == nil {
			var memberships []StaffMembership
			if err := s.repo.db.Where("identity_id = ? AND is_active = ?", identity.ID, true).Order("id").Find(&memberships).Error; err != nil {
				return nil, fmt.Errorf("failed to get staff memberships: %w", err)
			}
			staff := make([]Staff, 0, len(memberships))
			for i := range memberships {
				member, err := s.GetByID(memberships[i].LegacyStaffID)
				if err != nil {
					return nil, err
				}
				if member.IsActive {
					staff = append(staff, *member)
				}
			}
			return staff, nil
		}
		if !errors.Is(identityErr, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to get staff identity: %w", identityErr)
		}
	}
	var staff []Staff
	if err := s.repo.db.Where("LOWER(TRIM(email)) = ? AND is_active = ?", normalized, true).
		Order("id").Find(&staff).Error; err != nil {
		return nil, fmt.Errorf("failed to get records: %w", err)
	}
	return staff, nil
}

func (s *StaffService) GetByBusinessID(businessID uint) ([]Staff, error) {
	return s.repo.GetWhere("business_id = ? AND is_active = ?", businessID, true)
}

// GetAllByBusinessID returns every membership for owner-facing lifecycle
// management, including deactivated staff who must remain reachable for
// reactivation. Operational callers should keep using GetByBusinessID, whose
// active-only semantics prevent inactive staff from entering live workflows.
func (s *StaffService) GetAllByBusinessID(businessID uint) ([]Staff, error) {
	return s.repo.GetWhere("business_id = ?", businessID)
}

func (s *StaffService) Update(staff *Staff) error {
	staff.Email = normalizeStaffEmail(staff.Email)
	return s.repo.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(staff).Updates(staff).Error; err != nil {
			return fmt.Errorf("failed to update record: %w", err)
		}
		var current Staff
		if err := tx.First(&current, staff.ID).Error; err != nil {
			return err
		}
		return syncStaffMembershipTx(tx, &current)
	})
}

func (s *StaffService) UpdateLastLogin(staffID uint) error {
	now := time.Now()
	return s.repo.UpdateWhere("id = ?", map[string]interface{}{"last_login_at": &now}, staffID)
}

func (s *StaffService) SoftDelete(id uint) error {
	return s.repo.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Staff{}).Where("id = ?", id).Update("is_active", false).Error; err != nil {
			return fmt.Errorf("failed to update records: %w", err)
		}
		if tx.Migrator().HasTable(&StaffMembership{}) {
			if err := tx.Model(&StaffMembership{}).Where("legacy_staff_id = ?", id).
				Update("is_active", false).Error; err != nil {
				return fmt.Errorf("failed to deactivate staff membership: %w", err)
			}
		}
		return nil
	})
}

func normalizeStaffEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func syncStaffMembershipTx(tx *gorm.DB, staff *Staff) error {
	normalized := normalizeStaffEmail(staff.Email)
	identity := StaffIdentity{NormalizedEmail: normalized}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "normalized_email"}},
		DoNothing: true,
	}).Create(&identity).Error; err != nil {
		return fmt.Errorf("sync staff identity: %w", err)
	}
	if identity.ID == 0 {
		if err := tx.Where("normalized_email = ?", normalized).Take(&identity).Error; err != nil {
			return fmt.Errorf("load staff identity: %w", err)
		}
	}
	authzVersion := staff.AuthzVersion
	if authzVersion < 1 {
		authzVersion = 1
	}
	membership := StaffMembership{
		IdentityID: identity.ID, BusinessID: staff.BusinessID, LegacyStaffID: staff.ID,
		Name: staff.Name, Role: staff.Role, CustomPermissions: staff.CustomPermissions,
		RoleLevel: staff.RoleLevel, IsActive: staff.IsActive, AuthzVersion: authzVersion,
		InvitedBy: staff.InvitedBy, PermissionsUpdatedAt: staff.PermissionsUpdatedAt,
		PermissionsUpdatedBy: staff.PermissionsUpdatedBy,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "legacy_staff_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"identity_id", "business_id", "name", "role", "custom_permissions",
			"role_level", "is_active", "authz_version", "invited_by",
			"permissions_updated_at", "permissions_updated_by", "updated_at",
		}),
	}).Create(&membership).Error; err != nil {
		return err
	}
	// GORM applies default:true to a false bool during Create, including the
	// EXCLUDED row used by an upsert. Write the source-of-truth projection
	// explicitly so inactive memberships stay inactive during compensation and
	// legacy backfill.
	return tx.Model(&StaffMembership{}).Where("legacy_staff_id = ?", staff.ID).Updates(map[string]interface{}{
		"identity_id": identity.ID, "business_id": staff.BusinessID, "name": staff.Name,
		"role": staff.Role, "custom_permissions": staff.CustomPermissions,
		"role_level": staff.RoleLevel, "is_active": staff.IsActive,
		"authz_version": authzVersion, "invited_by": staff.InvitedBy,
		"permissions_updated_at": staff.PermissionsUpdatedAt,
		"permissions_updated_by": staff.PermissionsUpdatedBy,
	}).Error
}

// AcceptStaffInvitation creates a new scoped legacy staff row or reactivates an
// inactive same-business row. Identity/membership projection, invitation state,
// auth-version bump, and rehire audit are committed atomically.
func (s *StaffService) AcceptStaffInvitation(invitationID uint, displayName string) (*StaffInvitationAcceptance, error) {
	result := &StaffInvitationAcceptance{}
	err := s.repo.db.Transaction(func(tx *gorm.DB) error {
		var invitation StaffInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invitation, invitationID).Error; err != nil {
			return err
		}
		if invitation.Status != InvitationStatusPending || time.Now().After(invitation.ExpiresAt) {
			return fmt.Errorf("invitation is not pending")
		}

		var existing Staff
		err := tx.Where("business_id = ? AND LOWER(TRIM(email)) = ?", invitation.BusinessID, normalizeStaffEmail(invitation.Email)).First(&existing).Error
		switch {
		case err == nil && existing.IsActive:
			return ErrStaffMembershipActive
		case err == nil:
			previous := existing
			result.PreviousStaff = &previous
			if err := tx.Model(&Staff{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
				"email": normalizeStaffEmail(invitation.Email), "name": strings.TrimSpace(displayName),
				"role": invitation.Role, "is_active": true, "invited_by": invitation.InvitedBy,
				"authz_version":              gorm.Expr("authz_version + 1"),
				"login_code_failed_attempts": 0, "login_code_locked_until": nil,
			}).Error; err != nil {
				return err
			}
			if err := tx.First(&existing, existing.ID).Error; err != nil {
				return err
			}
			result.Staff = &existing
			audit := RBACAuditLog{
				StaffID: existing.ID, BusinessID: existing.BusinessID,
				Action: RBACActionStaffReactivated, OldRole: string(previous.Role),
				NewRole: string(existing.Role), OldPermissions: previous.CustomPermissions,
				NewPermissions: existing.CustomPermissions, ChangedBy: invitation.InvitedBy,
				Reason: "staff invitation accepted after removal",
			}
			if err := tx.Create(&audit).Error; err != nil {
				return fmt.Errorf("write staff reactivation audit: %w", err)
			}
			result.AuditLogID = audit.ID
		case errors.Is(err, gorm.ErrRecordNotFound):
			staff := &Staff{
				BusinessID: invitation.BusinessID, Email: normalizeStaffEmail(invitation.Email),
				Name: strings.TrimSpace(displayName), Role: invitation.Role, IsActive: true,
				InvitedBy: invitation.InvitedBy, AuthzVersion: 1,
			}
			if err := tx.Create(staff).Error; err != nil {
				return err
			}
			result.Staff = staff
			result.Created = true
		default:
			return err
		}

		if err := syncStaffMembershipTx(tx, result.Staff); err != nil {
			return err
		}
		return tx.Model(&StaffInvitation{}).Where("id = ?", invitation.ID).
			Update("status", InvitationStatusAccepted).Error
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RestoreInvitationAcceptance compensates when session creation fails after a
// successful DB acceptance, retaining the legacy behavior without deleting a
// reactivated historical staff row.
func (s *StaffService) RestoreInvitationAcceptance(invitationID uint, accepted *StaffInvitationAcceptance) error {
	if accepted == nil || accepted.Staff == nil {
		return nil
	}
	return s.repo.db.Transaction(func(tx *gorm.DB) error {
		if accepted.AuditLogID != 0 {
			if err := tx.Delete(&RBACAuditLog{}, accepted.AuditLogID).Error; err != nil {
				return err
			}
		}
		if accepted.Created {
			if tx.Migrator().HasTable(&StaffMembership{}) {
				if err := tx.Where("legacy_staff_id = ?", accepted.Staff.ID).Delete(&StaffMembership{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Delete(&Staff{}, accepted.Staff.ID).Error; err != nil {
				return err
			}
		} else if accepted.PreviousStaff != nil {
			previous := accepted.PreviousStaff
			if err := tx.Model(&Staff{}).Where("id = ?", previous.ID).Select("*").Updates(previous).Error; err != nil {
				return err
			}
			if err := syncStaffMembershipTx(tx, previous); err != nil {
				return err
			}
		}
		return tx.Model(&StaffInvitation{}).Where("id = ?", invitationID).
			Update("status", InvitationStatusPending).Error
	})
}

// StaffLoginMaxFailedAttempts is the number of failed email-code verifications,
// summed over every client, allowed before a staff member is temporarily
// locked out. The login handler stops each (client, email) pair after 5
// failures, so reaching this identity-wide lockout takes several distinct
// networks; a single stranger can no longer lock a staff member out. Each
// 10-minute six-digit code still faces at most this many guesses.
const StaffLoginMaxFailedAttempts = 20

// StaffLoginLockoutWindow is how long a staff member stays locked out after
// exceeding StaffLoginMaxFailedAttempts.
const StaffLoginLockoutWindow = 15 * time.Minute

// IsLoginLocked reports whether staff is currently locked out and, if so, until
// when. Used to block code-verification attempts regardless of source IP.
func (s *StaffService) IsLoginLocked(staffID uint) (bool, *time.Time, error) {
	staff, err := s.repo.GetByID(staffID)
	if err != nil {
		return false, nil, err
	}
	if staff.LoginCodeLockedUntil != nil && staff.LoginCodeLockedUntil.After(time.Now()) {
		return true, staff.LoginCodeLockedUntil, nil
	}
	return false, nil, nil
}

// RegisterFailedLoginAttempt atomically increments the failed-attempt counter
// and, once the threshold is reached, sets a lockout window and invalidates any
// outstanding active login codes for that staff member. Returns whether the
// staff is now locked.
func (s *StaffService) RegisterFailedLoginAttempt(staffID uint) (bool, error) {
	var locked bool
	err := s.repo.db.Transaction(func(tx *gorm.DB) error {
		var staff Staff
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&staff, staffID).Error; err != nil {
			return err
		}
		attempts := staff.LoginCodeFailedAttempts + 1
		updates := map[string]interface{}{"login_code_failed_attempts": attempts}
		if attempts >= StaffLoginMaxFailedAttempts {
			until := time.Now().Add(StaffLoginLockoutWindow)
			updates["login_code_locked_until"] = &until
			locked = true
			// Burn outstanding codes so a lockout can't be waited out and then
			// resumed against the same still-valid code.
			if err := tx.Model(&StaffLoginCode{}).
				Where("staff_id = ? AND used = ? AND expires_at > ?", staffID, false, time.Now()).
				Update("used", true).Error; err != nil {
				return err
			}
		}
		return tx.Model(&Staff{}).Where("id = ?", staffID).Updates(updates).Error
	})
	return locked, err
}

// ReserveLoginAttempt atomically claims one code-verification attempt for
// every given staff row (all memberships of one email) BEFORE the code is
// compared, so concurrent requests cannot exceed StaffLoginMaxFailedAttempts.
// Each row is claimed with a single conditional UPDATE that refuses while the
// row is locked, restarts the count once a lockout has expired, and sets the
// lockout on the attempt that reaches the ceiling. The claims commit together
// or not at all. Returns false when any row is locked (the caller must refuse
// the attempt). This ceiling is keyed on the staff identity, not the client,
// so rotating source addresses cannot buy more guesses.
func (s *StaffService) ReserveLoginAttempt(staffIDs []uint) (bool, error) {
	if len(staffIDs) == 0 {
		return false, nil
	}
	allowed := true
	err := s.repo.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		until := now.Add(StaffLoginLockoutWindow)
		for _, id := range staffIDs {
			res := tx.Model(&Staff{}).
				Where("id = ? AND (login_code_locked_until IS NULL OR login_code_locked_until <= ?)", id, now).
				Updates(map[string]interface{}{
					"login_code_failed_attempts": gorm.Expr(
						"CASE WHEN login_code_locked_until IS NOT NULL THEN 1 ELSE COALESCE(login_code_failed_attempts, 0) + 1 END"),
					"login_code_locked_until": nil,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				allowed = false
				return errStaffLoginReservationRefused
			}
			// The attempt that reaches the ceiling locks the row. The UPDATE
			// above holds the row lock until commit, so this cannot interleave
			// with another reservation for the same row.
			if err := tx.Model(&Staff{}).
				Where("id = ? AND login_code_failed_attempts >= ?", id, StaffLoginMaxFailedAttempts).
				Update("login_code_locked_until", until).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errStaffLoginReservationRefused) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return allowed, nil
}

var errStaffLoginReservationRefused = errors.New("staff login attempt refused: locked")

// BurnCodesIfLoginLocked marks every outstanding login code used for the given
// staff rows that are currently locked out, so a lockout reached by a failed
// attempt cannot be waited out and then resumed against the same code.
func (s *StaffService) BurnCodesIfLoginLocked(staffIDs []uint) error {
	if len(staffIDs) == 0 {
		return nil
	}
	now := time.Now()
	locked := s.repo.db.Model(&Staff{}).Select("id").
		Where("id IN ? AND login_code_locked_until > ?", staffIDs, now)
	return s.repo.db.Model(&StaffLoginCode{}).
		Where("staff_id IN (?) AND used = ? AND expires_at > ?", locked, false, now).
		Update("used", true).Error
}

// ResetLoginAttempts clears the failed-attempt counter and any lockout after a
// successful login.
func (s *StaffService) ResetLoginAttempts(staffID uint) error {
	return s.repo.UpdateWhere("id = ?", map[string]interface{}{
		"login_code_failed_attempts": 0,
		"login_code_locked_until":    nil,
	}, staffID)
}

// StaffInvitationService provides staff invitation operations
type StaffInvitationService struct {
	repo *Repository[StaffInvitation]
}

func NewStaffInvitationService() *StaffInvitationService {
	return &StaffInvitationService{
		repo: NewRepository[StaffInvitation](db),
	}
}

func (s *StaffInvitationService) Create(invitation *StaffInvitation) error {
	return s.repo.Create(invitation)
}

func (s *StaffInvitationService) GetByID(id uint) (*StaffInvitation, error) {
	return s.repo.GetByID(id)
}

func (s *StaffInvitationService) GetByToken(token string) (*StaffInvitation, error) {
	return s.repo.GetFirstWhere("token = ? AND status = ?", token, InvitationStatusPending)
}

func (s *StaffInvitationService) GetByBusinessID(businessID uint) ([]StaffInvitation, error) {
	return s.repo.GetWhere("business_id = ?", businessID)
}

func (s *StaffInvitationService) Update(invitation *StaffInvitation) error {
	return s.repo.Update(invitation)
}

func (s *StaffInvitationService) UpdateStatus(id uint, status InvitationStatus) error {
	return s.repo.UpdateWhere("id = ?", map[string]interface{}{"status": status}, id)
}

// StaffLoginCodeService provides login code operations
type StaffLoginCodeService struct {
	repo *Repository[StaffLoginCode]
}

func NewStaffLoginCodeService() *StaffLoginCodeService {
	return &StaffLoginCodeService{
		repo: NewRepository[StaffLoginCode](db),
	}
}

func (s *StaffLoginCodeService) Create(loginCode *StaffLoginCode) error {
	return s.repo.Create(loginCode)
}

func (s *StaffLoginCodeService) GetByStaffAndCode(staffID uint, code string) (*StaffLoginCode, error) {
	return s.repo.GetFirstWhere("staff_id = ? AND code = ? AND used = ? AND expires_at > ?", staffID, code, false, time.Now())
}

func (s *StaffLoginCodeService) ActivateCodeForStaff(staffID, loginCodeID uint) error {
	return s.repo.db.Transaction(func(tx *gorm.DB) error {
		var staff Staff
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&staff, staffID).Error; err != nil {
			return err
		}

		if err := tx.Model(&StaffLoginCode{}).
			Where("staff_id = ? AND id <> ? AND used = ? AND expires_at > ?", staffID, loginCodeID, false, time.Now()).
			Updates(map[string]interface{}{"used": true}).Error; err != nil {
			return err
		}

		result := tx.Model(&StaffLoginCode{}).
			Where("id = ? AND staff_id = ? AND expires_at > ?", loginCodeID, staffID, time.Now()).
			Update("used", false)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("login code not found")
		}

		return nil
	})
}

func (s *StaffLoginCodeService) ConsumeActiveCodesByStaffAndCode(staffID uint, code string) (bool, error) {
	result := s.repo.db.Model(&StaffLoginCode{}).
		Where("staff_id = ? AND code = ? AND used = ? AND expires_at > ?", staffID, code, false, time.Now()).
		Updates(map[string]interface{}{"used": true})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (s *StaffLoginCodeService) Delete(id uint) error {
	return s.repo.Delete(id)
}

func (s *StaffLoginCodeService) DeleteExpiredCodes() error {
	return s.repo.DeleteWhere("expires_at < ?", time.Now())
}
