package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const strongAdminPassword = "Tr4ffic-Lantern-Basil-91"

func TestValidateAdminPassword(t *testing.T) {
	cases := []struct {
		name    string
		pw      string
		wantErr error
	}{
		{"strong", strongAdminPassword, nil},
		{"passphrase", "olive oven midnight saffron", nil},
		{"exactly 12", "k7#Qm2!vRx9z", nil},
		{"11 chars", "k7#Qm2!vRx9", ErrAdminPasswordTooShort},
		{"empty", "", ErrAdminPasswordTooShort},
		{"multibyte counts runes", "ñandú-ñandú", ErrAdminPasswordTooShort},
		{"too long for bcrypt", strings.Repeat("aB3$", 19), ErrAdminPasswordTooLong},
		{"denylisted", "password1234", ErrAdminPasswordWeak},
		{"denylisted with separators", "Password-12-34", ErrAdminPasswordWeak},
		{"digits", "123456789012", ErrAdminPasswordWeak},
		{"placeholder", "change-me-to-a-strong-one", ErrAdminPasswordWeak},
		{"placeholder upper", "REPLACE_ME_BEFORE_DEPLOY", ErrAdminPasswordWeak},
		{"low variety", "abababababab", ErrAdminPasswordWeak},
		{"repeated", "zzzzzzzzzzzzzzzz", ErrAdminPasswordWeak},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAdminPassword(tc.pw)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// Decision D-5: the denylist is digests of normalised passwords and is
// injectable. A synthetic password is denied (separators and case ignored)
// once its digest is installed, and accepted again after restore.
func TestValidateAdminPassword_InjectedDigestDenylist(t *testing.T) {
	const synthetic = "Quartz-Meadow-Lamp-58"
	sum := sha256.Sum256([]byte("quartzmeadowlamp58"))
	require.NoError(t, ValidateAdminPassword(synthetic))

	restore := SetAdminPasswordDenylist([]string{hex.EncodeToString(sum[:])})
	require.ErrorIs(t, ValidateAdminPassword(synthetic), ErrAdminPasswordWeak)
	require.ErrorIs(t, ValidateAdminPassword("QUARTZ meadow lamp 58"), ErrAdminPasswordWeak)
	require.NoError(t, ValidateAdminPassword("password1234"), "injected set replaces the default")

	restore()
	require.NoError(t, ValidateAdminPassword(synthetic))
	require.ErrorIs(t, ValidateAdminPassword("password1234"), ErrAdminPasswordWeak)
}

func newBootstrapAdminDB(t *testing.T) *gorm.DB {
	t.Helper()
	_, db := newAuthEnvelopeHandler(t)
	return db
}

func countUsersByEmail(t *testing.T, db *gorm.DB, email string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&database.User{}).Where("LOWER(email) = LOWER(?)", email).Count(&n).Error)
	return n
}

func TestEnsureBootstrapAdminCreatesLoginableVerifiedAdmin(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	res, err := EnsureBootstrapAdmin(context.Background(), db, "  Owner@Example.COM ", strongAdminPassword)
	require.NoError(t, err)
	require.True(t, res.Created)
	require.False(t, res.PasswordIgnored)
	require.NotZero(t, res.UserID)

	var user database.User
	require.NoError(t, db.First(&user, res.UserID).Error)
	require.Equal(t, "owner@example.com", user.Email)
	require.Equal(t, "admin", user.Role)
	require.Equal(t, "email", user.AuthMethod)
	require.True(t, user.EmailVerified)
	require.NotNil(t, user.ActivatedAt)

	authRec, err := h.authService.LoginWithEmail("owner@example.com", strongAdminPassword)
	require.NoError(t, err, "bootstrap admin must be able to sign in with ADMIN_PASSWORD")
	require.True(t, authRec.EmailVerified)
	require.Equal(t, res.UserID, authRec.UserID)
}

func TestEnsureBootstrapAdminIsIdempotentAndNeverOverwritesPassword(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	ctx := context.Background()
	first, err := EnsureBootstrapAdmin(ctx, db, "owner@example.com", strongAdminPassword)
	require.NoError(t, err)

	second, err := EnsureBootstrapAdmin(ctx, db, "OWNER@example.com", "a-different-Strong-pass-77")
	require.NoError(t, err)
	require.False(t, second.Created)
	require.True(t, second.PasswordIgnored, "a second boot must report ADMIN_PASSWORD as ignored")
	require.Equal(t, first.UserID, second.UserID)

	require.Equal(t, int64(1), countUsersByEmail(t, db, "owner@example.com"))
	var creds int64
	require.NoError(t, db.Model(&UserAuth{}).Where("user_id = ?", first.UserID).Count(&creds).Error)
	require.Equal(t, int64(1), creds)

	_, err = h.authService.LoginWithEmail("owner@example.com", strongAdminPassword)
	require.NoError(t, err, "original password still works")
	_, err = h.authService.LoginWithEmail("owner@example.com", "a-different-Strong-pass-77")
	require.ErrorIs(t, err, ErrInvalidCredentials, "second ADMIN_PASSWORD must not replace the first")
}

// newTakeoverHandler adds the session and login-throttle tables a takeover
// writes to (test-only AutoMigrate; production owns them via migrations).
func newTakeoverHandler(t *testing.T) (*AuthHandler, *gorm.DB) {
	t.Helper()
	h, db := newAuthEnvelopeHandler(t)
	require.NoError(t, db.AutoMigrate(&session.UserSession{}, &database.AuthAttempt{}))
	return h, db
}

// seedPreRegisteredAccount stands in for someone who signed up with the
// operator's email before the bootstrap ran: the email/password registration
// they chose, plus whatever they linked and the sessions they hold.
func seedPreRegisteredAccount(t *testing.T, h *AuthHandler, db *gorm.DB, email, password string, verified bool) database.User {
	t.Helper()
	authRec, _, err := h.authService.RegisterWithEmail(email, password, "Squatter")
	require.NoError(t, err)
	user := database.User{Email: email, Role: "user", AuthMethod: "email", EmailVerified: verified}
	require.NoError(t, db.Create(&user).Error)
	authRec.UserID = user.ID
	authRec.EmailVerified = verified
	require.NoError(t, h.authService.CreateAuth(authRec))
	return user
}

func TestEnsureBootstrapAdminTakesOverPreRegisteredUnverifiedAccount(t *testing.T) {
	h, db := newTakeoverHandler(t)
	const squatterPassword = "squatter-chose-this-1"
	user := seedPreRegisteredAccount(t, h, db, "owner@example.com", squatterPassword, false)
	// A session on the unverified account can link any Google identity.
	require.NoError(t, db.Create(&UserAuth{UserID: user.ID, Provider: "google", ProviderUserID: "google-sub-of-squatter"}).Error)
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", user.ID).Update("google_id", "google-sub-of-squatter").Error)
	uid := user.ID
	require.NoError(t, db.Create(&session.UserSession{UserID: &uid, SessionToken: "squatter-session", Provider: "email"}).Error)
	require.NoError(t, db.Create(&database.AuthAttempt{Principal: "owner@example.com", Kind: PasswordLoginThrottleKind, Count: 7}).Error)

	res, err := EnsureBootstrapAdmin(context.Background(), db, "Owner@Example.com", strongAdminPassword)
	require.NoError(t, err)
	require.Equal(t, user.ID, res.UserID)
	require.False(t, res.Created)
	require.True(t, res.TookOver)
	require.True(t, res.RolePromoted)
	require.False(t, res.PasswordIgnored, "the operator's password is applied, not ignored")
	require.Equal(t, 1, res.UnlinkedSignInMethods)

	_, err = h.authService.LoginWithEmail("owner@example.com", squatterPassword)
	require.ErrorIs(t, err, ErrInvalidCredentials, "the pre-registered password must stop working")
	authRec, err := h.authService.LoginWithEmail("owner@example.com", strongAdminPassword)
	require.NoError(t, err)
	require.True(t, authRec.EmailVerified)
	require.Empty(t, authRec.VerificationToken)

	var reloaded database.User
	require.NoError(t, db.First(&reloaded, user.ID).Error)
	require.Equal(t, "admin", reloaded.Role)
	require.True(t, reloaded.EmailVerified)
	require.Equal(t, "email", reloaded.AuthMethod)
	require.Empty(t, reloaded.GoogleID)

	var providers []string
	require.NoError(t, db.Model(&UserAuth{}).Where("user_id = ?", user.ID).Pluck("provider", &providers).Error)
	require.Equal(t, []string{"email"}, providers, "every other sign-in method is unlinked")

	var sess session.UserSession
	require.NoError(t, db.Where("session_token = ?", "squatter-session").First(&sess).Error)
	require.True(t, sess.Revoked, "sessions opened before the takeover are revoked")
	require.Equal(t, string(session.RevocationReasonSecurityReset), sess.RevocationReason)
	var attempts int64
	require.NoError(t, db.Model(&database.AuthAttempt{}).Count(&attempts).Error)
	require.Zero(t, attempts, "the login lockout is cleared")

	// The next boot sees an admin and leaves it alone.
	again, err := EnsureBootstrapAdmin(context.Background(), db, "owner@example.com", "a-different-Strong-pass-77")
	require.NoError(t, err)
	require.True(t, again.PasswordIgnored)
	require.False(t, again.TookOver)
	_, err = h.authService.LoginWithEmail("owner@example.com", strongAdminPassword)
	require.NoError(t, err)
}

func TestEnsureBootstrapAdminTakesOverVerifiedAccountToo(t *testing.T) {
	// Verification only proves someone clicked the mail (possibly the operator,
	// for a registration they never made), not who chose the password.
	h, db := newTakeoverHandler(t)
	user := seedPreRegisteredAccount(t, h, db, "staff@example.com", "their-own-password-123", true)

	res, err := EnsureBootstrapAdmin(context.Background(), db, "staff@example.com", strongAdminPassword)
	require.NoError(t, err)
	require.True(t, res.TookOver)
	require.Zero(t, res.UnlinkedSignInMethods)
	var reloaded database.User
	require.NoError(t, db.First(&reloaded, user.ID).Error)
	require.Equal(t, "admin", reloaded.Role)
	_, err = h.authService.LoginWithEmail("staff@example.com", "their-own-password-123")
	require.ErrorIs(t, err, ErrInvalidCredentials)
	_, err = h.authService.LoginWithEmail("staff@example.com", strongAdminPassword)
	require.NoError(t, err)
}

func TestEnsureBootstrapAdminTakesOverGoogleOnlyAccount(t *testing.T) {
	h, db := newTakeoverHandler(t)
	user := database.User{Email: "chef@example.com", Role: "user", AuthMethod: "google", GoogleID: "sub-1"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&UserAuth{UserID: user.ID, Provider: "google", ProviderUserID: "sub-1", EmailVerified: true}).Error)

	res, err := EnsureBootstrapAdmin(context.Background(), db, "chef@example.com", strongAdminPassword)
	require.NoError(t, err)
	require.True(t, res.TookOver)
	require.Equal(t, 1, res.UnlinkedSignInMethods)
	authRec, err := h.authService.LoginWithEmail("chef@example.com", strongAdminPassword)
	require.NoError(t, err)
	require.Equal(t, user.ID, authRec.UserID)
	var googleCreds int64
	require.NoError(t, db.Model(&UserAuth{}).Where("user_id = ? AND provider = ?", user.ID, "google").Count(&googleCreds).Error)
	require.Zero(t, googleCreds)
}

func TestEnsureBootstrapAdminRefusesWalletLinkedAccount(t *testing.T) {
	cases := []struct {
		name string
		seed func(t *testing.T, db *gorm.DB, user *database.User)
	}{
		{"users.address", func(t *testing.T, db *gorm.DB, user *database.User) {
			require.NoError(t, db.Model(user).Update("address", "0xabc0000000000000000000000000000000000001").Error)
		}},
		{"wallet credential", func(t *testing.T, db *gorm.DB, user *database.User) {
			require.NoError(t, db.Create(&UserAuth{UserID: user.ID, Provider: "wallet", WalletAddress: "0xabc0000000000000000000000000000000000002"}).Error)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, db := newTakeoverHandler(t)
			const theirPassword = "their-own-password-123"
			user := seedPreRegisteredAccount(t, h, db, "wallet@example.com", theirPassword, false)
			tc.seed(t, db, &user)
			uid := user.ID
			require.NoError(t, db.Create(&session.UserSession{UserID: &uid, SessionToken: "their-session", Provider: "email"}).Error)

			_, err := EnsureBootstrapAdmin(context.Background(), db, "wallet@example.com", strongAdminPassword)
			require.ErrorIs(t, err, ErrBootstrapAdminWalletLinked)

			var reloaded database.User
			require.NoError(t, db.First(&reloaded, user.ID).Error)
			require.Equal(t, "user", reloaded.Role, "a wallet-linked account is never promoted")
			var cred UserAuth
			require.NoError(t, db.Where("user_id = ? AND provider = ?", user.ID, "email").First(&cred).Error)
			require.True(t, CheckPassword(theirPassword, cred.PasswordHash), "nothing is written on refusal")
			var sess session.UserSession
			require.NoError(t, db.Where("session_token = ?", "their-session").First(&sess).Error)
			require.False(t, sess.Revoked)
		})
	}
}

func TestEnsureBootstrapAdminRejectsWeakPasswordAndBadEmail(t *testing.T) {
	db := newBootstrapAdminDB(t)
	_, err := EnsureBootstrapAdmin(context.Background(), db, "owner@example.com", "changeme")
	require.ErrorIs(t, err, ErrAdminPasswordTooShort)
	_, err = EnsureBootstrapAdmin(context.Background(), db, "owner@example.com", "changeme-changeme")
	require.ErrorIs(t, err, ErrAdminPasswordWeak)
	_, err = EnsureBootstrapAdmin(context.Background(), db, "not-an-email", strongAdminPassword)
	require.ErrorIs(t, err, ErrAdminEmailInvalid)
	require.Zero(t, countUsersByEmail(t, db, "owner@example.com"))
}

func TestEnsureBootstrapAdminConcurrentCallsCreateOneUser(t *testing.T) {
	db := newBootstrapAdminDB(t)
	const workers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan BootstrapAdminResult, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := EnsureBootstrapAdmin(context.Background(), db, "race@example.com", strongAdminPassword)
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
	var ids = map[uint]struct{}{}
	for res := range results {
		if res.Created {
			created++
		}
		ids[res.UserID] = struct{}{}
	}
	require.Equal(t, 1, created, "exactly one caller creates the admin")
	require.Len(t, ids, 1, "every caller resolves the same user")
	require.Equal(t, int64(1), countUsersByEmail(t, db, "race@example.com"))
	var creds int64
	require.NoError(t, db.Model(&UserAuth{}).Where("provider = ? AND provider_user_id = ?", "email", "race@example.com").Count(&creds).Error)
	require.Equal(t, int64(1), creds)
}

func TestSetEmailPasswordResetsAndVerifies(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	res, err := EnsureBootstrapAdmin(context.Background(), db, "owner@example.com", strongAdminPassword)
	require.NoError(t, err)
	// Simulate an unverified + mid-reset credential.
	require.NoError(t, db.Model(&UserAuth{}).Where("user_id = ?", res.UserID).Updates(map[string]interface{}{"email_verified": false, "reset_token": "pending"}).Error)
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", res.UserID).Update("email_verified", false).Error)

	out, err := SetEmailPassword(context.Background(), db, "OWNER@example.com", "Brand-new-Admin-pass-42")
	require.NoError(t, err)
	require.Equal(t, res.UserID, out.UserID)
	require.False(t, out.CredentialCreated)

	_, err = h.authService.LoginWithEmail("owner@example.com", strongAdminPassword)
	require.ErrorIs(t, err, ErrInvalidCredentials)
	authRec, err := h.authService.LoginWithEmail("owner@example.com", "Brand-new-Admin-pass-42")
	require.NoError(t, err)
	require.True(t, authRec.EmailVerified)
	require.Empty(t, authRec.ResetToken)
	var user database.User
	require.NoError(t, db.First(&user, res.UserID).Error)
	require.True(t, user.EmailVerified)
}

func TestSetEmailPasswordAddsCredentialForOAuthOnlyUserAndRejectsUnknown(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	user := database.User{Email: "google@example.com", Role: "admin", AuthMethod: "google", EmailVerified: true}
	require.NoError(t, db.Create(&user).Error)

	out, err := SetEmailPassword(context.Background(), db, "google@example.com", strongAdminPassword)
	require.NoError(t, err)
	require.True(t, out.CredentialCreated)
	_, err = h.authService.LoginWithEmail("google@example.com", strongAdminPassword)
	require.NoError(t, err)

	_, err = SetEmailPassword(context.Background(), db, "nobody@example.com", strongAdminPassword)
	require.ErrorIs(t, err, ErrAdminUserNotFound)
	_, err = SetEmailPassword(context.Background(), db, "google@example.com", "short")
	require.ErrorIs(t, err, ErrAdminPasswordTooShort)
}

// mirrorProductionActiveEmailIndex swaps the test schema's full unique index on
// users.email for production's partial one (idx_users_email_lower_active, live
// rows only), so a soft-deleted account and a live one can share an address.
func mirrorProductionActiveEmailIndex(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec("DROP INDEX IF EXISTS idx_users_email").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_users_email_lower_active ON users (lower(trim(email))) WHERE email IS NOT NULL AND trim(email) <> '' AND deleted_at IS NULL").Error)
}

// seedSoftDeletedAccount is an account that registered the address and then
// self-deleted: account deletion only stamps deleted_at and keeps the email
// and its email/password login.
func seedSoftDeletedAccount(t *testing.T, h *AuthHandler, db *gorm.DB, email, password string) database.User {
	t.Helper()
	user := seedPreRegisteredAccount(t, h, db, email, password, true)
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"deleted_at": time.Now(), "deletion_scheduled_at": time.Now().Add(30 * 24 * time.Hour),
	}).Error)
	return user
}

func emailCredentialsFor(t *testing.T, db *gorm.DB, email string) []UserAuth {
	t.Helper()
	var creds []UserAuth
	require.NoError(t, db.Where("provider = ? AND LOWER(provider_user_id) = ?", "email", email).Order("id").Find(&creds).Error)
	return creds
}

func TestEnsureBootstrapAdminReleasesSoftDeletedAccountsEmailLogin(t *testing.T) {
	h, db := newTakeoverHandler(t)
	mirrorProductionActiveEmailIndex(t, db)
	const deletedPassword = "deleted-account-pass-1"
	deleted := seedSoftDeletedAccount(t, h, db, "owner@example.com", deletedPassword)

	res, err := EnsureBootstrapAdmin(context.Background(), db, "owner@example.com", strongAdminPassword)
	require.NoError(t, err)
	require.True(t, res.Created)
	require.NotEqual(t, deleted.ID, res.UserID)
	require.Equal(t, 1, res.ReleasedDeadCredentials)

	creds := emailCredentialsFor(t, db, "owner@example.com")
	require.Len(t, creds, 1, "only the admin's login is left for the address")
	require.Equal(t, res.UserID, creds[0].UserID)
	authRec, err := h.authService.LoginWithEmail("owner@example.com", strongAdminPassword)
	require.NoError(t, err, "the admin signs in with ADMIN_PASSWORD")
	require.Equal(t, res.UserID, authRec.UserID)
	_, err = h.authService.LoginWithEmail("owner@example.com", deletedPassword)
	require.ErrorIs(t, err, ErrInvalidCredentials)

	var kept database.User
	require.NoError(t, db.First(&kept, deleted.ID).Error)
	require.NotNil(t, kept.DeletedAt, "the deleted account itself is left for deletion review")
	require.Equal(t, "user", kept.Role)

	again, err := EnsureBootstrapAdmin(context.Background(), db, "owner@example.com", "a-different-Strong-pass-77")
	require.NoError(t, err)
	require.True(t, again.PasswordIgnored)
	require.Zero(t, again.ReleasedDeadCredentials)
}

func TestEnsureBootstrapAdminReleasesOrphanEmailLogin(t *testing.T) {
	h, db := newTakeoverHandler(t)
	// user_auths has no foreign key: a hard-deleted user can leave its login.
	require.NoError(t, db.Create(&UserAuth{UserID: 4242, Provider: "email", ProviderUserID: "Owner@Example.com ", PasswordHash: "x", EmailVerified: true}).Error)

	res, err := EnsureBootstrapAdmin(context.Background(), db, "owner@example.com", strongAdminPassword)
	require.NoError(t, err)
	require.True(t, res.Created)
	require.Equal(t, 1, res.ReleasedDeadCredentials)
	authRec, err := h.authService.LoginWithEmail("owner@example.com", strongAdminPassword)
	require.NoError(t, err)
	require.Equal(t, res.UserID, authRec.UserID)
}

func TestEnsureBootstrapAdminTakeoverReleasesSoftDeletedAccountsEmailLogin(t *testing.T) {
	// A deleted email/password account plus a later live Google-only account
	// with the same address (Google signup is not blocked by the deleted one).
	h, db := newTakeoverHandler(t)
	mirrorProductionActiveEmailIndex(t, db)
	seedSoftDeletedAccount(t, h, db, "chef@example.com", "deleted-account-pass-1")
	live := database.User{Email: "chef@example.com", Role: "user", AuthMethod: "google", GoogleID: "sub-9", EmailVerified: true}
	require.NoError(t, db.Create(&live).Error)
	require.NoError(t, db.Create(&UserAuth{UserID: live.ID, Provider: "google", ProviderUserID: "sub-9", EmailVerified: true}).Error)

	res, err := EnsureBootstrapAdmin(context.Background(), db, "chef@example.com", strongAdminPassword)
	require.NoError(t, err)
	require.True(t, res.TookOver)
	require.Equal(t, live.ID, res.UserID)
	require.Equal(t, 1, res.ReleasedDeadCredentials)
	authRec, err := h.authService.LoginWithEmail("chef@example.com", strongAdminPassword)
	require.NoError(t, err)
	require.Equal(t, live.ID, authRec.UserID)
}

func TestEnsureBootstrapAdminRefusesWhenAnotherLiveAccountHoldsTheLogin(t *testing.T) {
	h, db := newTakeoverHandler(t)
	const theirPassword = "their-own-password-123"
	other := database.User{Email: "renamed@example.com", Role: "user", AuthMethod: "email", EmailVerified: true}
	require.NoError(t, db.Create(&other).Error)
	theirHash, err := HashPassword(theirPassword)
	require.NoError(t, err)
	require.NoError(t, db.Create(&UserAuth{UserID: other.ID, Provider: "email", ProviderUserID: "owner@example.com", PasswordHash: theirHash, EmailVerified: true}).Error)

	_, err = EnsureBootstrapAdmin(context.Background(), db, "owner@example.com", strongAdminPassword)
	require.ErrorIs(t, err, ErrAdminEmailCredentialInUse)
	require.Zero(t, countUsersByEmail(t, db, "owner@example.com"), "nothing is created")
	creds := emailCredentialsFor(t, db, "owner@example.com")
	require.Len(t, creds, 1)
	require.Equal(t, other.ID, creds[0].UserID, "the live account's login is never touched")
	_, err = h.authService.LoginWithEmail("owner@example.com", theirPassword)
	require.NoError(t, err)

	// reset-password on a live account with that address refuses the same way.
	target := database.User{Email: "owner@example.com", Role: "admin", AuthMethod: "google", EmailVerified: true}
	require.NoError(t, db.Create(&target).Error)
	_, err = SetEmailPassword(context.Background(), db, "owner@example.com", strongAdminPassword)
	require.ErrorIs(t, err, ErrAdminEmailCredentialInUse)
	var targetCreds int64
	require.NoError(t, db.Model(&UserAuth{}).Where("user_id = ?", target.ID).Count(&targetCreds).Error)
	require.Zero(t, targetCreds)
}

func TestSetEmailPasswordRepairsAdminShadowedByDeletedAccountsLogin(t *testing.T) {
	// The state the pre-fix bootstrap left behind: a deleted account's older
	// login for the address shadows the admin's, so LoginWithEmail (oldest row
	// first) never reaches the admin. `admin reset-password` must repair it.
	h, db := newTakeoverHandler(t)
	mirrorProductionActiveEmailIndex(t, db)
	seedSoftDeletedAccount(t, h, db, "owner@example.com", "deleted-account-pass-1")
	admin := database.User{Email: "owner@example.com", Role: "admin", AuthMethod: "email", EmailVerified: true}
	require.NoError(t, db.Create(&admin).Error)
	staleHash, err := HashPassword(strongAdminPassword)
	require.NoError(t, err)
	require.NoError(t, db.Create(&UserAuth{UserID: admin.ID, Provider: "email", ProviderUserID: "owner@example.com", PasswordHash: staleHash, EmailVerified: true}).Error)
	_, err = h.authService.LoginWithEmail("owner@example.com", strongAdminPassword)
	require.ErrorIs(t, err, ErrInvalidCredentials, "precondition: the admin is shadowed")

	out, err := SetEmailPassword(context.Background(), db, "owner@example.com", "Brand-new-Admin-pass-42")
	require.NoError(t, err)
	require.Equal(t, admin.ID, out.UserID)
	require.Equal(t, 1, out.ReleasedDeadCredentials)
	authRec, err := h.authService.LoginWithEmail("owner@example.com", "Brand-new-Admin-pass-42")
	require.NoError(t, err, "the admin can sign in after reset-password")
	require.Equal(t, admin.ID, authRec.UserID)
}
