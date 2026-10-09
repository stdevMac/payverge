package database

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// BenchmarkReserveImageGeneration measures the per-generation metering cost.
// The limit is set absurdly high so the benchmark measures the success path,
// not the rejection path.
func BenchmarkReserveImageGeneration(b *testing.B) {
	gormDB, err := gorm.Open(sqlite.Open("file:ai_image_usage_bench?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		b.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	_ = gormDB.Migrator().DropTable(&AIImageUsage{})
	if err := gormDB.AutoMigrate(&AIImageUsage{}); err != nil {
		b.Fatal(err)
	}
	db = gormDB

	biz := &Business{ID: 1}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ReserveImageGeneration(biz, 1<<30, 1<<30); err != nil {
			b.Fatalf("reserve failed: %v", err)
		}
	}
}
