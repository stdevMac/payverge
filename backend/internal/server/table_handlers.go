package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"payverge/internal/database"

	"github.com/gin-gonic/gin"
)

// Table creation request
type CreateTableRequest struct {
	Name string `json:"name" binding:"required"`
}

// Table update request
type UpdateTableRequest struct {
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}

// CreateTable creates a new table for a business
func CreateTable(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessIDStr := c.Param("businessId")
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

	var req CreateTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate unique table code
	tableCode, err := generateTableCode()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate table code"})
		return
	}

	// Generate QR code URL (this would point to the guest interface)
	qrCodeURL := fmt.Sprintf("https://payverge.io/t/%s", tableCode)

	table := &database.Table{
		BusinessID: uint(businessID),
		TableCode:  tableCode,
		Name:       req.Name,
		QRCode:     qrCodeURL,
		IsActive:   true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := database.CreateTable(table); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create table"})
		return
	}

	c.JSON(http.StatusCreated, table)
}

// GetTables retrieves all tables for a business
func GetTables(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessIDStr := c.Param("businessId")
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

	tables, err := database.GetTablesByBusinessID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve tables"})
		return
	}

	c.JSON(http.StatusOK, tables)
}

// GetTable retrieves a specific table by ID
func GetTable(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessIDStr := c.Param("businessId")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	tableIDStr := c.Param("tableId")
	tableID, err := strconv.ParseUint(tableIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid table ID"})
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

	// Get the table and verify it belongs to the business
	table, err := database.GetTableByID(uint(tableID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve table"})
		return
	}

	if table.BusinessID != uint(businessID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Table does not belong to this business"})
		return
	}

	c.JSON(http.StatusOK, table)
}

// UpdateTable updates an existing table
func UpdateTable(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessIDStr := c.Param("businessId")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	tableIDStr := c.Param("tableId")
	tableID, err := strconv.ParseUint(tableIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid table ID"})
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

	// Get the table and verify it belongs to the business
	table, err := database.GetTableByID(uint(tableID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve table"})
		return
	}

	if table.BusinessID != uint(businessID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Table does not belong to this business"})
		return
	}

	var req UpdateTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update table fields
	if req.Name != "" {
		table.Name = req.Name
	}
	table.IsActive = req.IsActive
	table.UpdatedAt = time.Now()

	if err := database.UpdateTable(table); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update table"})
		return
	}

	c.JSON(http.StatusOK, table)
}

// DeleteTable soft deletes a table
func DeleteTable(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessIDStr := c.Param("businessId")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	tableIDStr := c.Param("tableId")
	tableID, err := strconv.ParseUint(tableIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid table ID"})
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

	// Get the table and verify it belongs to the business
	table, err := database.GetTableByID(uint(tableID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve table"})
		return
	}

	if table.BusinessID != uint(businessID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Table does not belong to this business"})
		return
	}

	if err := database.DeleteTable(uint(tableID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete table"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Table deleted successfully"})
}

// GetTableByCode retrieves table information by table code (public endpoint for guests)
func GetTableByCode(c *gin.Context) {
	tableCode := c.Param("code")
	if tableCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, err := database.GetTableByCode(tableCode)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve table"})
		return
	}

	// Get business information
	business, err := database.GetBusinessByID(table.BusinessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business information"})
		return
	}

	// Check if there's an open bill for this table
	bill, _, err := database.GetOpenBillByTableID(table.ID)
	var billInfo interface{}
	if err != nil && !strings.Contains(err.Error(), "no open bill") {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check for open bill"})
		return
	}
	if bill != nil {
		billInfo = gin.H{
			"id":          bill.ID,
			"bill_number": bill.BillNumber,
			"total":       bill.TotalAmount,
			"paid":        bill.PaidAmount,
			"status":      bill.Status,
		}
	}

	response := gin.H{
		"table":    table,
		"business": business,
		"bill":     billInfo,
	}

	c.JSON(http.StatusOK, response)
}

// Helper function to generate a unique table code
func generateTableCode() (string, error) {
	bytes := make([]byte, 4) // 8 character hex string
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(bytes)), nil
}

// Phase 2: Enhanced Table Management API Endpoints

// CreateTableWithQR creates a new table with automatic QR code generation
func CreateTableWithQR(c *gin.Context) {
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

	var req CreateTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate unique table code
	tableCode, err := database.GenerateUniqueTableCode(uint(businessID), req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate table code"})
		return
	}

	table := &database.Table{
		BusinessID: uint(businessID),
		Name:       req.Name,
		TableCode:  tableCode,
		IsActive:   true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := database.CreateTable(table); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return table with QR code URL
	response := gin.H{
		"id":         table.ID,
		"name":       table.Name,
		"table_code": table.TableCode,
		"qr_url":     fmt.Sprintf("/t/%s", table.TableCode),
		"is_active":  table.IsActive,
		"created_at": table.CreatedAt,
	}

	c.JSON(http.StatusCreated, response)
}

// UpdateTableDetails updates table information
func UpdateTableDetails(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	tableID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid table ID"})
		return
	}

	// Get table and verify ownership
	table, err := database.GetTableByID(uint(tableID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}

	business, err := database.GetBusinessByID(table.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this table"})
		return
	}

	var req UpdateTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update table fields
	if req.Name != "" {
		table.Name = req.Name
	}
	table.IsActive = req.IsActive
	table.UpdatedAt = time.Now()

	if err := database.UpdateTable(table); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         table.ID,
		"name":       table.Name,
		"table_code": table.TableCode,
		"qr_url":     fmt.Sprintf("/t/%s", table.TableCode),
		"is_active":  table.IsActive,
		"updated_at": table.UpdatedAt,
	})
}

// DeleteTableSoft soft deletes a table
func DeleteTableSoft(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	tableID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid table ID"})
		return
	}

	// Get table and verify ownership
	table, err := database.GetTableByID(uint(tableID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}

	business, err := database.GetBusinessByID(table.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to delete this table"})
		return
	}

	if err := database.DeleteTable(uint(tableID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Table deleted successfully"})
}

// GetBusinessTables gets all tables for a business with QR URLs
func GetBusinessTables(c *gin.Context) {
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
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to view this business"})
		return
	}

	tables, err := database.GetTablesByBusinessID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Add QR URLs to response
	var response []gin.H
	for _, table := range tables {
		response = append(response, gin.H{
			"id":         table.ID,
			"name":       table.Name,
			"table_code": table.TableCode,
			"qr_url":     fmt.Sprintf("/t/%s", table.TableCode),
			"is_active":  table.IsActive,
			"created_at": table.CreatedAt,
			"updated_at": table.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"tables": response})
}
