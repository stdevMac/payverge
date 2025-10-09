package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"payverge/internal/database"
)

// GetTableByCodePublic retrieves table information by table code for guests
func GetTableByCodePublic(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, err := database.GetTableByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}

	// Get business information
	business, err := database.GetBusinessByID(table.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	// Get menu for the business
	menu, categories, err := database.GetMenuByBusinessID(table.BusinessID)
	if err != nil {
		// Menu might not exist yet, return empty menu
		menu = &database.Menu{
			BusinessID: table.BusinessID,
			Categories: "[]",
		}
		categories = []database.MenuCategory{}
	}

	c.JSON(http.StatusOK, gin.H{
		"table":      table,
		"business":   business,
		"menu":       menu,
		"categories": categories,
	})
}

// GetOpenBillByTableCode retrieves the open bill for a table by table code
func GetOpenBillByTableCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, err := database.GetTableByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}

	// Get open bill for this table
	bill, items, err := database.GetOpenBillByTableID(table.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No open bill found for this table"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": items,
	})
}

// GetBusinessByTableCode retrieves business information by table code
func GetBusinessByTableCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, err := database.GetTableByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}

	business, err := database.GetBusinessByID(table.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"business": business,
	})
}

// CreateBillByTableCode allows guests to create a new bill for their table
func CreateBillByTableCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	// Get table information
	table, err := database.GetTableByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}

	// Check if there's already an open bill for this table
	_, _, err = database.GetOpenBillByTableID(table.ID)
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Table already has an open bill"})
		return
	}

	// Get business information for bill settings
	business, err := database.GetBusinessByID(table.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	// Create a new bill for the table
	bill := &database.Bill{
		BusinessID:      table.BusinessID,
		TableID:         table.ID,
		BillNumber:      "", // Will be auto-generated
		Subtotal:        0,
		TaxAmount:       0,
		ServiceFeeAmount: 0,
		TotalAmount:     0,
		PaidAmount:      0,
		TipAmount:       0,
		Status:          database.BillStatusOpen,
		SettlementAddr:  business.SettlementAddr,
		TippingAddr:     business.TippingAddr,
	}
	
	err = database.CreateBill(bill, []database.BillItem{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create bill"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"bill":  bill,
		"items": []database.BillItem{}, // Empty items array
	})
}

// GetMenuByTableCode retrieves menu information by table code
func GetMenuByTableCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, err := database.GetTableByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}

	menu, categories, err := database.GetMenuByBusinessID(table.BusinessID)
	if err != nil {
		// Menu might not exist yet, return empty menu
		menu = &database.Menu{
			BusinessID: table.BusinessID,
			Categories: "[]",
		}
		categories = []database.MenuCategory{}
	}

	c.JSON(http.StatusOK, gin.H{
		"menu":       menu,
		"categories": categories,
	})
}

// GetBillByNumber retrieves a bill by its bill number (public endpoint for guests)
func GetBillByNumberPublic(c *gin.Context) {
	billNumber := c.Param("bill_number")
	if billNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bill number is required"})
		return
	}

	bill, items, err := database.GetBillByNumber(billNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": items,
	})
}

// GetTableStatusByCode retrieves table status and current bill info by table code
func GetTableStatusByCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, err := database.GetTableByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}

	// Try to get open bill for this table
	bill, items, err := database.GetOpenBillByTableID(table.ID)
	
	response := gin.H{
		"table": table,
	}

	if err != nil {
		// No open bill
		response["has_open_bill"] = false
		response["bill"] = nil
		response["items"] = nil
	} else {
		// Has open bill
		response["has_open_bill"] = true
		response["bill"] = bill
		response["items"] = items
	}

	c.JSON(http.StatusOK, response)
}

// CreateGuestOrder creates an order from a guest (public endpoint)
func CreateGuestOrder(c *gin.Context) {
	tableCode := c.Param("code")
	
	fmt.Printf("CreateGuestOrder called for table code: %s\n", tableCode)
	
	// Get table by code
	table, err := database.GetTableByCode(tableCode)
	if err != nil {
		fmt.Printf("Error getting table by code %s: %v\n", tableCode, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}
	
	fmt.Printf("Found table: ID=%d, BusinessID=%d, Name=%s\n", table.ID, table.BusinessID, table.Name)

	// Parse request body
	var req struct {
		BillID uint `json:"bill_id" binding:"required"`
		Items  []struct {
			MenuItemName    string  `json:"menu_item_name" binding:"required"`
			Quantity        int     `json:"quantity" binding:"required"`
			Price           float64 `json:"price" binding:"required"`
			SpecialRequests string  `json:"special_requests"`
		} `json:"items" binding:"required"`
		Notes string `json:"notes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		fmt.Printf("Error parsing request body: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	fmt.Printf("Parsed request: BillID=%d, Items=%+v, Notes=%s\n", req.BillID, req.Items, req.Notes)

	db := database.GetDB()
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database connection failed"})
		return
	}

	// Verify the bill belongs to this table
	var bill database.Bill
	if err := db.Where("id = ? AND table_id = ?", req.BillID, table.ID).First(&bill).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found for this table"})
		return
	}

	// Generate order number
	timestamp := time.Now().UnixNano()
	orderNumber := fmt.Sprintf("G%d-%d", table.BusinessID, timestamp%100000000)

	// Create the order (pending approval)
	order := database.Order{
		BillID:      req.BillID,
		BusinessID:  table.BusinessID,
		OrderNumber: orderNumber,
		Status:      database.OrderStatusPending, // Requires staff approval
		CreatedBy:   "guest",
		Notes:       req.Notes,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Convert request items to order items
	var orderItems []database.OrderItem
	for _, itemReq := range req.Items {
		orderItem := database.OrderItem{
			ID:              fmt.Sprintf("%s-%d", itemReq.MenuItemName, time.Now().UnixNano()),
			MenuItemName:    itemReq.MenuItemName,
			Quantity:        itemReq.Quantity,
			Price:           itemReq.Price,
			SpecialRequests: itemReq.SpecialRequests,
			Subtotal:        itemReq.Price * float64(itemReq.Quantity),
		}
		orderItems = append(orderItems, orderItem)
	}

	// Create order with items
	if err := database.CreateOrder(&order, orderItems); err != nil {
		fmt.Printf("Error creating order: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create order"})
		return
	}
	
	fmt.Printf("Order created successfully: ID=%d, OrderNumber=%s, Status=%s\n", order.ID, order.OrderNumber, order.Status)

	// Load the complete order with relationships
	if err := db.
		Preload("Bill").
		Preload("Business").
		First(&order, order.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load order"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Order created successfully and pending staff approval",
		"order":   order,
	})
}
