package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Business operations

// CreateBusiness creates a new business in the database
func CreateBusiness(business *Business) error {
	if err := db.Create(business).Error; err != nil {
		return fmt.Errorf("failed to create business: %w", err)
	}
	return nil
}

// GetBusinessByID retrieves a business by its ID
func GetBusinessByID(id uint) (*Business, error) {
	var business Business
	if err := db.First(&business, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("business not found")
		}
		return nil, fmt.Errorf("failed to get business: %w", err)
	}
	return &business, nil
}

// GetBusinessByOwnerAddress retrieves businesses owned by a specific address
func GetBusinessByOwnerAddress(ownerAddress string) ([]Business, error) {
	var businesses []Business
	if err := db.Where("owner_address = ? AND is_active = ?", ownerAddress, true).Find(&businesses).Error; err != nil {
		return nil, fmt.Errorf("failed to get businesses: %w", err)
	}
	return businesses, nil
}

// UpdateBusiness updates an existing business
func UpdateBusiness(business *Business) error {
	if err := db.Save(business).Error; err != nil {
		return fmt.Errorf("failed to update business: %w", err)
	}
	return nil
}

// DeleteBusiness soft deletes a business (sets is_active to false)
func DeleteBusiness(id uint) error {
	if err := db.Model(&Business{}).Where("id = ?", id).Update("is_active", false).Error; err != nil {
		return fmt.Errorf("failed to delete business: %w", err)
	}
	return nil
}

// Menu operations

// CreateMenu creates a new menu for a business
func CreateMenu(menu *Menu, categories []MenuCategory) error {
	// Convert categories to JSON string for SQLite storage
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		return fmt.Errorf("failed to marshal categories: %w", err)
	}
	menu.Categories = string(categoriesJSON)

	if err := db.Create(menu).Error; err != nil {
		return fmt.Errorf("failed to create menu: %w", err)
	}
	return nil
}

// GetMenuByBusinessID retrieves the active menu for a business
func GetMenuByBusinessID(businessID uint) (*Menu, []MenuCategory, error) {
	var menu Menu
	if err := db.Where("business_id = ? AND is_active = ?", businessID, true).First(&menu).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("menu not found")
		}
		return nil, nil, fmt.Errorf("failed to get menu: %w", err)
	}

	// Parse categories from JSON
	var categories []MenuCategory
	if menu.Categories != "" {
		if err := json.Unmarshal([]byte(menu.Categories), &categories); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal categories: %w", err)
		}
	}

	return &menu, categories, nil
}

// UpdateMenu updates an existing menu
func UpdateMenu(menu *Menu, categories []MenuCategory) error {
	// Convert categories to JSON string for SQLite storage
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		return fmt.Errorf("failed to marshal categories: %w", err)
	}
	menu.Categories = string(categoriesJSON)

	if err := db.Save(menu).Error; err != nil {
		return fmt.Errorf("failed to update menu: %w", err)
	}
	return nil
}

// AddMenuCategory adds a new category to an existing menu, creating the menu if it doesn't exist
func AddMenuCategory(businessID uint, category MenuCategory) error {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		// If menu doesn't exist, create a new one
		if strings.Contains(err.Error(), "menu not found") {
			newMenu := &Menu{
				BusinessID: businessID,
				Categories: "",
				IsActive:   true,
			}
			emptyCategories := []MenuCategory{}
			if err := CreateMenu(newMenu, emptyCategories); err != nil {
				return fmt.Errorf("failed to create new menu: %w", err)
			}
			menu = newMenu
			categories = emptyCategories
		} else {
			return fmt.Errorf("failed to get menu: %w", err)
		}
	}

	// Add new category
	categories = append(categories, category)

	// Update menu with new categories
	return UpdateMenu(menu, categories)
}

// UpdateMenuCategory updates a specific category in a menu
func UpdateMenuCategory(businessID uint, categoryIndex int, updatedCategory MenuCategory) error {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return fmt.Errorf("failed to get menu: %w", err)
	}

	if categoryIndex < 0 || categoryIndex >= len(categories) {
		return fmt.Errorf("category index out of range")
	}

	// Update the category
	categories[categoryIndex] = updatedCategory

	// Update menu with modified categories
	return UpdateMenu(menu, categories)
}

// DeleteMenuCategory removes a category from a menu
func DeleteMenuCategory(businessID uint, categoryIndex int) error {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return fmt.Errorf("failed to get menu: %w", err)
	}

	if categoryIndex < 0 || categoryIndex >= len(categories) {
		return fmt.Errorf("category index out of range")
	}

	// Remove the category
	categories = append(categories[:categoryIndex], categories[categoryIndex+1:]...)

	// Update menu with modified categories
	return UpdateMenu(menu, categories)
}

// AddMenuItem adds a new item to a specific category
func AddMenuItem(businessID uint, categoryIndex int, item MenuItem) error {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return fmt.Errorf("failed to get menu: %w", err)
	}

	if categoryIndex < 0 || categoryIndex >= len(categories) {
		return fmt.Errorf("category index out of range")
	}

	// Add item to category
	categories[categoryIndex].Items = append(categories[categoryIndex].Items, item)

	// Update menu with modified categories
	return UpdateMenu(menu, categories)
}

// UpdateMenuItem updates a specific menu item
func UpdateMenuItem(businessID uint, categoryIndex, itemIndex int, updatedItem MenuItem) error {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return fmt.Errorf("failed to get menu: %w", err)
	}

	if categoryIndex < 0 || categoryIndex >= len(categories) {
		return fmt.Errorf("category index out of range")
	}

	if itemIndex < 0 || itemIndex >= len(categories[categoryIndex].Items) {
		return fmt.Errorf("item index out of range")
	}

	// Update the item
	categories[categoryIndex].Items[itemIndex] = updatedItem

	// Update menu with modified categories
	return UpdateMenu(menu, categories)
}

// DeleteMenuItem removes an item from a category
func DeleteMenuItem(businessID uint, categoryIndex, itemIndex int) error {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return fmt.Errorf("failed to get menu: %w", err)
	}

	if categoryIndex < 0 || categoryIndex >= len(categories) {
		return fmt.Errorf("category index out of range")
	}

	if itemIndex < 0 || itemIndex >= len(categories[categoryIndex].Items) {
		return fmt.Errorf("item index out of range")
	}

	// Remove the item
	items := categories[categoryIndex].Items
	categories[categoryIndex].Items = append(items[:itemIndex], items[itemIndex+1:]...)

	// Update menu with modified categories
	return UpdateMenu(menu, categories)
}

// Table operations

// CreateTable creates a new table for a business
func CreateTable(table *Table) error {
	if err := db.Create(table).Error; err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}
	return nil
}

// GetTablesByBusinessID retrieves all active tables for a business
func GetTablesByBusinessID(businessID uint) ([]Table, error) {
	var tables []Table
	if err := db.Where("business_id = ? AND is_active = ?", businessID, true).Find(&tables).Error; err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}
	return tables, nil
}

// GetTableByCode retrieves a table by its unique code
func GetTableByCode(tableCode string) (*Table, error) {
	var table Table
	if err := db.Where("table_code = ? AND is_active = ?", tableCode, true).First(&table).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("table not found")
		}
		return nil, fmt.Errorf("failed to get table: %w", err)
	}
	return &table, nil
}

// GetTableByID retrieves a table by its ID
func GetTableByID(id uint) (*Table, error) {
	var table Table
	if err := db.First(&table, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("table not found")
		}
		return nil, fmt.Errorf("failed to get table: %w", err)
	}
	return &table, nil
}

// UpdateTable updates an existing table
func UpdateTable(table *Table) error {
	if err := db.Save(table).Error; err != nil {
		return fmt.Errorf("failed to update table: %w", err)
	}
	return nil
}

// DeleteTable soft deletes a table (sets is_active to false)
func DeleteTable(id uint) error {
	if err := db.Model(&Table{}).Where("id = ?", id).Update("is_active", false).Error; err != nil {
		return fmt.Errorf("failed to delete table: %w", err)
	}
	return nil
}

// GenerateUniqueTableCode generates a unique 10-character random table code for a business
func GenerateUniqueTableCode(businessID uint, baseName string) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const codeLength = 10

	// Try up to 10 times to generate a unique code
	for attempts := 0; attempts < 10; attempts++ {
		// Generate random 10-character string
		tableCode := make([]byte, codeLength)
		for i := range tableCode {
			tableCode[i] = charset[rand.Intn(len(charset))]
		}

		codeStr := string(tableCode)

		// Check if code already exists
		var existing Table
		if err := db.Where("table_code = ?", codeStr).First(&existing).Error; err != nil {
			// Code doesn't exist (GORM returns error when not found), so it's unique
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return codeStr, nil
			}
			// Some other database error occurred
			return "", err
		}
		// Code exists, try again
	}

	return "", fmt.Errorf("failed to generate unique table code after 10 attempts")
}

// Bill operations

// CreateBill creates a new bill
func CreateBill(bill *Bill, items []BillItem) error {
	// Convert items to JSON string for SQLite storage
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal items: %w", err)
	}
	bill.Items = string(itemsJSON)

	if err := db.Create(bill).Error; err != nil {
		return fmt.Errorf("failed to create bill: %w", err)
	}
	return nil
}

// GetBillByID retrieves a bill by its ID with items parsed
func GetBillByID(id uint) (*Bill, []BillItem, error) {
	var bill Bill
	if err := db.Preload("Business").Preload("Table").Preload("Payments").First(&bill, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("bill not found")
		}
		return nil, nil, fmt.Errorf("failed to get bill: %w", err)
	}

	// Parse items from JSON
	var items []BillItem
	if bill.Items != "" {
		if err := json.Unmarshal([]byte(bill.Items), &items); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal items: %w", err)
		}
	}

	return &bill, items, nil
}

// GetBillByNumber retrieves a bill by its bill number
func GetBillByNumber(billNumber string) (*Bill, []BillItem, error) {
	var bill Bill
	if err := db.Preload("Business").Preload("Table").Preload("Payments").Where("bill_number = ?", billNumber).First(&bill).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("bill not found")
		}
		return nil, nil, fmt.Errorf("failed to get bill: %w", err)
	}

	// Parse items from JSON
	var items []BillItem
	if bill.Items != "" {
		if err := json.Unmarshal([]byte(bill.Items), &items); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal items: %w", err)
		}
	}

	return &bill, items, nil
}

// GetOpenBillsByBusinessID retrieves all open bills for a business
func GetOpenBillsByBusinessID(businessID uint) ([]Bill, error) {
	var bills []Bill
	if err := db.Where("business_id = ? AND status = ?", businessID, BillStatusOpen).Find(&bills).Error; err != nil {
		return nil, fmt.Errorf("failed to get open bills: %w", err)
	}
	return bills, nil
}

// GetAllBillsByBusinessID retrieves all bills for a business (all statuses)
func GetAllBillsByBusinessID(businessID uint) ([]Bill, error) {
	var bills []Bill
	if err := db.Where("business_id = ?", businessID).Order("created_at DESC").Find(&bills).Error; err != nil {
		return nil, fmt.Errorf("failed to get bills: %w", err)
	}
	return bills, nil
}

// GetOpenBillByTableID retrieves the open bill for a specific table
func GetOpenBillByTableID(tableID uint) (*Bill, []BillItem, error) {
	var bill Bill
	if err := db.Where("table_id = ? AND status = ?", tableID, BillStatusOpen).First(&bill).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("no open bill found for table")
		}
		return nil, nil, fmt.Errorf("failed to get open bill: %w", err)
	}

	// Parse items from JSON
	var items []BillItem
	if bill.Items != "" {
		if err := json.Unmarshal([]byte(bill.Items), &items); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal items: %w", err)
		}
	}

	return &bill, items, nil
}

// UpdateBill updates an existing bill
func UpdateBill(bill *Bill, items []BillItem) error {
	// Convert items to JSON string for SQLite storage
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal items: %w", err)
	}
	bill.Items = string(itemsJSON)

	if err := db.Save(bill).Error; err != nil {
		return fmt.Errorf("failed to update bill: %w", err)
	}
	return nil
}

// CloseBill closes a bill and sets the closed timestamp
func CloseBill(billID uint) error {
	now := time.Now()
	if err := db.Model(&Bill{}).Where("id = ?", billID).Updates(map[string]interface{}{
		"status":    BillStatusClosed,
		"closed_at": &now,
	}).Error; err != nil {
		return fmt.Errorf("failed to close bill: %w", err)
	}
	return nil
}

// Payment operations

// CreatePayment creates a new payment record
func CreatePayment(payment *Payment) error {
	if err := db.Create(payment).Error; err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}
	return nil
}

// GetPaymentsByBillID retrieves all payments for a bill
func GetPaymentsByBillID(billID uint) ([]Payment, error) {
	var payments []Payment
	if err := db.Where("bill_id = ?", billID).Find(&payments).Error; err != nil {
		return nil, fmt.Errorf("failed to get payments: %w", err)
	}
	return payments, nil
}

// GetPaymentByTxHash retrieves a payment by its transaction hash
func GetPaymentByTxHash(txHash string) (*Payment, error) {
	var payment Payment
	if err := db.Where("tx_hash = ?", txHash).First(&payment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("payment not found")
		}
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}
	return &payment, nil
}

// UpdatePaymentStatus updates the status of a payment
func UpdatePaymentStatus(paymentID uint, status PaymentStatus) error {
	if err := db.Model(&Payment{}).Where("id = ?", paymentID).Update("status", status).Error; err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}
	return nil
}

// UpdateBillPaidAmount updates the paid amount on a bill (called when payments are confirmed)
func UpdateBillPaidAmount(billID uint, paidAmount, tipAmount float64) error {
	if err := db.Model(&Bill{}).Where("id = ?", billID).Updates(map[string]interface{}{
		"paid_amount": paidAmount,
		"tip_amount":  tipAmount,
	}).Error; err != nil {
		return fmt.Errorf("failed to update bill paid amount: %w", err)
	}
	return nil
}

// CheckBillFullyPaid checks if a bill is fully paid and updates status if needed
func CheckBillFullyPaid(billID uint) error {
	var bill Bill
	if err := db.First(&bill, billID).Error; err != nil {
		return fmt.Errorf("failed to get bill: %w", err)
	}

	if bill.PaidAmount >= bill.TotalAmount {
		if err := db.Model(&Bill{}).Where("id = ?", billID).Update("status", BillStatusPaid).Error; err != nil {
			return fmt.Errorf("failed to update bill status to paid: %w", err)
		}
	}

	return nil
}

// Order operations

// CreateOrder creates a new order within a bill
func CreateOrder(order *Order, items []OrderItem) error {
	// Convert items to JSON string for SQLite storage
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal items: %w", err)
	}
	order.Items = string(itemsJSON)

	if err := db.Create(order).Error; err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}
	return nil
}

// GetOrderByID retrieves an order by its ID with items parsed
func GetOrderByID(id uint) (*Order, []OrderItem, error) {
	var order Order
	if err := db.Preload("Bill").Preload("Business").First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("order not found")
		}
		return nil, nil, fmt.Errorf("failed to get order: %w", err)
	}

	// Parse items from JSON
	var items []OrderItem
	if order.Items != "" {
		if err := json.Unmarshal([]byte(order.Items), &items); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal items: %w", err)
		}
	}

	return &order, items, nil
}

// GetOrdersByBillID retrieves all orders for a specific bill
func GetOrdersByBillID(billID uint) ([]Order, error) {
	var orders []Order
	if err := db.Where("bill_id = ?", billID).Order("created_at DESC").Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}
	return orders, nil
}

// GetOrdersByBusinessID retrieves orders for a business with optional status filter
func GetOrdersByBusinessID(businessID uint, status string) ([]Order, error) {
	query := db.Where("business_id = ?", businessID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	
	var orders []Order
	if err := query.Preload("Bill").Preload("Business").Order("created_at DESC").Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}
	return orders, nil
}

// UpdateOrderStatus updates the status of an order
func UpdateOrderStatus(orderID uint, status OrderStatus, approvedBy string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	
	if status == OrderStatusApproved && approvedBy != "" {
		updates["approved_by"] = approvedBy
		updates["approved_at"] = time.Now()
	}
	
	if err := db.Model(&Order{}).Where("id = ?", orderID).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}
	return nil
}

// Counter operations

// CreateCountersForBusiness creates default counters for a business
func CreateCountersForBusiness(businessID uint, count int, prefix string) error {
	var counters []Counter
	for i := 1; i <= count; i++ {
		counter := Counter{
			BusinessID:    businessID,
			CounterNumber: i,
			Name:          fmt.Sprintf("%s%d", prefix, i),
			IsActive:      true,
		}
		counters = append(counters, counter)
	}

	if err := db.Create(&counters).Error; err != nil {
		return fmt.Errorf("failed to create counters: %w", err)
	}
	return nil
}

// GetBusinessCounters retrieves all counters for a business
func GetBusinessCounters(businessID uint) ([]Counter, error) {
	var counters []Counter
	if err := db.Where("business_id = ?", businessID).Order("counter_number").Find(&counters).Error; err != nil {
		return nil, fmt.Errorf("failed to get counters: %w", err)
	}
	return counters, nil
}

// GetAvailableCounters retrieves counters that don't have active bills
func GetAvailableCounters(businessID uint) ([]Counter, error) {
	var counters []Counter
	if err := db.Where("business_id = ? AND is_active = ? AND current_bill_id IS NULL", businessID, true).Order("counter_number").Find(&counters).Error; err != nil {
		return nil, fmt.Errorf("failed to get available counters: %w", err)
	}
	return counters, nil
}

// UpdateCounterBill updates the current bill for a counter
func UpdateCounterBill(counterID uint, billID *uint) error {
	if err := db.Model(&Counter{}).Where("id = ?", counterID).Update("current_bill_id", billID).Error; err != nil {
		return fmt.Errorf("failed to update counter bill: %w", err)
	}
	return nil
}

// UpdateBusinessCounters updates counter configuration for a business
func UpdateBusinessCounters(businessID uint, enabled bool, count int, prefix string) error {
	// Start transaction
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Update business settings
	if err := tx.Model(&Business{}).Where("id = ?", businessID).Updates(map[string]interface{}{
		"counter_enabled": enabled,
		"counter_count":   count,
		"counter_prefix":  prefix,
	}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update business counter settings: %w", err)
	}

	if enabled {
		// Get existing counters
		var existingCounters []Counter
		if err := tx.Where("business_id = ?", businessID).Find(&existingCounters).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to get existing counters: %w", err)
		}

		// Deactivate counters beyond the new count
		if err := tx.Model(&Counter{}).Where("business_id = ? AND counter_number > ?", businessID, count).Update("is_active", false).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to deactivate excess counters: %w", err)
		}

		// Create or update counters up to the new count
		for i := 1; i <= count; i++ {
			var existingCounter Counter
			err := tx.Where("business_id = ? AND counter_number = ?", businessID, i).First(&existingCounter).Error
			
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Create new counter
				newCounter := Counter{
					BusinessID:    businessID,
					CounterNumber: i,
					Name:          fmt.Sprintf("%s%d", prefix, i),
					IsActive:      true,
				}
				if err := tx.Create(&newCounter).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to create counter %d: %w", i, err)
				}
			} else if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to check existing counter %d: %w", i, err)
			} else {
				// Update existing counter
				if err := tx.Model(&existingCounter).Updates(map[string]interface{}{
					"name":      fmt.Sprintf("%s%d", prefix, i),
					"is_active": true,
				}).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to update counter %d: %w", i, err)
				}
			}
		}
	} else {
		// Disable all counters for this business
		if err := tx.Model(&Counter{}).Where("business_id = ?", businessID).Update("is_active", false).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to disable counters: %w", err)
		}
	}

	return tx.Commit().Error
}

// GetCounterByID retrieves a counter by its ID
func GetCounterByID(id uint) (*Counter, error) {
	var counter Counter
	if err := db.First(&counter, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("counter not found")
		}
		return nil, fmt.Errorf("failed to get counter: %w", err)
	}
	return &counter, nil
}

// MarkBillAsPaid updates a bill's payment status and details
func MarkBillAsPaid(billID uint, amountPaid, tipAmount float64, paymentMethod, notes string) error {
	now := time.Now()
	
	updates := map[string]interface{}{
		"paid_amount":    amountPaid,
		"tip_amount":     tipAmount,
		"status":         BillStatusPaid,
		"updated_at":     now,
		"closed_at":      &now,
	}
	
	// Add payment method and notes to the bill's notes field if provided
	if notes != "" {
		var existingNotes string
		if err := db.Model(&Bill{}).Where("id = ?", billID).Select("notes").Scan(&existingNotes).Error; err == nil {
			if existingNotes != "" {
				updates["notes"] = fmt.Sprintf("%s\n\nPayment: %s via %s - %s", existingNotes, 
					fmt.Sprintf("$%.2f", amountPaid), paymentMethod, notes)
			} else {
				updates["notes"] = fmt.Sprintf("Payment: %s via %s - %s", 
					fmt.Sprintf("$%.2f", amountPaid), paymentMethod, notes)
			}
		}
	}
	
	return db.Model(&Bill{}).Where("id = ?", billID).Updates(updates).Error
}
