package demo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Seeded CRM points must track lifetime spend at the demo earn rate so the
// roster never shows points that ignore spend (the inverse-looking demo bug).
func TestEnsureCustomerLoyaltyPointsTrackSpend(t *testing.T) {
	db := newDemoServiceTestDB(t)
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})

	admin := database.User{Email: "demo-admin@example.com", Role: "admin"}
	require.NoError(t, db.Create(&admin).Error)
	business := database.Business{
		BusinessId: "loyalty-points-track",
		Name:       "Loyalty Points Track",
		IsActive:   true,
		UserID:     &admin.ID,
	}
	require.NoError(t, db.Create(&business).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.ensureLoyalty(context.Background(), tx, business.ID)
	}))

	var program database.LoyaltyProgram
	require.NoError(t, db.Where("business_id = ?", business.ID).First(&program).Error)
	// ARS: 0.01 points per peso earned, 1 point ≈ AR$1 on redemption.
	require.Equal(t, 0.01, program.PointsPerDollar)
	require.Equal(t, 1.0, program.RedemptionPointsPerDollar)

	var rows []database.CustomerBusiness
	for i := 0; i < 12; i++ {
		_, cb, err := svc.ensureCustomer(context.Background(), db, admin.ID, business.ID, i)
		require.NoError(t, err)
		rows = append(rows, *cb)
	}

	for _, row := range rows {
		expected := int(row.TotalSpent * program.PointsPerDollar)
		require.Equal(t, expected, row.LoyaltyPoints,
			"points must equal floor(spend × earn rate); spent=%.2f", row.TotalSpent)
	}

	// The specific inverse pair from the QA report: higher spend must mean
	// more (or equal) points when both rows use the same earn formula.
	var lowSpend, highSpend *database.CustomerBusiness
	for i := range rows {
		row := &rows[i]
		if lowSpend == nil || row.TotalSpent < lowSpend.TotalSpent {
			lowSpend = row
		}
		if highSpend == nil || row.TotalSpent > highSpend.TotalSpent {
			highSpend = row
		}
	}
	require.NotNil(t, lowSpend)
	require.NotNil(t, highSpend)
	require.Greater(t, highSpend.LoyaltyPoints, lowSpend.LoyaltyPoints)
}
