package database

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestDemoInstance_ShowroomColumnNames pins the physical column names of the
// two demo showroom venues to the genesis schema (migration 000004). The old
// plan-tier field name AIProBusinessID was snake-cased by GORM to
// "a_ipro_business_id", which once dirtied a migration; the primary/secondary
// names must map cleanly with no explicit column: tag.
func TestDemoInstance_ShowroomColumnNames(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&DemoInstance{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	for _, col := range []string{"primary_business_id", "secondary_business_id"} {
		if !db.Migrator().HasColumn(&DemoInstance{}, col) {
			t.Errorf("demo_instances missing canonical column %s (matches genesis + json tag)", col)
		}
	}
	for _, col := range []string{"core_business_id", "ai_pro_business_id", "a_ipro_business_id"} {
		if db.Migrator().HasColumn(&DemoInstance{}, col) {
			t.Errorf("demo_instances still has retired plan-tier column %s", col)
		}
	}
}
