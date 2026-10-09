package demo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Live regression (2026-08): production demo instances on the current seed
// version died mid-hourly-append with a duplicate on
// orders_guest_request_identity_uq. The awaiting-payment
// delivery fixture minted a #772 successor bill after the expiry sweeper
// killed the original, but the successor's order reused the per-business
// constant client_request_id while the dead leftover's order still held it.
// The plain SQLite harness never creates the partial unique index, so the
// suite stayed green while every prod append aborted. This test installs the
// production index and walks two sweeper-kill → append cycles.
func TestSuccessorDeliveryPayOrdersSurviveGuestRequestIdentityIndex(t *testing.T) {
	db := newDemoServiceTestDB(t)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX orders_guest_request_identity_uq
		ON orders (business_id, created_by, client_request_id)
		WHERE created_by = 'guest' AND client_request_id IS NOT NULL`).Error)

	admin := seedAdmin(t, db, "delivery-pay-request-id@example.com")
	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 3})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)

	killFixtures := func(diedAt time.Time) {
		t.Helper()
		require.NoError(t, db.Model(&database.Bill{}).
			Where("business_id IN ? AND notes = ? AND status = ?",
				businessIDs, "Demo delivery bill awaiting online payment", database.BillStatusOpen).
			Updates(map[string]interface{}{
				"status":       database.BillStatusAbandoned,
				"abandoned_at": diedAt,
				"closed_at":    diedAt,
				"updated_at":   diedAt,
			}).Error)
		require.NoError(t, db.Model(&database.DeliveryOrder{}).
			Where("business_id IN ? AND status = ?", businessIDs, database.DeliveryStatusConfirmed).
			Updates(map[string]interface{}{
				"payment_expires_at": diedAt,
				"status":             database.DeliveryStatusCancelled,
			}).Error)
	}

	// Two full sweeper-kill cycles: each append must mint a fresh successor
	// order next to the surviving dead ones without tripping the index.
	for cycle := 0; cycle < 2; cycle++ {
		killFixtures(now.Add(-30 * time.Minute))
		now = now.Add(2 * time.Hour)
		require.NoError(t, svc.AppendDueDaysForAdmin(context.Background(), admin.ID),
			"hourly append must survive successor order creation under the guest request-identity index (cycle %d)", cycle)
	}

	// The live fixture must exist again per business, and every guest
	// request id must be distinct (the index would have said so, but assert
	// the shape explicitly for readers).
	var openCount int64
	require.NoError(t, db.Model(&database.Bill{}).
		Where("business_id IN ? AND notes = ? AND status = ? AND paid_amount = 0",
			businessIDs, "Demo delivery bill awaiting online payment", database.BillStatusOpen).
		Count(&openCount).Error)
	require.Equal(t, int64(len(businessIDs)), openCount, "each business must have a live awaiting-payment fixture after append")

	var requestIDs []string
	require.NoError(t, db.Model(&database.Order{}).
		Where("business_id IN ? AND created_by = 'guest' AND client_request_id IS NOT NULL", businessIDs).
		Pluck("client_request_id", &requestIDs).Error)
	seen := map[string]bool{}
	for _, id := range requestIDs {
		require.False(t, seen[id], "guest client_request_id %q reused across demo orders", id)
		seen[id] = true
	}
}
