package demo

import (
	"context"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// The seeded schedule used positions[idx%len(positions)] — 5 staff, 4
// positions — so the 5th teammate ("Bea Server") got a Manager-position
// shift chip on the Schedule tab. Every demo shift must carry the staff
// member's own linked position.
func TestDemoShiftsCarryEachStaffMembersOwnPosition(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-shift@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var shifts []database.Shift
	require.NoError(t, db.Find(&shifts).Error)
	require.NotEmpty(t, shifts)

	assigned := 0
	for _, shift := range shifts {
		if shift.StaffID == nil {
			continue // open/unassigned demo shifts have no owner to match
		}
		assigned++
		var link database.StaffPosition
		require.NoError(
			t,
			db.Where("staff_id = ? AND business_id = ?", *shift.StaffID, shift.BusinessID).First(&link).Error,
			"staff %d has no position link", *shift.StaffID,
		)
		require.Equal(
			t, link.PositionID, shift.PositionID,
			"shift %d for staff %d carries position %d, but the staff member is linked to position %d",
			shift.ID, *shift.StaffID, shift.PositionID, link.PositionID,
		)
	}
	require.NotZero(t, assigned, "expected at least one assigned demo shift")
}

// The Director Console greets with the owner's first name; "Payverge Demo"
// produced "Good evening, Payverge." on the front door. The demo owner must
// read as a person, not the brand.
func TestDemoOwnerNameIsAHumanName(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-owner@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&businesses).Error)
	require.NotEmpty(t, businesses)
	for _, business := range businesses {
		first := strings.Fields(business.OwnerName)
		require.NotEmpty(t, first, "demo business %d has no owner name", business.BusinessId)
		require.NotEqual(t, "payverge", strings.ToLower(first[0]),
			"demo owner name %q greets as the brand, not a person", business.OwnerName)
	}
}
