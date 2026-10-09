package handlers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const exportCSVBatchSize = 500

// ExportEntriesCSV streams manual ledger entries as CSV using FindInBatches
// so memory stays O(batch) regardless of result size.
// GET /api/v1/inside/businesses/:id/accounting/entries/export.csv
func (h *AccountingHandler) ExportEntriesCSV(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	start, end, ok := parseDateRange(c, business)
	if !ok {
		return
	}
	startStr := strings.TrimSpace(c.Query("start"))
	endStr := strings.TrimSpace(c.Query("end"))

	query := h.db.GetGorm().
		Model(&database.ManualLedgerEntry{}).
		Where("business_id = ?", businessID).
		Where("occurred_at >= ? AND occurred_at < ?", start, end)

	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	switch status {
	case "", "all":
		// no status filter
	case "active":
		query = query.Where("voided_at IS NULL")
	case "voided":
		query = query.Where("voided_at IS NOT NULL")
	default:
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid status filter")
		return
	}

	if entryType := strings.TrimSpace(c.Query("type")); entryType != "" {
		query = query.Where("entry_type = ?", strings.ToLower(entryType))
	}
	if category := strings.ToLower(strings.TrimSpace(c.Query("category"))); category != "" {
		query = query.Where("category = ?", category)
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		query = query.Where(
			"(LOWER(description) LIKE ? OR LOWER(COALESCE(reference, '')) LIKE ?)",
			like, like,
		)
	}

	filename := fmt.Sprintf("entries-%s-%s.csv", startStr, endStr)
	writeCSVHeaders(c, filename)

	lang := resolveExportLang(c)

	writer := csv.NewWriter(c.Writer)
	if err := writer.Write(entriesCSVHeaders(lang)); err != nil {
		return
	}
	writer.Flush()

	var batch []database.ManualLedgerEntry
	err := query.Order("occurred_at DESC, id DESC").
		FindInBatches(&batch, exportCSVBatchSize, func(tx *gorm.DB, _ int) error {
			for i := range batch {
				e := &batch[i]
				statusLabel := "active"
				if e.VoidedAt != nil {
					statusLabel = "voided"
				}
				if err := writer.Write([]string{
					e.OccurredAt.UTC().Format(time.RFC3339),
					string(e.EntryType),
					utils.SanitizeCSVField(e.Category),
					utils.SanitizeCSVField(e.Description),
					utils.SanitizeCSVField(e.Reference),
					statusLabel,
					formatCentsAsDollars(e.Amount),
					e.Currency,
					utils.SanitizeCSVField(e.Notes),
				}); err != nil {
					return err
				}
			}
			writer.Flush()
			return writer.Error()
		})
	if err != nil {
		// Headers already sent; cannot switch to JSON error response.
		return
	}
	writer.Flush()
}

// ExportPayrollRunsCSV streams payroll runs as CSV with payee counts from a
// single GROUP BY aggregate per batch (no LineItems preload).
// GET /api/v1/inside/businesses/:id/accounting/payroll-runs/export.csv
func (h *AccountingHandler) ExportPayrollRunsCSV(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}

	start, end, ok := parseDateRange(c, business)
	if !ok {
		return
	}
	startStr := strings.TrimSpace(c.Query("start"))
	endStr := strings.TrimSpace(c.Query("end"))

	query := h.db.GetGorm().
		Model(&database.PayrollRun{}).
		Where("business_id = ?", businessID)

	var status *database.PayrollRunStatus
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		value := database.PayrollRunStatus(raw)
		status = &value
	}
	query = applyPayrollExportFilters(query, &start, &end, status)

	filename := fmt.Sprintf("payroll-runs-%s-%s.csv", startStr, endStr)
	writeCSVHeaders(c, filename)

	writer := csv.NewWriter(c.Writer)
	if err := writer.Write(payrollRunsCSVHeaders(resolveExportLang(c))); err != nil {
		return
	}
	writer.Flush()

	var batch []database.PayrollRun
	err := query.Order("period_start DESC, id DESC").
		FindInBatches(&batch, exportCSVBatchSize, func(tx *gorm.DB, _ int) error {
			counts := payrollPayeeCounts(h.db.GetGorm(), batch)
			for i := range batch {
				run := &batch[i]
				if err := writer.Write([]string{
					run.PeriodStart.UTC().Format("2006-01-02"),
					run.PeriodEnd.UTC().Format("2006-01-02"),
					string(run.Status),
					strconv.Itoa(counts[run.ID]),
					formatCentsAsDollars(run.GrossTotal),
					formatCentsAsDollars(run.BonusTotal),
					formatCentsAsDollars(run.DeductionTotal),
					formatCentsAsDollars(run.NetTotal),
					run.Currency,
				}); err != nil {
					return err
				}
			}
			writer.Flush()
			return writer.Error()
		})
	if err != nil {
		return
	}
	writer.Flush()
}

// entriesCSVHeaders returns localized column headers for the entries export
// (L6-12 / S-14). Includes notes. lang is "en" or "es".
// resolveExportLang picks the operator locale for a CSV export.
//
// An explicit `?lang=` is the operator's stated choice (the dashboard sends its
// SimpleTranslationProvider locale) and always wins. Accept-Language is only a
// fallback for callers that supply no locale — previously it OVERRODE an
// explicit `?lang=en`, so an English dashboard on a Spanish browser downloaded a
// Spanish-headed CSV.
func resolveExportLang(c *gin.Context) string {
	if q := strings.TrimSpace(c.Query("lang")); q != "" {
		return utils.NormalizeOperatorLang(q)
	}
	if al := strings.TrimSpace(c.GetHeader("Accept-Language")); al != "" {
		return utils.NormalizeOperatorLang(al)
	}
	return "en"
}

// payrollRunsCSVHeaders returns localized column headers for the payroll-runs
// export (#930). The entries export has localized since L6-12; payroll shipped
// hardcoded English, so a Spanish dashboard downloaded an English-headed payroll
// book. Only the header row is translated — the status enum, ISO dates and
// currency code stay machine tokens so the file can be re-imported. lang is
// already normalized to "en" or "es" by resolveExportLang.
func payrollRunsCSVHeaders(lang string) []string {
	if lang == "es" {
		return []string{
			"inicio_período", "fin_período", "estado", "beneficiarios", "bruto", "bono", "deducción", "neto", "moneda",
		}
	}
	return []string{
		"period_start", "period_end", "status", "payees", "gross", "bonus", "deduction", "net", "currency",
	}
}

// profitLossCSVHeaders returns localized column headers for the P&L export
// (#930). Section and category VALUES stay stable machine keys ("revenue",
// "net_profit_loss") — downstream consumers key off them.
func profitLossCSVHeaders(lang string) []string {
	if lang == "es" {
		return []string{"sección", "categoría", "monto", "moneda"}
	}
	return []string{"section", "category", "amount", "currency"}
}

func entriesCSVHeaders(lang string) []string {
	if lang == "es" {
		return []string{
			"fecha", "tipo", "categoría", "descripción", "referencia", "estado", "monto", "moneda", "notas",
		}
	}
	return []string{
		"occurred_at", "type", "category", "description", "reference", "status", "amount", "currency", "notes",
	}
}

func writeCSVHeaders(c *gin.Context, filename string) {
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Status(http.StatusOK)
}

func formatCentsAsDollars(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100.0)
}

// payrollPayeeCounts returns COUNT(*) of line items per run ID in one aggregate.
func payrollPayeeCounts(db *gorm.DB, runs []database.PayrollRun) map[uint]int {
	counts := make(map[uint]int, len(runs))
	if len(runs) == 0 {
		return counts
	}
	ids := make([]uint, len(runs))
	for i, run := range runs {
		ids[i] = run.ID
	}
	type payeeCountRow struct {
		PayrollRunID uint
		Cnt          int
	}
	var rows []payeeCountRow
	_ = db.Model(&database.PayrollLineItem{}).
		Select("payroll_run_id, COUNT(*) AS cnt").
		Where("payroll_run_id IN ?", ids).
		Group("payroll_run_id").
		Scan(&rows).Error
	for _, row := range rows {
		counts[row.PayrollRunID] = row.Cnt
	}
	return counts
}

// applyPayrollExportFilters mirrors accounting.applyPayrollRunsFilters so export
// windows match the list endpoint (paid → paid_at; draft/void → period_*).
func applyPayrollExportFilters(query *gorm.DB, startDate, endDate *time.Time, status *database.PayrollRunStatus) *gorm.DB {
	if status != nil {
		query = query.Where("status = ?", *status)
		switch *status {
		case database.PayrollRunStatusPaid:
			if startDate != nil {
				query = query.Where("paid_at IS NOT NULL AND paid_at >= ?", startDate.UTC())
			}
			if endDate != nil {
				query = query.Where("paid_at IS NOT NULL AND paid_at < ?", endDate.UTC())
			}
		default:
			if startDate != nil {
				query = query.Where("period_end >= ?", startDate.UTC())
			}
			if endDate != nil {
				query = query.Where("period_start < ?", endDate.UTC())
			}
		}
		return query
	}

	periodStatuses := []database.PayrollRunStatus{
		database.PayrollRunStatusDraft,
		database.PayrollRunStatusVoid,
	}
	switch {
	case startDate != nil && endDate != nil:
		query = query.Where(
			"(status = ? AND paid_at IS NOT NULL AND paid_at >= ? AND paid_at < ?) OR (status IN ? AND period_end >= ? AND period_start < ?)",
			database.PayrollRunStatusPaid,
			startDate.UTC(),
			endDate.UTC(),
			periodStatuses,
			startDate.UTC(),
			endDate.UTC(),
		)
	case startDate != nil:
		query = query.Where(
			"(status = ? AND paid_at IS NOT NULL AND paid_at >= ?) OR (status IN ? AND period_end >= ?)",
			database.PayrollRunStatusPaid,
			startDate.UTC(),
			periodStatuses,
			startDate.UTC(),
		)
	case endDate != nil:
		query = query.Where(
			"(status = ? AND paid_at IS NOT NULL AND paid_at < ?) OR (status IN ? AND period_start < ?)",
			database.PayrollRunStatusPaid,
			endDate.UTC(),
			periodStatuses,
			endDate.UTC(),
		)
	}
	return query
}
