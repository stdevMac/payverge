package demo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// #796 root cause: the pay-online fixture stamped every guest order with the
// per-business constant "demo-delivery-pay-<business_id>". The first time #772
// minted a successor for a consumed fixture, the new order collided with
// orders_guest_request_identity_uq and the 23505 aborted the WHOLE hourly
// append transaction — so the demo venue stopped growing bills entirely while
// caja kept showing cash, and the instance sat in status=failed.
func TestRearmAwaitingPaymentDeliveryMintsDistinctGuestIdentities(t *testing.T) {
	db := newDemoServiceTestDB(t)
	// Mirror the genesis index so sqlite enforces exactly what production does.
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS orders_guest_request_identity_uq
		ON orders (business_id, created_by, client_request_id)
		WHERE created_by = 'guest' AND client_request_id IS NOT NULL`).Error)

	admin := seedAdmin(t, db, "delivery-pay-identity@example.com")
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "identity-seed", BaselineDays: 5})
	require.NoError(t, mustEnsure(svc, admin.ID))

	var instance database.DemoInstance
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).First(&instance).Error)

	// The guest pays the standing fixture: it is no longer reusable, so the
	// next re-arm must mint a successor bill and a successor guest order.
	require.NoError(t, db.Model(&database.Bill{}).
		Where("notes = ?", "Demo delivery bill awaiting online payment").
		Updates(map[string]interface{}{"status": database.BillStatusPaid}).Error)

	var consumed int64
	require.NoError(t, db.Model(&database.Bill{}).
		Where("notes = ? AND status = ?", "Demo delivery bill awaiting online payment", database.BillStatusPaid).
		Count(&consumed).Error)
	require.Greater(t, consumed, int64(0), "precondition: a standing fixture existed to consume")

	// With the per-business constant this Create raised
	// "duplicate key value violates unique constraint
	// orders_guest_request_identity_uq" and rolled the append back.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.rearmAwaitingPaymentDeliveries(context.Background(), tx, &instance)
	}), "re-arming a consumed pay-online fixture must not violate the guest identity index")

	type row struct {
		BusinessID      uint
		ClientRequestID string
	}
	var rows []row
	require.NoError(t, db.Table("orders").
		Select("business_id, client_request_id").
		Where("created_by = ? AND client_request_id LIKE ?", "guest", "demo-delivery-pay-%").
		Scan(&rows).Error)
	require.GreaterOrEqual(t, len(rows), 2, "a successor guest order must have been minted")

	seen := map[string]bool{}
	for _, r := range rows {
		key := r.ClientRequestID
		require.False(t, seen[key], "guest request identity %q reused across delivery-pay orders", key)
		seen[key] = true
	}
}
