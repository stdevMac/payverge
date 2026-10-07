package services

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// setupDigestClaimTestDB opens an in-memory SQLite database and AutoMigrates
// the Business model (which includes director_digest_last_sent_at). The global test DB is wired so helpers that call
// database.GetDB() work.
func setupDigestClaimTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("setupDigestClaimTestDB: open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("setupDigestClaimTestDB: sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&database.Business{}); err != nil {
		t.Fatalf("setupDigestClaimTestDB: AutoMigrate: %v", err)
	}
	database.SetTestDB(db)
	return db
}

// seedTestBusiness inserts a minimal AI Pro business row and returns it.
func seedTestBusiness(t *testing.T) database.Business {
	t.Helper()
	db := database.GetDB()
	biz := database.Business{
		BusinessId:   fmt.Sprintf("test-biz-%s", t.Name()),
		Name:         "Test Bistro",
		OwnerAddress: "0x0000000000000000000000000000000000000001",
		IsActive:     true,
		// SettlementAddr and TippingAddr are NOT NULL in the schema.
		SettlementAddr: "0x0000000000000000000000000000000000000002",
		TippingAddr:    "0x0000000000000000000000000000000000000003",
	}
	if err := db.Create(&biz).Error; err != nil {
		t.Fatalf("seedTestBusiness: %v", err)
	}
	return biz
}

// TestDirectorDigestClaimOncePerDay asserts the DB-backed claim lets a digest
// fire once and blocks the same-day repeat, surviving the (now removed)
// in-memory map / a restart.
func TestDirectorDigestClaimOncePerDay(t *testing.T) {
	db := setupDigestClaimTestDB(t)
	biz := seedTestBusiness(t)
	s := NewDirectorDigestScheduler(db, nil, nil)

	// Pin midday UTC so now.Add(time.Minute) cannot cross the UTC day boundary
	// (wall-clock 23:59:xx + 1m used to make the "same-day" assert flake).
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	if !s.claimDigest(biz.ID, now) {
		t.Fatal("first claim should win")
	}
	if s.claimDigest(biz.ID, now.Add(time.Minute)) {
		t.Fatal("same-day second claim should lose")
	}
	if !s.claimDigest(biz.ID, now.Add(25*time.Hour)) {
		t.Fatal("next-day claim should win")
	}
}
