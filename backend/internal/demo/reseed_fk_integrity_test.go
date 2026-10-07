package demo

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Live-review regression (2026-07-03): the v3 forced reseed FAILED in
// production — the wipe deleted businesses while rows in tables it never
// touched still referenced them (business_revenue_aggregates is backfilled
// for EVERY business at startup; offers, AI-waiter chats, director tool
// calls, languages accrue from live usage). Postgres enforces those foreign
// keys, so the delete rolled back and the stale showroom survived every
// deploy. The SQLite test harness ran with FK enforcement OFF
// (DisableForeignKeyConstraintWhenMigrating + no pragma), so the suite
// stayed green. This harness turns enforcement ON to mirror production.
func newFKEnforcedDemoDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared&_foreign_keys=on"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
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

func TestForcedReseedSurvivesAccruedRowsWithEnforcedFKs(t *testing.T) {
	db := newFKEnforcedDemoDB(t)
	admin := seedAdmin(t, db, "demo-admin-fk@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed-v1", BaselineDays: 2})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var before []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Order("id ASC").Find(&before).Error)
	require.Len(t, before, 2)
	beforeIDs := []uint{before[0].ID, before[1].ID}

	// Simulate what production accrues on top of the seed between deploys.
	now := fixedNow()
	for _, business := range before {
		// Startup backfill writes one aggregate per business with bills —
		// guaranteed to exist, and the deterministic reseed blocker.
		require.NoError(t, db.Create(&database.BusinessRevenueAggregate{
			BusinessID: business.ID, NetRevenueCents: 100000, GrossRevenueCents: 110000,
		}).Error)
		require.NoError(t, db.Create(&database.BusinessMilestoneEvent{
			BusinessID: business.ID, MilestoneType: "first_bill", Status: "sent",
		}).Error)
		require.NoError(t, db.Create(&database.Offer{
			BusinessID: business.ID, Name: "Happy Hour", DiscountType: "percentage",
			DiscountValue: 10, IsActive: true,
		}).Error)
		require.NoError(t, db.Create(&database.BusinessLanguage{
			BusinessID: business.ID, LanguageCode: "es", IsDefault: false,
		}).Error)
		require.NoError(t, db.Create(&database.BusinessCurrency{
			BusinessID: business.ID, CurrencyCode: "USD", IsPreferred: true,
		}).Error)
		conversation := database.AiWaiterConversation{
			SessionID: "sess-" + business.BusinessId, BusinessID: business.ID,
			TableCode: "T1", Language: "en", Mode: "ordering", Status: "active",
		}
		require.NoError(t, db.Create(&conversation).Error)
		require.NoError(t, db.Create(&database.AiWaiterMessage{
			ConversationID: conversation.ID, Role: "user", Content: "hola", CreatedAt: now,
		}).Error)
		// A menu photo-extraction run: the page images FK-reference the job
		// (fk_menu_extraction_jobs_images). This is the exact pair that kept
		// legacy production demo instances failed — the wipe deleted jobs
		// without their images and Postgres rolled the whole reseed back.
		job := database.MenuExtractionJob{
			BusinessID: business.ID, Status: database.ExtractionStatusCompleted, ImageCount: 1,
		}
		require.NoError(t, db.Create(&job).Error)
		require.NoError(t, db.Create(&database.MenuExtractionImage{
			JobID: job.ID, FilePath: "menus/demo-page-1.jpg", PageOrder: 1, MIMEType: "image/jpeg",
		}).Error)
	}

	// A director tool call logged against the seeded briefing thread.
	var thread database.DirectorConsoleThread
	require.NoError(t, db.Where("business_id = ?", beforeIDs[1]).First(&thread).Error)
	require.NoError(t, db.Create(&database.DirectorToolCall{
		ThreadID: thread.ID, BusinessID: beforeIDs[1], ToolName: "analytics.revenue",
		ArgsJSON: "{}", CreatedAt: now,
	}).Error)

	// A guest split-pay hold on a seeded bill.
	var bill database.Bill
	require.NoError(t, db.Where("business_id = ?", beforeIDs[1]).First(&bill).Error)
	require.NoError(t, db.Create(&database.BillSplitShare{
		BillID: bill.ID, GuestSessionID: "guest-1", Mode: "items",
		AmountCents: 500, Status: "held",
	}).Error)

	// Grandchildren that accrue from LIVE use of the showroom and hang off a
	// demo row rather than off the business: a staff login code minted the
	// first time anyone signs into a demo dashboard, the per-attempt log of an
	// outbound plugin notification, and the refund-destination evidence row
	// bound to a payment. Every one of these FK-references its parent with NO
	// ACTION, so a wipe that misses it aborts the whole forced reseed (#921).
	for _, businessID := range beforeIDs {
		var staffRow database.Staff
		require.NoError(t, db.Where("business_id = ?", businessID).First(&staffRow).Error)
		require.NoError(t, db.Create(&database.StaffLoginCode{
			StaffID: staffRow.ID, Code: fmt.Sprintf("%06d", 100000+businessID),
			ExpiresAt: now.Add(10 * time.Minute), CreatedAt: now,
		}).Error)

		delivery := database.PluginNotificationDelivery{
			BusinessID: businessID, PluginName: "telegram", EventType: "bill.paid",
			EventID: fmt.Sprintf("evt-%d", businessID), Status: "delivered",
			NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, db.Create(&delivery).Error)
		require.NoError(t, db.Create(&database.PluginNotificationDeliveryAttempt{
			DeliveryID: delivery.ID, AttemptNumber: 1, Status: "delivered", AttemptedAt: now,
		}).Error)

		var paidBill database.Bill
		require.NoError(t, db.Where("business_id = ?", businessID).First(&paidBill).Error)
		payment := database.Payment{
			BillID: paidBill.ID, Amount: 1000, Status: database.PaymentStatusConfirmed,
			TxHash:    fmt.Sprintf("0xdemo-refund-evidence-%d", businessID),
			CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, db.Create(&payment).Error)
		require.NoError(t, db.Create(&database.PaymentRefundDestination{
			PaymentID: payment.ID, ChainID: 8453, Token: "USDC", AmountBaseUnits: 1000,
			RefundAddress: "0x0000000000000000000000000000000000000001",
			EvidenceType:  database.RefundEvidenceWalletSignature, VerifiedAt: now, CreatedAt: now,
		}).Error)
	}

	// Force the reseed the way the version bump does.
	require.NoError(t, db.Model(&database.DemoInstance{}).
		Where("admin_user_id = ?", admin.ID).
		Update("seed_version", "").Error)

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err, "forced reseed must survive accrued FK children under enforced foreign keys")

	var after []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Order("id ASC").Find(&after).Error)
	require.Len(t, after, 2)
	for _, business := range after {
		require.NotContains(t, beforeIDs, business.ID, "old business rows must be wiped, not kept")
	}

	// The accrued rows scoped to the dead businesses must be gone too — no
	// orphans pointing at business IDs that no longer exist.
	for model, column := range map[interface{}]string{
		&database.BusinessRevenueAggregate{}: "business_id",
		&database.BusinessMilestoneEvent{}:   "business_id",
		&database.Offer{}:                    "business_id",
		&database.BusinessLanguage{}:         "business_id",
		&database.BusinessCurrency{}:         "business_id",
		&database.AiWaiterConversation{}:     "business_id",
		&database.MenuExtractionJob{}:        "business_id",
	} {
		var count int64
		require.NoError(t, db.Model(model).Where(column+" IN ?", beforeIDs).Count(&count).Error)
		require.Zero(t, count, "orphaned rows left in %T", model)
	}
	// Every extraction image in this DB belonged to a wiped job — none may survive.
	// The live-accrued grandchildren must be gone with their parents.
	for model, table := range map[interface{}]string{
		&database.StaffLoginCode{}:                    "staff login codes",
		&database.PluginNotificationDeliveryAttempt{}: "plugin notification attempts",
		&database.PaymentRefundDestination{}:          "payment refund destinations",
	} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.Zerof(t, count, "%s outlived the wiped demo", table)
	}

	var orphanImages int64
	require.NoError(t, db.Model(&database.MenuExtractionImage{}).Count(&orphanImages).Error)
	require.Zero(t, orphanImages, "extraction page images must be wiped with their jobs")

	var instance database.DemoInstance
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).First(&instance).Error)
	require.Equal(t, "test-seed-v1", instance.SeedVersion)
	require.Equal(t, database.DemoInstanceStatusReady, instance.Status)

	_ = time.Now // keep time import if the harness stops using it
}
