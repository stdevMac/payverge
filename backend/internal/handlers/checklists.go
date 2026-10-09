package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

type ChecklistHandler struct{ db *database.DB }

func NewChecklistHandler(db *database.DB) *ChecklistHandler { return &ChecklistHandler{db: db} }

// staffIDFromContext reads the staff identity the auth middleware stamped. Used
// to row-scope "own run". Returns 0,false for non-staff tokens.
func staffIDFromContext(c *gin.Context) (uint, bool) {
	raw, ok := c.Get("staff_id")
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case uint:
		return v, v > 0
	case uint64:
		return uint(v), v > 0
	case int:
		return uint(v), v > 0
	case int64:
		return uint(v), v > 0
	}
	return 0, false
}

// callerIsManager reports whether the caller holds checklist:manage by role
// (manager) or is an owner/web3 token (no staff_role set). Owners bypass
// ownership scoping the same way RBAC middleware lets them bypass perms.
func callerIsManager(c *gin.Context) bool {
	role, ok := c.Get("staff_role")
	if !ok {
		return true // owner / non-staff token
	}
	return role == string(database.StaffRoleManager)
}

type checklistItemDTO struct {
	Label      string `json:"label"`
	SortOrder  int    `json:"sort_order"`
	IsRequired bool   `json:"is_required"`
}
type checklistTemplateDTO struct {
	Name       string             `json:"name"`
	Kind       string             `json:"kind"`
	PositionID *uint              `json:"position_id"`
	Items      []checklistItemDTO `json:"items"`
}

var validChecklistKind = map[string]bool{
	database.ChecklistKindOnboarding: true, database.ChecklistKindOpening: true,
	database.ChecklistKindClosing: true, database.ChecklistKindCustom: true,
}

func (h *ChecklistHandler) ListTemplates(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	list, err := h.db.ListChecklistTemplates(businessID, 200)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list templates")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

func (h *ChecklistHandler) CreateTemplate(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	var in checklistTemplateDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 255 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "name is required")
		return
	}
	if in.Kind == "" {
		in.Kind = database.ChecklistKindCustom
	}
	if !validChecklistKind[in.Kind] {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid kind")
		return
	}
	staffID, _ := staffIDFromContext(c)
	tpl := &database.ChecklistTemplate{
		BusinessID: businessID, Name: in.Name, Kind: in.Kind, PositionID: in.PositionID,
		IsActive: true, CreatedByStaffID: staffID,
	}
	items := make([]database.ChecklistItem, 0, len(in.Items))
	for _, it := range in.Items {
		label := strings.TrimSpace(it.Label)
		if label == "" || len(label) > 500 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "item label is required")
			return
		}
		items = append(items, database.ChecklistItem{Label: label, SortOrder: it.SortOrder, IsRequired: it.IsRequired})
	}
	if err := h.db.CreateChecklistTemplate(tpl, items); err != nil {
		if errors.Is(err, database.ErrPositionNotFound) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "position not in business")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create template")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": tpl})
}

type checklistRunDTO struct {
	TemplateID      uint   `json:"template_id"`
	AssignedStaffID *uint  `json:"assigned_staff_id"`
	ShiftID         *uint  `json:"shift_id"`
	ForDate         string `json:"for_date"` // RFC3339 date; defaults to now
}

func (h *ChecklistHandler) CreateRun(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	var in checklistRunDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if in.TemplateID == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "template_id is required")
		return
	}
	forDate := time.Now().UTC()
	if in.ForDate != "" {
		if parsed, err := time.Parse(time.RFC3339, in.ForDate); err == nil {
			forDate = parsed
		}
	}
	run, err := h.db.InstantiateChecklistRun(businessID, in.TemplateID, in.AssignedStaffID, in.ShiftID, forDate)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrChecklistTemplateNotFound):
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Template not found")
		case errors.Is(err, database.ErrInvalidChecklistAssignment):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "assignment target not in business")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create run")
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": run})
}

// ListRuns returns checklist runs. Default (BE-first, unchanged for the staff
// dashboard): the caller's OWN assigned runs. With ?scope=business — allowed only
// for a manager/owner (checklist:manage by role) — it returns the BUSINESS-WIDE
// run-status list (template + assignee names joined) so operators can see who has
// and hasn't finished an assigned checklist. A non-manager requesting
// scope=business is quietly served their own runs (no privilege escalation).
func (h *ChecklistHandler) ListRuns(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	if strings.TrimSpace(c.Query("scope")) == "business" && callerIsManager(c) {
		list, err := h.db.ListChecklistRunsForBusiness(businessID, 200)
		if err != nil {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list runs")
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff context required")
		return
	}
	list, err := h.db.ListChecklistRunsForStaff(businessID, staffID, 100)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list runs")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// RunDetail returns a run's items + per-run completion state so staff can tick
// items. Non-managers may only read a run assigned to them (own-run scope);
// managers/owners read any run. Gated by checklist:complete at the route layer.
func (h *ChecklistHandler) RunDetail(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	runID, ok := parseParamUint(c, "runId")
	if !ok {
		return
	}
	if !callerIsManager(c) {
		staffID, ok := staffIDFromContext(c)
		if !ok {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff context required")
			return
		}
		owned, err := h.db.RunOwnedByStaff(businessID, runID, staffID)
		if err != nil {
			if errors.Is(err, database.ErrChecklistRunNotFound) {
				server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Run not found")
				return
			}
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load run")
			return
		}
		if !owned {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Not your checklist")
			return
		}
	}
	detail, err := h.db.GetChecklistRunDetail(businessID, runID)
	if err != nil {
		if errors.Is(err, database.ErrChecklistRunNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Run not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load run detail")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": detail})
}

type tickDTO struct {
	Done bool   `json:"done"`
	Note string `json:"note"`
}

func (h *ChecklistHandler) TickItem(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	runID, ok := parseParamUint(c, "runId")
	if !ok {
		return
	}
	itemID, ok := parseParamUint(c, "itemId")
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff context required")
		return
	}
	var in tickDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	// Own-run scope: non-managers may only tick a run assigned to them.
	if !callerIsManager(c) {
		owned, err := h.db.RunOwnedByStaff(businessID, runID, staffID)
		if err != nil {
			if errors.Is(err, database.ErrChecklistRunNotFound) {
				server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Run not found")
				return
			}
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load run")
			return
		}
		if !owned {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Not your checklist")
			return
		}
	}
	run, err := h.db.TickChecklistItem(businessID, runID, itemID, staffID, in.Done, strings.TrimSpace(in.Note))
	if err != nil {
		switch {
		case errors.Is(err, database.ErrChecklistRunNotFound):
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Run not found")
		case errors.Is(err, database.ErrChecklistItemNotFound):
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Item not on this run")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to update item")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": run})
}
