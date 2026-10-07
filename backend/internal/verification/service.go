// Package verification owns the canonical operator email-verification state.
// Both HTTP authentication and refresh use this package so a legacy JWT cannot
// outlive the account's current verification state.
package verification

import (
	"errors"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"gorm.io/gorm"
)

var (
	ErrTokenInvalid = errors.New("invalid verification token")
	ErrTokenExpired = errors.New("expired verification token")
	// ErrUnprovenEmailAccount means an existing account holds an
	// email/password credential nobody proved by clicking a verification link,
	// and the account has already been used (it held a session, or owns a
	// workspace).
	// Another identity that proves the same address (a verified Google email)
	// must not be merged into it: the unproven password may belong to someone
	// who squatted the address while EMAIL_VERIFICATION was off.
	ErrUnprovenEmailAccount = errors.New("account email ownership is unproven")
)

// EmailReservationTTL is the maximum lifetime of an untouched email/password
// registration placeholder. Resending verification proves continued activity
// and rotates the expiry; otherwise the email becomes reusable after 24 hours.
const EmailReservationTTL = 24 * time.Hour

type emailAuthState struct {
	ID                 uint
	UserID             uint
	Provider           string
	ProviderUserID     string
	EmailVerified      bool
	VerificationToken  string
	VerificationExpiry *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (emailAuthState) TableName() string { return "user_auths" }

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

type VerifiedIdentity struct {
	User                database.User
	RegistrationLatency time.Duration
}

// VerifyEmail atomically consumes one active email-verification token and
// converges users + user_auths to verified state. The conditional auth update
// is the single-use claim: concurrent or replayed requests affect zero rows and
// cannot issue a second normal session.
func (s *Service) VerifyEmail(tokenHash string, now time.Time) (*VerifiedIdentity, error) {
	if s == nil || s.db == nil || strings.TrimSpace(tokenHash) == "" {
		return nil, ErrTokenInvalid
	}

	var result VerifiedIdentity
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var auth emailAuthState
		if err := tx.Where("provider = ? AND verification_token = ?", "email", tokenHash).First(&auth).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTokenInvalid
			}
			return err
		}
		if auth.EmailVerified || auth.VerificationToken == "" {
			return ErrTokenInvalid
		}
		if auth.VerificationExpiry == nil || !now.Before(*auth.VerificationExpiry) {
			return ErrTokenExpired
		}

		var user database.User
		if err := tx.First(&user, auth.UserID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTokenInvalid
			}
			return err
		}
		// The token was issued to ProviderUserID. A later canonical email
		// change must rotate verification rather than letting the old inbox
		// authorize the new identity.
		if strings.TrimSpace(user.Email) == "" || !strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(auth.ProviderUserID)) {
			return ErrTokenInvalid
		}

		claim := tx.Model(&emailAuthState{}).
			Where("id = ? AND provider = ? AND email_verified = ? AND verification_token = ? AND verification_expiry > ?",
				auth.ID, "email", false, tokenHash, now).
			Updates(map[string]interface{}{
				"email_verified":      true,
				"verification_token":  "",
				"verification_expiry": nil,
				"updated_at":          now,
			})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return ErrTokenInvalid
		}

		userUpdate := tx.Model(&database.User{}).
			Where("id = ? AND LOWER(email) = LOWER(?)", user.ID, auth.ProviderUserID).
			Updates(map[string]interface{}{"email_verified": true, "updated_at": now})
		if userUpdate.Error != nil {
			return userUpdate.Error
		}
		if userUpdate.RowsAffected != 1 {
			return ErrTokenInvalid
		}

		user.EmailVerified = true
		user.UpdatedAt = now
		result.User = user
		if !auth.CreatedAt.IsZero() && now.After(auth.CreatedAt) {
			result.RegistrationLatency = now.Sub(auth.CreatedAt)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// IsOperatorVerified reads the live provider verification state for an
// operator session. Email/register sessions also require the user's current
// email to remain bound to the email auth row. Other session realms (staff,
// customer, web3) are outside this boundary.
//
// When EMAIL_VERIFICATION resolves to "off" an email/register session needs
// only that binding: off mode never writes email_verified=true (the flags stay
// a truthful record of proven ownership), so the verified predicates are
// dropped instead. Google sessions always need Google's verified flag.
func (s *Service) IsOperatorVerified(userID uint, sessionProvider string) (bool, error) {
	if userID == 0 {
		return false, nil
	}
	provider := strings.ToLower(strings.TrimSpace(sessionProvider))
	authProvider := ""
	switch provider {
	case "email", "register":
		authProvider = "email"
	case "google":
		authProvider = "google"
	default:
		return true, nil
	}

	// One WHERE per shape: this runs on every authenticated email/register
	// request, and each extra chained Where costs allocations (see
	// BenchmarkIsOperatorVerified, docs/performance/oss-email.md).
	query := s.db.Table("user_auths AS ua").
		Joins("JOIN users AS u ON u.id = ua.user_id")
	switch {
	case authProvider == "email" && config.EmailVerificationRequired():
		query = query.Where("ua.user_id = ? AND ua.provider = ? AND u.deleted_at IS NULL AND ua.email_verified = ? AND u.email_verified = ? AND LOWER(u.email) = LOWER(ua.provider_user_id)",
			userID, authProvider, true, true)
	case authProvider == "email":
		query = query.Where("ua.user_id = ? AND ua.provider = ? AND u.deleted_at IS NULL AND LOWER(u.email) = LOWER(ua.provider_user_id)",
			userID, authProvider)
	default:
		query = query.Where("ua.user_id = ? AND ua.provider = ? AND u.deleted_at IS NULL AND ua.email_verified = ?",
			userID, authProvider, true)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// DropUnprovenEmailCredential prepares userID for a merge by an identity that
// proved the account's address out of band (a verified Google email). An
// unproven email/password credential on a pristine account — no session, no
// workspace, no other sign-in
// identity, no linked wallet — is deleted, so
// whoever chose that password cannot sign in to the merged account; the prover
// keeps the account. Any other account with an unproven credential returns
// ErrUnprovenEmailAccount and changes nothing: what it holds may have been
// planted by whoever chose the password, and handing it to the prover is the
// pre-account takeover this guard exists to stop. Accounts with no unproven
// credential are left untouched and return nil.
func (s *Service) DropUnprovenEmailCredential(userID uint) error {
	if s == nil || s.db == nil || userID == 0 {
		return ErrUnprovenEmailAccount
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var unproven []emailAuthState
		if err := tx.Where("user_id = ? AND provider = ? AND email_verified = ?", userID, "email", false).
			Find(&unproven).Error; err != nil {
			return err
		}
		if len(unproven) == 0 {
			return nil
		}
		used, err := accountWasUsed(tx, userID)
		if err != nil {
			return err
		}
		if used {
			return ErrUnprovenEmailAccount
		}
		var otherIdentities int64
		if err := tx.Model(&emailAuthState{}).
			Where("user_id = ? AND NOT (provider = ? AND email_verified = ?)", userID, "email", false).
			Count(&otherIdentities).Error; err != nil {
			return err
		}
		if otherIdentities != 0 {
			return ErrUnprovenEmailAccount
		}
		var user database.User
		if err := tx.Select("id", "address", "google_id").
			First(&user, userID).Error; err != nil {
			return err
		}
		if strings.TrimSpace(user.Address) != "" || strings.TrimSpace(user.GoogleID) != "" {
			return ErrUnprovenEmailAccount
		}
		ids := make([]uint, 0, len(unproven))
		for _, row := range unproven {
			ids = append(ids, row.ID)
		}
		return tx.Where("id IN ? AND user_id = ? AND provider = ? AND email_verified = ?", ids, userID, "email", false).
			Delete(&emailAuthState{}).Error
	})
}

// accountWasUsed reports whether userID ever held a session or owns a
// workspace. Tables absent from a trimmed test schema fall back to counting
// conservatively.
func accountWasUsed(tx *gorm.DB, userID uint) (bool, error) {
	migrator := tx.Migrator()
	if migrator.HasTable("user_sessions") {
		var sessions int64
		if err := tx.Table("user_sessions").Where("user_id = ?", userID).Count(&sessions).Error; err != nil {
			return false, err
		}
		if sessions != 0 {
			return true, nil
		}
	}
	if !migrator.HasTable("businesses") {
		return false, nil
	}
	var owned int64
	if err := tx.Table("businesses").Where("user_id = ?", userID).Count(&owned).Error; err != nil {
		return false, err
	}
	return owned != 0, nil
}

// ReleaseExpiredEmailReservation removes only an expired, email-only,
// unverified placeholder with no workspace or session history. It never claims
// or revives the old identity: a successful retry creates a fresh user/auth
// pair and a fresh password hash. A platform admin row is never a placeholder,
// whatever its verification state.
func (s *Service) ReleaseExpiredEmailReservation(email string, now time.Time) (bool, error) {
	normalized := strings.TrimSpace(strings.ToLower(email))
	if normalized == "" {
		return false, nil
	}

	released := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var user database.User
		if err := tx.Where("LOWER(email) = ?", normalized).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if user.EmailVerified || user.DeletedAt != nil || user.CreatedAt.After(now.Add(-EmailReservationTTL)) {
			return nil
		}
		if user.Role == string(structs.RoleAdmin) {
			return nil
		}

		var auths []emailAuthState
		if err := tx.Where("user_id = ?", user.ID).Find(&auths).Error; err != nil {
			return err
		}
		if len(auths) != 1 {
			return nil
		}
		auth := auths[0]
		if auth.Provider != "email" || auth.EmailVerified || auth.VerificationExpiry == nil || now.Before(*auth.VerificationExpiry) {
			return nil
		}

		used, err := accountWasUsed(tx, user.ID)
		if err != nil {
			return err
		}
		if used {
			return nil
		}

		if err := tx.Where("id = ? AND email_verified = ?", auth.ID, false).Delete(&emailAuthState{}).Error; err != nil {
			return err
		}
		deleteUser := tx.Where("id = ? AND email_verified = ?", user.ID, false).Delete(&database.User{})
		if deleteUser.Error != nil {
			return deleteUser.Error
		}
		if deleteUser.RowsAffected != 1 {
			return ErrTokenInvalid
		}
		released = true
		return nil
	})
	return released, err
}
