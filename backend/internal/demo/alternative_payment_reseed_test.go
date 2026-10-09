package demo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// L2-1: reseeding the same demo day must not stack duplicate confirmed
// alternative payments on a bill. The day generator builds a deterministic
// idempotency key per bill, so a second run has to dedupe on it exactly like
// the pending-payment seeder in generator.go does.
func TestCreateConfirmedAlternativePaymentIsReseedIdempotent(t *testing.T) {
	db := newDemoServiceTestDB(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed"})

	business := database.Business{BusinessId: "alt-reseed", Name: "Alt Reseed", IsActive: true}
	require.NoError(t, db.Create(&business).Error)
	bill := database.Bill{BusinessID: business.ID, BillNumber: "B-alt-reseed-1", Status: "paid", TotalAmount: 2500}
	require.NoError(t, db.Create(&bill).Error)

	confirmedAt := now.Add(-1 * time.Hour)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			return svc.createConfirmedAlternativePayment(context.Background(), tx, &bill, 2500, database.PaymentMethodCash, confirmedAt)
		}))
	}

	var count int64
	require.NoError(t, db.Model(&database.AlternativePayment{}).
		Where("bill_id = ?", bill.ID).Count(&count).Error)
	require.Equal(t, int64(1), count,
		"a reseed replays the same deterministic key and must not duplicate the confirmed payment")
}
