package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newShiftNoteTestDB(t *testing.T) (*DB, func()) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&Business{}, &ShiftNote{}))
	SetTestDB(g)
	return GetDBWrapper(), func() { _ = sqlDB.Close() }
}

// TestShiftNoteGenesisShape asserts the shift_notes table AND its composite
// index materialize on a genesis (force-baselined) DB from struct tags alone —
// the genesis-safety guard the prior slices established. On a fresh DB the SQL
// migration is force-baselined without running DDL, so GORM autoMigrate must
// reproduce the 000109 shape (columns + named index) byte-for-byte.
func TestShiftNoteGenesisShape(t *testing.T) {
	db, cleanup := newShiftNoteTestDB(t)
	defer cleanup()
	m := db.GetGorm().Migrator()

	require.True(t, m.HasTable(&ShiftNote{}), "shift_notes table must exist on genesis")
	for _, col := range []string{"id", "business_id", "shift_id", "for_date", "author_staff_id", "category", "content", "created_at"} {
		require.Truef(t, m.HasColumn(&ShiftNote{}, col), "shift_notes missing genesis column %s", col)
	}
	require.True(t, m.HasIndex(&ShiftNote{}, "idx_shift_notes_business_for_date"),
		"shift_notes composite (business_id, for_date) index must materialize on genesis")
}

func TestShiftNoteCreateAndListByDate(t *testing.T) {
	db, cleanup := newShiftNoteTestDB(t)
	defer cleanup()

	day := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	other := day.Add(24 * time.Hour)
	shiftID := uint(99)

	require.NoError(t, db.CreateShiftNote(&ShiftNote{
		BusinessID: 1, ForDate: day, AuthorStaffID: 7,
		Category: ShiftNoteCategorySales, Content: "Strong brunch", ShiftID: &shiftID,
	}))
	require.NoError(t, db.CreateShiftNote(&ShiftNote{
		BusinessID: 1, ForDate: day.Add(6 * time.Hour), AuthorStaffID: 8,
		Category: ShiftNoteCategoryMaintenance, Content: "Walk-in fan rattling",
	}))
	// Different day — must be excluded.
	require.NoError(t, db.CreateShiftNote(&ShiftNote{
		BusinessID: 1, ForDate: other, AuthorStaffID: 7,
		Category: ShiftNoteCategoryOther, Content: "Next day",
	}))
	// Different business — tenant isolation.
	require.NoError(t, db.CreateShiftNote(&ShiftNote{
		BusinessID: 2, ForDate: day, AuthorStaffID: 1,
		Category: ShiftNoteCategoryGuests, Content: "Other tenant",
	}))

	got, err := db.ListShiftNotes(1, day, other, 200)
	require.NoError(t, err)
	require.Len(t, got, 2)                                   // only biz 1, only this day
	require.Equal(t, "Walk-in fan rattling", got[0].Content) // newest first (for_date desc)
	require.Equal(t, ShiftNoteCategorySales, got[1].Category)
	require.NotNil(t, got[1].ShiftID)
	require.Equal(t, shiftID, *got[1].ShiftID)

	require.True(t, IsValidShiftNoteCategory("staffing"))
	require.False(t, IsValidShiftNoteCategory("payroll"))
}

func TestListShiftNotesIsBounded(t *testing.T) {
	db, cleanup := newShiftNoteTestDB(t)
	defer cleanup()
	day := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 250; i++ {
		require.NoError(t, db.CreateShiftNote(&ShiftNote{
			BusinessID: 1, ForDate: day.Add(time.Duration(i) * time.Minute),
			AuthorStaffID: 7, Category: ShiftNoteCategoryOther, Content: "n",
		}))
	}
	got, err := db.ListShiftNotes(1, day, day.Add(24*time.Hour), 200)
	require.NoError(t, err)
	require.Len(t, got, 200) // hard cap honored
}
