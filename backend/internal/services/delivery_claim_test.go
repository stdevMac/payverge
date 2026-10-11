package services

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupDeliveryClaimTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:delivery-claim-%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	database.SetTestDB(db)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.Bill{},
		&database.DeliveryOrder{},
		&database.DeliveryDriver{},
		&database.RBACAuditLog{},
	))
	return db
}

func seedClaimDelivery(t *testing.T, db *gorm.DB) (businessID, deliveryID uint) {
	t.Helper()
	biz := &database.Business{
		BusinessId: "claim-biz-" + t.Name(), Name: "Claim Biz",
		OwnerAddress: "0xowner", SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
	}
	require.NoError(t, db.Create(biz).Error)
	bill := &database.Bill{BusinessID: biz.ID, BillNumber: "B-claim", Status: "open"}
	require.NoError(t, db.Create(bill).Error)
	order := &database.DeliveryOrder{
		BusinessID: biz.ID, BillID: bill.ID, DeliveryNumber: "D-claim-1",
		DeliveryType: database.DeliveryTypeInHouse, Status: database.DeliveryStatusPreparing,
		CustomerName: "Guest", CustomerPhone: "5550001111",
	}
	require.NoError(t, db.Create(order).Error)
	return biz.ID, order.ID
}

func TestClaimDelivery_AtomicSingleWinner(t *testing.T) {
	db := setupDeliveryClaimTestDB(t)
	bizID, delID := seedClaimDelivery(t, db)
	svc := NewDeliveryService(db, nil)
	aID, bID := uint(1), uint(2)

	out, prev, err := svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "server",
	}, false)
	require.NoError(t, err)
	require.NotNil(t, out.ClaimedByStaffID)
	assert.EqualValues(t, aID, *out.ClaimedByStaffID)
	assert.Nil(t, prev)

	_, _, err = svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &bID, Name: "Bob", Role: "server",
	}, false)
	var held *DeliveryClaimHeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, "Ana", held.ClaimedByName)
}

func TestClaimDelivery_IdleExpiryAllowsReclaim(t *testing.T) {
	db := setupDeliveryClaimTestDB(t)
	bizID, delID := seedClaimDelivery(t, db)
	svc := NewDeliveryService(db, nil)
	aID, bID := uint(1), uint(2)

	_, _, err := svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "server",
	}, false)
	require.NoError(t, err)

	stale := time.Now().Add(-deliveryClaimIdleTTL - time.Minute)
	require.NoError(t, db.Model(&database.DeliveryOrder{}).Where("id = ?", delID).Update("claimed_at", stale).Error)

	out, _, err := svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &bID, Name: "Bob", Role: "server",
	}, false)
	require.NoError(t, err)
	require.NotNil(t, out.ClaimedByStaffID)
	assert.EqualValues(t, bID, *out.ClaimedByStaffID)
}

func TestClaimDelivery_StealRequiresFlagAndPermission(t *testing.T) {
	db := setupDeliveryClaimTestDB(t)
	bizID, delID := seedClaimDelivery(t, db)
	svc := NewDeliveryService(db, nil)
	aID, mgrID := uint(1), uint(9)

	_, _, err := svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "server",
	}, false)
	require.NoError(t, err)

	// Manager without steal flag → steal required.
	_, _, err = svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &mgrID, Name: "Mgr", Role: "manager", CanSteal: true,
	}, false)
	require.ErrorIs(t, err, ErrDeliveryClaimStealRequired)

	// Front-line with steal flag → forbidden.
	_, _, err = svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &mgrID, Name: "Bob", Role: "server", CanSteal: false,
	}, true)
	require.ErrorIs(t, err, ErrDeliveryClaimForbidden)

	// Manager with steal → succeeds + audit.
	out, prev, err := svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &mgrID, Name: "Mgr", Role: "manager", CanSteal: true,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, out.ClaimedByStaffID)
	assert.EqualValues(t, mgrID, *out.ClaimedByStaffID)
	require.NotNil(t, prev)
	assert.EqualValues(t, aID, *prev)

	var audits int64
	require.NoError(t, db.Model(&database.RBACAuditLog{}).
		Where("business_id = ? AND action = ?", bizID, database.RBACActionClaimStolen).
		Count(&audits).Error)
	assert.EqualValues(t, 1, audits)
}

func TestAssertDeliveryClaimHeld_BlocksOtherOperator(t *testing.T) {
	db := setupDeliveryClaimTestDB(t)
	bizID, delID := seedClaimDelivery(t, db)
	svc := NewDeliveryService(db, nil)
	aID, bID := uint(1), uint(2)

	_, _, err := svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "server",
	}, false)
	require.NoError(t, err)

	err = svc.AssertDeliveryClaimHeld(bizID, delID, ClaimActor{StaffID: &bID, Name: "Bob", Role: "server"})
	var held *DeliveryClaimHeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, "Ana", held.ClaimedByName)

	require.NoError(t, svc.AssertDeliveryClaimHeld(bizID, delID, ClaimActor{StaffID: &aID, Name: "Ana", Role: "server"}))
}

func TestReleaseDelivery_OnlyHolderOrStealer(t *testing.T) {
	db := setupDeliveryClaimTestDB(t)
	bizID, delID := seedClaimDelivery(t, db)
	svc := NewDeliveryService(db, nil)
	aID, bID := uint(1), uint(2)

	_, _, err := svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "server",
	}, false)
	require.NoError(t, err)

	err = svc.ReleaseDeliveryOrder(bizID, delID, ClaimActor{StaffID: &bID, Name: "Bob", Role: "server"})
	require.True(t, errors.As(err, new(*DeliveryClaimHeldError)))

	require.NoError(t, svc.ReleaseDeliveryOrder(bizID, delID, ClaimActor{StaffID: &aID, Name: "Ana", Role: "server"}))
	var reloaded database.DeliveryOrder
	require.NoError(t, db.First(&reloaded, delID).Error)
	assert.Nil(t, reloaded.ClaimedByStaffID)
	assert.Nil(t, reloaded.ClaimedAt)
}

// L4-8: manager force-release of another operator's claim must write an audit
// row (claim_force_released). Self-release must not.
func TestReleaseDelivery_ForceReleaseIsAudited(t *testing.T) {
	db := setupDeliveryClaimTestDB(t)
	bizID, delID := seedClaimDelivery(t, db)
	svc := NewDeliveryService(db, nil)
	aID, mgrID := uint(1), uint(9)

	_, _, err := svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "server",
	}, false)
	require.NoError(t, err)

	// Manager force-releases Ana's claim.
	require.NoError(t, svc.ReleaseDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &mgrID, Name: "Mgr", Role: "manager", CanSteal: true,
	}))

	var audits int64
	require.NoError(t, db.Model(&database.RBACAuditLog{}).
		Where("business_id = ? AND action = ?", bizID, database.RBACActionClaimForceReleased).
		Count(&audits).Error)
	assert.EqualValues(t, 1, audits, "force-release must leave claim_force_released audit")

	// Self-release after re-claim must not create a second force-release audit.
	_, _, err = svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "server",
	}, false)
	require.NoError(t, err)
	require.NoError(t, svc.ReleaseDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "server",
	}))
	require.NoError(t, db.Model(&database.RBACAuditLog{}).
		Where("business_id = ? AND action = ?", bizID, database.RBACActionClaimForceReleased).
		Count(&audits).Error)
	assert.EqualValues(t, 1, audits, "self-release must not audit as force-release")
}

func TestSweepStaleDeliveryClaims(t *testing.T) {
	db := setupDeliveryClaimTestDB(t)
	bizID, delID := seedClaimDelivery(t, db)
	svc := NewDeliveryService(db, nil)
	aID := uint(1)
	_, _, err := svc.ClaimDeliveryOrder(bizID, delID, ClaimActor{
		StaffID: &aID, Name: "Ana", Role: "server",
	}, false)
	require.NoError(t, err)

	stale := time.Now().Add(-deliveryClaimIdleTTL - time.Minute)
	require.NoError(t, db.Model(&database.DeliveryOrder{}).Where("id = ?", delID).Update("claimed_at", stale).Error)
	require.NoError(t, svc.SweepStaleDeliveryClaims(bizID))

	var reloaded database.DeliveryOrder
	require.NoError(t, db.First(&reloaded, delID).Error)
	assert.Nil(t, reloaded.ClaimedByStaffID)
	assert.Nil(t, reloaded.ClaimedAt)
}

func TestClaimFieldsRoundTripOnList(t *testing.T) {
	db := setupDeliveryClaimTestDB(t)
	bizID, delID := seedClaimDelivery(t, db)
	sid := uint(77)
	now := time.Now()
	require.NoError(t, db.Model(&database.DeliveryOrder{}).Where("id = ?", delID).Updates(map[string]interface{}{
		"claimed_by_staff_id": sid,
		"claimed_by_name":     "Ana",
		"claimed_by_role":     "server",
		"claimed_at":          now,
	}).Error)
	result, err := NewDeliveryService(db, nil).GetBusinessDeliveries(bizID, DeliveryListParams{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Deliveries, 1)
	require.NotNil(t, result.Deliveries[0].ClaimedByStaffID)
	require.Equal(t, "Ana", result.Deliveries[0].ClaimedByName)
}
