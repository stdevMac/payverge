//go:build integration_postgres

package auth

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/testperf"

	"github.com/stretchr/testify/require"
)

// TestPostgresConcurrentBootstrapAdminCreatesOneUser proves the advisory-lock
// path: several replicas booting with the same ADMIN_EMAIL at once produce
// exactly one admin user and one email credential, and nobody errors.
func TestPostgresConcurrentBootstrapAdminCreatesOneUser(t *testing.T) {
	ctx := context.Background()
	pg, err := testperf.StartIsolatedPostgres(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	// Test-only schema for the two tables this path touches.
	require.NoError(t, pg.DB.AutoMigrate(&database.User{}, &UserAuth{}))
	sqlDB, err := pg.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(16)

	const workers = 12
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan BootstrapAdminResult, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := EnsureBootstrapAdmin(ctx, pg.DB, "replica@example.com", strongAdminPassword)
			if err != nil {
				errs <- err
				return
			}
			results <- res
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		require.NoError(t, err)
	}
	created := 0
	for res := range results {
		if res.Created {
			created++
		}
	}
	require.Equal(t, 1, created)

	var users, creds int64
	require.NoError(t, pg.DB.Model(&database.User{}).Where("email = ?", "replica@example.com").Count(&users).Error)
	require.NoError(t, pg.DB.Model(&UserAuth{}).Where("provider_user_id = ?", "replica@example.com").Count(&creds).Error)
	require.Equal(t, int64(1), users)
	require.Equal(t, int64(1), creds)
}

// TestPostgresConcurrentBootstrapAdminTakesOverOnce: replicas booting together
// against a pre-registered (unverified) account take it over exactly once; the
// rest see an admin and leave it alone. Runs the takeover's session and
// lockout writes on real Postgres inside the advisory-locked transaction.
func TestPostgresConcurrentBootstrapAdminTakesOverOnce(t *testing.T) {
	ctx := context.Background()
	pg, err := testperf.StartIsolatedPostgres(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	// Test-only schema for the tables the takeover touches.
	require.NoError(t, pg.DB.AutoMigrate(&database.User{}, &UserAuth{}, &session.UserSession{}, &database.AuthAttempt{}))
	sqlDB, err := pg.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(16)

	hash, err := HashPassword("squatter-chose-this-1")
	require.NoError(t, err)
	squatter := database.User{Email: "replica@example.com", Role: "user", AuthMethod: "email"}
	require.NoError(t, pg.DB.Create(&squatter).Error)
	require.NoError(t, pg.DB.Create(&UserAuth{UserID: squatter.ID, Provider: "email", ProviderUserID: squatter.Email, PasswordHash: hash}).Error)
	require.NoError(t, pg.DB.Create(&UserAuth{UserID: squatter.ID, Provider: "google", ProviderUserID: "sub-squatter"}).Error)
	uid := squatter.ID
	require.NoError(t, pg.DB.Create(&session.UserSession{UserID: &uid, SessionToken: "squatter-tok", Provider: "email"}).Error)

	const workers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan BootstrapAdminResult, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := EnsureBootstrapAdmin(ctx, pg.DB, "replica@example.com", strongAdminPassword)
			if err != nil {
				errs <- err
				return
			}
			results <- res
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		require.NoError(t, err)
	}
	tookOver := 0
	for res := range results {
		require.Equal(t, squatter.ID, res.UserID)
		if res.TookOver {
			tookOver++
		} else {
			require.True(t, res.PasswordIgnored)
		}
	}
	require.Equal(t, 1, tookOver)

	var creds []UserAuth
	require.NoError(t, pg.DB.Where("user_id = ?", squatter.ID).Find(&creds).Error)
	require.Len(t, creds, 1)
	require.True(t, CheckPassword(strongAdminPassword, creds[0].PasswordHash))
	var sess session.UserSession
	require.NoError(t, pg.DB.Where("session_token = ?", "squatter-tok").First(&sess).Error)
	require.True(t, sess.Revoked)
}

// TestPostgresBootstrapAdminReleasesDeletedAccountsLogin runs the dead-login
// release against Postgres with production's partial active-email index: a
// self-deleted account's login no longer shadows the bootstrap admin's, and
// reset-password repairs an admin that is already shadowed.
func TestPostgresBootstrapAdminReleasesDeletedAccountsLogin(t *testing.T) {
	ctx := context.Background()
	pg, err := testperf.StartIsolatedPostgres(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	require.NoError(t, pg.DB.AutoMigrate(&database.User{}, &UserAuth{}, &session.UserSession{}, &database.AuthAttempt{}))
	require.NoError(t, pg.DB.Exec("DROP INDEX IF EXISTS idx_users_email").Error)
	require.NoError(t, pg.DB.Exec("CREATE UNIQUE INDEX idx_users_email_lower_active ON users USING btree (lower(TRIM(BOTH FROM email))) WHERE ((email IS NOT NULL) AND (btrim(email) <> ''::text) AND (deleted_at IS NULL))").Error)
	svc := NewAuthService(pg.DB)

	seedDeleted := func(email string) {
		t.Helper()
		hash, err := HashPassword("deleted-account-pass-1")
		require.NoError(t, err)
		now := time.Now()
		gone := database.User{Email: email, Role: "user", AuthMethod: "email", EmailVerified: true, DeletedAt: &now}
		require.NoError(t, pg.DB.Create(&gone).Error)
		require.NoError(t, pg.DB.Create(&UserAuth{UserID: gone.ID, Provider: "email", ProviderUserID: email, PasswordHash: hash, EmailVerified: true}).Error)
	}

	seedDeleted("owner@example.com")
	res, err := EnsureBootstrapAdmin(ctx, pg.DB, "owner@example.com", strongAdminPassword)
	require.NoError(t, err)
	require.True(t, res.Created)
	require.Equal(t, 1, res.ReleasedDeadCredentials)
	authRec, err := svc.LoginWithEmail("owner@example.com", strongAdminPassword)
	require.NoError(t, err)
	require.Equal(t, res.UserID, authRec.UserID)

	// Already shadowed (the pre-fix state): reset-password repairs it.
	seedDeleted("second@example.com")
	admin := database.User{Email: "second@example.com", Role: "admin", AuthMethod: "email", EmailVerified: true}
	require.NoError(t, pg.DB.Create(&admin).Error)
	require.NoError(t, pg.DB.Create(&UserAuth{UserID: admin.ID, Provider: "email", ProviderUserID: admin.Email, PasswordHash: "stale", EmailVerified: true}).Error)
	out, err := SetEmailPassword(ctx, pg.DB, "second@example.com", "Brand-new-Admin-pass-42")
	require.NoError(t, err)
	require.Equal(t, 1, out.ReleasedDeadCredentials)
	authRec, err = svc.LoginWithEmail("second@example.com", "Brand-new-Admin-pass-42")
	require.NoError(t, err)
	require.Equal(t, admin.ID, authRec.UserID)

	// A different live account's login for the address is never touched.
	other := database.User{Email: "renamed@example.com", Role: "user", AuthMethod: "email", EmailVerified: true}
	require.NoError(t, pg.DB.Create(&other).Error)
	require.NoError(t, pg.DB.Create(&UserAuth{UserID: other.ID, Provider: "email", ProviderUserID: "third@example.com", PasswordHash: "theirs"}).Error)
	_, err = EnsureBootstrapAdmin(ctx, pg.DB, "third@example.com", strongAdminPassword)
	require.ErrorIs(t, err, ErrAdminEmailCredentialInUse)
	var users int64
	require.NoError(t, pg.DB.Model(&database.User{}).Where("email = ?", "third@example.com").Count(&users).Error)
	require.Zero(t, users)
}
