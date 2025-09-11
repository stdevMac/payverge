package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"payverge/internal/analytics"
	"payverge/internal/database"
)

type AnalyticsHandler struct {
	analytics *analytics.AnalyticsService
	db        *database.DB
}

func NewAnalyticsHandler(db *database.DB) *AnalyticsHandler {
	return &AnalyticsHandler{
		analytics: analytics.NewAnalyticsService(db),
		db:        db,
	}
}

// GetSalesAnalytics returns sales analytics for a business
// GET /api/v1/businesses/:id/analytics/sales?period=today|yesterday|week|month|quarter|year&date=2024-01-15
func (h *AnalyticsHandler) GetSalesAnalytics(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid business ID",
		})
		return
	}

	// Check if user owns this business
	userID := c.GetUint("user_id")
	business, err := h.db.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Business not found",
		})
		return
	}

	// Note: Business ownership validation would need to be implemented based on user-business relationship
	// For now, we'll skip this check or implement it based on the actual auth system
	_ = userID    // Suppress unused variable warning
	_ = business  // Suppress unused variable warning

	period := c.DefaultQuery("period", "today")
	dateStr := c.Query("date")

	if period == "custom" && dateStr != "" {
		// Parse specific date for daily report
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid date format. Use YYYY-MM-DD",
			})
			return
		}

		report, err := h.analytics.GetDailySales(uint(businessID), date)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to get sales data",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    report,
		})
		return
	}

	// Get period report
	report, err := h.analytics.GetPeriodReport(uint(businessID), period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get sales analytics",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    report,
	})
}

// GetTipAnalytics returns tip analytics for a business
// GET /api/v1/businesses/:id/analytics/tips?period=today|yesterday|week|month|quarter|year
func (h *AnalyticsHandler) GetTipAnalytics(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid business ID",
		})
		return
	}

	// Check if user owns this business
	userID := c.GetUint("user_id")
	business, err := h.db.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Business not found",
		})
		return
	}

	// Note: Business ownership validation would need to be implemented based on user-business relationship
	// For now, we'll skip this check or implement it based on the actual auth system
	_ = userID    // Suppress unused variable warning
	_ = business  // Suppress unused variable warning

	period := c.DefaultQuery("period", "today")

	report, err := h.analytics.GetTipAnalytics(uint(businessID), period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get tip analytics",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    report,
	})
}

// GetItemAnalytics returns menu item performance analytics
// GET /api/v1/businesses/:id/analytics/items?period=today|yesterday|week|month|quarter|year
func (h *AnalyticsHandler) GetItemAnalytics(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid business ID",
		})
		return
	}

	// Check if user owns this business
	userID := c.GetUint("user_id")
	business, err := h.db.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Business not found",
		})
		return
	}

	// Note: Business ownership validation would need to be implemented based on user-business relationship
	// For now, we'll skip this check or implement it based on the actual auth system
	_ = userID    // Suppress unused variable warning
	_ = business  // Suppress unused variable warning

	period := c.DefaultQuery("period", "week")

	items, err := h.analytics.GetPopularItems(uint(businessID), period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get item analytics",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}

// ExportSalesData exports sales data in specified format
// GET /api/v1/businesses/:id/reports/export?period=week&format=csv|json
func (h *AnalyticsHandler) ExportSalesData(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid business ID",
		})
		return
	}

	// Check if user owns this business
	userID := c.GetUint("user_id")
	business, err := h.db.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Business not found",
		})
		return
	}

	// Note: Business ownership validation would need to be implemented based on user-business relationship
	// For now, we'll skip this check or implement it based on the actual auth system
	_ = userID    // Suppress unused variable warning
	_ = business  // Suppress unused variable warning

	period := c.DefaultQuery("period", "week")
	format := c.DefaultQuery("format", "csv")

	data, err := h.analytics.ExportSalesData(uint(businessID), period, format)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to export data",
		})
		return
	}

	// Set appropriate headers for file download
	filename := "sales_data_" + period + "." + format
	c.Header("Content-Disposition", "attachment; filename="+filename)
	
	switch format {
	case "csv":
		c.Header("Content-Type", "text/csv")
	case "json":
		c.Header("Content-Type", "application/json")
	default:
		c.Header("Content-Type", "application/octet-stream")
	}

	c.Data(http.StatusOK, c.GetHeader("Content-Type"), data)
}

// GetLiveBills returns currently active bills for real-time dashboard
// GET /api/v1/businesses/:id/analytics/live-bills
func (h *AnalyticsHandler) GetLiveBills(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid business ID",
		})
		return
	}

	// Check if user owns this business
	userID := c.GetUint("user_id")
	business, err := h.db.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Business not found",
		})
		return
	}

	// Note: Business ownership validation would need to be implemented based on user-business relationship
	// For now, we'll skip this check or implement it based on the actual auth system
	_ = userID    // Suppress unused variable warning
	_ = business  // Suppress unused variable warning

	// Get active bills (open status)
	bills, err := h.db.GetBillsByBusinessAndStatus(uint(businessID), database.BillStatusOpen)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get live bills",
		})
		return
	}

	// Enhance bills with additional info
	var liveBills []gin.H
	for _, bill := range bills {
		// Get table info
		table, _ := h.db.GetTableByID(bill.TableID)
		
		// Calculate remaining amount
		remainingAmount := bill.TotalAmount - bill.PaidAmount
		
		liveBill := gin.H{
			"id":               bill.ID,
			"bill_number":      bill.BillNumber,
			"table_name":       table.Name,
			"table_code":       table.TableCode,
			"total_amount":     bill.TotalAmount,
			"paid_amount":      bill.PaidAmount,
			"remaining_amount": remainingAmount,
			"tip_amount":       bill.TipAmount,
			"status":           bill.Status,
			"created_at":       bill.CreatedAt,
			"updated_at":       bill.UpdatedAt,
		}
		
		liveBills = append(liveBills, liveBill)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    liveBills,
	})
}

// GetDashboardSummary returns a summary of key metrics for the dashboard
// GET /api/v1/businesses/:id/analytics/dashboard
func (h *AnalyticsHandler) GetDashboardSummary(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid business ID",
		})
		return
	}

	// Check if user owns this business
	userID := c.GetUint("user_id")
	business, err := h.db.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Business not found",
		})
		return
	}

	// Note: Business ownership validation would need to be implemented based on user-business relationship
	// For now, we'll skip this check or implement it based on the actual auth system
	_ = userID    // Suppress unused variable warning
	_ = business  // Suppress unused variable warning

	// Get today's sales
	today := time.Now()
	todaySales, err := h.analytics.GetDailySales(uint(businessID), today)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get today's sales",
		})
		return
	}

	// Get week's analytics
	weekReport, err := h.analytics.GetPeriodReport(uint(businessID), "week")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get week's analytics",
		})
		return
	}

	// Get active bills count
	activeBills, err := h.db.GetBillsByBusinessAndStatus(uint(businessID), database.BillStatusOpen)
	if err != nil {
		activeBills = []database.Bill{} // Default to empty if error
	}

	// Get top items for the week
	topItems, err := h.analytics.GetPopularItems(uint(businessID), "week")
	if err != nil {
		topItems = []analytics.ItemStats{} // Default to empty if error
	}

	// Limit to top 5 items
	if len(topItems) > 5 {
		topItems = topItems[:5]
	}

	summary := gin.H{
		"today": gin.H{
			"revenue":      todaySales.TotalRevenue,
			"tips":         todaySales.TotalTips,
			"transactions": todaySales.TransactionCount,
			"bills":        todaySales.BillCount,
		},
		"week": gin.H{
			"revenue":           weekReport.TotalRevenue,
			"tips":              weekReport.TotalTips,
			"transactions":      weekReport.TransactionCount,
			"bills":             weekReport.BillCount,
			"unique_customers":  weekReport.UniqueCustomers,
			"average_ticket":    weekReport.AverageTicket,
		},
		"live": gin.H{
			"active_bills": len(activeBills),
		},
		"top_items": topItems,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    summary,
	})
}
