package handlers

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

type PositionHandler struct{ db *database.DB }

func NewPositionHandler(db *database.DB) *PositionHandler { return &PositionHandler{db: db} }

const maxPayRate = 100_000 // $100k/hour ceiling, mirrors staff_compensation

type positionDTO struct {
	Name       string `json:"name"`
	ColorHex   string `json:"color_hex"`
	Department string `json:"department"`
	SortOrder  int    `json:"sort_order"`
}

type positionUpdateDTO struct {
	Name       *string `json:"name"`
	ColorHex   *string `json:"color_hex"`
	Department *string `json:"department"`
	SortOrder  *int    `json:"sort_order"`
}

// validColorHex accepts an empty string (color optional) or a #RGB / #RRGGBB /
// #RRGGBBAA hex string (fits the varchar(9) column and the UI chip swatch).
func validColorHex(s string) bool {
	if s == "" {
		return true
	}
	if s[0] != '#' || (len(s) != 4 && len(s) != 7 && len(s) != 9) {
		return false
	}
	for _, r := range s[1:] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// isDuplicatePositionName reports whether err is the unique-index rejection
// from idx_positions_business_name_active, across the drivers we run on
// (Postgres in production, SQLite in tests) plus GORM's translated sentinel.
//
// The app-layer guard reads the existing names and then inserts, so two
// simultaneous creates can both pass the read. The index is what makes the
// duplicate impossible; this mapping is what turns the loser of that race into
// the same 409 the guard returns instead of a 500 that reads like an outage.
func isDuplicatePositionName(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value") || // Postgres 23505
		strings.Contains(msg, "UNIQUE constraint failed") // SQLite
}

// activeNameTaken reports whether another ACTIVE position in this business
// already carries name, compared the way idx_positions_business_name_active
// compares it: trimmed and case-folded. excludeID lets a rename ignore the row
// it is renaming (pass 0 on create).
func (h *PositionHandler) activeNameTaken(businessID uint, name string, excludeID uint) (bool, error) {
	existing, err := h.db.ListPositions(businessID)
	if err != nil {
		return false, err
	}
	needle := strings.ToLower(strings.TrimSpace(name))
	for _, pos := range existing {
		if pos.ID == excludeID {
			continue
		}
		if strings.ToLower(strings.TrimSpace(pos.Name)) == needle {
			return true, nil
		}
	}
	return false, nil
}

type assignDTO struct {
	PositionID uint `json:"position_id"`
	IsPrimary  bool `json:"is_primary"`
}

type rateDTO struct {
	PayRate float64 `json:"pay_rate"` // dollars
}

// rateRequestDTO is the PUT-body shape for SetRate. PayRate is a pointer so an
// omitted field or an explicit null is distinguishable from an intentional 0 —
// a bare `{}` (or `{"pay_rate":null}`) body must 400 as a missing required
// field, not silently overwrite the persisted wage to $0. The GetRate/SetRate
// JSON *responses* keep using rateDTO (the float64 form), so this pointer shape
// is request-only.
type rateRequestDTO struct {
	PayRate *float64 `json:"pay_rate"` // pointer so omitted/null is distinguishable from an intentional 0
}

func parseBusinessID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business id")
		return 0, false
	}
	return uint(id), true
}

func parseParamUint(c *gin.Context, name string) (uint, bool) {
	v, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid "+name)
		return 0, false
	}
	return uint(v), true
}

func (h *PositionHandler) List(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	list, err := h.db.ListPositions(businessID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list positions")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

func (h *PositionHandler) Create(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	var in positionDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "name is required")
		return
	}
	if len(in.Name) > 255 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "name too long")
		return
	}
	if !validColorHex(in.ColorHex) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "color_hex must be a hex color like #1a6b6a")
		return
	}
	if len(in.Department) > 16 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "department too long")
		return
	}
	// L5-18: case-insensitive name uniqueness, mirroring
	// idx_positions_business_name_active. The read-then-write here is racy by
	// construction; the index below is the real guarantee.
	taken, checkErr := h.activeNameTaken(businessID, in.Name, 0)
	if checkErr != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create position")
		return
	}
	if taken {
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "position name already exists")
		return
	}
	p := &database.Position{
		BusinessID: businessID, Name: in.Name, ColorHex: in.ColorHex,
		Department: in.Department, SortOrder: in.SortOrder, IsActive: true,
	}
	if err := h.db.CreatePosition(p); err != nil {
		if isDuplicatePositionName(err) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "position name already exists")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create position")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": p})
}

func (h *PositionHandler) Update(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	positionID, ok := parseParamUint(c, "positionId")
	if !ok {
		return
	}
	var in positionUpdateDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	fields := map[string]interface{}{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "name cannot be empty")
			return
		}
		if len(name) > 255 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "name too long")
			return
		}
		// Create refused duplicates; Update never did, so a rename walked
		// straight past the guard and produced exactly the duplicate the
		// create path rejects. Retired rows are exempt — they sit outside the
		// partial index, so renaming one onto a live name is not a conflict.
		current, getErr := h.db.GetPosition(businessID, positionID)
		if getErr != nil {
			if errors.Is(getErr, database.ErrPositionNotFound) {
				server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Position not found")
				return
			}
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to update position")
			return
		}
		if current.IsActive {
			taken, checkErr := h.activeNameTaken(businessID, name, positionID)
			if checkErr != nil {
				server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to update position")
				return
			}
			if taken {
				server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "position name already exists")
				return
			}
		}
		fields["name"] = name
	}
	if in.ColorHex != nil {
		if !validColorHex(*in.ColorHex) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "color_hex must be a hex color like #1a6b6a")
			return
		}
		fields["color_hex"] = *in.ColorHex
	}
	if in.Department != nil {
		if len(*in.Department) > 16 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "department too long")
			return
		}
		fields["department"] = *in.Department
	}
	if in.SortOrder != nil {
		fields["sort_order"] = *in.SortOrder
	}
	if len(fields) == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "no fields to update")
		return
	}
	if err := h.db.UpdatePosition(businessID, positionID, fields); err != nil {
		if errors.Is(err, database.ErrPositionNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Position not found")
			return
		}
		if isDuplicatePositionName(err) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "position name already exists")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to update position")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *PositionHandler) Delete(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	positionID, ok := parseParamUint(c, "positionId")
	if !ok {
		return
	}
	if err := h.db.DeletePosition(businessID, positionID); err != nil {
		if errors.Is(err, database.ErrPositionNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Position not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to retire position")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *PositionHandler) ListForStaff(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := parseParamUint(c, "staffId")
	if !ok {
		return
	}
	links, err := h.db.ListStaffPositions(businessID, staffID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list assignments")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": links})
}

func (h *PositionHandler) Assign(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := parseParamUint(c, "staffId")
	if !ok {
		return
	}
	var in assignDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if in.PositionID == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "position_id is required")
		return
	}
	if err := h.db.AssignPosition(businessID, staffID, in.PositionID, in.IsPrimary); err != nil {
		if errors.Is(err, database.ErrStaffNotFound) || errors.Is(err, database.ErrPositionNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Staff or position not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to assign position")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// callerHasPayrollWrite reports whether this caller may destroy a link that
// carries an owner-only pay rate. Owners resolve as all-access.
func callerHasPayrollWrite(c *gin.Context) bool {
	perms, all := server.ResolveContextPermissions(c)
	return all || permMatch(perms, "payroll:write")
}

func (h *PositionHandler) Unassign(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := parseParamUint(c, "staffId")
	if !ok {
		return
	}
	positionID, ok := parseParamUint(c, "positionId")
	if !ok {
		return
	}
	canClear := callerHasPayrollWrite(c)
	if err := h.db.UnassignPosition(businessID, staffID, positionID, canClear); err != nil {
		if errors.Is(err, database.ErrStaffPositionRated) {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "This position has a pay rate; only someone with payroll:write can unassign it")
			return
		}
		if errors.Is(err, database.ErrStaffPositionNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Assignment not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to unassign position")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetRate / SetRate are OWNER-ONLY (mounted behind payroll:write). Cents in DB,
// dollars on the wire — same contract as staff compensation.
func (h *PositionHandler) GetRate(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := parseParamUint(c, "staffId")
	if !ok {
		return
	}
	positionID, ok := parseParamUint(c, "positionId")
	if !ok {
		return
	}
	cents, err := h.db.GetPayRateCents(businessID, staffID, positionID)
	if err != nil {
		if errors.Is(err, database.ErrStaffPositionNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Assignment not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to read pay rate")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rateDTO{PayRate: float64(cents) / 100}})
}

func (h *PositionHandler) SetRate(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := parseParamUint(c, "staffId")
	if !ok {
		return
	}
	positionID, ok := parseParamUint(c, "positionId")
	if !ok {
		return
	}
	var in rateRequestDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if in.PayRate == nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "pay_rate is required")
		return
	}
	rate := *in.PayRate
	if rate < 0 || rate > maxPayRate {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "pay_rate out of range")
		return
	}
	cents := int64(math.Round(rate * 100))
	if err := h.db.SetPayRateCents(businessID, staffID, positionID, cents); err != nil {
		if errors.Is(err, database.ErrStaffPositionNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Assignment not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to set pay rate")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rateDTO{PayRate: float64(cents) / 100}})
}
