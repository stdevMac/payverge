package demo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// BenchmarkEnsureForAdmin measures the full demo generator write path
// (static seed + baseline day generation). Task 17 performance gate.
func BenchmarkEnsureForAdmin(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		dsn := fmt.Sprintf("file:bench-demo-%d-%d?mode=memory&cache=shared", time.Now().UnixNano(), i)
		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
			DisableForeignKeyConstraintWhenMigrating: true,
			Logger:                                   logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			b.Fatal(err)
		}
		if err := db.AutoMigrate(demoTestModels()...); err != nil {
			b.Fatal(err)
		}
		if err := db.Exec(`CREATE TABLE IF NOT EXISTS bill_items (
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
		)`).Error; err != nil {
			b.Fatal(err)
		}
		admin := database.User{Email: fmt.Sprintf("bench-admin-%d-%d@example.com", time.Now().UnixNano(), i), Role: "admin"}
		if err := db.Create(&admin).Error; err != nil {
			b.Fatal(err)
		}
		now := time.Date(2026, 7, 2, 16, 0, 0, 0, time.UTC)
		svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "bench-seed", BaselineDays: 7})
		b.StartTimer()
		if _, err := svc.EnsureForAdmin(context.Background(), admin.ID); err != nil {
			b.Fatal(err)
		}
	}
}
