package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/accounting"
	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/reporting"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
	"github.com/stdevmac/payverge/backend/internal/services/labor"
	"github.com/stdevmac/payverge/backend/internal/services/menuengineering"
	"github.com/stdevmac/payverge/backend/internal/services/wastevariance"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
)

type AccountingHandler struct {
	accounting *accounting.Service
	db         *database.DB
}

type CreateManualEntryRequest struct {
	EntryType string `json:"entry_type" binding:"required"`
	Category  string `json:"category" binding:"required"`
	// Direction comes from EntryType (income vs expense); Amount is a positive
	// MAGNITUDE summed per-type (summarizeManualEntries). A negative amount would
	// reverse the direction — a negative "income" would reduce revenue — corrupting
	// the books. gt=0 enforces the magnitude; lte cap is generous (ledger entries
	// can legitimately be large) but blocks absurd typos.
	Amount      float64   `json:"amount" binding:"required,gt=0,lte=100000000"`
	Currency    string    `json:"currency"`
	OccurredAt  time.Time `json:"occurred_at" binding:"required"`
	Description string    `json:"description" binding:"required"`
	Notes       string    `json:"notes"`
	Reference   string    `json:"reference"`
}

type VoidManualEntryRequest struct {
	Reason string `json:"reason"`
}

type CreatePayrollLineItemRequest struct {
	PayeeType string `json:"payee_type" binding:"required"`
	StaffID   *uint  `json:"staff_id"`
	PayeeName string `json:"payee_name"`
	// Net = Gross + Bonus - Deduction (accounting/service.go), so all three are
	// positive magnitudes. Without these bounds a negative DeductionAmount would
	// INFLATE net pay (- (-x) = +x) and a negative Gross/Bonus would corrupt the
	// total — so enforce non-negative (Gross strictly positive) + a sanity cap.
	GrossAmount     float64 `json:"gross_amount" binding:"required,gt=0,lte=1000000"`
	BonusAmount     float64 `json:"bonus_amount" binding:"gte=0,lte=1000000"`
	DeductionAmount float64 `json:"deduction_amount" binding:"gte=0,lte=1000000"`
	Notes           string  `json:"notes"`
}

type CreatePayrollRunRequest struct {
	PeriodStart time.Time                      `json:"period_start" binding:"required"`
	PeriodEnd   time.Time                      `json:"period_end" binding:"required"`
	Notes       string                         `json:"notes"`
	LineItems   []CreatePayrollLineItemRequest `json:"line_items" binding:"required"`
}

func NewAccountingHandler(db *database.DB) *AccountingHandler {
	return &AccountingHandler{
		accounting: accounting.NewService(db),
		db:         db,
	}
}

func (h *AccountingHandler) loadBusiness(c *gin.Context) (uint, *database.Business, bool) {
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
		return 0, nil, false
	}

	if !server.CheckBusinessAccess(c, business) {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied")
		return 0, nil, false
	}

	return business.ID, business, true
}

// maxAccountingRangeDays caps an explicit start/end span. Period presets are
// not subject to this bound. The comparison uses calendar days (AddDate), so a
// DST transition cannot change whether a span is accepted.
const maxAccountingRangeDays = 400

// parseDateRange parses the "start" and "end" YYYY-MM-DD query parameters in
// the business's local timezone, returning a half-open UTC window
// [start-of-day, next-day-start-of-day). Falls back to UTC if the business has
// an empty or unparseable Timezone. Mirrors the reservation handler pattern
// in backend/internal/server/reservation_handlers.go and reuses the cached
// resolveBusinessLocation helper in analytics.go.
//
// When neither bound is supplied, a "period" preset is honored through the
// canonical resolver (#900): every other money surface on the dashboard —
// summary sparklines, food cost, labor, analytics — takes today/yesterday/
// week/month/quarter/year, and answering 400 "start and end are required" to a
// preset the rest of the API accepts made Outstanding the odd one out. An
// explicit start+end still wins over a preset, and a bare request with neither
// stays a 400 rather than silently defaulting to a window.
func parseDateRange(c *gin.Context, business *database.Business) (time.Time, time.Time, bool) {
	startStr := strings.TrimSpace(c.Query("start"))
	endStr := strings.TrimSpace(c.Query("end"))
	loc := resolveBusinessLocation(business)

	if startStr == "" && endStr == "" {
		period := strings.TrimSpace(c.Query("period"))
		if period == "" {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidDateRange, "start and end query parameters are required")
			return time.Time{}, time.Time{}, false
		}
		win, err := reporting.ResolveWindow(period, nil, nil, loc)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidDateRange, err.Error())
			return time.Time{}, time.Time{}, false
		}
		return win.Start, win.End, true
	}

	if startStr == "" || endStr == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidDateRange, "start and end query parameters are required")
		return time.Time{}, time.Time{}, false
	}

	start, err := time.ParseInLocation("2006-01-02", startStr, loc)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidDateRange, "Invalid start date")
		return time.Time{}, time.Time{}, false
	}

	end, err := time.ParseInLocation("2006-01-02", endStr, loc)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidDateRange, "Invalid end date")
		return time.Time{}, time.Time{}, false
	}

	// Reject inverted ranges with a clear 400 rather than silently returning an
	// empty result set (mirrors the timeseries handler's to.Before(from) guard).
	if end.Before(start) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidDateRange, "end date must be on or after start date")
		return time.Time{}, time.Time{}, false
	}

	// (end - start) in calendar days must be <= 400. start.AddDate is DST-safe:
	// 2025-01-01..2026-02-05 is 400 days and allowed; 2025-01-01..2026-02-06 is 401 and rejected.
	if start.AddDate(0, 0, maxAccountingRangeDays).Before(end) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidDateRange, "date range cannot exceed 400 days")
		return time.Time{}, time.Time{}, false
	}

	// End boundary is exclusive: add one day in the same timezone so DST
	// transitions produce the correct wall-clock "next midnight".
	return start.UTC(), end.AddDate(0, 0, 1).UTC(), true
}

func actorIDs(c *gin.Context) (*uint, *uint) {
	toUintPtr := func(v any) *uint {
		switch value := v.(type) {
		case uint:
			return &value
		case int:
			converted := uint(value)
			return &converted
		case float64:
			converted := uint(value)
			return &converted
		default:
			return nil
		}
	}

	var userID *uint
	if raw, exists := c.Get("user_id"); exists {
		userID = toUintPtr(raw)
	}

	var staffID *uint
	if raw, exists := c.Get("staff_id"); exists {
		staffID = toUintPtr(raw)
	}

	return userID, staffID
}

func (h *AccountingHandler) GetSummary(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	start, end, ok := parseDateRange(c, business)
	if !ok {
		return
	}

	summary, err := h.accounting.GetSummary(businessID, start, end)
	if err != nil {
		if errors.Is(err, accounting.ErrInvalidDateRange) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid date range")
			return
		}
		// Don't leak raw DB errors to the client (matches the repo's error-leak convention).
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to compute accounting summary")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

// GetTimeseries returns a contiguous day-bucket series of manual income,
// expense, and paid payroll for Overview charts.
// GET /api/v1/inside/businesses/:id/accounting/timeseries?start=YYYY-MM-DD&end=YYYY-MM-DD
func (h *AccountingHandler) GetTimeseries(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	start, end, ok := parseDateRange(c, business)
	if !ok {
		return
	}

	loc := resolveBusinessLocation(business)
	series, err := h.accounting.GetTimeseries(businessID, start, end, loc)
	if err != nil {
		if errors.Is(err, accounting.ErrInvalidDateRange) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid date range")
			return
		}
		if errors.Is(err, accounting.ErrRangeTooLarge) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Date range exceeds 400 days")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to compute accounting timeseries")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": series})
}

// GetFoodCost returns the read-only food-cost / margin report for a window.
// It changes nothing in the P&L; it is a display metric. Gated on
// financial:read to match the Accounting dashboard.
//
// Window selection:
//   - ?start=YYYY-MM-DD&end=YYYY-MM-DD — explicit half-open business-local range
//     (same parseDateRange seam as summary/timeseries)
//   - else ?period=day|week|month (default week) — legacy presets
func (h *AccountingHandler) GetFoodCost(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	calc := foodcost.NewCalculator(h.db, analytics.NewAnalyticsService(h.db))
	var report foodcost.Report
	var err error
	if costHealthHasCustomRange(c) {
		start, end, rangeOK := parseDateRange(c, business)
		if !rangeOK {
			return
		}
		report, err = calc.AnalyzeWindow(businessID, start, end)
	} else {
		period := c.DefaultQuery("period", "week")
		report, err = calc.Analyze(businessID, period, resolveBusinessLocation(business))
	}
	if err != nil {
		if errors.Is(err, foodcost.ErrUnsupportedPeriod) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		} else {
			// Don't leak raw DB errors to the client (matches the repo's error-leak convention).
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to compute food cost")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": report})
}

// costHealthHasCustomRange is true when the client is asking for an explicit
// start/end window rather than a period preset. Either param alone is enough
// to enter the custom path (parseDateRange then validates both are present).
func costHealthHasCustomRange(c *gin.Context) bool {
	return strings.TrimSpace(c.Query("start")) != "" || strings.TrimSpace(c.Query("end")) != ""
}

// GetMenuEngineering returns the menu-engineering quadrant classification for a
// business over the requested period. Margin is financial data, so this is
// gated on financial:read. Mirrors GetFoodCost's access + error semantics and
// reuses the exact same data-access path (no new query shape).
func (h *AccountingHandler) GetMenuEngineering(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	period := c.DefaultQuery("period", "week")

	calc := foodcost.NewCalculator(h.db, analytics.NewAnalyticsService(h.db))
	report, err := calc.Analyze(businessID, period, resolveBusinessLocation(business))
	if err != nil {
		if errors.Is(err, foodcost.ErrUnsupportedPeriod) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		} else {
			// Don't leak raw DB errors to the client (matches the repo's error-leak convention).
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to compute menu engineering")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": menuengineering.Classify(report)})
}

// GetWasteVariance returns the read-only waste and ingredient-usage variance
// report for a business over the requested period. Gated on financial:read to
// match the Accounting dashboard. Mirrors GetMenuEngineering's access and error
// semantics; the analytics service is constructed once and passed as both the
// item-stats provider and the window provider.
func (h *AccountingHandler) GetWasteVariance(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	period := c.DefaultQuery("period", "week")

	an := analytics.NewAnalyticsService(h.db)
	calc := wastevariance.NewCalculator(h.db, an, h.db, h.db, an)
	report, err := calc.Analyze(businessID, period, resolveBusinessLocation(business))
	if err != nil {
		if errors.Is(err, foodcost.ErrUnsupportedPeriod) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		} else {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to compute waste and variance")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": report})
}

// callerHasFinancialRead reports whether the caller holds financial:read (owners
// and platform admins resolve to allAccess). It is the in-handler dollar gate for
// the labor-cost actuals breakdown — a non-financial:read caller sees worked
// hours + labor-% only, never a labor-$ amount or another person's rate-derived
// pay. Mirrors callerCanScheduleApprove's resolution semantics.
func callerHasFinancialRead(c *gin.Context) bool {
	perms, allAccess := server.ResolveContextPermissions(c)
	if allAccess {
		return true
	}
	return permMatch(perms, "financial:read")
}

// GetLaborCost returns the read-only labor-cost-%-of-sales report for a window
// (week|month|custom), and composes prime cost via reporting.Compute so food%,
// labor%, and prime% all share RecognizedRevenue as the denominator. Gated on
// financial:read like the rest of the Accounting dashboard. P&L is untouched.
//
// ?basis selects the labor numerator:
//   - "" / "payroll" (default): prorated PAID-payroll actuals (labor_accrued_prorated).
//   - "actual": approved time-clock worked-hours × each staff's primary pay rate
//     (Slice 4 actuals seam — display-only, never writes payroll). Adds a
//     per-staff breakdown + a variance vs the payroll basis. All labor-$ amounts
//     (aggregate, per-person, variance, net sales) are owner/financial:read-gated;
//     a non-financial caller sees worked hours + labor-% only.
//   - "scheduled": NOT served here by design. Scheduled (build-time) labor is
//     inherently per-SCHEDULE, not per-period, so it lives on the dedicated
//     GET /schedule/:scheduleId/labor-preview endpoint (Slice 6). This period
//     reporting endpoint stays actuals/payroll-only; forcing a period→schedules
//     aggregation here would conflate a build preview with a reporting metric.
//
// Cost-health fields (status, recipe_coverage_pct, reason, labor_basis, and the
// shared-denominator percentages) come from reporting.Compute. Ratios above 1.0
// surface as status=implausible with prime_cost_pct omitted — never clamped.
func (h *AccountingHandler) GetLaborCost(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	period := c.DefaultQuery("period", "week")
	basis := strings.ToLower(strings.TrimSpace(c.DefaultQuery("basis", "payroll")))
	loc := resolveBusinessLocation(business)
	an := analytics.NewAnalyticsService(h.db)

	switch basis {
	case "", "payroll", "actual":
		// supported here
	case "scheduled":
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "scheduled basis is served by the schedule labor-preview endpoint, not the period labor-cost report")
		return
	default:
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "basis must be payroll or actual")
		return
	}

	// Resolve the money window once via reporting.ResolveWindow so labor, food
	// COGS, and cost-health share identical bounds.
	customRange := costHealthHasCustomRange(c)
	var win reporting.Window
	if customRange {
		rangeStart, rangeEnd, rangeOK := parseDateRange(c, business)
		if !rangeOK {
			return
		}
		s, e := rangeStart, rangeEnd
		var werr error
		win, werr = reporting.ResolveWindow("", &s, &e, loc)
		if werr != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, werr.Error())
			return
		}
	} else {
		// Labor period reporting is week|month only (day has no payroll resolution).
		switch period {
		case "":
			period = "week"
		case "week", "month":
		default:
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput,
				"labor: unsupported period \""+period+"\" (allowed: week, month)")
			return
		}
		var werr error
		win, werr = reporting.ResolveWindow(period, nil, nil, loc)
		if werr != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, werr.Error())
			return
		}
	}

	laborCalc := labor.NewCalculator(h.db, an, an)
	rep, err := laborCalc.AnalyzeWindow(businessID, win.Start, win.End, win.Label)
	if err != nil {
		if errors.Is(err, labor.ErrUnsupportedPeriod) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		} else {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to compute labor cost")
		}
		return
	}

	// Food-cost COGS for the same window. Failure is non-fatal: labor must not
	// depend on recipe coverage. On error, COGS/mapped sales are 0.
	var fc foodcost.Report
	fcOK := false
	fcCalc := foodcost.NewCalculator(h.db, an)
	if fcr, ferr := fcCalc.AnalyzeWindow(businessID, win.Start, win.End); ferr == nil {
		fc = fcr
		fcOK = true
	}

	if basis == "actual" {
		if customRange {
			// Actual basis still resolves via period presets only; custom start/end
			// is payroll-basis for Overview cost-health. Reject to avoid silent
			// period mismatch.
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "basis=actual does not accept start/end; use period=week|month")
			return
		}
		actualRep, aerr := laborCalc.WithWorkedHours(h.db).AnalyzeActual(businessID, period, loc)
		if aerr != nil {
			if errors.Is(aerr, labor.ErrUnsupportedPeriod) {
				server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, aerr.Error())
			} else {
				server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to compute actual labor cost")
			}
			return
		}

		// Dollar gate: aggregate + per-person + variance + net-sales $ are
		// owner/financial:read-only. Worked hours and labor-% are always returned.
		canSeeDollars := callerHasFinancialRead(c)

		staffContribs := make([]gin.H, 0, len(actualRep.StaffContributions))
		for _, sc := range actualRep.StaffContributions {
			entry := gin.H{
				"staff_id":       sc.StaffID,
				"worked_minutes": sc.WorkedMinutes,
				"worked_hours":   sc.WorkedHours,
			}
			if canSeeDollars {
				entry["labor_cost"] = sc.LaborCost
			}
			staffContribs = append(staffContribs, entry)
		}

		// Shared-denominator cost health for the actual-basis labor numerator.
		ch := composeCostHealth(actualRep.NetSales, actualRep.LaborCost, fc, fcOK)
		// Payroll-basis labor% for variance (same recognized revenue).
		chPayroll := composeCostHealth(rep.NetSales, rep.LaborCost, fc, fcOK)

		// Variance vs the payroll basis (the existing recognized labor outflow).
		// The percentage delta is a ratio (safe for everyone); the dollar delta is
		// gated. (Scheduled-vs-actual variance lands once the Slice 6 scheduled
		// calculator exists; until then the payroll basis is the comparison point.)
		variance := gin.H{
			"compared_to":    "payroll",
			"labor_cost_pct": ch.LaborCostPct - chPayroll.LaborCostPct,
		}

		data := gin.H{
			"period":                       actualRep.Period,
			"basis":                        "actual",
			"worked_hours":                 actualRep.WorkedHours,
			"has_data":                     actualRep.HasData,
			"staff_contributions":          staffContribs,
			"payroll_basis_labor_cost_pct": chPayroll.LaborCostPct,
		}
		mergeCostHealthFields(data, ch)
		if canSeeDollars {
			data["labor_cost"] = actualRep.LaborCost
			data["net_sales"] = actualRep.NetSales
			data["payroll_basis_labor_cost"] = rep.LaborCost
			// Distinct labels when cash payroll vs accrued labor disagree (finding 8).
			data[reporting.LabelLaborAccruedProrated] = actualRep.LaborCost
			data[reporting.LabelPayrollPaidCashBasis] = rep.LaborCost
			variance["labor_cost"] = actualRep.LaborCost - rep.LaborCost
		}
		data["variance"] = variance

		c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
		return
	}

	ch := composeCostHealth(rep.NetSales, rep.LaborCost, fc, fcOK)
	data := gin.H{
		"period":            rep.Period,
		"labor_cost":        rep.LaborCost,
		"net_sales":         rep.NetSales,
		"payroll_run_count": rep.PayrollRunCount,
		"has_data":          rep.HasData,
		"contributions":     rep.Contributions,
		// labor_accrued_prorated is the same dollar as labor_cost; named so
		// GetSummary().PayrollTotal (cash) is never silently equated with it.
		reporting.LabelLaborAccruedProrated: rep.LaborCost,
	}
	mergeCostHealthFields(data, ch)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// composeCostHealth builds the shared-denominator cost-health report from
// recognized net sales (payment window), labor dollars, and optional foodcost
// COGS/recipe-mapped sales. Recipe-mapped sales are clamped to recognized
// revenue so a coverage gap never exceeds 100% and NewRevenueBasis stays valid.
func composeCostHealth(netSales, laborCost float64, fc foodcost.Report, fcOK bool) reporting.Report {
	cogs, mapped := 0.0, 0.0
	if fcOK {
		cogs = fc.EstimatedCOGS
		mapped = fc.TotalRevenue
	}
	if mapped > netSales {
		mapped = netSales
	}
	basis, err := reporting.NewRevenueBasis(reporting.RevenueComponents{
		RecognizedRevenue: netSales,
		RecipeMappedSales: mapped,
	})
	if err != nil {
		// Defensive: clamp already enforces mapped ≤ netSales; fall back to
		// zero coverage rather than failing the labor endpoint.
		basis = reporting.RevenueBasis{RecognizedRevenue: netSales}
	}
	return reporting.Compute(basis, laborCost, cogs)
}

// mergeCostHealthFields writes reporting.Report fields into a labor-cost
// response. prime_cost_pct is omitted when status is implausible/insufficient.
func mergeCostHealthFields(data gin.H, ch reporting.Report) {
	data["status"] = ch.Status
	data["food_cost_pct"] = ch.FoodCostPct
	data["labor_cost_pct"] = ch.LaborCostPct
	data["recipe_coverage_pct"] = ch.RecipeCoveragePct
	data["labor_basis"] = ch.LaborBasis
	if ch.Reason != "" {
		data["reason"] = ch.Reason
	}
	if ch.PrimeCostPct != nil {
		data["prime_cost_pct"] = *ch.PrimeCostPct
	}
}

func (h *AccountingHandler) ListEntries(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	input := accounting.ListEntriesInput{
		BusinessID: businessID,
		Page:       page,
		PageSize:   pageSize,
	}

	if entryType := strings.TrimSpace(c.Query("type")); entryType != "" {
		value := database.AccountingEntryType(entryType)
		input.EntryType = &value
	}

	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	switch status {
	case "", "all", "active", "voided":
		input.Status = status
	default:
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid status filter")
		return
	}

	input.Category = strings.TrimSpace(c.Query("category"))
	input.Query = strings.TrimSpace(c.Query("q"))

	sortParam := strings.TrimSpace(c.Query("sort"))
	switch sortParam {
	case "", "occurred_at_asc", "amount_desc", "amount_asc":
		input.Sort = sortParam
	default:
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid sort")
		return
	}

	loc := resolveBusinessLocation(business)

	if startStr := strings.TrimSpace(c.Query("start")); startStr != "" {
		start, err := time.ParseInLocation("2006-01-02", startStr, loc)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid start date")
			return
		}
		start = start.UTC()
		input.StartDate = &start
	}

	if endStr := strings.TrimSpace(c.Query("end")); endStr != "" {
		end, err := time.ParseInLocation("2006-01-02", endStr, loc)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid end date")
			return
		}
		end = end.AddDate(0, 0, 1).UTC()
		input.EndDate = &end
	}

	entries, err := h.accounting.ListEntries(input)
	if err != nil {
		// Don't leak raw DB errors to the client (matches the repo's error-leak convention).
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list accounting entries")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": entries})
}

func (h *AccountingHandler) CreateManualEntry(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	var req CreateManualEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if err := accounting.CheckPeriodUnlocked(h.db.GetGorm(), businessID, req.OccurredAt); err != nil {
		if respondPeriodLocked(c, err) {
			return
		}
	}

	userID, staffID := actorIDs(c)
	entry, err := h.accounting.CreateManualEntry(accounting.CreateManualLedgerEntryInput{
		BusinessID:       businessID,
		EntryType:        database.AccountingEntryType(req.EntryType),
		Category:         req.Category,
		Amount:           req.Amount,
		Currency:         req.Currency,
		OccurredAt:       req.OccurredAt,
		Description:      req.Description,
		Notes:            req.Notes,
		Reference:        req.Reference,
		CreatedByUserID:  userID,
		CreatedByStaffID: staffID,
	})
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": entry})
}

func (h *AccountingHandler) VoidManualEntry(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	entryID, err := strconv.ParseUint(c.Param("entryId"), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid entry ID")
		return
	}

	var req VoidManualEntryRequest
	_ = c.ShouldBindJSON(&req)

	// Period lock: voiding is a mutation on the entry's occurred_at date.
	var existing database.ManualLedgerEntry
	if err := h.db.GetGorm().Where("id = ? AND business_id = ?", entryID, businessID).First(&existing).Error; err == nil {
		if err := accounting.CheckPeriodUnlocked(h.db.GetGorm(), businessID, existing.OccurredAt); err != nil {
			if respondPeriodLocked(c, err) {
				return
			}
		}
	}

	userID, staffID := actorIDs(c)
	if err := h.accounting.VoidManualEntry(businessID, uint(entryID), userID, staffID); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *AccountingHandler) ListPayrollRuns(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	loc := resolveBusinessLocation(business)

	var startDate *time.Time
	if startStr := strings.TrimSpace(c.Query("start")); startStr != "" {
		start, err := time.ParseInLocation("2006-01-02", startStr, loc)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid start date")
			return
		}
		start = start.UTC()
		startDate = &start
	}

	var endDate *time.Time
	if endStr := strings.TrimSpace(c.Query("end")); endStr != "" {
		end, err := time.ParseInLocation("2006-01-02", endStr, loc)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid end date")
			return
		}
		end = end.AddDate(0, 0, 1).UTC()
		endDate = &end
	}

	rawStatus := strings.TrimSpace(c.Query("status"))

	// Always paginated (LIMIT + payee_count aggregate, no LineItems preload);
	// a missing page means page 1. There is no unbounded list path.
	page := 1
	if pageStr := strings.TrimSpace(c.Query("page")); pageStr != "" {
		parsed, err := strconv.Atoi(pageStr)
		if err != nil || parsed < 1 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid page")
			return
		}
		page = parsed
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	result, err := h.accounting.ListPayrollRunsPage(accounting.ListPayrollRunsParams{
		BusinessID: businessID,
		Status:     rawStatus,
		Start:      startDate,
		End:        endDate,
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		log.Printf("[accounting] ListPayrollRunsPage business_id=%d error: %v", businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to load payroll runs")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (h *AccountingHandler) CreatePayrollRun(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	var req CreatePayrollRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	// Period lock gates on the run's period end (Wave 5).
	// period_end is a calendar date, not an instant: compare it as-is.
	if err := accounting.CheckDateUnlocked(h.db.GetGorm(), businessID, req.PeriodEnd); err != nil {
		if respondPeriodLocked(c, err) {
			return
		}
	}

	lineItems := make([]accounting.CreatePayrollLineItemInput, 0, len(req.LineItems))
	for _, item := range req.LineItems {
		lineItems = append(lineItems, accounting.CreatePayrollLineItemInput{
			PayeeType:       database.PayrollPayeeType(item.PayeeType),
			StaffID:         item.StaffID,
			PayeeName:       item.PayeeName,
			GrossAmount:     item.GrossAmount,
			BonusAmount:     item.BonusAmount,
			DeductionAmount: item.DeductionAmount,
			Notes:           item.Notes,
		})
	}

	userID, staffID := actorIDs(c)
	run, err := h.accounting.CreatePayrollRun(accounting.CreatePayrollRunInput{
		BusinessID:       businessID,
		PeriodStart:      req.PeriodStart,
		PeriodEnd:        req.PeriodEnd,
		Notes:            req.Notes,
		LineItems:        lineItems,
		CreatedByUserID:  userID,
		CreatedByStaffID: staffID,
	})
	if err != nil {
		if errors.Is(err, accounting.ErrDuplicatePayrollPeriod) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, err.Error())
			return
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": run})
}

func (h *AccountingHandler) GetPayrollRun(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	runID, err := strconv.ParseUint(c.Param("runId"), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid payroll run ID")
		return
	}

	run, err := h.accounting.GetPayrollRun(businessID, uint(runID))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": run})
}

func (h *AccountingHandler) MarkPayrollRunPaid(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	runID, err := strconv.ParseUint(c.Param("runId"), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid payroll run ID")
		return
	}
	// Mark-paid writes paid_at = now, and paid_at is the date the run lands
	// in P&L (cash basis), so the new paid_at must be in an open period too.
	paidAt := time.Now().UTC()
	if err := h.checkPayrollRunPeriodUnlocked(businessID, uint(runID), paidAt); err != nil {
		if respondPeriodLocked(c, err) {
			return
		}
	}

	userID, staffID := actorIDs(c)
	run, err := h.accounting.MarkPayrollRunPaid(businessID, uint(runID), paidAt, userID, staffID)
	if err != nil {
		if errors.Is(err, accounting.ErrPeriodLocked) && respondPeriodLocked(c, err) {
			return
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": run})
}

// DeletePayrollRun removes a draft payroll run (owner-only via payroll:write).
func (h *AccountingHandler) DeletePayrollRun(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	runID, err := strconv.ParseUint(c.Param("runId"), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid payroll run ID")
		return
	}
	if err := h.checkPayrollRunPeriodUnlocked(businessID, uint(runID)); err != nil {
		if respondPeriodLocked(c, err) {
			return
		}
	}

	if err := h.accounting.DeletePayrollRun(businessID, uint(runID)); err != nil {
		if errors.Is(err, accounting.ErrPeriodLocked) && respondPeriodLocked(c, err) {
			return
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// VoidPayrollRun reverses a paid payroll run (owner-only via payroll:write),
// excluding it from P&L while keeping it for the audit trail.
func (h *AccountingHandler) VoidPayrollRun(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	runID, err := strconv.ParseUint(c.Param("runId"), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid payroll run ID")
		return
	}
	if err := h.checkPayrollRunPeriodUnlocked(businessID, uint(runID)); err != nil {
		if respondPeriodLocked(c, err) {
			return
		}
	}

	userID, staffID := actorIDs(c)
	run, err := h.accounting.VoidPayrollRun(businessID, uint(runID), time.Now().UTC(), userID, staffID)
	if err != nil {
		if errors.Is(err, accounting.ErrPeriodLocked) && respondPeriodLocked(c, err) {
			return
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": run})
}

// checkPayrollRunPeriodUnlocked loads the run and gates on its PeriodEnd, on
// its stored paid_at (a paid run's P&L date — voiding it removes the expense
// from that period), and on any extra dates the mutation is about to write
// (mark-paid's new paid_at).
func (h *AccountingHandler) checkPayrollRunPeriodUnlocked(businessID, runID uint, extra ...time.Time) error {
	run, err := h.accounting.GetPayrollRun(businessID, runID)
	if err != nil {
		return err
	}
	// period_end is a calendar date; paid_at and the extra dates are instants
	// read in the business timezone.
	if err := accounting.CheckDateUnlocked(h.db.GetGorm(), businessID, run.PeriodEnd); err != nil {
		return err
	}
	days := append([]time.Time{}, extra...)
	if run.PaidAt != nil {
		days = append(days, *run.PaidAt)
	}
	for _, day := range days {
		if err := accounting.CheckPeriodUnlocked(h.db.GetGorm(), businessID, day); err != nil {
			return err
		}
	}
	return nil
}
