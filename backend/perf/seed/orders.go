package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// billNumber returns the deterministic uniqueIndex BillNumber for the p-th
// historical bill of business i (1-indexed).
func billNumber(bizIdx, billSeq int) string {
	return fmt.Sprintf("PERF-%03d-%05d", bizIdx, billSeq)
}

// clientReqID returns the deterministic ClientRequestID for the p-th order
// of business i, used as the dedupe key on the orders table.
func clientReqID(bizIdx, billSeq int) string {
	return fmt.Sprintf("perfseed-%03d-%05d", bizIdx, billSeq)
}

// seedHistoricalOrders creates ordersPerBiz triplets of (Bill, Payment, Order)
// per perf business, spread over the past 90 days. All three are idempotent
// on re-run via their natural keys (bill_number / composite order identity)
// and the Payment is dedupd via the bill_id (re-inserts skipped if a Payment
// already attaches to the same Bill).
//
// Status distribution: ~80% delivered, 20% cancelled — matches the analytics
// expectations carried over from the plan.
func seedHistoricalOrders(ctx context.Context, db *gorm.DB, n, ordersPerBiz, menuItemsPerBiz int) error {
	if ordersPerBiz <= 0 {
		return nil
	}
	if menuItemsPerBiz <= 0 {
		// We need menu items to reference; without them, just skip cleanly.
		return nil
	}

	biz, err := loadSeededBusinesses(ctx, db)
	if err != nil {
		return err
	}
	tablesByBiz, err := loadSeededTables(ctx, db)
	if err != nil {
		return err
	}

	// Deterministic rng so two clean DB runs produce identical fixtures.
	rng := rand.New(rand.NewSource(42))
	now := time.Now().UTC()

	for i, b := range biz {
		idx := i + 1
		table, ok := tablesByBiz[b.ID]
		if !ok {
			return fmt.Errorf("no seed table for business %s (id=%d)", b.BusinessId, b.ID)
		}

		bills := make([]database.Bill, 0, ordersPerBiz)
		for p := 1; p <= ordersPerBiz; p++ {
			daysAgo := rng.Intn(90)
			created := now.AddDate(0, 0, -daysAgo)

			// Per-bill rng matches the seed used in the second pass below so
			// the totals computed here line up with the order items written
			// after we have the persisted bill IDs.
			itemRng := rand.New(rand.NewSource(int64(idx*1_000_000 + p)))
			itemCount := 1 + itemRng.Intn(3)
			subtotalCents := int64(0)
			for k := 0; k < itemCount; k++ {
				itemIdx := 1 + itemRng.Intn(menuItemsPerBiz)
				price := menuPriceForItem(idx, itemIdx)
				qty := 1 + itemRng.Intn(2)
				lineCents := int64(price*100+0.5) * int64(qty)
				subtotalCents += lineCents
			}
			// Flat 8% tax + 10% tip for realistic-looking totals.
			taxCents := subtotalCents * 8 / 100
			tipCents := subtotalCents * 10 / 100
			totalCents := subtotalCents + taxCents + tipCents

			bills = append(bills, database.Bill{
				BusinessID:     b.ID,
				TableID:        table.ID,
				BillNumber:     billNumber(idx, p),
				Items:          "[]",
				Subtotal:       subtotalCents,
				TaxAmount:      taxCents,
				TipAmount:      tipCents,
				TotalAmount:    totalCents,
				PaidAmount:     totalCents,
				Status:         database.BillStatusPaid,
				SettlementAddr: PlaceholderAddress,
				TippingAddr:    PlaceholderAddress,
				CreatedAt:      created,
				UpdatedAt:      created,
			})
		}

		// Upsert bills in a single batch — DoNothing on the unique
		// bill_number index keeps any pre-existing rows intact.
		if err := db.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "bill_number"}},
				DoNothing: true,
			}).
			CreateInBatches(bills, 200).Error; err != nil {
			return fmt.Errorf("create bills biz=%d: %w", idx, err)
		}

		// Re-load the bills now that we have their persisted IDs (the
		// in-memory `bills` slice's IDs may be zero for rows skipped by
		// DoNothing on Postgres).
		var persistedBills []database.Bill
		if err := db.WithContext(ctx).
			Where("business_id = ? AND bill_number LIKE ?", b.ID, fmt.Sprintf("PERF-%03d-%%", idx)).
			Order("bill_number").
			Find(&persistedBills).Error; err != nil {
			return fmt.Errorf("reload bills biz=%d: %w", idx, err)
		}

		// Build the corresponding payments + orders.
		payments := make([]database.Payment, 0, len(persistedBills))
		orders := make([]database.Order, 0, len(persistedBills))

		// Map bill_number -> seq number so we can re-derive ClientRequestID + items.
		for _, bill := range persistedBills {
			// Parse the sequence number out of the bill_number suffix.
			var billBiz, seq int
			if _, err := fmt.Sscanf(bill.BillNumber, "PERF-%03d-%05d", &billBiz, &seq); err != nil {
				return fmt.Errorf("parse bill_number %q: %w", bill.BillNumber, err)
			}

			// Re-derive deterministic items using the same rng seed pattern
			// (per-bill rng so payments/orders match what would have been
			// generated above, but without coupling to loop order).
			itemRng := rand.New(rand.NewSource(int64(idx*1_000_000 + seq)))
			itemCount := 1 + itemRng.Intn(3)
			orderItems := make([]database.OrderItem, 0, itemCount)
			for k := 0; k < itemCount; k++ {
				itemIdx := 1 + itemRng.Intn(menuItemsPerBiz)
				price := menuPriceForItem(idx, itemIdx)
				qty := 1 + itemRng.Intn(2)
				lineCents := int64(price*100+0.5) * int64(qty)
				orderItems = append(orderItems, database.OrderItem{
					ID:           fmt.Sprintf("%s-i%d", clientReqID(idx, seq), k),
					ItemType:     "menu_item",
					MenuItemID:   menuItemID(idx, itemIdx),
					MenuItemName: fmt.Sprintf("Item %d", itemIdx),
					Quantity:     qty,
					Price:        price,
					Subtotal:     float64(lineCents) / 100.0,
				})
			}
			itemsJSON, err := json.Marshal(orderItems)
			if err != nil {
				return fmt.Errorf("marshal order items biz=%d seq=%d: %w", idx, seq, err)
			}

			// 80% delivered, 20% cancelled — derived from seq for determinism.
			status := database.OrderStatusOrderDelivered
			if seq%5 == 0 {
				status = database.OrderStatusOrderCancelled
			}

			// Use bill.CreatedAt for the payment timestamp so the analytics
			// horizon (past 90 days) lines up with the bills.
			confirmed := bill.CreatedAt
			payments = append(payments, database.Payment{
				BillID:          bill.ID,
				PayerAddr:       PlaceholderAddress,
				Amount:          bill.TotalAmount,
				TipAmount:       bill.TipAmount,
				TxHash:          fmt.Sprintf("0xperfseed%03d%05d", idx, seq),
				Status:          database.PaymentStatusConfirmed,
				PaymentMethod:   "crypto",
				SettlementChain: "base",
				ConfirmedAt:     &confirmed,
				CreatedAt:       bill.CreatedAt,
				UpdatedAt:       bill.CreatedAt,
			})

			cr := clientReqID(idx, seq)
			orderNumber := fmt.Sprintf("PERFORD-%03d-%05d", idx, seq)
			orders = append(orders, database.Order{
				BillID:          bill.ID,
				BusinessID:      b.ID,
				OrderNumber:     orderNumber,
				Status:          status,
				CreatedBy:       "guest",
				ClientRequestID: &cr,
				Items:           string(itemsJSON),
				CreatedAt:       bill.CreatedAt,
				UpdatedAt:       bill.CreatedAt,
			})
		}

		// Payments are deduped by the unique tx_hash index — fully idempotent.
		if len(payments) > 0 {
			if err := db.WithContext(ctx).
				Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "tx_hash"}},
					DoNothing: true,
				}).
				CreateInBatches(payments, 200).Error; err != nil {
				return fmt.Errorf("create payments biz=%d: %w", idx, err)
			}
		}

		// Orders are deduped on the composite (bill_id, created_by, client_request_id)
		// unique index.
		if len(orders) > 0 {
			if err := db.WithContext(ctx).
				Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "bill_id"}, {Name: "created_by"}, {Name: "client_request_id"}},
					DoNothing: true,
				}).
				CreateInBatches(orders, 200).Error; err != nil {
				return fmt.Errorf("create orders biz=%d: %w", idx, err)
			}
		}
	}
	_ = n
	return nil
}
