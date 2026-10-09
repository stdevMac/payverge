package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/money"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MaxGuestCheckoutRequestIDLen is the longest client request id (the guest's
// X-Request-Id, in bytes after trimming) a checkout accepts. It is the
// idempotency key: a retry with the same id replays the original order.
const MaxGuestCheckoutRequestIDLen = 64

type GuestCheckoutInput struct {
	TableCode       string
	BillID          *uint
	ClientRequestID string
	Items           []PromotionInputLine
	Notes           string
	PromoCode       string
}

type GuestCheckoutResult struct {
	Bill   database.Bill
	Order  database.Order
	Quote  Quote
	Replay bool
}

// OrderQuoteProjection is the authoritative, read-only pricing and
// orderability projection used by both guest and operator quote endpoints.
type OrderQuoteProjection struct {
	Quote        Quote
	Orderability map[string]Orderability
}

type ItemNotOrderableError struct {
	Items []ItemNotOrderableDetail
}

func (e *ItemNotOrderableError) Error() string { return "one or more items are not orderable" }

type ItemNotOrderableDetail struct {
	MenuItemID string            `json:"menu_item_id"`
	Reason     OrderabilityState `json:"reason"`
}

const GuestCheckoutReplayConflictCode = "idempotency_conflict"

// GuestCheckoutReplayConflictError means the request identity already belongs
// to an order whose bill is outside the currently locked table. It deliberately
// carries no bill capability; callers must mint a fresh request identity for
// the intended table instead of receiving another table's replay result.
type GuestCheckoutReplayConflictError struct {
	ExistingBillID uint
}

func (e *GuestCheckoutReplayConflictError) Error() string {
	return "request identity is already associated with another table"
}

type GuestCheckoutService struct {
	db *gorm.DB
}

func NewGuestCheckoutService(db *gorm.DB) *GuestCheckoutService {
	return &GuestCheckoutService{db: db}
}

func (s *GuestCheckoutService) Checkout(ctx context.Context, input GuestCheckoutInput) (GuestCheckoutResult, error) {
	if s == nil || s.db == nil {
		return GuestCheckoutResult{}, gorm.ErrInvalidDB
	}
	input.TableCode = strings.TrimSpace(input.TableCode)
	input.ClientRequestID = strings.TrimSpace(input.ClientRequestID)
	if input.TableCode == "" {
		return GuestCheckoutResult{}, fmt.Errorf("table code is required")
	}
	if canonical, err := database.CanonicalGuestTableCode(input.TableCode); err == nil {
		input.TableCode = canonical
	}
	if input.ClientRequestID == "" || len(input.ClientRequestID) > MaxGuestCheckoutRequestIDLen {
		return GuestCheckoutResult{}, fmt.Errorf("client request id is required and must be at most 64 characters")
	}
	if len(input.Items) == 0 {
		return GuestCheckoutResult{}, fmt.Errorf("order must contain at least one item")
	}
	var tableIdentity database.Table
	if err := s.db.WithContext(ctx).Preload("Business").
		Where("table_code = ? AND is_active = ?", input.TableCode, true).
		Take(&tableIdentity).Error; err != nil {
		return GuestCheckoutResult{}, fmt.Errorf("table not found: %w", err)
	}
	// The eligibility lookup uses cached/global plugin state, so resolve it
	// before taking the checkout transaction's connection. This prevents a
	// nested global DB read from deadlocking single-connection test/runtime DBs.
	notifyTelegram := ShouldEnqueueTelegramNotification(tableIdentity.BusinessID, PluginEventOrderCreated)
	categories, bundles, offers, err := loadCheckoutPricing(&tableIdentity.Business, input.PromoCode)
	if err != nil {
		return GuestCheckoutResult{}, err
	}

	var result GuestCheckoutResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var table database.Table
		// Serialize checkout against reservation assignment and other table-scoped
		// occupancy mutations. Reservation writes lock this same row before their
		// final conflict check, so neither path can observe a stale "free" table
		// while the other is committing an active bill/assignment.
		// The business was loaded once before the transaction; the lock only
		// serialises table occupancy.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "business_id", "table_code", "name", "is_active").
			Where("id = ? AND is_active = ?", tableIdentity.ID, true).
			Take(&table).Error; err != nil {
			return fmt.Errorf("table not found: %w", err)
		}
		table.Business = tableIdentity.Business
		business := &table.Business

		if replay, err := loadGuestCheckoutReplayTx(tx, &table, input.ClientRequestID); err == nil {
			result = replay
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// Lock or create the bill before validating mutable menu state so a
		// closed/mismatched bill receives the stable lifecycle error and the
		// transaction has one authoritative aggregate from this point onward.
		bill, existingItems, err := s.resolveCheckoutBill(tx, business, &table, input.BillID)
		if err != nil {
			return err
		}

		businessOpen := BusinessOpenAt(tx, business, time.Now())
		// Closed Mode / kitchen gates are business-level — do not rely solely on
		// per-item projection matching. A forged or stale menu_item_id that is
		// absent from the projection previously skipped orderability and still
		// created pending orders after hours (FIND-035).
		if !businessOpen {
			return NewOrderValidationError(
				OrderErrCodeBusinessClosed,
				"Restaurant is currently closed for ordering",
			)
		}
		if !(business.KitchenEnabled && business.OrdersEnabled) {
			return NewOrderValidationError(
				OrderErrCodeOrderingDisabled,
				"Ordering is disabled for this business",
			)
		}
		projection, err := projectOrderabilityTx(tx, business, categories, OrderabilityContextGuest, businessOpen)
		if err != nil {
			return err
		}
		if unknown := unknownGuestCatalogItem(input.Items, categories, projection); unknown != "" {
			return NewOrderValidationError(OrderErrCodeItemNotFound, "menu item '%s' not found", unknown)
		}
		// Reject direct physical lines through the authoritative projection before
		// the pricing parser's legacy availability guard can collapse the reason to
		// item_unavailable. Bundle parents are checked after expansion below.
		if blocked := blockedPromotionInputs(input.Items, categories, projection); len(blocked) > 0 {
			return &ItemNotOrderableError{Items: blocked}
		}

		priced, err := ApplyPromotionsToOrder(categories, bundles, offers, input.Items)
		if err != nil {
			return err
		}
		if err := requirePricedOrderLines(priced); err != nil {
			return err
		}
		// Validate the resolved lines, not just the request lines. Bundle parents
		// expand into their physical menu-item children here, so inventory and
		// manual-availability decisions apply identically to standalone dishes
		// and dishes purchased through a bundle.
		blocked := blockedOrderItems(priced.Lines, projection)
		if len(blocked) > 0 {
			return &ItemNotOrderableError{Items: blocked}
		}
		quote := quoteFromPromotionResult(priced)
		applySettlementRates(&quote, business.TaxRate, business.ServiceFeeRate, 0)
		quoteSnapshot, err := marshalOrderQuoteSnapshot(quote)
		if err != nil {
			return err
		}

		now := time.Now()
		requestID := input.ClientRequestID
		order := database.Order{
			BillID: bill.ID, BusinessID: business.ID,
			OrderNumber: fmt.Sprintf("G%d-%d", business.ID, now.UnixNano()%100000000),
			Status:      database.OrderStatusPending, CreatedBy: "guest", ClientRequestID: &requestID,
			Notes: strings.TrimSpace(input.Notes), QuoteSnapshot: quoteSnapshot, CreatedAt: now, UpdatedAt: now,
		}
		if err := database.CreateOrderTx(tx, &order, priced.Lines); err != nil {
			return err
		}

		newItems := PromotionLinesToBillItems(priced.Lines)
		for i := range newItems {
			newItems[i].OrderID = &order.ID
		}
		allItems := append(existingItems, newItems...)
		bill.Subtotal += quote.TotalCents
		if bill.Subtotal < 0 {
			bill.Subtotal = 0
		}
		bill.TaxAmount = money.PercentageCents(bill.Subtotal, business.TaxRate)
		bill.ServiceFeeAmount = money.PercentageCents(bill.Subtotal, business.ServiceFeeRate)
		// Repricing must preserve an already-applied loyalty discount:
		bill.TotalAmount = database.NetBillTotalCents(
			bill.Subtotal+bill.TaxAmount+bill.ServiceFeeAmount,
			bill.LoyaltyDiscountCents,
		)
		bill.UpdatedAt = now
		if err := database.UpdateBillTx(tx, &bill, allItems); err != nil {
			return err
		}
		if notifyTelegram {
			itemCount := 0
			for _, line := range priced.Lines {
				if line.ItemType != OrderItemTypeDiscount && line.ItemType != OrderItemTypeBundle {
					itemCount += line.Quantity
				}
			}
			currency := business.DisplayCurrency
			if currency == "" {
				currency = business.DefaultCurrency
			}
			if _, _, err := EnqueuePluginNotificationTx(tx, PluginNotificationEvent{
				BusinessID: order.BusinessID,
				EventType:  PluginEventOrderCreated,
				EventID:    fmt.Sprintf("order:%d", order.ID),
				Payload: map[string]interface{}{
					"order_id": order.ID, "order_number": order.OrderNumber,
					"bill_id": bill.ID, "bill_number": bill.BillNumber,
					"table_id": table.ID, "table_name": table.Name,
					"item_count": itemCount, "total_cents": quote.FinalTotalCents,
					"currency": currency, "notes": order.Notes, "source": "guest",
				},
				CreatedAt: now.UTC(),
			}, "telegram"); err != nil {
				return err
			}
		}

		result = GuestCheckoutResult{Bill: bill, Order: order, Quote: quote}
		return nil
	})
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "unique") || strings.Contains(lower, "duplicate") {
			var replay GuestCheckoutResult
			replayErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var lockedTable database.Table
				if lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
					Where("id = ? AND table_code = ? AND is_active = ?", tableIdentity.ID, input.TableCode, true).
					First(&lockedTable).Error; lockErr != nil {
					return lockErr
				}
				var replayErr error
				replay, replayErr = loadGuestCheckoutReplayTx(tx, &lockedTable, input.ClientRequestID)
				return replayErr
			})
			if replayErr == nil {
				return replay, nil
			}
			var conflict *GuestCheckoutReplayConflictError
			if errors.As(replayErr, &conflict) {
				return GuestCheckoutResult{}, conflict
			}
		}
		return GuestCheckoutResult{}, err
	}
	return result, nil
}

func blockedOrderItems(lines []database.OrderItem, projection map[string]Orderability) []ItemNotOrderableDetail {
	blocked := make(map[string]OrderabilityState)
	for _, line := range lines {
		itemType := normalizeItemType(line.ItemType)
		if itemType == OrderItemTypeDiscount || itemType == OrderItemTypeBundle {
			continue
		}
		itemID := strings.TrimSpace(line.MenuItemID)
		// Unknown / empty IDs fail closed — never skip orderability (FIND-035).
		if itemID == "" {
			blocked["unknown"] = OrderabilityManualDisabled
			continue
		}
		decision, ok := projection[itemID]
		if !ok {
			blocked[itemID] = OrderabilityManualDisabled
			continue
		}
		if !decision.Orderable {
			blocked[itemID] = decision.State
		}
	}
	ids := make([]string, 0, len(blocked))
	for itemID := range blocked {
		ids = append(ids, itemID)
	}
	sort.Strings(ids)
	details := make([]ItemNotOrderableDetail, 0, len(ids))
	for _, itemID := range ids {
		details = append(details, ItemNotOrderableDetail{MenuItemID: itemID, Reason: blocked[itemID]})
	}
	return details
}

func catalogNameByID(categories []database.MenuCategory) map[string]string {
	itemIDByName := make(map[string]string)
	for _, category := range categories {
		for _, item := range category.Items {
			itemIDByName[strings.ToLower(strings.TrimSpace(item.Name))] = item.ID
		}
	}
	return itemIDByName
}

func unknownGuestCatalogItem(inputs []PromotionInputLine, categories []database.MenuCategory, projection map[string]Orderability) string {
	itemIDByName := catalogNameByID(categories)
	for _, input := range inputs {
		itemType := normalizeItemType(input.ItemType)
		if itemType == OrderItemTypeDiscount || itemType == OrderItemTypeBundle {
			continue
		}
		itemID := strings.TrimSpace(input.MenuItemID)
		nameKey := strings.ToLower(strings.TrimSpace(input.Name))
		if itemID != "" {
			if _, ok := projection[itemID]; ok {
				continue
			}
			if itemIDByName[nameKey] != "" {
				continue
			}
			return itemID
		}
		if itemIDByName[nameKey] != "" {
			continue
		}
		if name := strings.TrimSpace(input.Name); name != "" {
			return name
		}
	}
	return ""
}

func blockedPromotionInputs(inputs []PromotionInputLine, categories []database.MenuCategory, projection map[string]Orderability) []ItemNotOrderableDetail {
	itemIDByName := make(map[string]string)
	for _, category := range categories {
		for _, item := range category.Items {
			itemIDByName[strings.ToLower(strings.TrimSpace(item.Name))] = item.ID
		}
	}
	lines := make([]database.OrderItem, 0, len(inputs))
	for _, input := range inputs {
		itemType := normalizeItemType(input.ItemType)
		if itemType == OrderItemTypeDiscount || itemType == OrderItemTypeBundle {
			continue
		}
		itemID := strings.TrimSpace(input.MenuItemID)
		nameKey := strings.ToLower(strings.TrimSpace(input.Name))
		// Prefer catalog ID: empty ID, or non-empty ID missing from projection
		// (stale/forged ids used to skip orderability — FIND-035).
		if itemID == "" {
			itemID = itemIDByName[nameKey]
		} else if _, ok := projection[itemID]; !ok {
			if byName := itemIDByName[nameKey]; byName != "" {
				itemID = byName
			}
		}
		lines = append(lines, database.OrderItem{MenuItemID: itemID, ItemType: itemType})
	}
	return blockedOrderItems(lines, projection)
}

func loadGuestCheckoutReplayTx(tx *gorm.DB, table *database.Table, clientRequestID string) (GuestCheckoutResult, error) {
	if tx == nil || table == nil {
		return GuestCheckoutResult{}, gorm.ErrInvalidDB
	}
	order, orderItems, err := database.GetOrderByBusinessRequestIdentityTx(tx, table.BusinessID, "guest", clientRequestID)
	if err != nil {
		return GuestCheckoutResult{}, err
	}
	var bill database.Bill
	if err := tx.First(&bill, order.BillID).Error; err != nil {
		return GuestCheckoutResult{}, err
	}
	if order.BusinessID != table.BusinessID || bill.BusinessID != table.BusinessID || bill.TableID != table.ID {
		return GuestCheckoutResult{}, &GuestCheckoutReplayConflictError{ExistingBillID: bill.ID}
	}
	quote := quoteFromOrderItems(orderItems)
	// New orders replay their immutable cent-exact quote. Legacy rows and
	// snapshots that fail version, arithmetic, or persisted-line validation
	// deliberately fall back to reconstructed pricing at the current rates.
	if snapshot, ok := orderQuoteFromSnapshot(order.QuoteSnapshot, quote); ok {
		quote = snapshot
	} else {
		taxRate, serviceFeeRate := table.Business.TaxRate, table.Business.ServiceFeeRate
		if table.Business.ID == 0 {
			var business database.Business
			if err := tx.Select("id", "tax_rate", "service_fee_rate").First(&business, table.BusinessID).Error; err != nil {
				return GuestCheckoutResult{}, err
			}
			taxRate, serviceFeeRate = business.TaxRate, business.ServiceFeeRate
		}
		applySettlementRates(&quote, taxRate, serviceFeeRate, 0)
	}
	return GuestCheckoutResult{
		Bill: bill, Order: *order, Quote: quote, Replay: true,
	}, nil
}

// OperatorOrderabilityProjectionError identifies an infrastructure failure
// while a mutation was deriving its authoritative inventory projection. Write
// paths must fail closed on this error; read projections may remain tolerant.
type OperatorOrderabilityProjectionError struct {
	Err error
}

func (e *OperatorOrderabilityProjectionError) Error() string {
	return fmt.Sprintf("failed to project operator orderability: %v", e.Err)
}

func (e *OperatorOrderabilityProjectionError) Unwrap() error { return e.Err }

// ValidateOperatorOrderableLinesTx applies the authoritative operator
// projection inside the caller-owned mutation transaction. Bundle parents are
// skipped while their expanded child rows are checked, preventing a bundle
// from bypassing a hard inventory block. Projection errors are returned rather
// than treated as an empty inventory summary, so mutations fail closed.
func ValidateOperatorOrderableLinesTx(tx *gorm.DB, business *database.Business, categories []database.MenuCategory, lines []database.OrderItem) error {
	if tx == nil || business == nil {
		return &OperatorOrderabilityProjectionError{Err: gorm.ErrInvalidDB}
	}
	projection, err := projectOrderabilityTx(tx, business, categories, OrderabilityContextOperator, true)
	if err != nil {
		return &OperatorOrderabilityProjectionError{Err: err}
	}
	blocked := blockedOrderItems(lines, projection)
	if len(blocked) > 0 {
		return &ItemNotOrderableError{Items: blocked}
	}
	return nil
}

// projectOrderabilityTx is the transactional twin of ProjectOrderability used
// by guest checkout, quotes, and ValidateOperatorOrderableLinesTx. It must
// write/correct drifted recipe menu_item_id values and flag dishes with the
// same name-wins resolver as GetInventorySummary — otherwise the UI 86s Steak
// Plate while checkout 86s Harvest Bowl (#727 / #758).
func projectOrderabilityTx(tx *gorm.DB, business *database.Business, categories []database.MenuCategory, context OrderabilityContext, businessOpen bool) (map[string]Orderability, error) {
	var settings database.InventorySettings
	err := tx.Where("business_id = ?", business.ID).First(&settings).Error
	missingInventoryTable := err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table")
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && !missingInventoryTable {
		return nil, err
	}

	type inventoryFlags struct{ out, warning bool }
	flags := make(map[string]inventoryFlags)
	if settings.InventoryEnabled {
		if err := database.ReconcileInventoryRecipeMenuItemIDsTx(tx, business.ID, categories); err != nil {
			return nil, err
		}
		var recipes []database.InventoryRecipe
		if err := tx.Where("business_id = ?", business.ID).Find(&recipes).Error; err != nil {
			return nil, err
		}
		var items []database.InventoryItem
		if err := tx.Where("business_id = ?", business.ID).Find(&items).Error; err != nil {
			return nil, err
		}
		itemByID := make(map[uint]database.InventoryItem, len(items))
		for _, item := range items {
			itemByID[item.ID] = item
		}
		for _, recipe := range recipes {
			if recipe.QuantityRequired <= 0 {
				continue
			}
			menuItemID := database.ResolvedInventoryRecipeMenuItemID(recipe, categories)
			if menuItemID == "" {
				menuItemID = strings.TrimSpace(recipe.MenuItemID)
			}
			if menuItemID == "" {
				continue
			}
			flag := flags[menuItemID]
			item, ok := itemByID[recipe.InventoryItemID]
			if !ok || !item.IsActive || item.CurrentQuantity < recipe.QuantityRequired {
				flag.out = true
			} else if item.ReorderThreshold > 0 && item.CurrentQuantity <= item.ReorderThreshold {
				flag.warning = true
			}
			flags[menuItemID] = flag
		}
	}

	projection := make(map[string]Orderability)
	for _, category := range categories {
		for _, item := range category.Items {
			flag := flags[item.ID]
			projection[item.ID] = ResolveOrderability(OrderabilityFacts{
				Context: context, ManualAvailable: item.IsAvailable,
				InventoryEnabled: settings.InventoryEnabled, InventoryMode: settings.AvailabilitySyncMode,
				InventoryOut: flag.out, InventoryWarning: flag.warning,
				BusinessOpen: businessOpen, GuestOrderingEnabled: business.KitchenEnabled && business.OrdersEnabled,
			})
		}
	}
	return projection, nil
}

// QuoteForTable returns the same quote/orderability decisions checkout will
// enforce, without creating a bill or order.
func (s *GuestCheckoutService) QuoteForTable(ctx context.Context, tableCode string, items []PromotionInputLine, promoCode string) (OrderQuoteProjection, error) {
	if s == nil || s.db == nil {
		return OrderQuoteProjection{}, gorm.ErrInvalidDB
	}
	tableCode = strings.TrimSpace(tableCode)
	if tableCode == "" {
		return OrderQuoteProjection{}, fmt.Errorf("table code is required")
	}
	if canonical, err := database.CanonicalGuestTableCode(tableCode); err == nil {
		tableCode = canonical
	}
	var table database.Table
	if err := s.db.WithContext(ctx).Preload("Business").Where("table_code = ? AND is_active = ?", tableCode, true).First(&table).Error; err != nil {
		return OrderQuoteProjection{}, fmt.Errorf("table not found: %w", err)
	}
	return s.quoteForBusiness(ctx, &table.Business, items, promoCode, OrderabilityContextGuest)
}

// QuoteForBusiness provides the operator version of the shared quote. Guest
// opening-hours and ordering-toggle gates do not prevent staff-entered orders.
func (s *GuestCheckoutService) QuoteForBusiness(ctx context.Context, businessID uint, items []PromotionInputLine, promoCode string) (OrderQuoteProjection, error) {
	if s == nil || s.db == nil {
		return OrderQuoteProjection{}, gorm.ErrInvalidDB
	}
	var business database.Business
	if err := s.db.WithContext(ctx).First(&business, businessID).Error; err != nil {
		return OrderQuoteProjection{}, fmt.Errorf("business not found: %w", err)
	}
	return s.quoteForBusiness(ctx, &business, items, promoCode, OrderabilityContextOperator)
}

func (s *GuestCheckoutService) quoteForBusiness(ctx context.Context, business *database.Business, items []PromotionInputLine, promoCode string, orderContext OrderabilityContext) (OrderQuoteProjection, error) {
	if len(items) == 0 {
		return OrderQuoteProjection{}, fmt.Errorf("order must contain at least one item")
	}
	categories, bundles, offers, err := loadCheckoutPricing(business, promoCode)
	if err != nil {
		return OrderQuoteProjection{}, err
	}
	var result OrderQuoteProjection
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		projection, err := projectOrderabilityTx(tx, business, categories, orderContext, BusinessOpenAt(tx, business, time.Now()))
		if err != nil {
			return err
		}
		if orderContext == OrderabilityContextGuest {
			if unknown := unknownGuestCatalogItem(items, categories, projection); unknown != "" {
				return NewOrderValidationError(OrderErrCodeItemNotFound, "menu item '%s' not found", unknown)
			}
		}
		priced, err := ApplyPromotionsToOrder(categories, bundles, offers, items)
		if err != nil {
			return err
		}
		if err := requirePricedOrderLines(priced); err != nil {
			return err
		}
		if blocked := blockedPromotionInputs(items, categories, projection); len(blocked) > 0 {
			return &ItemNotOrderableError{Items: blocked}
		}
		if blocked := blockedOrderItems(priced.Lines, projection); len(blocked) > 0 {
			return &ItemNotOrderableError{Items: blocked}
		}
		quote := quoteFromPromotionResult(priced)
		applySettlementRates(&quote, business.TaxRate, business.ServiceFeeRate, 0)
		result = OrderQuoteProjection{Quote: quote, Orderability: projection}
		return nil
	})
	return result, err
}

// BusinessOpenAt evaluates today's configured operating window in the
// business timezone. No configured row means no hours-based restriction.
func BusinessOpenAt(tx *gorm.DB, business *database.Business, now time.Time) bool {
	if tx == nil || business == nil {
		return false
	}
	loc, err := time.LoadLocation(strings.TrimSpace(business.Timezone))
	if err != nil {
		loc = time.UTC
	}
	localNow := now.In(loc)
	var hours database.BusinessOperatingHours
	err = tx.Where("business_id = ? AND day_of_week = ?", business.ID, int(localNow.Weekday())).First(&hours).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table")) {
		return true
	}
	if err != nil || hours.IsClosed {
		return false
	}
	openClock, openErr := time.Parse("15:04", strings.TrimSpace(hours.OpenTime))
	closeClock, closeErr := time.Parse("15:04", strings.TrimSpace(hours.CloseTime))
	if openErr != nil || closeErr != nil {
		return false
	}
	minute := localNow.Hour()*60 + localNow.Minute()
	openMinute := openClock.Hour()*60 + openClock.Minute()
	closeMinute := closeClock.Hour()*60 + closeClock.Minute()
	if closeMinute <= openMinute {
		return minute >= openMinute || minute < closeMinute
	}
	return minute >= openMinute && minute < closeMinute
}

func (s *GuestCheckoutService) resolveCheckoutBill(tx *gorm.DB, business *database.Business, table *database.Table, billID *uint) (database.Bill, []database.BillItem, error) {
	if billID == nil {
		// A guest may retry from another tab/device without the locally cached
		// bill ID. The locked table row makes this lookup/create atomic and keeps
		// the invariant at one active bill per table.
		var active database.Bill
		err := tx.Where("table_id = ? AND business_id = ? AND status IN ?", table.ID, business.ID, database.ActiveBillStatuses()).
			Order("created_at DESC, id DESC").First(&active).Error
		if err == nil {
			if active.Status != database.BillStatusOpen {
				return database.Bill{}, nil, fmt.Errorf("bill is not open")
			}
			items, loadErr := loadCheckoutBillItems(tx, active)
			return active, items, loadErr
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return database.Bill{}, nil, err
		}
		bill := database.Bill{
			BusinessID: business.ID, TableID: table.ID, Status: database.BillStatusOpen,
			SettlementAddr: business.SettlementAddr, TippingAddr: business.TippingAddr,
		}
		if err := database.CreateBillTx(tx, &bill, nil); err != nil {
			return database.Bill{}, nil, err
		}
		return bill, []database.BillItem{}, nil
	}
	var bill database.Bill
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND table_id = ? AND business_id = ?", *billID, table.ID, business.ID).First(&bill).Error; err != nil {
		return database.Bill{}, nil, fmt.Errorf("bill not found for table: %w", err)
	}
	if bill.Status != database.BillStatusOpen {
		return database.Bill{}, nil, fmt.Errorf("bill is not open")
	}
	items, err := loadCheckoutBillItems(tx, bill)
	return bill, items, err
}

func loadCheckoutBillItems(tx *gorm.DB, bill database.Bill) ([]database.BillItem, error) {
	var items []database.BillItem
	if err := tx.Where("bill_id = ?", bill.ID).Order("created_at ASC, id ASC").Find(&items).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			if strings.TrimSpace(bill.Items) != "" {
				if decodeErr := json.Unmarshal([]byte(bill.Items), &items); decodeErr != nil {
					return nil, decodeErr
				}
			}
		} else {
			return nil, err
		}
	}
	return items, nil
}

func loadCheckoutPricing(business *database.Business, promoCode string) ([]database.MenuCategory, []database.Bundle, []database.Offer, error) {
	snap, err := getPricingSnapshotFrom(business.ID, business)
	if err != nil {
		return nil, nil, nil, err
	}
	categories, candidates, bundles := snap.categories, snap.offers, snap.bundles
	loc, err := time.LoadLocation(strings.TrimSpace(business.Timezone))
	if err != nil {
		loc = time.UTC
	}
	now := database.ScheduleNow().In(loc)
	active := make([]database.Offer, 0, len(candidates))
	for _, offer := range candidates {
		if offer.StartDate != nil && offer.StartDate.After(now) || offer.EndDate != nil && offer.EndDate.Before(now) || !database.OfferActiveAt(offer, now) {
			continue
		}
		active = append(active, offer)
	}
	return categories, bundles, filterOffersForPromoCode(active, promoCode), nil
}

func quoteFromPromotionResult(result PromotionResult) Quote {
	quote := Quote{
		SubtotalCents: majorToMinor(result.BaseSubtotal),
		DiscountCents: majorToMinor(result.DiscountTotal),
		Lines:         make([]QuotedLine, 0, len(result.Lines)),
	}
	quote.TotalCents = quote.SubtotalCents - quote.DiscountCents
	applySettlementRates(&quote, 0, 0, 0)
	for _, line := range result.Lines {
		quote.Lines = append(quote.Lines, QuotedLine{Key: line.MenuItemID, LineType: normalizeItemType(line.ItemType), UnitPriceCents: majorToMinor(line.Price), Quantity: line.Quantity, SubtotalCents: majorToMinor(line.Subtotal)})
	}
	return quote
}

func quoteFromOrderItems(items []database.OrderItem) Quote {
	result := PromotionResult{Lines: items}
	for _, item := range items {
		if item.ItemType == OrderItemTypeDiscount {
			result.DiscountTotal += -item.Subtotal
		} else if item.ItemType != OrderItemTypeBundleItem {
			result.BaseSubtotal += item.Subtotal
		}
	}
	return quoteFromPromotionResult(result)
}
