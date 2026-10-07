package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/money"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	// ErrMenuCatalogUnavailable is returned when leftover $0 bundle children
	// need catalog prices and the active menu cannot be loaded.
	ErrMenuCatalogUnavailable = errors.New("menu catalog unavailable")
	// ErrBundleChildCatalogMiss is returned when a leftover $0 bundle child
	// is not in the active menu. Callers must not keep shipping $0.
	ErrBundleChildCatalogMiss = errors.New("bundle child catalog price missing")
)

// menuCatalogPrices maps catalog identity (id and lowercased name) to unit price.
type menuCatalogPrices struct {
	byID   map[string]float64
	byName map[string]float64
}

func (c menuCatalogPrices) lookup(menuItemID, name string) (float64, bool) {
	if id := strings.TrimSpace(menuItemID); id != "" {
		if price, ok := c.byID[id]; ok {
			return price, true
		}
	}
	if key := strings.ToLower(strings.TrimSpace(name)); key != "" {
		if price, ok := c.byName[key]; ok {
			return price, true
		}
	}
	return 0, false
}

func loadMenuCatalogPrices(conn *gorm.DB, businessID uint) (menuCatalogPrices, error) {
	out := menuCatalogPrices{
		byID:   map[string]float64{},
		byName: map[string]float64{},
	}
	if conn == nil {
		conn = db
	}
	if businessID == 0 || conn == nil {
		return out, fmt.Errorf("%w: business_id=%d", ErrMenuCatalogUnavailable, businessID)
	}
	var menu Menu
	if err := conn.Where("business_id = ? AND is_active = ?", businessID, true).First(&menu).Error; err != nil {
		return out, fmt.Errorf("%w: business_id=%d: %v", ErrMenuCatalogUnavailable, businessID, err)
	}
	if strings.TrimSpace(menu.Categories) == "" {
		return out, fmt.Errorf("%w: empty categories business_id=%d", ErrMenuCatalogUnavailable, businessID)
	}
	var categories []MenuCategory
	if err := json.Unmarshal([]byte(menu.Categories), &categories); err != nil {
		return out, fmt.Errorf("%w: parse categories business_id=%d: %v", ErrMenuCatalogUnavailable, businessID, err)
	}
	for _, category := range categories {
		for _, item := range category.Items {
			if id := strings.TrimSpace(item.ID); id != "" {
				out.byID[id] = item.Price
			}
			if key := strings.ToLower(strings.TrimSpace(item.Name)); key != "" {
				out.byName[key] = item.Price
			}
		}
	}
	if len(out.byID) == 0 && len(out.byName) == 0 {
		return out, fmt.Errorf("%w: empty catalog business_id=%d", ErrMenuCatalogUnavailable, businessID)
	}
	return out, nil
}

func billItemsNeedDisplayHydration(items []BillItem) bool {
	for _, item := range items {
		if item.CreatedAt.IsZero() {
			return true
		}
		if money.IsInformationalBillLine(item.ItemType) && item.Price == 0 {
			return true
		}
	}
	return false
}

func billItemDisplayFallback(preferred time.Time, items []BillItem) time.Time {
	if !preferred.IsZero() {
		return preferred.UTC()
	}
	for _, item := range items {
		if !item.CreatedAt.IsZero() {
			return item.CreatedAt.UTC()
		}
	}
	return time.Now().UTC()
}

func applyCatalogPriceToBundleChild(item *BillItem, catalog menuCatalogPrices) error {
	if item == nil || !money.IsInformationalBillLine(item.ItemType) {
		return nil
	}
	// Money lives on the parent bundle line. A leftover $0 Price is an unset
	// catalog unit, not a $0 charge — fill it or fail. Never keep $0.
	item.Subtotal = 0
	if item.Price != 0 {
		return nil
	}
	if price, ok := catalog.lookup(item.MenuItemID, item.Name); ok && price != 0 {
		item.Price = price
		return nil
	}
	return fmt.Errorf("%w: name=%q menu_item_id=%q", ErrBundleChildCatalogMiss, item.Name, item.MenuItemID)
}

// stampBillItemsForPersist fills missing CreatedAt and leftover $0 bundle-child
// catalog prices before the JSON snapshot and relational rows are written.
// Callers that already set a real price/timestamp are left unchanged.
func stampBillItemsForPersist(conn *gorm.DB, items []BillItem, businessID uint, stamp time.Time) error {
	if stamp.IsZero() {
		stamp = time.Now().UTC()
	} else {
		stamp = stamp.UTC()
	}
	needCatalog := false
	for i := range items {
		// Mint an ID only when the line has none. Do not rewrite existing
		// non-UUID leftovers — cancel matches snapshot IDs as written.
		if strings.TrimSpace(items[i].ID) == "" {
			items[i].ID = uuid.New().String()
		}
		if items[i].CreatedAt.IsZero() {
			items[i].CreatedAt = stamp
		}
		if money.IsInformationalBillLine(items[i].ItemType) && items[i].Price == 0 {
			needCatalog = true
		}
	}
	var catalog menuCatalogPrices
	if needCatalog {
		loaded, err := loadMenuCatalogPrices(conn, businessID)
		if err != nil {
			return err
		}
		catalog = loaded
	}
	for i := range items {
		if err := applyCatalogPriceToBundleChild(&items[i], catalog); err != nil {
			return err
		}
	}
	return nil
}

func billItemsNeedCatalogHydration(items []BillItem) bool {
	for _, item := range items {
		if money.IsInformationalBillLine(item.ItemType) && item.Price == 0 {
			return true
		}
	}
	return false
}

func billListSnapshotNeedsHydration(raw string, items []BillItem) bool {
	if strings.Contains(raw, "0001-01-01") {
		return true
	}
	return billItemsNeedCatalogHydration(items)
}

func applyBillItemDisplayHydration(items []BillItem, catalog menuCatalogPrices, fallback time.Time) error {
	if len(items) == 0 || !billItemsNeedDisplayHydration(items) {
		return nil
	}
	stamp := billItemDisplayFallback(fallback, items)
	for i := range items {
		if items[i].CreatedAt.IsZero() {
			items[i].CreatedAt = stamp
		}
		if err := applyCatalogPriceToBundleChild(&items[i], catalog); err != nil {
			return err
		}
	}
	return nil
}

func hydrateBillItemsForRead(conn *gorm.DB, items []BillItem, businessID uint, fallback time.Time) error {
	if len(items) == 0 || !billItemsNeedDisplayHydration(items) {
		return nil
	}
	var catalog menuCatalogPrices
	if billItemsNeedCatalogHydration(items) {
		loaded, err := loadMenuCatalogPrices(conn, businessID)
		if err != nil {
			return err
		}
		catalog = loaded
	}
	return applyBillItemDisplayHydration(items, catalog, fallback)
}

// hydrateBillListRowsForRead fills leftover Date Night $0 / year-1 children on
// the list snapshot (bills.items). A catalog miss fails the list instead of
// shipping $0 children.
func hydrateBillListRowsForRead(conn *gorm.DB, rows []BillListRow) error {
	if len(rows) == 0 {
		return nil
	}
	if conn == nil {
		conn = db
	}
	var catalog menuCatalogPrices
	catalogLoaded := false
	for i := range rows {
		raw := strings.TrimSpace(rows[i].Items)
		if raw == "" || raw == "[]" || raw == "null" {
			continue
		}
		var items []BillItem
		if err := json.Unmarshal([]byte(rows[i].Items), &items); err != nil {
			continue
		}
		// Do not treat omitted created_at as leftover. Sparse pre-bill_items
		// snapshots like [{"name":"Burger","quantity":1}] must stay as stored.
		if !billListSnapshotNeedsHydration(rows[i].Items, items) {
			continue
		}
		if billItemsNeedCatalogHydration(items) && !catalogLoaded {
			loaded, err := loadMenuCatalogPrices(conn, rows[i].BusinessID)
			if err != nil {
				return err
			}
			catalog = loaded
			catalogLoaded = true
		}
		if err := applyBillItemDisplayHydration(items, catalog, rows[i].CreatedAt); err != nil {
			return err
		}
		encoded, err := json.Marshal(items)
		if err != nil {
			return fmt.Errorf("failed to marshal hydrated bill list items: %w", err)
		}
		rows[i].Items = string(encoded)
	}
	return nil
}

func hydrateBillSnapshotInPlace(conn *gorm.DB, bill *Bill) error {
	if bill == nil {
		return nil
	}
	raw := strings.TrimSpace(bill.Items)
	if raw == "" || raw == "[]" || raw == "null" {
		return nil
	}
	var items []BillItem
	if err := json.Unmarshal([]byte(bill.Items), &items); err != nil {
		return nil
	}
	if !billListSnapshotNeedsHydration(bill.Items, items) && !billItemsNeedDisplayHydration(items) {
		return nil
	}
	if err := hydrateBillItemsForRead(conn, items, bill.BusinessID, bill.CreatedAt); err != nil {
		return err
	}
	return syncBillItemsSnapshot(bill, items)
}

// HydrateBillSnapshotForWire fills leftover $0 / year-1 children on bill.Items
// before JSON or SSE. A catalog miss is an error — callers must not publish $0.
func HydrateBillSnapshotForWire(bill *Bill) error {
	return hydrateBillSnapshotInPlace(db, bill)
}

func hydrateBillItemsIfNeeded(conn *gorm.DB, billID uint, items []BillItem) ([]BillItem, error) {
	if !billItemsNeedDisplayHydration(items) {
		return items, nil
	}
	if conn == nil {
		conn = db
	}
	var meta struct {
		BusinessID uint
		CreatedAt  time.Time
	}
	if conn != nil {
		_ = conn.Model(&Bill{}).Select("business_id", "created_at").Where("id = ?", billID).Take(&meta).Error
	}
	if err := hydrateBillItemsForRead(conn, items, meta.BusinessID, meta.CreatedAt); err != nil {
		return items, err
	}
	return items, nil
}

func syncBillItemsSnapshot(bill *Bill, items []BillItem) error {
	if bill == nil {
		return nil
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal hydrated bill items: %w", err)
	}
	bill.Items = string(encoded)
	return nil
}

func finalizeBillItemsRead(bill *Bill, items []BillItem) ([]BillItem, error) {
	if bill == nil {
		return items, nil
	}
	if err := hydrateBillItemsForRead(db, items, bill.BusinessID, bill.CreatedAt); err != nil {
		return items, err
	}
	if err := syncBillItemsSnapshot(bill, items); err != nil {
		return items, err
	}
	return items, nil
}

func applyCatalogPriceToOrderChild(item *OrderItem, catalog menuCatalogPrices) error {
	if item == nil || !money.IsInformationalBillLine(item.ItemType) {
		return nil
	}
	item.Subtotal = 0
	if item.Price != 0 {
		return nil
	}
	if price, ok := catalog.lookup(item.MenuItemID, item.MenuItemName); ok && price != 0 {
		item.Price = price
		return nil
	}
	return fmt.Errorf("%w: name=%q menu_item_id=%q", ErrBundleChildCatalogMiss, item.MenuItemName, item.MenuItemID)
}

func orderItemsNeedCatalogHydration(items []OrderItem) bool {
	for _, item := range items {
		if money.IsInformationalBillLine(item.ItemType) && item.Price == 0 {
			return true
		}
	}
	return false
}

func orderSnapshotNeedsDisplayHydration(raw string, items []OrderItem) bool {
	if strings.Contains(raw, "0001-01-01") {
		return true
	}
	return orderItemsNeedCatalogHydration(items)
}

func applyOrderItemDisplayHydration(items []OrderItem, catalog menuCatalogPrices, stamp time.Time) error {
	if stamp.IsZero() {
		stamp = time.Now().UTC()
	} else {
		stamp = stamp.UTC()
	}
	for i := range items {
		if items[i].CreatedAt.IsZero() {
			items[i].CreatedAt = stamp
		}
		if err := applyCatalogPriceToOrderChild(&items[i], catalog); err != nil {
			return err
		}
	}
	return nil
}

func parseOrderItemsSnapshot(raw string) ([]OrderItem, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var items []OrderItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal items: %w", err)
	}
	return items, nil
}

func syncOrderItemsSnapshot(order *Order, items []OrderItem) error {
	if order == nil {
		return nil
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal hydrated order items: %w", err)
	}
	order.Items = string(encoded)
	return nil
}

// stampOrderItemsForPersist fills leftover $0 bundle-child catalog prices and
// missing CreatedAt before the kitchen JSON snapshot is written.
func stampOrderItemsForPersist(conn *gorm.DB, items []OrderItem, businessID uint, stamp time.Time) error {
	needCatalog := orderItemsNeedCatalogHydration(items)
	var catalog menuCatalogPrices
	if needCatalog {
		loaded, err := loadMenuCatalogPrices(conn, businessID)
		if err != nil {
			return err
		}
		catalog = loaded
	}
	needStamp := false
	for _, item := range items {
		if item.CreatedAt.IsZero() {
			needStamp = true
			break
		}
	}
	if !needCatalog && !needStamp {
		return nil
	}
	return applyOrderItemDisplayHydration(items, catalog, stamp)
}

func hydrateOrderSnapshotInPlace(conn *gorm.DB, order *Order) error {
	if order == nil {
		return nil
	}
	raw := strings.TrimSpace(order.Items)
	if raw == "" || raw == "[]" || raw == "null" {
		return nil
	}
	var items []OrderItem
	if err := json.Unmarshal([]byte(order.Items), &items); err != nil {
		return nil
	}
	if !orderSnapshotNeedsDisplayHydration(order.Items, items) {
		return nil
	}
	var catalog menuCatalogPrices
	if orderItemsNeedCatalogHydration(items) {
		loaded, err := loadMenuCatalogPrices(conn, order.BusinessID)
		if err != nil {
			return err
		}
		catalog = loaded
	}
	if err := applyOrderItemDisplayHydration(items, catalog, order.CreatedAt); err != nil {
		return err
	}
	return syncOrderItemsSnapshot(order, items)
}

func hydrateOrderSnapshotForRead(order *Order) error {
	return hydrateOrderSnapshotInPlace(db, order)
}

// HydrateOrderSnapshotForWire fills leftover $0 / year-1 children on order.Items
// before JSON or SSE. A catalog miss is an error — callers must not publish $0.
func HydrateOrderSnapshotForWire(order *Order) error {
	return hydrateOrderSnapshotInPlace(db, order)
}

// hydrateOrdersForRead rewrites leftover kitchen JSON ($0 bundle children and
// year-1 created_at) so KDS list/detail match GetBillByID. A catalog miss
// fails the list instead of shipping $0 children.
func hydrateOrdersForRead(conn *gorm.DB, orders []Order) error {
	if len(orders) == 0 {
		return nil
	}
	if conn == nil {
		conn = db
	}
	var catalog menuCatalogPrices
	catalogLoaded := false
	for i := range orders {
		raw := strings.TrimSpace(orders[i].Items)
		if raw == "" || raw == "[]" || raw == "null" {
			continue
		}
		var items []OrderItem
		if err := json.Unmarshal([]byte(orders[i].Items), &items); err != nil {
			continue
		}
		if !orderSnapshotNeedsDisplayHydration(orders[i].Items, items) {
			continue
		}
		if orderItemsNeedCatalogHydration(items) && !catalogLoaded {
			loaded, err := loadMenuCatalogPrices(conn, orders[i].BusinessID)
			if err != nil {
				return err
			}
			catalog = loaded
			catalogLoaded = true
		}
		if err := applyOrderItemDisplayHydration(items, catalog, orders[i].CreatedAt); err != nil {
			return err
		}
		if err := syncOrderItemsSnapshot(&orders[i], items); err != nil {
			return err
		}
	}
	return nil
}
