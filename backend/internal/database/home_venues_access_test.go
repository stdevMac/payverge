package database

import (
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// FindPublishedVenue backs the public instance home ("/"). Every lookup
// stage must be able to use an index: the numeric id (primary key),
// LOWER(custom_url) (idx_businesses_custom_url_lower) and business_id as an
// exact match (idx_businesses_business_id). A LOWER(business_id) predicate
// cannot use the case-sensitive unique index and scans businesses on every
// cache miss for an unknown or business_id-shaped PRIMARY_VENUE.
func TestFindPublishedVenueBusinessIDStageIsIndexable(t *testing.T) {
	seedHomeVenues(t)
	rec := &billProjectionSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	base := db
	db = db.Session(&gorm.Session{Logger: rec})
	t.Cleanup(func() { db = base })

	if _, err := FindPublishedVenue("missing-venue"); err != nil {
		t.Fatalf("FindPublishedVenue: %v", err)
	}
	if len(rec.statements) != 2 {
		t.Fatalf("want 2 lookups (custom_url, business_id), got %d:\n%v", len(rec.statements), rec.statements)
	}
	for _, s := range rec.statements {
		n := strings.ToLower(s)
		if strings.Contains(n, "lower(business_id)") {
			t.Fatalf("business_id stage must be an exact, indexable match:\n%s", s)
		}
		if strings.Contains(n, "select *") || strings.Contains(n, "`businesses`.*") {
			t.Fatalf("lookup must project columns:\n%s", s)
		}
	}
	if !strings.Contains(strings.ToLower(rec.statements[1]), "business_id = ") {
		t.Fatalf("second stage must match business_id exactly:\n%s", rec.statements[1])
	}
}

// business_id is canonical as stored; the reference matches it exactly.
func TestFindPublishedVenueBusinessIDIsExact(t *testing.T) {
	seedHomeVenues(t)
	if v, err := FindPublishedVenue("v-a"); err != nil || v == nil || v.Name != "Alpha" {
		t.Fatalf("exact business_id: %+v %v", v, err)
	}
	if v, err := FindPublishedVenue("V-A"); err != nil || v != nil {
		t.Fatalf("business_id match must be exact, got %+v %v", v, err)
	}
}

// Performance Gate: the public "/" cache-miss path for a PRIMARY_VENUE that
// resolves by business_id, and for one that resolves to nothing.
//
//	go test ./internal/database -run '^$' -bench 'FindPublishedVenue' -benchmem -count=3
func BenchmarkFindPublishedVenue(b *testing.B) {
	t := &testing.T{}
	setupTestDB(t)
	rows := make([]Business, 0, 2000)
	for i := 0; i < 2000; i++ {
		rows = append(rows, Business{
			BusinessId:          fmt.Sprintf("venue-%04d", i),
			Name:                fmt.Sprintf("Venue %04d", i),
			CustomURL:           fmt.Sprintf("venue-page-%04d", i),
			BusinessPageEnabled: true,
			IsActive:            true,
		})
	}
	if err := db.CreateInBatches(rows, 200).Error; err != nil {
		b.Fatalf("seed: %v", err)
	}
	for _, ref := range []string{"venue-1999", "missing-venue"} {
		b.Run(ref, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := FindPublishedVenue(ref); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
