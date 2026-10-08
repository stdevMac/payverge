package main

import (
	"context"
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// showcaseStaffPinHash is the bcrypt of "showcase". Computed once at startup.
var showcaseStaffPinHash string

func init() {
	h, err := bcrypt.GenerateFromPassword([]byte("showcase"), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("showcase seed: bcrypt staff pin: %v", err)
	}
	showcaseStaffPinHash = string(h)
}

type staffSpec struct {
	Email string
	Name  string
	Role  database.StaffRole
	Level int
}

// roleLevel returns the RBAC hierarchy level matching backend conventions:
// manager=4, server=3, host=2, kitchen=1.
func roleLevel(r database.StaffRole) int {
	switch r {
	case database.StaffRoleManager:
		return 4
	case database.StaffRoleServer:
		return 3
	case database.StaffRoleHost:
		return 2
	case database.StaffRoleKitchen:
		return 1
	}
	return 0
}

func showcaseStaff() []staffSpec {
	return []staffSpec{
		{Email: "marco.rossi@trattoriabellavista.example", Name: "Marco Rossi", Role: database.StaffRoleManager},
		{Email: "sofia.conti@trattoriabellavista.example", Name: "Sofia Conti", Role: database.StaffRoleManager},
		{Email: "giulia.romano@trattoriabellavista.example", Name: "Giulia Romano", Role: database.StaffRoleServer},
		{Email: "luca.bianchi@trattoriabellavista.example", Name: "Luca Bianchi", Role: database.StaffRoleServer},
		{Email: "elena.marino@trattoriabellavista.example", Name: "Elena Marino", Role: database.StaffRoleServer},
		{Email: "antonio.greco@trattoriabellavista.example", Name: "Antonio Greco", Role: database.StaffRoleServer},
		{Email: "beatrice.romano@trattoriabellavista.example", Name: "Beatrice Romano", Role: database.StaffRoleHost},
		{Email: "paolo.ferrari@trattoriabellavista.example", Name: "Paolo Ferrari (Chef)", Role: database.StaffRoleKitchen},
		{Email: "davide.esposito@trattoriabellavista.example", Name: "Davide Esposito (Sous Chef)", Role: database.StaffRoleKitchen},
	}
}

// seedStaff upserts the 9 named staff. Dedupe key is the unique `email`
// column. PINs are bcrypt-hashed once at startup (constant "showcase").
func seedStaff(ctx context.Context, db *gorm.DB, bizID uint) error {
	specs := showcaseStaff()
	rows := make([]database.Staff, 0, len(specs))
	for _, s := range specs {
		rows = append(rows, database.Staff{
			BusinessID: bizID,
			Email:      s.Email,
			Name:       s.Name,
			Role:       s.Role,
			RoleLevel:  roleLevel(s.Role),
			InvitedBy:  PlaceholderAddress,
			IsActive:   true,
			PinHash:    showcaseStaffPinHash,
		})
	}
	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "email"}},
			DoNothing: true,
		}).
		CreateInBatches(rows, 100).Error; err != nil {
		return fmt.Errorf("create staff: %w", err)
	}
	return nil
}

// loadStaff returns the showcase staff so callers can reference them by
// ID when seeding bills (CreatedByStaffID, ClosedByStaffID).
func loadStaff(ctx context.Context, db *gorm.DB, bizID uint) ([]database.Staff, error) {
	var staff []database.Staff
	if err := db.WithContext(ctx).
		Where("business_id = ? AND email LIKE ?", bizID, "%@trattoriabellavista.example").
		Order("id").
		Find(&staff).Error; err != nil {
		return nil, fmt.Errorf("load staff: %w", err)
	}
	return staff, nil
}
