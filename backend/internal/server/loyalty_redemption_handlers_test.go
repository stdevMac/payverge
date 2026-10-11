package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnqueueLoyaltyPaidReceipt_PaidBillQueuesExactlyOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf(
		"file:loyalty-paid-receipt-%d?mode=memory&cache=shared", time.Now().UnixNano(),
	)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Printer{},
		&database.PrintJob{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
	))
	business := database.Business{BusinessId: "loyalty-receipt", Name: "Loyalty Receipt"}
	require.NoError(t, db.Create(&business).Error)
	bill := database.Bill{
		BusinessID: business.ID, BillNumber: "LOYALTY-PAID-RECEIPT",
		Status: database.BillStatusPaid, TotalAmount: 2000, PaidAmount: 2000,
	}
	require.NoError(t, db.Create(&bill).Error)

	enqueueLoyaltyPaidReceipt(db, bill.ID)
	enqueueLoyaltyPaidReceipt(db, bill.ID)

	var count int64
	require.NoError(t, db.Model(&database.PrintJob{}).
		Where("business_id = ? AND kind = ? AND source_type = ? AND source_id = ?",
			business.ID, database.PrintJobKindReceipt, "bill", bill.ID).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}
