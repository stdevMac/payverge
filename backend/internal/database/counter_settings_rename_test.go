package database

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCounterSettingsDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())),
		&gorm.Config{},
	)
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Counter{}))
	prior := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prior) })
	return gormDB
}

func seedCounterBusiness(t *testing.T, gormDB *gorm.DB, prefix string, count int) Business {
	t.Helper()
	business := Business{
		Name:           "Counter Co",
		CounterEnabled: true,
		CounterCount:   count,
		CounterPrefix:  prefix,
	}
	require.NoError(t, gormDB.Create(&business).Error)
	require.NoError(t, UpdateBusinessCounters(business.ID, true, count, prefix))
	return business
}

func counterNames(t *testing.T, gormDB *gorm.DB, businessID uint) map[int]Counter {
	t.Helper()
	var rows []Counter
	require.NoError(t, gormDB.Where("business_id = ?", businessID).
		Order("counter_number").Find(&rows).Error)
	out := make(map[int]Counter, len(rows))
	for _, row := range rows {
		out[row.CounterNumber] = row
	}
	return out
}

// L2-35 regression: saving count/enable alone must never destroy custom names.
// This path shipped without coverage, which is how R2-12 slipped through.
func TestUpdateBusinessCounters_PreservesCustomNames(t *testing.T) {
	gormDB := setupCounterSettingsDB(t)
	business := seedCounterBusiness(t, gormDB, "C", 3)

	require.NoError(t, gormDB.Model(&Counter{}).
		Where("business_id = ? AND counter_number = ?", business.ID, 2).
		Update("name", "Barra principal").Error)

	// Same prefix, count raised — nothing about the naming changed.
	require.NoError(t, UpdateBusinessCounters(business.ID, true, 4, "C"))

	rows := counterNames(t, gormDB, business.ID)
	require.Equal(t, "Barra principal", rows[2].Name)
	require.Equal(t, "C1", rows[1].Name)
	require.Equal(t, "C4", rows[4].Name)
}

func TestUpdateBusinessCounters_PrefixChangeRewritesNames(t *testing.T) {
	gormDB := setupCounterSettingsDB(t)
	business := seedCounterBusiness(t, gormDB, "C", 3)

	require.NoError(t, UpdateBusinessCounters(business.ID, true, 3, "M"))

	rows := counterNames(t, gormDB, business.ID)
	require.Equal(t, "M1", rows[1].Name)
	require.Equal(t, "M2", rows[2].Name)
	require.Equal(t, "M3", rows[3].Name)
}

// L2-35: prefix rename must not destroy operator-typed counter names.
func TestUpdateBusinessCounters_PrefixChangePreservesCustomNames(t *testing.T) {
	gormDB := setupCounterSettingsDB(t)
	business := seedCounterBusiness(t, gormDB, "C", 3)

	require.NoError(t, gormDB.Model(&Counter{}).
		Where("business_id = ? AND counter_number = ?", business.ID, 2).
		Update("name", "Barra principal").Error)

	require.NoError(t, UpdateBusinessCounters(business.ID, true, 3, "M"))

	rows := counterNames(t, gormDB, business.ID)
	require.Equal(t, "M1", rows[1].Name)
	require.Equal(t, "Barra principal", rows[2].Name)
	require.Equal(t, "M3", rows[3].Name)
}

// #393: shrinking 2→3→2 must soft-disable the extra row, not delete it, and
// raising the count again must reactivate the same record (no duplicate).
func TestUpdateBusinessCounters_ShrinkSoftDisablesWithoutDelete(t *testing.T) {
	gormDB := setupCounterSettingsDB(t)
	business := seedCounterBusiness(t, gormDB, "D", 2)

	require.NoError(t, UpdateBusinessCounters(business.ID, true, 3, "D"))
	grown := counterNames(t, gormDB, business.ID)
	require.Len(t, grown, 3)
	require.True(t, grown[3].IsActive)
	thirdID := grown[3].ID

	require.NoError(t, UpdateBusinessCounters(business.ID, true, 2, "D"))
	shrunk := counterNames(t, gormDB, business.ID)
	require.Len(t, shrunk, 3, "excess counters stay as historical rows")
	require.True(t, shrunk[1].IsActive)
	require.True(t, shrunk[2].IsActive)
	require.False(t, shrunk[3].IsActive)
	require.Equal(t, thirdID, shrunk[3].ID)

	var live []Counter
	require.NoError(t, gormDB.Where("business_id = ? AND is_active = ?", business.ID, true).
		Order("counter_number").Find(&live).Error)
	require.Len(t, live, 2)
	require.Equal(t, []int{1, 2}, []int{live[0].CounterNumber, live[1].CounterNumber})

	require.NoError(t, UpdateBusinessCounters(business.ID, true, 3, "D"))
	restored := counterNames(t, gormDB, business.ID)
	require.Len(t, restored, 3, "reactivation must not insert a duplicate row")
	require.Equal(t, thirdID, restored[3].ID)
	require.True(t, restored[3].IsActive)
}

// R2-12: counters deactivated under an OLD prefix kept that prefix forever.
// Shrinking under prefix "C" leaves C4/C5 inactive; renaming to "M" only
// touches the live rows; raising the count later has prefixChanged=false, so
// the reactivated rows came back as "C4"/"C5" beside "M1".."M3".
func TestUpdateBusinessCounters_ReactivationClearsStalePrefix(t *testing.T) {
	gormDB := setupCounterSettingsDB(t)
	business := seedCounterBusiness(t, gormDB, "C", 5)

	// Shrink to 3 — counters 4 and 5 go inactive still named C4/C5.
	require.NoError(t, UpdateBusinessCounters(business.ID, true, 3, "C"))
	// Rename the live ones to M1..M3.
	require.NoError(t, UpdateBusinessCounters(business.ID, true, 3, "M"))
	stale := counterNames(t, gormDB, business.ID)
	require.Equal(t, "C4", stale[4].Name, "precondition: inactive rows keep the old prefix")
	require.False(t, stale[4].IsActive)

	// Raise the count back with the prefix unchanged.
	require.NoError(t, UpdateBusinessCounters(business.ID, true, 5, "M"))

	rows := counterNames(t, gormDB, business.ID)
	require.Equal(t, "M4", rows[4].Name)
	require.Equal(t, "M5", rows[5].Name)
	require.True(t, rows[4].IsActive)
	require.True(t, rows[5].IsActive)
}

// The stale-prefix repair must not become a licence to clobber operator names.
func TestUpdateBusinessCounters_ReactivationKeepsCustomName(t *testing.T) {
	gormDB := setupCounterSettingsDB(t)
	business := seedCounterBusiness(t, gormDB, "C", 4)

	require.NoError(t, gormDB.Model(&Counter{}).
		Where("business_id = ? AND counter_number = ?", business.ID, 4).
		Update("name", "Ventana de retiro").Error)
	require.NoError(t, UpdateBusinessCounters(business.ID, true, 3, "C"))
	require.NoError(t, UpdateBusinessCounters(business.ID, true, 4, "C"))

	rows := counterNames(t, gormDB, business.ID)
	require.Equal(t, "Ventana de retiro", rows[4].Name)
	require.True(t, rows[4].IsActive)
}

// A name that is already correct for the current prefix must not be rewritten,
// and an ACTIVE row is never touched by the repair (only reactivation is).
func TestUpdateBusinessCounters_ActiveRowsAreNotRenamed(t *testing.T) {
	gormDB := setupCounterSettingsDB(t)
	business := seedCounterBusiness(t, gormDB, "C", 3)

	// An active row that happens to look auto-generated under another prefix.
	require.NoError(t, gormDB.Model(&Counter{}).
		Where("business_id = ? AND counter_number = ?", business.ID, 2).
		Update("name", "X2").Error)
	require.NoError(t, UpdateBusinessCounters(business.ID, true, 3, "C"))

	rows := counterNames(t, gormDB, business.ID)
	require.Equal(t, "X2", rows[2].Name)
}

func TestIsGeneratedCounterName(t *testing.T) {
	require.True(t, isGeneratedCounterName("C4", 4))
	require.True(t, isGeneratedCounterName("MOSTR12", 12))
	require.False(t, isGeneratedCounterName("Ventana de retiro", 4))
	require.False(t, isGeneratedCounterName("Barra 4", 4), "spaces mean an operator typed it")
	require.False(t, isGeneratedCounterName("4", 4), "a bare number has no prefix")
	require.False(t, isGeneratedCounterName("C5", 4), "wrong number")
	require.False(t, isGeneratedCounterName("DEMASIADO4", 4), "prefix longer than the 5-char limit")
	require.False(t, isGeneratedCounterName("C11", 1), "digit-adjacent prefix is ambiguous")
}

func TestIsSeedDefaultCounterName(t *testing.T) {
	require.True(t, isSeedDefaultCounterName("Counter 1", 1))
	require.True(t, isSeedDefaultCounterName("counter 2", 2))
	require.True(t, isSeedDefaultCounterName("Mostrador 3", 3))
	require.True(t, isSeedDefaultCounterName("Pickup 1", 1))
	require.False(t, isSeedDefaultCounterName("Barra principal", 1))
	require.False(t, isSeedDefaultCounterName("Counter 2", 1), "wrong number")
	require.False(t, isSeedDefaultCounterName("D1", 1), "compact generated form is not a seed word")
}

// #192: demo leftover "Counter N" under prefix "D" must rewrite on save even
// when the prefix field itself did not change.
func TestUpdateBusinessCounters_SeedDefaultNamesSyncToCurrentPrefix(t *testing.T) {
	gormDB := setupCounterSettingsDB(t)
	business := seedCounterBusiness(t, gormDB, "D", 2)

	require.NoError(t, gormDB.Model(&Counter{}).
		Where("business_id = ? AND counter_number = ?", business.ID, 1).
		Update("name", "Counter 1").Error)
	require.NoError(t, gormDB.Model(&Counter{}).
		Where("business_id = ? AND counter_number = ?", business.ID, 2).
		Update("name", "Counter 2").Error)

	// Same prefix "D" — previously a no-op for these seed labels.
	require.NoError(t, UpdateBusinessCounters(business.ID, true, 2, "D"))

	rows := counterNames(t, gormDB, business.ID)
	require.Equal(t, "D1", rows[1].Name)
	require.Equal(t, "D2", rows[2].Name)
}

func TestUpdateBusinessCounters_SeedDefaultNamesDoNotClobberCustom(t *testing.T) {
	gormDB := setupCounterSettingsDB(t)
	business := seedCounterBusiness(t, gormDB, "D", 2)

	require.NoError(t, gormDB.Model(&Counter{}).
		Where("business_id = ? AND counter_number = ?", business.ID, 1).
		Update("name", "Counter 1").Error)
	require.NoError(t, gormDB.Model(&Counter{}).
		Where("business_id = ? AND counter_number = ?", business.ID, 2).
		Update("name", "Barra principal").Error)

	require.NoError(t, UpdateBusinessCounters(business.ID, true, 2, "D"))

	rows := counterNames(t, gormDB, business.ID)
	require.Equal(t, "D1", rows[1].Name)
	require.Equal(t, "Barra principal", rows[2].Name)
}
