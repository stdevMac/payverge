package jobs

import (
	"github.com/stdevmac/payverge/backend/internal/crm"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"gorm.io/gorm"
)

const defaultCRMSettlementReconciliationLimit = 100

// ReconcileCRMSettlementVisits backfills missing CRM visits for already-paid bills.
func ReconcileCRMSettlementVisits(db *gorm.DB, limit int) {
	if db == nil {
		logger.Logger.Warn("CRM settlement reconciliation skipped: database is nil")
		return
	}
	if limit <= 0 {
		limit = defaultCRMSettlementReconciliationLimit
	}

	var bills []database.Bill
	if err := db.Model(&database.Bill{}).
		Select("bills.*").
		Joins("JOIN customer_businesses cb ON cb.customer_id = bills.crm_customer_id AND cb.business_id = bills.business_id AND cb.is_active = ?", true).
		Joins("LEFT JOIN customer_visits cv ON cv.bill_id = bills.id").
		Where("bills.status = ? AND bills.crm_customer_id IS NOT NULL AND cv.id IS NULL", database.BillStatusPaid).
		Order("bills.updated_at ASC, bills.id ASC").
		Limit(limit).
		Find(&bills).Error; err != nil {
		logger.Logger.Warnf("Failed to query CRM settlement reconciliation bills: %v", err)
		return
	}

	service := crm.NewService(db)
	for _, bill := range bills {
		if err := service.RecordBillSettlementVisit(bill.ID); err != nil {
			logger.Logger.Warnf("Failed to reconcile CRM settlement visit for bill %d: %v", bill.ID, err)
		}
	}
}
