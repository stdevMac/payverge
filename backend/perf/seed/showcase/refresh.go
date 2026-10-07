package main

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// refreshShowcaseBills hard-deletes the showcase bills (both historical
// `BV-%05d` and active `BV-A-%`, which share the `BV-` prefix) along with
// their payments (`0xshowcase%` tx hashes) and orders (`BV-%` order numbers).
//
// Why: bills are keyed on bill_number with OnConflict DoNothing, and their
// CreatedAt is stamped at seed time. So a plain re-run on an already-seeded
// business is a no-op and the dates never refresh — the dashboard's 30-day
// revenue window goes stale/empty. Clearing them lets seedHistoricalBills and
// seedActiveBills recreate them dated to the current 90-day window.
//
// Raw DELETEs (not GORM model deletes) so this is an unconditional HARD delete
// regardless of any soft-delete column, and so the unique bill_number index is
// freed for the immediate re-seed. Children are removed before parents and the
// whole thing runs in one transaction.
func refreshShowcaseBills(ctx context.Context, db *gorm.DB, bizID uint) error {
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`DELETE FROM payments WHERE bill_id IN (SELECT id FROM bills WHERE business_id = ? AND bill_number LIKE 'BV-%')`,
			bizID,
		).Error; err != nil {
			return fmt.Errorf("delete payments: %w", err)
		}
		if err := tx.Exec(
			`DELETE FROM orders WHERE business_id = ? AND order_number LIKE 'BV-%'`,
			bizID,
		).Error; err != nil {
			return fmt.Errorf("delete orders: %w", err)
		}
		if err := tx.Exec(
			`DELETE FROM bill_items WHERE bill_id IN (SELECT id FROM bills WHERE business_id = ? AND bill_number LIKE 'BV-%')`,
			bizID,
		).Error; err != nil {
			return fmt.Errorf("delete bill items: %w", err)
		}
		if err := tx.Exec(
			`DELETE FROM bills WHERE business_id = ? AND bill_number LIKE 'BV-%'`,
			bizID,
		).Error; err != nil {
			return fmt.Errorf("delete bills: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("refresh-bills: cleared existing BV-* bills/payments/orders for business id=%d\n", bizID)
	return nil
}

// resetShowcaseBusiness deletes the whole showcase business by its natural key.
// FK cascades remove most child rows; seedAll then rebuilds everything fresh.
func resetShowcaseBusiness(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).
		Exec(`DELETE FROM businesses WHERE business_id = ?`, BusinessSeedID).Error
}

// printSeedSummary reports the post-seed bill counts so an operator can confirm
// the data landed without needing psql access to the (host-internal) DB.
func printSeedSummary(ctx context.Context, db *gorm.DB, bizID uint) {
	var total, last30 int64
	db.WithContext(ctx).Model(&database.Bill{}).
		Where("business_id = ? AND bill_number LIKE ?", bizID, "BV-%").
		Count(&total)
	db.WithContext(ctx).Model(&database.Bill{}).
		Where("business_id = ? AND bill_number LIKE ? AND created_at >= ?", bizID, "BV-%", time.Now().AddDate(0, 0, -30)).
		Count(&last30)
	fmt.Printf("showcase bills: %d total, %d dated within the last 30 days\n", total, last30)
}
