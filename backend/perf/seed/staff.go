package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// roleAssignment cycles through the four staff roles so each seeded business
// has a representative mix. The RoleLevel mirrors the hierarchy used by RBAC
// helpers elsewhere in the codebase (manager > server > host > kitchen).
var roleAssignment = []struct {
	role  database.StaffRole
	level int
}{
	{database.StaffRoleManager, 4},
	{database.StaffRoleServer, 3},
	{database.StaffRoleHost, 2},
	{database.StaffRoleKitchen, 1},
}

// perfStaffPinHash is the bcrypt of "perfseed", computed once at startup so
// the seeder doesn't pay the 60-100ms bcrypt cost per staff row.
var perfStaffPinHash string

func init() {
	h, err := bcrypt.GenerateFromPassword([]byte("perfseed"), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("perf seed: bcrypt staff pin: %v", err)
	}
	perfStaffPinHash = string(h)
}

// seedStaff inserts `staffPerBiz` staff rows per perf business, deduped by
// email. Roles round-robin through manager/server/host/kitchen.
func seedStaff(ctx context.Context, db *gorm.DB, n, staffPerBiz int) error {
	biz, err := loadSeededBusinesses(ctx, db)
	if err != nil {
		return err
	}

	rows := make([]database.Staff, 0, len(biz)*staffPerBiz)
	for i, b := range biz {
		idx := i + 1 // 1-indexed
		for s := 1; s <= staffPerBiz; s++ {
			ra := roleAssignment[(s-1)%len(roleAssignment)]
			rows = append(rows, database.Staff{
				BusinessID: b.ID,
				Email:      fmt.Sprintf("perf-seed-staff-%03d-%d@example.test", idx, s),
				Name:       fmt.Sprintf("Staff %d-%d", idx, s),
				Role:       ra.role,
				RoleLevel:  ra.level,
				InvitedBy:  PlaceholderAddress,
				IsActive:   true,
				PinHash:    perfStaffPinHash,
			})
		}
	}

	// Email is intentionally not globally unique: one identity can belong to
	// multiple businesses. The fixture key is therefore (business_id, email),
	// and FirstOrCreate avoids relying on a nonexistent global email constraint.
	for i := range rows {
		row := rows[i]
		if err := db.WithContext(ctx).
			Where("business_id = ? AND email = ?", row.BusinessID, row.Email).
			FirstOrCreate(&row).Error; err != nil {
			return fmt.Errorf("create staff %s: %w", row.Email, err)
		}
	}
	if err := seedStaffMemberships(ctx, db); err != nil {
		return err
	}
	_ = n
	return nil
}

// seedStaffMemberships mirrors the production expand-schema dual write. A
// direct fixture insert into legacy `staff` alone is no longer sufficient once
// the normalized identity tables exist: staff login reads those tables first.
func seedStaffMemberships(ctx context.Context, db *gorm.DB) error {
	if !db.Migrator().HasTable(&database.StaffIdentity{}) || !db.Migrator().HasTable(&database.StaffMembership{}) {
		return nil
	}
	var staff []database.Staff
	if err := db.WithContext(ctx).Where("email LIKE ?", "perf-seed-staff-%").Order("id").Find(&staff).Error; err != nil {
		return fmt.Errorf("load staff memberships: %w", err)
	}
	for i := range staff {
		s := staff[i]
		identity := database.StaffIdentity{NormalizedEmail: strings.ToLower(strings.TrimSpace(s.Email))}
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "normalized_email"}}, DoNothing: true,
		}).Create(&identity).Error; err != nil {
			return fmt.Errorf("create staff identity %s: %w", s.Email, err)
		}
		if identity.ID == 0 {
			if err := db.WithContext(ctx).Where("normalized_email = ?", identity.NormalizedEmail).First(&identity).Error; err != nil {
				return fmt.Errorf("reload staff identity %s: %w", s.Email, err)
			}
		}
		authzVersion := s.AuthzVersion
		if authzVersion < 1 {
			authzVersion = 1
		}
		membership := database.StaffMembership{
			IdentityID: identity.ID, BusinessID: s.BusinessID, LegacyStaffID: s.ID,
			Name: s.Name, Role: s.Role, CustomPermissions: s.CustomPermissions,
			RoleLevel: s.RoleLevel, IsActive: s.IsActive, AuthzVersion: authzVersion,
			InvitedBy: s.InvitedBy,
		}
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "legacy_staff_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"identity_id", "business_id", "name", "role", "custom_permissions",
				"role_level", "is_active", "authz_version", "invited_by", "updated_at",
			}),
		}).Create(&membership).Error; err != nil {
			return fmt.Errorf("upsert staff membership %s: %w", s.Email, err)
		}
	}
	return nil
}
