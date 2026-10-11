package services

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
)

func setupAccountErasureTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&database.User{}, &database.Business{}, &session.UserSession{}))
	require.NoError(t, db.Exec(`CREATE TABLE user_auths (
		id integer primary key autoincrement,
		user_id integer not null,
		provider text not null,
		password_hash text,
		provider_user_id text,
		wallet_address text,
		email_verified boolean default false,
		verification_token text,
		verification_expiry datetime,
		reset_token text,
		reset_expiry datetime,
		created_at datetime,
		updated_at datetime
	)`).Error)
	return db
}

type erasureSeed struct {
	user     database.User
	business database.Business
	session  session.UserSession
}

func seedErasureAccount(t *testing.T, db *gorm.DB, email, address string, scheduled time.Time) erasureSeed {
	t.Helper()
	deletedAt := scheduled.Add(-30 * 24 * time.Hour)
	user := database.User{
		Email: email, Address: address, Name: "Ana Owner", Username: "ana", Picture: "https://example.com/a.png",
		GoogleID: "google-" + email, EmailVerified: true,
		DeletedAt: &deletedAt, DeletionScheduledAt: &scheduled, DeletionReason: "moving to Lisbon, call +351 555",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Exec(`INSERT INTO user_auths (user_id, provider, password_hash, provider_user_id, wallet_address, email_verified, reset_token)
		VALUES (?, 'email', 'bcrypt-hash', ?, ?, true, 'reset-secret')`, user.ID, email, address).Error)
	userID := user.ID
	business := database.Business{BusinessId: "biz-" + email, Name: "Ana's Bistro", UserID: &userID, OwnerAddress: address, Email: "bistro@example.com"}
	require.NoError(t, db.Create(&business).Error)
	sess := session.UserSession{
		UserID: &userID, Address: address, SessionToken: "tok-" + email, Provider: "email",
		ExpiresAt: scheduled.Add(24 * time.Hour), RefreshExpiresAt: scheduled.Add(48 * time.Hour),
	}
	require.NoError(t, db.Create(&sess).Error)
	return erasureSeed{user: user, business: business, session: sess}
}

func TestRunAccountErasureAnonymizesPastDueAccountsAndRevokesSessions(t *testing.T) {
	db := setupAccountErasureTestDB(t)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	due := seedErasureAccount(t, db, "due@example.com", "0xAbCdEf0000000000000000000000000000000001", now.Add(-time.Hour))
	notYet := seedErasureAccount(t, db, "later@example.com", "0xAbCdEf0000000000000000000000000000000002", now.Add(24*time.Hour))
	// A web3 sibling session keyed by address only (user_id NULL) must go too.
	require.NoError(t, db.Create(&session.UserSession{
		Address: "0xabcdef0000000000000000000000000000000001", SessionToken: "web3-due", Provider: "email",
		ExpiresAt: now.Add(time.Hour), RefreshExpiresAt: now.Add(time.Hour),
	}).Error)

	res, err := RunAccountErasure(context.Background(), db, session.NewStore(db), now, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, res.UsersAnonymized)

	var row struct {
		Email, Address, Name, Username, Picture, GoogleID sql.NullString
		DeletionReason                                    string
		EmailVerified                                     bool
		DeletedAt                                         sql.NullTime
		DeletionScheduledAt                               sql.NullTime
	}
	require.NoError(t, db.Raw(`SELECT email, address, name, username, picture, google_id,
		deletion_reason, email_verified, deleted_at, deletion_scheduled_at FROM users WHERE id = ?`, due.user.ID).Scan(&row).Error)
	for name, v := range map[string]sql.NullString{
		"email": row.Email, "address": row.Address, "name": row.Name, "username": row.Username,
		"picture": row.Picture, "google_id": row.GoogleID,
	} {
		assert.False(t, v.Valid && v.String != "", "users.%s must be erased, got %q", name, v.String)
	}
	assert.Empty(t, row.DeletionReason, "free-text deletion reason can carry PII")
	assert.False(t, row.EmailVerified)
	assert.True(t, row.DeletedAt.Valid, "erased account stays soft-deleted")
	assert.True(t, row.DeletionScheduledAt.Valid, "the scheduled date is kept as the audit trail")

	var auth struct {
		PasswordHash, ProviderUserID, WalletAddress, ResetToken sql.NullString
	}
	require.NoError(t, db.Raw(`SELECT password_hash, provider_user_id, wallet_address, reset_token FROM user_auths WHERE user_id = ?`, due.user.ID).Scan(&auth).Error)
	assert.False(t, auth.PasswordHash.Valid)
	assert.False(t, auth.ProviderUserID.Valid)
	assert.False(t, auth.WalletAddress.Valid)
	assert.False(t, auth.ResetToken.Valid)

	var liveDueSessions int64
	require.NoError(t, db.Model(&session.UserSession{}).
		Where("(user_id = ? OR LOWER(address) = ?) AND revoked = false", due.user.ID, "0xabcdef0000000000000000000000000000000001").
		Count(&liveDueSessions).Error)
	assert.Zero(t, liveDueSessions, "every session for the erased account must be revoked")

	// Owned businesses are left alone for accounting retention.
	var business database.Business
	require.NoError(t, db.First(&business, due.business.ID).Error)
	assert.Equal(t, "Ana's Bistro", business.Name)
	require.NotNil(t, business.UserID)
	assert.Equal(t, due.user.ID, *business.UserID)

	// An account still inside its grace window is untouched.
	var later database.User
	require.NoError(t, db.First(&later, notYet.user.ID).Error)
	assert.Equal(t, "later@example.com", later.Email)
	var laterSession session.UserSession
	require.NoError(t, db.First(&laterSession, notYet.session.ID).Error)
	assert.False(t, laterSession.Revoked)

	// A second pass finds nothing left to erase.
	again, err := RunAccountErasure(context.Background(), db, session.NewStore(db), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.Zero(t, again.UsersAnonymized)
}

func TestRunAccountErasureDrainsAcrossBatches(t *testing.T) {
	db := setupAccountErasureTestDB(t)
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	for _, email := range []string{"a@example.com", "b@example.com", "c@example.com"} {
		seedErasureAccount(t, db, email, "", now.Add(-time.Minute))
	}
	res, err := RunAccountErasure(context.Background(), db, nil, now, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, res.UsersAnonymized)
	var remaining int64
	require.NoError(t, db.Model(&database.User{}).Where("email IS NOT NULL").Count(&remaining).Error)
	assert.Zero(t, remaining)
}

func TestStartAccountErasureJanitorStopsOnCancel(t *testing.T) {
	db := setupAccountErasureTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := StartAccountErasureJanitor(ctx, db, nil, time.Hour)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("account erasure janitor did not stop after cancel")
	}
}

// TestRunAccountErasureKeepsAccountWhenRevocationFails pins the ordering:
// sessions are revoked before the identifiers are cleared. If revocation
// fails, the account must stay due (address intact) so the next pass can
// still find and revoke its wallet-linked sessions.
func TestRunAccountErasureKeepsAccountWhenRevocationFails(t *testing.T) {
	db := setupAccountErasureTestDB(t)
	now := time.Now().UTC()
	seed := seedErasureAccount(t, db, "fail@example.com", "0xFAILREVOKE", now.Add(-time.Hour))

	// A session store over a database without user_sessions: every revoke errors.
	brokenDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	brokenSQL, err := brokenDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = brokenSQL.Close() })

	res, err := RunAccountErasure(context.Background(), db, session.NewStore(brokenDB), now, 10)
	require.Error(t, err)
	assert.Zero(t, res.UsersAnonymized)

	var user database.User
	require.NoError(t, db.Unscoped().First(&user, seed.user.ID).Error)
	assert.Equal(t, "0xFAILREVOKE", user.Address, "identifiers must survive a failed revocation so the next pass retries")

	res, err = RunAccountErasure(context.Background(), db, session.NewStore(db), now, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, res.UsersAnonymized)
	var sess session.UserSession
	require.NoError(t, db.First(&sess, seed.session.ID).Error)
	assert.True(t, sess.Revoked)
}
