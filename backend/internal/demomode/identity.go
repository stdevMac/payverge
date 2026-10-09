package demomode

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"gorm.io/gorm"
)

// ShowroomOwnerEmail is the public demo's venue owner: a non-admin account
// with no password and no sign-in method of its own. Only the one-click demo
// sign-in (POST /api/v1/auth/demo/login, DEMO_MODE only) issues it a session.
// The .invalid TLD (RFC 2606) can never receive mail.
const ShowroomOwnerEmail = "owner@demo.payverge.invalid"

// SessionProvider is the user_sessions.provider of a demo owner session. It
// is valid only while DEMO_MODE is on (verifyPersistedOperatorSession).
const SessionProvider = "demo"

// StaffSessionProvider is the user_sessions.provider of a one-click demo
// staff session (waiter or kitchen). It behaves like a staff_code session but
// is, like SessionProvider, valid only while DEMO_MODE is on, so a demo
// database reused without DEMO_MODE cannot keep a showroom staff signed in.
const StaffSessionProvider = "demo_staff"

// IsSharedSessionProvider reports whether provider marks a session of a
// shared public-demo identity: every visitor who clicks the same demo button
// signs in as the same user or staff row. Refresh-token reuse on such a
// session must revoke only that session, never the identity's whole family,
// or one visitor could sign every other visitor out.
func IsSharedSessionProvider(provider string) bool {
	return provider == SessionProvider || provider == StaffSessionProvider
}

// ErrShowroomOwnerIsAdmin means the showroom owner address belongs to a
// platform admin, which the public demo must never hand out.
var ErrShowroomOwnerIsAdmin = errors.New("demo showroom owner account is a platform admin; refusing to expose it")

// EnsureShowroomOwner creates (or repairs) the showroom owner account and
// returns its id. It never promotes, and refuses an admin.
func EnsureShowroomOwner(ctx context.Context, db *gorm.DB) (uint, error) {
	if db == nil {
		return 0, errors.New("database unavailable")
	}
	var u database.User
	err := db.WithContext(ctx).Where("LOWER(email) = ?", ShowroomOwnerEmail).First(&u).Error
	switch {
	case err == nil:
		if strings.EqualFold(u.Role, "admin") {
			return 0, ErrShowroomOwnerIsAdmin
		}
		if err := db.WithContext(ctx).Model(&database.User{}).Where("id = ?", u.ID).Updates(map[string]interface{}{
			"role":           "user",
			"auth_method":    SessionProvider,
			"email_verified": true,
			"deleted_at":     nil,
		}).Error; err != nil {
			return 0, fmt.Errorf("repair demo owner: %w", err)
		}
		return u.ID, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		u = database.User{
			Email:            ShowroomOwnerEmail,
			Name:             "Demo owner",
			Role:             "user",
			AuthMethod:       SessionProvider,
			EmailVerified:    true,
			LanguageSelected: "en",
		}
		if err := db.WithContext(ctx).Create(&u).Error; err != nil {
			return 0, fmt.Errorf("create demo owner: %w", err)
		}
		return u.ID, nil
	default:
		return 0, fmt.Errorf("load demo owner: %w", err)
	}
}

// FindShowroomOwner loads the showroom owner, or gorm.ErrRecordNotFound.
func FindShowroomOwner(ctx context.Context, db *gorm.DB) (*database.User, error) {
	var u database.User
	if err := db.WithContext(ctx).Where("LOWER(email) = ? AND deleted_at IS NULL", ShowroomOwnerEmail).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// ShowroomVenues lists the active demo venues the showroom owner owns, the
// core venue ("-core" business id) first.
func ShowroomVenues(ctx context.Context, db *gorm.DB, ownerID uint) ([]database.Business, error) {
	var venues []database.Business
	err := db.WithContext(ctx).
		Select("id", "business_id", "name", "custom_url").
		Where("user_id = ? AND is_demo = ? AND is_active = ?", ownerID, true, true).
		Order("CASE WHEN business_id LIKE '%-core' THEN 0 ELSE 1 END, id").
		Find(&venues).Error
	return venues, err
}

// StaffRoleFor maps a demo sign-in role to the seeded staff role.
func StaffRoleFor(role string) (database.StaffRole, bool) {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "kitchen":
		return database.StaffRoleKitchen, true
	case "waiter", "server":
		return database.StaffRoleServer, true
	default:
		return "", false
	}
}

// ShowroomStaff returns the first active staff member with role at the core
// demo venue.
func ShowroomStaff(ctx context.Context, db *gorm.DB, ownerID uint, role database.StaffRole) (*database.Staff, error) {
	venues, err := ShowroomVenues(ctx, db, ownerID)
	if err != nil {
		return nil, err
	}
	if len(venues) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var staff database.Staff
	if err := db.WithContext(ctx).
		Where("business_id = ? AND role = ? AND is_active = ?", venues[0].ID, role, true).
		Order("id").First(&staff).Error; err != nil {
		return nil, err
	}
	return &staff, nil
}
