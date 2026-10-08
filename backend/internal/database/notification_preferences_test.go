package database

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupUserPrefTestDB stands up an in-memory SQLite DB and migrates only the
// User model, assigning it to the package-global db. It is intentionally
// uniquely named so it does not collide with the shared setupTestDB (which does
// NOT migrate User).
func setupUserPrefTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err, "open in-memory sqlite")
	db = gormDB
	require.NoError(t, db.AutoMigrate(&User{}), "migrate User")
}

// TestUpdateNotificationPreferences_PersistsFalse is the regression test for the
// "can't disable a notification" bug. The previous implementation routed through
// the struct-based UpdateUser, whose Updates(&struct) skips bool false (the Go
// zero value), so toggling any preference OFF was silently dropped and the user
// kept receiving it. The map-based fix writes every column explicitly.
//
// Against the OLD implementation this test FAILS (the false fields read back as
// true); against the map-based fix it PASSES.
func TestUpdateNotificationPreferences_PersistsFalse(t *testing.T) {
	setupUserPrefTestDB(t)

	const addr = "0xprefuser"

	// Seed a user with every preference enabled.
	seed := &User{
		Address:                 addr,
		Role:                    "user",
		NotificationPreferences: structs.NewDefaultNotificationPreferences(),
	}
	require.NoError(t, db.Create(seed).Error)

	// Sanity: confirm the seed really persisted all-true.
	var before User
	require.NoError(t, db.Where("address = ?", addr).First(&before).Error)
	require.True(t, before.NotificationPreferences.EmailEnabled)
	require.True(t, before.NotificationPreferences.NewsEnabled)
	require.True(t, before.NotificationPreferences.ReportsEnabled)

	// User disables some notifications, keeps others on.
	newPrefs := structs.NotificationPreferences{
		EmailEnabled:         false, // turned OFF
		NewsEnabled:          false, // turned OFF
		UpdatesEnabled:       true,  // stays ON
		TransactionalEnabled: true,  // stays ON
		SecurityEnabled:      false, // turned OFF
		ReportsEnabled:       true,  // stays ON
		StatisticsEnabled:    false, // turned OFF
	}

	require.NoError(t, UpdateNotificationPreferences(
		structs.User{Address: addr},
		newPrefs,
	))

	// Read the row straight back from the DB.
	var after User
	require.NoError(t, db.Where("address = ?", addr).First(&after).Error)
	got := after.NotificationPreferences

	// The OFF toggles must have persisted as false (this is what the old
	// struct-Updates path silently dropped).
	assert.False(t, got.EmailEnabled, "EmailEnabled should persist as false")
	assert.False(t, got.NewsEnabled, "NewsEnabled should persist as false")
	assert.False(t, got.SecurityEnabled, "SecurityEnabled should persist as false")
	assert.False(t, got.StatisticsEnabled, "StatisticsEnabled should persist as false")

	// The ON toggles must remain true.
	assert.True(t, got.UpdatesEnabled, "UpdatesEnabled should stay true")
	assert.True(t, got.TransactionalEnabled, "TransactionalEnabled should stay true")
	assert.True(t, got.ReportsEnabled, "ReportsEnabled should stay true")
}

// TestUpdateNotificationPreferences_DisableAll covers the most important guest/
// operator action: turning EVERYTHING off. With the old path this was a no-op
// (all-false == zero value == every column skipped).
func TestUpdateNotificationPreferences_DisableAll(t *testing.T) {
	setupUserPrefTestDB(t)

	const addr = "0xdisableall"
	require.NoError(t, db.Create(&User{
		Address:                 addr,
		Role:                    "user",
		NotificationPreferences: structs.NewDefaultNotificationPreferences(),
	}).Error)

	require.NoError(t, UpdateNotificationPreferences(
		structs.User{Address: addr},
		structs.NotificationPreferences{}, // all false
	))

	var after User
	require.NoError(t, db.Where("address = ?", addr).First(&after).Error)
	got := after.NotificationPreferences

	assert.False(t, got.EmailEnabled)
	assert.False(t, got.NewsEnabled)
	assert.False(t, got.UpdatesEnabled)
	assert.False(t, got.TransactionalEnabled)
	assert.False(t, got.SecurityEnabled)
	assert.False(t, got.ReportsEnabled)
	assert.False(t, got.StatisticsEnabled)
}

// TestUpdateNotificationPreferences_DoesNotFanOutAcrossAddresslessUsers is the
// regression test for the multi-tenant safety bug: modern OAuth/email users have
// NO wallet address (Address is empty, and the column is a non-unique index), so
// keying the UPDATE on Where("address = ?", "") matched EVERY address-less user
// and rewrote all their preferences in one call. The fix keys on the unique email
// when present. This test seeds two address-less users and asserts that updating
// one leaves the other completely untouched.
func TestUpdateNotificationPreferences_DoesNotFanOutAcrossAddresslessUsers(t *testing.T) {
	setupUserPrefTestDB(t)

	// Two distinct OAuth users, both with an empty Address (the realistic shape).
	require.NoError(t, db.Create(&User{
		Email:                   "alice@example.com",
		Role:                    "user",
		NotificationPreferences: structs.NewDefaultNotificationPreferences(),
	}).Error)
	require.NoError(t, db.Create(&User{
		Email:                   "bob@example.com",
		Role:                    "user",
		NotificationPreferences: structs.NewDefaultNotificationPreferences(),
	}).Error)

	// Alice turns everything off.
	require.NoError(t, UpdateNotificationPreferences(
		structs.User{Email: "alice@example.com"},
		structs.NotificationPreferences{}, // all false
	))

	var alice User
	require.NoError(t, db.Where("email = ?", "alice@example.com").First(&alice).Error)
	assert.False(t, alice.NotificationPreferences.EmailEnabled, "alice's prefs should be disabled")
	assert.False(t, alice.NotificationPreferences.ReportsEnabled, "alice's prefs should be disabled")

	// Bob — also address-less — must be entirely unaffected. Against the old
	// address-keyed code his row would have been clobbered to all-false too.
	var bob User
	require.NoError(t, db.Where("email = ?", "bob@example.com").First(&bob).Error)
	assert.True(t, bob.NotificationPreferences.EmailEnabled, "bob's prefs must be untouched")
	assert.True(t, bob.NotificationPreferences.NewsEnabled, "bob's prefs must be untouched")
	assert.True(t, bob.NotificationPreferences.SecurityEnabled, "bob's prefs must be untouched")
	assert.True(t, bob.NotificationPreferences.StatisticsEnabled, "bob's prefs must be untouched")
}

// TestUpdateNotificationPreferences_RejectsIdentitylessUser asserts the function
// refuses to run an UPDATE with no identifying predicate (neither email nor
// address) rather than matching every address-less row.
func TestUpdateNotificationPreferences_RejectsIdentitylessUser(t *testing.T) {
	setupUserPrefTestDB(t)

	require.NoError(t, db.Create(&User{
		Email:                   "carol@example.com",
		Role:                    "user",
		NotificationPreferences: structs.NewDefaultNotificationPreferences(),
	}).Error)

	err := UpdateNotificationPreferences(
		structs.User{}, // no email, no address
		structs.NotificationPreferences{},
	)
	require.Error(t, err, "an identity-less update must be rejected")

	// Carol's all-true prefs must remain intact (no fan-out write occurred).
	var carol User
	require.NoError(t, db.Where("email = ?", "carol@example.com").First(&carol).Error)
	assert.True(t, carol.NotificationPreferences.EmailEnabled)
	assert.True(t, carol.NotificationPreferences.ReportsEnabled)
}
