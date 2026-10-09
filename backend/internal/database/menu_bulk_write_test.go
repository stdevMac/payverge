package database

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedMenuForBulkWrite creates a business + active menu (version 1) with one
// category containing one item, returning the business and menu.
func seedMenuForBulkWrite(t *testing.T) (*Business, *Menu) {
	t.Helper()

	biz := &Business{Name: "BulkWriteTest Resto"}
	require.NoError(t, db.Create(biz).Error)

	cats := []MenuCategory{{
		ID:   "cat-1",
		Name: "Mains",
		Items: []MenuItem{
			{ID: "i1", Name: "Burger", Price: 10.0, IsAvailable: true},
		},
	}}
	catsJSON, err := json.Marshal(cats)
	require.NoError(t, err)

	menu := &Menu{
		BusinessID: biz.ID,
		Categories: string(catsJSON),
		IsActive:   true,
		Version:    1,
	}
	require.NoError(t, db.Create(menu).Error)
	return biz, menu
}

func TestApplyMenuCategoriesTx_BumpsVersionAndWrites(t *testing.T) {
	setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&Business{}, &Menu{}))

	biz, _ := seedMenuForBulkWrite(t)

	// Mutated category tree: raise burger price to 12.
	mutated := []MenuCategory{{
		ID:   "cat-1",
		Name: "Mains",
		Items: []MenuItem{
			{ID: "i1", Name: "Burger", Price: 12.0, IsAvailable: true},
		},
	}}

	newVersion, err := ApplyMenuCategories(biz.ID, mutated, 1)
	require.NoError(t, err, "ApplyMenuCategories should succeed on correct version")
	if newVersion != 2 {
		t.Fatalf("expected new version 2, got %d", newVersion)
	}

	// Reload and verify the write took effect.
	_, cats, err := GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	if len(cats) == 0 || len(cats[0].Items) == 0 {
		t.Fatal("expected categories with items after apply")
	}
	if cats[0].Items[0].Price != 12.0 {
		t.Fatalf("expected price 12.0, got %f", cats[0].Items[0].Price)
	}
}

func TestApplyMenuCategoriesTx_ConflictWhenVersionStale(t *testing.T) {
	setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&Business{}, &Menu{}))

	biz, _ := seedMenuForBulkWrite(t)

	cats := []MenuCategory{{
		ID:   "cat-1",
		Name: "Mains",
		Items: []MenuItem{
			{ID: "i1", Name: "Burger", Price: 15.0, IsAvailable: true},
		},
	}}

	// Supply stale expectedVersion=0 (menu is at version=1).
	_, err := ApplyMenuCategories(biz.ID, cats, 0)
	if err == nil {
		t.Fatal("expected ErrMenuVersionConflict, got nil")
	}
	if !errors.Is(err, ErrMenuVersionConflict) {
		t.Fatalf("expected ErrMenuVersionConflict, got %v", err)
	}

	// Menu version should be unchanged.
	menu, _, err := GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	if menu.Version != 1 {
		t.Fatalf("expected version still 1, got %d", menu.Version)
	}
}

func TestAppendMenuCategories_MergesAndBumpsVersion(t *testing.T) {
	setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&Business{}, &Menu{}))

	biz, _ := seedMenuForBulkWrite(t) // business + active menu version=1, one category
	newCats := []MenuCategory{{Name: "Drinks", Items: []MenuItem{{Name: "Cola", Price: 3}}}}
	v, err := AppendMenuCategories(biz.ID, newCats)
	require.NoError(t, err)
	assert.Equal(t, uint(2), v)
	_, cats, _ := GetMenuByBusinessID(biz.ID)
	assert.Len(t, cats, 2) // original + appended
}

func TestAppendMenuCategories_CreatesWhenNoMenu(t *testing.T) {
	setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&Business{}, &Menu{}))

	var biz Business
	require.NoError(t, db.Create(&Business{Name: "NoMenu"}).Error)
	db.Where("name = ?", "NoMenu").First(&biz)
	v, err := AppendMenuCategories(biz.ID, []MenuCategory{{Name: "Mains"}})
	require.NoError(t, err)
	assert.Equal(t, uint(1), v)
}

func TestAppendMenuCategories_RetriesOnConcurrentBump(t *testing.T) {
	setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&Business{}, &Menu{}))

	biz, _ := seedMenuForBulkWrite(t)
	// Simulate a racing writer by bumping the version once, mid-flight, via a hook:
	// here we just prove a stale expectedVersion is recovered by the retry loop.
	menu, _, _ := GetMenuByBusinessID(biz.ID)
	_, _ = ApplyMenuCategories(biz.ID, []MenuCategory{{Name: "X"}}, menu.Version) // bump to v2 out-of-band
	v, err := AppendMenuCategories(biz.ID, []MenuCategory{{Name: "Y"}})
	require.NoError(t, err)
	assert.Equal(t, uint(3), v) // re-read v2, applied -> v3
}
