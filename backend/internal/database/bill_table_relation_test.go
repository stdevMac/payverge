package database

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// TestBillTableRelationNeverSynthesizesAForeignKey pins the `-:migration` tag on
// Bill.Table. bills.table_id uses 0 as the "no table" sentinel for delivery and
// counter bills, so a foreign key to tables(id) would reject them. Genesis has
// no such key (TestGenesisInvariants_NoAutoMigrateTableFK); this keeps test
// fixtures built with GORM AutoMigrate from synthesizing one, while
// Preload("Table") and serialization keep working.
func TestBillTableRelationNeverSynthesizesAForeignKey(t *testing.T) {
	s, err := schema.Parse(&Bill{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse Bill schema: %v", err)
	}
	rel, ok := s.Relationships.Relations["Table"]
	if !ok {
		t.Fatal(`Bill.Table relation is gone; Preload("Table") would break`)
	}
	if !rel.Field.IgnoreMigration {
		t.Fatal("Bill.Table relation participates in migration: AutoMigrate would synthesize fk_bills_table and reject table_id = 0 sentinel bills")
	}
	if !rel.Field.Readable {
		t.Fatal("Bill.Table relation is not readable; API responses would drop the table")
	}
}
