package print

import (
	"errors"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// ErrNoPrinterForRole is returned when no enabled printer is registered for the
// requested (business, location, role) combination.
var ErrNoPrinterForRole = errors.New("no printer registered for role")

// Router resolves which printer should handle a given print job.
type Router struct {
	db *gorm.DB
}

// NewRouter creates a Router backed by db.
func NewRouter(db *gorm.DB) *Router {
	return &Router{db: db}
}

// Route returns the best enabled printer for the (businessID, locationID, role)
// combination.  When locationID is non-nil, printers scoped to that location are
// preferred over global (NULL location) printers; both sets are candidates.
// When locationID is nil only global printers are considered.
//
// The ORDER BY expression uses the SQLite-compatible idiom
//
//	location_id IS NULL, location_id DESC, id ASC
//
// which evaluates to 0/1 for non-null/null in both SQLite and PostgreSQL, so
// location-specific rows sort before global (NULL) rows, and ties break on id.
func (r *Router) Route(businessID uint, locationID *uint, role string) (*database.Printer, error) {
	q := r.db.Where("business_id = ? AND role = ? AND enabled = ?", businessID, role, true)
	if locationID != nil {
		q = q.Where("location_id = ? OR location_id IS NULL", *locationID)
	} else {
		q = q.Where("location_id IS NULL")
	}

	var p database.Printer
	if err := q.Order("location_id IS NULL, location_id DESC, id ASC").First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoPrinterForRole
		}
		return nil, err
	}
	return &p, nil
}
