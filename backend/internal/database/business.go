package database

import (
	"encoding/json"
	"errors"
	"fmt"
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

// GenerateUniqueTableCode generates a unique table code for a business
func GenerateUniqueTableCode(businessID uint, baseName string) (string, error) {
	// Simple implementation - can be enhanced with better uniqueness logic
	var count int64
	db.Model(&Table{}).Where("business_id = ? AND is_active = ?", businessID, true).Count(&count)
	
	tableCode := fmt.Sprintf("%s_%d_%d", baseName, businessID, count+1)
	
	// Check if code already exists
	var existing Table
	if err := db.Where("table_code = ?", tableCode).First(&existing).Error; err == nil {
		// Code exists, append timestamp
		tableCode = fmt.Sprintf("%s_%d", tableCode, time.Now().Unix())
	}
	
	return tableCode, nil
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
