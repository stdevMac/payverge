package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/verification"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUserExists          = errors.New("user with this email already exists")
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrUserNotFound        = errors.New("user not found")
	ErrTokenExpired        = errors.New("token has expired")
	ErrTokenInvalid        = errors.New("invalid token")
	ErrEmailNotVerified    = errors.New("email not verified")
	ErrWalletAlreadyLinked = errors.New("wallet already linked to another account")
	ErrProviderNotLinked   = errors.New("this provider is not linked to your account")
	// ErrUnverifiedOAuthEmail is returned when an OAuth provider reports the
	// account's email as unverified and the requested operation would merge that
	// identity into a pre-existing account by email — the account-takeover path.
	// Callers must fail closed (reject the callback) rather than merge or create
	// a colliding account.
	ErrUnverifiedOAuthEmail = errors.New("oauth email is not verified")
	// ErrArchivedAccount is returned when an existing operator identity is
	// scheduled for deletion. Providers must fail closed instead of signing
	// the user in or mutating the archived row.
	ErrArchivedAccount = errors.New("this account is scheduled for deletion")
	// ErrEmailIdentityUnbound is returned when EMAIL_VERIFICATION is off and
	// the email auth row used to sign in is no longer bound to the account's
	// current email. Off mode only vouches for the address the account holds
	// now, so the stale identity is refused (403), never signed in.
	ErrEmailIdentityUnbound = errors.New("email identity is not bound to the account's current email")
	// ErrUnprovenEmailAccount is returned when a verified OAuth email matches
	// an existing account whose email/password credential was never proven and
	// which has already been used. Merging would let whoever chose that
	// password into the prover's account (pre-account takeover), so callers
	// refuse the merge (409) instead.
	ErrUnprovenEmailAccount = errors.New("an account with this email exists but its email was never verified")
)

// AuthService handles authentication operations
type AuthService struct {
	db                   *gorm.DB
	verificationLimiter  *RateLimiter
	passwordResetLimiter *RateLimiter
}

// NewAuthService creates a new auth service
func NewAuthService(db *gorm.DB) *AuthService {
	return &AuthService{
		db:                   db,
		verificationLimiter:  NewRateLimiter(time.Hour, 3),
		passwordResetLimiter: NewRateLimiter(time.Hour, 3),
	}
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword compares a password with a hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// passwordMatches is the compare LoginWithEmail uses. Tests replace it to
// prove a missing account still pays for a compare, so response timing does
// not reveal whether the email is registered.
var passwordMatches = CheckPassword

var (
	dummyPasswordOnce sync.Once
	dummyPassword     string
)

// dummyPasswordHash returns a real bcrypt hash so a login for an unknown
// email spends the same compare as a wrong password. The plaintext is a
// fixed throwaway and is never stored as a user password.
func dummyPasswordHash() string {
	dummyPasswordOnce.Do(func() {
		// A fixed 27-byte input at DefaultCost cannot fail; on the
		// impossible error the compare runs against "" and still rejects.
		hash, _ := bcrypt.GenerateFromPassword([]byte("payverge-login-timing-dummy"), bcrypt.DefaultCost)
		dummyPassword = string(hash)
	})
	return dummyPassword
}

// GenerateToken generates a random token
func GenerateToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// RegisterWithEmail registers a new user with email/password. It returns the
// unsaved auth record and the PLAINTEXT verification token the caller must email
// to the user. The token is persisted only as a hash (see VerificationToken
// below), so the plaintext exists solely in this return value — a DB read cannot
// recover a usable token.
//
// When EMAIL_VERIFICATION resolves to "off" (config.EmailVerificationMode), the
// auth record carries no token and the returned token is "". It is still
// created UNVERIFIED: email_verified records proven ownership of the address,
// and off mode proves nothing, so the flag stays false. The caller signs the
// account in anyway (off mode does not gate sessions on the flag), and a later
// switch to "required" asks the account to verify like any pending one.
func (s *AuthService) RegisterWithEmail(email, password, name string) (*UserAuth, string, error) {
	// Check if user exists
	var existingAuth UserAuth
	if err := s.db.Where("provider = ? AND LOWER(provider_user_id) = LOWER(?)", "email", email).First(&existingAuth).Error; err == nil {
		return nil, "", ErrUserExists
	}

	// Hash password
	hashedPassword, err := HashPassword(password)
	if err != nil {
		return nil, "", fmt.Errorf("failed to hash password: %w", err)
	}

	if !config.EmailVerificationRequired() {
		now := time.Now()
		return &UserAuth{
			Provider:       string(AuthProviderEmail),
			ProviderUserID: email,
			PasswordHash:   hashedPassword,
			EmailVerified:  false,
			CreatedAt:      now,
			UpdatedAt:      now,
		}, "", nil
	}

	// Generate verification token
	verificationToken, err := GenerateToken(32)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate verification token: %w", err)
	}

	// Create user first (we need the user ID)
	// This will be handled by the caller who creates the user record
	// For now, we return the auth record without user_id set

	verificationExpiry := time.Now().Add(24 * time.Hour)
	auth := &UserAuth{
		Provider:           string(AuthProviderEmail),
		ProviderUserID:     email, // Use email as provider user ID for email auth
		PasswordHash:       hashedPassword,
		EmailVerified:      false,
		VerificationToken:  HashToken(verificationToken), // store hash; email the plaintext
		VerificationExpiry: &verificationExpiry,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	return auth, verificationToken, nil
}

// LoginWithEmail authenticates a user with email/password
func (s *AuthService) LoginWithEmail(email, password string) (*UserAuth, error) {
	var auth UserAuth
	if err := s.db.Where("provider = ? AND LOWER(provider_user_id) = LOWER(?)", "email", email).First(&auth).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = passwordMatches(password, dummyPasswordHash())
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Password is checked before email verification so an unverified account
	// with a wrong password is indistinguishable from a missing account.
	if !passwordMatches(password, auth.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	// Check email verification. With EMAIL_VERIFICATION=off (or auto on a
	// box whose EMAIL_PROVIDER is log) an unverified record is let through
	// unchanged: off mode signs it in without ever marking it verified, and
	// the session middleware's live check applies the same rule.
	if !auth.EmailVerified && config.EmailVerificationRequired() {
		return nil, ErrEmailNotVerified
	}

	return &auth, nil
}

// RotateVerificationToken always generates and persists a new verification
// token for an unverified email-auth account, invalidating any previously
// issued one. Used by the resend-verification flow where the user's
// implicit signal is "the previous link is unusable."
func (s *AuthService) RotateVerificationToken(email string) (string, error) {
	var auth UserAuth
	if err := s.db.Where("provider = ? AND LOWER(provider_user_id) = LOWER(?)", "email", email).First(&auth).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrUserNotFound
		}
		return "", err
	}

	if auth.EmailVerified {
		return "", nil
	}

	verificationToken, err := GenerateToken(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate verification token: %w", err)
	}
	verificationExpiry := time.Now().Add(24 * time.Hour)

	auth.VerificationToken = HashToken(verificationToken) // store hash; return plaintext
	auth.VerificationExpiry = &verificationExpiry
	auth.UpdatedAt = time.Now()

	if err := s.db.Save(&auth).Error; err != nil {
		return "", err
	}

	return verificationToken, nil
}

// ResendVerification unconditionally rotates the verification token for an
// unverified email-auth account and returns the new token to dispatch,
// invalidating any previously issued link. Returns ("", nil) silently when
// rate-limited, when the account is unknown, or when the account is already
// verified — preventing account enumeration.
func (s *AuthService) ResendVerification(email string) (string, error) {
	if !s.verificationLimiter.Allow(email) {
		return "", nil
	}
	token, err := s.RotateVerificationToken(email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return "", nil
		}
		return "", err
	}
	return token, nil
}

// CreateOrUpdateOAuthUser creates or updates a user from OAuth.
//
// emailVerified must reflect whether the OAuth provider actually verified the
// email (e.g. Google's "verified_email" flag). It is NOT safe to assume OAuth
// emails are verified — Google Workspace and some providers return unverified
// addresses, which is an account-takeover vector when emails are merged. The
// caller is responsible for passing the provider's real verified flag (and for
// rejecting/gating unverified emails before any email-based account merge).
func (s *AuthService) CreateOrUpdateOAuthUser(provider, providerUserID, email, name string, emailVerified bool) (*UserAuth, bool, error) {
	var auth UserAuth
	isNewUser := false

	err := s.db.Where("provider = ? AND provider_user_id = ?", provider, providerUserID).First(&auth).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// New user. Provider tokens stay in the callback's memory; they are
			// not persisted.
			isNewUser = true
			auth = UserAuth{
				Provider:       provider,
				ProviderUserID: providerUserID,
				EmailVerified:  emailVerified, // set from the provider's real verified flag — never hardcode
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}
		} else {
			return nil, false, err
		}
	} else {
		auth.UpdatedAt = time.Now()
	}

	return &auth, isNewUser, nil
}

// LinkWallet links a wallet address to a user
func (s *AuthService) LinkWallet(userID uint, walletAddress string) error {
	// Check if wallet is already linked to another user
	var existingAuth UserAuth
	if err := s.db.Where("wallet_address = ? AND user_id != ?", walletAddress, userID).First(&existingAuth).Error; err == nil {
		return ErrWalletAlreadyLinked
	}

	// Update or create wallet auth record
	var auth UserAuth
	err := s.db.Where("user_id = ? AND provider = ?", userID, "wallet").First(&auth).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create new wallet auth
			auth = UserAuth{
				UserID:        userID,
				Provider:      string(AuthProviderWallet),
				WalletAddress: walletAddress,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			return s.db.Create(&auth).Error
		}
		return err
	}

	// Update existing wallet auth
	auth.WalletAddress = walletAddress
	auth.UpdatedAt = time.Now()
	return s.db.Save(&auth).Error
}

// GetUserWallet gets the linked wallet for a user
func (s *AuthService) GetUserWallet(userID uint) (string, error) {
	var auth UserAuth
	if err := s.db.Where("user_id = ? AND wallet_address != ''", userID).First(&auth).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil // No wallet linked
		}
		return "", err
	}
	return auth.WalletAddress, nil
}

// VerifyEmailIdentity atomically converges users + user_auths and returns the
// canonical user that may receive the first normal operator session.
func (s *AuthService) VerifyEmailIdentity(token string) (*verification.VerifiedIdentity, error) {
	identity, err := verification.NewService(s.db).VerifyEmail(HashToken(token), time.Now())
	if errors.Is(err, verification.ErrTokenInvalid) {
		return nil, ErrTokenInvalid
	}
	if errors.Is(err, verification.ErrTokenExpired) {
		return nil, ErrTokenExpired
	}
	return identity, err
}

// RequestPasswordReset initiates a password reset.
// Returns ("", nil) when per-email rate-limited so handlers keep a uniform
// anti-enumeration response and do not rotate/invalidate the prior token.
func (s *AuthService) RequestPasswordReset(email string) (string, error) {
	if !s.passwordResetLimiter.Allow(email) {
		return "", nil
	}
	return s.issuePasswordResetToken(email)
}

// RequestPasswordResetForAdmin is the operator-console path: it skips the
// public per-email limiter so a support-initiated reset is not blocked by
// prior self-service abuse against the same inbox.
func (s *AuthService) RequestPasswordResetForAdmin(email string) (string, error) {
	return s.issuePasswordResetToken(email)
}

func (s *AuthService) issuePasswordResetToken(email string) (string, error) {
	var auth UserAuth
	if err := s.db.Where("provider = ? AND LOWER(provider_user_id) = LOWER(?)", "email", email).First(&auth).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrUserNotFound
		}
		return "", err
	}

	resetToken, err := GenerateToken(32)
	if err != nil {
		return "", err
	}

	resetExpiry := time.Now().Add(1 * time.Hour)
	auth.ResetToken = HashToken(resetToken) // store hash; email the plaintext
	auth.ResetExpiry = &resetExpiry
	auth.UpdatedAt = time.Now()

	if err := s.db.Save(&auth).Error; err != nil {
		return "", err
	}

	return resetToken, nil
}

// ResetPassword resets a user's password
// ResetPassword validates the reset token, sets the new password hash, and
// returns the affected user's ID so the caller can revoke that user's existing
// sessions (a password reset must invalidate any sessions/refresh tokens an
// attacker may hold).
func (s *AuthService) ResetPassword(token, newPassword string) (uint, error) {
	if strings.TrimSpace(token) == "" {
		return 0, ErrTokenInvalid
	}
	// Tokens are stored hashed; match on the hash of the presented plaintext.
	tokenHash := HashToken(token)

	// Cheap pre-check so an unknown or expired token is refused without
	// paying for bcrypt. It decides nothing on the success path: the
	// conditional UPDATE below is the only thing that consumes the token.
	var pending UserAuth
	if err := s.db.Select("id", "reset_expiry").Where("reset_token = ?", tokenHash).First(&pending).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrTokenInvalid
		}
		return 0, err
	}
	if pending.ResetExpiry != nil && time.Now().After(*pending.ResetExpiry) {
		return 0, ErrTokenExpired
	}

	hashedPassword, err := HashPassword(newPassword)
	if err != nil {
		return 0, err
	}
	if resetPasswordAfterLookupHook != nil {
		resetPasswordAfterLookupHook()
	}

	// Consume the token and set the password in one statement. The old
	// SELECT-then-Save let two concurrent requests with the same token both
	// succeed (the later one silently overwriting the earlier password) and
	// rewrote every column of the row from a stale read.
	now := time.Now()
	var claimed []struct{ UserID uint }
	if err := s.db.Raw(
		`UPDATE user_auths
		    SET password_hash = ?, reset_token = '', reset_expiry = NULL, updated_at = ?
		  WHERE id = ? AND reset_token = ? AND (reset_expiry IS NULL OR reset_expiry > ?)
		RETURNING user_id`,
		hashedPassword, now, pending.ID, tokenHash, now,
	).Scan(&claimed).Error; err != nil {
		return 0, err
	}
	if len(claimed) == 0 {
		// Consumed by a concurrent reset, or expired since the pre-check.
		return 0, ErrTokenInvalid
	}
	return claimed[0].UserID, nil
}

// resetPasswordAfterLookupHook is a test seam that runs between the token
// pre-check and the consuming UPDATE in ResetPassword.
var resetPasswordAfterLookupHook func()

// GetAuthByUserID gets all auth methods for a user
func (s *AuthService) GetAuthByUserID(userID uint) ([]UserAuth, error) {
	var auths []UserAuth
	if err := s.db.Where("user_id = ?", userID).Find(&auths).Error; err != nil {
		return nil, err
	}
	return auths, nil
}

// SaveAuth saves an auth record
func (s *AuthService) SaveAuth(auth *UserAuth) error {
	return s.db.Save(auth).Error
}

// CreateAuth creates a new auth record
func (s *AuthService) CreateAuth(auth *UserAuth) error {
	return s.db.Create(auth).Error
}

// UnlinkWallet removes the wallet link from a user
func (s *AuthService) UnlinkWallet(userID uint) error {
	return s.db.Where("user_id = ? AND provider = ?", userID, "wallet").Delete(&UserAuth{}).Error
}
