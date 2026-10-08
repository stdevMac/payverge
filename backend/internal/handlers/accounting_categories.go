package handlers

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
)

var categoryKeyPattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// ListAccountingCategories GET /accounting/categories — defaults + business extras.
func (h *AccountingHandler) ListAccountingCategories(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	entryType := strings.ToLower(strings.TrimSpace(c.Query("entry_type")))
	// Manage-panel restore needs inactive rows; entry pickers omit them (default).
	includeInactive := strings.EqualFold(strings.TrimSpace(c.Query("include_inactive")), "true") ||
		strings.TrimSpace(c.Query("include_inactive")) == "1"
	var custom []database.AccountingCategory
	q := h.db.GetGorm().Where("business_id = ?", businessID)
	if !includeInactive {
		q = q.Where("active = ?", true)
	}
	if entryType == "income" || entryType == "expense" {
		q = q.Where("entry_type = ?", entryType)
	}
	_ = q.Order("position ASC, id ASC").Find(&custom).Error

	// Hardcoded defaults (not stored) — FE also has categories.ts; API is source for merge.
	defaults := []gin.H{}
	if entryType == "" || entryType == "income" {
		for _, k := range []string{"off_platform_sale", "catering", "event", "service", "adjustment", "other"} {
			defaults = append(defaults, gin.H{"key": k, "label": k, "entry_type": "income", "source": "default"})
		}
	}
	if entryType == "" || entryType == "expense" {
		for _, k := range []string{"rent", "utilities", "supplies", "inventory", "marketing", "software", "maintenance", "logistics", "tax", "other"} {
			defaults = append(defaults, gin.H{"key": k, "label": k, "entry_type": "expense", "source": "default"})
		}
	}
	extras := make([]gin.H, 0, len(custom))
	for _, cat := range custom {
		extras = append(extras, gin.H{
			"id": cat.ID, "key": cat.Key, "label": cat.Label,
			"entry_type": cat.EntryType, "source": "custom", "active": cat.Active, "position": cat.Position,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"defaults": defaults, "custom": extras}})
}

type categoryRequest struct {
	Key       string `json:"key" binding:"required"`
	Label     string `json:"label" binding:"required"`
	EntryType string `json:"entry_type" binding:"required"`
	Active    *bool  `json:"active"`
	Position  *int   `json:"position"`
}

// CreateAccountingCategory POST /accounting/categories
func (h *AccountingHandler) CreateAccountingCategory(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	var req categoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	key := strings.ToLower(strings.TrimSpace(req.Key))
	if !categoryKeyPattern.MatchString(key) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "key must be snake_case alphanumeric")
		return
	}
	et := database.AccountingEntryType(strings.ToLower(strings.TrimSpace(req.EntryType)))
	if et != database.AccountingEntryTypeIncome && et != database.AccountingEntryTypeExpense {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "entry_type must be income or expense")
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	pos := 0
	if req.Position != nil {
		pos = *req.Position
	}
	row := database.AccountingCategory{
		BusinessID: businessID,
		Key:        key,
		Label:      strings.TrimSpace(req.Label),
		EntryType:  et,
		Active:     active,
		Position:   pos,
	}
	if err := h.db.GetGorm().Create(&row).Error; err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Could not create category (duplicate key?)")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": row})
}

// UpdateAccountingCategory PATCH /accounting/categories/:categoryId
func (h *AccountingHandler) UpdateAccountingCategory(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	cid, err := strconv.ParseUint(c.Param("categoryId"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid category id")
		return
	}
	var row database.AccountingCategory
	if err := h.db.GetGorm().Where("id = ? AND business_id = ?", cid, businessID).First(&row).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "category not found")
		return
	}
	var req categoryRequest
	_ = c.ShouldBindJSON(&req)
	if strings.TrimSpace(req.Label) != "" {
		row.Label = strings.TrimSpace(req.Label)
	}
	if req.Active != nil {
		row.Active = *req.Active
	}
	if req.Position != nil {
		row.Position = *req.Position
	}
	if err := h.db.GetGorm().Save(&row).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to update category")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}
