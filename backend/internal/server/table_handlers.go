package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
)

// checkTableBusinessOwnership verifies the caller can access the business that
// owns the table. This includes owners and same-business staff members and does
// not grant access from a matching contact email alone.
func checkTableBusinessOwnership(c *gin.Context, business *database.Business) bool {
	return CheckBusinessAccess(c, business)
}

func getTableRouteBusiness(c *gin.Context) (*database.Business, bool) {
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return nil, false
	}
	return business, true
}

// Table creation request
type CreateTableRequest struct {
	Name     string `json:"name" binding:"required,max=64"`
	Capacity int    `json:"capacity"`
}

// Table update request
type UpdateTableRequest struct {
	Name               string `json:"name" binding:"omitempty,max=64"`
	Capacity           *int   `json:"capacity"`
	IsActive           *bool  `json:"is_active"`
	QRLogoURL          string `json:"qr_logo_url"`
	QRForegroundColor  string `json:"qr_foreground_color"`
	QRBackgroundColor  string `json:"qr_background_color"`
	QRLogoSize         int    `json:"qr_logo_size"`
	QRShowBusinessName bool   `json:"qr_show_business_name"`
	QRShowTableName    bool   `json:"qr_show_table_name"`
	QRTextFont         string `json:"qr_text_font"`
}

// UpdateTableDetailsRequest represents the request to update table details
type UpdateTableDetailsRequest struct {
	Name               *string `json:"name" binding:"omitempty,max=64"`
	Capacity           *int    `json:"capacity"`
	IsActive           *bool   `json:"is_active"`
	QRLogoURL          *string `json:"qr_logo_url"`
	QRForegroundColor  *string `json:"qr_foreground_color"`
	QRBackgroundColor  *string `json:"qr_background_color"`
	QRLogoSize         *int    `json:"qr_logo_size"`
	QRShowBusinessName *bool   `json:"qr_show_business_name"`
	QRShowTableName    *bool   `json:"qr_show_table_name"`
	QRTextFont         *string `json:"qr_text_font"`
}

// GetTablesWithStatus retrieves all tables with their current status (occupied, available, reserved)
func GetTablesWithStatus(c *gin.Context) {
	// Check authentication - accept both Web3 users (address) and OAuth users (user_id)
	_, hasAddress := c.Get("address")
	_, hasUserID := c.Get("user_id")

	if !hasAddress && !hasUserID {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Check if business exists and user owns it
	business, ok := getTableRouteBusiness(c)
	if !ok {
		return
	}

	// Use helper function to check ownership (supports both Web3 and OAuth)
	if !checkTableBusinessOwnership(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't own this business"})
		return
	}

	// Optional include_inactive (BE-first): absent/"false" keeps the legacy
	// active-only board; "true" surfaces soft-deleted tables so the operator
	// can reach and reactivate them (deactivation was a permanent trapdoor).
	includeInactive := false
	if raw := c.Query("include_inactive"); raw != "" {
		if parsed, perr := strconv.ParseBool(raw); perr == nil {
			includeInactive = parsed
		}
	}

	tablesWithStatus, err := database.GetTablesWithStatus(business.ID, includeInactive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve table status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tables": tablesWithStatus})
}

// GetTable retrieves a specific table by ID
func GetTable(c *gin.Context) {
	business, ok := getTableRouteBusiness(c)
	if !ok {
		return
	}

	tableIDStr := c.Param("tableId")
	tableID, err := strconv.ParseUint(tableIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid table ID"})
		return
	}

	// Access control is handled by HybridAuthenticationMiddleware + RoleBasedAccessMiddleware
	// Business access validation and permission checking is done in middleware

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

	if table.BusinessID != business.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Table does not belong to this business"})
		return
	}

	c.JSON(http.StatusOK, table)
}

// UpdateTable updates an existing table
func UpdateTable(c *gin.Context) {
	tableIDStr := c.Param("tableId")
	tableID, err := strconv.ParseUint(tableIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid table ID"})
		return
	}

	// Check if business exists and user owns it
	business, ok := getTableRouteBusiness(c)
	if !ok {
		return
	}

	// Use helper function to check ownership (supports both Web3 and OAuth)
	if !checkTableBusinessOwnership(c, business) {
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

	// Verify table belongs to the business
	if table.BusinessID != business.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Table does not belong to this business"})
		return
	}

	var req UpdateTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if config.DemoModeEnabled() && !demoQRLogoAllowed(req.QRLogoURL, business, table.QRLogoURL) {
		demomode.Refuse(c, demomode.KindStorefront, demoStorefrontIdentityAction)
		return
	}

	// Update table fields
	if req.Name != "" {
		table.Name = req.Name
	}
	if req.Capacity != nil && *req.Capacity > 0 {
		table.Capacity = *req.Capacity
	}
	// Only toggle activation when the client explicitly sent is_active. A
	// partial body must not silently deactivate the table (R3-OK-1, mirrored
	// from UpdateTableDetails).
	if req.IsActive != nil {
		table.IsActive = *req.IsActive
	}

	// Update QR customization fields - always update these when provided in request
	// Allow empty logo URL (user might want to remove logo)
	table.QRLogoURL = req.QRLogoURL

	// Always update colors if provided
	if req.QRForegroundColor != "" {
		table.QRForegroundColor = req.QRForegroundColor
	}
	if req.QRBackgroundColor != "" {
		table.QRBackgroundColor = req.QRBackgroundColor
	}

	// Update logo size with validation
	if req.QRLogoSize >= 10 && req.QRLogoSize <= 30 {
		table.QRLogoSize = req.QRLogoSize
	} else if req.QRLogoSize > 0 {
		// If size is provided but invalid, use default
		table.QRLogoSize = 20
	}

	// Update QR text display options
	table.QRShowBusinessName = req.QRShowBusinessName
	table.QRShowTableName = req.QRShowTableName

	// Update QR text font if provided
	if req.QRTextFont != "" {
		table.QRTextFont = req.QRTextFont
	}

	table.UpdatedAt = time.Now()

	if err := database.UpdateTable(table); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update table"})
		return
	}
	services.InvalidatePublicGuestBusiness(table.BusinessID)
	invalidateReservationAvailability(table.BusinessID)

	c.JSON(http.StatusOK, table)
}

// Phase 2: Enhanced Table Management API Endpoints

// CreateTableWithQR creates a new table with automatic QR code generation
func CreateTableWithQR(c *gin.Context) {
	// Verify business ownership
	business, ok := getTableRouteBusiness(c)
	if !ok {
		return
	}

	// Use helper function to check ownership (supports both Web3 and OAuth)
	if !checkTableBusinessOwnership(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	var req CreateTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	// Generate unique table code
	tableCode, err := database.GenerateUniqueTableCode(business.ID, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate table code"})
		return
	}

	// Set default capacity if not provided
	capacity := req.Capacity
	if capacity == 0 {
		capacity = 4 // Default to 4 seats
	}

	table := &database.Table{
		BusinessID: business.ID,
		Name:       req.Name,
		TableCode:  tableCode,
		Capacity:   capacity,
		IsActive:   true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := database.CreateTable(table); err != nil {
		// FIND-060: never surface GORM/driver text to operators.
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not create table")
		return
	}

	// P2-20: a first table can complete required setup — stamp server-side,
	// off the request path.
	businessID := business.ID
	runStampOnboardingAsync(func() {
		if _, err := services.StampOnboardingCompletedIfReady(businessID); err != nil {
			log.Printf("onboarding stamp after table create failed for business %d: %v", businessID, err)
		}
	})

	// Return the full persisted table so the operator UI's optimistic list
	// insert has every field (capacity, QR overrides, timestamps, business_id)
	// and is byte-identical to a later /tables/status refetch. The previous
	// hand-rolled subset omitted capacity, so a freshly created row showed 0
	// seats until a manual refresh — and any other field the row/drawer reads
	// had the same latent staleness. Embedding the struct keeps this in sync
	// with GetTable automatically; qr_url is retained as a convenience field.
	invalidateReservationAvailability(business.ID)
	c.JSON(http.StatusCreated, struct {
		*database.Table
		QRURL string `json:"qr_url"`
	}{Table: table, QRURL: fmt.Sprintf("/t/%s", table.TableCode)})
}

// UpdateTableDetails updates table information
func UpdateTableDetails(c *gin.Context) {
	tableID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid table ID"})
		return
	}

	// Get table
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

	if !checkTableBusinessOwnership(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this table"})
		return
	}

	// Bind the pointer-based request so partial bodies (name-only,
	// capacity-only, is_active-only) apply ONLY the fields the client sent.
	// The previous non-pointer UpdateTableRequest defaulted is_active to
	// false on every partial update, silently DEACTIVATING the table and
	// breaking its guest QR (R3-OK-1).
	var req UpdateTableDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if config.DemoModeEnabled() && req.QRLogoURL != nil && !demoQRLogoAllowed(*req.QRLogoURL, business, table.QRLogoURL) {
		demomode.Refuse(c, demomode.KindStorefront, demoStorefrontIdentityAction)
		return
	}

	// Update only the fields explicitly present in the request.
	if req.Name != nil && *req.Name != "" {
		table.Name = *req.Name
	}
	if req.Capacity != nil && *req.Capacity > 0 {
		table.Capacity = *req.Capacity
	}
	if req.IsActive != nil {
		table.IsActive = *req.IsActive
	}
	table.UpdatedAt = time.Now()

	if err := database.UpdateTable(table); err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not update table")
		return
	}
	services.InvalidatePublicGuestBusiness(table.BusinessID)
	invalidateReservationAvailability(table.BusinessID)

	// Return the FULL persisted table (mirroring CreateTableWithQR) so the
	// operator UI's list swap doesn't wipe qr_*/business_id from local state
	// with a 7-field subset (R3-OK-5). qr_url is retained as a convenience.
	c.JSON(http.StatusOK, struct {
		*database.Table
		QRURL string `json:"qr_url"`
	}{Table: table, QRURL: fmt.Sprintf("/t/%s", table.TableCode)})
}

// DeleteTableSoft soft deletes a table
func DeleteTableSoft(c *gin.Context) {
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

	// Use helper function to check ownership (supports both Web3 and OAuth)
	if !checkTableBusinessOwnership(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to delete this table"})
		return
	}

	if err := database.DeleteTable(uint(tableID)); err != nil {
		if errors.Is(err, database.ErrTableNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not delete table")
		return
	}
	services.InvalidatePublicGuestBusiness(table.BusinessID)
	invalidateReservationAvailability(table.BusinessID)

	c.JSON(http.StatusOK, gin.H{"message": "Table deleted successfully"})
}

// GetBusinessTables gets all tables for a business with QR URLs
func GetBusinessTables(c *gin.Context) {
	business, ok := getTableRouteBusiness(c)
	if !ok {
		return
	}

	// Access control is handled by HybridAuthenticationMiddleware + RoleBasedAccessMiddleware
	// Business access validation and permission checking is done in middleware

	tables, err := database.GetTablesByBusinessID(business.ID)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not list tables")
		return
	}

	// Add QR URLs to response
	var response []gin.H
	for _, table := range tables {
		response = append(response, gin.H{
			"id":                    table.ID,
			"business_id":           table.BusinessID,
			"name":                  table.Name,
			"capacity":              table.Capacity,
			"table_code":            table.TableCode,
			"qr_code":               table.QRCode,
			"qr_url":                fmt.Sprintf("/t/%s", table.TableCode),
			"is_active":             table.IsActive,
			"qr_logo_url":           table.QRLogoURL,
			"qr_foreground_color":   table.QRForegroundColor,
			"qr_background_color":   table.QRBackgroundColor,
			"qr_logo_size":          table.QRLogoSize,
			"qr_show_business_name": table.QRShowBusinessName,
			"qr_show_table_name":    table.QRShowTableName,
			"qr_text_font":          table.QRTextFont,
			"created_at":            table.CreatedAt,
			"updated_at":            table.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"tables": response})
}

// ApplyQRBrandingRequest is the "Apply to All" payload — the same QR branding
// fields UpdateTableDetails accepts, applied across every table transactionally.
type ApplyQRBrandingRequest struct {
	QRLogoURL          string `json:"qr_logo_url"`
	QRForegroundColor  string `json:"qr_foreground_color"`
	QRBackgroundColor  string `json:"qr_background_color"`
	QRLogoSize         int    `json:"qr_logo_size"`
	QRShowBusinessName bool   `json:"qr_show_business_name"`
	QRShowTableName    bool   `json:"qr_show_table_name"`
	QRTextFont         string `json:"qr_text_font"`
}

// ApplyQRBrandingToAllTables applies one QR branding set to every table of a
// business plus the business defaults in a single transaction. Replaces the
// per-table PUT fan-out (201 concurrent, non-atomic requests at 200 tables).
func ApplyQRBrandingToAllTables(c *gin.Context) {
	business, ok := getTableRouteBusiness(c)
	if !ok {
		return
	}
	if !checkTableBusinessOwnership(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't own this business"})
		return
	}

	var req ApplyQRBrandingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if config.DemoModeEnabled() && !demoQRLogoAllowed(req.QRLogoURL, business, business.DefaultQRLogoURL) {
		demomode.Refuse(c, demomode.KindStorefront, demoStorefrontIdentityAction)
		return
	}

	affected, err := database.ApplyQRBrandingToAllTables(business.ID, database.QRBranding{
		LogoURL:          req.QRLogoURL,
		ForegroundColor:  req.QRForegroundColor,
		BackgroundColor:  req.QRBackgroundColor,
		LogoSize:         req.QRLogoSize,
		ShowBusinessName: req.QRShowBusinessName,
		ShowTableName:    req.QRShowTableName,
		TextFont:         req.QRTextFont,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to apply QR branding"})
		return
	}
	services.InvalidatePublicGuestBusiness(business.ID)

	c.JSON(http.StatusOK, gin.H{"updated": affected})
}
