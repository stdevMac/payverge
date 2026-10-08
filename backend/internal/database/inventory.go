package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type InventoryRecipeReplacement struct {
	InventoryItemID  uint    `json:"inventory_item_id"`
	QuantityRequired float64 `json:"quantity_required"`
}

type InventoryItemHealth struct {
	ID               uint    `json:"id"`
	Name             string  `json:"name"`
	SKU              string  `json:"sku"`
	Category         string  `json:"category"`
	Unit             string  `json:"unit"`
	CurrentQuantity  float64 `json:"current_quantity"`
	ReorderThreshold float64 `json:"reorder_threshold"`
	Status           string  `json:"status"`
}

type InventoryMenuItemStatus struct {
	MenuItemID           string   `json:"menu_item_id"`
	MenuItemName         string   `json:"menu_item_name"`
	CategoryName         string   `json:"category_name"`
	ManualAvailable      bool     `json:"manual_available"`
	HasRecipe            bool     `json:"has_recipe"`
	Status               string   `json:"status"`
	MaxPossibleServings  int      `json:"max_possible_servings"`
	RecommendedAvailable bool     `json:"recommended_available"`
	ShowsWarning         bool     `json:"shows_warning"`
	BlocksSale           bool     `json:"blocks_sale"`
	AffectedInventory    []string `json:"affected_inventory"`
	WarningInventory     []string `json:"warning_inventory"`
}

type InventorySummary struct {
	Settings        InventorySettings `json:"settings"`
	TotalItems      int               `json:"total_items"`
	LowStockItems   int               `json:"low_stock_items"`
	OutOfStockItems int               `json:"out_of_stock_items"`
	// TotalStockValue is the sum of max(0, current_quantity) * cost_per_unit
	// across active items, in business-currency dollars (inventory costs are
	// stored as float dollars, not cents). Single source of truth for the stat
	// card so the FE stops recomputing it from the full item list (audit §3.5 LOW).
	TotalStockValue     float64                   `json:"total_stock_value"`
	TotalRecipes        int                       `json:"total_recipes"`
	MenuItemsTracked    int                       `json:"menu_items_tracked"`
	MenuItemsLowStock   int                       `json:"menu_items_low_stock"`
	MenuItemsOutOfStock int                       `json:"menu_items_out_of_stock"`
	LowStockDetails     []InventoryItemHealth     `json:"low_stock_details"`
	OutOfStockDetails   []InventoryItemHealth     `json:"out_of_stock_details"`
	MenuItemStatuses    []InventoryMenuItemStatus `json:"menu_item_statuses"`
}

type inventoryMenuLookup struct {
	ID       string
	Name     string
	Category string
}

type inventoryMovementMetadata struct {
	MovementType     string
	Reason           string
	Actor            string
	MenuItemID       string
	MenuItemName     string
	ReferenceOrderID *uint
	ReferenceBillID  *uint
}

type orderInventoryDemand struct {
	MenuItemID   string
	MenuItemName string
	Quantity     int
}

func getMenuCategoriesTx(tx *gorm.DB, businessID uint) ([]MenuCategory, error) {
	var menu Menu
	if err := tx.Where("business_id = ? AND is_active = ?", businessID, true).First(&menu).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []MenuCategory{}, nil
		}
		return nil, fmt.Errorf("failed to get menu: %w", err)
	}

	if strings.TrimSpace(menu.Categories) == "" {
		return []MenuCategory{}, nil
	}

	var categories []MenuCategory
	if err := json.Unmarshal([]byte(menu.Categories), &categories); err != nil {
		return nil, fmt.Errorf("failed to unmarshal categories: %w", err)
	}

	return categories, nil
}

func getOrCreateInventorySettingsTx(tx *gorm.DB, businessID uint) (*InventorySettings, error) {
	var settings InventorySettings
	err := tx.Where("business_id = ?", businessID).First(&settings).Error
	if err == nil {
		if strings.TrimSpace(settings.AvailabilitySyncMode) == "" {
			settings.AvailabilitySyncMode = InventoryAvailabilityModeWarn
		}
		return &settings, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to load inventory settings: %w", err)
	}

	settings = InventorySettings{
		BusinessID:                businessID,
		InventoryEnabled:          false,
		AutoDeductOnOrderApproval: true,
		LowStockWarningsEnabled:   true,
		AvailabilitySyncMode:      InventoryAvailabilityModeWarn,
	}
	// ON CONFLICT DO NOTHING on the UNIQUE(business_id) index: two orders
	// approved concurrently for a business that never touched inventory would
	// both miss the First() and race to INSERT. A bare Create would raise a
	// unique violation on the loser and abort the whole order-approval
	// transaction (spurious 500 on the money path). DoNothing + re-read makes the
	// first-touch idempotent.
	res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "business_id"}}, DoNothing: true}).Create(&settings)
	if res.Error != nil {
		return nil, fmt.Errorf("failed to create inventory settings: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Lost the race — the row now exists; read the winner's copy.
		if err := tx.Where("business_id = ?", businessID).First(&settings).Error; err != nil {
			return nil, fmt.Errorf("failed to load inventory settings after conflict: %w", err)
		}
		if strings.TrimSpace(settings.AvailabilitySyncMode) == "" {
			settings.AvailabilitySyncMode = InventoryAvailabilityModeWarn
		}
	}
	return &settings, nil
}

func GetInventorySettings(businessID uint) (*InventorySettings, error) {
	// Read-first: the steady-state case (row already exists) must not open a
	// write-capable transaction. GetInventorySettings is on the guest AI-waiter
	// hot path (UnrecommendableMenuItemIDs), which is called per chat message, so
	// a BEGIN/COMMIT per read is pure overhead once the row exists.
	var settings InventorySettings
	err := db.Where("business_id = ?", businessID).First(&settings).Error
	if err == nil {
		if strings.TrimSpace(settings.AvailabilitySyncMode) == "" {
			settings.AvailabilitySyncMode = InventoryAvailabilityModeWarn
		}
		return &settings, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to load inventory settings: %w", err)
	}

	// First touch: create under a transaction to serialize with concurrent
	// callers.
	var created *InventorySettings
	if txErr := db.Transaction(func(tx *gorm.DB) error {
		var err error
		created, err = getOrCreateInventorySettingsTx(tx, businessID)
		return err
	}); txErr != nil {
		return nil, txErr
	}
	return created, nil
}

func UpsertInventorySettings(businessID uint, updates map[string]interface{}) (*InventorySettings, error) {
	var settings *InventorySettings
	err := db.Transaction(func(tx *gorm.DB) error {
		current, err := getOrCreateInventorySettingsTx(tx, businessID)
		if err != nil {
			return err
		}
		if len(updates) > 0 {
			updates["updated_at"] = time.Now()
			if err := tx.Model(&InventorySettings{}).
				Where("business_id = ?", businessID).
				Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to update inventory settings: %w", err)
			}
			if err := tx.Where("business_id = ?", businessID).First(current).Error; err != nil {
				return fmt.Errorf("failed to reload inventory settings: %w", err)
			}
			if err := advanceGuestOrderabilityRevisionTx(tx, businessID); err != nil {
				return err
			}
		}
		settings = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// advanceGuestOrderabilityRevisionTx invalidates guest-menu validators after
// any write that can change the authoritative inventory projection. Reusing
// businesses.updated_at keeps guest reads query-neutral: the existing cached
// business lookup supplies the revision, while each mutation pays one UPDATE.
func advanceGuestOrderabilityRevisionTx(tx *gorm.DB, businessID uint) error {
	if err := tx.Model(&Business{}).
		Where("id = ?", businessID).
		UpdateColumn("updated_at", time.Now().UTC()).Error; err != nil {
		return fmt.Errorf("failed to advance guest orderability revision: %w", err)
	}
	return nil
}

func ListInventoryItemsByBusinessID(businessID uint, includeInactive bool) ([]InventoryItem, error) {
	query := db.Where("business_id = ?", businessID)
	if !includeInactive {
		query = query.Where("is_active = ?", true)
	}

	var items []InventoryItem
	if err := query.Order("name ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to list inventory items: %w", err)
	}
	// Presentation-quantize so operators never see binary residue on read
	// (write path also quantizes; this covers pre-fix noisy rows).
	for i := range items {
		items[i].CurrentQuantity = QuantizeInventoryQuantity(items[i].CurrentQuantity)
		items[i].ReorderThreshold = QuantizeInventoryQuantity(items[i].ReorderThreshold)
		items[i].CostPerUnit = QuantizeInventoryQuantity(items[i].CostPerUnit)
	}
	return items, nil
}

func GetInventoryItemByID(businessID, itemID uint) (*InventoryItem, error) {
	var item InventoryItem
	if err := db.Where("business_id = ? AND id = ?", businessID, itemID).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("inventory item not found")
		}
		return nil, fmt.Errorf("failed to get inventory item: %w", err)
	}
	return &item, nil
}

// inventoryQuantityPrecision is 6 decimal places — enough for gram/ml recipe
// fractions while collapsing binary float residue (29.920000000000016 → 29.92).
const inventoryQuantityPrecision = 1e6

// QuantizeInventoryQuantity rounds stock amounts so repeated fractional
// recipe deductions do not accumulate IEEE-754 display noise in the API.
func QuantizeInventoryQuantity(q float64) float64 {
	if math.IsNaN(q) || math.IsInf(q, 0) {
		return q
	}
	return math.Round(q*inventoryQuantityPrecision) / inventoryQuantityPrecision
}

func applyInventoryMovementTx(
	tx *gorm.DB,
	item *InventoryItem,
	quantityDelta float64,
	metadata inventoryMovementMetadata,
) error {
	// Quantize both ends so the ledger balances and noisy historical rows
	// self-heal on the next movement without a data migration.
	before := QuantizeInventoryQuantity(item.CurrentQuantity)
	after := QuantizeInventoryQuantity(before + quantityDelta)
	appliedDelta := QuantizeInventoryQuantity(after - before)

	if err := tx.Model(&InventoryItem{}).
		Where("id = ?", item.ID).
		Updates(map[string]interface{}{
			"current_quantity": after,
			"updated_at":       time.Now(),
		}).Error; err != nil {
		return fmt.Errorf("failed to update inventory quantity: %w", err)
	}

	movement := InventoryMovement{
		BusinessID:       item.BusinessID,
		InventoryItemID:  item.ID,
		MovementType:     metadata.MovementType,
		QuantityDelta:    appliedDelta,
		QuantityBefore:   before,
		QuantityAfter:    after,
		Reason:           metadata.Reason,
		Actor:            metadata.Actor,
		MenuItemID:       metadata.MenuItemID,
		MenuItemName:     metadata.MenuItemName,
		ReferenceOrderID: metadata.ReferenceOrderID,
		ReferenceBillID:  metadata.ReferenceBillID,
	}
	if err := tx.Create(&movement).Error; err != nil {
		return fmt.Errorf("failed to create inventory movement: %w", err)
	}

	item.CurrentQuantity = after
	item.UpdatedAt = time.Now()
	return nil
}

func validateInventoryItemValues(item *InventoryItem) error {
	item.CurrentQuantity = QuantizeInventoryQuantity(item.CurrentQuantity)
	item.ReorderThreshold = QuantizeInventoryQuantity(item.ReorderThreshold)
	item.CostPerUnit = QuantizeInventoryQuantity(item.CostPerUnit)
	if math.IsNaN(item.CurrentQuantity) || math.IsInf(item.CurrentQuantity, 0) {
		return fmt.Errorf("current quantity must be a valid number")
	}
	if item.CurrentQuantity < 0 {
		return fmt.Errorf("current quantity cannot be negative")
	}
	if math.IsNaN(item.ReorderThreshold) || math.IsInf(item.ReorderThreshold, 0) {
		return fmt.Errorf("reorder threshold must be a valid number")
	}
	if item.ReorderThreshold < 0 {
		return fmt.Errorf("reorder threshold cannot be negative")
	}
	if math.IsNaN(item.CostPerUnit) || math.IsInf(item.CostPerUnit, 0) {
		return fmt.Errorf("cost per unit must be a valid number")
	}
	if item.CostPerUnit < 0 {
		return fmt.Errorf("cost per unit cannot be negative")
	}
	return nil
}

func CreateInventoryItem(item *InventoryItem, actor string) error {
	item.Name = strings.TrimSpace(item.Name)
	item.Unit = strings.TrimSpace(item.Unit)
	if item.Name == "" {
		return fmt.Errorf("inventory item name is required")
	}
	if item.Unit == "" {
		item.Unit = "unit"
	}
	if err := validateInventoryItemValues(item); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		initialQuantity := item.CurrentQuantity
		item.CurrentQuantity = 0
		if err := tx.Create(item).Error; err != nil {
			return err
		}
		if initialQuantity != 0 {
			if err := applyInventoryMovementTx(tx, item, initialQuantity, inventoryMovementMetadata{
				MovementType: InventoryMovementTypeCorrection,
				Reason:       "Initial stock level",
				Actor:        actor,
			}); err != nil {
				return err
			}
		}
		return advanceGuestOrderabilityRevisionTx(tx, item.BusinessID)
	})
}

func UpdateInventoryItem(item *InventoryItem, previous InventoryItem, actor string) error {
	item.Name = strings.TrimSpace(item.Name)
	item.Unit = strings.TrimSpace(item.Unit)
	if item.Name == "" {
		return fmt.Errorf("inventory item name is required")
	}
	if item.Unit == "" {
		item.Unit = "unit"
	}
	if err := validateInventoryItemValues(item); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// Lock and re-read the row so the operator's quantity change is applied as
		// a delta against the LIVE quantity, not their stale (unlocked) snapshot.
		// Otherwise a concurrent deduction/adjustment committed between the
		// handler's read and this write would be silently clobbered, diverging
		// on-hand quantity from the movement ledger.
		var locked InventoryItem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("business_id = ? AND id = ?", item.BusinessID, item.ID).
			First(&locked).Error; err != nil {
			return fmt.Errorf("failed to lock inventory item: %w", err)
		}

		// The operator's intended change, relative to the value they saw.
		quantityDelta := item.CurrentQuantity - previous.CurrentQuantity

		// Persist the non-quantity edits while keeping the live quantity, so Save
		// cannot overwrite a concurrent change; the delta is applied below.
		item.CurrentQuantity = locked.CurrentQuantity
		if err := tx.Omit(clause.Associations).Save(item).Error; err != nil {
			return err
		}

		if quantityDelta != 0 {
			if locked.CurrentQuantity+quantityDelta < 0 {
				return fmt.Errorf("quantity change would reduce %s below zero", locked.Name)
			}
			// applyInventoryMovementTx sets item.CurrentQuantity to
			// locked.CurrentQuantity + quantityDelta and records the movement.
			if err := applyInventoryMovementTx(tx, item, quantityDelta, inventoryMovementMetadata{
				MovementType: InventoryMovementTypeCorrection,
				Reason:       "Stock level updated from inventory item editor",
				Actor:        actor,
			}); err != nil {
				return err
			}
		}

		if previous.IsActive && !item.IsActive {
			if err := tx.Where("business_id = ? AND inventory_item_id = ?", item.BusinessID, item.ID).
				Delete(&InventoryRecipe{}).Error; err != nil {
				return fmt.Errorf("failed to remove inventory recipes: %w", err)
			}
		}

		return advanceGuestOrderabilityRevisionTx(tx, item.BusinessID)
	})
}

func DeactivateInventoryItem(businessID, itemID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&InventoryItem{}).
			Where("business_id = ? AND id = ?", businessID, itemID).
			Updates(map[string]interface{}{
				"is_active":  false,
				"updated_at": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("failed to deactivate inventory item: %w", err)
		}

		if err := tx.Where("business_id = ? AND inventory_item_id = ?", businessID, itemID).
			Delete(&InventoryRecipe{}).Error; err != nil {
			return fmt.Errorf("failed to remove inventory recipes: %w", err)
		}

		return advanceGuestOrderabilityRevisionTx(tx, businessID)
	})
}

func ListInventoryRecipesByBusinessID(businessID uint) ([]InventoryRecipe, error) {
	var recipes []InventoryRecipe
	if err := db.Where("business_id = ?", businessID).
		Preload("InventoryItem", func(db *gorm.DB) *gorm.DB {
			// Project only the columns consumed by this preload's callers:
			//   - GetInventorySummary reads recipe.InventoryItem.Name as a label
			//     fallback when the item is absent/inactive in its active-item map.
			//   - The direct HTTP endpoint (ListInventoryRecipes) serialises the
			//     nested object. Operators and API clients read
			//     inventory_item.cost_per_unit next to the items list; omitting
			//     it zeroed every recipe cost on the wire.
			return db.Select("id", "name", "cost_per_unit")
		}).
		Order("menu_item_name ASC, inventory_item_id ASC").
		Find(&recipes).Error; err != nil {
		return nil, fmt.Errorf("failed to list inventory recipes: %w", err)
	}
	if len(recipes) == 0 {
		return recipes, nil
	}
	_, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		categories = []MenuCategory{}
	}
	byID, byName := buildInventoryMenuLookup(categories)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return reconcileInventoryRecipeMenuItems(tx, recipes, byID, byName)
	}); err != nil {
		return nil, err
	}
	return recipes, nil
}

func ReplaceInventoryRecipeForMenuItem(businessID uint, menuItemID, menuItemName string, entries []InventoryRecipeReplacement) error {
	menuItemID = strings.TrimSpace(menuItemID)
	menuItemName = strings.TrimSpace(menuItemName)
	if menuItemID == "" {
		return fmt.Errorf("menu item ID is required")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		categories, err := getMenuCategoriesTx(tx, businessID)
		if err != nil {
			return err
		}
		byID, byName := buildInventoryMenuLookup(categories)
		resolved := resolveInventoryRecipeMenuItem(InventoryRecipe{
			MenuItemID:   menuItemID,
			MenuItemName: menuItemName,
		}, byID, byName)
		if strings.TrimSpace(resolved.ID) != "" {
			menuItemID = strings.TrimSpace(resolved.ID)
		}
		if strings.TrimSpace(resolved.Name) != "" {
			menuItemName = strings.TrimSpace(resolved.Name)
		}

		var activeItems []InventoryItem
		if len(entries) > 0 {
			itemIDs := make([]uint, 0, len(entries))
			for _, entry := range entries {
				if entry.InventoryItemID == 0 {
					return fmt.Errorf("inventory item ID is required")
				}
				if entry.QuantityRequired <= 0 {
					return fmt.Errorf("recipe quantity must be greater than zero")
				}
				itemIDs = append(itemIDs, entry.InventoryItemID)
			}
			if err := tx.Where("business_id = ? AND id IN ? AND is_active = ?", businessID, itemIDs, true).
				Find(&activeItems).Error; err != nil {
				return fmt.Errorf("failed to validate inventory items: %w", err)
			}
			if len(activeItems) != len(entries) {
				return fmt.Errorf("one or more inventory items are invalid or inactive")
			}
		}

		if err := tx.Where("business_id = ? AND menu_item_id = ?", businessID, menuItemID).
			Delete(&InventoryRecipe{}).Error; err != nil {
			return fmt.Errorf("failed to replace inventory recipe: %w", err)
		}

		if len(entries) > 0 {
			recipes := make([]InventoryRecipe, 0, len(entries))
			for _, entry := range entries {
				recipes = append(recipes, InventoryRecipe{
					BusinessID:       businessID,
					MenuItemID:       menuItemID,
					MenuItemName:     menuItemName,
					InventoryItemID:  entry.InventoryItemID,
					QuantityRequired: QuantizeInventoryQuantity(entry.QuantityRequired),
				})
			}
			if err := tx.Create(&recipes).Error; err != nil {
				return fmt.Errorf("failed to save inventory recipe: %w", err)
			}
		}
		return advanceGuestOrderabilityRevisionTx(tx, businessID)
	})
}

// InventoryMovementFilter narrows a movement-ledger read. All fields are
// optional: a zero-value filter is "the newest page of the whole business
// ledger", matching the legacy list. ItemID scopes to one item (the item
// drawer's per-item history), MovementType scopes to one movement kind (the
// Activity tab's type chip), and Limit/Offset page the result. This is the
// server-side ledger the frontend previously faked by filtering 25 global rows
// in the browser (audit §3.5 HIGH/C1).
type InventoryMovementFilter struct {
	ItemID       uint   // 0 = all items
	MovementType string // "" = all types
	Limit        int    // page size (clamped 1..200, default 50)
	Offset       int    // page offset (clamped >= 0)
}

// EffectiveLimit resolves the requested Limit against the same default/cap the
// query applies, so handlers can compute a page offset and echo the effective
// page size without duplicating the clamp bounds.
func (f InventoryMovementFilter) EffectiveLimit() int {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return limit
}

// ListInventoryMovementsFiltered returns one page of a business's inventory
// movement ledger plus the total count of rows matching the filter, so the UI
// can paginate honestly. The scope (business + optional item + optional type)
// is applied identically to the COUNT and the page read, so the total always
// matches what paging would return. Rows are newest-first, projecting only the
// InventoryItem columns the operator UI renders.
func ListInventoryMovementsFiltered(businessID uint, filter InventoryMovementFilter) ([]InventoryMovement, int64, error) {
	limit := filter.EffectiveLimit()
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	// One scoped query builder reused for COUNT and the page read so both agree.
	scope := func(q *gorm.DB) *gorm.DB {
		q = q.Where("business_id = ?", businessID)
		if filter.ItemID > 0 {
			q = q.Where("inventory_item_id = ?", filter.ItemID)
		}
		if filter.MovementType != "" {
			q = q.Where("movement_type = ?", filter.MovementType)
		}
		return q
	}

	var total int64
	if err := scope(db.Model(&InventoryMovement{})).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count inventory movements: %w", err)
	}

	var movements []InventoryMovement
	if err := scope(db.Model(&InventoryMovement{})).
		Preload("InventoryItem", func(db *gorm.DB) *gorm.DB {
			// Same projection as the legacy list — FE renders name + cost_per_unit.
			return db.Select("id", "name", "cost_per_unit")
		}).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&movements).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list inventory movements: %w", err)
	}
	return movements, total, nil
}

func normalizeAdjustmentMovementType(movementType string) string {
	switch strings.TrimSpace(strings.ToLower(movementType)) {
	case InventoryMovementTypePurchase:
		return InventoryMovementTypePurchase
	case InventoryMovementTypeWaste:
		return InventoryMovementTypeWaste
	case InventoryMovementTypeRestock:
		return InventoryMovementTypeRestock
	case InventoryMovementTypeCorrection:
		return InventoryMovementTypeCorrection
	default:
		return InventoryMovementTypeManualAdjustment
	}
}

// applyLockedInventoryMovementTx is the shared core for stock mutations that an
// operator initiates against a single item: it locks the row FOR UPDATE,
// computes the delta from the LIVE quantity (via computeDelta), enforces the
// non-negative floor, records the movement, and reloads the persisted movement
// row so the caller can return it. Both the relative adjustment path
// (CreateInventoryAdjustment) and the absolute physical-count path
// (RecordInventoryCount) reuse this so the lock + ledger semantics stay
// identical. The delta is computed inside the lock so an absolute count
// reconciles against the live value, not a stale client snapshot (INV-M1).
func applyLockedInventoryMovementTx(
	tx *gorm.DB,
	businessID, itemID uint,
	movementType, reason, actor string,
	computeDelta func(live float64) float64,
) (*InventoryItem, *InventoryMovement, error) {
	var item InventoryItem
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("business_id = ? AND id = ?", businessID, itemID).
		First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("inventory item not found")
		}
		return nil, nil, fmt.Errorf("failed to load inventory item: %w", err)
	}

	delta := computeDelta(item.CurrentQuantity)
	if item.CurrentQuantity+delta < 0 {
		return nil, nil, fmt.Errorf("quantity change would reduce %s below zero", item.Name)
	}

	if err := applyInventoryMovementTx(tx, &item, delta, inventoryMovementMetadata{
		MovementType: movementType,
		Reason:       reason,
		Actor:        actor,
	}); err != nil {
		return nil, nil, err
	}

	entry := &InventoryMovement{}
	if err := tx.Where(
		"business_id = ? AND inventory_item_id = ? AND movement_type = ? AND quantity_after = ?",
		businessID,
		item.ID,
		movementType,
		item.CurrentQuantity,
	).Order("id DESC").First(entry).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to reload inventory movement: %w", err)
	}

	return &item, entry, nil
}

func CreateInventoryAdjustment(
	businessID uint,
	itemID uint,
	movementType string,
	quantityDelta float64,
	reason string,
	actor string,
) (*InventoryItem, *InventoryMovement, error) {
	var updatedItem *InventoryItem
	var movement *InventoryMovement

	movementType = normalizeAdjustmentMovementType(movementType)
	reason = strings.TrimSpace(reason)
	if quantityDelta == 0 {
		return nil, nil, fmt.Errorf("quantity change must be non-zero")
	}
	if movementType == InventoryMovementTypeWaste && quantityDelta > 0 {
		quantityDelta = -quantityDelta
	}
	if (movementType == InventoryMovementTypePurchase || movementType == InventoryMovementTypeRestock) && quantityDelta < 0 {
		quantityDelta = math.Abs(quantityDelta)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		var e error
		updatedItem, movement, e = applyLockedInventoryMovementTx(
			tx, businessID, itemID, movementType, reason, actor,
			func(float64) float64 { return quantityDelta },
		)
		if e != nil {
			return e
		}
		return advanceGuestOrderabilityRevisionTx(tx, businessID)
	})
	if err != nil {
		return nil, nil, err
	}

	return updatedItem, movement, nil
}

// RecordInventoryCount reconciles an item's on-hand quantity to an ABSOLUTE
// physically-counted value. The delta is computed inside the row lock as
// (target - live), so the operator's count always wins even if orders deducted
// stock between the page load and the save (INV-M1). A zero-variance count
// (target == live) is recorded as a correction movement so the ledger carries
// the verification, unlike a relative zero-delta adjustment which is rejected.
func RecordInventoryCount(
	businessID uint,
	itemID uint,
	target float64,
	reason string,
	actor string,
) (*InventoryItem, *InventoryMovement, error) {
	if math.IsNaN(target) || math.IsInf(target, 0) {
		return nil, nil, fmt.Errorf("counted quantity must be a valid number")
	}
	if target < 0 {
		return nil, nil, fmt.Errorf("counted quantity cannot be negative")
	}
	reason = strings.TrimSpace(reason)

	var updatedItem *InventoryItem
	var movement *InventoryMovement
	err := db.Transaction(func(tx *gorm.DB) error {
		var e error
		updatedItem, movement, e = applyLockedInventoryMovementTx(
			tx, businessID, itemID, InventoryMovementTypeCorrection, reason, actor,
			func(live float64) float64 { return target - live },
		)
		if e != nil {
			return e
		}
		return advanceGuestOrderabilityRevisionTx(tx, businessID)
	})
	if err != nil {
		return nil, nil, err
	}

	return updatedItem, movement, nil
}

func buildInventoryMenuLookup(categories []MenuCategory) (map[string]inventoryMenuLookup, map[string]inventoryMenuLookup) {
	byID := make(map[string]inventoryMenuLookup)
	byName := make(map[string]inventoryMenuLookup)
	for _, category := range categories {
		for _, item := range category.Items {
			lookup := inventoryMenuLookup{
				ID:       strings.TrimSpace(item.ID),
				Name:     strings.TrimSpace(item.Name),
				Category: strings.TrimSpace(category.Name),
			}
			if lookup.ID != "" {
				byID[lookup.ID] = lookup
			}
			if lookup.Name != "" {
				byName[strings.ToLower(lookup.Name)] = lookup
			}
		}
	}
	return byID, byName
}

// resolveInventoryRecipeMenuItem maps a recipe onto the live menu dish it
// actually describes. Prefer the stored menu_item_id whenever it points at a
// live dish — including when menu_item_name names a DIFFERENT live dish.
//
// #727 (and its B2 review): recipes are one row per (dish, ingredient), so a
// multi-ingredient dish legitimately owns several rows sharing its id, and a
// conflicted row cannot be disambiguated by names — the id is the strong key
// and names drift ({id:demo-steak, name:"Harvest Bowl"} is the steak's beef
// row, {id:demo-bowl, name:"Steak Plate"} is the bowl's second ingredient).
// Trusting the id is also exactly how the guest orderability path behaves
// (OutOfStockMenuItemIDs JOINs on the stored id), so operator chips, summary,
// consume, and guest QR hide all agree without persisting any guess.
// The stored name only decides when the id no longer matches any live dish
// (rewritten menus, dead demo ids).
func resolveInventoryRecipeMenuItem(recipe InventoryRecipe, byID, byName map[string]inventoryMenuLookup) inventoryMenuLookup {
	idKey := strings.TrimSpace(recipe.MenuItemID)
	nameKey := strings.ToLower(strings.TrimSpace(recipe.MenuItemName))
	byIDHit, hasID := byID[idKey]
	if hasID {
		return byIDHit
	}
	if nameKey != "" {
		if byNameHit, hasName := byName[nameKey]; hasName {
			return byNameHit
		}
	}
	return inventoryMenuLookup{ID: idKey, Name: strings.TrimSpace(recipe.MenuItemName)}
}

// inventoryRecipeMenuItemConflicted reports whether a recipe row's stored id
// and stored name point at two DIFFERENT live dishes. Such a row is resolved
// at read time (id wins, see resolveInventoryRecipeMenuItem) but must never be
// healed in the database: a heuristic can guess wrong, and a wrong heal bakes
// a recipe row onto a dish that never had one (#727 B2).
func inventoryRecipeMenuItemConflicted(recipe InventoryRecipe, byID, byName map[string]inventoryMenuLookup) bool {
	idKey := strings.TrimSpace(recipe.MenuItemID)
	nameKey := strings.ToLower(strings.TrimSpace(recipe.MenuItemName))
	byIDHit, hasID := byID[idKey]
	if !hasID || nameKey == "" {
		return false
	}
	byNameHit, hasName := byName[nameKey]
	return hasName && byNameHit.ID != byIDHit.ID
}

func inventoryRecipeOwnerKey(menuItemID string, inventoryItemID uint) string {
	return strings.TrimSpace(menuItemID) + "\x00" + fmt.Sprint(inventoryItemID)
}

// reconcileInventoryRecipeMenuItems writes resolved live menu_item_id values
// onto recipe rows whose stored id no longer matches any live dish (rewritten
// menus, dead demo ids) — consume and OutOfStockMenuItemIDs SELECT the stored
// id, so a dead id would silently stop constraining anything. It only heals
// rows whose resolution is unambiguous: a CONFLICTED row (stored id and name
// naming two different live dishes) is skipped entirely and resolved id-wins
// at read time instead (#727 B2 — a heuristic heal must never bake a recipe
// row onto a recipe-less dish).
func reconcileInventoryRecipeMenuItems(tx *gorm.DB, recipes []InventoryRecipe, byID, byName map[string]inventoryMenuLookup) error {
	if len(recipes) == 0 {
		return nil
	}
	occupied := make(map[string]uint, len(recipes))
	for i := range recipes {
		occupied[inventoryRecipeOwnerKey(recipes[i].MenuItemID, recipes[i].InventoryItemID)] = recipes[i].ID
	}
	for i := range recipes {
		recipe := &recipes[i]
		if inventoryRecipeMenuItemConflicted(*recipe, byID, byName) {
			continue
		}
		resolved := resolveInventoryRecipeMenuItem(*recipe, byID, byName)
		wantID := strings.TrimSpace(resolved.ID)
		wantName := strings.TrimSpace(resolved.Name)
		if wantID == "" {
			continue
		}
		curID := strings.TrimSpace(recipe.MenuItemID)
		curName := strings.TrimSpace(recipe.MenuItemName)
		if curID == wantID && (wantName == "" || curName == wantName) {
			continue
		}
		if wantID != curID {
			newKey := inventoryRecipeOwnerKey(wantID, recipe.InventoryItemID)
			if otherID, exists := occupied[newKey]; exists && otherID != recipe.ID {
				if err := tx.Where("id = ?", recipe.ID).Delete(&InventoryRecipe{}).Error; err != nil {
					return fmt.Errorf("failed to drop drifted inventory recipe: %w", err)
				}
				delete(occupied, inventoryRecipeOwnerKey(curID, recipe.InventoryItemID))
				recipe.MenuItemID = wantID
				if wantName != "" {
					recipe.MenuItemName = wantName
				}
				continue
			}
			delete(occupied, inventoryRecipeOwnerKey(curID, recipe.InventoryItemID))
			occupied[newKey] = recipe.ID
		}
		updates := map[string]interface{}{"menu_item_id": wantID}
		if wantName != "" {
			updates["menu_item_name"] = wantName
		}
		if err := tx.Model(&InventoryRecipe{}).Where("id = ?", recipe.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to correct inventory recipe menu item: %w", err)
		}
		recipe.MenuItemID = wantID
		if wantName != "" {
			recipe.MenuItemName = wantName
		}
	}
	return nil
}

// ReconcileInventoryRecipeMenuItemIDsTx writes resolved live menu_item_id
// values onto DEAD-id recipe rows inside the caller-owned transaction, so a
// leftover demo id cannot silently stop constraining availability at pay time.
// Conflicted rows (id and name naming two live dishes) are left untouched and
// resolve id-wins at read time (#727 B2).
func ReconcileInventoryRecipeMenuItemIDsTx(tx *gorm.DB, businessID uint, categories []MenuCategory) error {
	return reconcileInventoryRecipeMenuItemsForBusinessTx(tx, businessID, categories)
}

// ResolvedInventoryRecipeMenuItemID is the shared drift mapper used by
// summary, consume, OOS, chips, and checkout projection. A conflicted id/name
// pair resolves id-wins (#727 B2), matching OutOfStockMenuItemIDs' JOIN on the
// stored id, so every read path agrees without reconcile persisting a guess.
func ResolvedInventoryRecipeMenuItemID(recipe InventoryRecipe, categories []MenuCategory) string {
	byID, byName := buildInventoryMenuLookup(categories)
	return strings.TrimSpace(resolveInventoryRecipeMenuItem(recipe, byID, byName).ID)
}

func reconcileInventoryRecipeMenuItemsForBusinessTx(tx *gorm.DB, businessID uint, categories []MenuCategory) error {
	var recipes []InventoryRecipe
	if err := tx.Where("business_id = ?", businessID).Find(&recipes).Error; err != nil {
		return fmt.Errorf("failed to load inventory recipes for reconcile: %w", err)
	}
	if len(recipes) == 0 {
		return nil
	}
	if categories == nil {
		loaded, err := getMenuCategoriesTx(tx, businessID)
		if err != nil {
			return err
		}
		categories = loaded
	}
	byID, byName := buildInventoryMenuLookup(categories)
	return reconcileInventoryRecipeMenuItems(tx, recipes, byID, byName)
}

func GetInventorySummary(businessID uint) (*InventorySummary, error) {
	settings, err := GetInventorySettings(businessID)
	if err != nil {
		return nil, err
	}
	return GetInventorySummaryWithSettings(businessID, settings)
}

// GetInventorySummaryWithSettings builds the summary from settings the caller
// already loaded, so hot paths that gate on InventoryEnabled first do not read
// the settings row twice.
func GetInventorySummaryWithSettings(businessID uint, settings *InventorySettings) (*InventorySummary, error) {
	items, err := ListInventoryItemsByBusinessID(businessID, false)
	if err != nil {
		return nil, err
	}

	recipes, err := ListInventoryRecipesByBusinessID(businessID)
	if err != nil {
		return nil, err
	}

	_, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		categories = []MenuCategory{}
	}

	if !settings.InventoryEnabled {
		menuStatuses := make([]InventoryMenuItemStatus, 0)
		for _, category := range categories {
			for _, menuItem := range category.Items {
				menuItemID := strings.TrimSpace(menuItem.ID)
				status := InventoryMenuItemStatus{
					MenuItemID:           menuItemID,
					MenuItemName:         menuItem.Name,
					CategoryName:         category.Name,
					ManualAvailable:      menuItem.IsAvailable,
					HasRecipe:            false,
					Status:               "untracked",
					MaxPossibleServings:  -1,
					RecommendedAvailable: menuItem.IsAvailable,
					ShowsWarning:         false,
					BlocksSale:           false,
					AffectedInventory:    []string{},
					WarningInventory:     []string{},
				}
				if !menuItem.IsAvailable {
					status.Status = "manual_unavailable"
					status.RecommendedAvailable = false
					status.BlocksSale = true
				}
				menuStatuses = append(menuStatuses, status)
			}
		}

		return &InventorySummary{
			Settings:            *settings,
			TotalItems:          len(items),
			LowStockItems:       0,
			OutOfStockItems:     0,
			TotalRecipes:        len(recipes),
			MenuItemsTracked:    0,
			MenuItemsLowStock:   0,
			MenuItemsOutOfStock: 0,
			LowStockDetails:     []InventoryItemHealth{},
			OutOfStockDetails:   []InventoryItemHealth{},
			MenuItemStatuses:    menuStatuses,
		}, nil
	}

	itemByID := make(map[uint]InventoryItem, len(items))
	lowStockDetails := make([]InventoryItemHealth, 0)
	outOfStockDetails := make([]InventoryItemHealth, 0)
	lowStockCount := 0
	outOfStockCount := 0
	// Total on-hand value: sum of max(0, qty) * cost_per_unit across items, so
	// the FE reads one number instead of recomputing over the whole item list.
	totalStockValue := 0.0

	for _, item := range items {
		itemByID[item.ID] = item
		if item.CurrentQuantity > 0 {
			totalStockValue += item.CurrentQuantity * item.CostPerUnit
		}
		switch {
		case item.CurrentQuantity <= 0:
			outOfStockCount++
			outOfStockDetails = append(outOfStockDetails, InventoryItemHealth{
				ID:               item.ID,
				Name:             item.Name,
				SKU:              item.SKU,
				Category:         item.Category,
				Unit:             item.Unit,
				CurrentQuantity:  item.CurrentQuantity,
				ReorderThreshold: item.ReorderThreshold,
				Status:           "out_of_stock",
			})
		case item.ReorderThreshold > 0 && item.CurrentQuantity <= item.ReorderThreshold:
			lowStockCount++
			lowStockDetails = append(lowStockDetails, InventoryItemHealth{
				ID:               item.ID,
				Name:             item.Name,
				SKU:              item.SKU,
				Category:         item.Category,
				Unit:             item.Unit,
				CurrentQuantity:  item.CurrentQuantity,
				ReorderThreshold: item.ReorderThreshold,
				Status:           "low_stock",
			})
		}
	}

	byMenuID, byMenuName := buildInventoryMenuLookup(categories)
	recipesByMenuItem := make(map[string][]InventoryRecipe)
	for _, recipe := range recipes {
		resolved := resolveInventoryRecipeMenuItem(recipe, byMenuID, byMenuName)
		key := strings.TrimSpace(resolved.ID)
		if key == "" {
			continue
		}
		recipesByMenuItem[key] = append(recipesByMenuItem[key], recipe)
	}

	menuStatuses := make([]InventoryMenuItemStatus, 0)
	menuTrackedCount := 0
	menuLowCount := 0
	menuOutCount := 0

	for _, category := range categories {
		for _, menuItem := range category.Items {
			menuItemID := strings.TrimSpace(menuItem.ID)
			entry := InventoryMenuItemStatus{
				MenuItemID:           menuItemID,
				MenuItemName:         menuItem.Name,
				CategoryName:         category.Name,
				ManualAvailable:      menuItem.IsAvailable,
				HasRecipe:            false,
				Status:               "untracked",
				MaxPossibleServings:  -1,
				RecommendedAvailable: menuItem.IsAvailable,
				ShowsWarning:         false,
				BlocksSale:           !menuItem.IsAvailable,
				AffectedInventory:    []string{},
				WarningInventory:     []string{},
			}

			linkedRecipes := recipesByMenuItem[menuItemID]
			if len(linkedRecipes) == 0 {
				if !menuItem.IsAvailable {
					entry.Status = "manual_unavailable"
					entry.RecommendedAvailable = false
				}
				menuStatuses = append(menuStatuses, entry)
				continue
			}

			entry.HasRecipe = true
			menuTrackedCount++
			maxPossible := math.MaxFloat64
			outOfStock := false

			for _, recipe := range linkedRecipes {
				item, exists := itemByID[recipe.InventoryItemID]
				if !exists || !item.IsActive {
					outOfStock = true
					itemLabel := strings.TrimSpace(recipe.InventoryItem.Name)
					if itemLabel == "" {
						itemLabel = strings.TrimSpace(recipe.MenuItemName)
					}
					entry.AffectedInventory = append(entry.AffectedInventory, itemLabel)
					continue
				}

				if recipe.QuantityRequired <= 0 {
					continue
				}

				possible := math.Floor(item.CurrentQuantity / recipe.QuantityRequired)
				if possible < maxPossible {
					maxPossible = possible
				}
				if item.CurrentQuantity < recipe.QuantityRequired || item.CurrentQuantity <= 0 {
					outOfStock = true
					entry.AffectedInventory = append(entry.AffectedInventory, item.Name)
				} else if item.ReorderThreshold > 0 && item.CurrentQuantity <= item.ReorderThreshold {
					entry.WarningInventory = append(entry.WarningInventory, item.Name)
				}
			}

			// -1 is the "no constraining recipe / untracked" sentinel. A computed
			// negative (warn-mode oversell drove an ingredient below zero) clamps
			// to 0 so the UI never shows negative servings, and a runaway-large
			// value is capped to keep the int conversion well-defined (INV-L4).
			switch {
			case maxPossible == math.MaxFloat64:
				maxPossible = -1
			case maxPossible < 0:
				maxPossible = 0
			case maxPossible > float64(math.MaxInt32):
				maxPossible = float64(math.MaxInt32)
			}
			entry.MaxPossibleServings = int(maxPossible)

			switch {
			case !menuItem.IsAvailable:
				entry.Status = "manual_unavailable"
				entry.RecommendedAvailable = false
				entry.ShowsWarning = false
				entry.BlocksSale = true
			case outOfStock:
				entry.Status = "out_of_stock"
				entry.RecommendedAvailable = settings.AvailabilitySyncMode == InventoryAvailabilityModeManual
				entry.ShowsWarning = settings.AvailabilitySyncMode != InventoryAvailabilityModeManual
				// Warn and hard_block both 86 recipe-linked dishes at zero stock.
				// Manual mode leaves sellability to the operator's is_available flag.
				entry.BlocksSale = settings.AvailabilitySyncMode != InventoryAvailabilityModeManual
				menuOutCount++
			case len(entry.WarningInventory) > 0:
				entry.Status = "low_stock"
				entry.ShowsWarning = settings.LowStockWarningsEnabled
				menuLowCount++
			default:
				entry.Status = "ok"
			}

			menuStatuses = append(menuStatuses, entry)
		}
	}

	// Stock value is dollars on the wire — round to cents so operators never
	// see IEEE residue (36*4.2 → 151.20000000000002 → total 687.032). (FIND-043)
	totalStockValue = math.Round(totalStockValue*100) / 100

	return &InventorySummary{
		Settings:            *settings,
		TotalItems:          len(items),
		LowStockItems:       lowStockCount,
		OutOfStockItems:     outOfStockCount,
		TotalStockValue:     totalStockValue,
		TotalRecipes:        len(recipes),
		MenuItemsTracked:    menuTrackedCount,
		MenuItemsLowStock:   menuLowCount,
		MenuItemsOutOfStock: menuOutCount,
		LowStockDetails:     lowStockDetails,
		OutOfStockDetails:   outOfStockDetails,
		MenuItemStatuses:    menuStatuses,
	}, nil
}

// UnrecommendableMenuItemIDs returns the set of menu item IDs the AI waiter
// must not proactively recommend or take orders for, because they are either
// blocked for sale or genuinely out of stock (no makeable serving). It reuses
// the canonical GetInventorySummary status computation so the assistant's view
// of availability never drifts from the inventory dashboard's. Warn and
// hard_block both exclude depleted recipe dishes (BlocksSale); manual mode
// leaves sellability to the operator's is_available flag.
func UnrecommendableMenuItemIDs(businessID uint) (map[string]bool, error) {
	summary, err := GetInventorySummary(businessID)
	if err != nil {
		return nil, err
	}
	hidden := make(map[string]bool)
	for _, st := range summary.MenuItemStatuses {
		id := strings.TrimSpace(st.MenuItemID)
		if id == "" {
			continue
		}
		if st.BlocksSale || st.Status == "out_of_stock" {
			hidden[id] = true
		}
	}
	return hidden, nil
}

// OutOfStockMenuItemIDs returns the set of menu item IDs whose linked
// ingredients cannot currently produce a single serving. It is the set-based
// counterpart of GetInventorySummary's per-item "out_of_stock" status
// (and of UnrecommendableMenuItemIDs above). Drifted recipe ids are written
// onto the live dish first so the JOIN cannot 86 Harvest Bowl for beef.
//
// Canonical out-of-stock semantics, mirrored from GetInventorySummary:
//   - a recipe whose inventory item is missing or inactive is out of stock;
//   - a recipe with quantity_required <= 0 never constrains availability;
//   - otherwise strict current_quantity < quantity_required — exactly enough
//     stock for one serving counts as available (intentional).
//
// Both JOIN sides are scoped by business_id so a recipe can never match
// another tenant's inventory row. Callers own the policy of WHEN this set
// hides dishes (see AvailabilitySyncMode); this function only answers what is
// physically out of stock.
func OutOfStockMenuItemIDs(businessID uint) (map[string]bool, error) {
	if err := db.Transaction(func(tx *gorm.DB) error {
		return ReconcileInventoryRecipeMenuItemIDsTx(tx, businessID, nil)
	}); err != nil {
		return nil, err
	}

	var ids []string
	err := db.Table("inventory_recipes r").
		Select("DISTINCT r.menu_item_id").
		Joins("LEFT JOIN inventory_items i ON i.id = r.inventory_item_id AND i.business_id = r.business_id").
		Where("r.business_id = ? AND (i.id IS NULL OR i.is_active = ? OR (r.quantity_required > 0 AND i.current_quantity < r.quantity_required))", businessID, false).
		Scan(&ids).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out[id] = true
	}
	return out, nil
}

func shouldConsumeInventoryForOrderItem(itemType string) bool {
	switch strings.TrimSpace(strings.ToLower(itemType)) {
	case "", "menu_item", "bundle_item":
		return true
	default:
		return false
	}
}

func resolveInventoryMenuItem(orderItem OrderItem, byID, byName map[string]inventoryMenuLookup) inventoryMenuLookup {
	menuItemID := strings.TrimSpace(orderItem.MenuItemID)
	if menuItemID != "" {
		if resolved, ok := byID[menuItemID]; ok {
			return resolved
		}
	}
	name := strings.TrimSpace(orderItem.MenuItemName)
	if name != "" {
		if resolved, ok := byName[strings.ToLower(name)]; ok {
			return resolved
		}
	}
	return inventoryMenuLookup{
		ID:   menuItemID,
		Name: name,
	}
}

func buildOrderInventoryDemand(orderItems []OrderItem, byID, byName map[string]inventoryMenuLookup) map[string]orderInventoryDemand {
	demand := make(map[string]orderInventoryDemand)
	for _, orderItem := range orderItems {
		if !shouldConsumeInventoryForOrderItem(orderItem.ItemType) {
			continue
		}

		resolved := resolveInventoryMenuItem(orderItem, byID, byName)
		menuItemID := strings.TrimSpace(resolved.ID)
		if menuItemID == "" {
			continue
		}

		quantity := orderItem.Quantity
		if quantity <= 0 {
			quantity = 1
		}

		current := demand[menuItemID]
		current.MenuItemID = menuItemID
		current.Quantity += quantity
		if strings.TrimSpace(current.MenuItemName) == "" {
			current.MenuItemName = strings.TrimSpace(resolved.Name)
		}
		demand[menuItemID] = current
	}
	return demand
}

func inventoryRecipesByMenuItem(recipes []InventoryRecipe) map[string][]InventoryRecipe {
	recipesByMenuItem := make(map[string][]InventoryRecipe)
	for _, recipe := range recipes {
		key := strings.TrimSpace(recipe.MenuItemID)
		if key == "" {
			continue
		}
		recipesByMenuItem[key] = append(recipesByMenuItem[key], recipe)
	}
	return recipesByMenuItem
}

func inventoryItemsByID(items []InventoryItem) map[uint]*InventoryItem {
	itemsByID := make(map[uint]*InventoryItem, len(items))
	for i := range items {
		itemsByID[items[i].ID] = &items[i]
	}
	return itemsByID
}

func uniqueSortedUintIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return []uint{}
	}
	seen := make(map[uint]struct{}, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func DeductApprovedOrderInventoryTx(tx *gorm.DB, order *Order, orderItems []OrderItem, actor string) error {
	settings, err := getOrCreateInventorySettingsTx(tx, order.BusinessID)
	if err != nil {
		return err
	}
	if !settings.InventoryEnabled || !settings.AutoDeductOnOrderApproval {
		return nil
	}

	var existingCount int64
	if err := tx.Model(&InventoryMovement{}).
		Where("business_id = ? AND reference_order_id = ? AND movement_type = ?", order.BusinessID, order.ID, InventoryMovementTypeOrderConsumption).
		Count(&existingCount).Error; err != nil {
		return fmt.Errorf("failed to verify inventory movement state: %w", err)
	}
	if existingCount > 0 {
		return nil
	}

	categories, err := getMenuCategoriesTx(tx, order.BusinessID)
	if err != nil {
		// Fail the approval transaction rather than silently skipping stock
		// deduction (empty categories → empty demand → return nil success).
		return fmt.Errorf("failed to load menu categories for inventory deduction: %w", err)
	}
	byID, byName := buildInventoryMenuLookup(categories)
	if err := ReconcileInventoryRecipeMenuItemIDsTx(tx, order.BusinessID, categories); err != nil {
		return fmt.Errorf("failed to reconcile inventory recipe menu items: %w", err)
	}

	demandByMenuItem := buildOrderInventoryDemand(orderItems, byID, byName)
	if len(demandByMenuItem) == 0 {
		return nil
	}

	menuItemIDs := make([]string, 0, len(demandByMenuItem))
	for menuItemID := range demandByMenuItem {
		menuItemIDs = append(menuItemIDs, menuItemID)
	}
	sort.Strings(menuItemIDs)

	var recipes []InventoryRecipe
	if err := tx.Where("business_id = ? AND menu_item_id IN ?", order.BusinessID, menuItemIDs).Find(&recipes).Error; err != nil {
		return fmt.Errorf("failed to load inventory recipes: %w", err)
	}
	if len(recipes) == 0 {
		return nil
	}

	inventoryItemIDs := make([]uint, 0, len(recipes))
	for _, recipe := range recipes {
		inventoryItemIDs = append(inventoryItemIDs, recipe.InventoryItemID)
	}
	inventoryItemIDs = uniqueSortedUintIDs(inventoryItemIDs)
	if len(inventoryItemIDs) == 0 {
		return nil
	}

	var inventoryItems []InventoryItem
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("business_id = ? AND id IN ?", order.BusinessID, inventoryItemIDs).
		Order("id ASC").
		Find(&inventoryItems).Error; err != nil {
		return fmt.Errorf("failed to load inventory items for deduction: %w", err)
	}

	recipesByMenuItem := inventoryRecipesByMenuItem(recipes)
	inventoryByID := inventoryItemsByID(inventoryItems)
	hardBlock := settings.AvailabilitySyncMode == InventoryAvailabilityModeHardBlock

	type plannedConsumption struct {
		item   *InventoryItem
		delta  float64
		demand orderInventoryDemand
	}
	type accumulatedNeed struct {
		item   *InventoryItem
		need   float64
		labels []string
	}
	planned := make([]plannedConsumption, 0)
	needs := make(map[uint]*accumulatedNeed)
	labelFor := func(demand orderInventoryDemand) string {
		menuItemLabel := strings.TrimSpace(demand.MenuItemName)
		if menuItemLabel == "" {
			menuItemLabel = strings.TrimSpace(demand.MenuItemID)
		}
		if menuItemLabel == "" {
			menuItemLabel = "this item"
		}
		return menuItemLabel
	}

	// Hard block checks every required ingredient before any movement is
	// written, so a later failure cannot leave a partial deduction when the
	// caller does not roll the transaction back. Demand for one inventory
	// item is summed across dishes: two plates that each fit on their own
	// can still exceed stock together.
	for _, menuItemID := range menuItemIDs {
		demand := demandByMenuItem[menuItemID]
		linkedRecipes := recipesByMenuItem[demand.MenuItemID]
		if len(linkedRecipes) == 0 {
			continue
		}

		for _, recipe := range linkedRecipes {
			if recipe.QuantityRequired <= 0 {
				continue
			}

			inventoryItem, exists := inventoryByID[recipe.InventoryItemID]
			if !exists || !inventoryItem.IsActive {
				if hardBlock {
					return fmt.Errorf(
						"inventory hard block prevents approving order: a required ingredient for %s is unavailable",
						labelFor(demand),
					)
				}
				continue
			}

			quantityDelta := -recipe.QuantityRequired * float64(demand.Quantity)
			if hardBlock {
				acc := needs[inventoryItem.ID]
				if acc == nil {
					acc = &accumulatedNeed{item: inventoryItem}
					needs[inventoryItem.ID] = acc
				}
				acc.need += -quantityDelta
				label := labelFor(demand)
				if len(acc.labels) == 0 || acc.labels[len(acc.labels)-1] != label {
					acc.labels = append(acc.labels, label)
				}
			}
			planned = append(planned, plannedConsumption{
				item:   inventoryItem,
				delta:  quantityDelta,
				demand: demand,
			})
		}
	}

	if hardBlock {
		needIDs := make([]uint, 0, len(needs))
		for id := range needs {
			needIDs = append(needIDs, id)
		}
		sort.Slice(needIDs, func(i, j int) bool { return needIDs[i] < needIDs[j] })
		for _, id := range needIDs {
			acc := needs[id]
			if acc.item.CurrentQuantity-acc.need < 0 {
				label := strings.Join(acc.labels, ", ")
				if label == "" {
					label = "this item"
				}
				return fmt.Errorf(
					"inventory hard block prevents approving order: %s does not have enough stock for %s",
					acc.item.Name,
					label,
				)
			}
		}
	}

	inventoryChanged := false
	for _, move := range planned {
		if err := applyInventoryMovementTx(tx, move.item, move.delta, inventoryMovementMetadata{
			MovementType:     InventoryMovementTypeOrderConsumption,
			Reason:           fmt.Sprintf("Consumed by approved order %s", order.OrderNumber),
			Actor:            actor,
			MenuItemID:       move.demand.MenuItemID,
			MenuItemName:     move.demand.MenuItemName,
			ReferenceOrderID: &order.ID,
			ReferenceBillID:  &order.BillID,
		}); err != nil {
			return err
		}
		inventoryChanged = true
	}

	if inventoryChanged {
		return advanceGuestOrderabilityRevisionTx(tx, order.BusinessID)
	}
	return nil
}

// orderConsumptionMenuItemID is the menu_item_id buildOrderInventoryDemand stores
// on consumption movements. A bill line keeps orderItem.MenuItemID, or the dish
// name when that id was empty; catalog resolution must use the same key.
func orderConsumptionMenuItemID(tx *gorm.DB, businessID uint, menuItemID string) (string, error) {
	menuItemID = strings.TrimSpace(menuItemID)
	if menuItemID == "" {
		return "", nil
	}
	categories, err := getMenuCategoriesTx(tx, businessID)
	if err != nil {
		return "", err
	}
	byID, byName := buildInventoryMenuLookup(categories)
	resolved := resolveInventoryMenuItem(OrderItem{
		MenuItemID:   menuItemID,
		MenuItemName: menuItemID,
	}, byID, byName)
	id := strings.TrimSpace(resolved.ID)
	if id == "" {
		return menuItemID, nil
	}
	return id, nil
}

type inventoryLineKey struct {
	inventoryItemID uint
	menuItemID      string
}

type inventoryLineNet struct {
	inventoryItemID uint
	menuItemID      string
	menuItemName    string
	referenceBillID *uint
	net             float64
}

// positiveOrderInventoryNets groups consumption (negative) and restoration
// (positive) deltas. net = -(sumConsumption + sumRestoration); only net > 0
// is returned, so a second restore is a no-op.
func positiveOrderInventoryNets(movements []InventoryMovement, menuItemID string) []inventoryLineNet {
	filter := strings.TrimSpace(menuItemID)
	type acc struct {
		sum             float64
		menuItemName    string
		referenceBillID *uint
	}
	grouped := make(map[inventoryLineKey]*acc)
	var keys []inventoryLineKey
	for i := range movements {
		movement := &movements[i]
		if movement.MovementType != InventoryMovementTypeOrderConsumption &&
			movement.MovementType != InventoryMovementTypeOrderRestoration {
			continue
		}
		menuID := strings.TrimSpace(movement.MenuItemID)
		if filter != "" && menuID != filter {
			continue
		}
		key := inventoryLineKey{inventoryItemID: movement.InventoryItemID, menuItemID: menuID}
		row := grouped[key]
		if row == nil {
			row = &acc{}
			grouped[key] = row
			keys = append(keys, key)
		}
		row.sum += movement.QuantityDelta
		if movement.MovementType == InventoryMovementTypeOrderConsumption {
			if row.menuItemName == "" {
				row.menuItemName = movement.MenuItemName
			}
			if row.referenceBillID == nil && movement.ReferenceBillID != nil {
				billID := *movement.ReferenceBillID
				row.referenceBillID = &billID
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].inventoryItemID != keys[j].inventoryItemID {
			return keys[i].inventoryItemID < keys[j].inventoryItemID
		}
		return keys[i].menuItemID < keys[j].menuItemID
	})
	out := make([]inventoryLineNet, 0, len(keys))
	for _, key := range keys {
		row := grouped[key]
		net := QuantizeInventoryQuantity(-row.sum)
		if net <= 0 {
			continue
		}
		out = append(out, inventoryLineNet{
			inventoryItemID: key.inventoryItemID,
			menuItemID:      key.menuItemID,
			menuItemName:    row.menuItemName,
			referenceBillID: row.referenceBillID,
			net:             net,
		})
	}
	return out
}

func loadOrderInventoryMovementsTx(tx *gorm.DB, order *Order) ([]InventoryMovement, error) {
	var movements []InventoryMovement
	if err := tx.Where(
		"business_id = ? AND reference_order_id = ? AND movement_type IN ?",
		order.BusinessID,
		order.ID,
		[]string{InventoryMovementTypeOrderConsumption, InventoryMovementTypeOrderRestoration},
	).Order("id ASC").Find(&movements).Error; err != nil {
		return nil, fmt.Errorf("failed to load consumed inventory movements: %w", err)
	}
	return movements, nil
}

func lockInventoryItemsTx(tx *gorm.DB, businessID uint, ids []uint) (map[uint]*InventoryItem, error) {
	ids = uniqueSortedUintIDs(ids)
	if len(ids) == 0 {
		return map[uint]*InventoryItem{}, nil
	}
	var inventoryItems []InventoryItem
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("business_id = ? AND id IN ?", businessID, ids).
		Order("id ASC").
		Find(&inventoryItems).Error; err != nil {
		return nil, fmt.Errorf("failed to load inventory items for restoration: %w", err)
	}
	return inventoryItemsByID(inventoryItems), nil
}

// RestoreOrderLineInventoryTx puts back stock for one voided order line.
// Each recipe restores quantity_required * quantity, capped at the net still
// consumed for that order, inventory item, and menu item.
func RestoreOrderLineInventoryTx(tx *gorm.DB, order *Order, menuItemID string, quantity int, actor string) error {
	if tx == nil || order == nil || quantity <= 0 {
		return nil
	}
	menuItemID, err := orderConsumptionMenuItemID(tx, order.BusinessID, menuItemID)
	if err != nil {
		return err
	}
	if menuItemID == "" {
		return nil
	}

	var recipes []InventoryRecipe
	if err := tx.Where("business_id = ? AND menu_item_id = ?", order.BusinessID, menuItemID).
		Find(&recipes).Error; err != nil {
		return fmt.Errorf("failed to load inventory recipes: %w", err)
	}
	if len(recipes) == 0 {
		return nil
	}
	sort.Slice(recipes, func(i, j int) bool {
		if recipes[i].InventoryItemID != recipes[j].InventoryItemID {
			return recipes[i].InventoryItemID < recipes[j].InventoryItemID
		}
		return recipes[i].ID < recipes[j].ID
	})

	inventoryItemIDs := make([]uint, 0, len(recipes))
	for _, recipe := range recipes {
		if recipe.QuantityRequired <= 0 {
			continue
		}
		inventoryItemIDs = append(inventoryItemIDs, recipe.InventoryItemID)
	}
	inventoryByID, err := lockInventoryItemsTx(tx, order.BusinessID, inventoryItemIDs)
	if err != nil {
		return err
	}
	if len(inventoryByID) == 0 {
		return nil
	}

	movements, err := loadOrderInventoryMovementsTx(tx, order)
	if err != nil {
		return err
	}
	netByItem := make(map[uint]float64)
	for _, line := range positiveOrderInventoryNets(movements, menuItemID) {
		netByItem[line.inventoryItemID] = line.net
	}

	inventoryChanged := false
	for _, recipe := range recipes {
		if recipe.QuantityRequired <= 0 {
			continue
		}
		inventoryItem := inventoryByID[recipe.InventoryItemID]
		if inventoryItem == nil {
			continue
		}
		net := netByItem[recipe.InventoryItemID]
		if net <= 0 {
			continue
		}
		restoreQty := QuantizeInventoryQuantity(recipe.QuantityRequired * float64(quantity))
		if restoreQty <= 0 {
			continue
		}
		if restoreQty > net {
			restoreQty = net
		}
		if err := applyInventoryMovementTx(tx, inventoryItem, restoreQty, inventoryMovementMetadata{
			MovementType:     InventoryMovementTypeOrderRestoration,
			Reason:           fmt.Sprintf("Restored by voided order line %s", order.OrderNumber),
			Actor:            actor,
			MenuItemID:       menuItemID,
			MenuItemName:     strings.TrimSpace(recipe.MenuItemName),
			ReferenceOrderID: &order.ID,
			ReferenceBillID:  &order.BillID,
		}); err != nil {
			return err
		}
		netByItem[recipe.InventoryItemID] = QuantizeInventoryQuantity(net - restoreQty)
		inventoryChanged = true
	}
	if inventoryChanged {
		return advanceGuestOrderabilityRevisionTx(tx, order.BusinessID)
	}
	return nil
}

func RestoreCancelledOrderInventoryTx(tx *gorm.DB, order *Order, actor string) error {
	if tx == nil || order == nil {
		return nil
	}
	movements, err := loadOrderInventoryMovementsTx(tx, order)
	if err != nil {
		return err
	}
	nets := positiveOrderInventoryNets(movements, "")
	if len(nets) == 0 {
		return nil
	}

	inventoryItemIDs := make([]uint, 0, len(nets))
	for _, line := range nets {
		inventoryItemIDs = append(inventoryItemIDs, line.inventoryItemID)
	}
	inventoryByID, err := lockInventoryItemsTx(tx, order.BusinessID, inventoryItemIDs)
	if err != nil {
		return err
	}

	inventoryChanged := false
	for _, line := range nets {
		inventoryItem := inventoryByID[line.inventoryItemID]
		if inventoryItem == nil || line.net <= 0 {
			continue
		}
		if err := applyInventoryMovementTx(tx, inventoryItem, line.net, inventoryMovementMetadata{
			MovementType:     InventoryMovementTypeOrderRestoration,
			Reason:           fmt.Sprintf("Restored by cancelled order %s", order.OrderNumber),
			Actor:            actor,
			MenuItemID:       line.menuItemID,
			MenuItemName:     line.menuItemName,
			ReferenceOrderID: &order.ID,
			ReferenceBillID:  line.referenceBillID,
		}); err != nil {
			return err
		}
		inventoryChanged = true
	}

	if inventoryChanged {
		return advanceGuestOrderabilityRevisionTx(tx, order.BusinessID)
	}
	return nil
}
