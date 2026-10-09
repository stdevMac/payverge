package verification

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var verificationTestDBSeq atomic.Int64

func newVerificationTestDB(t testing.TB) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), verificationTestDBSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&database.User{}, &emailAuthState{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedPendingEmailIdentity(t *testing.T, db *gorm.DB, email, providerUserID string) database.User {
	t.Helper()
	user := database.User{Email: email, AuthMethod: "email", Role: "user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(time.Hour)
	auth := emailAuthState{
		UserID: user.ID, Provider: "email", ProviderUserID: providerUserID,
		VerificationToken: "hash", VerificationExpiry: &expiry,
	}
	if err := db.Create(&auth).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func TestIsOperatorVerifiedOffModeNeedsOnlyTheBinding(t *testing.T) {
	db := newVerificationTestDB(t)
	bound := seedPendingEmailIdentity(t, db, "owner@example.com", "Owner@Example.com")
	stale := seedPendingEmailIdentity(t, db, "new@example.com", "old@example.com")
	svc := NewService(db)

	t.Setenv("EMAIL_VERIFICATION", "required")
	if ok, err := svc.IsOperatorVerified(bound.ID, "email"); err != nil || ok {
		t.Fatalf("required: unverified identity accepted (ok=%v err=%v)", ok, err)
	}

	t.Setenv("EMAIL_VERIFICATION", "off")
	if ok, err := svc.IsOperatorVerified(bound.ID, "email"); err != nil || !ok {
		t.Fatalf("off: bound identity rejected (ok=%v err=%v)", ok, err)
	}
	if ok, err := svc.IsOperatorVerified(bound.ID, "register"); err != nil || !ok {
		t.Fatalf("off: register session rejected (ok=%v err=%v)", ok, err)
	}
	if ok, err := svc.IsOperatorVerified(stale.ID, "email"); err != nil || ok {
		t.Fatalf("off: identity bound to an old address accepted (ok=%v err=%v)", ok, err)
	}
	// Off mode never vouches for a Google identity Google did not verify.
	if err := db.Create(&emailAuthState{UserID: bound.ID, Provider: "google", ProviderUserID: "g-1"}).Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := svc.IsOperatorVerified(bound.ID, "google"); err != nil || ok {
		t.Fatalf("off: unverified google identity accepted (ok=%v err=%v)", ok, err)
	}

	// The flags stay truthful: checking never converges them.
	var reloaded database.User
	if err := db.First(&reloaded, bound.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reloaded.EmailVerified {
		t.Fatal("off mode must not write users.email_verified")
	}
}

func TestDropUnprovenEmailCredential(t *testing.T) {
	db := newVerificationTestDB(t)
	if err := db.Exec("CREATE TABLE user_sessions (id INTEGER PRIMARY KEY, user_id INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	unused := seedPendingEmailIdentity(t, db, "unused@example.com", "unused@example.com")
	used := seedPendingEmailIdentity(t, db, "used@example.com", "used@example.com")
	if err := db.Exec("INSERT INTO user_sessions (user_id) VALUES (?)", used.ID).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService(db)

	if err := svc.DropUnprovenEmailCredential(unused.ID); err != nil {
		t.Fatalf("unused account: %v", err)
	}
	var left int64
	db.Model(&emailAuthState{}).Where("user_id = ?", unused.ID).Count(&left)
	if left != 0 {
		t.Fatalf("unused account kept %d unproven credential(s)", left)
	}
	var kept database.User
	if err := db.First(&kept, unused.ID).Error; err != nil {
		t.Fatalf("the user row itself must survive: %v", err)
	}

	if err := svc.DropUnprovenEmailCredential(used.ID); err != ErrUnprovenEmailAccount {
		t.Fatalf("used account: err=%v, want ErrUnprovenEmailAccount", err)
	}
	db.Model(&emailAuthState{}).Where("user_id = ?", used.ID).Count(&left)
	if left != 1 {
		t.Fatalf("a refused merge must not touch the credential, have %d rows", left)
	}

	// Nothing unproven: no-op.
	if err := svc.DropUnprovenEmailCredential(unused.ID); err != nil {
		t.Fatalf("second call: %v", err)
	}

	// A sign-in identity planted next to the unproven password (a wallet,
	// another OAuth login) makes the account non-pristine even after its
	// sessions were purged: refuse rather than hand the planted identity
	// a merged account.
	walletLinked := seedPendingEmailIdentity(t, db, "wallet@example.com", "wallet@example.com")
	if err := db.Model(&database.User{}).Where("id = ?", walletLinked.ID).Update("address", "0xabc").Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.DropUnprovenEmailCredential(walletLinked.ID); err != ErrUnprovenEmailAccount {
		t.Fatalf("wallet-linked account: err=%v, want ErrUnprovenEmailAccount", err)
	}
	otherAuth := seedPendingEmailIdentity(t, db, "other@example.com", "other@example.com")
	if err := db.Create(&emailAuthState{UserID: otherAuth.ID, Provider: "instagram", ProviderUserID: "ig-1", EmailVerified: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.DropUnprovenEmailCredential(otherAuth.ID); err != ErrUnprovenEmailAccount {
		t.Fatalf("account with another identity: err=%v, want ErrUnprovenEmailAccount", err)
	}
}

// Any owned workspace counts as use: the unproven credential is kept and the
// merge is refused.
func TestDropUnprovenEmailCredentialCountsOwnedWorkspaces(t *testing.T) {
	db := newVerificationTestDB(t)
	if err := db.Exec("CREATE TABLE businesses (id INTEGER PRIMARY KEY, user_id INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	user := seedPendingEmailIdentity(t, db, "legacy@example.com", "legacy@example.com")
	if err := db.Exec("INSERT INTO businesses (user_id) VALUES (?)", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewService(db).DropUnprovenEmailCredential(user.ID); err != ErrUnprovenEmailAccount {
		t.Fatalf("err=%v, want ErrUnprovenEmailAccount", err)
	}
}

// newReservationDB is a SQLite stand-in for users + user_auths (test-only
// AutoMigrate; businesses/user_sessions are absent, which the release path
// tolerates via HasTable).
func newReservationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&database.User{}, &emailAuthState{}))
	return db
}

// seedExpiredPlaceholder creates an unverified email-only registration whose
// verification window closed two days ago.
func seedExpiredPlaceholder(t *testing.T, db *gorm.DB, email, role string, now time.Time) database.User {
	t.Helper()
	created := now.Add(-2 * EmailReservationTTL)
	user := database.User{Email: email, Role: role, AuthMethod: "email", CreatedAt: created, UpdatedAt: created}
	require.NoError(t, db.Create(&user).Error)
	expiry := now.Add(-EmailReservationTTL)
	require.NoError(t, db.Create(&emailAuthState{
		UserID: user.ID, Provider: "email", ProviderUserID: email,
		VerificationExpiry: &expiry, CreatedAt: created, UpdatedAt: created,
	}).Error)
	return user
}

func TestReleaseExpiredEmailReservationReleasesAbandonedPlaceholder(t *testing.T) {
	db := newReservationDB(t)
	now := time.Now()
	user := seedExpiredPlaceholder(t, db, "abandoned@example.com", "user", now)

	released, err := NewService(db).ReleaseExpiredEmailReservation("Abandoned@Example.com", now)
	require.NoError(t, err)
	require.True(t, released)
	var n int64
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", user.ID).Count(&n).Error)
	require.Zero(t, n)
}

func TestReleaseExpiredEmailReservationNeverReleasesAPlatformAdmin(t *testing.T) {
	db := newReservationDB(t)
	now := time.Now()
	admin := seedExpiredPlaceholder(t, db, "admin@example.com", "admin", now)

	released, err := NewService(db).ReleaseExpiredEmailReservation("admin@example.com", now)
	require.NoError(t, err)
	require.False(t, released, "an admin row must never be treated as an abandoned signup")
	var n int64
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", admin.ID).Count(&n).Error)
	require.Equal(t, int64(1), n)
}
