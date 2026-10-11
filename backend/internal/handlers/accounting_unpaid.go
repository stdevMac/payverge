package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/stdevmac/payverge/backend/internal/accounting"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
)

// UnpaidBillRow is a narrow projection for the Outstanding bills table.
// Money fields are dollars on the wire (cents/100).
//
// Currency names the unit those dollars are in (#900). It is the venue's
// resolved display/default currency — bills have no currency column of their
// own (Bill.Currency is gorm:"-") — but the row must carry it rather than let
// the client borrow one from a different endpoint's response: the Outstanding
// tab formats these amounts with summary.currency, which defaults to USD, so a
// row rendered before or without a summary prints an ARS book as dollars.
//
// TableID rides the LEFT JOIN that already produces TableLabel, so a row can be
// linked back to the table it belongs to instead of matching on a display name.
type UnpaidBillRow struct {
	ID          uint      `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	TableID     *uint     `json:"table_id,omitempty"`
	TableLabel  string    `json:"table_label,omitempty"`
	Total       float64   `json:"total"`
	Paid        float64   `json:"paid"`
	Outstanding float64   `json:"outstanding"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	AgeDays     int       `json:"age_days"`
}

// GetUnpaidBills GET /accounting/unpaid-bills?start&end&page&page_size
// Range-scoped like GetSummary.collection_gap: leftover remainings (non-void,
// total > paid) created in [start, end), same predicate, so every check that
// makes the KPI appears here (#770) and the date filter is honored (#799 —
// a single-day filter used to replay the whole leftover book). Pre-range
// debt is not hidden (#222 / L6-23): it is surfaced as carried_over count +
// carried_over_amount alongside the in-range rows.
func (h *AccountingHandler) GetUnpaidBills(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	start, end, ok := parseDateRange(c, business)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Outstanding in [start, end): leftover remainings on open / partial /
	// abandoned / closed checks. Live floor tabs with money still owed are
	// part of billed−collected and must be drillable.
	//
	// Decision-14: COUNT skips the tables LEFT JOIN (label is display-only for
	// the page query). Same remaining-due predicate on bills only so total
	// stays correct while the set does not pay for a join on every count.
	type row struct {
		ID         uint
		CreatedAt  time.Time
		TableID    *uint
		TableLabel string
		TotalCents int64
		PaidCents  int64
		Status     string
	}
	billsPredicate := accounting.RemainingDueInRange(
		h.db.GetGorm().Table("bills"), businessID, start, end, "bills")

	var total int64
	if err := billsPredicate.Count(&total).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to count unpaid bills")
		return
	}

	// Pre-range debt stays visible as an aggregate (#222 / L6-23): leftover
	// remainings created before start, still due, are named — not listed as
	// fake in-range rows, not silently dropped (#799).
	var carried struct {
		Count       int64
		AmountCents int64
	}
	if err := accounting.RemainingDueCarriedOver(
		h.db.GetGorm().Table("bills"), businessID, start, "bills").
		Select("COUNT(*) AS count, COALESCE(SUM(bills.total_amount - bills.paid_amount), 0) AS amount_cents").
		Scan(&carried).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to count carried-over bills")
		return
	}

	var rows []row
	pageQ := h.db.GetGorm().Table("bills").
		Select(`
			bills.id AS id,
			bills.created_at AS created_at,
			bills.table_id AS table_id,
			COALESCE(tables.name, '') AS table_label,
			bills.total_amount AS total_cents,
			bills.paid_amount AS paid_cents,
			bills.status AS status
		`).
		Joins("LEFT JOIN tables ON tables.id = bills.table_id AND tables.business_id = bills.business_id")
	if err := accounting.RemainingDueInRange(pageQ, businessID, start, end, "bills").
		Order("(bills.total_amount - bills.paid_amount) DESC, bills.id DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Scan(&rows).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to list unpaid bills")
		return
	}

	// The venue currency comes off the business row loadBusiness already
	// fetched — naming the unit costs no extra query (#900).
	currency := business.ResolvedCurrency()

	now := time.Now().UTC()
	out := make([]UnpaidBillRow, 0, len(rows))
	for _, r := range rows {
		outStanding := float64(r.TotalCents-r.PaidCents) / 100.0
		age := int(now.Sub(r.CreatedAt).Hours() / 24)
		if age < 0 {
			age = 0
		}
		out = append(out, UnpaidBillRow{
			ID:          r.ID,
			CreatedAt:   r.CreatedAt,
			TableID:     r.TableID,
			TableLabel:  r.TableLabel,
			Total:       float64(r.TotalCents) / 100.0,
			Paid:        float64(r.PaidCents) / 100.0,
			Outstanding: outStanding,
			Currency:    currency,
			Status:      r.Status,
			AgeDays:     age,
		})
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"bills": out,
			// Denominates carried_over_amount as well as the rows, matching the
			// Summary / Timeseries envelopes (#900).
			"currency":    currency,
			"total":       total,
			"page":        page,
			"page_size":   pageSize,
			"total_pages": totalPages,
			// Leftover remainings created before `start`, still due — dollars
			// on the wire (cents/100), same money contract as the rows.
			"carried_over":        carried.Count,
			"carried_over_amount": float64(carried.AmountCents) / 100.0,
		},
	})
}
