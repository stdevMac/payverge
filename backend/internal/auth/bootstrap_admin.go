package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/stdevmac/payverge/backend/internal/auththrottle"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"gorm.io/gorm"
)

// Self-host admin provisioning: ADMIN_EMAIL/ADMIN_PASSWORD on boot and the
// `server admin create|reset-password` subcommands. Every path here creates or
// updates an email/password platform admin directly — it never goes through
// signup admission, so REGISTRATION_MODE (even "closed") cannot lock the
// operator out of their own instance.

// PasswordLoginThrottleKind is the auththrottle kind AuthHandler.Login records
// for failed email/password sign-ins; provisioning clears it so the operator
// is not locked out of an account they just (re)keyed.
const PasswordLoginThrottleKind = "password_login"

// AdminPasswordMinLength is the minimum length for operator-provisioned
// platform admin passwords (stricter than the 8-character signup floor).
const AdminPasswordMinLength = 12

// bcrypt silently truncates (or, in newer x/crypto, rejects) input past 72
// bytes; refuse it up front so the operator learns before the hash step.
const adminPasswordMaxBytes = 72

// bootstrapAdminAdvisoryLockKey serializes concurrent admin provisioning
// (several replicas booting with the same ADMIN_EMAIL, or a CLI call racing a
// boot) on Postgres. ASCII "PVADMIN1".
const bootstrapAdminAdvisoryLockKey int64 = 0x5056_4144_4d49_4e31

var (
	ErrAdminPasswordTooShort = fmt.Errorf("admin password must be at least %d characters", AdminPasswordMinLength)
	ErrAdminPasswordTooLong  = fmt.Errorf("admin password must be at most %d bytes", adminPasswordMaxBytes)
	ErrAdminPasswordWeak     = errors.New("admin password is a known default, placeholder or trivially guessable value")
	ErrAdminEmailInvalid     = errors.New("admin email is not a valid address")
	ErrAdminUserNotFound     = errors.New("no active user with that email")
	// ErrBootstrapAdminWalletLinked: the email belongs to an existing non-admin
	// account with a wallet identity. Taking it over would mean clearing the
	// wallet that may own its restaurants, and keeping it would hand whoever
	// linked that wallet a platform admin; so nothing is changed.
	ErrBootstrapAdminWalletLinked = errors.New("an existing non-admin account with this email has a linked wallet; it is never promoted automatically — use a different admin email")
	// ErrAdminEmailCredentialInUse: a DIFFERENT active account (its users.email
	// is another address) holds an email/password login for this address.
	// Login and password reset resolve the oldest such login, so provisioning
	// next to it would leave the admin unable to sign in, and removing it would
	// lock that account's owner out; so nothing is changed.
	ErrAdminEmailCredentialInUse = errors.New("another active account holds an email/password login for this address; nothing was changed — resolve that account first or use a different admin email")
)

// defaultAdminPasswordDenylistSHA256 holds lowercase hex SHA-256 digests of
// normalised (lowercase, separators removed; see normalizePasswordForDenylist)
// values that must never guard a platform admin: shipped examples, the
// documented local-dev credential, and the usual top-of-the-list guesses.
// Digests, not plaintext, so no real credential is spelled out in source
// (decision D-5). To add one: sha256 of the normalised value, lowercase hex.
var defaultAdminPasswordDenylistSHA256 = []string{
	"5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
	"0b14d501a594442a01c6859541bcb3e8164d183d32937b851835442f69d5c94e",
	"b3d17ebbe4f2b75d27b6309cfaae1487b667301a73951e7d523a039cd2dfe110",
	"ef92b778bafe771e89245b89ecbc08a44a4e166c06659911881f383d4473e94f",
	"b9c950640e1b3740e98acb93e669c65766f6670dd1609ba91ff41052ba48c6f3",
	"2671f872402f235bf78c0b27e68c74d3d5772d3b3b2096bf2fde6569f880ea85",
	"2e2b24f8ee40bb847fe85bb23336a39ef5948e6b49d897419ced68766b16967a",
	"4194d1706ed1f408d5e02d672777019f4d5385c766a8c6ca8acba3167d36a7b9",
	"a3fa51f0a359a8169317cf6f3de707d21ade6557ee00272ed31b82ba6761b2c1",
	"d82494f05d6917ba02f7aaa29689ccb444bb73f20380876cb05d1f37537b7892",
	"5af9af63c3d67ede16c88986dee08673112a36afecdd8b9f49d3b395fe8dd1eb",
	"c7c7c392e92774ca08a39aefd22efadc9b9019927e8fe3ce38d47332f4fa1231",
	"749f09bade8aca755660eeb17792da880218d4fbdc4e25fbec279d7fe9f65d70",
	"4179a76f97865271164fa5cccdaa0e3f173dc2e27b72becaf83fddc4eb799b9d",
	"3c8872c094682f4c3fcdffdab80a8d351dbeee9b31a29e6ea092b51cf473e732",
	"e34f92a20532a873cb3184398070b4b82a8fa29cf48572c203dc5f0fa6158231",
	"955be190467031e0cb01c26ed5335c74c65131881b0ea6918bf6da0c9b724a5f",
	"dba93d9374d2c9db02d22fd85d684d634c6a6b734f4ff911e3f4eccfa65fa14f",
	"798644c97014d180ae001c4e2728862bb3cb511775d57da429d041b49fd29054",
	"817a91b1b04645e8fef17060965b354dd12ba74b4cfcf2043d961f2e488ad521",
	"2d13867b6f9c840b9554b417cf70e07af1c9f021baf4038a67b566e7e0126317",
	"fc82221ae7cff8982a4015c773b113a0c09aab7f25f84d1459323b14ae8e5c3f",
	"fc501463eefbbba73a774e1e1644c92d76fc19784a7663c8354192f6d1b918b8",
	"f378987f067b026fe7923c37ea11d66a2f8666e394e5661f34dee6d9cb5fc96a",
	"6a5859a092236f950374f6df5722bbaacfb4cd3e1af829eacbf51bd6786a9bce",
	"b1b2c1c7ddc14dd7c299dcfc1eec5a47c81487c81ef3945fc93615ed0fec8814",
	"0de05e45b9df33951c59ccee62ec3f38d61f0daf187f569599a9efd6e9e71fe5",
	"3a5745a05f87ddee1db68b217dc043bfa206d1c7aaa1dd0a7dd76b852a733597",
	"2a33349e7e606a8ad2e30e3c84521f9377450cf09083e162e0a9b1480ce0f972",
	"bca2b41a2b25e137c83fee346af7bd1e0f52bd560583ca07a1b42f9944c5c50b",
	"c17c025fb9ed44eae8a9d5c9df0312af5c6161bd79bd669692364fc5ecaf108a",
	"958d51602bbfbd18b2a084ba848a827c29952bfef170c936419b0922994c0589",
	"a76256b648ff4d3fc47564ca4fc0280fdcea768a1d9283cf1b218209a4bb93b5",
	"4c2d3d363fbebf75e0cd3ec423c371690d1d376a37e05805ebfdfcb09ddb7431",
	"cbea43979d680c76fa000607b8d4e50a411bfe5ba1aa18b87c58acf1175a06a6",
	"cbe6beb26479b568e5f15b50217c6c83c0ee051dc4e522b9840d8e291d6aaf46",
	"69ce07183fe5531d1a15fd7d290243fa5e494aef4f376135bc62f39a5ef909c4",
	"5f7365c0a79c6a0582c33798fadb2458dc42a2d61cc4124c4e30c68deaa39357",
	"028ed9adcd85dddad55ac8510444377925bc22404282f1d329f3fb8d6c698a48",
}

// adminPasswordDenylist is the active digest set. Tests and embedders replace
// it with SetAdminPasswordDenylist.
var adminPasswordDenylist = newAdminPasswordDenylist(defaultAdminPasswordDenylistSHA256)

func newAdminPasswordDenylist(hexDigests []string) map[[sha256.Size]byte]struct{} {
	set := make(map[[sha256.Size]byte]struct{}, len(hexDigests))
	for _, h := range hexDigests {
		raw, err := hex.DecodeString(strings.TrimSpace(h))
		if err != nil || len(raw) != sha256.Size {
			panic(fmt.Sprintf("admin password denylist: invalid sha256 digest %q", h))
		}
		var key [sha256.Size]byte
		copy(key[:], raw)
		set[key] = struct{}{}
	}
	return set
}

// SetAdminPasswordDenylist replaces the weak-password digest set (hex SHA-256
// of normalised passwords) and returns a func restoring the previous set.
// Not safe for concurrent use with ValidateAdminPassword; call it at start-up
// or in tests.
func SetAdminPasswordDenylist(hexDigests []string) (restore func()) {
	prev := adminPasswordDenylist
	adminPasswordDenylist = newAdminPasswordDenylist(hexDigests)
	return func() { adminPasswordDenylist = prev }
}

func adminPasswordDenied(normalized string) bool {
	_, denied := adminPasswordDenylist[sha256.Sum256([]byte(normalized))]
	return denied
}

// adminPasswordPlaceholders are substrings that mark an unedited template
// value (e.g. ADMIN_PASSWORD=change-me-to-something-strong).
var adminPasswordPlaceholders = []string{
	"changeme", "changethis", "replaceme", "yourpassword", "yourstrongpassword",
	"yoursecurepassword", "examplepassword", "setastrongpassword", "strongpasswordhere",
}

// ValidateAdminPassword enforces the platform-admin password policy: at least
// AdminPasswordMinLength characters, at most 72 bytes (bcrypt), not a
// denylisted/placeholder value, and not trivially low-variety.
func ValidateAdminPassword(password string) error {
	if utf8.RuneCountInString(password) < AdminPasswordMinLength {
		return ErrAdminPasswordTooShort
	}
	if len(password) > adminPasswordMaxBytes {
		return ErrAdminPasswordTooLong
	}
	normalized := normalizePasswordForDenylist(password)
	if adminPasswordDenied(normalized) {
		return ErrAdminPasswordWeak
	}
	for _, placeholder := range adminPasswordPlaceholders {
		if strings.Contains(normalized, placeholder) {
			return ErrAdminPasswordWeak
		}
	}
	distinct := map[rune]struct{}{}
	for _, r := range password {
		distinct[r] = struct{}{}
	}
	if len(distinct) < 5 {
		return ErrAdminPasswordWeak
	}
	return nil
}

func normalizePasswordForDenylist(password string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(password) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// BootstrapAdminResult reports what EnsureBootstrapAdmin did.
type BootstrapAdminResult struct {
	UserID uint
	// Created is true when a new admin user + email credential were inserted.
	Created bool
	// RolePromoted is true when an existing user was raised to role=admin
	// (always together with TookOver).
	RolePromoted bool
	// TookOver is true when an existing non-admin account was re-keyed to the
	// operator: password replaced, other sign-in methods unlinked, sessions
	// revoked (see takeOverAccountForAdminTx).
	TookOver bool
	// UnlinkedSignInMethods counts the non-email credentials removed by a
	// takeover.
	UnlinkedSignInMethods int
	// PasswordIgnored is true when the user was already a platform admin: a
	// re-run (every later boot) never rewrites an admin's password.
	PasswordIgnored bool
	// ReleasedDeadCredentials counts email/password logins for this address
	// that belonged to a soft-deleted or missing account and were removed so
	// they cannot shadow the admin's login (see releaseDeadEmailCredentialsTx).
	ReleasedDeadCredentials int
}

// EnsureBootstrapAdmin idempotently provisions an email/password platform
// admin. A missing user is created (users row role=admin, email verified +
// user_auths provider=email with a bcrypt hash) in one transaction. An
// existing platform admin is left untouched (PasswordIgnored=true), so later
// boots with ADMIN_PASSWORD still set never revert a changed password.
//
// An existing NON-admin account is never promoted as-is: whoever registered
// that email first chose its password and may have linked other sign-in
// methods, and verification proves nothing about who holds them (the operator
// may have clicked the mail). The account is instead taken over in the same
// transaction (TookOver=true) — or refused with ErrBootstrapAdminWalletLinked
// when a wallet identity is attached.
//
// Whenever it writes a credential it first removes email/password logins for
// the same address left by soft-deleted or missing accounts (they would
// otherwise shadow the new one), and refuses with ErrAdminEmailCredentialInUse
// when a different live account holds one.
//
// On Postgres the transaction holds a pg_advisory_xact_lock so concurrent
// callers serialize; a lost insert race on another engine falls back to the
// existing-user path.
func EnsureBootstrapAdmin(ctx context.Context, db *gorm.DB, email, password string) (BootstrapAdminResult, error) {
	email = NormalizeEmail(email)
	if !ValidateEmail(email) {
		return BootstrapAdminResult{}, ErrAdminEmailInvalid
	}
	if err := ValidateAdminPassword(password); err != nil {
		return BootstrapAdminResult{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return BootstrapAdminResult{}, fmt.Errorf("hash admin password: %w", err)
	}

	result, err := ensureBootstrapAdminTx(ctx, db, email, hash)
	if err != nil && database.IsUniqueConstraintError(err) {
		// Another provisioner committed the same email between our read and
		// insert (only reachable without the Postgres advisory lock). Re-run:
		// the second pass sees the row and takes the promote-only path.
		result, err = ensureBootstrapAdminTx(ctx, db, email, hash)
	}
	return result, err
}

func ensureBootstrapAdminTx(ctx context.Context, db *gorm.DB, email, hash string) (BootstrapAdminResult, error) {
	var result BootstrapAdminResult
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockBootstrapAdmin(tx); err != nil {
			return err
		}
		existing, found, err := findActiveUserByEmail(tx, email)
		if err != nil {
			return err
		}
		if found && existing.Role == string(structs.RoleAdmin) {
			result.UserID = existing.ID
			result.PasswordIgnored = true
			return nil
		}
		// Both remaining branches write the address's email credential.
		released, err := releaseDeadEmailCredentialsTx(tx, email, existing.ID)
		if err != nil {
			return err
		}
		result.ReleasedDeadCredentials = released
		if found {
			result.UserID = existing.ID
			unlinked, err := takeOverAccountForAdminTx(tx, existing, email, hash)
			if err != nil {
				return err
			}
			result.RolePromoted = true
			result.TookOver = true
			result.UnlinkedSignInMethods = unlinked
			return nil
		}

		now := time.Now()
		user := database.User{
			Email:         email,
			Name:          adminDisplayName(email),
			Role:          string(structs.RoleAdmin),
			AuthMethod:    "email",
			EmailVerified: true,
			SignupSource:  "self_host_admin",
			ActivatedAt:   &now,
			CreatedAt:     now,
			UpdatedAt:     now,
			NotificationPreferences: structs.NotificationPreferences{
				EmailEnabled:         true,
				UpdatesEnabled:       true,
				TransactionalEnabled: true,
				SecurityEnabled:      true,
				ReportsEnabled:       true,
				StatisticsEnabled:    true,
			},
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if err := tx.Create(&UserAuth{
			UserID:         user.ID,
			Provider:       "email",
			ProviderUserID: email,
			PasswordHash:   hash,
			EmailVerified:  true,
			CreatedAt:      now,
			UpdatedAt:      now,
		}).Error; err != nil {
			return fmt.Errorf("create admin email credential: %w", err)
		}
		result.UserID = user.ID
		result.Created = true
		return nil
	})
	if err != nil {
		return BootstrapAdminResult{}, err
	}
	return result, nil
}

// SetAdminPasswordResult reports what SetEmailPassword changed.
type SetAdminPasswordResult struct {
	UserID uint
	// CredentialCreated is true when the user had no email credential (e.g. a
	// Google-only account) and one was added.
	CredentialCreated bool
	// Addresses are wallet addresses linked to the user, for session revocation.
	Addresses []string
	// ReleasedDeadCredentials counts email/password logins for this address
	// left by soft-deleted or missing accounts that were removed, because they
	// shadowed this user's login.
	ReleasedDeadCredentials int
}

// SetEmailPassword sets (or adds) the email/password credential of an existing
// active user and marks the email verified on both the credential and the
// user, so the operator can sign in immediately. Pending reset/verification
// tokens are cleared, and dead logins for the same address that would shadow
// it are removed (ErrAdminEmailCredentialInUse when a different live account
// holds one). It enforces the admin password policy. Callers revoke sessions
// afterwards.
func SetEmailPassword(ctx context.Context, db *gorm.DB, email, password string) (SetAdminPasswordResult, error) {
	email = NormalizeEmail(email)
	if !ValidateEmail(email) {
		return SetAdminPasswordResult{}, ErrAdminEmailInvalid
	}
	if err := ValidateAdminPassword(password); err != nil {
		return SetAdminPasswordResult{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return SetAdminPasswordResult{}, fmt.Errorf("hash password: %w", err)
	}
	var result SetAdminPasswordResult
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockBootstrapAdmin(tx); err != nil {
			return err
		}
		user, found, err := findActiveUserByEmail(tx, email)
		if err != nil {
			return err
		}
		if !found {
			return ErrAdminUserNotFound
		}
		result.UserID = user.ID
		now := time.Now()

		released, err := releaseDeadEmailCredentialsTx(tx, email, user.ID)
		if err != nil {
			return err
		}
		result.ReleasedDeadCredentials = released
		created, err := setVerifiedEmailCredentialTx(tx, user.ID, email, hash, now)
		if err != nil {
			return err
		}
		result.CredentialCreated = created
		if !user.EmailVerified {
			if err := tx.Model(&database.User{}).Where("id = ?", user.ID).
				Updates(map[string]interface{}{"email_verified": true, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		if addr := strings.TrimSpace(user.Address); addr != "" {
			result.Addresses = append(result.Addresses, addr)
		}
		var wallets []string
		if err := tx.Model(&UserAuth{}).Where("user_id = ? AND wallet_address <> ''", user.ID).
			Pluck("wallet_address", &wallets).Error; err == nil {
			result.Addresses = append(result.Addresses, wallets...)
		}
		return nil
	})
	if err != nil {
		return SetAdminPasswordResult{}, err
	}
	return result, nil
}

// takeOverAccountForAdminTx re-keys an existing non-admin account to the
// operator inside the provisioning transaction:
//
//   - refuses (ErrBootstrapAdminWalletLinked, nothing written) when a wallet
//     identity is attached — wallet sign-in resolves users.address,
//     which may also be the only owner key of the account's restaurants;
//   - unlinks every other non-email credential (Google, …), since a session on
//     an unverified account can link any Google identity;
//   - sets the email credential to hash, verified, with pending reset and
//     verification tokens cleared;
//   - marks the user verified, email-authenticated and role=admin;
//   - revokes every operator session and clears the password-login lockout.
//
// It returns how many credentials were unlinked.
func takeOverAccountForAdminTx(tx *gorm.DB, user database.User, email, hash string) (int, error) {
	if strings.TrimSpace(user.Address) != "" {
		return 0, ErrBootstrapAdminWalletLinked
	}
	var wallets int64
	if err := tx.Model(&UserAuth{}).
		Where("user_id = ? AND (provider = ? OR wallet_address <> '')", user.ID, string(AuthProviderWallet)).
		Count(&wallets).Error; err != nil {
		return 0, fmt.Errorf("inspect linked wallets: %w", err)
	}
	if wallets > 0 {
		return 0, ErrBootstrapAdminWalletLinked
	}

	now := time.Now()
	unlink := tx.Where("user_id = ? AND provider <> ?", user.ID, "email").Delete(&UserAuth{})
	if unlink.Error != nil {
		return 0, fmt.Errorf("unlink other sign-in methods: %w", unlink.Error)
	}
	if _, err := setVerifiedEmailCredentialTx(tx, user.ID, email, hash, now); err != nil {
		return 0, err
	}
	if err := tx.Model(&database.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"role":           string(structs.RoleAdmin),
		"email_verified": true,
		"auth_method":    "email",
		"google_id":      "",
		"updated_at":     now,
	}).Error; err != nil {
		return 0, fmt.Errorf("promote existing user to admin: %w", err)
	}
	if err := session.NewStore(tx).RevokeAllLinkedOperatorSessions(user.ID, nil, session.RevocationReasonSecurityReset); err != nil {
		return 0, fmt.Errorf("revoke existing sessions: %w", err)
	}
	if err := auththrottle.New(tx, auththrottle.Config{}).Clear(email, PasswordLoginThrottleKind); err != nil {
		return 0, fmt.Errorf("clear login lockout: %w", err)
	}
	return int(unlink.RowsAffected), nil
}

// setVerifiedEmailCredentialTx points the user's email credential at hash and
// marks it verified, clearing pending reset/verification tokens; it inserts
// the credential when the user has none (Google-only accounts). It reports
// whether a credential was created.
func setVerifiedEmailCredentialTx(tx *gorm.DB, userID uint, email, hash string, now time.Time) (bool, error) {
	var creds int64
	if err := tx.Model(&UserAuth{}).Where("user_id = ? AND provider = ?", userID, "email").Count(&creds).Error; err != nil {
		return false, err
	}
	if creds == 0 {
		if err := tx.Create(&UserAuth{
			UserID: userID, Provider: "email", ProviderUserID: email, PasswordHash: hash,
			EmailVerified: true, CreatedAt: now, UpdatedAt: now,
		}).Error; err != nil {
			return false, fmt.Errorf("create email credential: %w", err)
		}
		return true, nil
	}
	if err := tx.Model(&UserAuth{}).Where("user_id = ? AND provider = ?", userID, "email").
		Updates(map[string]interface{}{
			"password_hash":       hash,
			"provider_user_id":    email,
			"email_verified":      true,
			"reset_token":         "",
			"reset_expiry":        nil,
			"verification_token":  "",
			"verification_expiry": nil,
			"updated_at":          now,
		}).Error; err != nil {
		return false, fmt.Errorf("update email credential: %w", err)
	}
	return false, nil
}

// Holder states of an email credential, as classified by
// releaseDeadEmailCredentialsTx.
const (
	credentialHolderMissing     = 0
	credentialHolderLive        = 1
	credentialHolderSoftDeleted = 2
)

// releaseDeadEmailCredentialsTx makes sure the email credential about to be
// written for keepUserID (0: a user not created yet) is the only one the
// login and password-reset lookups can resolve for email. Those lookups match
// provider=email by address and take the oldest row, and nothing in the schema
// stops a second row: account deletion only soft-deletes the user and keeps
// its credential, and user_auths has no foreign key to users.
//
// Credentials for the address held by a soft-deleted or missing user are
// deleted — they can never sign in (a soft-deleted account is refused at
// login) and that account can no longer be restored under this address once
// a live account uses it. A credential held by a different LIVE user is never
// touched: ErrAdminEmailCredentialInUse, nothing written. It returns how many
// credentials were removed.
func releaseDeadEmailCredentialsTx(tx *gorm.DB, email string, keepUserID uint) (int, error) {
	var holders []struct {
		ID          uint
		HolderState int
	}
	// Literal state codes (not bind parameters): Postgres would type untyped
	// CASE results as text.
	q := tx.Table("user_auths").
		Select(fmt.Sprintf("user_auths.id AS id, CASE WHEN users.id IS NULL THEN %d WHEN users.deleted_at IS NULL THEN %d ELSE %d END AS holder_state",
			credentialHolderMissing, credentialHolderLive, credentialHolderSoftDeleted)).
		Joins("LEFT JOIN users ON users.id = user_auths.user_id").
		Where("user_auths.provider = ? AND LOWER(TRIM(user_auths.provider_user_id)) = ?", "email", email)
	if keepUserID != 0 {
		q = q.Where("(user_auths.user_id IS NULL OR user_auths.user_id <> ?)", keepUserID)
	}
	if err := q.Scan(&holders).Error; err != nil {
		return 0, fmt.Errorf("inspect existing email logins: %w", err)
	}
	dead := make([]uint, 0, len(holders))
	for _, h := range holders {
		if h.HolderState == credentialHolderLive {
			return 0, ErrAdminEmailCredentialInUse
		}
		dead = append(dead, h.ID)
	}
	if len(dead) == 0 {
		return 0, nil
	}
	if err := tx.Where("id IN ?", dead).Delete(&UserAuth{}).Error; err != nil {
		return 0, fmt.Errorf("remove email logins of deleted accounts: %w", err)
	}
	return len(dead), nil
}

func lockBootstrapAdmin(tx *gorm.DB) error {
	if tx.Dialector == nil || tx.Dialector.Name() != "postgres" {
		return nil
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", bootstrapAdminAdvisoryLockKey).Error; err != nil {
		return fmt.Errorf("bootstrap admin advisory lock: %w", err)
	}
	return nil
}

func findActiveUserByEmail(tx *gorm.DB, email string) (database.User, bool, error) {
	// Find+Limit (not Take) so a first boot does not log "record not found".
	var users []database.User
	if err := tx.Where("LOWER(TRIM(email)) = ? AND deleted_at IS NULL", email).Order("id ASC").Limit(1).Find(&users).Error; err != nil {
		return database.User{}, false, err
	}
	if len(users) == 0 {
		return database.User{}, false, nil
	}
	return users[0], true, nil
}

func adminDisplayName(email string) string {
	if at := strings.IndexByte(email, '@'); at > 0 {
		return email[:at]
	}
	return "Admin"
}
