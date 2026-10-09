package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// QRBranding is the full QR-customization payload applied across every table
// (and mirrored onto the business defaults) by the bulk "Apply to All" endpoint.
type QRBranding struct {
	LogoURL          string
	ForegroundColor  string
	BackgroundColor  string
	LogoSize         int
	ShowBusinessName bool
	ShowTableName    bool
	TextFont         string
}

// ApplyQRBrandingToAllTables writes the branding onto every table of a business
// AND the business QR defaults in ONE transaction, replacing the old N-table
// PUT fan-out (201 concurrent requests at 200 tables, non-atomic — a partial
// apply left tables inconsistent). Returns the number of tables updated.
func ApplyQRBrandingToAllTables(businessID uint, b QRBranding) (int64, error) {
	var affected int64
	err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Table{}).
			Where("business_id = ?", businessID).
			Updates(map[string]interface{}{
				"qr_logo_url":           b.LogoURL,
				"qr_foreground_color":   b.ForegroundColor,
				"qr_background_color":   b.BackgroundColor,
				"qr_logo_size":          b.LogoSize,
				"qr_show_business_name": b.ShowBusinessName,
				"qr_show_table_name":    b.ShowTableName,
				"qr_text_font":          b.TextFont,
			})
		if res.Error != nil {
			return fmt.Errorf("failed to apply QR branding to tables: %w", res.Error)
		}
		affected = res.RowsAffected

		if err := tx.Model(&Business{}).
			Where("id = ?", businessID).
			Updates(map[string]interface{}{
				"default_qr_logo_url":           b.LogoURL,
				"default_qr_foreground_color":   b.ForegroundColor,
				"default_qr_background_color":   b.BackgroundColor,
				"default_qr_logo_size":          b.LogoSize,
				"default_qr_show_business_name": b.ShowBusinessName,
				"default_qr_show_table_name":    b.ShowTableName,
				"default_qr_text_font":          b.TextFont,
			}).Error; err != nil {
			return fmt.Errorf("failed to apply QR branding to business defaults: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// GetActiveBillsByTableID retrieves all active bills for a table.
func GetActiveBillsByTableID(tableID uint) ([]Bill, error) {
	var bills []Bill
	if err := db.Where("table_id = ? AND status IN ?", tableID, activeBillStatusStrings()).
		Order("created_at DESC").
		Find(&bills).Error; err != nil {
		return nil, err
	}
	return bills, nil
}

// GetTablesWithStatus retrieves all tables with their current status (active bills and reservations).
// Uses batch queries to avoid N+1: 1 query for tables, 1 for bills, 1 for reservations.
//
// When includeInactive is true the is_active predicate is dropped so
// soft-deleted tables become reachable again — without it a deactivated table
// is a permanent UI trapdoor (both list queries filtered is_active=true with
// no reactivation path).
func GetTablesWithStatus(businessID uint, includeInactive bool) ([]map[string]interface{}, error) {
	var tables []Table
	tablesQuery := db.Select(
		"id", "business_id", "name", "table_code", "capacity", "is_active",
		"qr_foreground_color", "qr_background_color",
	).Where("business_id = ?", businessID)
	if !includeInactive {
		tablesQuery = tablesQuery.Where("is_active = ?", true)
	}
	if err := tablesQuery.Find(&tables).Error; err != nil {
		return nil, err
	}

	if len(tables) == 0 {
		return []map[string]interface{}{}, nil
	}

	tableIDs := make([]uint, len(tables))
	for i, t := range tables {
		tableIDs[i] = t.ID
	}

	// Batch: all active bills for these tables (single query)
	var allBills []Bill
	if err := db.Select(
		"id",
		"business_id",
		"table_id",
		"counter_id",
		"bill_number",
		"notes",
		"subtotal",
		"tax_amount",
		"service_fee_amount",
		"total_amount",
		"paid_amount",
		"tip_amount",
		"status",
		"settlement_addr",
		"tipping_addr",
		"created_by_staff_id",
		"closed_by_staff_id",
		"crm_customer_id",
		"created_at",
		"updated_at",
		"closed_at",
		"feedback_email_sent_at",
	).
		Where("table_id IN ? AND status IN ?", tableIDs, activeBillStatusStrings()).
		Order("created_at DESC").
		Find(&allBills).Error; err != nil {
		return nil, err
	}
	billsByTable := make(map[uint][]Bill, len(tables))
	activeBillIDs := make([]uint, 0, len(allBills))
	for _, b := range allBills {
		billsByTable[b.TableID] = append(billsByTable[b.TableID], b)
		activeBillIDs = append(activeBillIDs, b.ID)
	}

	// Unfinished service on a terminal check still occupies the table:
	// live kitchen tickets AND pending guest sends (T1 / 1129+1125).
	// Only tables without an open/partial check can be orphans, so skip
	// the join when every table already has an active bill.
	// One grouped projection — never N+1, never SELECT * on orders.
	type occupyingClock struct {
		SeatedAt time.Time
		LastSeen time.Time
	}
	unfinishedByTable := make(map[uint]occupyingClock)
	orphanCandidateIDs := make([]uint, 0, len(tables))
	for _, table := range tables {
		if len(billsByTable[table.ID]) == 0 {
			orphanCandidateIDs = append(orphanCandidateIDs, table.ID)
		}
	}
	if len(orphanCandidateIDs) > 0 && db.Migrator().HasTable(&Order{}) {
		orphanStatuses := append(KitchenLiveOrderStatuses(), OrderStatusPending)
		var occupyingBills []struct {
			TableID   uint      `gorm:"column:table_id"`
			CreatedAt time.Time `gorm:"column:created_at"`
			UpdatedAt time.Time `gorm:"column:updated_at"`
		}
		if err := db.Table("orders").
			Select("bills.table_id, bills.created_at, bills.updated_at").
			Joins("JOIN bills ON bills.id = orders.bill_id").
			Where("bills.table_id IN ? AND orders.status IN ?", orphanCandidateIDs, orphanStatuses).
			Scan(&occupyingBills).Error; err != nil {
			return nil, err
		}
		for _, row := range occupyingBills {
			cur, ok := unfinishedByTable[row.TableID]
			if !ok {
				cur.SeatedAt = row.CreatedAt
			} else if usableSeatedInstant(row.CreatedAt) &&
				(!usableSeatedInstant(cur.SeatedAt) || row.CreatedAt.Before(cur.SeatedAt)) {
				cur.SeatedAt = row.CreatedAt
			}
			last := row.UpdatedAt
			if !usableSeatedInstant(last) {
				last = row.CreatedAt
			}
			if !ok || last.After(cur.LastSeen) {
				cur.LastSeen = last
			}
			unfinishedByTable[row.TableID] = cur
		}
	}

	type billPhysicalQuantity struct {
		BillID   uint  `gorm:"column:bill_id"`
		Quantity int64 `gorm:"column:quantity"`
	}
	physicalQuantityByBill := make(map[uint]int64, len(activeBillIDs))
	if len(activeBillIDs) > 0 {
		var quantities []billPhysicalQuantity
		if err := db.Model(&BillItem{}).
			// Sellable / covers units: menu_item + bundle parents. Expanded
			// bundle_item children and discounts must not inflate table chips
			// (4× Date Night → 4, not 16 component plates).
			Select(`bill_id, COALESCE(SUM(
				CASE
					-- Sellable lines only (menu + bundle parents). Bundle children
					-- are included components — counting them inflated floor totals
					-- ("25 prepared units" for a handful of Date Night bundles).
					WHEN COALESCE(NULLIF(item_type, ''), 'menu_item') IN ('menu_item', 'bundle')
						THEN CASE WHEN quantity > 0 THEN quantity ELSE 0 END
					ELSE 0
				END
			), 0) AS quantity`).
			Where("bill_id IN ?", activeBillIDs).
			Group("bill_id").
			Scan(&quantities).Error; err != nil {
			return nil, err
		}
		for _, quantity := range quantities {
			physicalQuantityByBill[quantity.BillID] = quantity.Quantity
		}
	}

	// Batch: the NEXT active reservation per table (single query). The host
	// stand renders only the soonest booking, so the floor poll never ships
	// a table's whole future book: ROW_NUMBER keeps one row per table and
	// idx_table_reservations_table_status_time serves the scan.
	// Hold through the no-show grace so a 5-minute-late confirmed booking
	// still marks the table Reserved (same clock as NEXT ARRIVAL).
	currentTime := time.Now().UTC()
	holdCutoff := reservationHoldCutoff(businessID, currentTime)
	var allReservations []TableReservation
	nextPerTable := db.Model(&TableReservation{}).
		Select(`id, business_id, table_id, customer_name, party_size,
			reservation_time, duration, status, confirmation_code,
			ROW_NUMBER() OVER (PARTITION BY table_id ORDER BY reservation_time ASC, id ASC) AS rn`).
		Where("table_id IN ? AND status IN ? AND reservation_time >= ?",
			tableIDs, []string{"pending", "confirmed"}, holdCutoff)
	if err := db.Table("(?) AS next_res", nextPerTable).
		Select("id", "business_id", "table_id", "customer_name", "party_size",
			"reservation_time", "duration", "status", "confirmation_code").
		Where("rn = 1").
		Order("reservation_time ASC").
		Find(&allReservations).Error; err != nil {
		return nil, err
	}
	resByTable := make(map[uint][]TableReservation, len(tables))
	for _, r := range allReservations {
		if r.TableID != nil {
			resByTable[*r.TableID] = append(resByTable[*r.TableID], r)
		}
	}

	// Batch staff names for the host Live View "server" column (created_by on
	// the open bill). One query — never N+1 per table.
	staffNameByID := map[uint]string{}
	staffIDs := make([]uint, 0, len(allBills))
	seenStaff := map[uint]struct{}{}
	for _, bill := range allBills {
		if bill.CreatedByStaffID == nil {
			continue
		}
		id := *bill.CreatedByStaffID
		if _, ok := seenStaff[id]; ok {
			continue
		}
		seenStaff[id] = struct{}{}
		staffIDs = append(staffIDs, id)
	}
	if len(staffIDs) > 0 {
		type staffNameRow struct {
			ID   uint   `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		var staffRows []staffNameRow
		if err := db.Model(&Staff{}).
			Select("id", "name").
			Where("business_id = ? AND id IN ?", businessID, staffIDs).
			Find(&staffRows).Error; err != nil {
			return nil, err
		}
		for _, row := range staffRows {
			staffNameByID[row.ID] = row.Name
		}
	}

	// Assemble results in memory.
	// Host-stand rule: any upcoming or late-within-grace pending/confirmed
	// reservation assigned to the table marks it Reserved. Occupied (open
	// bill) still wins. Past-grace Confirmed no longer holds the table.
	result := make([]map[string]interface{}, len(tables))
	for i, table := range tables {
		activeBills := billsByTable[table.ID]
		activeReservations := resByTable[table.ID]

		status := "available"
		occupying, unfinished := unfinishedByTable[table.ID]
		if len(activeBills) > 0 {
			status = "occupied"
		} else if unfinished {
			// Closed $0 / abandoned checks must not read Available while expo
			// is still cooking or a guest send is waiting (#704 T5 / T1).
			status = "occupied"
		} else if len(activeReservations) > 0 {
			status = "reserved"
		}

		var activeBillPhysicalItemQuantity int64
		var activeBillServerName string
		var seatedAt interface{}
		var lastSeen interface{}
		if len(activeBills) > 0 {
			activeBillPhysicalItemQuantity = physicalQuantityByBill[activeBills[0].ID]
			if activeBills[0].CreatedByStaffID != nil {
				activeBillServerName = staffNameByID[*activeBills[0].CreatedByStaffID]
			}
			seatedAt = jsonTimeOrNil(activeBills[0].CreatedAt)
			lastSeen = jsonTimeOrNil(activeBills[0].UpdatedAt)
			if lastSeen == nil {
				lastSeen = seatedAt
			}
		} else if unfinished {
			seatedAt = jsonTimeOrNil(occupying.SeatedAt)
			lastSeen = jsonTimeOrNil(occupying.LastSeen)
			if lastSeen == nil {
				lastSeen = seatedAt
			}
		}
		result[i] = map[string]interface{}{
			"table":                              table,
			"status":                             status,
			"active_bills":                       activeBills,
			"active_bills_count":                 len(activeBills),
			"active_bill_physical_item_quantity": activeBillPhysicalItemQuantity,
			"active_bill_server_name":            activeBillServerName,
			"seated_at":                          seatedAt,
			"last_seen":                          lastSeen,
			"reservations":                       activeReservations,
			"reservations_count":                 len(activeReservations),
		}
	}

	return result, nil
}

func usableSeatedInstant(value time.Time) bool {
	return !value.IsZero() && value.Year() >= 2020
}

func jsonTimeOrNil(value time.Time) interface{} {
	if !usableSeatedInstant(value) {
		return nil
	}
	return value
}
