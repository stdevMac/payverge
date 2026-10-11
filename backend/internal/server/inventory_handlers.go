package server

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
)

type inventorySettingsRequest struct {
	InventoryEnabled          *bool   `json:"inventory_enabled"`
	AutoDeductOnOrderApproval *bool   `json:"auto_deduct_on_order_approval"`
	LowStockWarningsEnabled   *bool   `json:"low_stock_warnings_enabled"`
	AvailabilitySyncMode      *string `json:"availability_sync_mode"`
}

type createInventoryItemRequest struct {
	Name             string   `json:"name" binding:"required"`
	SKU              string   `json:"sku"`
	Category         string   `json:"category"`
	Unit             string   `json:"unit"`
	CurrentQuantity  *float64 `json:"current_quantity"`
	ReorderThreshold *float64 `json:"reorder_threshold"`
	CostPerUnit      *float64 `json:"cost_per_unit"`
	IsActive         *bool    `json:"is_active"`
}

type updateInventoryItemRequest struct {
	Name             *string  `json:"name"`
	SKU              *string  `json:"sku"`
	Category         *string  `json:"category"`
	Unit             *string  `json:"unit"`
	CurrentQuantity  *float64 `json:"current_quantity"`
	ReorderThreshold *float64 `json:"reorder_threshold"`
	CostPerUnit      *float64 `json:"cost_per_unit"`
	IsActive         *bool    `json:"is_active"`
}

type replaceInventoryRecipeRequest struct {
	MenuItemName string                                `json:"menu_item_name"`
	Entries      []database.InventoryRecipeReplacement `json:"entries"`
}

type createInventoryAdjustmentRequest struct {
	InventoryItemID uint     `json:"inventory_item_id" binding:"required"`
	MovementType    string   `json:"movement_type"`
	QuantityChange  *float64 `json:"quantity_change"`
	// TargetQuantity, when present, reconciles on-hand to this ABSOLUTE value
	// (a physical count). The backend computes the delta against the live
	// locked row, so the count is not corrupted by stock that moved since the
	// operator opened the drawer (INV-M1). Takes precedence over QuantityChange.
	TargetQuantity *float64 `json:"target_quantity"`
	Reason         string   `json:"reason"`
}

func getInventoryBusiness(c *gin.Context) (*database.Business, bool) {
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return nil, false
	}

	if !CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You do not have access to this business"})
		return nil, false
	}

	return business, true
}

func GetInventorySettings(c *gin.Context) {
	business, ok := getInventoryBusiness(c)
	if !ok {
		return
	}

	settings, err := database.GetInventorySettings(business.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load inventory settings"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

func UpdateInventorySettings(c *gin.Context) {
	business, ok := getInventoryBusiness(c)
	if !ok {
		return
	}

	var req inventorySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	updates := make(map[string]interface{})
	if req.InventoryEnabled != nil {
		updates["inventory_enabled"] = *req.InventoryEnabled
	}
	if req.AutoDeductOnOrderApproval != nil {
		updates["auto_deduct_on_order_approval"] = *req.AutoDeductOnOrderApproval
	}
	if req.LowStockWarningsEnabled != nil {
		updates["low_stock_warnings_enabled"] = *req.LowStockWarningsEnabled
	}
	if req.AvailabilitySyncMode != nil {
		mode := strings.TrimSpace(strings.ToLower(*req.AvailabilitySyncMode))
		switch mode {
		case database.InventoryAvailabilityModeWarn, database.InventoryAvailabilityModeManual, database.InventoryAvailabilityModeHardBlock:
			updates["availability_sync_mode"] = mode
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid availability_sync_mode"})
			return
		}
	}

	settings, err := database.UpsertInventorySettings(business.ID, updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update inventory settings"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// respondInventoryWriteError classifies a database write error so the client
// gets a useful message without leaking raw driver/SQL internals. The DB layer
// returns hand-written validation strings (safe to echo), bare unique-constraint
// violations (mapped to a friendly 409), and "failed to …: %w" wraps that carry
// driver text (masked as 500 and logged).
func respondInventoryWriteError(c *gin.Context, action string, err error) {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "unique constraint") ||
		strings.Contains(lower, "duplicate key") ||
		strings.Contains(lower, "duplicate entry"):
		c.JSON(http.StatusConflict, gin.H{"error": "An inventory item with this SKU already exists"})
	case strings.Contains(lower, "failed to"):
		// Wrapped driver failure — may contain SQL/driver text. Mask it.
		log.Printf("[inventory] %s failed: %v", action, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save inventory changes"})
	default:
		// Hand-written validation message (name required, negative quantity,
		// insufficient stock, …) — safe and useful to surface.
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
	}
}

func ListInventoryItems(c *gin.Context) {
	business, ok := getInventoryBusiness(c)
	if !ok {
		return
	}

	includeInactive := strings.EqualFold(c.Query("include_inactive"), "true")
	items, err := database.ListInventoryItemsByBusinessID(business.ID, includeInactive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load inventory items"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

func CreateInventoryItem(c *gin.Context) {
	business, ok := getInventoryBusiness(c)
	if !ok {
		return
	}

	var req createInventoryItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	item := &database.InventoryItem{
		BusinessID: business.ID,
		Name:       req.Name,
		SKU:        req.SKU,
		Category:   req.Category,
		Unit:       req.Unit,
		IsActive:   true,
	}
	if req.CurrentQuantity != nil {
		item.CurrentQuantity = *req.CurrentQuantity
	}
	if req.ReorderThreshold != nil {
		item.ReorderThreshold = *req.ReorderThreshold
	}
	if req.CostPerUnit != nil {
		item.CostPerUnit = *req.CostPerUnit
	}
	if req.IsActive != nil {
		item.IsActive = *req.IsActive
	}

	if err := database.CreateInventoryItem(item, getBusinessActionActor(c)); err != nil {
		respondInventoryWriteError(c, "create item", err)
		return
	}

	invalidateOwnerHomeCaches(business.ID)

	c.JSON(http.StatusCreated, item)
}

func UpdateInventoryItem(c *gin.Context) {
	business, ok := getInventoryBusiness(c)
	if !ok {
		return
	}

	itemID, err := strconv.ParseUint(c.Param("itemId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid inventory item ID"})
		return
	}

	var req updateInventoryItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	item, err := database.GetInventoryItemByID(business.ID, uint(itemID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Inventory item not found"})
		return
	}
	previous := *item

	// Changing current_quantity here mutates on-hand stock via a correction
	// movement — the same effect POST /inventory/adjustments gates behind
	// inventory:adjust. This route only requires inventory:write, so enforce the
	// stronger permission when the edit actually moves stock; otherwise an
	// operator granted write-but-not-adjust could bypass the split via the editor.
	if req.CurrentQuantity != nil && *req.CurrentQuantity != previous.CurrentQuantity &&
		!callerHasPermission(c, "inventory:adjust") {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Changing stock level requires the inventory:adjust permission; use the stock adjustment action",
		})
		return
	}

	if req.Name != nil {
		item.Name = *req.Name
	}
	if req.SKU != nil {
		item.SKU = *req.SKU
	}
	if req.Category != nil {
		item.Category = *req.Category
	}
	if req.Unit != nil {
		item.Unit = *req.Unit
	}
	if req.CurrentQuantity != nil {
		item.CurrentQuantity = *req.CurrentQuantity
	}
	if req.ReorderThreshold != nil {
		item.ReorderThreshold = *req.ReorderThreshold
	}
	if req.CostPerUnit != nil {
		item.CostPerUnit = *req.CostPerUnit
	}
	if req.IsActive != nil {
		item.IsActive = *req.IsActive
	}

	if err := database.UpdateInventoryItem(item, previous, getBusinessActionActor(c)); err != nil {
		respondInventoryWriteError(c, "update item", err)
		return
	}

	invalidateOwnerHomeCaches(business.ID)

	c.JSON(http.StatusOK, item)
}

func DeleteInventoryItem(c *gin.Context) {
	business, ok := getInventoryBusiness(c)
	if !ok {
		return
	}

	itemID, err := strconv.ParseUint(c.Param("itemId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid inventory item ID"})
		return
	}

	if err := database.DeactivateInventoryItem(business.ID, uint(itemID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete inventory item"})
		return
	}

	invalidateOwnerHomeCaches(business.ID)

	c.JSON(http.StatusOK, gin.H{"message": "Inventory item deleted"})
}

func ListInventoryRecipes(c *gin.Context) {
	business, ok := getInventoryBusiness(c)
	if !ok {
		return
	}

	recipes, err := database.ListInventoryRecipesByBusinessID(business.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load inventory recipes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"recipes": recipes})
}

func ReplaceInventoryRecipe(c *gin.Context) {
	business, ok := getInventoryBusiness(c)
	if !ok {
		return
	}

	menuItemID := strings.TrimSpace(c.Param("menuItemId"))
	if menuItemID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Menu item ID is required"})
		return
	}

	var req replaceInventoryRecipeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	if err := database.ReplaceInventoryRecipeForMenuItem(business.ID, menuItemID, req.MenuItemName, req.Entries); err != nil {
		// FIND-060: domain validation messages are product-safe; wrapped DB
		// failures must not surface driver/GORM text to operators.
		msg := err.Error()
		if strings.Contains(msg, "failed to ") || strings.Contains(msg, "sql:") || strings.Contains(msg, "SQLSTATE") {
			RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not update inventory recipe")
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Inventory recipe updated"})
}

func ListInventoryMovements(c *gin.Context) {
	business, ok := getInventoryBusiness(c)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	// New optional filters/paging. Absent params keep the legacy behavior:
	// newest page of the whole business ledger, `{movements}` shape (BE-first).
	// `item_id` scopes to one item (item drawer per-item history), `movement_type`
	// scopes to one kind (Activity type chip), and page/offset page the result.
	filter := database.InventoryMovementFilter{Limit: limit}
	if v := strings.TrimSpace(c.Query("item_id")); v != "" {
		if itemID, err := strconv.ParseUint(v, 10, 32); err == nil {
			filter.ItemID = uint(itemID)
		}
	}
	if v := strings.TrimSpace(c.Query("movement_type")); v != "" {
		filter.MovementType = v
	}

	// Paging: either an explicit `offset`, or a 1-based `page` combined with the
	// limit. `page` wins when both are sent so the UI can page by number.
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		if off, err := strconv.Atoi(v); err == nil && off >= 0 {
			filter.Offset = off
		}
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if page, err := strconv.Atoi(v); err == nil && page > 1 {
			filter.Offset = (page - 1) * filter.EffectiveLimit()
		}
	}

	movements, total, err := database.ListInventoryMovementsFiltered(business.ID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load inventory movements"})
		return
	}

	// `movements` is the legacy key (unchanged). `total`/`offset`/`limit` are
	// additive so a paginated client can render an honest "showing X of N".
	c.JSON(http.StatusOK, gin.H{
		"movements": movements,
		"total":     total,
		"offset":    filter.Offset,
		"limit":     filter.EffectiveLimit(),
	})
}

func CreateInventoryAdjustment(c *gin.Context) {
	business, ok := getInventoryBusiness(c)
	if !ok {
		return
	}

	var req createInventoryAdjustmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if req.TargetQuantity == nil && req.QuantityChange == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quantity_change is required"})
		return
	}

	actor := getBusinessActionActor(c)
	var (
		item     *database.InventoryItem
		movement *database.InventoryMovement
		err      error
	)
	if req.TargetQuantity != nil {
		// Absolute physical count — the delta is computed server-side under the
		// row lock, so a concurrent deduction cannot corrupt the count.
		item, movement, err = database.RecordInventoryCount(
			business.ID,
			req.InventoryItemID,
			*req.TargetQuantity,
			req.Reason,
			actor,
		)
	} else {
		item, movement, err = database.CreateInventoryAdjustment(
			business.ID,
			req.InventoryItemID,
			req.MovementType,
			*req.QuantityChange,
			req.Reason,
			actor,
		)
	}
	if err != nil {
		respondInventoryWriteError(c, "adjust stock", err)
		return
	}
	enqueueTelegramInventoryLowStockAlertsForBusiness(business.ID)

	c.JSON(http.StatusCreated, gin.H{
		"item":     item,
		"movement": movement,
	})
}

func enqueueTelegramInventoryLowStockAlertsForBusiness(businessID uint) {
	// Funnel through the gated helper so the manual-adjustment path skips the
	// (relatively expensive) summary+enqueue when Telegram is not connected,
	// matching the order path (INV-L2).
	if _, err := services.MaybeEnqueueTelegramInventoryLowStockAlerts(businessID, time.Now().UTC()); err != nil {
		log.Printf("Failed to enqueue Telegram inventory low stock notification for business_id=%d: %v", businessID, err)
	}
}

func GetInventorySummary(c *gin.Context) {
	business, ok := getInventoryBusiness(c)
	if !ok {
		return
	}

	summary, err := database.GetInventorySummary(business.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load inventory summary"})
		return
	}

	c.JSON(http.StatusOK, summary)
}
