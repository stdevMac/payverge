package database

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBillHasOptionalCRMCustomerRelation(t *testing.T) {
	customerID := uint(42)
	bill := Bill{CRMCustomerID: &customerID}

	require.NotNil(t, bill.CRMCustomerID)
	require.Equal(t, uint(42), *bill.CRMCustomerID)
}

func TestAttachCRMCustomerIDToBillIfEmptyAttachesWhenEmpty(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)
	bill := helperBill(t, business, nil, 0)
	customerID := uint(42)

	updated, err := AttachCRMCustomerIDToBillIfEmpty(bill.ID, customerID)

	require.NoError(t, err)
	require.NotNil(t, updated.CRMCustomerID)
	require.Equal(t, customerID, *updated.CRMCustomerID)

	var saved Bill
	require.NoError(t, db.First(&saved, bill.ID).Error)
	require.NotNil(t, saved.CRMCustomerID)
	require.Equal(t, customerID, *saved.CRMCustomerID)
}

func TestAttachCRMCustomerIDToBillIfEmptyUsesNarrowBillProjection(t *testing.T) {
	recorder := &crmBillSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupCRMBillTestDBWithLogger(t, recorder)
	business := helperBusiness(t, 0, 0)
	bill := helperBill(t, business, nil, 0)
	customerID := uint(42)

	recorder.statements = nil
	updated, err := AttachCRMCustomerIDToBillIfEmpty(bill.ID, customerID)

	require.NoError(t, err)
	require.Equal(t, bill.ID, updated.BillID)
	require.NotNil(t, updated.CRMCustomerID)
	require.Equal(t, customerID, *updated.CRMCustomerID)
	require.Zero(t, recorder.selectStarCount("bills"), "crm attachment should lock only id/status/crm_customer_id")

	var saved Bill
	require.NoError(t, db.First(&saved, bill.ID).Error)
	require.NotNil(t, saved.CRMCustomerID)
	require.Equal(t, customerID, *saved.CRMCustomerID)
}

func TestAttachCRMCustomerIDToBillIfEmptyIsIdempotentForSameCustomer(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)
	customerID := uint(42)
	bill := helperBill(t, business, nil, 0)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Update("crm_customer_id", customerID).Error)

	updated, err := AttachCRMCustomerIDToBillIfEmpty(bill.ID, customerID)

	require.NoError(t, err)
	require.NotNil(t, updated.CRMCustomerID)
	require.Equal(t, customerID, *updated.CRMCustomerID)
}

func TestAttachCRMCustomerIDToBillIfEmptyRejectsDifferentCustomer(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)
	existingCustomerID := uint(42)
	attemptingCustomerID := uint(99)
	bill := helperBill(t, business, nil, 0)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Update("crm_customer_id", existingCustomerID).Error)

	updated, err := AttachCRMCustomerIDToBillIfEmpty(bill.ID, attemptingCustomerID)

	require.ErrorIs(t, err, ErrBillCRMCustomerConflict)
	require.Nil(t, updated)

	var saved Bill
	require.NoError(t, db.First(&saved, bill.ID).Error)
	require.NotNil(t, saved.CRMCustomerID)
	require.Equal(t, existingCustomerID, *saved.CRMCustomerID)
}

func TestAttachCRMCustomerIDToBillIfEmptyRejectsZeroCustomerID(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)
	bill := helperBill(t, business, nil, 0)

	updated, err := AttachCRMCustomerIDToBillIfEmpty(bill.ID, 0)

	require.ErrorIs(t, err, ErrInvalidCRMCustomerID)
	require.Nil(t, updated)

	var saved Bill
	require.NoError(t, db.First(&saved, bill.ID).Error)
	require.Nil(t, saved.CRMCustomerID)
}

func TestAttachCRMCustomerIDToBillIfEmptyRejectsInactiveBill(t *testing.T) {
	for _, status := range []BillStatus{BillStatusPaid, BillStatusClosed} {
		t.Run(string(status), func(t *testing.T) {
			setupOrderTestDB(t)
			business := helperBusiness(t, 0, 0)
			bill := helperBill(t, business, nil, 0)
			require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Update("status", status).Error)

			updated, err := AttachCRMCustomerIDToBillIfEmpty(bill.ID, 42)

			require.ErrorIs(t, err, ErrBillNotOpen)
			require.Nil(t, updated)

			var saved Bill
			require.NoError(t, db.First(&saved, bill.ID).Error)
			require.Nil(t, saved.CRMCustomerID)
			require.Equal(t, status, saved.Status)
		})
	}
}

type crmBillSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *crmBillSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *crmBillSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") || strings.Contains(normalized, "from \""+table+"\"") {
			count++
		}
	}
	return count
}

func setupCRMBillTestDBWithLogger(t *testing.T, gormLogger logger.Interface) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: gormLogger})
	require.NoError(t, err, "open in-memory database")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	db = gormDB
	require.NoError(t, db.AutoMigrate(
		&Business{},
		&Bill{},
	))
}
