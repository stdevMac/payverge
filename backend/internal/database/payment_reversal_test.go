package database

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupReversalTestDB creates an in-memory SQLite DB with the tables needed for
// payment-reversal tests.
func setupReversalTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	db = gormDB

	require.NoError(t, db.AutoMigrate(
		&Business{},
		&Bill{},
		&Payment{},
		&AlternativePayment{},
		&BillSplitShare{},
		&BusinessMilestoneEvent{},
		&BusinessRevenueAggregate{},
		&BillHistoryEvent{},
		&CashRegisterSession{},
		&CashRegisterMovement{},
	))
}

func setupReversalTestDBWithLogger(t *testing.T, gormLogger logger.Interface) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: gormLogger})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	db = gormDB

	require.NoError(t, db.AutoMigrate(
		&Business{},
		&Bill{},
		&Payment{},
		&AlternativePayment{},
		&BillSplitShare{},
		&BusinessMilestoneEvent{},
		&BusinessRevenueAggregate{},
		&BillHistoryEvent{},
		&CashRegisterSession{},
		&CashRegisterMovement{},
	))
}

type paymentSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *paymentSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func TestGetBillBusinessAccessByBillIDReturnsBusinessAccessFields(t *testing.T) {
	setupReversalTestDB(t)
	userID := uint(42)
	biz := helperBusiness(t, 0, 0)
	require.NoError(t, db.Model(&Business{}).Where("id = ?", biz.ID).Updates(map[string]interface{}{
		"owner_address": "0xOwner",
		"user_id":       &userID,
	}).Error)
	bill := helperBill(t, biz, nil, 100)

	business, err := GetBillBusinessAccessByBillID(bill.ID)

	require.NoError(t, err)
	require.Equal(t, biz.ID, business.ID)
	require.Equal(t, "0xOwner", business.OwnerAddress)
	require.NotNil(t, business.UserID)
	require.Equal(t, userID, *business.UserID)
}

func TestGetBillBusinessAccessByBillIDProjectsDemoAccessFields(t *testing.T) {
	setupReversalTestDB(t)
	demoOwnerID := uint(77)
	biz := helperBusiness(t, 0, 0)
	require.NoError(t, db.Model(&Business{}).Where("id = ?", biz.ID).Updates(map[string]interface{}{
		"is_demo":            true,
		"kind":               BusinessKindDemo,
		"demo_owner_user_id": demoOwnerID,
	}).Error)
	bill := helperBill(t, biz, nil, 100)

	business, err := GetBillBusinessAccessByBillID(bill.ID)
	require.NoError(t, err)
	require.True(t, business.IsDemo)
	require.Equal(t, BusinessKindDemo, business.Kind)
	require.NotNil(t, business.DemoOwnerUserID)
	require.Equal(t, demoOwnerID, *business.DemoOwnerUserID)
}

func TestGetBillBusinessAccessByBillIDUsesDashboardDecisionColumns(t *testing.T) {
	recorder := &paymentSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupReversalTestDBWithLogger(t, recorder)
	demoOwnerID := uint(77)
	biz := helperBusiness(t, 0, 0)
	require.NoError(t, db.Model(&Business{}).Where("id = ?", biz.ID).Updates(map[string]interface{}{
		"is_demo":            true,
		"kind":               BusinessKindDemo,
		"demo_owner_user_id": demoOwnerID,
	}).Error)
	bill := helperBill(t, biz, nil, 100)

	recorder.statements = nil
	business, err := GetBillBusinessAccessByBillID(bill.ID)
	require.NoError(t, err)
	require.True(t, business.IsDemo)
	require.Equal(t, BusinessKindDemo, business.Kind)
	require.Equal(t, biz.ID, business.ID)

	joined := strings.ToLower(strings.Join(recorder.statements, "\n"))
	require.Contains(t, joined, "select business_id from bills")
}

func TestGetBillBusinessAccessByBillIDDoesNotUseBillPrimaryKeyAsBusinessID(t *testing.T) {
	setupReversalTestDB(t)
	demoOwnerID := uint(77)
	biz := helperBusiness(t, 0, 0)
	for i := 0; i < 5; i++ {
		_ = helperBill(t, biz, nil, 10)
	}
	require.NoError(t, db.Model(&Business{}).Where("id = ?", biz.ID).Updates(map[string]interface{}{
		"is_demo":            true,
		"kind":               BusinessKindDemo,
		"demo_owner_user_id": demoOwnerID,
	}).Error)
	bill := helperBill(t, biz, nil, 100)
	require.NotEqual(t, bill.ID, biz.ID)

	business, err := GetBillBusinessAccessByBillID(bill.ID)
	require.NoError(t, err)
	require.Equal(t, biz.ID, business.ID)
	require.True(t, business.IsDemo)
}

// helperPayment inserts a Payment record and returns it.
func helperPayment(t *testing.T, bill *Bill, amount, tipAmount float64, txHash string, status PaymentStatus) *Payment {
	t.Helper()
	p := &Payment{
		BillID:        bill.ID,
		PayerAddr:     "test-payer",
		Amount:        int64(math.Round(amount * 100)),
		TipAmount:     int64(math.Round(tipAmount * 100)),
		TxHash:        txHash,
		Status:        status,
		PaymentMethod: "plugin",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(p).Error)
	return p
}

// ─── ReversePluginPayment ───

func TestReversePluginPayment_NotFound_IsNoop(t *testing.T) {
	setupReversalTestDB(t)
	// A txHash that doesn't exist should return nil (idempotent).
	err := ReversePluginPayment("nonexistent_hash")
	assert.NoError(t, err)
}

func TestReversePluginPayment_EmptyHash_ReturnsError(t *testing.T) {
	setupReversalTestDB(t)
	err := ReversePluginPayment("")
	assert.Error(t, err)
}

func TestReversePluginPayment_AlreadyReversed_IsNoop(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)
	// Manually set amounts as though a payment was already applied then reversed.
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Update("paid_amount", 0).Error)

	p := helperPayment(t, bill, 50, 5, "plugin_already_reversed", PaymentStatusReversed)

	err := ReversePluginPayment(p.TxHash)
	assert.NoError(t, err)

	// Bill should be unchanged (paid_amount stays 0).
	var refreshedBill Bill
	require.NoError(t, db.First(&refreshedBill, bill.ID).Error)
	assert.InDelta(t, 0.0, refreshedBill.PaidAmount, 0.01)
}

// TestReversePluginPayment_SkipsRefundPending locks F-REFUNDWEBHOOK-RACE: while
// the manual refund flow holds a payment in refund_pending (it has called the
// PSP and will finalize via RefundBillPayment), an inbound provider 'refunded'
// webhook must be a no-op here. Otherwise the webhook reverses the row and the
// manual finalize then 409s the operator after the PSP already moved the money
// (and the audit/credit-note enrichment is skipped).
func TestReversePluginPayment_SkipsRefundPending(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"paid_amount": 5000,
		"status":      BillStatusPaid,
	}).Error)

	p := helperPayment(t, bill, 50, 0, "plugin_refund_pending", PaymentStatusRefundPending)

	require.NoError(t, ReversePluginPayment(p.TxHash))

	var refreshedBill Bill
	require.NoError(t, db.First(&refreshedBill, bill.ID).Error)
	assert.EqualValues(t, 5000, refreshedBill.PaidAmount, "refund_pending must not be reversed by the webhook")
	assert.Equal(t, BillStatusPaid, refreshedBill.Status)

	var refreshedPayment Payment
	require.NoError(t, db.First(&refreshedPayment, p.ID).Error)
	assert.Equal(t, PaymentStatusRefundPending, refreshedPayment.Status, "the manual flow still owns the reversal")
}

func TestReversePluginPayment_ReversesPaidAmounts(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)

	// Simulate that 60 (+ 5 tip) has been applied to the bill.
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"paid_amount": 6000,
		"tip_amount":  500,
		"status":      BillStatusPartial,
	}).Error)

	p := helperPayment(t, bill, 60, 5, "plugin_abc123", PaymentStatusConfirmed)

	err := ReversePluginPayment(p.TxHash)
	require.NoError(t, err)

	var refreshedBill Bill
	require.NoError(t, db.First(&refreshedBill, bill.ID).Error)
	assert.InDelta(t, 0.0, refreshedBill.PaidAmount, 0.01)
	assert.InDelta(t, 0.0, refreshedBill.TipAmount, 0.01)
	assert.Equal(t, BillStatusOpen, refreshedBill.Status)

	var refreshedPayment Payment
	require.NoError(t, db.First(&refreshedPayment, p.ID).Error)
	assert.Equal(t, PaymentStatusReversed, refreshedPayment.Status)

	var aggregate BusinessRevenueAggregate
	require.NoError(t, db.Where("business_id = ?", biz.ID).First(&aggregate).Error)
	assert.Equal(t, int64(0), aggregate.NetRevenueCents)
	assert.Equal(t, int64(0), aggregate.NetTipCents)
	assert.Equal(t, int64(6000), aggregate.GrossRevenueCents)
	assert.Equal(t, int64(500), aggregate.GrossTipCents)
	assert.Equal(t, int64(1), aggregate.PositiveEventCount)
	assert.Equal(t, int64(0), aggregate.RecognizedBillCount)
}

func TestReversePluginPayment_ReopensPaidBill(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)

	// Bill is fully paid.
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"paid_amount": 10000,
		"status":      BillStatusPaid,
	}).Error)

	p := helperPayment(t, bill, 100, 0, "plugin_fullpay", PaymentStatusConfirmed)

	err := ReversePluginPayment(p.TxHash)
	require.NoError(t, err)

	var refreshedBill Bill
	require.NoError(t, db.First(&refreshedBill, bill.ID).Error)
	assert.InDelta(t, 0.0, refreshedBill.PaidAmount, 0.01)
	assert.Equal(t, BillStatusOpen, refreshedBill.Status)
}

func TestReversePluginPaymentSetsReversedAt(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"paid_amount": 10000,
		"status":      BillStatusPaid,
	}).Error)
	payment := helperPayment(t, bill, 100, 0, "plugin_reverse_timestamp", PaymentStatusConfirmed)

	require.NoError(t, ReversePluginPayment(payment.TxHash))

	var reloaded Payment
	require.NoError(t, db.First(&reloaded, payment.ID).Error)
	require.Equal(t, PaymentStatusReversed, reloaded.Status)
	require.NotNil(t, reloaded.ReversedAt)
}

func TestReversePluginPayment_PaidMultiPaymentBillBecomesPartialAndClearsClosure(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)
	closedAt := time.Now().Add(-5 * time.Minute)
	closedByStaffID := uint(42)

	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"paid_amount":        10000,
		"status":             BillStatusPaid,
		"closed_at":          &closedAt,
		"closed_by_staff_id": closedByStaffID,
	}).Error)
	helperPayment(t, bill, 40, 0, "plugin_partial_remaining", PaymentStatusConfirmed)
	reversed := helperPayment(t, bill, 60, 0, "plugin_reversed_from_full", PaymentStatusConfirmed)

	require.NoError(t, ReversePluginPayment(reversed.TxHash))

	var refreshedBill Bill
	require.NoError(t, db.First(&refreshedBill, bill.ID).Error)
	require.Equal(t, int64(4000), refreshedBill.PaidAmount)
	require.Equal(t, BillStatusPartial, refreshedBill.Status)
	require.Nil(t, refreshedBill.ClosedAt)
	require.Nil(t, refreshedBill.ClosedByStaffID)
}

func TestApplyConfirmedPaymentTxHashConflictForAnotherBill(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	firstBill := helperBill(t, biz, nil, 100)
	secondBill := &Bill{
		BusinessID:  biz.ID,
		BillNumber:  "B-shared-tx-conflict",
		Status:      BillStatusOpen,
		Items:       "[]",
		TotalAmount: 10000,
	}
	require.NoError(t, db.Create(secondBill).Error)
	helperPayment(t, firstBill, 100, 0, "shared_tx_hash", PaymentStatusConfirmed)

	_, _, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID:        secondBill.ID,
		PayerAddr:     "test-payer",
		Amount:        10000,
		TxHash:        "shared_tx_hash",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}, nil)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrPaymentTxHashConflict))

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, secondBill.ID).Error)
	require.Equal(t, int64(0), reloaded.PaidAmount)
	require.Equal(t, BillStatusOpen, reloaded.Status)
}

// TestApplyConfirmedPayment_PersistsBlockEvidence pins that a confirmed crypto
// settlement carrying block evidence persists block_number/block_hash on the
// Payment row — the durable input the reorg reconciler re-verifies. A settlement
// without a block hash leaves the columns nil (not a reorg candidate).
func TestApplyConfirmedPayment_PersistsBlockEvidence(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)

	bn := int64(555123)
	bh := "0xabc123def"
	_, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "crypto_guest",
		Amount:        10000,
		TxHash:        "0xinboundtx",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		BlockNumber:   &bn,
		BlockHash:     &bh,
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)

	var p Payment
	require.NoError(t, db.Where("tx_hash = ?", "0xinboundtx").First(&p).Error)
	require.NotNil(t, p.BlockHash, "confirmed crypto payment must persist its block hash")
	require.Equal(t, "0xabc123def", *p.BlockHash)
	require.NotNil(t, p.BlockNumber)
	require.Equal(t, int64(555123), *p.BlockNumber)

	// A settlement with no block evidence (manual close / verify-only) is left
	// nil so the reorg sweep never treats it as a candidate.
	bill2 := helperBill(t, biz, nil, 100)
	_, applied2, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID:        bill2.ID,
		PayerAddr:     "cash",
		Amount:        10000,
		TxHash:        "manual-close-1",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "cash",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied2)
	var p2 Payment
	require.NoError(t, db.Where("tx_hash = ?", "manual-close-1").First(&p2).Error)
	require.Nil(t, p2.BlockHash, "non-crypto/verify-only settlement must not record block evidence")
	require.Nil(t, p2.BlockNumber)
}

func TestApplyConfirmedPaymentIdempotentRetryRequiresMatchingPayload(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)

	firstBill, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "test-payer",
		Amount:        4000,
		TipAmount:     500,
		TxHash:        "same_bill_retry_payload",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		SourceChain:   "ethereum",
		SourceToken:   "USDC",
		LifiRouteID:   "route-123",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, int64(4000), firstBill.PaidAmount)

	secondBill, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "test-payer",
		Amount:        4000,
		TipAmount:     500,
		TxHash:        "same_bill_retry_payload",
		Status:        "",
		PaymentMethod: "CRYPTO",
		SourceChain:   "ethereum",
		SourceToken:   "USDC",
		LifiRouteID:   "route-123",
	}, nil)
	require.NoError(t, err)
	require.False(t, applied)
	require.Equal(t, int64(4000), secondBill.PaidAmount)

	var paymentCount int64
	require.NoError(t, db.Model(&Payment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
	require.EqualValues(t, 1, paymentCount)

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.Equal(t, int64(4000), reloaded.PaidAmount)
	require.Equal(t, int64(500), reloaded.TipAmount)
	require.Equal(t, BillStatusPartial, reloaded.Status)
}

func TestApplyConfirmedPaymentTxHashConflictForSameBillMismatchedPayload(t *testing.T) {
	cases := []struct {
		name     string
		mutating func(ConfirmedPaymentInput) ConfirmedPaymentInput
	}{
		{
			name: "amount",
			mutating: func(input ConfirmedPaymentInput) ConfirmedPaymentInput {
				input.Amount = 4500
				return input
			},
		},
		{
			name: "tip",
			mutating: func(input ConfirmedPaymentInput) ConfirmedPaymentInput {
				input.TipAmount = 600
				return input
			},
		},
		{
			name: "status",
			mutating: func(input ConfirmedPaymentInput) ConfirmedPaymentInput {
				input.Status = PaymentStatusPending
				return input
			},
		},
		{
			name: "payment_method",
			mutating: func(input ConfirmedPaymentInput) ConfirmedPaymentInput {
				input.PaymentMethod = "cash"
				return input
			},
		},
		{
			name: "payer_addr",
			mutating: func(input ConfirmedPaymentInput) ConfirmedPaymentInput {
				input.PayerAddr = "other-payer"
				return input
			},
		},
		{
			name: "source_chain",
			mutating: func(input ConfirmedPaymentInput) ConfirmedPaymentInput {
				input.SourceChain = "polygon"
				return input
			},
		},
		{
			name: "source_token",
			mutating: func(input ConfirmedPaymentInput) ConfirmedPaymentInput {
				input.SourceToken = "USDT"
				return input
			},
		},
		{
			name: "settlement_chain",
			mutating: func(input ConfirmedPaymentInput) ConfirmedPaymentInput {
				input.SettlementChain = "ethereum"
				return input
			},
		},
		{
			name: "lifi_route_id",
			mutating: func(input ConfirmedPaymentInput) ConfirmedPaymentInput {
				input.LifiRouteID = "route-other"
				return input
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupReversalTestDB(t)
			biz := helperBusiness(t, 0, 0)
			bill := helperBill(t, biz, nil, 100)
			baseInput := ConfirmedPaymentInput{
				BillID:        bill.ID,
				PayerAddr:     "test-payer",
				Amount:        4000,
				TipAmount:     500,
				TxHash:        "same_bill_mismatch_" + tc.name,
				Status:        PaymentStatusConfirmed,
				PaymentMethod: "crypto",
				SourceChain:   "ethereum",
				SourceToken:   "USDC",
				LifiRouteID:   "route-123",
			}

			_, applied, err := ApplyConfirmedPayment(baseInput, nil)
			require.NoError(t, err)
			require.True(t, applied)

			_, applied, err = ApplyConfirmedPayment(tc.mutating(baseInput), nil)
			require.Error(t, err)
			require.False(t, applied)
			require.True(t, errors.Is(err, ErrPaymentTxHashConflict))

			var paymentCount int64
			require.NoError(t, db.Model(&Payment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
			require.EqualValues(t, 1, paymentCount)

			var reloaded Bill
			require.NoError(t, db.First(&reloaded, bill.ID).Error)
			require.Equal(t, int64(4000), reloaded.PaidAmount)
			require.Equal(t, int64(500), reloaded.TipAmount)
			require.Equal(t, BillStatusPartial, reloaded.Status)
		})
	}
}

func TestApplyConfirmedPaymentRejectsNegativeTip(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)

	_, _, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "test-payer",
		Amount:        5000,
		TipAmount:     -100,
		TxHash:        "negative_tip_payment",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}, nil)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidTipAmount))

	var paymentCount int64
	require.NoError(t, db.Model(&Payment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
	require.Zero(t, paymentCount)

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.Equal(t, int64(0), reloaded.PaidAmount)
	require.Equal(t, int64(0), reloaded.TipAmount)
	require.Equal(t, BillStatusOpen, reloaded.Status)
}

func TestApplyConfirmedPaymentGeneratedManualTxHashSkipsPreInsertProbe(t *testing.T) {
	recorder := &paymentSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupReversalTestDBWithLogger(t, recorder)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)

	_, _, err := ApplyConfirmedPayment(ConfirmedPaymentInput{BillID: bill.ID, PayerAddr: "staff_manual", Amount: 10000, Status: PaymentStatusConfirmed, PaymentMethod: "cash"}, nil)
	require.NoError(t, err)

	for _, statement := range recorder.statements {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		if strings.HasPrefix(normalized, "select") &&
			strings.Contains(normalized, "payments") &&
			strings.Contains(normalized, "tx_hash") {
			t.Fatalf("generated manual payment should not pre-query payments.tx_hash, got SQL: %s", statement)
		}
	}
}

func TestTxHashConflictUniqueConstraintDetectionRecognizesGormDuplicatedKey(t *testing.T) {
	require.True(t, isUniqueConstraintError(gorm.ErrDuplicatedKey))
}

func TestApplyConfirmedPaymentGeneratedManualTxHashConflictReturnsError(t *testing.T) {
	setupReversalTestDB(t)
	originalGenerator := newManualPaymentTxHash
	newManualPaymentTxHash = func(uint) string { return "manual_collision" }
	defer func() { newManualPaymentTxHash = originalGenerator }()

	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)
	helperPayment(t, bill, 1, 0, "manual_collision", PaymentStatusConfirmed)

	_, _, err := ApplyConfirmedPayment(ConfirmedPaymentInput{BillID: bill.ID, PayerAddr: "staff_manual", Amount: 10000, Status: PaymentStatusConfirmed, PaymentMethod: "cash"}, nil)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrPaymentTxHashConflict))

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.Equal(t, int64(0), reloaded.PaidAmount)
	require.Equal(t, BillStatusOpen, reloaded.Status)
	require.Empty(t, reloaded.Notes)
}

func TestRefundBillAlternativePaymentCreatesCashRegisterRefundMovementWhenSessionOpen(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	openAlternativePaymentCashRegisterSession(t, biz.ID, 0)
	bill := helperBill(t, biz, nil, 100)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"paid_amount": 5000,
		"status":      BillStatusPartial,
	}).Error)
	payment := helperAlternativePaymentForRefund(t, bill, PaymentMethodCash, 5000, 0)

	_, refunded, err := RefundBillAlternativePayment(bill.ID, payment.ID, "staff:2", "cash returned")

	require.NoError(t, err)
	require.Equal(t, AltPaymentStatusRefunded, refunded.Status)
	movement := requireSingleCashRegisterMovement(t)
	require.Equal(t, CashRegisterMovementTypeCashRefund, movement.MovementType)
	require.Equal(t, int64(5000), movement.AmountCents)
	require.Equal(t, "system:refund", movement.ActorLabel)
	require.NotNil(t, movement.AlternativePaymentID)
	require.Equal(t, payment.ID, *movement.AlternativePaymentID)
	requireCashRegisterSessionTotals(t, biz.ID, 0, 5000)
}

func TestRefundBillAlternativePaymentAllowsCashRefundWhenNoOpenCashRegisterSession(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"paid_amount": 5000,
		"status":      BillStatusPartial,
	}).Error)
	payment := helperAlternativePaymentForRefund(t, bill, PaymentMethodCash, 5000, 0)

	_, refunded, err := RefundBillAlternativePayment(bill.ID, payment.ID, "staff:2", "cash returned")

	require.NoError(t, err)
	require.Equal(t, AltPaymentStatusRefunded, refunded.Status)
	requireCashRegisterMovementCount(t, 0)
}

func TestRefundBillAlternativePaymentDoesNotCreateRefundMovementForNonCashPayment(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	openAlternativePaymentCashRegisterSession(t, biz.ID, 0)
	bill := helperBill(t, biz, nil, 100)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"paid_amount": 5000,
		"status":      BillStatusPartial,
	}).Error)
	payment := helperAlternativePaymentForRefund(t, bill, PaymentMethodCard, 5000, 0)

	_, refunded, err := RefundBillAlternativePayment(bill.ID, payment.ID, "staff:2", "card refunded")

	require.NoError(t, err)
	require.Equal(t, AltPaymentStatusRefunded, refunded.Status)
	requireCashRegisterMovementCount(t, 0)
	requireCashRegisterSessionTotals(t, biz.ID, 0, 0)
}

func helperAlternativePaymentForRefund(t *testing.T, bill *Bill, method AlternativePaymentMethod, billAmountCents int64, tipAmountCents int64) *AlternativePayment {
	t.Helper()
	now := time.Now().UTC()
	payment := &AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "cashier",
		ParticipantName: "Cashier",
		Amount:          billAmountCents,
		BillAmountCents: billAmountCents,
		TipAmountCents:  tipAmountCents,
		PaymentMethod:   method,
		Status:          AltPaymentStatusConfirmed,
		ConfirmedBy:     "staff:2",
		ConfirmedAt:     &now,
	}
	require.NoError(t, db.Create(payment).Error)
	return payment
}

// An operator refund (status=refunded, aggregate already decremented once) must
// make a subsequent provider 'refunded' webhook a NO-OP: ReversePluginPayment
// must not decrement the revenue aggregate a second time.
func TestReversePluginPayment_RefundedIsIdempotentAgainstAggregate(t *testing.T) {
	setupReversalTestDB(t)

	biz := Business{}
	require.NoError(t, db.Create(&biz).Error)
	bill := Bill{BusinessID: biz.ID, TotalAmount: 1000, PaidAmount: 1000, TipAmount: 0, Status: BillStatusPaid}
	require.NoError(t, db.Create(&bill).Error)
	// Seed the aggregate as if this payment had been recognized once.
	require.NoError(t, db.Create(&BusinessRevenueAggregate{
		BusinessID: biz.ID, NetRevenueCents: 1000, NetTipCents: 0, RecognizedBillCount: 1,
	}).Error)
	// Operator path already flipped the payment to "refunded".
	pay := Payment{BillID: bill.ID, Amount: 1000, TipAmount: 0, TxHash: "plugin_REFUNDED_ONCE", Status: PaymentStatusRefunded}
	require.NoError(t, db.Create(&pay).Error)

	require.NoError(t, ReversePluginPayment("plugin_REFUNDED_ONCE"))

	var agg BusinessRevenueAggregate
	require.NoError(t, db.Where("business_id = ?", biz.ID).First(&agg).Error)
	require.Equal(t, int64(1000), agg.NetRevenueCents,
		"webhook re-reversal double-decremented the revenue aggregate")
}

func TestReversePluginPayment_PartialReversal_BillRemainsPartial(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)

	// Two payments applied: 40 + 40 = 80; only reversing one of them.
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"paid_amount": 8000,
		"status":      BillStatusPartial,
	}).Error)

	// Insert both payments; we only reverse the second.
	helperPayment(t, bill, 40, 0, "plugin_first", PaymentStatusConfirmed)
	p2 := helperPayment(t, bill, 40, 0, "plugin_second", PaymentStatusConfirmed)

	err := ReversePluginPayment(p2.TxHash)
	require.NoError(t, err)

	var refreshedBill Bill
	require.NoError(t, db.First(&refreshedBill, bill.ID).Error)
	assert.Equal(t, int64(4000), refreshedBill.PaidAmount)
	// 40 < 100 so the bill is still partially paid.
	assert.Equal(t, BillStatusPartial, refreshedBill.Status)
}
