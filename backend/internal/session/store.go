package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GlobalStore is initialized in main.go after DB setup.
// Both the auth and server packages reference this to avoid import cycles.
var GlobalStore *Store

// ErrRefreshHistoryTokenCollision means a replay token is already owned by a
// different session. Treat it as an integrity failure and roll back rotation.
var ErrRefreshHistoryTokenCollision = errors.New("refresh history token belongs to another session")

// UserSession represents an active user session (the runtime model for user_sessions).
// Numbered SQL migrations own the production schema; this package owns runtime operations.
type UserSession struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	UserID           *uint      `gorm:"index" json:"user_id"`
	Address          string     `gorm:"index" json:"address"`
	SessionToken     string     `gorm:"uniqueIndex;not null" json:"-"`
	Provider         string     `json:"provider"`
	IPAddress        string     `json:"ip_address"`
	UserAgent        string     `json:"user_agent"`
	Revoked          bool       `gorm:"default:false" json:"revoked"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	RevocationReason string     `json:"revocation_reason,omitempty"`
	ExpiresAt        time.Time  `json:"expires_at"`
	CreatedAt        time.Time  `json:"created_at"`
	LastUsedAt       time.Time  `json:"last_used_at"`
	RefreshToken     string     `gorm:"index" json:"-"`
	RefreshExpiresAt time.Time  `json:"-"`
	// PreviousRefreshToken holds the hash of the refresh token that was rotated
	// away on the last rotation. A request that presents an already-rotated
	// token (it lives here, not in RefreshToken) is a reuse/replay and triggers
	// session-family revocation. Indexed for the detection lookup.
	PreviousRefreshToken string `gorm:"index" json:"-"`
	// PreviousRotatedAt is when the last rotation happened. A previous-token
	// replay within RefreshReuseGracePeriod of this is treated as a benign
	// concurrent double-refresh (not theft), avoiding a false family revoke.
	PreviousRotatedAt *time.Time `json:"-"`
}

// RefreshTokenHistory durably records every refresh-token hash rotated away
// from a session. Rows share the session lifecycle through the database foreign
// key's ON DELETE CASCADE; CleanExpired also removes them explicitly in the same
// transaction for stores whose test database does not enforce foreign keys.
type RefreshTokenHistory struct {
	SessionID uint      `gorm:"primaryKey" json:"-"`
	TokenHash string    `gorm:"primaryKey;uniqueIndex" json:"-"`
	RotatedAt time.Time `gorm:"not null;index" json:"-"`
	ExpiresAt time.Time `gorm:"not null;index" json:"-"`
}

func (RefreshTokenHistory) TableName() string {
	return "user_session_refresh_history"
}

// RefreshReuseGracePeriod bounds how recently a refresh token may have been
// rotated for a replay of the rotated-away token to count as a benign concurrent
// double-refresh rather than a stolen-token reuse. The window covers the observed
// one-minute delayed browser retry while all stale tokens remain invalid.
const RefreshReuseGracePeriod = 90 * time.Second

// RevocationReason is the audit reason recorded when a live session is revoked.
type RevocationReason string

const (
	RevocationReasonUnspecified       RevocationReason = "unspecified"
	RevocationReasonUserLogout        RevocationReason = "user_logout"
	RevocationReasonRefreshTokenReuse RevocationReason = "refresh_token_reuse"
	RevocationReasonSecurityReset     RevocationReason = "security_reset"
	RevocationReasonAccountDisabled   RevocationReason = "account_disabled"
	RevocationReasonAdministrative    RevocationReason = "administrative_revocation"
	RevocationReasonInvalidSession    RevocationReason = "invalid_session"
	RevocationReasonEmailUnverified   RevocationReason = "email_unverified"
	// RevocationReasonRoleChanged marks sessions revoked because the live
	// users.role no longer matches the role the token was minted with
	// (platform-admin demotion).
	RevocationReasonRoleChanged RevocationReason = "role_changed"
)

func normalizeRevocationReason(reason RevocationReason) RevocationReason {
	switch reason {
	case RevocationReasonUnspecified,
		RevocationReasonUserLogout,
		RevocationReasonRefreshTokenReuse,
		RevocationReasonSecurityReset,
		RevocationReasonAccountDisabled,
		RevocationReasonAdministrative,
		RevocationReasonInvalidSession,
		RevocationReasonEmailUnverified,
		RevocationReasonRoleChanged:
		return reason
	default:
		return RevocationReasonUnspecified
	}
}

func revokeUpdates(reason RevocationReason, now time.Time) map[string]interface{} {
	return map[string]interface{}{
		"revoked":           true,
		"revoked_at":        now,
		"revocation_reason": normalizeRevocationReason(reason),
	}
}

// RecentlyRotated reports whether the session's last rotation is within the
// reuse grace window relative to now. Used to distinguish a benign concurrent
// refresh race (skip family revocation) from a genuine reuse (revoke).
func RecentlyRotated(sess *UserSession, now time.Time) bool {
	return sess != nil && sess.PreviousRotatedAt != nil && now.Sub(*sess.PreviousRotatedAt) < RefreshReuseGracePeriod
}

func (UserSession) TableName() string {
	return "user_sessions"
}

// Store manages UserSession persistence.
type Store struct {
	db *gorm.DB
}

// CreateInput collects the data needed to create a new session row.
type CreateInput struct {
	UserID  *uint
	Address string
	// TokenHash may be omitted while bootstrapping a session whose signed JWT
	// must contain the database-generated session ID. Create will persist a
	// cryptographically random provisional hash until UpdateTokenHash replaces it.
	TokenHash string
	Provider  string
	IPAddress string
	UserAgent string
	ExpiresAt time.Time
}

// NewStore returns a ready-to-use store backed by the given database.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Ping runs a trivial query against the sessions table so that missing
// migrations or a broken DB handle surface at startup rather than on the
// first sign-in request (where the handler would silently degrade and
// accept tokens without revocation state).
func (s *Store) Ping() error {
	var count int64
	return s.db.Model(&UserSession{}).Count(&count).Error
}

// HashToken returns the hex-encoded SHA-256 digest of a raw JWT string.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// Create persists a new session and returns the populated model (including ID).
func (s *Store) Create(input CreateInput) (*UserSession, error) {
	tokenHash := input.TokenHash
	if tokenHash == "" || tokenHash == "pending" {
		provisionalToken, err := GenerateRefreshToken()
		if err != nil {
			return nil, err
		}
		// Domain-separate the provisional value before storing it so the column
		// always contains a digest, never a usable bearer token.
		tokenHash = HashToken("provisional-session:" + provisionalToken)
	}

	sess := &UserSession{
		UserID:       input.UserID,
		Address:      strings.ToLower(strings.TrimSpace(input.Address)),
		SessionToken: tokenHash,
		Provider:     input.Provider,
		IPAddress:    input.IPAddress,
		UserAgent:    input.UserAgent,
		ExpiresAt:    input.ExpiresAt,
		LastUsedAt:   time.Now(),
	}
	if err := s.db.Create(sess).Error; err != nil {
		return nil, err
	}
	return sess, nil
}

// UpdateTokenHash replaces the stored hash for a session (used after the JWT
// is generated, since we need the session ID inside the token).
func (s *Store) UpdateTokenHash(sessionID uint, newHash string) error {
	return s.db.Model(&UserSession{}).Where("id = ?", sessionID).
		Update("session_token", newHash).Error
}

// Validate checks that a session exists, is not revoked, and the token hash
// matches. It also bumps last_used_at.
func (s *Store) Validate(sessionID uint, tokenHash string) (bool, error) {
	var sess UserSession
	if err := s.db.First(&sess, sessionID).Error; err != nil {
		return false, err
	}
	if sess.Revoked {
		return false, nil
	}
	if time.Now().After(sess.ExpiresAt) {
		return false, nil
	}
	if sess.SessionToken != tokenHash {
		return false, nil
	}
	// Best-effort update of last_used_at; ignore errors.
	_ = s.db.Model(&UserSession{}).Where("id = ?", sessionID).
		Update("last_used_at", time.Now()).Error
	return true, nil
}

// Get returns the persisted session realm and principal binding. Callers use
// it only after Validate has authenticated the token hash.
func (s *Store) Get(sessionID uint) (*UserSession, error) {
	var sess UserSession
	if err := s.db.First(&sess, sessionID).Error; err != nil {
		return nil, err
	}
	return &sess, nil
}

// RevokeWithReason marks a single live session as revoked with audit metadata.
func (s *Store) RevokeWithReason(sessionID uint, reason RevocationReason) error {
	return s.db.Model(&UserSession{}).Where("id = ? AND revoked = false", sessionID).
		Updates(revokeUpdates(reason, time.Now())).Error
}

// RevokeAllForScopedUserWithReason marks all live sessions for a provider/user pair as revoked with audit metadata.
func (s *Store) RevokeAllForScopedUserWithReason(userID uint, reason RevocationReason, providers ...string) error {
	query := s.db.Model(&UserSession{}).Where("user_id = ? AND revoked = false", userID)
	if len(providers) > 0 {
		query = query.Where("provider IN ?", providers)
	}
	return query.Updates(revokeUpdates(reason, time.Now())).Error
}

// RevokeAllLinkedOperatorSessions revokes every live operator-class session
// for a numeric user id plus production-shaped address-keyed siblings
// (web3/SIWE rows with user_id NULL). Staff and customer rows that
// merely share the numeric id are left alone.
func (s *Store) RevokeAllLinkedOperatorSessions(userID uint, addresses []string, reason RevocationReason) error {
	providers := UserSessionProviders()
	stamp := time.Now()
	updates := revokeUpdates(reason, stamp)
	if userID != 0 {
		if err := s.db.Model(&UserSession{}).
			Where("user_id = ? AND revoked = false AND provider IN ?", userID, providers).
			Updates(updates).Error; err != nil {
			return err
		}
	}
	seen := map[string]struct{}{}
	for _, raw := range addresses {
		addr := strings.ToLower(strings.TrimSpace(raw))
		if addr == "" {
			continue
		}
		if _, dup := seen[addr]; dup {
			continue
		}
		seen[addr] = struct{}{}
		if err := s.db.Model(&UserSession{}).
			Where("LOWER(address) = ? AND user_id IS NULL AND revoked = false AND provider IN ?", addr, providers).
			Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

// RevokeByTokenHashWithReason revokes the live session matching a token hash with audit metadata.
func (s *Store) RevokeByTokenHashWithReason(tokenHash string, reason RevocationReason) error {
	return s.db.Model(&UserSession{}).Where("session_token = ? AND revoked = false", tokenHash).
		Updates(revokeUpdates(reason, time.Now())).Error
}

// RevokeByRefreshTokenHashWithReason revokes the live session whose current
// refresh_token hash matches. Used by logout when only the refresh cookie is
// present (access JWT missing/expired) so stolen refresh tokens cannot outlive
// an explicit logout.
func (s *Store) RevokeByRefreshTokenHashWithReason(tokenHash string, reason RevocationReason) error {
	if tokenHash == "" {
		return nil
	}
	return s.db.Model(&UserSession{}).Where("refresh_token = ? AND revoked = false", tokenHash).
		Updates(revokeUpdates(reason, time.Now())).Error
}

// CleanExpired hard-deletes sessions that are both expired and revoked (or
// simply expired past a grace window).
func (s *Store) CleanExpired() error {
	now := time.Now()
	cutoff := now.Add(-24 * time.Hour)
	const expiredSessionPredicate = `
		(revoked = true AND (
			(revoked_at IS NOT NULL AND revoked_at < ?) OR
			(revoked_at IS NULL AND expires_at < ? AND (refresh_expires_at IS NULL OR refresh_expires_at < ?))
		)) OR
		(revoked = false AND expires_at < ? AND (refresh_expires_at IS NULL OR refresh_expires_at < ?))`
	predicateArgs := []interface{}{cutoff, cutoff, cutoff, cutoff, cutoff}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("expires_at < ?", now).Delete(&RefreshTokenHistory{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id IN (?)",
			tx.Model(&UserSession{}).Select("id").Where(expiredSessionPredicate, predicateArgs...),
		).Delete(&RefreshTokenHistory{}).Error; err != nil {
			return err
		}
		return tx.Where(expiredSessionPredicate, predicateArgs...).Delete(&UserSession{}).Error
	})
}

// GenerateRefreshToken generates a cryptographically random 64-char hex string.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ValidateRefreshToken finds a session by refresh token hash that is not expired and not revoked.
func (s *Store) ValidateRefreshToken(tokenHash string) (*UserSession, error) {
	var sess UserSession
	if err := s.db.Where("refresh_token = ? AND revoked = false AND refresh_expires_at > ?", tokenHash, time.Now()).
		First(&sess).Error; err != nil {
		return nil, err
	}
	return &sess, nil
}

// RotateRefreshToken updates the refresh token hash and expiry for a session.
// This is the LOGIN-ISSUANCE path: it sets the initial refresh token on a fresh
// session, where there is no prior token to guard against. The refresh-rotation
// path uses RotateSession (compare-and-swap) instead.
func (s *Store) RotateRefreshToken(sessionID uint, newHash string, expiresAt time.Time) error {
	return s.db.Model(&UserSession{}).Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"refresh_token":      newHash,
			"refresh_expires_at": expiresAt,
		}).Error
}

// RotateSession atomically rotates a session's refresh AND access token hashes
// in a single transactional compare-and-swap keyed on the OLD refresh hash.
// It returns matched=false (and mutates nothing) when no live row still carries
// oldRefreshHash — meaning a concurrent rotation already advanced the token, so
// this caller lost the race and must not mint a parallel valid token. The
// rotated-away hash is recorded in previous_refresh_token so a later replay of
// it is detectable as reuse (see FindByPreviousRefreshToken).
func (s *Store) RotateSession(sessionID uint, oldRefreshHash, newRefreshHash, newSessionHash string, refreshExpiresAt, sessionExpiresAt time.Time) (bool, error) {
	var matched bool
	err := s.db.Transaction(func(tx *gorm.DB) error {
		rotatedAt := time.Now()
		res := tx.Model(&UserSession{}).
			Where("id = ? AND refresh_token = ? AND revoked = false", sessionID, oldRefreshHash).
			Updates(map[string]interface{}{
				"previous_refresh_token": oldRefreshHash,
				"previous_rotated_at":    rotatedAt,
				"refresh_token":          newRefreshHash,
				"refresh_expires_at":     refreshExpiresAt,
				"session_token":          newSessionHash,
				"expires_at":             sessionExpiresAt,
			})
		if res.Error != nil {
			return res.Error
		}
		matched = res.RowsAffected > 0
		if !matched {
			return nil
		}
		historyWrite := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "token_hash"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"token_hash": gorm.Expr("excluded.token_hash"),
			}),
			Where: clause.Where{Exprs: []clause.Expression{clause.Expr{
				SQL: "user_session_refresh_history.session_id = excluded.session_id",
			}}},
		}).Create(&RefreshTokenHistory{
			SessionID: sessionID,
			TokenHash: oldRefreshHash,
			RotatedAt: rotatedAt,
			ExpiresAt: refreshExpiresAt,
		})
		if historyWrite.Error != nil {
			return historyWrite.Error
		}
		if historyWrite.RowsAffected == 0 {
			return ErrRefreshHistoryTokenCollision
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return matched, err
}

// FindByPreviousRefreshToken returns the session associated with any durable
// rotated-away refresh-token hash, or (nil, nil) when none does. The matched
// history timestamp is projected onto PreviousRotatedAt so RecentlyRotated can
// classify that specific generation rather than only the latest rotation.
func (s *Store) FindByPreviousRefreshToken(tokenHash string) (*UserSession, error) {
	if tokenHash == "" {
		return nil, nil
	}
	type refreshHistoryMatch struct {
		UserSession
		MatchedTokenHash string    `gorm:"column:matched_token_hash"`
		MatchedRotatedAt time.Time `gorm:"column:matched_rotated_at"`
	}
	var match refreshHistoryMatch
	result := s.db.Model(&RefreshTokenHistory{}).
		Select("user_sessions.*, user_session_refresh_history.token_hash AS matched_token_hash, user_session_refresh_history.rotated_at AS matched_rotated_at").
		Joins("JOIN user_sessions ON user_sessions.id = user_session_refresh_history.session_id").
		Where("user_session_refresh_history.token_hash = ?", tokenHash).
		Where("user_session_refresh_history.expires_at > ?", time.Now()).
		Take(&match)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	match.UserSession.PreviousRefreshToken = match.MatchedTokenHash
	match.UserSession.PreviousRotatedAt = &match.MatchedRotatedAt
	return &match.UserSession, nil
}

// providerClass groups session providers that share an identity namespace.
// user_sessions.user_id is drawn from THREE different PK spaces (User.ID for
// user/web3 sessions, Staff.ID for staff sessions, Customer.ID for customer
// sessions), so a family revoke MUST stay within the offending session's class
// or it would log out an unrelated identity that merely shares the numeric id.
func providerClass(provider string) []string {
	switch provider {
	case "customer":
		return []string{"customer"}
	case "google_staff", "staff_code", "staff_invite", "staff", "demo_staff":
		return []string{"google_staff", "staff_code", "staff_invite", "staff", "demo_staff"}
	case "email", "google", "register", "web3", "dynamic", "dynamic_web3", "demo":
		return []string{"email", "google", "register", "web3", "dynamic", "dynamic_web3", "demo"}
	default:
		if provider == "" {
			return nil
		}
		return []string{provider}
	}
}

// UserSessionProviders returns the canonical provider class for sessions whose
// user_id references users.id. Return a copy so callers cannot mutate the
// authorization boundary shared by replay and bulk-revocation paths.
func UserSessionProviders() []string {
	providers := providerClass("email")
	return append([]string(nil), providers...)
}

// RevokeFamilyWithReason revokes every live session in the same provider-scoped
// identity family as sess and records one reason and timestamp for the operation.
// The family is scoped to sess's provider class (see providerClass): by
// user_id when present, else by address (web3 sessions carry a nil user_id),
// falling back to the single session. Used on detected refresh-token reuse to
// contain a suspected token theft without over-revoking unrelated identities.
func (s *Store) RevokeFamilyWithReason(sess *UserSession, reason RevocationReason) error {
	if sess == nil {
		return nil
	}
	providers := providerClass(sess.Provider)
	if len(providers) == 0 {
		return s.RevokeWithReason(sess.ID, reason)
	}
	if sess.UserID != nil && *sess.UserID != 0 {
		return s.db.Model(&UserSession{}).
			Where("user_id = ? AND revoked = false AND provider IN ?", *sess.UserID, providers).
			Updates(revokeUpdates(reason, time.Now())).Error
	}
	if sess.Address != "" {
		return s.db.Model(&UserSession{}).
			Where("address = ? AND revoked = false AND provider IN ?", sess.Address, providers).
			Updates(revokeUpdates(reason, time.Now())).Error
	}
	return s.RevokeWithReason(sess.ID, reason)
}
