package main

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demo"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// cliDemoTestModels mirrors internal/demo's demoTestModels (test files are
// not importable across packages): every table the demo seeder and its
// owned-data wipe touch.
func cliDemoTestModels() []interface{} {
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

func newCLIDemoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newCLITestDB(t, cliDemoTestModels()...)
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

// fastCLIDemo pins the clock and shrinks the baseline window so the real
// demo seeder runs in a unit test.
func fastCLIDemo(db *gorm.DB) cliDemoService {
	return demo.NewService(db, demo.Options{
		Now:          func() time.Time { return time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC) },
		SeedVersion:  "cli-test-seed",
		BaselineDays: 2,
	})
}

func seedCLIUser(t *testing.T, db *gorm.DB, email, role string) database.User {
	t.Helper()
	u := database.User{Email: email, Role: role, AuthMethod: "email", EmailVerified: true}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func demoBusinessesFor(t *testing.T, db *gorm.DB, ownerID uint) []database.Business {
	t.Helper()
	var rows []database.Business
	require.NoError(t, db.Where("user_id = ?", ownerID).Order("id ASC").Find(&rows).Error)
	return rows
}

func TestDemoSeedIsIdempotentAndOwnedByTheAdmin(t *testing.T) {
	db := newCLIDemoTestDB(t)
	admin := seedCLIUser(t, db, "owner@example.com", "admin")

	h := newCLIHarness(t, db, "", nil)
	h.env.newDemo = fastCLIDemo
	require.Equal(t, cliExitOK, h.run("demo", "seed", "--owner-email", "Owner@Example.com"), h.stderr.String())
	require.Contains(t, h.stdout.String(), "Demo restaurants ready for")

	first := demoBusinessesFor(t, db, admin.ID)
	require.NotEmpty(t, first)
	for _, b := range first {
		require.True(t, b.IsDemo, "seeded restaurants are flagged is_demo")
		require.Equal(t, database.BusinessKindDemo, b.Kind)
	}

	h = newCLIHarness(t, db, "", nil)
	h.env.newDemo = fastCLIDemo
	require.Equal(t, cliExitOK, h.run("demo", "seed", "--owner-email", "owner@example.com"), h.stderr.String())

	var instances int64
	require.NoError(t, db.Model(&database.DemoInstance{}).Count(&instances).Error)
	require.Equal(t, int64(1), instances, "a second seed re-ensures the same instance")
	second := demoBusinessesFor(t, db, admin.ID)
	require.Len(t, second, len(first), "a second seed adds no restaurants")
	for i := range first {
		require.Equal(t, first[i].ID, second[i].ID)
	}
}

func TestDemoSeedDefaultsToAdminEmailAndRefusesNonAdmins(t *testing.T) {
	db := newCLIDemoTestDB(t)
	seedCLIUser(t, db, "staff@example.com", "user")

	h := newCLIHarness(t, db, "", nil)
	h.env.newDemo = fastCLIDemo
	require.Equal(t, cliExitUsage, h.run("demo", "seed"), "no owner and no ADMIN_EMAIL")
	require.Zero(t, h.opened)

	h = newCLIHarness(t, db, "", map[string]string{"ADMIN_EMAIL": "staff@example.com"})
	h.env.newDemo = fastCLIDemo
	require.Equal(t, cliExitError, h.run("demo", "seed"))
	require.Contains(t, h.stderr.String(), "is not a platform admin")
	require.Contains(t, h.stderr.String(), "takes the account over", "the hint warns that admin create re-keys a non-admin account")
	require.NotContains(t, h.stderr.String(), "promotes an existing user")

	h = newCLIHarness(t, db, "", nil)
	h.env.newDemo = fastCLIDemo
	require.Equal(t, cliExitError, h.run("demo", "seed", "--owner-email", "ghost@example.com"))
	require.Contains(t, h.stderr.String(), "no active account")

	var instances int64
	require.NoError(t, db.Model(&database.DemoInstance{}).Count(&instances).Error)
	require.Zero(t, instances)
}

type recordingDemoService struct {
	ensured []uint
	reset   []uint
}

func (r *recordingDemoService) EnsureForAdmin(_ context.Context, id uint) (*database.DemoInstance, error) {
	r.ensured = append(r.ensured, id)
	return &database.DemoInstance{ID: 1, AdminUserID: id, Status: database.DemoInstanceStatusReady}, nil
}

func (r *recordingDemoService) ResetForAdmin(_ context.Context, id uint) (*database.DemoInstance, error) {
	r.reset = append(r.reset, id)
	return &database.DemoInstance{ID: 1, AdminUserID: id, Status: database.DemoInstanceStatusReady}, nil
}

func TestDemoResetTargetsOneOwnerOrAllInstances(t *testing.T) {
	db := newCLIDemoTestDB(t)
	a := seedCLIUser(t, db, "a@example.com", "admin")
	b := seedCLIUser(t, db, "b@example.com", "admin")
	for _, id := range []uint{b.ID, a.ID} {
		require.NoError(t, db.Create(&database.DemoInstance{AdminUserID: id, Status: database.DemoInstanceStatusReady, SeedVersion: "x", BaselineStartDate: time.Now(), Timezone: "UTC"}).Error)
	}

	h := newCLIHarness(t, db, "", nil)
	require.Equal(t, cliExitUsage, h.run("demo", "reset"), "exactly one of --owner-email / --all")
	h = newCLIHarness(t, db, "", nil)
	require.Equal(t, cliExitUsage, h.run("demo", "reset", "--all", "--owner-email", "a@example.com"))

	rec := &recordingDemoService{}
	h = newCLIHarness(t, db, "", nil)
	h.env.newDemo = func(*gorm.DB) cliDemoService { return rec }
	require.Equal(t, cliExitOK, h.run("demo", "reset", "--owner-email", "b@example.com"), h.stderr.String())
	require.Equal(t, []uint{b.ID}, rec.reset)

	rec = &recordingDemoService{}
	h = newCLIHarness(t, db, "", nil)
	h.env.newDemo = func(*gorm.DB) cliDemoService { return rec }
	require.Equal(t, cliExitOK, h.run("demo", "reset", "--all"), h.stderr.String())
	require.Equal(t, []uint{a.ID, b.ID}, rec.reset)
}

func TestDemoResetWrapsTheRealService(t *testing.T) {
	db := newCLIDemoTestDB(t)
	admin := seedCLIUser(t, db, "owner@example.com", "admin")
	h := newCLIHarness(t, db, "", nil)
	h.env.newDemo = fastCLIDemo
	require.Equal(t, cliExitOK, h.run("demo", "seed", "--owner-email", "owner@example.com"), h.stderr.String())
	var before database.DemoInstance
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).First(&before).Error)

	h = newCLIHarness(t, db, "", nil)
	h.env.newDemo = fastCLIDemo
	require.Equal(t, cliExitOK, h.run("demo", "reset", "--owner-email", "owner@example.com"), h.stderr.String())
	require.Contains(t, h.stdout.String(), "Demo data reset")
	var after []database.DemoInstance
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).Find(&after).Error)
	require.Len(t, after, 1)
	require.Equal(t, database.DemoInstanceStatusReady, after[0].Status)
	require.NotEmpty(t, demoBusinessesFor(t, db, admin.ID))
}
