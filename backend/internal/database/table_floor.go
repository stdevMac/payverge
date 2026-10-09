package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/money"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Floor-ops error contract. Handlers map these to stable API codes so the host
// Live View can show restaurant-operator copy (seat / clear / transfer / merge)
// without parsing free-form error strings.
var (
	ErrFloorTableNotFound       = errors.New("table not found")
	ErrFloorTableInactive       = errors.New("table is inactive")
	ErrFloorTableOccupied       = errors.New("table already has an open check")
	ErrFloorNoActiveBill        = errors.New("table has no open check")
	ErrFloorSettleRequired      = errors.New("settle the open check before clearing")
	ErrFloorTargetOccupied      = errors.New("target table already has an open check")
	ErrFloorTargetSame          = errors.New("source and target tables must differ")
	ErrFloorBusinessMismatch    = errors.New("table does not belong to this business")
	ErrFloorMergePaymentsBlock  = errors.New("cannot merge checks that already have payments")
	ErrFloorBillNotTransferable = errors.New("only open or partial checks can be moved")
	ErrFloorLiveKitchenTickets  = errors.New("table still has live kitchen tickets")
	ErrFloorPendingOrders       = errors.New("table still has guest orders awaiting approval")
	ErrFloorMergeLoyaltyBlock   = errors.New("cannot merge: both checks carry a loyalty discount that cannot be returned")
)

// SeatWalkInInput opens an empty walk-in check on a free table.
type SeatWalkInInput struct {
	BusinessID uint
	TableID    uint
	PartySize  int
	Notes      string
	Actor      string
	StaffID    *uint
}

// TransferBillInput moves an active check from one table to another.
type TransferBillInput struct {
	BusinessID    uint
	SourceTableID uint
	TargetTableID uint
	Actor         string
}

// MergeTablesInput merges the source table's open check into the target's.
// When the target has no open check, this is equivalent to a transfer.
// Tax and service fee come from the business row, not the caller.
type MergeTablesInput struct {
	BusinessID    uint
	SourceTableID uint
	TargetTableID uint
	Actor         string
}

// ClearTableInput frees a table by voiding its empty unpaid open check.
type ClearTableInput struct {
	BusinessID uint
	TableID    uint
	Actor      string
}

// SeatWalkIn creates an empty open bill on an available table so the host can
// seat a walk-in party from Live View without leaving for Bills.
func SeatWalkIn(input SeatWalkInInput) (*Bill, error) {
	if input.BusinessID == 0 || input.TableID == 0 {
		return nil, fmt.Errorf("business and table are required")
	}

	var bill *Bill
	err := db.Transaction(func(tx *gorm.DB) error {
		table, err := loadFloorTableTx(tx, input.BusinessID, input.TableID)
		if err != nil {
			return err
		}
		if !table.IsActive {
			return ErrFloorTableInactive
		}

		existing, err := loadActiveBillForTableTx(tx, table.ID)
		if err == nil && existing != nil {
			return ErrFloorTableOccupied
		}
		if err != nil && !errors.Is(err, ErrNoActiveBill) {
			return err
		}
		// Abandoned/closed checks do not occupy the open-bill slot, but the
		// host must not seat over food still in the pass or a guest send
		// waiting in the queue (#704 T5 / T1).
		if err := refuseUnfinishedTableServiceTx(tx, table.ID); err != nil {
			return err
		}

		business, err := getBusinessByIDTx(tx, input.BusinessID)
		if err != nil {
			return err
		}

		notes := strings.TrimSpace(input.Notes)
		if notes == "" {
			if input.PartySize > 0 {
				notes = fmt.Sprintf("Walk-in · %d covers", input.PartySize)
			} else {
				notes = "Walk-in"
			}
		}

		created := &Bill{
			BusinessID:       input.BusinessID,
			TableID:          table.ID,
			Notes:            notes,
			Status:           BillStatusOpen,
			SettlementAddr:   business.SettlementAddr,
			TippingAddr:      business.TippingAddr,
			CreatedByStaffID: input.StaffID,
		}
		events := []BillHistoryEvent{{
			BusinessID: input.BusinessID,
			EventType:  BillHistoryEventBillCreated,
			Actor:      input.Actor,
			Details: map[string]interface{}{
				"source":     "live_view_seat",
				"table_id":   table.ID,
				"party_size": input.PartySize,
			},
		}}
		if err := CreateBillWithHistoryTx(tx, created, []BillItem{}, events); err != nil {
			if errors.Is(err, ErrActiveBillExists) {
				return ErrFloorTableOccupied
			}
			return err
		}
		bill = created
		return nil
	})
	return bill, err
}

// ClearTable voids an empty unpaid open check so the host can free the table
// after a no-order walk-in. A $0 check moved no money, so settled_at stays
// unset. Checks with items or captured payments must be settled (or voided
// via the bills flow) — Live View will not write off money.
func ClearTable(input ClearTableInput) (*Bill, error) {
	if input.BusinessID == 0 || input.TableID == 0 {
		return nil, fmt.Errorf("business and table are required")
	}

	var cleared *Bill
	err := db.Transaction(func(tx *gorm.DB) error {
		table, err := loadFloorTableTx(tx, input.BusinessID, input.TableID)
		if err != nil {
			return err
		}

		// Inspect the table, not only the current open check. Live Liberar on
		// T9 (open partial $18.04) returned settle_required because kitchen
		// tickets were only counted after the money door. Tickets can also sit
		// on a closed/abandoned bill for the same table while Live View reads
		// Available (#704 bill 1132 / ticket 1123). Kitchen and the approval
		// queue must win, and they must stay distinct.
		liveKitchen, err := countLiveKitchenTicketsForTableTx(tx, table.ID)
		if err != nil {
			return err
		}
		if liveKitchen > 0 {
			return ErrFloorLiveKitchenTickets
		}
		pendingOrders, err := countPendingOrdersForTableTx(tx, table.ID)
		if err != nil {
			return err
		}
		if pendingOrders > 0 {
			return ErrFloorPendingOrders
		}

		bill, err := loadActiveBillForTableTx(tx, table.ID)
		if err != nil {
			if errors.Is(err, ErrNoActiveBill) {
				return ErrFloorNoActiveBill
			}
			return err
		}

		if bill.Status != BillStatusOpen {
			return ErrFloorSettleRequired
		}
		if bill.PaidAmount > 0 || bill.TotalAmount > 0 {
			return ErrFloorSettleRequired
		}

		items, err := billItemsForBillSnapshotTx(tx, bill.ID, bill.Items)
		if err != nil {
			return err
		}
		if liveBillItemCount(items) > 0 {
			return ErrFloorSettleRequired
		}

		now := time.Now()
		result := tx.Model(&Bill{}).
			Where("id = ? AND status = ? AND paid_amount = 0 AND total_amount = 0", bill.ID, BillStatusOpen).
			Updates(map[string]interface{}{
				"status":     BillStatusVoided,
				"closed_at":  &now,
				"updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrFloorSettleRequired
		}

		event := BillHistoryEvent{
			BillID:     bill.ID,
			BusinessID: bill.BusinessID,
			EventType:  BillHistoryEventBillVoided,
			Actor:      input.Actor,
			Reason:     "Cleared from Live View",
			Details: map[string]interface{}{
				"source":   "live_view_clear",
				"table_id": table.ID,
			},
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}

		var refreshed Bill
		if err := tx.First(&refreshed, bill.ID).Error; err != nil {
			return err
		}
		cleared = &refreshed
		return nil
	})
	return cleared, err
}

// TransferActiveBill moves an open/partial check onto another free table.
func TransferActiveBill(input TransferBillInput) (*Bill, error) {
	if input.BusinessID == 0 || input.SourceTableID == 0 || input.TargetTableID == 0 {
		return nil, fmt.Errorf("business, source table, and target table are required")
	}
	if input.SourceTableID == input.TargetTableID {
		return nil, ErrFloorTargetSame
	}

	var moved *Bill
	err := db.Transaction(func(tx *gorm.DB) error {
		source, err := loadFloorTableTx(tx, input.BusinessID, input.SourceTableID)
		if err != nil {
			return err
		}
		target, err := loadFloorTableTx(tx, input.BusinessID, input.TargetTableID)
		if err != nil {
			return err
		}
		if !target.IsActive {
			return ErrFloorTableInactive
		}

		bill, err := loadActiveBillForTableTx(tx, source.ID)
		if err != nil {
			if errors.Is(err, ErrNoActiveBill) {
				return ErrFloorNoActiveBill
			}
			return err
		}
		if bill.Status != BillStatusOpen && bill.Status != BillStatusPartial {
			return ErrFloorBillNotTransferable
		}

		if _, err := loadActiveBillForTableTx(tx, target.ID); err == nil {
			return ErrFloorTargetOccupied
		} else if !errors.Is(err, ErrNoActiveBill) {
			return err
		}
		if err := refuseUnfinishedTableServiceTx(tx, target.ID); err != nil {
			return err
		}

		fromTableID := bill.TableID
		now := time.Now()
		if err := tx.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
			"table_id":   target.ID,
			"updated_at": now,
		}).Error; err != nil {
			if errors.Is(err, ErrActiveBillExists) || strings.Contains(strings.ToLower(err.Error()), "unique") {
				return ErrFloorTargetOccupied
			}
			return err
		}

		event := BillHistoryEvent{
			BillID:     bill.ID,
			BusinessID: bill.BusinessID,
			EventType:  BillHistoryEventBillUpdated,
			Actor:      input.Actor,
			Reason:     "Transferred from Live View",
			Details: map[string]interface{}{
				"source":        "live_view_transfer",
				"from_table_id": fromTableID,
				"to_table_id":   target.ID,
			},
		}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}

		// Keep a seated reservation assigned to the party when the host moves them.
		_ = tx.Model(&TableReservation{}).
			Where("business_id = ? AND table_id = ? AND status = ?", input.BusinessID, source.ID, "seated").
			Update("table_id", target.ID).Error

		var refreshed Bill
		if err := tx.First(&refreshed, bill.ID).Error; err != nil {
			return err
		}
		moved = &refreshed
		return nil
	})
	return moved, err
}

// MergeTableChecks folds the source table's open check into the target.
// Target empty → transfer. Both occupied → move unpaid items, void the $0 source.
func MergeTableChecks(input MergeTablesInput) (targetBill *Bill, sourceBill *Bill, err error) {
	if input.BusinessID == 0 || input.SourceTableID == 0 || input.TargetTableID == 0 {
		return nil, nil, fmt.Errorf("business, source table, and target table are required")
	}
	if input.SourceTableID == input.TargetTableID {
		return nil, nil, ErrFloorTargetSame
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		source, err := loadFloorTableTx(tx, input.BusinessID, input.SourceTableID)
		if err != nil {
			return err
		}
		target, err := loadFloorTableTx(tx, input.BusinessID, input.TargetTableID)
		if err != nil {
			return err
		}
		if !target.IsActive {
			return ErrFloorTableInactive
		}

		srcBill, err := loadActiveBillForTableTx(tx, source.ID)
		if err != nil {
			if errors.Is(err, ErrNoActiveBill) {
				return ErrFloorNoActiveBill
			}
			return err
		}

		tgtBill, tgtErr := loadActiveBillForTableTx(tx, target.ID)
		if tgtErr != nil && !errors.Is(tgtErr, ErrNoActiveBill) {
			return tgtErr
		}
		if errors.Is(tgtErr, ErrNoActiveBill) {
			// No target check — transfer is the merge. Still refuse if the
			// target table has orphaned kitchen/queue work.
			if err := refuseUnfinishedTableServiceTx(tx, target.ID); err != nil {
				return err
			}
			moved, transferErr := transferBillTx(tx, srcBill, source.ID, target.ID, input.Actor)
			if transferErr != nil {
				return transferErr
			}
			targetBill = moved
			sourceBill = nil
			return nil
		}

		if srcBill.PaidAmount > 0 || tgtBill.PaidAmount > 0 {
			return ErrFloorMergePaymentsBlock
		}
		if srcBill.Status != BillStatusOpen || tgtBill.Status != BillStatusOpen {
			return ErrFloorMergePaymentsBlock
		}

		srcItems, err := billItemsForBillSnapshotTx(tx, srcBill.ID, srcBill.Items)
		if err != nil {
			return err
		}
		tgtItems, err := billItemsForBillSnapshotTx(tx, tgtBill.ID, tgtBill.Items)
		if err != nil {
			return err
		}

		// Moved lines keep their ids: a line's id is how a later void finds
		// the kitchen line it came from (normalizeBillItemUUID(orderItem.ID)),
		// and how split assignments key it. Only a clash with a target line
		// gets a fresh id.
		merged := make([]BillItem, 0, len(tgtItems)+len(srcItems))
		merged = append(merged, tgtItems...)
		usedIDs := make(map[string]struct{}, len(tgtItems)+len(srcItems))
		for _, item := range tgtItems {
			usedIDs[item.ID] = struct{}{}
		}
		for _, item := range srcItems {
			movedItem := item
			if _, clash := usedIDs[movedItem.ID]; clash || movedItem.ID == "" {
				movedItem.ID = uuid.New().String()
			}
			usedIDs[movedItem.ID] = struct{}{}
			movedItem.BillID = tgtBill.ID
			merged = append(merged, movedItem)
		}
		// Free the source rows' primary keys before the target rewrites its
		// relational rows with the same ids.
		if err := tx.Where("bill_id = ?", srcBill.ID).Delete(&BillItem{}).Error; err != nil && !isMissingBillItemsRelationError(err) {
			return err
		}

		var business Business
		if err := tx.First(&business, input.BusinessID).Error; err != nil {
			return err
		}

		// Source redemption moves onto a clear target; otherwise the points go back.
		loyaltyMoved := false
		loyaltyPointsRestored := 0
		sourceRedeemed := srcBill.LoyaltyDiscountCents > 0 || srcBill.LoyaltyPointsRedeemed > 0
		targetClear := tgtBill.LoyaltyDiscountCents == 0 && tgtBill.LoyaltyPointsRedeemed == 0 && tgtBill.LoyaltyRedeemedByCustomerID == nil
		if sourceRedeemed && targetClear {
			tgtBill.LoyaltyDiscountCents = srcBill.LoyaltyDiscountCents
			tgtBill.LoyaltyPointsRedeemed = srcBill.LoyaltyPointsRedeemed
			tgtBill.LoyaltyRedeemedByCustomerID = srcBill.LoyaltyRedeemedByCustomerID
			loyaltyMoved = true
		} else if sourceRedeemed {
			// The target keeps its own redemption, so the source's points go
			// back to the guest. A discount with no points or redeemer to
			// return it to, or a redeemer with no active membership row, would
			// vanish silently: refuse the merge instead.
			if srcBill.LoyaltyPointsRedeemed <= 0 || srcBill.LoyaltyRedeemedByCustomerID == nil {
				return ErrFloorMergeLoyaltyBlock
			}
			restore := tx.Model(&CustomerBusiness{}).
				Where("customer_id = ? AND business_id = ? AND is_active = ?",
					*srcBill.LoyaltyRedeemedByCustomerID, srcBill.BusinessID, true).
				Update("loyalty_points", gorm.Expr("loyalty_points + ?", srcBill.LoyaltyPointsRedeemed))
			if restore.Error != nil {
				return fmt.Errorf("failed to restore loyalty points on merge: %w", restore.Error)
			}
			if restore.RowsAffected != 1 {
				return ErrFloorMergeLoyaltyBlock
			}
			loyaltyPointsRestored = srcBill.LoyaltyPointsRedeemed
		}

		applyBillTotals(tgtBill, merged, &business)

		if err := updateBillTx(tx, tgtBill, merged); err != nil {
			return err
		}
		if loyaltyMoved {
			if err := tx.Model(&Bill{}).Where("id = ?", tgtBill.ID).Updates(map[string]interface{}{
				"loyalty_discount_cents":          tgtBill.LoyaltyDiscountCents,
				"loyalty_points_redeemed":         tgtBill.LoyaltyPointsRedeemed,
				"loyalty_redeemed_by_customer_id": tgtBill.LoyaltyRedeemedByCustomerID,
			}).Error; err != nil {
				return err
			}
		}

		now := time.Now()
		if err := tx.Model(&Bill{}).Where("id = ?", srcBill.ID).Updates(map[string]interface{}{
			"status":                          BillStatusVoided,
			"subtotal":                        int64(0),
			"tax_amount":                      int64(0),
			"service_fee_amount":              int64(0),
			"total_amount":                    int64(0),
			"items":                           "[]",
			"loyalty_discount_cents":          int64(0),
			"loyalty_points_redeemed":         0,
			"loyalty_redeemed_by_customer_id": nil,
			"closed_at":                       &now,
			"updated_at":                      now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("bill_id = ?", srcBill.ID).Delete(&BillItem{}).Error; err != nil && !isMissingBillItemsRelationError(err) {
			return err
		}
		// The source check no longer exists as a service record: its items and
		// its money now live on the target. Its orders have to move with them,
		// or expo keeps cooking tickets that point at a voided $0 bill (#704).
		ordersMoved := tx.Model(&Order{}).
			Where("bill_id = ?", srcBill.ID).
			Updates(map[string]interface{}{
				"bill_id":    tgtBill.ID,
				"updated_at": now,
			})
		if ordersMoved.Error != nil {
			return fmt.Errorf("failed to move orders onto the merged check: %w", ordersMoved.Error)
		}

		targetDetails := map[string]interface{}{
			"source":            "live_view_merge",
			"merged_from_bill":  srcBill.ID,
			"merged_from_table": source.ID,
			"items_moved":       len(srcItems),
			"orders_moved":      ordersMoved.RowsAffected,
		}
		if loyaltyMoved {
			targetDetails["loyalty_moved"] = true
		}
		if loyaltyPointsRestored > 0 {
			targetDetails["loyalty_points_restored"] = loyaltyPointsRestored
		}
		events := []BillHistoryEvent{
			{
				BillID:     tgtBill.ID,
				BusinessID: tgtBill.BusinessID,
				EventType:  BillHistoryEventBillUpdated,
				Actor:      input.Actor,
				Reason:     "Merged from Live View",
				Details:    targetDetails,
			},
			{
				BillID:     srcBill.ID,
				BusinessID: srcBill.BusinessID,
				EventType:  BillHistoryEventBillVoided,
				Actor:      input.Actor,
				Reason:     "Merged into another table from Live View",
				Details: map[string]interface{}{
					"source":            "live_view_merge",
					"merged_into_bill":  tgtBill.ID,
					"merged_into_table": target.ID,
				},
			},
		}
		if err := tx.Create(&events).Error; err != nil {
			return err
		}

		_ = tx.Model(&TableReservation{}).
			Where("business_id = ? AND table_id = ? AND status = ?", input.BusinessID, source.ID, "seated").
			Update("table_id", target.ID).Error

		var refreshedTarget, refreshedSource Bill
		if err := tx.First(&refreshedTarget, tgtBill.ID).Error; err != nil {
			return err
		}
		if err := tx.First(&refreshedSource, srcBill.ID).Error; err != nil {
			return err
		}
		targetBill = &refreshedTarget
		sourceBill = &refreshedSource
		return nil
	})
	return targetBill, sourceBill, err
}

func transferBillTx(tx *gorm.DB, bill *Bill, fromTableID, toTableID uint, actor string) (*Bill, error) {
	now := time.Now()
	if err := tx.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"table_id":   toTableID,
		"updated_at": now,
	}).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrFloorTargetOccupied
		}
		return nil, err
	}
	event := BillHistoryEvent{
		BillID:     bill.ID,
		BusinessID: bill.BusinessID,
		EventType:  BillHistoryEventBillUpdated,
		Actor:      actor,
		Reason:     "Transferred from Live View (merge)",
		Details: map[string]interface{}{
			"source":        "live_view_merge_transfer",
			"from_table_id": fromTableID,
			"to_table_id":   toTableID,
		},
	}
	if err := tx.Create(&event).Error; err != nil {
		return nil, err
	}
	_ = tx.Model(&TableReservation{}).
		Where("business_id = ? AND table_id = ? AND status = ?", bill.BusinessID, fromTableID, "seated").
		Update("table_id", toTableID).Error

	var refreshed Bill
	if err := tx.First(&refreshed, bill.ID).Error; err != nil {
		return nil, err
	}
	return &refreshed, nil
}

func loadFloorTableTx(tx *gorm.DB, businessID, tableID uint) (*Table, error) {
	var table Table
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", tableID).First(&table).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFloorTableNotFound
		}
		return nil, err
	}
	if table.BusinessID != businessID {
		return nil, ErrFloorBusinessMismatch
	}
	return &table, nil
}

func loadActiveBillForTableTx(tx *gorm.DB, tableID uint) (*Bill, error) {
	var bill Bill
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("table_id = ? AND status IN ?", tableID, activeBillStatusStrings()).
		Order("created_at DESC").
		First(&bill).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoActiveBill
		}
		return nil, err
	}
	return &bill, nil
}

func getBusinessByIDTx(tx *gorm.DB, businessID uint) (*Business, error) {
	var business Business
	if err := tx.First(&business, businessID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("business not found")
		}
		return nil, err
	}
	return &business, nil
}

func billItemsForBillSnapshotTx(tx *gorm.DB, billID uint, snapshot string) ([]BillItem, error) {
	var items []BillItem
	err := tx.Where("bill_id = ?", billID).Find(&items).Error
	if err == nil && len(items) > 0 {
		return hydrateBillItemsIfNeeded(tx, billID, items)
	}
	if err != nil && !isMissingBillItemsRelationError(err) {
		return nil, err
	}
	return billItemsFromJSONSnapshot(billID, snapshot)
}

// countLiveKitchenTicketsTx counts tickets expo still owes this check
// (approved / in_kitchen / ready). Terminal tickets — delivered, cancelled —
// and never-fired pending ones are history and do not block a bill action.
func countLiveKitchenTicketsTx(tx *gorm.DB, billID uint) (int64, error) {
	var live int64
	if err := tx.Model(&Order{}).
		Where("bill_id = ? AND status IN ?", billID, KitchenLiveOrderStatuses()).
		Count(&live).Error; err != nil {
		return 0, fmt.Errorf("failed to check kitchen tickets: %w", err)
	}
	return live, nil
}

// RefuseUnfinishedTableServiceTx is the shared leftover-service door used by
// walk-in Liberar/seat/transfer and reservation check-in / Live View POST
// /seat. Abandoned and closed $0 checks do not occupy the open-bill slot, but
// a host must not sit a new party while expo still owns the table (#704 T5 /
// ticket 1123).
func RefuseUnfinishedTableServiceTx(tx *gorm.DB, tableID uint) error {
	return refuseUnfinishedTableServiceTx(tx, tableID)
}

// countLiveKitchenTicketsForTableTx counts live kitchen work still attached to
// any bill on this table — including closed $0 / abandoned checks that no
// longer occupy the open-bill slot.
func refuseUnfinishedTableServiceTx(tx *gorm.DB, tableID uint) error {
	liveKitchen, err := countLiveKitchenTicketsForTableTx(tx, tableID)
	if err != nil {
		return err
	}
	if liveKitchen > 0 {
		return ErrFloorLiveKitchenTickets
	}
	pendingOrders, err := countPendingOrdersForTableTx(tx, tableID)
	if err != nil {
		return err
	}
	if pendingOrders > 0 {
		return ErrFloorPendingOrders
	}
	return nil
}

func countLiveKitchenTicketsForTableTx(tx *gorm.DB, tableID uint) (int64, error) {
	var live int64
	if err := tx.Model(&Order{}).
		Joins("JOIN bills ON bills.id = orders.bill_id").
		Where("bills.table_id = ? AND orders.status IN ?", tableID, KitchenLiveOrderStatuses()).
		Count(&live).Error; err != nil {
		return 0, fmt.Errorf("failed to check kitchen tickets: %w", err)
	}
	return live, nil
}

func countPendingOrdersForTableTx(tx *gorm.DB, tableID uint) (int64, error) {
	var pending int64
	if err := tx.Model(&Order{}).
		Joins("JOIN bills ON bills.id = orders.bill_id").
		Where("bills.table_id = ? AND orders.status = ?", tableID, OrderStatusPending).
		Count(&pending).Error; err != nil {
		return 0, fmt.Errorf("failed to check pending orders: %w", err)
	}
	return pending, nil
}

func liveBillItemCount(items []BillItem) int {
	count := 0
	for _, item := range items {
		if item.Quantity <= 0 {
			continue
		}
		if money.IsInformationalBillLine(item.ItemType) {
			continue
		}
		count++
	}
	return count
}
