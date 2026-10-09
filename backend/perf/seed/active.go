package main

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func activeBillNumber(bizIdx int) string {
	return fmt.Sprintf("PERF-ACTIVE-%03d", bizIdx)
}

func perfLoginCode(bizIdx int) string {
	return fmt.Sprintf("86%04d", bizIdx)
}

// seedActiveFixtures installs the mutable-but-repeatable portion of the load
// fixture: one open bill and one one-use login code per seeded business.
// Re-running the seeder restores both, so every capacity run starts from a
// known authentication and order state without hand-edited IDs or JWTs.
func seedActiveFixtures(ctx context.Context, db *gorm.DB) error {
	businesses, err := loadSeededBusinesses(ctx, db)
	if err != nil {
		return err
	}
	tables, err := loadSeededTables(ctx, db)
	if err != nil {
		return err
	}

	for i := range businesses {
		idx := i + 1
		business := businesses[i]
		table, ok := tables[business.ID]
		if !ok {
			return fmt.Errorf("active fixtures: missing table for business %s", business.BusinessId)
		}

		// Perf fixtures are explicitly orderable even when a previous local run
		// toggled the guest switch. This update is scoped to perf-seed rows only.
		if err := db.WithContext(ctx).Model(&database.Business{}).
			Where("id = ?", business.ID).
			Updates(map[string]any{
				"is_active":       true,
				"kitchen_enabled": true, "orders_enabled": true,
			}).Error; err != nil {
			return fmt.Errorf("activate perf business %s: %w", business.BusinessId, err)
		}

		bill := database.Bill{
			BusinessID: business.ID, TableID: table.ID,
			BillNumber: activeBillNumber(idx), Status: database.BillStatusOpen,
			Items: "[]", SettlementAddr: business.SettlementAddr, TippingAddr: business.TippingAddr,
		}
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "bill_number"}},
			DoUpdates: clause.Assignments(map[string]any{
				"business_id": business.ID, "table_id": table.ID,
				"status": database.BillStatusOpen, "updated_at": time.Now().UTC(),
			}),
		}).Create(&bill).Error; err != nil {
			return fmt.Errorf("upsert active bill %s: %w", bill.BillNumber, err)
		}
		if err := db.WithContext(ctx).Where("bill_number = ?", bill.BillNumber).First(&bill).Error; err != nil {
			return fmt.Errorf("reload active bill %s: %w", bill.BillNumber, err)
		}
		// Remove only D5-generated order rows, then restore the bill aggregate.
		// This prevents a rerun from replaying old request IDs or measuring an
		// ever-growing bill. Historical PERF-* rows remain untouched.
		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("bill_id = ?", bill.ID).Delete(&database.BillItem{}).Error; err != nil {
				return err
			}
			if err := tx.Where("bill_id = ? AND created_by = ? AND client_request_id LIKE ?", bill.ID, "guest", "d5-%").
				Delete(&database.Order{}).Error; err != nil {
				return err
			}
			return tx.Model(&database.Bill{}).Where("id = ?", bill.ID).Updates(map[string]any{
				"items": "[]", "subtotal": 0, "tax_amount": 0, "tip_amount": 0,
				"service_fee_amount": 0, "total_amount": 0, "paid_amount": 0,
				"loyalty_discount_cents": 0, "status": database.BillStatusOpen,
				"closed_at": nil, "settled_at": nil, "updated_at": time.Now().UTC(),
			}).Error
		}); err != nil {
			return fmt.Errorf("reset active bill %s: %w", bill.BillNumber, err)
		}

		var staff database.Staff
		email := fmt.Sprintf("perf-seed-staff-%03d-1@example.test", idx)
		if err := db.WithContext(ctx).
			Where("business_id = ? AND email = ? AND is_active = ?", business.ID, email, true).
			First(&staff).Error; err != nil {
			return fmt.Errorf("load perf manager %s: %w", email, err)
		}
		code := perfLoginCode(idx)
		if err := db.WithContext(ctx).
			Where("staff_id = ? AND code = ?", staff.ID, code).
			Delete(&database.StaffLoginCode{}).Error; err != nil {
			return fmt.Errorf("reset perf login code %s: %w", code, err)
		}
		loginCode := database.StaffLoginCode{
			StaffID: staff.ID, Code: code, ExpiresAt: time.Now().UTC().Add(24 * time.Hour), Used: false,
		}
		if err := db.WithContext(ctx).Create(&loginCode).Error; err != nil {
			return fmt.Errorf("create perf login code %s: %w", code, err)
		}
	}
	return nil
}
