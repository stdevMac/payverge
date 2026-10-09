package handlers

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services/labor"
)

// LaborPreview returns build-time SCHEDULED-labor for one schedule (the operator
// ScheduleBuilder footer): total scheduled minutes/hours, labor-% vs an editable
// sales target, a per-position breakdown, and the compliance warning chips
// (overtime / posted-late / minor-late). It is display-only and never writes
// payroll.
//
// Route gate is `schedule:write` (manager + owner) — NOT `financial:read` — so a
// manager whose financial visibility the owner has restricted can still build and
// see hours and warnings. Dollar figures (aggregate labor_cost, labor_cost_pct,
// the sales_target echo, and per-position labor_cost) are emitted ONLY to
// owner/`financial:read` callers via a structurally-separate financial DTO;
// every other caller gets the safe DTO which has NO dollar-typed field and no
// labor percentage (defense beyond the model's `json:"-"`).
func (h *ScheduleHandler) LaborPreview(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	scheduleID, ok := parseParamUint(c, "scheduleId")
	if !ok {
		return
	}
	sched, err := h.db.GetScheduleByID(businessID, scheduleID)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Schedule not found")
		return
	}

	// salesTarget is dollars on the wire (operator input); convert to cents.
	var salesTargetCents int64
	if v := c.Query("salesTarget"); v != "" {
		f, perr := strconv.ParseFloat(v, 64)
		if perr != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > 100000000 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "salesTarget must be a finite number between 0 and 100000000")
			return
		}
		salesTargetCents = int64(f*100 + 0.5)
	}

	// Business timezone drives posted-late (now vs week start) and the minor-late
	// end-of-day check; default UTC if the row can't be loaded.
	loc := time.UTC
	var business database.Business
	if bErr := h.db.GetGorm().Where("id = ?", businessID).First(&business).Error; bErr == nil {
		loc = resolveBusinessLocation(&business)
	}

	calc := labor.NewScheduledCalculator(h.db, h.db, h.db)
	preview, err := calc.Preview(labor.ScheduledPreviewInput{
		BusinessID:       businessID,
		ScheduleID:       scheduleID,
		WeekStart:        sched.WeekStart,
		PublishedAt:      sched.PublishedAt,
		Now:              time.Now().In(loc),
		Loc:              loc,
		SalesTargetCents: salesTargetCents,
	})
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to compute labor preview")
		return
	}

	if callerHasFinancialRead(c) {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": newLaborPreviewFinancialDTO(preview)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": newLaborPreviewSafeDTO(preview)})
}

// laborMinutesToHours rounds minutes to two-decimal hours for the wire.
func laborMinutesToHours(min int) float64 {
	return float64(min) / 60.0
}

func laborCentsToDollars(cents int64) float64 {
	return float64(cents) / 100.0
}

// --- Safe DTO (no dollars) — emitted to every non-financial caller. ---

type laborLineSafe struct {
	PositionID uint    `json:"position_id"`
	ShiftCount int     `json:"shift_count"`
	Minutes    int     `json:"minutes"`
	Hours      float64 `json:"hours"`
}

type laborPreviewSafeDTO struct {
	ScheduleID    uint            `json:"schedule_id"`
	TotalMinutes  int             `json:"total_minutes"`
	TotalHours    float64         `json:"total_hours"`
	CanSeeDollars bool            `json:"can_see_dollars"` // always false on the safe DTO — the FE's second lock
	Lines         []laborLineSafe `json:"lines"`
	Warnings      []labor.Warning `json:"warnings"`
}

func newLaborPreviewSafeDTO(p labor.ScheduledPreview) laborPreviewSafeDTO {
	lines := make([]laborLineSafe, 0, len(p.Lines))
	for _, l := range p.Lines {
		lines = append(lines, laborLineSafe{
			PositionID: l.PositionID,
			ShiftCount: l.ShiftCount,
			Minutes:    l.Minutes,
			Hours:      laborMinutesToHours(l.Minutes),
		})
	}
	warnings := p.Warnings
	if warnings == nil {
		warnings = []labor.Warning{}
	}
	return laborPreviewSafeDTO{
		ScheduleID:    p.ScheduleID,
		TotalMinutes:  p.TotalMinutes,
		TotalHours:    laborMinutesToHours(p.TotalMinutes),
		CanSeeDollars: false,
		Lines:         lines,
		Warnings:      warnings,
	}
}

// --- Financial DTO (adds dollars) — owner / financial:read only. ---

type laborLineFinancial struct {
	PositionID uint    `json:"position_id"`
	ShiftCount int     `json:"shift_count"`
	Minutes    int     `json:"minutes"`
	Hours      float64 `json:"hours"`
	LaborCost  float64 `json:"labor_cost"`
}

type laborPreviewFinancialDTO struct {
	ScheduleID    uint                 `json:"schedule_id"`
	TotalMinutes  int                  `json:"total_minutes"`
	TotalHours    float64              `json:"total_hours"`
	LaborCostPct  float64              `json:"labor_cost_pct"`
	CanSeeDollars bool                 `json:"can_see_dollars"` // always true on the financial DTO
	LaborCost     float64              `json:"labor_cost"`
	SalesTarget   float64              `json:"sales_target"`
	Lines         []laborLineFinancial `json:"lines"`
	Warnings      []labor.Warning      `json:"warnings"`
}

func newLaborPreviewFinancialDTO(p labor.ScheduledPreview) laborPreviewFinancialDTO {
	lines := make([]laborLineFinancial, 0, len(p.Lines))
	for _, l := range p.Lines {
		lines = append(lines, laborLineFinancial{
			PositionID: l.PositionID,
			ShiftCount: l.ShiftCount,
			Minutes:    l.Minutes,
			Hours:      laborMinutesToHours(l.Minutes),
			LaborCost:  laborCentsToDollars(l.LaborCostCents),
		})
	}
	warnings := p.Warnings
	if warnings == nil {
		warnings = []labor.Warning{}
	}
	return laborPreviewFinancialDTO{
		ScheduleID:    p.ScheduleID,
		TotalMinutes:  p.TotalMinutes,
		TotalHours:    laborMinutesToHours(p.TotalMinutes),
		LaborCostPct:  p.LaborCostPct,
		CanSeeDollars: true,
		LaborCost:     laborCentsToDollars(p.LaborCostCents),
		SalesTarget:   laborCentsToDollars(p.SalesTargetCents),
		Lines:         lines,
		Warnings:      warnings,
	}
}
