package database

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupItemWriteTestDB opens a NAMED shared-cache in-memory SQLite DB and points
// the package-level `db` at it. Unlike the unnamed file::memory: used elsewhere,
// cache=shared means every pooled connection (and NewDB sessions) sees the same
// tables — required by the concurrency callback below, which writes on a separate
// handle without deadlocking on a single-connection pool.
func setupItemWriteTestDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, e := gormDB.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	db = gormDB
	require.NoError(t, db.AutoMigrate(&Business{}, &Menu{}))
}

// seedMenuForItemWrite creates a business + active menu (version 1) with one
// category holding one item, for the legacy index-based item-write tests.
func seedMenuForItemWrite(t *testing.T) *Business {
	t.Helper()
	biz := &Business{Name: "ItemWriteTest Resto"}
	require.NoError(t, db.Create(biz).Error)

	cats := []MenuCategory{{
		ID:    "cat-1",
		Name:  "Mains",
		Items: []MenuItem{{ID: "i1", Name: "Burger", Price: 10.0, IsAvailable: true}},
	}}
	catsJSON, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, db.Create(&Menu{
		BusinessID: biz.ID,
		Categories: string(catsJSON),
		IsActive:   true,
		Version:    1,
	}).Error)
	return biz
}

func TestLegacyMenuCreates_AssignMissingCategoryAndItemIDs(t *testing.T) {
	setupItemWriteTestDB(t)

	biz := &Business{Name: "Identity Test Resto"}
	require.NoError(t, db.Create(biz).Error)
	_, _, _, err := AddMenuCategory(biz.ID, MenuCategory{Name: "Mains"})
	require.NoError(t, err)
	_, _, _, err = AddMenuItem(biz.ID, 0, MenuItem{Name: "Boundary Bowl", Price: 18.5, IsAvailable: true})
	require.NoError(t, err)

	_, categories, err := GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	require.Len(t, categories, 1)
	require.Len(t, categories[0].Items, 1)
	assert.NotEmpty(t, categories[0].ID)
	assert.NotEmpty(t, categories[0].Items[0].ID)
	assert.NoError(t, uuid.Validate(categories[0].ID))
	assert.NoError(t, uuid.Validate(categories[0].Items[0].ID))
}

// raceInjectConcurrentItem registers a one-shot Before("gorm:update") callback
// that, the first time a `menus` row is updated, simulates a concurrent writer
// that read v1 and persisted its OWN item under a CAS bump — committed on a fresh
// handle BEFORE the writer-under-test's apply lands. Under the old non-CAS
// UpdateMenu (blind db.Save) the writer-under-test clobbers this concurrent item
// (lost update); under mutateMenuUnderCAS the apply misses, re-reads, and merges,
// so both items survive. Returns a *bool reporting whether it fired.
func raceInjectConcurrentItem(t *testing.T, name string, bizID uint, concurrent MenuItem) *bool {
	t.Helper()
	fired := false
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(name, func(g *gorm.DB) {
		if fired {
			return
		}
		if g.Statement == nil || g.Statement.Table != "menus" {
			return
		}
		fired = true
		fresh := db.Session(&gorm.Session{NewDB: true})
		var menu Menu
		if err := fresh.Where("business_id = ? AND is_active = ?", bizID, true).First(&menu).Error; err != nil {
			return
		}
		var cats []MenuCategory
		_ = json.Unmarshal([]byte(menu.Categories), &cats)
		if len(cats) == 0 {
			return
		}
		cats[0].Items = append(cats[0].Items, concurrent)
		catsJSON, _ := json.Marshal(cats)
		fresh.Model(&Menu{}).
			Where("id = ? AND version = ?", menu.ID, menu.Version).
			Updates(map[string]interface{}{"categories": string(catsJSON), "version": menu.Version + 1})
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(name) })
	return &fired
}

func TestAddMenuItem_AtomicUnderConcurrency(t *testing.T) {
	setupItemWriteTestDB(t)

	biz := seedMenuForItemWrite(t)
	fired := raceInjectConcurrentItem(t, "race_add", biz.ID,
		MenuItem{ID: "concurrent", Name: "Concurrent", Price: 9.0, IsAvailable: true})

	_, _, _, err := AddMenuItem(biz.ID, 0, MenuItem{ID: "i2", Name: "Fries", Price: 4.0, IsAvailable: true})
	require.NoError(t, err, "AddMenuItem should recover from a concurrent write via retry")
	assert.True(t, *fired, "the racing concurrent write should have fired")

	_, cats, err := GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	require.Len(t, cats, 1)
	names := map[string]bool{}
	for _, it := range cats[0].Items {
		names[it.Name] = true
	}
	assert.True(t, names["Fries"], "our item must survive")
	assert.True(t, names["Concurrent"], "the concurrent writer's item must NOT be lost")
}

func TestUpdateMenuItem_AtomicUnderConcurrency(t *testing.T) {
	setupItemWriteTestDB(t)

	biz := seedMenuForItemWrite(t)
	fired := raceInjectConcurrentItem(t, "race_upd", biz.ID,
		MenuItem{ID: "concurrent", Name: "Concurrent", Price: 9.0, IsAvailable: true})

	_, _, err := UpdateMenuItem(biz.ID, 0, 0, MenuItem{ID: "i1", Name: "Cheeseburger", Price: 12.0, IsAvailable: true})
	require.NoError(t, err)
	assert.True(t, *fired)

	_, cats, err := GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	names := map[string]float64{}
	for _, it := range cats[0].Items {
		names[it.Name] = it.Price
	}
	assert.Equal(t, 12.0, names["Cheeseburger"], "our update must apply")
	_, hasConcurrent := names["Concurrent"]
	assert.True(t, hasConcurrent, "the concurrent writer's item must NOT be lost")
}

func TestDeleteMenuItem_AtomicUnderConcurrency(t *testing.T) {
	setupItemWriteTestDB(t)

	biz := seedMenuForItemWrite(t)
	fired := raceInjectConcurrentItem(t, "race_del", biz.ID,
		MenuItem{ID: "concurrent", Name: "Concurrent", Price: 9.0, IsAvailable: true})

	_, err := DeleteMenuItem(biz.ID, 0, 0) // delete "Burger"
	require.NoError(t, err)
	assert.True(t, *fired)

	_, cats, err := GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	require.Len(t, cats, 1)
	names := map[string]bool{}
	for _, it := range cats[0].Items {
		names[it.Name] = true
	}
	assert.False(t, names["Burger"], "the targeted item must be removed")
	assert.True(t, names["Concurrent"], "the concurrent writer's item must NOT be lost")
}

func TestDeleteMenuItem_OutOfRangePreservesSentinel(t *testing.T) {
	setupItemWriteTestDB(t)

	biz := seedMenuForItemWrite(t)
	_, err := DeleteMenuItem(biz.ID, 0, 99)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "item index out of range")
}

func TestGenerateUniqueTableCodeUsesCryptoRand(t *testing.T) {
	source, err := os.ReadFile("business.go")
	require.NoError(t, err)
	require.Contains(t, string(source), `"crypto/rand"`)
	require.NotContains(t, string(source), `"math/rand"`)
}

func TestRandomTableCodeShape(t *testing.T) {
	code, err := randomTableCode(10)
	require.NoError(t, err)
	require.Len(t, code, 10)
	for _, r := range code {
		require.Contains(t, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", string(r))
	}
}
