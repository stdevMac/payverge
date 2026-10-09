package print

import (
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// BusinessHasEnabledKitchenPrinter reports whether the business has at least
// one enabled printer that can receive kitchen/bar tickets. Used by the
// approve-time enqueue (X-2) so printer-less businesses never accumulate
// pending print jobs.
func BusinessHasEnabledKitchenPrinter(db *gorm.DB, businessID uint) bool {
	var count int64
	if err := db.Model(&database.Printer{}).
		Where("business_id = ? AND enabled = ? AND role IN ?", businessID, true, []string{"kitchen", "bar"}).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

// OrderHasKitchenJob reports whether a kitchen/bar print job already exists
// for the order — the approve-time enqueue and the orphan sweep must never
// double-print the same ticket.
func OrderHasKitchenJob(db *gorm.DB, orderID uint) bool {
	var count int64
	if err := db.Model(&database.PrintJob{}).
		Where("order_id = ? AND kind IN ?", orderID,
			[]database.PrintJobKind{database.PrintJobKindKitchen, database.PrintJobKindBar}).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}
