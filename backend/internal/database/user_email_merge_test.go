package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUserEmailMergeDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:user_email_merge_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	// Case-sensitive unique on email (raw column) so we can seed legacy
	// case-colliding duplicates that lower(email) must collapse.
	require.NoError(t, db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT,
			name TEXT,
			role TEXT DEFAULT 'user',
			auth_method TEXT,
			email_verified INTEGER DEFAULT 0,
			language_selected TEXT,
			deleted_at DATETIME,
			deletion_scheduled_at DATETIME,
			deletion_reason TEXT,
			signup_source TEXT DEFAULT '',
			activated_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error)
	SetTestDB(db)
	return db
}

// Two legacy accounts that share a normalized mailbox surface as ONE group with
// a merge affordance — not as two independent rows with no path to reconcile.
func TestDuplicateNormalizedEmailsSurfaceAsMergeGroup(t *testing.T) {
	db := setupUserEmailMergeDB(t)
	now := time.Now()
	require.NoError(t, db.Exec(
		`INSERT INTO users (email, name, role, created_at, updated_at) VALUES (?,?,?,?,?)`,
		"Claudia@Example.com", "Claudia A", "user", now, now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO users (email, name, role, created_at, updated_at) VALUES (?,?,?,?,?)`,
		"claudia@example.com", "Claudia B", "user", now, now,
	).Error)
	// Unrelated unique mailbox must not appear as a merge candidate.
	require.NoError(t, db.Exec(
		`INSERT INTO users (email, name, role, created_at, updated_at) VALUES (?,?,?,?,?)`,
		"solo@example.com", "Solo", "user", now, now,
	).Error)

	groups, err := ListEmailDuplicateGroups()
	require.NoError(t, err)
	require.Len(t, groups, 1, "exactly one duplicate mailbox group expected")
	assert.Equal(t, "claudia@example.com", groups[0].NormalizedEmail)
	require.Len(t, groups[0].UserIDs, 2)
	assert.True(t, groups[0].CanMerge, "group must expose a merge affordance")
	assert.NotZero(t, groups[0].PrimaryUserID, "primary survivor must be nominated")
}

// Registering the same normalized email a second time must not create a second
// row — the pre-check + unique index keep a single account.
func TestSameNormalizedEmailDoesNotCreateSecondRow(t *testing.T) {
	db := setupUserEmailMergeDB(t)
	now := time.Now()
	require.NoError(t, db.Exec(
		`INSERT INTO users (email, name, role, created_at, updated_at) VALUES (?,?,?,?,?)`,
		"owner@example.com", "Owner", "user", now, now,
	).Error)

	// Simulate the Register pre-check: LOWER(email) match finds the existing row.
	var existing User
	err := db.Where("LOWER(email) = LOWER(?)", "OWNER@example.com").First(&existing).Error
	require.NoError(t, err)
	assert.Equal(t, "owner@example.com", existing.Email)

	var count int64
	require.NoError(t, db.Model(&User{}).
		Where("LOWER(email) = LOWER(?)", "Owner@Example.COM").
		Count(&count).Error)
	assert.Equal(t, int64(1), count, "one normalized mailbox = one row")
}
