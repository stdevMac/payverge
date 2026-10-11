package database

import (
	"fmt"
	"time"
)

// MovementAggregate is one (inventory item, movement type) bucket of summed
// signed quantity deltas over a window. Used by the waste-variance service.
type MovementAggregate struct {
	InventoryItemID uint
	MovementType    string
	TotalDelta      float64
}

// AggregateInventoryMovements returns, for one business, the SUM of signed
// quantity_delta grouped by (inventory_item_id, movement_type) over [start,end).
// ONE grouped aggregate query — no row hydration, no N+1.
func (db *DB) AggregateInventoryMovements(businessID uint, start, end time.Time) ([]MovementAggregate, error) {
	var rows []MovementAggregate
	if err := db.GetGorm().
		Model(&InventoryMovement{}).
		Where("business_id = ? AND created_at >= ? AND created_at < ?", businessID, start, end).
		Group("inventory_item_id, movement_type").
		Select("inventory_item_id, movement_type, SUM(quantity_delta) as total_delta").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("AggregateInventoryMovements businessID=%d: %w", businessID, err)
	}
	return rows, nil
}

// InventoryItemDim is the costing dimension for an inventory item.
type InventoryItemDim struct {
	ID          uint
	Name        string
	Unit        string
	CostPerUnit float64
}

// GetInventoryItemDimsForBusiness returns id/name/unit/cost_per_unit for every
// inventory item of the business — the dimension table the waste-variance service
// joins recipe + movement aggregates against. It deliberately includes INACTIVE
// items: an item deactivated after it accrued waste/correction movements in the
// window must still be valued, otherwise tracked loss is understated and the row
// renders blank (no name/cost). ONE projected query. Callers only surface items
// that actually have recipe or movement activity, so inactive-but-idle items add
// nothing to the report.
func (db *DB) GetInventoryItemDimsForBusiness(businessID uint) ([]InventoryItemDim, error) {
	var rows []InventoryItemDim
	if err := db.GetGorm().
		Model(&InventoryItem{}).
		Where("business_id = ?", businessID).
		Select("id", "name", "unit", "cost_per_unit").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("GetInventoryItemDimsForBusiness businessID=%d: %w", businessID, err)
	}
	return rows, nil
}
