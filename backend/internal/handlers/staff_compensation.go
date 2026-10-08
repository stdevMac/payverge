package handlers

import (
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

type StaffCompensationHandler struct{ db *database.DB }

func NewStaffCompensationHandler(db *database.DB) *StaffCompensationHandler {
	return &StaffCompensationHandler{db: db}
}

type compensationDTO struct {
	EmploymentType string  `json:"employment_type"`
	HourlyRate     float64 `json:"hourly_rate"`   // dollars
	AnnualSalary   float64 `json:"annual_salary"` // dollars
}

// Sanity ceilings for compensation money (dollars). Far above any real wage;
// they exist only to reject garbage and keep dollars*100 within int64.
const (
	maxHourlyRate   = 100_000     // $100k/hour
	maxAnnualSalary = 100_000_000 // $100M/year
)

func validEmploymentType(t string) bool {
	return t == "" || t == "hourly" || t == "salaried"
}

// loadStaffInBusiness loads a staff row, enforcing it belongs to :id (tenant guard).
func (h *StaffCompensationHandler) loadStaffInBusiness(c *gin.Context) (*database.Staff, bool) {
	businessID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business id")
		return nil, false
	}
	staffID, err := strconv.ParseUint(c.Param("staffId"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid staff id")
		return nil, false
	}
	var s database.Staff
	if err := h.db.GetGorm().Where("id = ? AND business_id = ?", uint(staffID), uint(businessID)).First(&s).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Staff not found")
		return nil, false
	}
	return &s, true
}

func (h *StaffCompensationHandler) GetCompensation(c *gin.Context) {
	s, ok := h.loadStaffInBusiness(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": compensationDTO{
		EmploymentType: s.EmploymentType,
		HourlyRate:     float64(s.HourlyRateCents) / 100,
		AnnualSalary:   float64(s.AnnualSalaryCents) / 100,
	}})
}

func (h *StaffCompensationHandler) UpdateCompensation(c *gin.Context) {
	s, ok := h.loadStaffInBusiness(c)
	if !ok {
		return
	}
	var in compensationDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if !validEmploymentType(in.EmploymentType) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "employment_type must be one of: hourly, salaried, or empty")
		return
	}
	if in.HourlyRate < 0 || in.AnnualSalary < 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "rates must be non-negative")
		return
	}
	// Upper bounds keep dollars*100 comfortably inside int64 (a float beyond the
	// int64 range converts to an implementation-defined cents value) and reject
	// nonsensical input. These ceilings are absurdly high for any real wage.
	if in.HourlyRate > maxHourlyRate || in.AnnualSalary > maxAnnualSalary {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "rates exceed the allowed maximum")
		return
	}
	hourlyCents := int64(math.Round(in.HourlyRate * 100))
	annualCents := int64(math.Round(in.AnnualSalary * 100))
	updates := map[string]interface{}{
		"employment_type":     in.EmploymentType,
		"hourly_rate_cents":   hourlyCents,
		"annual_salary_cents": annualCents,
	}
	if err := h.db.GetGorm().Model(&database.Staff{}).Where("id = ?", s.ID).Updates(updates).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to update compensation")
		return
	}
	// Echo the persisted (cents-rounded) value, not the raw input, so the PUT
	// response matches what a subsequent GET returns.
	c.JSON(http.StatusOK, gin.H{"success": true, "data": compensationDTO{
		EmploymentType: in.EmploymentType,
		HourlyRate:     float64(hourlyCents) / 100,
		AnnualSalary:   float64(annualCents) / 100,
	}})
}
