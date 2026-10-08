package database

import (
	"encoding/json"
	"time"
)

const (
	InventoryMovementTypePurchase         = "purchase"
	InventoryMovementTypeManualAdjustment = "manual_adjustment"
	InventoryMovementTypeWaste            = "waste"
	InventoryMovementTypeRestock          = "restock"
	InventoryMovementTypeCorrection       = "correction"
	InventoryMovementTypeOrderConsumption = "order_consumption"
	InventoryMovementTypeOrderRestoration = "order_restoration"
)

const (
	InventoryAvailabilityModeWarn      = "warn"
	InventoryAvailabilityModeManual    = "manual"
	InventoryAvailabilityModeHardBlock = "hard_block"
)

// InventoryItem represents a stock-tracked ingredient or operational supply.
type InventoryItem struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	BusinessID uint   `gorm:"index;not null" json:"business_id"`
	Name       string `gorm:"not null" json:"name"`
	// Uniqueness of (business_id, sku) is enforced by a PARTIAL unique index
	// (WHERE sku <> '') created in the genesis schema — blank SKUs are valid,
	// optional operator items and must be able to coexist. GORM struct tags
	// can't express a partial WHERE, and a plain composite uniqueIndex tag
	// would make auto-migrate forbid multiple blank-SKU rows. So the SQL
	// migration is authoritative; the tag stays a plain non-unique index and
	// test harnesses create the partial index explicitly to mirror prod.
	SKU              string    `gorm:"index" json:"sku"`
	Category         string    `json:"category"`
	Unit             string    `gorm:"not null;default:'unit'" json:"unit"`
	CurrentQuantity  float64   `gorm:"default:0" json:"current_quantity"`
	ReorderThreshold float64   `gorm:"default:0" json:"reorder_threshold"`
	CostPerUnit      float64   `gorm:"default:0" json:"cost_per_unit"`
	IsActive         bool      `gorm:"default:true" json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// Business is a GORM association only. encoding/json never omits non-pointer
	// zero structs, so a bare Business{} used to dump ~3KB of empty business
	// fields on every inventory row (items/recipes/movements/settings). Clients
	// already receive business_id and never read nested business.
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// MarshalJSON emits a slim inventory_item nested on movements/recipes when the
// row was preloaded with only id/name/(cost) columns. A full default marshal
// would invent is_active:false, current_quantity:0, and zero timestamps that
// look authoritative on the operator UI. (FIND-042)
func (item InventoryItem) MarshalJSON() ([]byte, error) {
	// Partial projection heuristic: real items always have business_id + unit +
	// created_at. Movement preload Select("id","name","cost_per_unit") and
	// recipe Select("id","name") leave those zero.
	if item.ID != 0 && item.BusinessID == 0 && item.Unit == "" && item.CreatedAt.IsZero() {
		return json.Marshal(struct {
			ID          uint    `json:"id"`
			Name        string  `json:"name"`
			CostPerUnit float64 `json:"cost_per_unit"`
		}{
			ID:          item.ID,
			Name:        item.Name,
			CostPerUnit: item.CostPerUnit,
		})
	}
	type alias InventoryItem
	return json.Marshal(alias(item))
}

// InventoryRecipe maps a menu item to the stock items it consumes.
type InventoryRecipe struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	BusinessID       uint      `gorm:"index;not null" json:"business_id"`
	MenuItemID       string    `gorm:"index;not null" json:"menu_item_id"`
	MenuItemName     string    `json:"menu_item_name"`
	InventoryItemID  uint      `gorm:"index;not null" json:"inventory_item_id"`
	QuantityRequired float64   `gorm:"not null" json:"quantity_required"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	Business      Business      `gorm:"foreignKey:BusinessID" json:"-"`
	InventoryItem InventoryItem `gorm:"foreignKey:InventoryItemID" json:"inventory_item,omitempty"`
}

// InventoryMovement stores the append-only stock ledger.
type InventoryMovement struct {
	ID               uint    `gorm:"primaryKey" json:"id"`
	BusinessID       uint    `gorm:"index;not null;index:idx_inventory_movements_business_created,priority:1" json:"business_id"`
	InventoryItemID  uint    `gorm:"index;not null" json:"inventory_item_id"`
	MovementType     string  `gorm:"index;not null" json:"movement_type"`
	QuantityDelta    float64 `gorm:"not null" json:"quantity_delta"`
	QuantityBefore   float64 `gorm:"not null" json:"quantity_before"`
	QuantityAfter    float64 `gorm:"not null" json:"quantity_after"`
	Reason           string  `gorm:"type:text" json:"reason"`
	Actor            string  `json:"actor"`
	MenuItemID       string  `json:"menu_item_id"`
	MenuItemName     string  `json:"menu_item_name"`
	ReferenceOrderID *uint   `gorm:"index" json:"reference_order_id,omitempty"`
	ReferenceBillID  *uint   `gorm:"index" json:"reference_bill_id,omitempty"`
	// Composite (business_id, created_at) serves the movement-list LIMIT scan and
	// the waste-variance window aggregate.
	CreatedAt time.Time `json:"created_at" gorm:"index:idx_inventory_movements_business_created,priority:2,sort:desc"`
	UpdatedAt time.Time `json:"updated_at"`

	Business      Business      `gorm:"foreignKey:BusinessID" json:"-"`
	InventoryItem InventoryItem `gorm:"foreignKey:InventoryItemID" json:"inventory_item,omitempty"`
}

// InventorySettings stores business-level inventory behavior.
type InventorySettings struct {
	ID                        uint      `gorm:"primaryKey" json:"id"`
	BusinessID                uint      `gorm:"uniqueIndex;not null" json:"business_id"`
	InventoryEnabled          bool      `gorm:"default:false" json:"inventory_enabled"`
	AutoDeductOnOrderApproval bool      `gorm:"default:true" json:"auto_deduct_on_order_approval"`
	LowStockWarningsEnabled   bool      `gorm:"default:true" json:"low_stock_warnings_enabled"`
	AvailabilitySyncMode      string    `gorm:"default:'warn'" json:"availability_sync_mode"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`

	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

func (InventoryItem) TableName() string {
	return "inventory_items"
}

func (InventoryRecipe) TableName() string {
	return "inventory_recipes"
}

func (InventoryMovement) TableName() string {
	return "inventory_movements"
}

func (InventorySettings) TableName() string {
	return "inventory_settings"
}
