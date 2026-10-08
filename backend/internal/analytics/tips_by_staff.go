package analytics

import (
	"fmt"
	"time"
)

// StaffTipRow is one row of the tips-by-staff rollup. Attribution rule:
// prefer CreatedByStaffID (the opener owns the covers) over ClosedByStaffID.
// Kitchen staff are never credited — closing a check from the line is not tip
// ownership. Bills that resolve to no eligible staff land in the unattributed
// bucket (nil StaffID, empty StaffName). Money: tip_amount is summed in CENTS
// in SQL and converted once here (money wire contract).
//
// Tip pooling / tip-out is not modeled; this is an operational snapshot, not
// payroll.
type StaffTipRow struct {
	StaffID   *uint   `json:"staff_id"`
	StaffName string  `json:"staff_name"`
	TotalTips float64 `json:"total_tips"`
	BillCount int64   `json:"bill_count"`
}

type staffTipScanRow struct {
	StaffID   *uint
	StaffName string
	TipCents  int64
	BillCount int64
}

// GetTipsByStaff aggregates settled-bill tips per attributed staff member for
// a period, in one grouped query (LEFT JOIN staff for names; no per-row
// reloads).
func (s *AnalyticsService) GetTipsByStaff(businessID uint, period string, loc *time.Location) ([]StaffTipRow, error) {
	loc = locOrUTC(loc)
	startDate, endDate, err := s.parsePeriod(period, loc)
	if err != nil {
		return nil, fmt.Errorf("invalid period: %w", err)
	}

	recognizedSQL, args := s.recognizedPaymentBillAmountsSubquerySQL(businessID, startDate, endDate)
	recognizedCTE := s.recognizedEventsCTE(recognizedSQL)
	// Prefer the opener (covers owner). Fall back to the closer only when the
	// opener is missing. Never credit kitchen — BOH closing a check must not
	// silently appear on a tip leaderboard.
	query := fmt.Sprintf(`
		WITH %s,
		staff_bill_tips AS (
			SELECT bill_id, SUM(tip_cents) AS tip_cents
			FROM recognized_events
			GROUP BY bill_id
		),
		attributed AS (
			SELECT
				staff_bill_tips.tip_cents AS tip_cents,
				CASE
					WHEN creator.id IS NOT NULL AND LOWER(COALESCE(creator.role, '')) <> 'kitchen'
						THEN creator.id
					WHEN closer.id IS NOT NULL AND LOWER(COALESCE(closer.role, '')) <> 'kitchen'
						THEN closer.id
					ELSE NULL
				END AS staff_id,
				CASE
					WHEN creator.id IS NOT NULL AND LOWER(COALESCE(creator.role, '')) <> 'kitchen'
						THEN creator.name
					WHEN closer.id IS NOT NULL AND LOWER(COALESCE(closer.role, '')) <> 'kitchen'
						THEN closer.name
					ELSE ''
				END AS staff_name
			FROM staff_bill_tips
			JOIN bills ON bills.id = staff_bill_tips.bill_id
			LEFT JOIN staff AS creator ON creator.id = bills.created_by_staff_id
			LEFT JOIN staff AS closer ON closer.id = bills.closed_by_staff_id
		)
		SELECT
			staff_id,
			COALESCE(MAX(staff_name), '') AS staff_name,
			SUM(tip_cents) AS tip_cents,
			COUNT(CASE WHEN tip_cents > 0 THEN 1 END) AS bill_count
		FROM attributed
		GROUP BY staff_id
		HAVING SUM(tip_cents) <> 0
		ORDER BY tip_cents DESC
	`, recognizedCTE)
	var scanned []staffTipScanRow
	err = s.db.GetGorm().Raw(query, args...).Scan(&scanned).Error
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate tips by staff: %w", err)
	}

	rows := make([]StaffTipRow, 0, len(scanned))
	for _, r := range scanned {
		rows = append(rows, StaffTipRow{
			StaffID:   r.StaffID,
			StaffName: r.StaffName,
			TotalTips: centsToDollars(r.TipCents),
			BillCount: r.BillCount,
		})
	}
	return rows, nil
}
