package demo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// #855: the pay-online delivery fixture expires after 15 minutes, so EVERY
// hourly re-arm found a dead fixture and minted a day/minute-stamped
// SUCCESSOR bill instead of recycling one. Six hours of dinner service left
// six identical unpaid ARS 38.900 checks (table_id 0, note "Demo delivery
// bill awaiting online payment") on the accounting board, and because the
// collection-gap predicate only excludes `voided`, the showroom claimed a
// ARS 251.950 hole that no guest ever owed.
//
// The fixture must be a SINGLETON: one live pay-online check per demo venue,
// no matter how many hourly ticks run.
func TestHourlyRearmKeepsExactlyOnePayOnlineFixture(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "delivery-pay-clone@example.com")

	clock := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return clock }, SeedVersion: "clone-seed", BaselineDays: 5})
	require.NoError(t, mustEnsure(svc, admin.ID))

	var instance database.DemoInstance
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).First(&instance).Error)
	require.NotNil(t, instance.PrimaryBusinessID)
	require.NotNil(t, instance.SecondaryBusinessID)
	businessIDs := []uint{*instance.PrimaryBusinessID, *instance.SecondaryBusinessID}

	// QA fixtures that live on the same venues and must survive untouched:
	// the half-paid table check and a guest's own unpaid delivery walk-out.
	qaPartial := database.Bill{
		BusinessID: businessIDs[1], BillNumber: "QA-PARTIAL-1", Notes: "Cuenta parcial — tarjeta pendiente de confirmación",
		Subtotal: 37100, TotalAmount: 37100, PaidAmount: 18550, Status: database.BillStatusPartial, CreatedAt: clock,
	}
	require.NoError(t, db.Create(&qaPartial).Error)
	guestWalkout := database.Bill{
		BusinessID: businessIDs[1], BillNumber: "QA-GUEST-1", Notes: "Pedido del comensal",
		Subtotal: 9000, TotalAmount: 9000, PaidAmount: 0, Status: database.BillStatusAbandoned, CreatedAt: clock,
	}
	require.NoError(t, db.Create(&guestWalkout).Error)

	// Six hourly ticks. Each hour the delivery payment window (15m) expires
	// and AbandonUnpaidOpenBill writes the check off unpaid — exactly what
	// production's bill lifecycle sweeper does to this fixture.
	for tick := 0; tick < 6; tick++ {
		clock = clock.Add(time.Hour)
		require.NoError(t, db.Model(&database.Bill{}).
			Where("notes = ? AND status = ?", deliveryPayFixtureNote, database.BillStatusOpen).
			Updates(map[string]interface{}{
				"status":     database.BillStatusAbandoned,
				"closed_at":  clock,
				"updated_at": clock,
			}).Error)
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			return svc.rearmAwaitingPaymentDeliveries(context.Background(), tx, &instance)
		}), "tick %d", tick)
	}

	for _, businessID := range businessIDs {
		var fixtures []database.Bill
		require.NoError(t, db.Where("business_id = ? AND notes = ?", businessID, deliveryPayFixtureNote).
			Order("id ASC").Find(&fixtures).Error)
		require.Len(t, fixtures, 1, "business %d must keep exactly one pay-online fixture after 6 hourly re-arms", businessID)
		require.Equal(t, database.BillStatusOpen, fixtures[0].Status)
		require.Zero(t, fixtures[0].PaidAmount)

		// The showroom's unpaid board must show one ARS 38.900 check (3.890.000 cents), not six.
		var gap int64
		require.NoError(t, db.Model(&database.Bill{}).
			Where("business_id = ? AND notes = ? AND status <> ?", businessID, deliveryPayFixtureNote, database.BillStatusVoided).
			Select("COALESCE(SUM(total_amount - paid_amount), 0)").Scan(&gap).Error)
		require.Equal(t, int64(3890000), gap, "business %d collection gap from the demo fixture", businessID)

		// Orders follow their bill — no orphan intake cards.
		var orders int64
		require.NoError(t, db.Model(&database.Order{}).
			Where("business_id = ? AND notes = ?", businessID, "Demo delivery order awaiting payment").
			Count(&orders).Error)
		require.Equal(t, int64(1), orders, "business %d pay-online guest orders", businessID)

		var orphanDeliveries int64
		require.NoError(t, db.Model(&database.DeliveryOrder{}).
			Where("business_id = ? AND bill_id NOT IN (?)", businessID,
				db.Model(&database.Bill{}).Select("id").Where("business_id = ?", businessID)).
			Count(&orphanDeliveries).Error)
		require.Zero(t, orphanDeliveries, "business %d must not keep delivery rows pointing at purged bills", businessID)
	}

	// QA fixtures untouched.
	var stillPartial database.Bill
	require.NoError(t, db.Where("bill_number = ?", "QA-PARTIAL-1").First(&stillPartial).Error)
	require.Equal(t, database.BillStatusPartial, stillPartial.Status)
	require.Equal(t, int64(18550), stillPartial.PaidAmount)
	var stillWalkout database.Bill
	require.NoError(t, db.Where("bill_number = ?", "QA-GUEST-1").First(&stillWalkout).Error)
	require.Equal(t, database.BillStatusAbandoned, stillWalkout.Status)
}

// A fixture the guest actually PAID is history, not a clone: the purge must
// leave it alone and mint a successor beside it (#772 / #796).
func TestPurgeKeepsSettledPayOnlineFixture(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "delivery-pay-settled@example.com")

	clock := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return clock }, SeedVersion: "settled-seed", BaselineDays: 5})
	require.NoError(t, mustEnsure(svc, admin.ID))

	var instance database.DemoInstance
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).First(&instance).Error)

	require.NoError(t, db.Model(&database.Bill{}).
		Where("notes = ?", deliveryPayFixtureNote).
		Updates(map[string]interface{}{
			"status":      database.BillStatusPaid,
			"paid_amount": gorm.Expr("total_amount"),
			"closed_at":   clock,
		}).Error)

	clock = clock.Add(time.Hour)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.rearmAwaitingPaymentDeliveries(context.Background(), tx, &instance)
	}))

	var paid int64
	require.NoError(t, db.Model(&database.Bill{}).
		Where("notes = ? AND status = ?", deliveryPayFixtureNote, database.BillStatusPaid).
		Count(&paid).Error)
	require.Equal(t, int64(2), paid, "settled pay-online history must survive the purge")

	var open int64
	require.NoError(t, db.Model(&database.Bill{}).
		Where("notes = ? AND status = ?", deliveryPayFixtureNote, database.BillStatusOpen).
		Count(&open).Error)
	require.Equal(t, int64(2), open, "each venue must re-arm exactly one live fixture")
}

// installBillFKGuards reproduces the three BLOCKING foreign keys that point at
// `bills` in the production Postgres schema — `business_milestone_events.bill_id`,
// `counters.current_bill_id` and `customer_visits.bill_id` are all declared with
// no ON DELETE clause, so Postgres refuses to delete a bill any of them still
// references. The SQLite harness migrates without constraints
// (DisableForeignKeyConstraintWhenMigrating), so a purge that leaves a dangling
// reference reads green here and dies on the live showroom. These triggers make
// the harness fail the same way.
func installBillFKGuards(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, guard := range []struct{ name, table, column string }{
		{"fk_guard_milestone_bill", "business_milestone_events", "bill_id"},
		{"fk_guard_counter_bill", "counters", "current_bill_id"},
		{"fk_guard_visit_bill", "customer_visits", "bill_id"},
	} {
		require.NoError(t, db.Exec(fmt.Sprintf(
			`CREATE TRIGGER %s BEFORE DELETE ON bills FOR EACH ROW `+
				`WHEN EXISTS (SELECT 1 FROM %s WHERE %s = OLD.id) `+
				`BEGIN SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: %s.%s'); END`,
			guard.name, guard.table, guard.column, guard.table, guard.column)).Error)
	}
}

// #855 follow-up: the purge deletes the clone's own children (orders, items,
// splits, history, payments, delivery rows) but three OTHER tables carry a
// blocking FK into `bills`, and none of them is a child the purge owns:
//
//   - counters.current_bill_id — a physical takeaway counter holding the check
//   - business_milestone_events.bill_id — a milestone the venue already earned
//   - customer_visits.bill_id — a guest's CRM visit + loyalty history
//
// On Postgres each one turns the purge into a constraint violation, which
// aborts the whole ensure/re-arm transaction: the clones survive AND the
// hourly re-arm stops working. The fix detaches the reference and keeps the
// row — deleting a counter, re-arming a milestone the operator already got, or
// erasing a guest's visit would be a far worse cure than the disease.
func TestPurgeDetachesBlockingBillReferencesBeforeDeleting(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "delivery-pay-fk@example.com")

	clock := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return clock }, SeedVersion: "fk-seed", BaselineDays: 5})
	require.NoError(t, mustEnsure(svc, admin.ID))

	var instance database.DemoInstance
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).First(&instance).Error)
	require.NotNil(t, instance.PrimaryBusinessID)
	businessID := *instance.PrimaryBusinessID

	var doomed database.Bill
	require.NoError(t, db.Where("business_id = ? AND notes = ?", businessID, deliveryPayFixtureNote).
		Order("id ASC").First(&doomed).Error)

	// The 15-minute payment window expired — this is exactly the dead
	// predecessor the hourly re-arm has to retire.
	require.NoError(t, db.Model(&database.Bill{}).Where("id = ?", doomed.ID).
		Updates(map[string]interface{}{
			"status":     database.BillStatusAbandoned,
			"closed_at":  clock,
			"updated_at": clock,
		}).Error)

	counter := database.Counter{
		BusinessID: businessID, CounterNumber: 91, Name: "Mostrador 91",
		IsActive: true, CurrentBillID: &doomed.ID,
	}
	require.NoError(t, db.Create(&counter).Error)

	milestone := database.BusinessMilestoneEvent{
		BusinessID: businessID, MilestoneType: database.BusinessMilestoneTypeFirstOrder,
		ThresholdCents: 0, BillID: &doomed.ID, Status: database.BusinessMilestoneStatusSent,
		CreatedAt: clock,
	}
	require.NoError(t, db.Create(&milestone).Error)

	customer := database.Customer{Name: "Delivery FK Guest", Email: "delivery-fk-guest@example.com"}
	require.NoError(t, db.Create(&customer).Error)
	link := database.CustomerBusiness{
		CustomerID: customer.ID, BusinessID: businessID, LoyaltyPoints: 120,
		TotalSpent: 389, VisitCount: 1, FirstVisitAt: clock, IsActive: true,
	}
	require.NoError(t, db.Create(&link).Error)
	visit := database.CustomerVisit{
		CustomerBusinessID: link.ID, BillID: &doomed.ID, AmountSpent: 389,
		PointsEarned: 120, VisitDate: clock, CreatedAt: clock,
	}
	require.NoError(t, db.Create(&visit).Error)

	installBillFKGuards(t, db)

	clock = clock.Add(time.Hour)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.rearmAwaitingPaymentDeliveries(context.Background(), tx, &instance)
	}), "the re-arm must not be aborted by a blocking reference into the clone")

	var survivors int64
	require.NoError(t, db.Model(&database.Bill{}).Where("id = ?", doomed.ID).Count(&survivors).Error)
	require.Zero(t, survivors, "the dead pay-online clone must actually be purged")

	// Every referencing row is a business record of its own: released, not erased.
	var keptCounter database.Counter
	require.NoError(t, db.First(&keptCounter, counter.ID).Error)
	require.Nil(t, keptCounter.CurrentBillID, "the counter must be freed, never deleted")
	require.Equal(t, "Mostrador 91", keptCounter.Name)

	var keptMilestone database.BusinessMilestoneEvent
	require.NoError(t, db.First(&keptMilestone, milestone.ID).Error)
	require.Nil(t, keptMilestone.BillID, "the milestone must be detached, never deleted")
	require.Equal(t, database.BusinessMilestoneStatusSent, keptMilestone.Status,
		"deleting the row would let an already-sent milestone fire a second time")

	var keptVisit database.CustomerVisit
	require.NoError(t, db.First(&keptVisit, visit.ID).Error)
	require.Nil(t, keptVisit.BillID, "the guest visit must be detached, never deleted")
	require.Equal(t, float64(389), keptVisit.AmountSpent, "loyalty history must survive the purge")
	require.Equal(t, 120, keptVisit.PointsEarned)

	// And the venue still ends the tick with exactly one live fixture, which
	// advertises no tipping wallet: the demo venue owns no chain address, so
	// showing one on a guest's check is a claim the showroom cannot back (#856).
	var live []database.Bill
	require.NoError(t, db.Where("business_id = ? AND notes = ? AND status = ?",
		businessID, deliveryPayFixtureNote, database.BillStatusOpen).Find(&live).Error)
	require.Len(t, live, 1)
	require.Empty(t, live[0].TippingAddr, "the pay-online fixture must not advertise a synthetic tipping wallet")
}
