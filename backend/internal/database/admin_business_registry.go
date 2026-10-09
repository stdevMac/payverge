package database

import (
	"strings"

	"gorm.io/gorm"
)

// BusinessKind classifies a business for the admin registry and ops surfaces.
// real  — production signups
// demo  — Demo Center seeder output (is_demo=true)
// test  — CI / local fixtures (@payverge.test, @payverge.local, etc.)
type BusinessKind string

const (
	BusinessKindReal BusinessKind = "real"
	BusinessKindDemo BusinessKind = "demo"
	BusinessKindTest BusinessKind = "test"

	// AdminBusinessKindAll is the filter sentinel that disables kind scoping.
	AdminBusinessKindAll = "all"
)

// Admin lifecycle status vocabulary (derived from is_active / closed_at).
const (
	BusinessStatusActive    = "active"
	BusinessStatusSuspended = "suspended"
	BusinessStatusClosed    = "closed"
)

// AdminBusinessStatus derives the admin lifecycle status of a business.
func AdminBusinessStatus(b *Business) string {
	switch {
	case b == nil:
		return ""
	case b.ClosedAt != nil:
		return BusinessStatusClosed
	case !b.IsActive:
		return BusinessStatusSuspended
	default:
		return BusinessStatusActive
	}
}

// AdminBusinessFilter is the single filter shape for the admin business registry.
// Default Kind is real (empty string is treated as real).
type AdminBusinessFilter struct {
	Kind   string // real | demo | test | all  (default: real)
	Status string // active | suspended | closed (empty: any)
	Search string
	Page   int
	Limit  int
}

// NormalizeKind returns the effective kind filter value.
// Empty → real. Unknown values fall back to real so the registry never
// accidentally opens to fixtures.
func (f AdminBusinessFilter) NormalizeKind() string {
	k := strings.ToLower(strings.TrimSpace(f.Kind))
	switch k {
	case "", string(BusinessKindReal):
		return string(BusinessKindReal)
	case string(BusinessKindDemo), string(BusinessKindTest), AdminBusinessKindAll:
		return k
	default:
		return string(BusinessKindReal)
	}
}

// ApplyAdminBusinessFilter scopes a businesses query by AdminBusinessFilter.
// This is the shared WHERE path every admin surface must use for registry
// membership (dashboard counts, list, emails, fiscal joins).
func ApplyAdminBusinessFilter(query *gorm.DB, filter AdminBusinessFilter) *gorm.DB {
	if query == nil {
		return query
	}

	kind := filter.NormalizeKind()
	if kind != AdminBusinessKindAll {
		query = query.Where("businesses.kind = ?", kind)
	}

	if search := strings.TrimSpace(filter.Search); search != "" {
		// LOWER()/LIKE (not ILIKE) so the same path works on SQLite unit tests and Postgres.
		// Join users for owner-email search; LEFT so wallet-only owners still match name.
		term := "%" + strings.ToLower(search) + "%"
		query = query.Joins("LEFT JOIN users ON users.id = businesses.user_id").
			Where(
				"LOWER(businesses.name) LIKE ? OR LOWER(businesses.owner_name) LIKE ? OR LOWER(users.email) LIKE ?",
				term, term, term,
			)
	}

	switch strings.ToLower(strings.TrimSpace(filter.Status)) {
	case BusinessStatusActive:
		query = query.Where("businesses.is_active = ? AND businesses.closed_at IS NULL", true)
	case BusinessStatusSuspended:
		query = query.Where("businesses.is_active = ? AND businesses.closed_at IS NULL", false)
	case BusinessStatusClosed:
		query = query.Where("businesses.closed_at IS NOT NULL")
	}

	return query
}

// CountBusinessesForAdmin returns how many businesses match filter.
// Single count entry point for admin dashboard / registry / emails / fiscal scope.
func CountBusinessesForAdmin(filter AdminBusinessFilter) (int64, error) {
	db := GetDB()
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	var total int64
	err := ApplyAdminBusinessFilter(db.Model(&Business{}), filter).Count(&total).Error
	return total, err
}

// ListBusinessesForAdmin returns a page of businesses plus the matching total.
// This is the single list entry point for the admin business registry.
func ListBusinessesForAdmin(filter AdminBusinessFilter) ([]Business, int64, error) {
	db := GetDB()
	if db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	// Registry UI caps at 100 via the handler; email/broadcast dumps may request
	// up to 10k so they still go through this single entry point.
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 10000 {
		limit = 10000
	}

	query := ApplyAdminBusinessFilter(db.Model(&Business{}), filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	var businesses []Business
	if err := query.Select("businesses.*").
		Offset(offset).
		Limit(limit).
		Order("businesses.created_at DESC").
		Find(&businesses).Error; err != nil {
		return nil, 0, err
	}
	return businesses, total, nil
}

// Business lock denial codes. A business the server administrator suspended
// (is_active=false) or closed (closed_at) is the only lock Payverge has;
// every operator surface reports it with one of these codes.
const (
	BusinessLockCodeSuspended = "business_suspended"
	BusinessLockCodeClosed    = "business_closed"
)

// BusinessLockDenial reports the denial code and operator-facing message for a
// business that is not operational. locked is false (and code/message empty)
// when the business may operate.
func BusinessLockDenial(b *Business) (code, message string, locked bool) {
	if IsBusinessOperational(b) {
		return "", "", false
	}
	if b != nil && b.ClosedAt != nil {
		return BusinessLockCodeClosed, "This business has been closed by the server administrator. Contact them to restore access.", true
	}
	return BusinessLockCodeSuspended, "This business has been suspended by the server administrator. Contact them to restore access.", true
}
