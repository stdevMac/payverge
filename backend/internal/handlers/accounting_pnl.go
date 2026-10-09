package handlers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/accounting"
	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"

	"github.com/gin-gonic/gin"
)

// ProfitLossLine is one category line on the statement (dollars on the wire).
type ProfitLossLine struct {
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
}

// ProfitLossPeriod is one window of the P&L statement.
type ProfitLossPeriod struct {
	StartDate             time.Time        `json:"start_date"`
	EndDate               time.Time        `json:"end_date"`
	Currency              string           `json:"currency"`
	Revenue               float64          `json:"revenue"` // recognized platform payments (GetSummary.auto_income)
	OtherIncome           float64          `json:"other_income"`
	OtherIncomeByCategory []ProfitLossLine `json:"other_income_by_category"`
	COGS                  float64          `json:"cogs"`
	Labor                 float64          `json:"labor"` // paid payroll total (GetSummary.payroll_total)
	Opex                  float64          `json:"opex"`
	OpexByCategory        []ProfitLossLine `json:"opex_by_category"`
	Net                   float64          `json:"net"`
}

// ProfitLossStatement is the full response; Previous is set when compare=prev.
type ProfitLossStatement struct {
	Current  ProfitLossPeriod  `json:"current"`
	Previous *ProfitLossPeriod `json:"previous,omitempty"`
	Delta    *ProfitLossDelta  `json:"delta,omitempty"`
}

// ProfitLossDelta is current − previous for headline totals.
type ProfitLossDelta struct {
	Revenue     float64 `json:"revenue"`
	OtherIncome float64 `json:"other_income"`
	COGS        float64 `json:"cogs"`
	Labor       float64 `json:"labor"`
	Opex        float64 `json:"opex"`
	Net         float64 `json:"net"`
}

// ComposeProfitLoss builds a P&L period from a GetSummary result + food-cost COGS.
// Pure of I/O so access-shape tests can assert composition without HTTP.
//
// COGS here is the recipe-based ESTIMATE from foodcost (ingredient usage priced
// from recipes × sales), NOT purchases. Manual "inventory" expense entries are
// deliberately kept as a separate opex line: they are a cash-purchases ledger,
// a different basis that must not be netted against the estimate. An operator
// logging both will see both lines — the FE statement labels COGS as estimated
// so the distinction is visible rather than silently merged.
func ComposeProfitLoss(summary *accounting.Summary, cogsDollars float64) ProfitLossPeriod {
	if summary == nil {
		return ProfitLossPeriod{}
	}
	otherLines := make([]ProfitLossLine, 0, len(summary.IncomeBreakdown))
	for _, c := range summary.IncomeBreakdown {
		otherLines = append(otherLines, ProfitLossLine{Category: c.Category, Amount: c.Total})
	}
	opexLines := make([]ProfitLossLine, 0, len(summary.ExpenseBreakdown))
	for _, c := range summary.ExpenseBreakdown {
		opexLines = append(opexLines, ProfitLossLine{Category: c.Category, Amount: c.Total})
	}
	net := summary.AutoIncomeTotal + summary.ManualIncomeTotal - cogsDollars - summary.PayrollTotal - summary.ExpenseTotal
	return ProfitLossPeriod{
		StartDate:             summary.StartDate,
		EndDate:               summary.EndDate,
		Currency:              summary.Currency,
		Revenue:               summary.AutoIncomeTotal,
		OtherIncome:           summary.ManualIncomeTotal,
		OtherIncomeByCategory: otherLines,
		COGS:                  cogsDollars,
		Labor:                 summary.PayrollTotal,
		Opex:                  summary.ExpenseTotal,
		OpexByCategory:        opexLines,
		Net:                   net,
	}
}

// previousPeriodWindow returns the immediately preceding window of the same
// CALENDAR-DAY length, re-derived on business-local midnights. Subtracting the
// raw UTC duration instead would skew the boundary by an hour whenever the
// prior window crosses a DST transition in the business's timezone.
func previousPeriodWindow(start, end time.Time, loc *time.Location) (time.Time, time.Time) {
	startLocal := start.In(loc)
	endLocal := end.In(loc)
	days := calendarDaysBetween(startLocal, endLocal)
	if days < 1 {
		days = 1
	}
	prevStart := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), 0, 0, 0, 0, loc).
		AddDate(0, 0, -days)
	return prevStart.UTC(), start
}

// calendarDaysBetween counts local calendar days from a's date to b's date.
func calendarDaysBetween(a, b time.Time) int {
	au := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	bu := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(bu.Sub(au).Hours() / 24)
}

func (h *AccountingHandler) buildProfitLossPeriod(businessID uint, start, end time.Time) (ProfitLossPeriod, error) {
	summary, err := h.accounting.GetSummary(businessID, start, end)
	if err != nil {
		return ProfitLossPeriod{}, err
	}
	cogs := 0.0
	calc := foodcost.NewCalculator(h.db, analytics.NewAnalyticsService(h.db))
	if report, ferr := calc.AnalyzeWindow(businessID, start, end); ferr == nil {
		cogs = report.EstimatedCOGS
	}
	// Food-cost failure is non-fatal for the statement: COGS stays 0 and net still computes.
	return ComposeProfitLoss(summary, cogs), nil
}

// GetProfitLoss handles GET /accounting/profit-loss?start&end[&compare=prev].
func (h *AccountingHandler) GetProfitLoss(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	start, end, ok := parseDateRange(c, business)
	if !ok {
		return
	}

	current, err := h.buildProfitLossPeriod(businessID, start, end)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to compute profit & loss")
		return
	}

	stmt := ProfitLossStatement{Current: current}
	if strings.EqualFold(strings.TrimSpace(c.Query("compare")), "prev") {
		prevStart, prevEnd := previousPeriodWindow(start, end, resolveBusinessLocation(business))
		prev, perr := h.buildProfitLossPeriod(businessID, prevStart, prevEnd)
		if perr != nil {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to compute prior-period profit & loss")
			return
		}
		stmt.Previous = &prev
		stmt.Delta = &ProfitLossDelta{
			Revenue:     current.Revenue - prev.Revenue,
			OtherIncome: current.OtherIncome - prev.OtherIncome,
			COGS:        current.COGS - prev.COGS,
			Labor:       current.Labor - prev.Labor,
			Opex:        current.Opex - prev.Opex,
			Net:         current.Net - prev.Net,
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": stmt})
}

// ExportProfitLossCSV handles GET /accounting/profit-loss/export.csv.
func (h *AccountingHandler) ExportProfitLossCSV(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	start, end, ok := parseDateRange(c, business)
	if !ok {
		return
	}
	current, err := h.buildProfitLossPeriod(businessID, start, end)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to export profit & loss")
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(
		`attachment; filename="profit-loss-%s-%s.csv"`,
		start.UTC().Format("2006-01-02"),
		end.UTC().Format("2006-01-02"),
	))
	w := csv.NewWriter(c.Writer)
	// #930: headers follow the operator locale like every other accounting
	// export; the section/category values below stay machine keys.
	_ = w.Write(profitLossCSVHeaders(resolveExportLang(c)))
	writePLCSV := func(section, category string, amount float64) {
		_ = w.Write([]string{section, category, fmt.Sprintf("%.2f", amount), current.Currency})
	}
	writePLCSV("revenue", "recognized_payments", current.Revenue)
	for _, line := range current.OtherIncomeByCategory {
		writePLCSV("other_income", line.Category, line.Amount)
	}
	writePLCSV("other_income", "_total", current.OtherIncome)
	writePLCSV("cogs", "estimated_food_cost", current.COGS)
	writePLCSV("labor", "paid_payroll", current.Labor)
	for _, line := range current.OpexByCategory {
		writePLCSV("opex", line.Category, line.Amount)
	}
	writePLCSV("opex", "_total", current.Opex)
	writePLCSV("net", "net_profit_loss", current.Net)
	w.Flush()
}
