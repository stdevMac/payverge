package database

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

func setupMilestoneEventTestDB(t *testing.T) *DB {
	t.Helper()
	return setupMilestoneEventTestDBWithLogger(t, nil)
}

func setupMilestoneEventTestDBWithLogger(t *testing.T, gormLogger logger.Interface) *DB {
	t.Helper()

	config := &gorm.Config{}
	if gormLogger != nil {
		config.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), config)
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, gormDB.AutoMigrate(
		&Business{},
		&Bill{},
		&Payment{},
		&AlternativePayment{},
		&BusinessMilestoneEvent{},
		&BusinessRevenueAggregate{},
	))
	SetTestDB(gormDB)
	return GetDBWrapper()
}

func TestApplyConfirmedPaymentRecordsRevenueMilestoneForPartialPayment(t *testing.T) {
	db := setupMilestoneEventTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	previousBill := createPaymentLedgerBill(t, db, business.ID, "milestone-previous", 99000, 99000, 0, BillStatusPaid, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour), nil)
	previousBill.ClosedAt = &previousBill.UpdatedAt
	require.NoError(t, db.GetGorm().Save(previousBill).Error)
	currentBill := createPaymentLedgerBill(t, db, business.ID, "milestone-partial", 20000, 0, 0, BillStatusOpen, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour), nil)

	updatedBill, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID:        currentBill.ID,
		PayerAddr:     "0xmilestone",
		Amount:        2000,
		TxHash:        "milestone_partial_crossing",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, BillStatusPartial, updatedBill.Status)

	var events []BusinessMilestoneEvent
	require.NoError(t, db.GetGorm().Where("business_id = ?", business.ID).Find(&events).Error)
	require.Len(t, events, 1)
	assert.Equal(t, BusinessMilestoneTypeRevenue, events[0].MilestoneType)
	assert.Equal(t, int64(100000), events[0].ThresholdCents)
	assert.Equal(t, BusinessMilestoneStatusPending, events[0].Status)
	require.NotNil(t, events[0].BillID)
	assert.Equal(t, currentBill.ID, *events[0].BillID)
}

func TestApplyConfirmedPaymentRecordsFirstOrderMilestoneInPaymentTransaction(t *testing.T) {
	db := setupMilestoneEventTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	bill := createPaymentLedgerBill(t, db, business.ID, "milestone-first-order", 1000, 0, 0, BillStatusOpen, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour), nil)

	updatedBill, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "0xfirst",
		Amount:        1000,
		TxHash:        "milestone_first_order",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, BillStatusPaid, updatedBill.Status)

	var event BusinessMilestoneEvent
	require.NoError(t, db.GetGorm().Where("business_id = ? AND milestone_type = ?", business.ID, BusinessMilestoneTypeFirstOrder).First(&event).Error)
	assert.Equal(t, int64(0), event.ThresholdCents)
	assert.Equal(t, BusinessMilestoneStatusPending, event.Status)
	require.NotNil(t, event.BillID)
	assert.Equal(t, bill.ID, *event.BillID)
}

func TestPaymentMilestoneRecordsAreIdempotent(t *testing.T) {
	db := setupMilestoneEventTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	bill := createPaymentLedgerBill(t, db, business.ID, "milestone-idempotent", 100000, 100000, 0, BillStatusPaid, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour), nil)

	require.NoError(t, RecordPaymentMilestonesTx(db.GetGorm(), business.ID, bill.ID, 100000, 0, 0, BillStatusPaid))
	require.NoError(t, RecordPaymentMilestonesTx(db.GetGorm(), business.ID, bill.ID, 100000, 0, 0, BillStatusPaid))

	var count int64
	require.NoError(t, db.GetGorm().Model(&BusinessMilestoneEvent{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Equal(t, int64(2), count, "one revenue event and one first-order event should be present")
}

func TestRecordPaymentMilestonesTxUsesNetRecognizedRevenue(t *testing.T) {
	db := setupMilestoneEventTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	confirmedAt := time.Date(2026, time.May, 11, 12, 0, 0, 0, time.UTC)
	reversedAt := confirmedAt.Add(time.Hour)
	bill := createPaymentLedgerBill(t, db, business.ID, "milestone-tx-reversed", 100000, 100000, 0, BillStatusPartial, confirmedAt.Add(-time.Hour), reversedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xmilestone_tx_reversed",
		Amount:        100000,
		TxHash:        "milestone_tx_reversed",
		Status:        PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	require.NoError(t, db.GetGorm().Transaction(func(tx *gorm.DB) error {
		return RecordPaymentMilestonesTx(tx, business.ID, bill.ID, 100000, 0, 0, BillStatusPartial)
	}))

	var count int64
	require.NoError(t, db.GetGorm().Model(&BusinessMilestoneEvent{}).
		Where("business_id = ? AND milestone_type = ?", business.ID, BusinessMilestoneTypeRevenue).
		Count(&count).Error)
	assert.Zero(t, count)
}

func TestRecordPaymentMilestonesLocksBusinessBeforeAggregating(t *testing.T) {
	db := setupMilestoneEventTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	bill := createPaymentLedgerBill(t, db, business.ID, "milestone-lock", 1000, 1000, 0, BillStatusPaid, time.Now().Add(-time.Hour), time.Now(), nil)

	var businessQueryObserved atomic.Bool
	var businessQueryLocked atomic.Bool
	const cbName = "payverge:test:capture_milestone_business_lock"
	require.NoError(t, db.GetGorm().Callback().Query().Before("gorm:query").Register(cbName, func(tx *gorm.DB) {
		if businessQueryObserved.Load() {
			return
		}
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "businesses" {
			return
		}
		businessQueryObserved.Store(true)
		if locking, ok := tx.Statement.Clauses["FOR"]; ok {
			if l, ok := locking.Expression.(clause.Locking); ok && l.Strength == "UPDATE" {
				businessQueryLocked.Store(true)
			}
		}
	}))
	t.Cleanup(func() {
		_ = db.GetGorm().Callback().Query().Remove(cbName)
	})

	require.NoError(t, RecordPaymentMilestonesTx(db.GetGorm(), business.ID, bill.ID, 1000, 0, 0, BillStatusPaid))
	require.True(t, businessQueryObserved.Load(), "expected milestone recording to query the business row")
	assert.True(t, businessQueryLocked.Load(), "milestone revenue aggregation must lock the business row before summing recognized payments")
}

func TestRecordPaymentMilestonesUsesRevenueAggregateWithoutHistoryScan(t *testing.T) {
	queryLogger := &recordingPaymentLedgerLogger{}
	db := setupMilestoneEventTestDBWithLogger(t, queryLogger)
	business := createPaymentLedgerBusiness(t, db)
	bill := createPaymentLedgerBill(t, db, business.ID, "milestone-aggregate-fast", 100000, 100000, 0, BillStatusPaid, time.Now().Add(-time.Hour), time.Now(), nil)
	require.NoError(t, db.GetGorm().Create(&BusinessRevenueAggregate{
		BusinessID:         business.ID,
		NetRevenueCents:    99000,
		GrossRevenueCents:  99000,
		PositiveEventCount: 1,
	}).Error)

	queryLogger.Reset()
	require.NoError(t, RecordPaymentMilestonesTx(db.GetGorm(), business.ID, bill.ID, 1000, 0, 1, BillStatusPaid))
	sql := queryLogger.SQL()

	assert.NotContains(t, sql, "FROM payments", "steady-state milestone recording should not rescan payment history")
	assert.NotContains(t, sql, "FROM alternative_payments", "steady-state milestone recording should not check alternative payment history")

	var aggregate BusinessRevenueAggregate
	require.NoError(t, db.GetGorm().Where("business_id = ?", business.ID).First(&aggregate).Error)
	assert.Equal(t, int64(100000), aggregate.NetRevenueCents)
	assert.Equal(t, int64(100000), aggregate.GrossRevenueCents)
	assert.Equal(t, int64(2), aggregate.PositiveEventCount)
	assert.Equal(t, int64(1), aggregate.RecognizedBillCount)
}

func TestCreateBusinessInitializesRevenueAggregate(t *testing.T) {
	db := setupMilestoneEventTestDB(t)

	business := &Business{Name: "Aggregate New Business"}
	require.NoError(t, CreateBusiness(business))

	var aggregate BusinessRevenueAggregate
	require.NoError(t, db.GetGorm().Where("business_id = ?", business.ID).First(&aggregate).Error)
	assert.Zero(t, aggregate.NetRevenueCents)
	assert.Zero(t, aggregate.RecognizedBillCount)
}

func TestClaimPendingBusinessMilestoneEventsClaimsOnce(t *testing.T) {
	db := setupMilestoneEventTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	event := BusinessMilestoneEvent{
		BusinessID:     business.ID,
		MilestoneType:  BusinessMilestoneTypeRevenue,
		ThresholdCents: 100000,
		Status:         BusinessMilestoneStatusPending,
	}
	require.NoError(t, db.GetGorm().Create(&event).Error)

	claimed, err := ClaimPendingBusinessMilestoneEvents(business.ID, 25)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, BusinessMilestoneStatusProcessing, claimed[0].Status)
	assert.NotEmpty(t, claimed[0].ProcessingToken)

	claimedAgain, err := ClaimPendingBusinessMilestoneEvents(business.ID, 25)
	require.NoError(t, err)
	assert.Empty(t, claimedAgain)

	require.NoError(t, MarkBusinessMilestoneEventSent(event.ID, "stale-token"))
	var staleTokenReload BusinessMilestoneEvent
	require.NoError(t, db.GetGorm().First(&staleTokenReload, event.ID).Error)
	assert.Equal(t, BusinessMilestoneStatusProcessing, staleTokenReload.Status)

	require.NoError(t, MarkBusinessMilestoneEventSent(claimed[0].ID, claimed[0].ProcessingToken))
	var sent BusinessMilestoneEvent
	require.NoError(t, db.GetGorm().First(&sent, event.ID).Error)
	assert.Equal(t, BusinessMilestoneStatusSent, sent.Status)
	assert.Empty(t, sent.ProcessingToken)
	assert.NotNil(t, sent.SentAt)
}

func TestStaleProcessingBusinessMilestoneEventsAreClaimable(t *testing.T) {
	db := setupMilestoneEventTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	staleUpdatedAt := time.Now().Add(-20 * time.Minute)
	event := BusinessMilestoneEvent{
		BusinessID:      business.ID,
		MilestoneType:   BusinessMilestoneTypeRevenue,
		ThresholdCents:  100000,
		Status:          BusinessMilestoneStatusProcessing,
		ProcessingToken: "stale-token",
		CreatedAt:       staleUpdatedAt,
		UpdatedAt:       staleUpdatedAt,
	}
	require.NoError(t, db.GetGorm().Create(&event).Error)

	businessIDs, err := ListBusinessesWithPendingMilestoneEvents(100)
	require.NoError(t, err)
	assert.Contains(t, businessIDs, business.ID)

	claimed, err := ClaimPendingBusinessMilestoneEvents(business.ID, 25)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, BusinessMilestoneStatusProcessing, claimed[0].Status)
	assert.NotEqual(t, "stale-token", claimed[0].ProcessingToken)
	assert.NotEmpty(t, claimed[0].ProcessingToken)
}
