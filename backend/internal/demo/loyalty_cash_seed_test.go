package demo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// F5: reseeding the loyalty ladder must be idempotent and self-heal a
// pre-existing duplicate, always yielding exactly the three seeded tiers.
func TestEnsureLoyaltyIsIdempotentAndDedupes(t *testing.T) {
	db := newDemoServiceTestDB(t)
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})

	business := database.Business{BusinessId: "loyalty-idem", Name: "Loyalty Idem", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	// First seed.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.ensureLoyalty(context.Background(), tx, business.ID)
	}))

	var program database.LoyaltyProgram
	require.NoError(t, db.Where("business_id = ?", business.ID).First(&program).Error)

	// Simulate data drift: an orphan duplicate "Bronze" row at $0.
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Bronze",
		MinLifetimeSpentCents: 0,
		SortOrder:             99,
		Color:                 "#000000",
	}).Error)

	var drifted int64
	require.NoError(t, db.Model(&database.LoyaltyTier{}).Where("loyalty_program_id = ?", program.ID).Count(&drifted).Error)
	require.Equal(t, int64(4), drifted, "precondition: duplicate Bronze present")

	// Reseed: the duplicate must be cleaned and the ladder restored to 3 tiers.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.ensureLoyalty(context.Background(), tx, business.ID)
	}))

	var tiers []database.LoyaltyTier
	require.NoError(t, db.Where("loyalty_program_id = ?", program.ID).Order("min_lifetime_spent_cents asc").Find(&tiers).Error)
	require.Len(t, tiers, 3, "reseeding twice must yield exactly 3 tiers")
	require.Equal(t, "Bronce", tiers[0].Name)
	require.Equal(t, int64(0), tiers[0].MinLifetimeSpentCents)
	require.Equal(t, 0, tiers[0].SortOrder)
	require.Equal(t, "Plata", tiers[1].Name)
	require.Equal(t, int64(15000000), tiers[1].MinLifetimeSpentCents)
	require.Equal(t, 1, tiers[1].SortOrder)
	require.Equal(t, "Oro", tiers[2].Name)
	require.Equal(t, int64(45000000), tiers[2].MinLifetimeSpentCents)
	require.Equal(t, 2, tiers[2].SortOrder)
}

// F19: a seeded cash-register session for today must never close in the future.
func TestGenerateCashSessionClampsClosedAtToNow(t *testing.T) {
	db := newDemoServiceTestDB(t)

	// "Now" is today at 10:00 — before the nominal 23:00 close, so the clamp
	// must fire for today's session.
	now := time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})

	business := database.Business{BusinessId: "cash-clamp", Name: "Cash Clamp", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	today := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.generateCashSessionForDay(context.Background(), tx, business.ID, today)
	}))

	var session database.CashRegisterSession
	require.NoError(t, db.Where("business_id = ?", business.ID).First(&session).Error)
	require.NotNil(t, session.ClosedAt)
	require.False(t, session.ClosedAt.After(now), "closed_at must not be in the future")
	require.False(t, session.ClosedAt.Before(session.OpenedAt), "closed_at must be >= opened_at")

	// Movements must stay within the (clamped) session window.
	var movements []database.CashRegisterMovement
	require.NoError(t, db.Where("session_id = ?", session.ID).Find(&movements).Error)
	for _, m := range movements {
		require.False(t, m.OccurredAt.After(*session.ClosedAt), "movement must not occur after close")
	}
}
