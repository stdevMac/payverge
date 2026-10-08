package database

import "gorm.io/gorm"

// BusinessHasPaidBill reports whether the business has EVER taken a paid bill —
// the activation boundary used by the day-7 no-first-order nudge and the
// setup-status "no orders yet" banner. Single EXISTS probe; add no ORDER BY —
// it must stay O(index probe) on the bills(business_id) index.
func BusinessHasPaidBill(businessID uint) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	var exists bool
	err := db.Raw(
		"SELECT EXISTS(SELECT 1 FROM bills WHERE business_id = ? AND status = ?)",
		businessID, BillStatusPaid,
	).Scan(&exists).Error
	return exists, err
}
