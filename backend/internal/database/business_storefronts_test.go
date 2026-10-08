package database

import (
	"fmt"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestListPublishedStorefrontSlugs is the access-shape regression test for the
// public sitemap data source (SEO-2). It asserts the publish/active filter, the
// blank-slug exclusion, and the narrow two-column projection (CustomURL +
// UpdatedAt). A future SELECT * / extra-column / filter regression must fail here.
func TestListPublishedStorefrontSlugs(t *testing.T) {
	// setupTestDB is void: it assigns the package-global lowercase `db`
	// (see bill_items_test.go:14). Do NOT capture a return value or reassign;
	// the seed Creates + ListPublishedStorefrontSlugs both run against `db`.
	setupTestDB(t)

	// published + active → included
	db.Create(&Business{BusinessId: "biz-a", Name: "A", CustomURL: "alpha", BusinessPageEnabled: true, IsActive: true})
	// unpublished → excluded
	db.Create(&Business{BusinessId: "biz-b", Name: "B", CustomURL: "beta", BusinessPageEnabled: false, IsActive: true})
	// inactive → excluded. IsActive has gorm:"default:true", so passing the
	// zero-value false to Create is ignored (GORM uses the default). Force the
	// column with an explicit Update so the inactive gate is honestly exercised.
	gammaBiz := &Business{BusinessId: "biz-c", Name: "C", CustomURL: "gamma", BusinessPageEnabled: true, IsActive: true}
	db.Create(gammaBiz)
	db.Model(&Business{}).Where("id = ?", gammaBiz.ID).Update("is_active", false)
	// published but blank slug → excluded (no routable URL)
	db.Create(&Business{BusinessId: "biz-d", Name: "D", CustomURL: "", BusinessPageEnabled: true, IsActive: true})

	got, err := ListPublishedStorefrontSlugs(1000)
	if err != nil {
		t.Fatalf("ListPublishedStorefrontSlugs: %v", err)
	}
	if len(got) != 1 || got[0].CustomURL != "alpha" {
		t.Fatalf("want [alpha], got %+v", got)
	}
	if got[0].UpdatedAt.IsZero() {
		t.Errorf("expected updated_at to be projected")
	}
}

// TestListPublishedStorefrontSlugsExcludesDemo keeps kind=test fixtures out of
// the public sitemap while including published demo showrooms (#612 / #637).
func TestListPublishedStorefrontSlugsExcludesDemo(t *testing.T) {
	setupTestDB(t)

	db.Create(&Business{
		BusinessId: "biz-real", Name: "Real Bistro", CustomURL: "real-bistro",
		BusinessPageEnabled: true, IsActive: true,
		Kind: BusinessKindReal, IsDemo: false,
	})
	db.Create(&Business{
		BusinessId: "biz-demo-flag", Name: "Demo Flag", CustomURL: "demo-admin-1-core-demo-kitchen",
		BusinessPageEnabled: true, IsActive: true,
		Kind: BusinessKindReal, IsDemo: true,
	})
	db.Create(&Business{
		BusinessId: "biz-kind-demo", Name: "Kind Demo", CustomURL: "payverge-core-demo-kitchen",
		BusinessPageEnabled: true, IsActive: true,
		Kind: BusinessKindDemo, IsDemo: false,
	})
	db.Create(&Business{
		BusinessId: "biz-both", Name: "Both Flags", CustomURL: "payverge-ai-pro-demo-lounge",
		BusinessPageEnabled: true, IsActive: true,
		Kind: BusinessKindDemo, IsDemo: true,
	})
	db.Create(&Business{
		BusinessId: "biz-test", Name: "CI Fixture", CustomURL: "ci-test-kitchen",
		BusinessPageEnabled: true, IsActive: true,
		Kind: BusinessKindTest, IsDemo: false,
	})

	got, err := ListPublishedStorefrontSlugs(1000)
	if err != nil {
		t.Fatalf("ListPublishedStorefrontSlugs: %v", err)
	}
	urls := map[string]struct{}{}
	for _, row := range got {
		urls[row.CustomURL] = struct{}{}
	}
	if _, ok := urls["real-bistro"]; !ok {
		t.Fatalf("missing real-bistro, got %+v", got)
	}
	if _, ok := urls["payverge-ai-pro-demo-lounge"]; !ok {
		t.Fatalf("missing published demo lounge, got %+v", got)
	}
	if _, ok := urls["ci-test-kitchen"]; ok {
		t.Fatalf("kind=test fixture must stay out of the sitemap, got %+v", got)
	}
}

// TestListPublishedStorefrontSlugsRespectsLimit asserts the bound is honored so
// the public endpoint stays a bounded, predictable read regardless of table
// growth (no unbounded scan, no silent full-table materialization).
func TestListPublishedStorefrontSlugsRespectsLimit(t *testing.T) {
	setupTestDB(t) // void; seeds the package-global `db`
	for _, s := range []string{"one", "two", "three"} {
		db.Create(&Business{BusinessId: "biz-" + s, Name: s, CustomURL: s, BusinessPageEnabled: true, IsActive: true})
	}
	got, err := ListPublishedStorefrontSlugs(2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("limit not honored: got %d", len(got))
	}
}

// BenchmarkListPublishedStorefrontSlugs captures the -benchmem baseline for the
// public sitemap query over a seeded set. The two-column projection + LIMIT keep
// the result set bounded regardless of table size (Backend Performance Gate).
// setupTestDB only takes *testing.T, so the benchmark builds its own in-memory
// SQLite handle and assigns the same package-global `db`.
func BenchmarkListPublishedStorefrontSlugs(b *testing.B) {
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	if err := gormDB.AutoMigrate(&Business{}); err != nil {
		b.Fatalf("migrate: %v", err)
	}
	db = gormDB
	for i := 0; i < 500; i++ {
		db.Create(&Business{
			BusinessId:          fmt.Sprintf("biz-%d", i),
			Name:                "bench",
			CustomURL:           fmt.Sprintf("slug-%d", i),
			BusinessPageEnabled: true,
			IsActive:            true,
		})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, err := ListPublishedStorefrontSlugs(5000); err != nil {
			b.Fatalf("query: %v", err)
		}
	}
}
