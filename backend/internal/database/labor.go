package database

import (
	"fmt"
	"time"
)

// PayrollRunCost is a narrow projection of a paid payroll run for labor-cost
// proration. GrossTotal+BonusTotal is the operator's labor outflow (deductions
// are withheld from gross, not an operator saving, so they are not subtracted).
type PayrollRunCost struct {
	ID          uint
	PeriodStart time.Time
	PeriodEnd   time.Time
	GrossTotal  int64
	BonusTotal  int64
}

// GetPaidPayrollRunsOverlapping returns PAID runs whose work period
// [PeriodStart,PeriodEnd) overlaps [start,end). ONE projected query — no
// LineItems Preload, no SELECT *. Windowing is on the work period (labor
// incurred), distinct from ListPayrollRuns which windows paid runs by paid_at.
func (db *DB) GetPaidPayrollRunsOverlapping(businessID uint, start, end time.Time) ([]PayrollRunCost, error) {
	var rows []PayrollRunCost
	if err := db.GetGorm().
		Model(&PayrollRun{}).
		Where("business_id = ? AND status = ? AND period_end > ? AND period_start < ?",
			businessID, PayrollRunStatusPaid, start.UTC(), end.UTC()).
		Select("id, period_start, period_end, gross_total, bonus_total").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("GetPaidPayrollRunsOverlapping businessID=%d: %w", businessID, err)
	}
	return rows, nil
}
