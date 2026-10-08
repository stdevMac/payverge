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

// taxRate matches the rate set on the showcase business so totals line up.
const taxRate = 0.0875

// itemPick is a single line item used by both bill totals and order JSON.
type itemPick struct {
	MenuItemID string
	Name       string
	Price      float64
	Quantity   int
}

// pickItems pseudo-randomly selects 1-4 items from the menu for a single
// bill, using a deterministic RNG seeded from the bill sequence number.
func pickItems(seq int64) []itemPick {
	prices := menuItemPrices()
	names := menuItemNames()
	ids := make([]string, 0, len(prices))
	for id := range prices {
		ids = append(ids, id)
	}
	// Stable order for determinism — ids from a map iterate randomly.
	sortStrings(ids)

	rng := rand.New(rand.NewSource(seq))
	count := 2 + rng.Intn(3) // 2..4 items
	picks := make([]itemPick, 0, count)
	for k := 0; k < count; k++ {
		id := ids[rng.Intn(len(ids))]
		qty := 1 + rng.Intn(2)
		picks = append(picks, itemPick{
			MenuItemID: id,
			Name:       names[id],
			Price:      prices[id],
			Quantity:   qty,
		})
	}
	return picks
}

func sortStrings(s []string) {
	// Avoid importing "sort" twice via different files — local insertion sort
	// is fine for the small (~34) menu.
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// computeTotals returns subtotal, tax, tip and total cents from a pick set.
// Tip defaults to 18% of (subtotal + tax) so the analytics charts have a
// realistic tip distribution.
func computeTotals(picks []itemPick) (subtotal, tax, tip, total int64) {
	for _, p := range picks {
		subtotal += int64(p.Price*100+0.5) * int64(p.Quantity)
	}
	tax = int64(float64(subtotal)*taxRate + 0.5)
	tip = (subtotal + tax) * 18 / 100
	total = subtotal + tax + tip
	return
}

// seedHistoricalBills creates ~640 paid bills spread across the past 90 days
// — every day gets between 4 and 12 bills (variable by day-of-week, with
// Friday and Saturday peaks) so analytics, accounting, and director-console
// daily charts have continuous signal.
//
// Each bill gets its corresponding Payment and Order rows.
func seedHistoricalBills(ctx context.Context, db *gorm.DB, bizID uint) error {
	tables, err := loadTables(ctx, db, bizID)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return fmt.Errorf("no tables to attach bills to")
	}
	staff, err := loadStaff(ctx, db, bizID)
	if err != nil {
		return err
	}

	rng := rand.New(rand.NewSource(2026_05_28))
	now := time.Now().UTC()

	// Bills-per-day shape — busier on Fri/Sat, lighter on Mon (closed in
	// the showcase hours, but keep a couple of takeout orders for realism).
	// Index: 0=Sun, 1=Mon, ..., 6=Sat. Values are baseline counts before
	// a ±2 jitter and lunch-vs-dinner spread.
	billsByDow := [7]int{8, 2, 5, 5, 7, 11, 12}

	type billSpec struct {
		seq     int
		created time.Time
	}
	var specs []billSpec
	seq := 0
	// Walk every day in the past 90, including today.
	for day := 89; day >= 0; day-- {
		dayStart := now.Add(-time.Duration(day) * 24 * time.Hour).Truncate(24 * time.Hour)
		count := billsByDow[int(dayStart.Weekday())] + rng.Intn(3) - 1 // ±1 jitter
		if count < 1 {
			count = 1
		}
		for i := 0; i < count; i++ {
			seq++
			// Spread within lunch (11:30-14:00) and dinner (17:30-22:00)
			// service windows. 30% lunch, 70% dinner.
			var hour, minute int
			if rng.Intn(10) < 3 {
				hour = 11 + rng.Intn(3) // 11..13
				minute = 30 + rng.Intn(30)
			} else {
				hour = 17 + rng.Intn(5) // 17..21
				minute = 30 + rng.Intn(30)
			}
			created := dayStart.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
			specs = append(specs, billSpec{seq: seq, created: created})
		}
	}

	billCount := len(specs)
	bills := make([]database.Bill, 0, billCount)

	for _, sp := range specs {
		seq := sp.seq
		created := sp.created
		table := tables[rng.Intn(len(tables))]

		picks := pickItems(int64(seq * 31))
		subtotal, tax, tip, total := computeTotals(picks)

		billItemsJSON, err := json.Marshal(picksToOrderItems(picks))
		if err != nil {
			return fmt.Errorf("marshal bill items seq=%d: %w", seq, err)
		}

		var createdBy, closedBy *uint
		if len(staff) > 0 {
			s1 := staff[rng.Intn(len(staff))].ID
			s2 := staff[rng.Intn(len(staff))].ID
			createdBy = &s1
			closedBy = &s2
		}
		closed := created.Add(90 * time.Minute)

		bills = append(bills, database.Bill{
			BusinessID:       bizID,
			TableID:          table.ID,
			BillNumber:       fmt.Sprintf("BV-%05d", seq),
			Notes:            "",
			Items:            string(billItemsJSON),
			Subtotal:         subtotal,
			TaxAmount:        tax,
			TipAmount:        tip,
			TotalAmount:      total,
			PaidAmount:       total,
			Status:           database.BillStatusPaid,
			SettlementAddr:   PlaceholderAddress,
			TippingAddr:      PlaceholderAddress,
			CreatedByStaffID: createdBy,
			ClosedByStaffID:  closedBy,
			CreatedAt:        created,
			UpdatedAt:        closed,
			ClosedAt:         &closed,
		})
	}

	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "bill_number"}},
			DoNothing: true,
		}).
		CreateInBatches(bills, 100).Error; err != nil {
		return fmt.Errorf("create bills: %w", err)
	}

	// Reload persisted bills to recover IDs (rows skipped by DoNothing have
	// id=0 in the input slice on Postgres).
	var persisted []database.Bill
	if err := db.WithContext(ctx).
		Where("business_id = ? AND bill_number LIKE ?", bizID, "BV-%").
		Order("bill_number").
		Find(&persisted).Error; err != nil {
		return fmt.Errorf("reload bills: %w", err)
	}

	payments := make([]database.Payment, 0, len(persisted))
	orders := make([]database.Order, 0, len(persisted))
	billItems := make([]database.BillItem, 0, len(persisted)*3)
	for _, b := range persisted {
		var seq int
		if _, err := fmt.Sscanf(b.BillNumber, "BV-%05d", &seq); err != nil {
			continue // Skip non-historical bills (e.g. active bills BV-A-*)
		}

		picks := pickItems(int64(seq * 31))
		billItems = append(billItems, buildBillItems(b.ID, picks)...)
		method := "crypto"
		if seq%4 == 0 {
			method = "card" // 25% card payments
		} else if seq%7 == 0 {
			method = "cash"
		}
		confirmed := b.CreatedAt.Add(2 * time.Minute)

		payments = append(payments, database.Payment{
			BillID:          b.ID,
			PayerAddr:       PlaceholderAddress,
			Amount:          b.TotalAmount,
			TipAmount:       b.TipAmount,
			TxHash:          fmt.Sprintf("0xshowcase%05d", seq),
			Status:          database.PaymentStatusConfirmed,
			PaymentMethod:   method,
			SettlementChain: "base",
			ConfirmedAt:     &confirmed,
			CreatedAt:       b.CreatedAt,
			UpdatedAt:       b.CreatedAt,
		})

		itemsJSON, err := json.Marshal(picksToOrderItems(picks))
		if err != nil {
			return fmt.Errorf("marshal order items seq=%d: %w", seq, err)
		}
		cr := fmt.Sprintf("showcase-%05d", seq)
		approved := b.CreatedAt.Add(1 * time.Minute)
		orders = append(orders, database.Order{
			BillID:          b.ID,
			BusinessID:      bizID,
			OrderNumber:     fmt.Sprintf("BV-O%05d", seq),
			Status:          database.OrderStatusOrderDelivered,
			CreatedBy:       "guest",
			ClientRequestID: &cr,
			ApprovedBy:      "marco.rossi@trattoriabellavista.example",
			Items:           string(itemsJSON),
			CreatedAt:       b.CreatedAt,
			UpdatedAt:       b.CreatedAt,
			ApprovedAt:      &approved,
		})
	}

	if len(payments) > 0 {
		if err := db.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "tx_hash"}},
				DoNothing: true,
			}).
			CreateInBatches(payments, 100).Error; err != nil {
			return fmt.Errorf("create payments: %w", err)
		}
	}
	if len(orders) > 0 {
		if err := db.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "bill_id"}, {Name: "created_by"}, {Name: "client_request_id"}},
				DoNothing: true,
			}).
			CreateInBatches(orders, 100).Error; err != nil {
			return fmt.Errorf("create orders: %w", err)
		}
	}
	if len(billItems) > 0 {
		if err := db.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				DoNothing: true,
			}).
			CreateInBatches(billItems, 100).Error; err != nil {
			return fmt.Errorf("create bill items: %w", err)
		}
	}

	return nil
}

// buildBillItems materialises normalised bill_items rows for a bill. Item
// analytics (AnalyticsService.GetPopularItems) aggregates exclusively from the
// bill_items table — the JSON snapshot on Bill.Items/Order.Items is invisible
// to it — so without these rows the Menu analytics panel is empty. IDs are
// derived deterministically from (billID, index) so re-runs are idempotent via
// OnConflict DoNothing on the primary key.
func buildBillItems(billID uint, picks []itemPick) []database.BillItem {
	items := make([]database.BillItem, 0, len(picks))
	for k, p := range picks {
		items = append(items, database.BillItem{
			ID:         billItemUUID(billID, k),
			BillID:     billID,
			MenuItemID: p.MenuItemID,
			Name:       p.Name,
			Price:      p.Price,
			Quantity:   p.Quantity,
			ItemType:   "menu_item",
			Subtotal:   p.Price * float64(p.Quantity),
		})
	}
	return items
}

// billItemUUID returns a deterministic, valid 8-4-4-4-12 UUID string for a
// (billID, index) pair so seeded bill_items have stable primary keys.
func billItemUUID(billID uint, k int) string {
	return fmt.Sprintf("%08x-%04x-4000-8000-%012x", uint32(billID), uint16(k), uint64(billID))
}

// picksToOrderItems converts a pick set to OrderItem JSON used by both bill
// snapshot and Order.Items columns.
func picksToOrderItems(picks []itemPick) []database.OrderItem {
	items := make([]database.OrderItem, 0, len(picks))
	for k, p := range picks {
		items = append(items, database.OrderItem{
			ID:           fmt.Sprintf("item-%d", k),
			ItemType:     "menu_item",
			MenuItemID:   p.MenuItemID,
			MenuItemName: p.Name,
			Quantity:     p.Quantity,
			Price:        p.Price,
			Subtotal:     p.Price * float64(p.Quantity),
		})
	}
	return items
}

// seedActiveBills creates 6 open bills with orders in varied kitchen states
// so the Kitchen Display + Bills views have realistic in-flight data:
//
//   - 2 in "approved" (awaiting kitchen pickup)
//   - 2 in "in_kitchen" (cooking)
//   - 2 in "ready" (plated, awaiting server)
func seedActiveBills(ctx context.Context, db *gorm.DB, bizID uint) error {
	tables, err := loadTables(ctx, db, bizID)
	if err != nil {
		return err
	}
	staff, err := loadStaff(ctx, db, bizID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	type spec struct {
		Seq    int
		Status database.OrderStatus
		Mins   int
		Table  int
		Notes  string
	}
	specs := []spec{
		{1, database.OrderStatusApproved, 4, 8, "No onion"},
		{2, database.OrderStatusApproved, 7, 14, ""},
		{3, database.OrderStatusInKitchen, 9, 5, "Allergy: nuts"},
		{4, database.OrderStatusInKitchen, 12, 12, ""},
		{5, database.OrderStatusOrderReady, 14, 16, ""},
		{6, database.OrderStatusOrderReady, 18, 4, "VIP table"},
	}

	for _, s := range specs {
		if s.Table >= len(tables) {
			continue
		}
		table := tables[s.Table]
		picks := pickItems(int64(s.Seq*101) + 9999)
		subtotal, tax, tip, total := computeTotals(picks)
		created := now.Add(-time.Duration(s.Mins) * time.Minute)
		billItemsJSON, _ := json.Marshal(picksToOrderItems(picks))

		var createdBy *uint
		if len(staff) > 0 {
			id := staff[s.Seq%len(staff)].ID
			createdBy = &id
		}

		bill := database.Bill{
			BusinessID:       bizID,
			TableID:          table.ID,
			BillNumber:       fmt.Sprintf("BV-A-%03d", s.Seq),
			Notes:            s.Notes,
			Items:            string(billItemsJSON),
			Subtotal:         subtotal,
			TaxAmount:        tax,
			TipAmount:        tip,
			TotalAmount:      total,
			PaidAmount:       0,
			Status:           database.BillStatusOpen,
			SettlementAddr:   PlaceholderAddress,
			TippingAddr:      PlaceholderAddress,
			CreatedByStaffID: createdBy,
			CreatedAt:        created,
			UpdatedAt:        created,
		}
		if err := db.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "bill_number"}},
				DoNothing: true,
			}).
			Create(&bill).Error; err != nil {
			return fmt.Errorf("create active bill seq=%d: %w", s.Seq, err)
		}

		// Reload to get ID (in case it was upserted as a no-op)
		var persisted database.Bill
		if err := db.WithContext(ctx).
			Where("bill_number = ?", bill.BillNumber).
			First(&persisted).Error; err != nil {
			return fmt.Errorf("reload active bill %s: %w", bill.BillNumber, err)
		}

		cr := fmt.Sprintf("showcase-active-%03d", s.Seq)
		approved := created.Add(1 * time.Minute)
		orderItemsJSON, _ := json.Marshal(picksToOrderItems(picks))

		var kitchenAcked *time.Time
		if s.Status == database.OrderStatusInKitchen || s.Status == database.OrderStatusOrderReady {
			t := created.Add(2 * time.Minute)
			kitchenAcked = &t
		}

		order := database.Order{
			BillID:          persisted.ID,
			BusinessID:      bizID,
			OrderNumber:     fmt.Sprintf("BV-AO-%03d", s.Seq),
			Status:          s.Status,
			CreatedBy:       "guest",
			ClientRequestID: &cr,
			ApprovedBy:      "marco.rossi@trattoriabellavista.example",
			Notes:           s.Notes,
			Items:           string(orderItemsJSON),
			CreatedAt:       created,
			UpdatedAt:       created,
			ApprovedAt:      &approved,
			KitchenAckedAt:  kitchenAcked,
		}
		if err := db.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "bill_id"}, {Name: "created_by"}, {Name: "client_request_id"}},
				DoNothing: true,
			}).
			Create(&order).Error; err != nil {
			return fmt.Errorf("create active order seq=%d: %w", s.Seq, err)
		}

		if err := db.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				DoNothing: true,
			}).
			Create(buildBillItems(persisted.ID, picks)).Error; err != nil {
			return fmt.Errorf("create active bill items seq=%d: %w", s.Seq, err)
		}
	}

	return nil
}
