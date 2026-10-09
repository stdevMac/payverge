package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/reporting"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/utils"
)

var tzCache sync.Map // map[string]*time.Location

type AnalyticsHandler struct {
	analytics *analytics.AnalyticsService
	db        *database.DB
}

type dashboardSummariesRequest struct {
	BusinessIDs []uint `json:"business_ids"`
}

// dashboardSummariesBusinessColumns is the exact UNION of every business.X
// column the per-id dashboard-summary path reads, derived from its consumers so
// the batched projected load reproduces a full-row load's access + billing
// decisions exactly (N1-01):
//
//   - CheckBusinessAccess → CheckBusinessOwnership: owner_address, user_id,
//     demo_owner_user_id; listedDemoForAdmin: is_demo, kind;
//     staff path compares business.ID → id.
//   - IsBusinessOperational: is_demo, is_active, closed_at. All three are
//     load-bearing — omitting is_active/closed_at fail-OPENs a suspended or
//     closed business (the OF-04 / sibling fail-open class of bug).
//   - dashboardSummaryForBusiness/buildTodayByStaff: id (analytics key) +
//     resolveBusinessLocation: timezone.
var dashboardSummariesBusinessColumns = database.BusinessAccessDecisionColumns

func NewAnalyticsHandler(db *database.DB) *AnalyticsHandler {
	return &AnalyticsHandler{
		analytics: analytics.NewAnalyticsService(db),
		db:        db,
	}
}

func businessFromRouteIdentifier(c *gin.Context) (*database.Business, error) {
	return database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
}

// analyticsRangeParam reads an explicit-window bound under either spelling.
// The accounting family speaks start/end; the timeseries and page-analytics
// endpoints speak from/to and start_date/end_date. Accepting both here closes
// the gap that let a hand-built ?start_date=…&end_date=… fall through
// completely unread (#925).
func analyticsRangeParam(c *gin.Context, primary, alias string) string {
	if v := strings.TrimSpace(c.Query(primary)); v != "" {
		return v
	}
	return strings.TrimSpace(c.Query(alias))
}

// parseAnalyticsRange resolves the optional explicit sales window into a
// half-open [start, end) pair of UTC instants anchored on the venue's calendar
// (end is the requested day inclusive, so the exclusive bound is the next local
// midnight — same convention as accounting's parseDateRange).
//
// Returns (nil, nil, true) when no range was asked for, leaving the preset
// period in charge. Any range that IS asked for and cannot be honoured — one
// bound only, an unparseable date, or end before start — writes a 400 and
// returns ok=false. Silently substituting the default period for a bad range is
// exactly the #925 defect: the owner asked for last night and got last week
// with a 200.
func parseAnalyticsRange(c *gin.Context, loc *time.Location) (*time.Time, *time.Time, bool) {
	startStr := analyticsRangeParam(c, "start", "start_date")
	endStr := analyticsRangeParam(c, "end", "end_date")

	if startStr == "" && endStr == "" {
		return nil, nil, true
	}
	if startStr == "" || endStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    server.ErrCodeInvalidDateRange,
			"error":   "start and end must be provided together. Use YYYY-MM-DD",
		})
		return nil, nil, false
	}

	start, err := time.ParseInLocation("2006-01-02", startStr, loc)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    server.ErrCodeInvalidDateRange,
			"error":   "Invalid start date. Use YYYY-MM-DD",
		})
		return nil, nil, false
	}
	end, err := time.ParseInLocation("2006-01-02", endStr, loc)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    server.ErrCodeInvalidDateRange,
			"error":   "Invalid end date. Use YYYY-MM-DD",
		})
		return nil, nil, false
	}
	if end.Before(start) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    server.ErrCodeInvalidDateRange,
			"error":   "end date must be on or after start date",
		})
		return nil, nil, false
	}

	// Inclusive end day → exclusive next local midnight (AddDate keeps the
	// right wall clock across DST).
	startUTC := start.UTC()
	endUTC := end.AddDate(0, 0, 1).UTC()
	return &startUTC, &endUTC, true
}

// analyticsRangeFilenamePart names a custom window for a download filename
// ("2026-08-22_2026-08-22"); empty when no custom range was requested, so the
// preset keeps its historical `sales_data_week.csv` name.
func analyticsRangeFilenamePart(c *gin.Context, start, end *time.Time, loc *time.Location) string {
	if start == nil || end == nil {
		return ""
	}
	// end is exclusive; step back inside the window to name the last included day.
	last := end.In(loc).AddDate(0, 0, -1)
	return start.In(loc).Format("2006-01-02") + "_" + last.Format("2006-01-02")
}

// GetSalesAnalytics returns sales analytics for a business
// GET /api/v1/businesses/:id/analytics/sales?period=today|yesterday|week|month|quarter|year&date=2024-01-15
func (h *AnalyticsHandler) GetSalesAnalytics(c *gin.Context) {
	business, err := businessFromRouteIdentifier(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Business not found",
		})
		return
	}

	if !server.CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Access denied"})
		return
	}

	period := c.DefaultQuery("period", "today")
	dateStr := c.Query("date")
	loc := resolveBusinessLocation(business)

	if period == "custom" {
		if dateStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "date is required when period=custom",
			})
			return
		}

		// Parse specific date for daily report in the business's own timezone so
		// the day window matches the operator's calendar, not the server clock.
		date, err := time.ParseInLocation("2006-01-02", dateStr, loc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid date format. Use YYYY-MM-DD",
			})
			return
		}

		report, err := h.analytics.GetDailySales(business.ID, date)
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

	// An explicit start/end (or start_date/end_date) window wins over the
	// preset; a broken one 400s instead of quietly becoming the preset (#925).
	rangeStart, rangeEnd, ok := parseAnalyticsRange(c, loc)
	if !ok {
		return
	}

	// Get period report
	report, err := h.analytics.GetPeriodReportRange(business.ID, period, rangeStart, rangeEnd, loc)
	if err != nil {
		writeAnalyticsServiceError(c, err, "Failed to get sales analytics")
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
	business, err := businessFromRouteIdentifier(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Business not found",
		})
		return
	}

	if !server.CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Access denied"})
		return
	}

	period := c.DefaultQuery("period", "today")

	report, err := h.analytics.GetTipAnalytics(business.ID, period, resolveBusinessLocation(business))
	if err != nil {
		writeAnalyticsServiceError(c, err, "Failed to get tip analytics")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    report,
	})
}

// GetTipsByStaff returns the per-staff tip rollup for a period.
// GET /api/v1/businesses/:id/analytics/tips-by-staff?period=today|yesterday|week|month|quarter|year
func (h *AnalyticsHandler) GetTipsByStaff(c *gin.Context) {
	business, err := businessFromRouteIdentifier(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Business not found"})
		return
	}
	if !server.CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Access denied"})
		return
	}
	period := c.DefaultQuery("period", "week")
	rows, err := h.analytics.GetTipsByStaff(business.ID, period, resolveBusinessLocation(business))
	if err != nil {
		writeAnalyticsServiceError(c, err, "Failed to get tips by staff")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// GetItemAnalytics returns menu item performance analytics
// GET /api/v1/businesses/:id/analytics/items?period=today|yesterday|week|month|quarter|year
func (h *AnalyticsHandler) GetItemAnalytics(c *gin.Context) {
	business, err := businessFromRouteIdentifier(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Business not found",
		})
		return
	}

	if !server.CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Access denied"})
		return
	}

	period := c.DefaultQuery("period", "week")

	// Bound the item list so the analytics tab doesn't render every menu item as
	// a full card. Default MaxPopularItemsLimit; an explicit ?limit overrides
	// (0/absent = default, keeping the endpoint additive/BE-first) but cannot
	// exceed the cap. GetPopularItems applies the same cap at SQL.
	limit := analytics.MaxPopularItemsLimit
	if v := c.Query("limit"); v != "" {
		if parsed, perr := strconv.Atoi(v); perr == nil && parsed > 0 {
			limit = parsed
			if limit > analytics.MaxPopularItemsLimit {
				limit = analytics.MaxPopularItemsLimit
			}
		}
	}

	items, err := h.analytics.GetPopularItems(business.ID, limit, period, resolveBusinessLocation(business))
	if err != nil {
		writeAnalyticsServiceError(c, err, "Failed to get item analytics")
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
	business, err := businessFromRouteIdentifier(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Business not found",
		})
		return
	}

	if !server.CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Access denied"})
		return
	}

	period := c.DefaultQuery("period", "week")
	format := c.DefaultQuery("format", "csv")
	// Share the accounting exports' locale resolution (#930): an explicit
	// ?lang= still wins, but a browser that only sends Accept-Language now gets
	// a localized header row instead of a silent English default.
	lang := resolveExportLang(c)
	loc := resolveBusinessLocation(business)

	// An explicit start/end (or start_date/end_date) window wins over the
	// preset; a broken one 400s instead of quietly downloading the preset's
	// rows under a filename that names a period nobody asked for (#925).
	rangeStart, rangeEnd, ok := parseAnalyticsRange(c, loc)
	if !ok {
		return
	}

	data, err := h.analytics.ExportSalesDataRange(business.ID, period, rangeStart, rangeEnd, format, loc, lang)
	if err != nil {
		status, message := exportSalesDataErrorResponse(err)
		c.JSON(status, gin.H{
			"success": false,
			"error":   message,
		})
		return
	}

	// Set appropriate headers for file download. A custom window names itself
	// so the saved file cannot claim to be the default period.
	window := period
	if named := analyticsRangeFilenamePart(c, rangeStart, rangeEnd, loc); named != "" {
		window = named
	}
	filename := "sales_data_" + window + "." + format
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

func exportSalesDataErrorResponse(err error) (int, string) {
	if message, ok := analyticsValidationErrorMessage(err); ok {
		return http.StatusBadRequest, message
	}

	switch {
	case errors.Is(err, database.ErrRecognizedPaymentEventRangeTooLarge):
		return http.StatusRequestEntityTooLarge, "recognized payment export range is too large; choose a period of 31 days or less"
	case errors.Is(err, database.ErrRecognizedPaymentEventLimitExceeded):
		return http.StatusRequestEntityTooLarge, "recognized payment export has too many rows; narrow the period and try again"
	default:
		return http.StatusInternalServerError, "Failed to export data"
	}
}

func writeAnalyticsServiceError(c *gin.Context, err error, fallbackMessage string) {
	if message, ok := analyticsValidationErrorMessage(err); ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   message,
		})
		return
	}

	c.JSON(http.StatusInternalServerError, gin.H{
		"success": false,
		"error":   fallbackMessage,
	})
}

func analyticsValidationErrorMessage(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	message := err.Error()
	if index := strings.Index(message, "unsupported period:"); index >= 0 {
		return message[index:], true
	}
	if strings.HasPrefix(message, "unsupported format:") {
		return message, true
	}
	return "", false
}

// GetTimeseries returns a zero-filled per-day analytics series.
// GET /api/v1/businesses/:id/analytics/timeseries?from=YYYY-MM-DD&to=YYYY-MM-DD
// Optional period=today|7d|week|month|… is used when from/to are omitted
// (default remains last 30 local days). Explicit from/to still win.
func (h *AnalyticsHandler) GetTimeseries(c *gin.Context) {
	business, err := businessFromRouteIdentifier(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Business not found"})
		return
	}
	if !server.CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Access denied"})
		return
	}

	// Anchor the series window to the business's own calendar so day buckets
	// align to the operator's local days, not the server clock.
	loc := resolveBusinessLocation(business)
	now := time.Now().In(loc)
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	from := to.AddDate(0, 0, -29) // default: last 30 days inclusive

	hasFrom := c.Query("from") != ""
	hasTo := c.Query("to") != ""
	if period := strings.TrimSpace(c.Query("period")); period != "" && !hasFrom && !hasTo {
		win, perr := reporting.ResolveWindowAt(period, nil, nil, loc, now)
		if perr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": server.ErrCodeInvalidDateRange, "error": "unsupported period: " + period})
			return
		}
		from = win.Start.In(loc)
		// GetDailySeries treats `to` as an inclusive calendar day. ResolveWindow
		// is half-open [start, end), so the last included instant is end-ε.
		last := win.End.In(loc).Add(-time.Nanosecond)
		to = time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, loc)
	}

	if v := c.Query("from"); v != "" {
		parsed, perr := time.ParseInLocation("2006-01-02", v, loc)
		if perr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": server.ErrCodeInvalidDateRange, "error": "Invalid from date. Use YYYY-MM-DD"})
			return
		}
		from = parsed
	}
	if v := c.Query("to"); v != "" {
		parsed, perr := time.ParseInLocation("2006-01-02", v, loc)
		if perr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": server.ErrCodeInvalidDateRange, "error": "Invalid to date. Use YYYY-MM-DD"})
			return
		}
		to = parsed
	}
	if to.Before(from) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": server.ErrCodeInvalidDateRange, "error": "to must be on or after from"})
		return
	}
	// Bound the range so a hand-edited URL can't request thousands of days.
	if from.Before(to.AddDate(0, 0, -366)) {
		from = to.AddDate(0, 0, -366)
	}

	// GetDailySeries widens the SQL window to whole days internally, so plain
	// day-aligned bounds are correct here — no need to add 24h to `to`.
	series, err := h.analytics.GetDailySeries(business.ID, from, to)
	if err != nil {
		writeAnalyticsServiceError(c, err, "Failed to get analytics series")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": series})
}

// GetLiveBills returns currently active bills for real-time dashboard
// GET /api/v1/businesses/:id/analytics/live-bills
func (h *AnalyticsHandler) GetLiveBills(c *gin.Context) {
	business, err := businessFromRouteIdentifier(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Business not found",
		})
		return
	}

	if !server.CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Access denied"})
		return
	}

	// Optional ?limit bounds the payload (default DefaultActiveBillSummaryLimit).
	// The `capped` flag lets the dashboard show an honest "showing N of more"
	// banner instead of silently truncating the busiest boards. A larger
	// ?limit is clamped so it cannot reach SQL.
	const maxLiveBillsLimit = database.DefaultActiveBillSummaryLimit
	limit := maxLiveBillsLimit
	if v := c.Query("limit"); v != "" {
		if parsed, perr := strconv.Atoi(v); perr == nil && parsed > 0 {
			limit = parsed
			if limit > maxLiveBillsLimit {
				limit = maxLiveBillsLimit
			}
		}
	}

	bills, capped, err := h.db.GetActiveBillSummariesByBusinessIDBounded(business.ID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get live bills",
		})
		return
	}

	// Enhance bills with additional info
	tableByID := make(map[uint]database.Table)
	tableIDs := make([]uint, 0, len(bills))
	seenTables := make(map[uint]struct{})
	for _, bill := range bills {
		if bill.TableID == 0 {
			continue
		}
		if _, seen := seenTables[bill.TableID]; seen {
			continue
		}
		seenTables[bill.TableID] = struct{}{}
		tableIDs = append(tableIDs, bill.TableID)
	}
	if len(tableIDs) > 0 {
		var tables []database.Table
		if err := database.GetDB().Select("id", "name", "table_code").Where("id IN ?", tableIDs).Find(&tables).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to get live bill tables",
			})
			return
		}
		for _, table := range tables {
			tableByID[table.ID] = table
		}
	}

	var liveBills []gin.H
	for _, bill := range bills {
		table := tableByID[bill.TableID]

		// Calculate remaining amount
		remainingAmount := bill.TotalAmount - bill.PaidAmount

		liveBill := gin.H{
			"id":          bill.ID,
			"bill_number": bill.BillNumber,
			// table_id = 0 is the "no table" sentinel; counter_id lets the
			// dashboard tell counter bills from delivery bills when labeling.
			"table_id":         bill.TableID,
			"counter_id":       bill.CounterID,
			"table_name":       table.Name,
			"table_code":       table.TableCode,
			"total_amount":     float64(bill.TotalAmount) / 100.0,
			"paid_amount":      float64(bill.PaidAmount) / 100.0,
			"remaining":        float64(remainingAmount) / 100.0,
			"remaining_amount": float64(remainingAmount) / 100.0,
			"tip_amount":       float64(bill.TipAmount) / 100.0,
			"status":           bill.Status,
			"created_at":       bill.CreatedAt,
			"updated_at":       bill.UpdatedAt,
		}

		liveBills = append(liveBills, liveBill)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    liveBills,
		// `capped` is true when more active bills exist than were returned, so
		// the dashboard can show a "showing first N" banner. `limit` echoes the
		// bound that was applied. Both are additive — existing clients that only
		// read `data` are unaffected.
		"capped": capped,
		"limit":  limit,
	})
}

// GetDashboardSummary returns a summary of key metrics for the dashboard
// GET /api/v1/businesses/:id/analytics/dashboard
func (h *AnalyticsHandler) GetDashboardSummary(c *gin.Context) {
	business, err := businessFromRouteIdentifier(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Business not found",
		})
		return
	}

	if !server.CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Access denied"})
		return
	}

	summary, status, message := h.dashboardSummaryForBusiness(c, business)
	if status != 0 {
		c.JSON(status, gin.H{
			"success": false,
			"error":   message,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    summary,
	})
}

// GetDashboardSummaries returns dashboard summaries for multiple businesses in
// a single request. Each business is still authorization-checked individually.
func (h *AnalyticsHandler) GetDashboardSummaries(c *gin.Context) {
	var req dashboardSummariesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if len(req.BusinessIDs) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Too many businesses requested"})
		return
	}

	// Collect the deduped, non-zero ids first (preserving the per-id 0 → 400
	// check in request order), then batch-load the businesses in ONE query.
	seen := make(map[uint]struct{}, len(req.BusinessIDs))
	ids := make([]uint, 0, len(req.BusinessIDs))
	for _, businessID := range req.BusinessIDs {
		if businessID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid business id"})
			return
		}
		if _, ok := seen[businessID]; ok {
			continue
		}
		seen[businessID] = struct{}{}
		ids = append(ids, businessID)
	}

	// dashboardSummariesBusinessColumns is the projection that exactly reproduces
	// every business.X read on the per-id path: ownership (CheckBusinessAccess →
	// CheckBusinessOwnership: owner_address, user_id, demo_owner_user_id;
	// listedDemoForAdmin: is_demo, kind; staff path: id), the admin lifecycle
	// gate (IsBusinessOperational: is_demo, is_active, closed_at), and the
	// summary build (resolveBusinessLocation: timezone; analytics keyed on id).
	// Dropping any of those columns silently fail-opens/closes the gate (see
	// the N1-01 / OF-04 regression guards).
	var businesses []database.Business
	if len(ids) > 0 {
		if err := h.db.GetGorm().
			Select(dashboardSummariesBusinessColumns).
			Where("id IN ?", ids).
			Find(&businesses).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to load businesses"})
			return
		}
	}
	byID := make(map[uint]*database.Business, len(businesses))
	for i := range businesses {
		byID[businesses[i].ID] = &businesses[i]
	}

	results := gin.H{}
	for _, businessID := range ids {
		business, ok := byID[businessID]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Business not found"})
			return
		}
		if !server.CheckBusinessAccess(c, business) {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Access denied"})
			return
		}
		// Skip suspended or closed businesses instead of failing the
		// whole batch. The cross-business overview page lists every owned venue;
		// one suspended venue must not hide metrics for the rest.
		if !database.IsBusinessOperational(business) {
			continue
		}

		summary, status, message := h.dashboardSummaryForBusiness(c, business)
		if status != 0 {
			c.JSON(status, gin.H{"success": false, "error": message})
			return
		}
		results[strconv.FormatUint(uint64(business.ID), 10)] = summary
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
	})
}

func (h *AnalyticsHandler) dashboardSummaryForBusiness(c *gin.Context, business *database.Business) (gin.H, int, string) {
	// Get today's sales using the business's own timezone so the day boundary
	// matches the window used by buildTodayByStaff — avoiding the mismatch where
	// today.revenue and by_current_staff.revenue_from_paid could span different
	// calendar days for restaurants east of UTC.
	loc := resolveBusinessLocation(business)
	todayStart := database.ServiceDayStart(time.Now(), loc, business.ServiceDayStartMinute)
	todaySales, err := h.analytics.GetPaymentWindowSummary(business.ID, todayStart, todayStart.AddDate(0, 0, 1))
	if err != nil {
		return nil, http.StatusInternalServerError, "Failed to get today's sales"
	}

	// Same ISO week-to-date window as GET /analytics/sales?period=week so
	// Overview and Analytics cannot disagree (#703). Overview labels this
	// block "This week".
	weekReport, err := h.analytics.GetPaymentPeriodSummary(business.ID, "week", loc)
	if err != nil {
		return nil, http.StatusInternalServerError, "Failed to get week's analytics"
	}

	activeBills, err := h.db.GetActiveBillSummariesByBusinessID(business.ID)
	if err != nil {
		activeBills = []database.ActiveBillSummary{} // Default to empty if error
	}

	// Get top 5 items for the week; limit is applied at SQL level.
	topItems, err := h.analytics.GetPopularItems(business.ID, 5, "week", loc)
	if err != nil {
		topItems = []analytics.ItemStats{} // Default to empty if error
	}

	collectedRevenue := todaySales.TotalRevenue
	floorRemaining := 0.0
	if openRemaining, openErr := h.analytics.LiveOpenCheckRemaining(business.ID); openErr == nil {
		// Live remaining stays a separate floor figure. Mixing it into
		// today.revenue sold leftover open/partial due as "ventas" (#703).
		floorRemaining = openRemaining
	}
	kitchenTickets, kitchenErr := h.analytics.LiveKitchenTicketCount(business.ID)
	if kitchenErr != nil {
		kitchenTickets = 0
	}

	todayBlock := gin.H{
		"revenue":           collectedRevenue,
		"collected_revenue": collectedRevenue,
		"floor_remaining":   floorRemaining,
		"tips":              todaySales.TotalTips,
		"transactions":      todaySales.TransactionCount,
		"bills":             todaySales.BillCount,
		"order_count":       kitchenTickets,
	}

	// If the caller is a staff member, append their personal today stats.
	if tokenType, _ := c.Get("token_type"); tokenType == "staff" {
		if raw, exists := c.Get("staff_id"); exists {
			var staffID uint
			switch v := raw.(type) {
			case uint:
				staffID = v
			case int:
				staffID = uint(v)
			case int64:
				staffID = uint(v)
			case float64:
				staffID = uint(v)
			}
			if staffID > 0 {
				byStaff, err := h.buildTodayByStaff(business, staffID, time.Now())
				if err != nil {
					return nil, http.StatusInternalServerError, "Failed to get staff's today analytics"
				}
				todayBlock["by_current_staff"] = byStaff
			}
		}
	}

	byTable := buildActiveBillSummariesByTable(activeBills, time.Now())
	untabled := 0
	for _, bill := range activeBills {
		if bill.TableID == 0 {
			untabled++
		}
	}
	// open_tables = distinct occupied tables (table_id > 0). Never use
	// active_bills (bill count) for this — delivery/counter bills inflate it.
	// untabled_bills keeps those delivery/counter checks visible so badge,
	// Overview, and floor can agree by labeling rather than hiding (#619).
	summary := gin.H{
		"today": todayBlock,
		"week": gin.H{
			"revenue":          weekReport.TotalRevenue,
			"tips":             weekReport.TotalTips,
			"transactions":     weekReport.TransactionCount,
			"bills":            weekReport.BillCount,
			"unique_customers": weekReport.UniqueCustomers,
			"average_ticket":   weekReport.AverageTicket,
		},
		"live": gin.H{
			"active_bills":          len(activeBills),
			"active_bills_by_table": byTable,
			"open_tables":           len(byTable),
			"untabled_bills":        untabled,
		},
		"top_items": topItems,
	}

	return summary, 0, ""
}

func buildActiveBillSummariesByTable(activeBills []database.ActiveBillSummary, now time.Time) map[uint]gin.H {
	type agg struct {
		Count        int
		OldestMins   int
		OldestBillID uint
	}
	byTable := map[uint]*agg{}
	for _, b := range activeBills {
		if b.TableID == 0 {
			continue
		}
		minsOpen := int(now.Sub(b.CreatedAt).Minutes())
		if minsOpen < 0 {
			minsOpen = 0
		}
		if existing, ok := byTable[b.TableID]; ok {
			existing.Count++
			if minsOpen > existing.OldestMins {
				existing.OldestMins = minsOpen
				existing.OldestBillID = b.ID
			}
		} else {
			byTable[b.TableID] = &agg{Count: 1, OldestMins: minsOpen, OldestBillID: b.ID}
		}
	}
	result := make(map[uint]gin.H, len(byTable))
	for tableID, a := range byTable {
		result[tableID] = gin.H{
			"count":          a.Count,
			"oldest_bill_id": a.OldestBillID,
			"oldest_minutes": a.OldestMins,
		}
	}
	return result
}

// resolveBusinessLocation returns the *time.Location for the given business's IANA timezone
// string. Falls back to UTC if the field is empty or the value is unrecognised.
// Results are cached in tzCache to avoid repeated tzdata file reads; unknown zones
// are negative-cached to UTC so the same invalid value doesn't retry-thrash.
func resolveBusinessLocation(business *database.Business) *time.Location {
	if business == nil || business.Timezone == "" {
		return time.UTC
	}
	if cached, ok := tzCache.Load(business.Timezone); ok {
		return cached.(*time.Location)
	}
	loc, err := time.LoadLocation(business.Timezone)
	if err != nil {
		tzCache.Store(business.Timezone, time.UTC) // negative cache
		return time.UTC
	}
	tzCache.Store(business.Timezone, loc)
	return loc
}

// buildTodayByStaff returns per-staff aggregates for the current calendar day.
// bills_created: bills opened by this staff member today.
// bills_paid: bills closed/paid by this staff member today with positive net recognized revenue.
// revenue_from_paid / tips_from_paid: net recognized payment totals from those paid bills.
// The day boundary is computed in the business's own timezone so that restaurants in
// non-UTC zones see the correct window.
//
// A non-nil error is returned when either underlying query fails; callers must surface
// it rather than reporting the zero-valued aggregates, otherwise a transient DB fault
// would silently masquerade as a staff member with no activity today.
func (h *AnalyticsHandler) buildTodayByStaff(business *database.Business, staffID uint, now time.Time) (gin.H, error) {
	loc := resolveBusinessLocation(business)
	startOfDay := database.ServiceDayStart(now, loc, business.ServiceDayStartMinute)
	endOfDay := startOfDay.AddDate(0, 0, 1) // next service-day boundary (DST-safe)

	db := database.GetDB()

	var billsCreated int64
	if err := db.Model(&database.Bill{}).
		Where("business_id = ? AND created_by_staff_id = ? AND created_at >= ? AND created_at < ?",
			business.ID, staffID, startOfDay, endOfDay).
		Count(&billsCreated).Error; err != nil {
		return nil, err
	}

	type paidResult struct {
		Count   int64
		Revenue float64
		Tips    float64
	}
	var result paidResult
	billManagedAlternativePaymentMethods := database.BillManagedAlternativePaymentMethodStrings()
	if err := db.Raw(`
		SELECT
			COUNT(*) AS count,
			COALESCE(SUM(net_amount_cents), 0) / 100.0 AS revenue,
			COALESCE(SUM(net_tip_cents), 0) / 100.0 AS tips
		FROM (
			SELECT
				bill_id,
				SUM(amount_cents) AS net_amount_cents,
				SUM(tip_cents) AS net_tip_cents
			FROM (
				SELECT
					payments.bill_id AS bill_id,
					COALESCE(payments.amount, 0) AS amount_cents,
					COALESCE(payments.tip_amount, 0) AS tip_cents
				FROM payments
				JOIN bills ON payments.bill_id = bills.id
				WHERE bills.business_id = ?
					AND bills.closed_by_staff_id = ?
					AND bills.closed_at >= ? AND bills.closed_at < ?
					AND (
						(payments.status IN ? AND payments.confirmed_at IS NOT NULL AND payments.confirmed_at >= ? AND payments.confirmed_at < ?)
						OR (payments.status = ? AND payments.confirmed_at IS NULL AND payments.updated_at >= ? AND payments.updated_at < ?)
						OR (payments.status IN ? AND payments.confirmed_at IS NULL AND payments.created_at >= ? AND payments.created_at < ?)
					)
				UNION ALL
				SELECT
					payments.bill_id AS bill_id,
					(-COALESCE(payments.amount, 0)) AS amount_cents,
					(-COALESCE(payments.tip_amount, 0)) AS tip_cents
				FROM payments
				JOIN bills ON payments.bill_id = bills.id
				WHERE bills.business_id = ?
					AND bills.closed_by_staff_id = ?
					AND bills.closed_at >= ? AND bills.closed_at < ?
					AND payments.status IN ?
					AND (
						(payments.reversed_at IS NOT NULL AND payments.reversed_at >= ? AND payments.reversed_at < ?)
						OR (payments.reversed_at IS NULL AND payments.updated_at >= ? AND payments.updated_at < ?)
					)
				UNION ALL
				SELECT
					alternative_payments.bill_id AS bill_id,
					COALESCE(alternative_payments.amount, 0) AS amount_cents,
					COALESCE(alternative_payments.tip_amount_cents, 0) AS tip_cents
				FROM alternative_payments
				JOIN bills ON alternative_payments.bill_id = bills.id
				WHERE bills.business_id = ?
					AND bills.closed_by_staff_id = ?
					AND bills.closed_at >= ? AND bills.closed_at < ?
					AND alternative_payments.status IN ?
					AND alternative_payments.payment_method IN ?
					AND (
						(alternative_payments.confirmed_at IS NOT NULL AND alternative_payments.confirmed_at >= ? AND alternative_payments.confirmed_at < ?)
						OR (alternative_payments.confirmed_at IS NULL AND alternative_payments.created_at >= ? AND alternative_payments.created_at < ?)
					)
				UNION ALL
				SELECT
					alternative_payments.bill_id AS bill_id,
					(-COALESCE(alternative_payments.amount, 0)) AS amount_cents,
					(-COALESCE(alternative_payments.tip_amount_cents, 0)) AS tip_cents
				FROM alternative_payments
				JOIN bills ON alternative_payments.bill_id = bills.id
				WHERE bills.business_id = ?
					AND bills.closed_by_staff_id = ?
					AND bills.closed_at >= ? AND bills.closed_at < ?
					AND alternative_payments.status = ?
					AND alternative_payments.payment_method IN ?
					AND alternative_payments.updated_at >= ? AND alternative_payments.updated_at < ?
				UNION ALL
				SELECT
					bills.id AS bill_id,
					CASE
						WHEN COALESCE(bills.paid_amount, 0) > 0 THEN COALESCE(bills.paid_amount, 0)
						ELSE COALESCE(bills.total_amount, 0)
					END AS amount_cents,
					COALESCE(bills.tip_amount, 0) AS tip_cents
				FROM bills
				WHERE bills.business_id = ?
					AND bills.closed_by_staff_id = ?
					AND bills.closed_at >= ? AND bills.closed_at < ?
					AND (bills.paid_amount > ? OR (bills.status = ? AND bills.total_amount > ?))
					AND NOT EXISTS (
						SELECT 1
						FROM payments recognized_payments
						WHERE recognized_payments.bill_id = bills.id
							AND recognized_payments.status IN ?
					)
					AND NOT EXISTS (
						SELECT 1
						FROM alternative_payments recognized_alternative_payments
						WHERE recognized_alternative_payments.bill_id = bills.id
							AND recognized_alternative_payments.status IN ?
							AND recognized_alternative_payments.payment_method IN ?
					)
			) recognized_events
			GROUP BY bill_id
			HAVING SUM(amount_cents) > 0
		) recognized_bills
	`,
		business.ID, staffID, startOfDay, endOfDay,
		database.RecognizedPaymentStatuses, startOfDay, endOfDay,
		database.PaymentStatusConfirmed, startOfDay, endOfDay,
		database.ReversedOrRefundedPaymentStatuses, startOfDay, endOfDay,
		business.ID, staffID, startOfDay, endOfDay, database.ReversedOrRefundedPaymentStatuses, startOfDay, endOfDay, startOfDay, endOfDay,
		business.ID, staffID, startOfDay, endOfDay, database.RecognizedAltPaymentStatuses, billManagedAlternativePaymentMethods, startOfDay, endOfDay, startOfDay, endOfDay,
		business.ID, staffID, startOfDay, endOfDay, database.AltPaymentStatusRefunded, billManagedAlternativePaymentMethods, startOfDay, endOfDay,
		business.ID, staffID, startOfDay, endOfDay, 0, database.BillStatusPaid, 0, database.RecognizedPaymentStatuses, database.RecognizedAltPaymentStatuses, billManagedAlternativePaymentMethods,
	).Scan(&result).Error; err != nil {
		return nil, err
	}

	return gin.H{
		"staff_id":          staffID,
		"bills_created":     billsCreated,
		"bills_paid":        result.Count,
		"revenue_from_paid": result.Revenue,
		"tips_from_paid":    result.Tips,
	}, nil
}
