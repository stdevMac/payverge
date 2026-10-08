package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestEnqueueFiscalJobFailureRaisesOperatorAlert locks Task 13b: when
// HandleBillPaid fails at payment time (job queue write), the operator must
// see a fiscal_issue_failed alert rather than a log-only drop.
//
// HandleBillPaid swallows bill-not-found (returns nil), so the ghost-bill trick
// cannot force the error path. Instead we seed a paid bill + automatic fiscal
// settings so enqueue reaches CreateJobIfNotExists, then drop fiscal_jobs so
// the create fails deterministically.
func TestEnqueueFiscalJobFailureRaisesOperatorAlert(t *testing.T) {
	db := setupPaymentRegressionDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.OperationalAlert{}, &database.OperationalAlertEvent{}))

	business := createPaymentRegressionBusiness(t, "fiscal-alert", nil, "fiscal-alert@test.local")
	seedPaymentRegressionFiscalSettings(t, business.ID)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "GHOST-FISCAL-1",
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 100,
		PaidAmount:  100,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	// Force CreateJobIfNotExists (and thus HandleBillPaid) to error.
	require.NoError(t, db.Exec("DROP TABLE fiscal_jobs").Error)

	enqueueFiscalJobForPaidBill(bill, nil, "test")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("business_id = ? AND alert_type = ?", business.ID, database.OperationalAlertTypeFiscalIssueFailed).
		Count(&count).Error)
	require.EqualValues(t, 1, count, "fiscal enqueue failure must raise an operator alert")
}
