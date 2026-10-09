package demo

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func newDemoServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(demoTestModels()...))
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS bill_items (
		id TEXT PRIMARY KEY,
		bill_id INTEGER NOT NULL,
		menu_item_id TEXT DEFAULT '',
		name TEXT NOT NULL,
		price REAL NOT NULL,
		quantity INTEGER NOT NULL,
		options TEXT,
		item_type TEXT DEFAULT 'menu_item',
		bundle_id INTEGER,
		parent_bundle_id INTEGER,
		source_offer_id INTEGER,
		order_id INTEGER,
		subtotal REAL NOT NULL,
		created_at DATETIME
	)`).Error)
	return db
}

func demoTestModels() []interface{} {
	return []interface{}{
		&database.User{},
		&database.Business{},
		&database.BusinessGalleryImage{},
		&database.BusinessOperatingHours{},
		&database.BusinessSpecialFeature{},
		&database.ReservationSettings{},
		&database.TableReservation{},
		&database.ReservationStatusHistory{},
		&database.InventorySettings{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.InventoryMovement{},
		&database.Table{},
		&database.Counter{},
		&database.Menu{},
		&database.Bill{},
		&database.BillHistoryEvent{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.Order{},
		&database.Customer{},
		&database.CustomerPreferences{},
		&database.CustomerAddress{},
		&database.CustomerBusiness{},
		&database.CustomerVisit{},
		&database.CustomerCommunication{},
		&database.DeliverySettings{},
		&database.DeliveryZone{},
		&database.DeliveryDriver{},
		&database.DeliveryOrder{},
		&database.DeliveryStatusHistory{},
		&database.Staff{},
		&database.StaffInvitation{},
		&database.Position{},
		&database.StaffPosition{},
		&database.BusinessScheduleSettings{},
		&database.Schedule{},
		&database.Shift{},
		&database.StaffAvailability{},
		&database.TimeOffRequest{},
		&database.TimeEntry{},
		&database.OpenShiftClaim{},
		&database.ShiftSwapRequest{},
		&database.ChatChannel{},
		&database.ChatChannelMember{},
		&database.ChatMessage{},
		&database.ChatRead{},
		&database.Announcement{},
		&database.AnnouncementAck{},
		&database.ShiftNote{},
		&database.StaffNotification{},
		&database.ChecklistTemplate{},
		&database.ChecklistItem{},
		&database.ChecklistRun{},
		&database.ChecklistItemCompletion{},
		&database.Document{},
		&database.DocumentAck{},
		&database.Shoutout{},
		&database.Poll{},
		&database.PollOption{},
		&database.PollVote{},
		&database.BusinessFiscalSettings{},
		&database.FiscalReceipt{},
		&database.FiscalJob{},
		&database.FiscalAuditEvent{},
		&database.FiscalDeliveryTask{},
		&database.Printer{},
		&database.PrintJob{},
		&database.CashRegisterSession{},
		&database.CashRegisterMovement{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
		&database.BusinessAlertSettings{},
		&database.ManualLedgerEntry{},
		&database.PayrollRun{},
		&database.PayrollLineItem{},
		&database.LoyaltyProgram{},
		&database.LoyaltyTier{},
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.AIImageUsage{},
		&database.AIGeneratedImage{},
		&database.DirectorConsoleThread{},
		&database.DirectorConsoleMessage{},
		&database.DirectorProposedAction{},
		&database.DemoInstance{},
		&database.DemoRun{},
		// FK-bearing children of businesses/bills/threads that live outside
		// the seeder but accrue rows in production (startup backfills, live
		// operator/guest usage). The wipe deletes them, so every demo test
		// needs the tables to exist.
		&database.BusinessRevenueAggregate{},
		&database.BusinessMilestoneEvent{},
		&database.Offer{},
		&database.Bundle{},
		&database.BusinessCurrency{},
		&database.BusinessLanguage{},
		&database.Translation{},
		&database.AiWaiterConversation{},
		&database.AiWaiterMessage{},
		&database.DirectorToolCall{},
		&database.DirectorActionAudit{},
		&database.MenuWizardSession{},
		&database.MenuWizardMessage{},
		&database.MenuExtractionJob{},
		&database.MenuExtractionImage{},
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
		&database.PaymentRefundDestination{},
		&database.StaffLoginCode{},
		&database.TelegramConnectionToken{},
		&database.TelegramUpdateReceipt{},
		&database.RBACAuditLog{},
		&database.CompVoidAudit{},
		&database.WithdrawalHistory{},
		&database.ReportSchedule{},
		&database.PushSubscription{},
		&database.InventoryAlertLog{},
		&database.BillSplitShare{},
	}
}

func seedAdmin(t *testing.T, db *gorm.DB, email string) database.User {
	t.Helper()
	admin := database.User{Email: email, Role: "admin"}
	require.NoError(t, db.Create(&admin).Error)
	return admin
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
}

func TestEnsureForAdminCreatesIsolatedFullFeatureDemo(t *testing.T) {
	db := newDemoServiceTestDB(t)
	adminA := seedAdmin(t, db, "demo-admin-a@example.com")
	adminB := seedAdmin(t, db, "demo-admin-b@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	instanceA, err := svc.EnsureForAdmin(context.Background(), adminA.ID)
	require.NoError(t, err)
	instanceB, err := svc.EnsureForAdmin(context.Background(), adminB.ID)
	require.NoError(t, err)

	require.NotEqual(t, instanceA.ID, instanceB.ID)
	require.NotNil(t, instanceA.PrimaryBusinessID)
	require.NotNil(t, instanceA.SecondaryBusinessID)
	require.NotNil(t, instanceB.PrimaryBusinessID)
	require.NotNil(t, instanceB.SecondaryBusinessID)

	for _, admin := range []database.User{adminA, adminB} {
		var businesses []database.Business
		require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Order("business_id").Find(&businesses).Error)
		require.Len(t, businesses, 2)
		for _, business := range businesses {
			require.True(t, business.IsDemo)
			require.NotNil(t, business.DemoOwnerUserID)
			require.Equal(t, admin.ID, *business.DemoOwnerUserID)
			require.NotNil(t, business.UserID)
			require.Equal(t, admin.ID, *business.UserID)
			// Task 35: logo intentionally empty — neutral initials placeholder.
			require.Empty(t, business.Logo)
			require.NotContains(t, business.Logo, "dummyimage")
		}

		var ladders []database.LoyaltyProgram
		businessIDs := []uint{businesses[0].ID, businesses[1].ID}
		require.NoError(t, db.Preload("Tiers", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("sort_order ASC")
		}).Where("business_id IN ?", businessIDs).Find(&ladders).Error)
		require.Len(t, ladders, 2)
		for _, ladder := range ladders {
			require.Len(t, ladder.Tiers, 3, "demo business %d must have one canonical ladder", ladder.BusinessID)
			require.Equal(t, []string{"Bronce", "Plata", "Oro"}, []string{
				ladder.Tiers[0].Name,
				ladder.Tiers[1].Name,
				ladder.Tiers[2].Name,
			}, "demo business %d tier names", ladder.BusinessID)
			require.Equal(t, []int64{0, 15000000, 45000000}, []int64{
				ladder.Tiers[0].MinLifetimeSpentCents,
				ladder.Tiers[1].MinLifetimeSpentCents,
				ladder.Tiers[2].MinLifetimeSpentCents,
			}, "demo business %d tier thresholds", ladder.BusinessID)
		}
	}

	result, err := svc.VerifyForAdmin(context.Background(), adminA.ID)
	require.NoError(t, err)
	require.Equal(t, VerificationPassed, result.Status, result.Errors)
	requireCoveragePassed(t, result,
		"business_profile",
		"menu_images",
		"tables",
		"bills",
		"bill_items",
		"payments",
		"alternative_payments",
		"orders",
		"crm",
		"customer_addresses",
		"inventory",
		"reservations",
		"delivery",
		"delivery_payment",
		"staff",
		"staff_invitations",
		"staff_notifications",
		"scheduling",
		"time_off",
		"coverage_claims",
		"coverage_swaps",
		"shift_logbook",
		"chat",
		"engagement",
		"fiscal",
		"printers",
		"cash_register",
		"manual_ledger",
		"payroll",
		"loyalty",
		"plugin_configs",
		"operational_alerts",
		"ai",
	)

	assertThirtyDayWindow(t, db, adminA.ID, "2026-06-03", "2026-07-02")
	assertBillMoneyInvariant(t, db, adminA.ID)
	assertAlternativePaymentsSeeded(t, db, adminA.ID)
	assertDeliveryPaymentFixturesSeeded(t, db, adminA.ID, fixedNow())
}

func TestEnsureForAdminIsIdempotentAndAppendDueDaysHasNoCap(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "append-admin@example.com")

	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})
	first, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	second, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)

	before := countAdminBills(t, db, admin.ID)
	now = now.AddDate(0, 0, 3)
	require.NoError(t, svc.AppendDueDays(context.Background()))
	after := countAdminBills(t, db, admin.ID)
	require.Greater(t, after, before)

	var reloaded database.DemoInstance
	require.NoError(t, db.First(&reloaded, first.ID).Error)
	require.NotNil(t, reloaded.LastSimulatedBusinessDate)
	// The pointer records the last FULLY simulated day — "today" (07-05) is
	// still in progress at the frozen 08:00 ET clock, so it stays revisitable.
	require.Equal(t, "2026-07-04", reloaded.LastSimulatedBusinessDate.Format("2006-01-02"))

	require.NoError(t, svc.AppendDueDays(context.Background()))
	require.Equal(t, after, countAdminBills(t, db, admin.ID))
}

// The pay-online demo fixture uses a 15-minute window. Hourly append is a
// no-op once last_simulated is already today, so the window expires and
// stays dead (#433 delivery_payment coverage).
func TestAppendDueDaysRearmsExpiredDeliveryPaymentWindow(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "rearm-pay-admin@example.com")

	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 7})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	assertDeliveryPaymentFixturesSeeded(t, db, admin.ID, now)

	expired := now.Add(-time.Minute)
	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Where("business_id IN ?", businessIDs).
		Updates(map[string]interface{}{
			"payment_expires_at": expired,
			"status":             database.DeliveryStatusCancelled,
		}).Error)

	var dead int64
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Where("business_id IN ?", businessIDs).
		Where("status = ?", database.DeliveryStatusConfirmed).
		Where("payment_expires_at > ?", now).
		Count(&dead).Error)
	require.Zero(t, dead, "precondition: expired windows must not count as live")

	require.NoError(t, svc.AppendDueDays(context.Background()))
	assertDeliveryPaymentFixturesSeeded(t, db, admin.ID, now)
}

// Production #433: the deterministic delivery-pay bill was later marked paid,
// so re-arming the delivery left coverage at 0 (count requires bills.status
// <> paid). Append must mint a fresh unpaid bill.
func TestAppendDueDaysRecoversPaidDeliveryPayBill(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "rearm-paid-admin@example.com")

	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 7})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)
	require.NoError(t, db.Model(&database.Bill{}).
		Where("business_id IN ? AND bill_number LIKE ?", businessIDs, "B%").
		Where("notes = ?", "Demo delivery bill awaiting online payment").
		Updates(map[string]interface{}{
			"status":      database.BillStatusPaid,
			"paid_amount": 100,
		}).Error)
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Where("business_id IN ?", businessIDs).
		Updates(map[string]interface{}{
			"payment_expires_at": now.Add(-time.Minute),
			"status":             database.DeliveryStatusCancelled,
		}).Error)

	var live int64
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Joins("JOIN bills ON bills.id = delivery_orders.bill_id").
		Where("delivery_orders.business_id IN ?", businessIDs).
		Where("delivery_orders.status = ?", database.DeliveryStatusConfirmed).
		Where("delivery_orders.payment_expires_at > ?", now).
		Where("bills.status <> ?", database.BillStatusPaid).
		Count(&live).Error)
	require.Zero(t, live)

	require.NoError(t, svc.AppendDueDays(context.Background()))
	assertDeliveryPaymentFixturesSeeded(t, db, admin.ID, now)
}

// #772: leftover 762/760 stayed abandoned unpaid while closed_at jumped
// +2h/+4h. Hourly append reopened the dead check; 15m later the expiry
// sweeper wrote now() again.
//
// #855 showed the other half of that bug: every re-arm minted a stamped
// SUCCESSOR beside the corpse, so six hours of dinner service left six
// identical unpaid ARS 38.900 checks inflating the collection gap. The
// re-arm now purges its own dead, never-paid fixtures — there is nothing
// left to restamp, and coverage recovers on a single live successor.
func TestAppendDueDaysDoesNotRestampAbandonedDeliveryPayClosedAt(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "rearm-abandoned-closed-at@example.com")

	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 7})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)

	diedAt := now.Add(-4 * time.Hour)
	require.NoError(t, db.Model(&database.Bill{}).
		Where("business_id IN ? AND notes = ?", businessIDs, "Demo delivery bill awaiting online payment").
		Updates(map[string]interface{}{
			"status":       database.BillStatusAbandoned,
			"abandoned_at": diedAt,
			"closed_at":    diedAt,
			"updated_at":   diedAt,
		}).Error)
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Where("business_id IN ?", businessIDs).
		Updates(map[string]interface{}{
			"payment_expires_at": now.Add(-time.Minute),
			"status":             database.DeliveryStatusCancelled,
		}).Error)

	var leftovers []database.Bill
	require.NoError(t, db.Where("business_id IN ? AND notes = ? AND status = ?",
		businessIDs, "Demo delivery bill awaiting online payment", database.BillStatusAbandoned).
		Find(&leftovers).Error)
	require.NotEmpty(t, leftovers)
	originalIDs := make([]uint, 0, len(leftovers))
	for _, leftover := range leftovers {
		require.NotNil(t, leftover.ClosedAt)
		require.Equal(t, diedAt.Unix(), leftover.ClosedAt.Unix(), "precondition: leftover closed_at is the original death time")
		originalIDs = append(originalIDs, leftover.ID)
	}

	now = now.Add(2 * time.Hour)
	require.NoError(t, svc.AppendDueDays(context.Background()))
	assertDeliveryPaymentFixturesSeeded(t, db, admin.ID, now)

	var after []database.Bill
	require.NoError(t, db.Where("id IN ?", originalIDs).Find(&after).Error)
	require.Empty(t, after, "dead, never-paid pay-online fixtures must be purged, not reopened or restamped")

	for _, businessID := range businessIDs {
		var fixtures []database.Bill
		require.NoError(t, db.Where("business_id = ? AND notes = ?", businessID, "Demo delivery bill awaiting online payment").
			Find(&fixtures).Error)
		require.Len(t, fixtures, 1, "business %d must be left with exactly one pay-online fixture", businessID)
		require.Equal(t, database.BillStatusOpen, fixtures[0].Status)
		require.Zero(t, fixtures[0].PaidAmount)
	}

	var successorLive int64
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Joins("JOIN bills ON bills.id = delivery_orders.bill_id").
		Where("delivery_orders.business_id IN ?", businessIDs).
		Where("delivery_orders.status = ?", database.DeliveryStatusConfirmed).
		Where("delivery_orders.payment_expires_at > ?", now).
		Where("bills.id NOT IN ?", originalIDs).
		Where("bills.status = ?", database.BillStatusOpen).
		Count(&successorLive).Error)
	require.Greater(t, successorLive, int64(0), "coverage must recover on a new unpaid successor, not the leftover")
}

// Startup ensure right after an hourly append runs two AppendDueDays inside the
// same minute. With the day-stamped delivery-pay bill dead, both passes derive
// the identical minute-stamped successor number; the second pass must adopt the
// already-minted successor instead of dying on idx_bills_bill_number.
func TestAppendDueDaysSameMinuteReusesMintedDeliveryPaySuccessor(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "same-minute-successor@example.com")

	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 7})
	instance, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	businessIDs := compactBusinessIDs(instance.PrimaryBusinessID, instance.SecondaryBusinessID)

	payAllDeliveryPayBills := func() {
		require.NoError(t, db.Model(&database.Bill{}).
			Where("business_id IN ? AND notes = ?", businessIDs, "Demo delivery bill awaiting online payment").
			Updates(map[string]interface{}{
				"status":      database.BillStatusPaid,
				"paid_amount": 100,
			}).Error)
	}
	expireOrders := func() {
		require.NoError(t, db.Model(&database.DeliveryOrder{}).
			Where("business_id IN ?", businessIDs).
			Where("status = ?", database.DeliveryStatusConfirmed).
			Updates(map[string]interface{}{
				"payment_expires_at": now.Add(-time.Minute),
				"status":             database.DeliveryStatusCancelled,
			}).Error)
	}

	// Kill the base fixture: append #1 mints the day-stamped successor.
	payAllDeliveryPayBills()
	expireOrders()
	require.NoError(t, svc.AppendDueDays(context.Background()))

	// Kill the day-stamped successor too: append #2 mints the minute-stamped one.
	payAllDeliveryPayBills()
	expireOrders()
	require.NoError(t, svc.AppendDueDays(context.Background()))

	// Same wall-clock minute, orders expired again but the minute-stamped bill
	// is still open: append #3 must adopt it, not re-create the same number.
	expireOrders()
	require.NoError(t, svc.AppendDueDays(context.Background()))
	assertDeliveryPaymentFixturesSeeded(t, db, admin.ID, now)
}

func TestAppendDueDaysContinuesAfterFailedInstance(t *testing.T) {
	db := newDemoServiceTestDB(t)
	adminA := seedAdmin(t, db, "append-failed-a@example.com")
	adminB := seedAdmin(t, db, "append-failed-b@example.com")

	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})
	require.NoError(t, mustEnsure(svc, adminA.ID))
	require.NoError(t, mustEnsure(svc, adminB.ID))

	var broken database.DemoInstance
	require.NoError(t, db.Where("admin_user_id = ?", adminA.ID).First(&broken).Error)
	require.NoError(t, db.Model(&broken).Update("timezone", "Invalid/Timezone").Error)

	beforeB := countAdminBills(t, db, adminB.ID)
	now = now.AddDate(0, 0, 1)

	err := svc.AppendDueDays(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "demo instance")
	require.Greater(t, countAdminBills(t, db, adminB.ID), beforeB)

	var healthy database.DemoInstance
	require.NoError(t, db.Where("admin_user_id = ?", adminB.ID).First(&healthy).Error)
	require.NotNil(t, healthy.LastSimulatedBusinessDate)
	// Last FULLY simulated day: the frozen clock sits at 08:00 ET on 07-03,
	// so 07-03 is partial and the pointer rests on 07-02.
	require.Equal(t, "2026-07-02", healthy.LastSimulatedBusinessDate.Format("2006-01-02"))
}

func TestEnsureForAdminRegeneratesWhenSeedVersionChanges(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "versioned-admin@example.com")

	v1 := NewService(db, Options{Now: fixedNow, SeedVersion: "seed-v1", BaselineDays: 30})
	first, err := v1.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	beforeBills := countAdminBills(t, db, admin.ID)

	require.NoError(t, db.Model(&database.Business{}).
		Where("business_id = ?", fmt.Sprintf("demo-admin-%d-primary", admin.ID)).
		Update("name", "Stale Demo Name").Error)

	v2 := NewService(db, Options{Now: fixedNow, SeedVersion: "seed-v2", BaselineDays: 30})
	second, err := v2.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, "seed-v2", second.SeedVersion)
	require.Greater(t, countAdminBills(t, db, admin.ID), int64(0))
	require.Less(t, countAdminBills(t, db, admin.ID), beforeBills*2)

	var core database.Business
	require.NoError(t, db.Where("business_id = ?", fmt.Sprintf("demo-admin-%d-primary", admin.ID)).First(&core).Error)
	require.Equal(t, "Bodegón Mesa Larga", core.Name)

	var runs []database.DemoRun
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).Find(&runs).Error)
	require.Len(t, runs, 1)
	require.Equal(t, "seed-v2", runs[0].SeedVersion)
}

func TestEnsureForAdminDoesNotDuplicateStaticFeatureRows(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "static-idempotent@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	require.NoError(t, mustEnsure(svc, admin.ID))
	before := map[string]int64{
		"shifts":                    countAdminBusinessRows(t, db, admin.ID, &database.Shift{}),
		"staff_positions":           countAdminBusinessRows(t, db, admin.ID, &database.StaffPosition{}),
		"chat_channel_members":      countAdminBusinessRows(t, db, admin.ID, &database.ChatChannelMember{}),
		"chat_reads":                countAdminBusinessRows(t, db, admin.ID, &database.ChatRead{}),
		"announcement_acks":         countAdminBusinessRows(t, db, admin.ID, &database.AnnouncementAck{}),
		"document_acks":             countAdminBusinessRows(t, db, admin.ID, &database.DocumentAck{}),
		"poll_votes":                countAdminBusinessRows(t, db, admin.ID, &database.PollVote{}),
		"director_proposed_actions": countAdminBusinessRows(t, db, admin.ID, &database.DirectorProposedAction{}),
		"manual_ledger":             countAdminBusinessRows(t, db, admin.ID, &database.ManualLedgerEntry{}),
		"payroll_runs":              countAdminBusinessRows(t, db, admin.ID, &database.PayrollRun{}),
		"payroll_line_items":        countAdminBusinessRows(t, db, admin.ID, &database.PayrollLineItem{}),
		"loyalty_programs":          countAdminBusinessRows(t, db, admin.ID, &database.LoyaltyProgram{}),
		"business_plugins":          countAdminBusinessRows(t, db, admin.ID, &database.BusinessPlugin{}),
		"time_off_requests":         countAdminBusinessRows(t, db, admin.ID, &database.TimeOffRequest{}),
		"open_shift_claims":         countAdminBusinessRows(t, db, admin.ID, &database.OpenShiftClaim{}),
		"shift_swap_requests":       countAdminBusinessRows(t, db, admin.ID, &database.ShiftSwapRequest{}),
		"shift_notes":               countAdminBusinessRows(t, db, admin.ID, &database.ShiftNote{}),
	}

	require.NoError(t, mustEnsure(svc, admin.ID))

	for key, expected := range before {
		require.Equalf(t, expected, countAdminBusinessRows(t, db, admin.ID, modelForStaticCount(key)), "%s duplicated after second ensure", key)
	}
}

func TestSummaryIncludesDemoAccessIdentitiesWithConfiguredEmailDomain(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "access-admin@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30, EmailDomain: "demo.example.com"})
	summary, err := svc.SummaryForAdmin(context.Background(), admin.ID, true)
	require.NoError(t, err)

	require.Len(t, summary.Access, 10)
	roles := map[string]bool{}
	pinsByBiz := map[uint]map[string]bool{}
	for _, identity := range summary.Access {
		require.Contains(t, identity.Email, "@demo.example.com")
		require.Equal(t, "/staff/login", identity.LoginPath)
		require.NotEmpty(t, identity.PINHint)
		require.Len(t, identity.PINHint, 4)
		if pinsByBiz[identity.BusinessID] == nil {
			pinsByBiz[identity.BusinessID] = map[string]bool{}
		}
		require.Falsef(t, pinsByBiz[identity.BusinessID][identity.PINHint],
			"duplicate PIN %s on business %d", identity.PINHint, identity.BusinessID)
		pinsByBiz[identity.BusinessID][identity.PINHint] = true
		roles[identity.Role] = true
	}
	require.True(t, roles[string(database.StaffRoleManager)])
	require.True(t, roles[string(database.StaffRoleServer)])
	require.True(t, roles[string(database.StaffRoleHost)])
	require.True(t, roles[string(database.StaffRoleKitchen)])
}

func TestResetForAdminDeletesOnlyOwnedDemoDataAndRegenerates(t *testing.T) {
	db := newDemoServiceTestDB(t)
	adminA := seedAdmin(t, db, "reset-a@example.com")
	adminB := seedAdmin(t, db, "reset-b@example.com")
	ownerID := uint(999)
	nonDemo := database.Business{
		BusinessId:     "real-business",
		UserID:         &ownerID,
		OwnerAddress:   "0xreal",
		OwnerName:      "Real Owner",
		Name:           "Real Business",
		SettlementAddr: "0xreal",
		TippingAddr:    "0xreal",
		IsActive:       true,
	}
	require.NoError(t, db.Create(&nonDemo).Error)
	require.NoError(t, db.Create(&database.Bill{
		BusinessID:     nonDemo.ID,
		BillNumber:     "REAL-1",
		Status:         database.BillStatusPaid,
		Subtotal:       1000,
		TotalAmount:    1000,
		PaidAmount:     1000,
		SettlementAddr: "0xreal",
		TippingAddr:    "0xreal",
	}).Error)

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	require.NoError(t, mustEnsure(svc, adminA.ID))
	require.NoError(t, mustEnsure(svc, adminB.ID))
	beforeB := countAdminBills(t, db, adminB.ID)

	resetInstance, err := svc.ResetForAdmin(context.Background(), adminA.ID)
	require.NoError(t, err)
	require.NotNil(t, resetInstance.PrimaryBusinessID)
	require.NotNil(t, resetInstance.SecondaryBusinessID)

	var realCount int64
	require.NoError(t, db.Model(&database.Bill{}).Where("business_id = ?", nonDemo.ID).Count(&realCount).Error)
	require.EqualValues(t, 1, realCount)
	require.Equal(t, beforeB, countAdminBills(t, db, adminB.ID))

	result, err := svc.VerifyForAdmin(context.Background(), adminA.ID)
	require.NoError(t, err)
	require.Equal(t, VerificationPassed, result.Status, result.Errors)
}

func TestAppendDueDaysForAdminOnlyMutatesThatAdminsDemo(t *testing.T) {
	db := newDemoServiceTestDB(t)
	adminA := seedAdmin(t, db, "append-one-a@example.com")
	adminB := seedAdmin(t, db, "append-one-b@example.com")

	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})
	require.NoError(t, mustEnsure(svc, adminA.ID))
	require.NoError(t, mustEnsure(svc, adminB.ID))

	beforeA := countAdminBills(t, db, adminA.ID)
	beforeB := countAdminBills(t, db, adminB.ID)
	now = now.AddDate(0, 0, 2)

	require.NoError(t, svc.AppendDueDaysForAdmin(context.Background(), adminA.ID))

	require.Greater(t, countAdminBills(t, db, adminA.ID), beforeA)
	require.Equal(t, beforeB, countAdminBills(t, db, adminB.ID))
}

func TestSummaryForAdminEnsuresAndReturnsFullAdminSnapshot(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "summary-admin@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	summary, err := svc.SummaryForAdmin(context.Background(), admin.ID, true)
	require.NoError(t, err)
	require.NotNil(t, summary.Instance)
	require.Len(t, summary.Businesses, 2)
	require.NotEmpty(t, summary.Runs)
	require.Equal(t, VerificationPassed, summary.Verification.Status, summary.Verification.Errors)
	requireCoveragePassed(t, summary.Verification, "menu_images", "bills", "orders", "crm", "inventory")
}

func mustEnsure(svc *Service, adminID uint) error {
	_, err := svc.EnsureForAdmin(context.Background(), adminID)
	return err
}

func requireCoveragePassed(t *testing.T, result VerificationResult, keys ...string) {
	t.Helper()
	byKey := make(map[string]CoverageCheck, len(result.Coverage))
	for _, check := range result.Coverage {
		byKey[check.Key] = check
	}
	for _, key := range keys {
		check, ok := byKey[key]
		require.Truef(t, ok, "missing coverage check %s", key)
		require.Equalf(t, CoveragePassed, check.Status, "coverage %s failed: %s", key, check.Message)
		require.Greaterf(t, check.Count, int64(0), "coverage %s should count seeded rows", key)
	}
}

func assertThirtyDayWindow(t *testing.T, db *gorm.DB, adminID uint, wantStart, wantEnd string) {
	t.Helper()
	var first, last database.Bill
	require.NoError(t, db.Model(&database.Bill{}).
		Joins("JOIN businesses ON businesses.id = bills.business_id").
		Where("businesses.demo_owner_user_id = ?", adminID).
		Order("bills.created_at ASC").
		First(&first).Error)
	require.NoError(t, db.Model(&database.Bill{}).
		Joins("JOIN businesses ON businesses.id = bills.business_id").
		Where("businesses.demo_owner_user_id = ?", adminID).
		Order("bills.created_at DESC").
		First(&last).Error)
	require.Equal(t, wantStart, first.CreatedAt.Format("2006-01-02"))
	require.Equal(t, wantEnd, last.CreatedAt.Format("2006-01-02"))
}

func assertBillMoneyInvariant(t *testing.T, db *gorm.DB, adminID uint) {
	t.Helper()
	var bills []database.Bill
	require.NoError(t, db.
		Joins("JOIN businesses ON businesses.id = bills.business_id").
		Where("businesses.demo_owner_user_id = ?", adminID).
		Find(&bills).Error)
	require.NotEmpty(t, bills)
	for _, bill := range bills {
		var items []database.BillItem
		require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&items).Error)
		require.NotEmpty(t, items)
		var subtotal int64
		for _, item := range items {
			subtotal += int64(math.Round(item.Subtotal * 100))
		}
		require.Equalf(t, bill.Subtotal, subtotal, "bill %s item subtotal mismatch", bill.BillNumber)

		var paid int64
		require.NoError(t, db.Model(&database.Payment{}).
			Where("bill_id = ? AND status = ?", bill.ID, database.PaymentStatusConfirmed).
			Select("COALESCE(SUM(amount), 0)").Scan(&paid).Error)
		var altPaid int64
		require.NoError(t, db.Model(&database.AlternativePayment{}).
			Where("bill_id = ? AND status = ?", bill.ID, database.AltPaymentStatusConfirmed).
			Select("COALESCE(SUM(amount), 0)").Scan(&altPaid).Error)
		recognizedPaid := paid + altPaid
		require.Equalf(t, bill.PaidAmount, recognizedPaid, "bill %s paid amount mismatch", bill.BillNumber)
		if bill.Status == database.BillStatusPaid {
			require.Equalf(t, bill.TotalAmount, recognizedPaid, "bill %s payment mismatch", bill.BillNumber)
		}
	}
}

func assertAlternativePaymentsSeeded(t *testing.T, db *gorm.DB, adminID uint) {
	t.Helper()
	var confirmed, pending int64
	require.NoError(t, db.Model(&database.AlternativePayment{}).
		Joins("JOIN bills ON bills.id = alternative_payments.bill_id").
		Joins("JOIN businesses ON businesses.id = bills.business_id").
		Where("businesses.demo_owner_user_id = ? AND alternative_payments.status = ?", adminID, database.AltPaymentStatusConfirmed).
		Count(&confirmed).Error)
	require.NoError(t, db.Model(&database.AlternativePayment{}).
		Joins("JOIN bills ON bills.id = alternative_payments.bill_id").
		Joins("JOIN businesses ON businesses.id = bills.business_id").
		Where("businesses.demo_owner_user_id = ? AND alternative_payments.status = ?", adminID, database.AltPaymentStatusPending).
		Count(&pending).Error)
	require.Greater(t, confirmed, int64(0), "demo should seed confirmed alternative payments for accounting/payment history")
	require.Greater(t, pending, int64(0), "demo should seed pending alternative payments for the alternative payments UI")
}

func assertDeliveryPaymentFixturesSeeded(t *testing.T, db *gorm.DB, adminID uint, now time.Time) {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Joins("JOIN bills ON bills.id = delivery_orders.bill_id").
		Joins("JOIN businesses ON businesses.id = delivery_orders.business_id").
		Where("businesses.demo_owner_user_id = ?", adminID).
		Where("delivery_orders.status = ?", database.DeliveryStatusConfirmed).
		Where("delivery_orders.payment_expires_at > ?", now).
		Where("bills.status <> ?", database.BillStatusPaid).
		Count(&count).Error)
	require.Greater(t, count, int64(0), "demo should seed an awaiting-payment delivery for /delivery/:number/pay")
}

func countAdminBills(t *testing.T, db *gorm.DB, adminID uint) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&database.Bill{}).
		Joins("JOIN businesses ON businesses.id = bills.business_id").
		Where("businesses.demo_owner_user_id = ?", adminID).
		Count(&count).Error)
	return count
}

func countAdminBusinessRows(t *testing.T, db *gorm.DB, adminID uint, model interface{}) int64 {
	t.Helper()
	var businessIDs []uint
	require.NoError(t, db.Model(&database.Business{}).
		Where("demo_owner_user_id = ?", adminID).
		Pluck("id", &businessIDs).Error)
	require.NotEmpty(t, businessIDs)

	var count int64
	require.NoError(t, db.Model(model).Where("business_id IN ?", businessIDs).Count(&count).Error)
	return count
}

func modelForStaticCount(key string) interface{} {
	switch key {
	case "shifts":
		return &database.Shift{}
	case "staff_positions":
		return &database.StaffPosition{}
	case "chat_channel_members":
		return &database.ChatChannelMember{}
	case "chat_reads":
		return &database.ChatRead{}
	case "announcement_acks":
		return &database.AnnouncementAck{}
	case "document_acks":
		return &database.DocumentAck{}
	case "poll_votes":
		return &database.PollVote{}
	case "director_proposed_actions":
		return &database.DirectorProposedAction{}
	case "manual_ledger":
		return &database.ManualLedgerEntry{}
	case "payroll_runs":
		return &database.PayrollRun{}
	case "payroll_line_items":
		return &database.PayrollLineItem{}
	case "loyalty_programs":
		return &database.LoyaltyProgram{}
	case "business_plugins":
		return &database.BusinessPlugin{}
	case "time_off_requests":
		return &database.TimeOffRequest{}
	case "open_shift_claims":
		return &database.OpenShiftClaim{}
	case "shift_swap_requests":
		return &database.ShiftSwapRequest{}
	case "shift_notes":
		return &database.ShiftNote{}
	default:
		panic("unknown static count model: " + key)
	}
}
