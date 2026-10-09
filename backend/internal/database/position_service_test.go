package database

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newPositionTestDB opens an isolated in-memory SQLite DB, migrates the tables
// used by the position service, registers it as the package DB, and returns the
// wrapper + raw gorm handle.
func newPositionTestDB(t *testing.T) (*DB, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}))
	require.NoError(t, gormDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_staff_one_primary ON staff_positions (business_id, staff_id) WHERE is_primary").Error)
	prev := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })
	return GetDBWrapper(), gormDB
}

func TestPositionModels_Migrate(t *testing.T) {
	_, gormDB := newPositionTestDB(t)
	require.True(t, gormDB.Migrator().HasTable(&Position{}))
	require.True(t, gormDB.Migrator().HasTable(&StaffPosition{}))
	require.True(t, gormDB.Migrator().HasColumn(&StaffPosition{}, "pay_rate_cents"))
}

func TestPositionCRUD(t *testing.T) {
	db, _ := newPositionTestDB(t)

	// Create
	p := &Position{BusinessID: 1, Name: "Server", ColorHex: "#1a6b6a", Department: "FOH", SortOrder: 1}
	require.NoError(t, db.CreatePosition(p))
	require.NotZero(t, p.ID)

	// List returns only this business's active positions
	other := &Position{BusinessID: 2, Name: "Cook"}
	require.NoError(t, db.CreatePosition(other))
	list, err := db.ListPositions(1)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "Server", list[0].Name)

	// Update (tenant-guarded: wrong business is a no-op error)
	require.Error(t, db.UpdatePosition(99, p.ID, map[string]interface{}{"name": "Waiter"}))
	require.NoError(t, db.UpdatePosition(1, p.ID, map[string]interface{}{"name": "Waiter"}))
	reloaded, err := db.GetPosition(1, p.ID)
	require.NoError(t, err)
	require.Equal(t, "Waiter", reloaded.Name)

	// Soft-retire (DeletePosition sets is_active=false; stays out of List)
	require.NoError(t, db.DeletePosition(1, p.ID))
	list, err = db.ListPositions(1)
	require.NoError(t, err)
	require.Len(t, list, 0)
}

func TestStaffPositionAssignmentAndRate(t *testing.T) {
	db, gormDB := newPositionTestDB(t)
	require.NoError(t, gormDB.Create(&Staff{ID: 10, BusinessID: 1, Email: "a@x.io", Name: "A", Role: StaffRoleServer, InvitedBy: "owner"}).Error)
	server := &Position{BusinessID: 1, Name: "Server"}
	bar := &Position{BusinessID: 1, Name: "Bartender"}
	require.NoError(t, db.CreatePosition(server))
	require.NoError(t, db.CreatePosition(bar))

	// Assign two positions; second is primary
	require.NoError(t, db.AssignPosition(1, 10, server.ID, false))
	require.NoError(t, db.AssignPosition(1, 10, bar.ID, true))

	links, err := db.ListStaffPositions(1, 10)
	require.NoError(t, err)
	require.Len(t, links, 2)

	// Exactly one primary, and it's the bartender
	var primaries int
	for _, l := range links {
		if l.IsPrimary {
			primaries++
			require.Equal(t, bar.ID, l.PositionID)
		}
	}
	require.Equal(t, 1, primaries)

	// Re-assigning is idempotent (unique index on staff_id+position_id)
	require.NoError(t, db.AssignPosition(1, 10, server.ID, true))
	links, _ = db.ListStaffPositions(1, 10)
	require.Len(t, links, 2) // still 2, not 3

	// Setting primary on server flipped bar's primary off
	primaries = 0
	for _, l := range links {
		if l.IsPrimary {
			primaries++
			require.Equal(t, server.ID, l.PositionID)
		}
	}
	require.Equal(t, 1, primaries)

	// Pay rate (owner path): stored in cents, read back in cents
	require.NoError(t, db.SetPayRateCents(1, 10, server.ID, 2550))
	cents, err := db.GetPayRateCents(1, 10, server.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2550), cents)

	// Pay rate for an unassigned position is an error, not a silent 0
	_, err = db.GetPayRateCents(1, 10, 99999)
	require.Error(t, err)

	// Unassign removes the link
	require.NoError(t, db.UnassignPosition(1, 10, bar.ID, true))
	links, _ = db.ListStaffPositions(1, 10)
	require.Len(t, links, 1)
}

// TestAssignTenantGuardNegatives verifies the assignment guards reject staff or
// positions that don't belong to the caller's business, with typed sentinels.
func TestAssignTenantGuardNegatives(t *testing.T) {
	db, gormDB := newPositionTestDB(t)
	// Staff 10 lives in business 1; staff 20 lives in business 2.
	require.NoError(t, gormDB.Create(&Staff{ID: 10, BusinessID: 1, Email: "a@x.io", Name: "A", Role: StaffRoleServer, InvitedBy: "o"}).Error)
	require.NoError(t, gormDB.Create(&Staff{ID: 20, BusinessID: 2, Email: "b@x.io", Name: "B", Role: StaffRoleServer, InvitedBy: "o"}).Error)
	pos := &Position{BusinessID: 1, Name: "Server"}
	require.NoError(t, db.CreatePosition(pos))

	// Staff not in business 1 → ErrStaffNotFound (staff 20 belongs to biz 2).
	err := db.AssignPosition(1, 20, pos.ID, false)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrStaffNotFound))

	// Missing/cross-business position → ErrPositionNotFound.
	err = db.AssignPosition(1, 10, 99999, false)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrPositionNotFound))
}

// TestAssignToRetiredPositionRejected (I2) verifies a soft-retired position is
// not a valid assignment target.
func TestAssignToRetiredPositionRejected(t *testing.T) {
	db, gormDB := newPositionTestDB(t)
	require.NoError(t, gormDB.Create(&Staff{ID: 10, BusinessID: 1, Email: "a@x.io", Name: "A", Role: StaffRoleServer, InvitedBy: "o"}).Error)
	pos := &Position{BusinessID: 1, Name: "Server"}
	require.NoError(t, db.CreatePosition(pos))

	// Soft-retire the position.
	require.NoError(t, db.DeletePosition(1, pos.ID))

	// Assigning to a retired position is rejected as if it doesn't exist.
	err := db.AssignPosition(1, 10, pos.ID, true)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrPositionNotFound))
}

// TestSinglePrimaryIndexEnforced proves the idx_staff_one_primary partial unique
// index actually constrains the data — independently of AssignPosition, which
// clears existing primaries in-transaction so the index never fires. We RAW-insert
// StaffPosition rows directly (bypassing AssignPosition) so the clearing never
// happens: a SECOND primary for the same business+staff (different position) must
// be rejected by the partial index, while a NON-primary second link is allowed
// (the index only constrains rows WHERE is_primary). This is the assertion that
// fails if the partial index is dropped or its predicate is typo'd anywhere.
func TestSinglePrimaryIndexEnforced(t *testing.T) {
	_, gormDB := newPositionTestDB(t)
	const (
		biz = uint(1)
		stf = uint(10)
		p1  = uint(100)
		p2  = uint(200)
	)

	// First primary link — allowed.
	require.NoError(t, gormDB.Create(&StaffPosition{
		BusinessID: biz, StaffID: stf, PositionID: p1, IsPrimary: true,
	}).Error)

	// Second primary for the SAME business+staff (different position) — the
	// partial unique index must reject it (unique violation).
	err := gormDB.Create(&StaffPosition{
		BusinessID: biz, StaffID: stf, PositionID: p2, IsPrimary: true,
	}).Error
	require.Error(t, err, "a second is_primary row for the same staff must violate idx_staff_one_primary")

	// A NON-primary second link is allowed — the partial index only constrains
	// rows WHERE is_primary.
	require.NoError(t, gormDB.Create(&StaffPosition{
		BusinessID: biz, StaffID: stf, PositionID: p2, IsPrimary: false,
	}).Error)
}

// TestSentinelIdentity asserts the typed sentinels (not just generic errors)
// surface for cross-business misses and missing links.
func TestSentinelIdentity(t *testing.T) {
	db, _ := newPositionTestDB(t)
	pos := &Position{BusinessID: 1, Name: "Server"}
	require.NoError(t, db.CreatePosition(pos))

	// Cross-business GetPosition miss → ErrPositionNotFound.
	_, err := db.GetPosition(99, pos.ID)
	require.True(t, errors.Is(err, ErrPositionNotFound))

	// Cross-business UpdatePosition miss → ErrPositionNotFound.
	err = db.UpdatePosition(99, pos.ID, map[string]interface{}{"name": "X"})
	require.True(t, errors.Is(err, ErrPositionNotFound))

	// Unassign of a non-existent link → ErrStaffPositionNotFound.
	err = db.UnassignPosition(1, 12345, pos.ID, true)
	require.True(t, errors.Is(err, ErrStaffPositionNotFound))
}

// TestUnassignPositionPayRateGuard: a schedule:write caller (canClearPayRate
// false) deletes an unrated link and is refused on a rated one; payroll:write
// (true) deletes the rated link.
func TestUnassignPositionPayRateGuard(t *testing.T) {
	db, gormDB := newPositionTestDB(t)
	require.NoError(t, gormDB.Create(&Staff{ID: 10, BusinessID: 1, Email: "a@x.io", Name: "A", Role: StaffRoleServer, InvitedBy: "o"}).Error)
	rated := &Position{BusinessID: 1, Name: "Server"}
	free := &Position{BusinessID: 1, Name: "Host"}
	ownerClear := &Position{BusinessID: 1, Name: "Bartender"}
	require.NoError(t, db.CreatePosition(rated))
	require.NoError(t, db.CreatePosition(free))
	require.NoError(t, db.CreatePosition(ownerClear))
	require.NoError(t, db.AssignPosition(1, 10, rated.ID, true))
	require.NoError(t, db.AssignPosition(1, 10, free.ID, false))
	require.NoError(t, db.AssignPosition(1, 10, ownerClear.ID, false))
	require.NoError(t, db.SetPayRateCents(1, 10, rated.ID, 2550))
	require.NoError(t, db.SetPayRateCents(1, 10, ownerClear.ID, 900))

	err := db.UnassignPosition(1, 10, rated.ID, false)
	require.ErrorIs(t, err, ErrStaffPositionRated)
	cents, err := db.GetPayRateCents(1, 10, rated.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2550), cents)

	require.NoError(t, db.UnassignPosition(1, 10, free.ID, false))
	_, err = db.GetPayRateCents(1, 10, free.ID)
	require.ErrorIs(t, err, ErrStaffPositionNotFound)

	require.NoError(t, db.UnassignPosition(1, 10, ownerClear.ID, true))
	_, err = db.GetPayRateCents(1, 10, ownerClear.ID)
	require.ErrorIs(t, err, ErrStaffPositionNotFound)
}
