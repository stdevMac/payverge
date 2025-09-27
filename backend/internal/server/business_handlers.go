package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"payverge/internal/database"
)

// Business creation request
type CreateBusinessRequest struct {
	Name             string                      `json:"name" binding:"required"`
	Logo             string                      `json:"logo"`
	Address          database.BusinessAddress   `json:"address"`
	SettlementAddr   string                      `json:"settlement_address" binding:"required"`
	TippingAddr      string                      `json:"tipping_address" binding:"required"`
	TaxRate          float64                     `json:"tax_rate"`
	ServiceFeeRate   float64                     `json:"service_fee_rate"`
	TaxInclusive     bool                        `json:"tax_inclusive"`
	ServiceInclusive bool                        `json:"service_inclusive"`
	// New fields for enhanced business features
	Description          string                  `json:"description"`
	CustomURL            string                  `json:"custom_url"`
	Phone                string                  `json:"phone"`
	Website              string                  `json:"website"`
	SocialMedia          string                  `json:"social_media"`
	BannerImages         string                  `json:"banner_images"`
	BusinessPageEnabled  bool                    `json:"business_page_enabled"`
	ShowReviews          bool                    `json:"show_reviews"`
	GoogleReviewsEnabled bool                    `json:"google_reviews_enabled"`
}

// Business update request
type UpdateBusinessRequest struct {
	Name             string                      `json:"name"`
	Logo             string                      `json:"logo"`
	Address          database.BusinessAddress   `json:"address"`
	SettlementAddr   string                      `json:"settlement_address"`
	TippingAddr      string                      `json:"tipping_address"`
	TaxRate          float64                     `json:"tax_rate"`
	ServiceFeeRate   float64                     `json:"service_fee_rate"`
	TaxInclusive     bool                        `json:"tax_inclusive"`
	ServiceInclusive bool                        `json:"service_inclusive"`
	// New fields for enhanced business features
	Description          string                  `json:"description"`
	CustomURL            string                  `json:"custom_url"`
	Phone                string                  `json:"phone"`
	Website              string                  `json:"website"`
	SocialMedia          string                  `json:"social_media"`
	BannerImages         string                  `json:"banner_images"`
	BusinessPageEnabled  bool                    `json:"business_page_enabled"`
	ShowReviews          bool                    `json:"show_reviews"`
	GoogleReviewsEnabled bool                    `json:"google_reviews_enabled"`
}

// CreateBusiness creates a new business for the authenticated user
func CreateBusiness(c *gin.Context) {
	// Get user address from authentication middleware
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req CreateBusinessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate Ethereum addresses
	if !isValidEthereumAddress(req.SettlementAddr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid settlement address"})
		return
	}
	if !isValidEthereumAddress(req.TippingAddr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tipping address"})
		return
	}

	// Validate custom URL if provided
	if req.CustomURL != "" {
		if err := validateCustomURL(req.CustomURL, 0); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	business := &database.Business{
		OwnerAddress:         userAddress.(string),
		Name:                 req.Name,
		Logo:                 req.Logo,
		Address:              req.Address,
		SettlementAddr:       req.SettlementAddr,
		TippingAddr:          req.TippingAddr,
		TaxRate:              req.TaxRate,
		ServiceFeeRate:       req.ServiceFeeRate,
		TaxInclusive:         req.TaxInclusive,
		ServiceInclusive:     req.ServiceInclusive,
		IsActive:             true,
		// New fields
		Description:          req.Description,
		CustomURL:            req.CustomURL,
		Phone:                req.Phone,
		Website:              req.Website,
		SocialMedia:          req.SocialMedia,
		BannerImages:         req.BannerImages,
		BusinessPageEnabled:  req.BusinessPageEnabled,
		ShowReviews:          req.ShowReviews,
		GoogleReviewsEnabled: req.GoogleReviewsEnabled,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	if err := database.CreateBusiness(business); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create business"})
		return
	}

	c.JSON(http.StatusCreated, business)
}

// GetMyBusinesses retrieves all businesses owned by the authenticated user
func GetMyBusinesses(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businesses, err := database.GetBusinessByOwnerAddress(userAddress.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve businesses"})
		return
	}

	c.JSON(http.StatusOK, businesses)
}

// GetBusiness retrieves a specific business by ID
func GetBusiness(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		return
	}

	c.JSON(http.StatusOK, business)
}

// UpdateBusiness updates an existing business
func UpdateBusiness(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Check if business exists and user owns it
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't own this business"})
		return
	}

	var req UpdateBusinessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update business fields
	if req.Name != "" {
		business.Name = req.Name
	}
	if req.Logo != "" {
		business.Logo = req.Logo
	}
	business.Address = req.Address
	if req.SettlementAddr != "" {
		if !isValidEthereumAddress(req.SettlementAddr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid settlement address"})
			return
		}
		business.SettlementAddr = req.SettlementAddr
	}
	if req.TippingAddr != "" {
		if !isValidEthereumAddress(req.TippingAddr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tipping address"})
			return
		}
		business.TippingAddr = req.TippingAddr
	}
	business.TaxRate = req.TaxRate
	business.ServiceFeeRate = req.ServiceFeeRate
	business.TaxInclusive = req.TaxInclusive
	business.ServiceInclusive = req.ServiceInclusive
	
	// Update new fields
	if req.Description != "" {
		business.Description = req.Description
	}
	if req.CustomURL != "" {
		// Check if custom URL is already taken by another business
		if err := validateCustomURL(req.CustomURL, uint(businessID)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		business.CustomURL = req.CustomURL
	}
	if req.Phone != "" {
		business.Phone = req.Phone
	}
	if req.Website != "" {
		business.Website = req.Website
	}
	if req.SocialMedia != "" {
		business.SocialMedia = req.SocialMedia
	}
	if req.BannerImages != "" {
		business.BannerImages = req.BannerImages
	}
	business.BusinessPageEnabled = req.BusinessPageEnabled
	business.ShowReviews = req.ShowReviews
	business.GoogleReviewsEnabled = req.GoogleReviewsEnabled
	
	business.UpdatedAt = time.Now()

	if err := database.UpdateBusiness(business); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update business"})
		return
	}

	c.JSON(http.StatusOK, business)
}

// DeleteBusiness soft deletes a business
func DeleteBusiness(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Check if business exists and user owns it
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't own this business"})
		return
	}

	if err := database.DeleteBusiness(uint(businessID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete business"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Business deleted successfully"})
}

// Menu management

// CreateMenuRequest represents a request to create/update a menu
type CreateMenuRequest struct {
	Categories []database.MenuCategory `json:"categories" binding:"required"`
}

// CreateMenu creates or updates a menu for a business
func CreateMenu(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Check if business exists and user owns it
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't own this business"})
		return
	}

	var req CreateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if menu already exists
	existingMenu, _, err := database.GetMenuByBusinessID(uint(businessID))
	if err != nil && !strings.Contains(err.Error(), "not found") {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check existing menu"})
		return
	}

	if existingMenu != nil {
		// Update existing menu
		existingMenu.UpdatedAt = time.Now()
		if err := database.UpdateMenu(existingMenu, req.Categories); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update menu"})
			return
		}
		c.JSON(http.StatusOK, existingMenu)
	} else {
		// Create new menu
		menu := &database.Menu{
			BusinessID: uint(businessID),
			IsActive:   true,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := database.CreateMenu(menu, req.Categories); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create menu"})
			return
		}
		c.JSON(http.StatusCreated, menu)
	}
}

// GetMenu retrieves a menu for a business
func GetMenu(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	menu, categories, err := database.GetMenuByBusinessID(uint(businessID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Return empty menu structure instead of error
			c.JSON(http.StatusOK, gin.H{
				"id":          0,
				"business_id": businessID,
				"categories":  []interface{}{},
				"is_active":   true,
				"created_at":  "",
				"updated_at":  "",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve menu"})
		return
	}

	response := struct {
		*database.Menu
		ParsedCategories []database.MenuCategory `json:"parsed_categories"`
	}{
		Menu:             menu,
		ParsedCategories: categories,
	}

	c.JSON(http.StatusOK, response)
}

// Helper function to validate Ethereum addresses
func isValidEthereumAddress(address string) bool {
	// Basic validation - starts with 0x and is 42 characters long
	if len(address) != 42 {
		return false
	}
	if !strings.HasPrefix(address, "0x") {
		return false
	}
	// Additional validation could be added here (checksum, etc.)
	return true
}

// Phase 2: Enhanced Menu Management API Endpoints

// Menu category request structures
type AddCategoryRequest struct {
	Name        string                `json:"name" binding:"required"`
	Description string                `json:"description"`
	Items       []database.MenuItem   `json:"items"`
}

type UpdateCategoryRequest struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Items       []database.MenuItem   `json:"items"`
}

// Menu item request structures
type AddMenuItemRequest struct {
	CategoryIndex int                 `json:"category_index" binding:"min=0"`
	Item          database.MenuItem   `json:"item" binding:"required"`
}

type UpdateMenuItemRequest struct {
	CategoryIndex int                 `json:"category_index" binding:"min=0"`
	ItemIndex     int                 `json:"item_index" binding:"min=0"`
	Item          database.MenuItem   `json:"item" binding:"required"`
}

// AddMenuCategory adds a new category to a business menu
func AddMenuCategory(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	var req AddCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	category := database.MenuCategory{
		Name:        req.Name,
		Description: req.Description,
		Items:       req.Items,
	}

	if err := database.AddMenuCategory(uint(businessID), category); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Category added successfully"})
}

// UpdateMenuCategory updates a specific category in a business menu
func UpdateMenuCategory(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	categoryIndex, err := strconv.Atoi(c.Param("category_index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category index"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	var req UpdateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	category := database.MenuCategory{
		Name:        req.Name,
		Description: req.Description,
		Items:       req.Items,
	}

	if err := database.UpdateMenuCategory(uint(businessID), categoryIndex, category); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Category updated successfully"})
}

// DeleteMenuCategory removes a category from a business menu
func DeleteMenuCategory(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	categoryIndex, err := strconv.Atoi(c.Param("category_index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category index"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	if err := database.DeleteMenuCategory(uint(businessID), categoryIndex); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Category deleted successfully"})
}

// AddMenuItem adds a new item to a menu category
func AddMenuItem(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	var req AddMenuItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := database.AddMenuItem(uint(businessID), req.CategoryIndex, req.Item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Menu item added successfully"})
}

// UpdateMenuItem updates a specific menu item
func UpdateMenuItem(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	var req UpdateMenuItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := database.UpdateMenuItem(uint(businessID), req.CategoryIndex, req.ItemIndex, req.Item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Menu item updated successfully"})
}

// DeleteMenuItem removes an item from a menu category
func DeleteMenuItem(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	categoryIndex, err := strconv.Atoi(c.Param("category_index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category index"})
		return
	}

	itemIndex, err := strconv.Atoi(c.Param("item_index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item index"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	if err := database.DeleteMenuItem(uint(businessID), categoryIndex, itemIndex); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Menu item deleted successfully"})
}

// Bill management handlers

// CreateBillRequest represents the request to create a new bill
type CreateBillRequest struct {
	TableID uint                 `json:"table_id" binding:"required"`
	Items   []database.BillItem  `json:"items"`
}

// UpdateBillRequest represents the request to update a bill
type UpdateBillRequest struct {
	Items []database.BillItem `json:"items"`
}

// AddBillItemRequest represents the request to add an item to a bill
type AddBillItemRequest struct {
	MenuItemID string                     `json:"menu_item_id" binding:"required"`
	Name       string                     `json:"name" binding:"required"`
	Price      float64                    `json:"price" binding:"required"`
	Quantity   int                        `json:"quantity" binding:"required"`
	Options    []database.MenuItemOption  `json:"options"`
}

// CreateBill creates a new bill for a business
func CreateBill(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	var req CreateBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this business"})
		return
	}

	// Verify table belongs to business
	table, err := database.GetTableByID(req.TableID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}

	if table.BusinessID != uint(businessID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table does not belong to this business"})
		return
	}

	// Check if table already has an open bill
	existingBill, _, err := database.GetOpenBillByTableID(req.TableID)
	if err == nil && existingBill != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Table already has an open bill"})
		return
	}

	// Calculate totals
	subtotal := 0.0
	for i := range req.Items {
		req.Items[i].Subtotal = req.Items[i].Price * float64(req.Items[i].Quantity)
		subtotal += req.Items[i].Subtotal
	}

	taxAmount := subtotal * business.TaxRate / 100
	serviceFeeAmount := subtotal * business.ServiceFeeRate / 100
	totalAmount := subtotal + taxAmount + serviceFeeAmount

	// Generate bill number
	billNumber := fmt.Sprintf("B%d-%d", businessID, time.Now().Unix())

	// Create bill
	bill := &database.Bill{
		BusinessID:       uint(businessID),
		TableID:          req.TableID,
		BillNumber:       billNumber,
		Subtotal:         subtotal,
		TaxAmount:        taxAmount,
		ServiceFeeAmount: serviceFeeAmount,
		TotalAmount:      totalAmount,
		Status:           database.BillStatusOpen,
		SettlementAddr:   business.SettlementAddr,
		TippingAddr:      business.TippingAddr,
	}

	if err := database.CreateBill(bill, req.Items); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}


	c.JSON(http.StatusCreated, gin.H{
		"bill":  bill,
		"items": req.Items,
	})
}

// GetBusinessBills retrieves all bills for a business
func GetBusinessBills(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this business"})
		return
	}

	bills, err := database.GetOpenBillsByBusinessID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"bills": bills})
}

// GetBill retrieves a specific bill by ID
func GetBill(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	bill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this bill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": items,
	})
}

// UpdateBill updates an existing bill
func UpdateBill(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req UpdateBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bill, _, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	if bill.Status != database.BillStatusOpen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify closed bill"})
		return
	}

	// Recalculate totals
	subtotal := 0.0
	for i := range req.Items {
		req.Items[i].Subtotal = req.Items[i].Price * float64(req.Items[i].Quantity)
		subtotal += req.Items[i].Subtotal
	}

	taxAmount := subtotal * business.TaxRate / 100
	serviceFeeAmount := subtotal * business.ServiceFeeRate / 100
	totalAmount := subtotal + taxAmount + serviceFeeAmount

	bill.Subtotal = subtotal
	bill.TaxAmount = taxAmount
	bill.ServiceFeeAmount = serviceFeeAmount
	bill.TotalAmount = totalAmount

	if err := database.UpdateBill(bill, req.Items); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": req.Items,
	})
}

// AddBillItem adds an item to an existing bill
func AddBillItem(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req AddBillItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	if bill.Status != database.BillStatusOpen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify closed bill"})
		return
	}

	// Create new bill item
	newItem := database.BillItem{
		ID:         fmt.Sprintf("item_%d", time.Now().UnixNano()),
		MenuItemID: req.MenuItemID,
		Name:       req.Name,
		Price:      req.Price,
		Quantity:   req.Quantity,
		Options:    req.Options,
		Subtotal:   req.Price * float64(req.Quantity),
	}

	// Add to existing items
	items = append(items, newItem)

	// Recalculate totals
	subtotal := 0.0
	for _, item := range items {
		subtotal += item.Subtotal
	}

	taxAmount := subtotal * business.TaxRate / 100
	serviceFeeAmount := subtotal * business.ServiceFeeRate / 100
	totalAmount := subtotal + taxAmount + serviceFeeAmount

	bill.Subtotal = subtotal
	bill.TaxAmount = taxAmount
	bill.ServiceFeeAmount = serviceFeeAmount
	bill.TotalAmount = totalAmount

	if err := database.UpdateBill(bill, items); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": items,
	})
}

// RemoveBillItem removes an item from a bill
func RemoveBillItem(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	itemID := c.Param("item_id")
	if itemID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Item ID is required"})
		return
	}

	bill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	if bill.Status != database.BillStatusOpen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify closed bill"})
		return
	}

	// Remove item from items slice
	var updatedItems []database.BillItem
	itemFound := false
	for _, item := range items {
		if item.ID != itemID {
			updatedItems = append(updatedItems, item)
		} else {
			itemFound = true
		}
	}

	if !itemFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item not found in bill"})
		return
	}

	// Recalculate totals
	subtotal := 0.0
	for _, item := range updatedItems {
		subtotal += item.Subtotal
	}

	taxAmount := subtotal * business.TaxRate / 100
	serviceFeeAmount := subtotal * business.ServiceFeeRate / 100
	totalAmount := subtotal + taxAmount + serviceFeeAmount

	bill.Subtotal = subtotal
	bill.TaxAmount = taxAmount
	bill.ServiceFeeAmount = serviceFeeAmount
	bill.TotalAmount = totalAmount

	if err := database.UpdateBill(bill, updatedItems); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": updatedItems,
	})
}

// CloseBill closes a bill
func CloseBill(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	bill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	if bill.Status != database.BillStatusOpen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bill is already closed"})
		return
	}

	if err := database.CloseBill(uint(billID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get updated bill
	updatedBill, _, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve updated bill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  updatedBill,
		"items": items,
	})
}

// validateCustomURL checks if a custom URL is available for use
func validateCustomURL(customURL string, excludeBusinessID uint) error {
	if customURL == "" {
		return nil
	}

	// Basic validation - only allow alphanumeric characters and hyphens
	if matched, _ := regexp.MatchString("^[a-zA-Z0-9-]+$", customURL); !matched {
		return errors.New("custom URL can only contain letters, numbers, and hyphens")
	}

	// Check minimum length
	if len(customURL) < 2 {
		return errors.New("custom URL must be at least 2 characters long")
	}

	// Check if URL is already taken by another business
	var existingBusiness database.Business
	result := database.GetDB().Where("custom_url = ? AND id != ?", customURL, excludeBusinessID).First(&existingBusiness)
	if result.Error == nil {
		return errors.New("this custom URL is already taken")
	}

	return nil
}

// CheckCustomURLAvailability checks if a custom URL is available
func CheckCustomURLAvailability(c *gin.Context) {
	customURL := c.Query("url")
	businessIDStr := c.Query("exclude_business_id")
	
	if customURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL parameter is required"})
		return
	}

	var excludeBusinessID uint = 0
	if businessIDStr != "" {
		if id, err := strconv.ParseUint(businessIDStr, 10, 32); err == nil {
			excludeBusinessID = uint(id)
		}
	}

	// Validate the URL format and availability
	if err := validateCustomURL(customURL, excludeBusinessID); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"available": true,
		"url": customURL,
	})
}
