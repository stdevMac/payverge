package handlers

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
)

type recurringTemplateRequest struct {
	EntryType   string  `json:"entry_type" binding:"required"`
	Category    string  `json:"category" binding:"required"`
	Amount      float64 `json:"amount" binding:"required,gt=0,lte=100000000"` // dollars on wire; capped so cents fit int64 with room
	Currency    string  `json:"currency"`
	Description string  `json:"description" binding:"required"`
	Notes       string  `json:"notes"`
	Reference   string  `json:"reference"`
	Cadence     string  `json:"cadence" binding:"required"` // monthly|weekly
	AnchorDay   int     `json:"anchor_day" binding:"required"`
	NextRunOn   string  `json:"next_run_on" binding:"required"` // YYYY-MM-DD
	Active      *bool   `json:"active"`
}

func templateToJSON(t database.RecurringEntryTemplate) gin.H {
	return gin.H{
		"id":                t.ID,
		"business_id":       t.BusinessID,
		"entry_type":        t.EntryType,
		"category":          t.Category,
		"amount":            float64(t.AmountCents) / 100.0,
		"amount_cents":      t.AmountCents,
		"currency":          t.Currency,
		"description":       t.Description,
		"notes":             t.Notes,
		"reference":         t.Reference,
		"cadence":           t.Cadence,
		"anchor_day":        t.AnchorDay,
		"next_run_on":       t.NextRunOn.Format("2006-01-02"),
		"active":            t.Active,
		"needs_attention":   t.NeedsAttention,
		"last_generated_at": t.LastGeneratedAt,
		"created_at":        t.CreatedAt,
		"updated_at":        t.UpdatedAt,
	}
}

// ListRecurringTemplates GET /accounting/recurring-templates
func (h *AccountingHandler) ListRecurringTemplates(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	var rows []database.RecurringEntryTemplate
	if err := h.db.GetGorm().Where("business_id = ?", businessID).
		Order("active DESC, next_run_on ASC, id ASC").
		Find(&rows).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list recurring templates")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, templateToJSON(r))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// CreateRecurringTemplate POST /accounting/recurring-templates
func (h *AccountingHandler) CreateRecurringTemplate(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	var req recurringTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	tmpl, err := buildTemplateFromRequest(businessID, business, req)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	if uid, ok := c.Get("user_id"); ok {
		if id, ok := uid.(uint); ok {
			tmpl.CreatedByUserID = &id
		}
	}
	if err := h.db.GetGorm().Create(tmpl).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create template")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": templateToJSON(*tmpl)})
}

// UpdateRecurringTemplate PATCH /accounting/recurring-templates/:templateId
func (h *AccountingHandler) UpdateRecurringTemplate(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	tid, err := strconv.ParseUint(c.Param("templateId"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid template id")
		return
	}
	var existing database.RecurringEntryTemplate
	if err := h.db.GetGorm().Where("id = ? AND business_id = ?", tid, businessID).First(&existing).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "template not found")
		return
	}
	var req recurringTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	built, err := buildTemplateFromRequest(businessID, business, req)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	existing.EntryType = built.EntryType
	existing.Category = built.Category
	existing.AmountCents = built.AmountCents
	existing.Currency = built.Currency
	existing.Description = built.Description
	existing.Notes = built.Notes
	existing.Reference = built.Reference
	existing.Cadence = built.Cadence
	existing.AnchorDay = built.AnchorDay
	existing.NextRunOn = built.NextRunOn
	if req.Active != nil {
		existing.Active = *req.Active
	}
	if err := h.db.GetGorm().Save(&existing).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to update template")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": templateToJSON(existing)})
}

// DeleteRecurringTemplate DELETE /accounting/recurring-templates/:templateId
func (h *AccountingHandler) DeleteRecurringTemplate(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	tid, err := strconv.ParseUint(c.Param("templateId"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid template id")
		return
	}
	res := h.db.GetGorm().Where("id = ? AND business_id = ?", tid, businessID).Delete(&database.RecurringEntryTemplate{})
	if res.Error != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to delete template")
		return
	}
	if res.RowsAffected == 0 {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "template not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func buildTemplateFromRequest(businessID uint, business *database.Business, req recurringTemplateRequest) (*database.RecurringEntryTemplate, error) {
	et := database.AccountingEntryType(strings.ToLower(strings.TrimSpace(req.EntryType)))
	if et != database.AccountingEntryTypeIncome && et != database.AccountingEntryTypeExpense {
		return nil, errStr("entry_type must be income or expense")
	}
	cadence := strings.ToLower(strings.TrimSpace(req.Cadence))
	if cadence != "monthly" && cadence != "weekly" {
		return nil, errStr("cadence must be monthly or weekly")
	}
	if cadence == "monthly" && (req.AnchorDay < 1 || req.AnchorDay > 28) {
		return nil, errStr("anchor_day for monthly must be 1-28")
	}
	if cadence == "weekly" && (req.AnchorDay < 0 || req.AnchorDay > 6) {
		return nil, errStr("anchor_day for weekly must be 0-6 (weekday)")
	}
	day, err := time.Parse("2006-01-02", strings.TrimSpace(req.NextRunOn))
	if err != nil {
		return nil, errStr("next_run_on must be YYYY-MM-DD")
	}
	cur := strings.ToUpper(strings.TrimSpace(req.Currency))
	if cur == "" && business != nil {
		cur = strings.ToUpper(strings.TrimSpace(business.DefaultCurrency))
	}
	if cur == "" {
		cur = "USD"
	}
	cents := int64(math.Round(req.Amount * 100))
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	return &database.RecurringEntryTemplate{
		BusinessID:  businessID,
		EntryType:   et,
		Category:    strings.ToLower(strings.TrimSpace(req.Category)),
		AmountCents: cents,
		Currency:    cur,
		Description: strings.TrimSpace(req.Description),
		Notes:       strings.TrimSpace(req.Notes),
		Reference:   strings.TrimSpace(req.Reference),
		Cadence:     cadence,
		AnchorDay:   req.AnchorDay,
		NextRunOn:   day,
		Active:      active,
	}, nil
}

type strError string

func (e strError) Error() string { return string(e) }
func errStr(s string) error      { return strError(s) }
