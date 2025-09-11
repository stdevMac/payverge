package server

import (
	"net/http"

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
