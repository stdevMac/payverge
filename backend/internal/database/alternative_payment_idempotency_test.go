package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCreateConfirmedAlternativePayment_DistinctKeysBothApply locks F-ALTKEY:
// two genuinely separate cash tenders of the same amount/participant on one
// bill — each carrying its own idempotency key — must BOTH be credited. The old
// content-equality dedup silently dropped the second (under-credit). A retry
// reusing a prior key is still deduped.
func TestCreateConfirmedAlternativePayment_DistinctKeysBothApply(t *testing.T) {
	setupPaymentLedgerTestDB(t)
	require.NoError(t, db.AutoMigrate(&BillSplitShare{}))

	biz := &Business{Name: "Alt Co", IsActive: true}
	require.NoError(t, db.Create(biz).Error)
	bill := &Bill{BusinessID: biz.ID, BillNumber: "ALT-IDEM-1", Status: BillStatusOpen, Items: "[]", TotalAmount: 10000}
	require.NoError(t, db.Create(bill).Error)

	pay := func(key string) bool {
		_, applied, err := CreateConfirmedAlternativePayment(&AlternativePayment{
			BillID:          bill.ID,
			ParticipantAddr: "cash",
			Amount:          5000,
			PaymentMethod:   PaymentMethodCash,
			Status:          AltPaymentStatusConfirmed,
			IdempotencyKey:  key,
		}, nil)
		require.NoError(t, err)
		return applied
	}

	require.True(t, pay("req-1"), "first tender applies")
	require.True(t, pay("req-2"), "second distinct tender must also apply")

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.EqualValues(t, 10000, reloaded.PaidAmount, "two distinct cash tenders must both be credited")

	// A retry reusing an earlier key is deduped (not re-credited).
	require.False(t, pay("req-1"), "a retry with the same key must dedupe")
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.EqualValues(t, 10000, reloaded.PaidAmount)
}

// Without an idempotency key the legacy content-equality dedup is preserved, so
// a keyless double-submit can't double-credit (no regression for old clients).
func TestCreateConfirmedAlternativePayment_NoKeyContentDedupPreserved(t *testing.T) {
	setupPaymentLedgerTestDB(t)
	require.NoError(t, db.AutoMigrate(&BillSplitShare{}))

	biz := &Business{Name: "Alt Co", IsActive: true}
	require.NoError(t, db.Create(biz).Error)
	bill := &Bill{BusinessID: biz.ID, BillNumber: "ALT-IDEM-2", Status: BillStatusOpen, Items: "[]", TotalAmount: 10000}
	require.NoError(t, db.Create(bill).Error)

	pay := func() bool {
		_, applied, err := CreateConfirmedAlternativePayment(&AlternativePayment{
			BillID:          bill.ID,
			ParticipantAddr: "cash",
			Amount:          5000,
			PaymentMethod:   PaymentMethodCash,
			Status:          AltPaymentStatusConfirmed,
		}, nil)
		require.NoError(t, err)
		return applied
	}

	require.True(t, pay())
	require.False(t, pay(), "without a key, identical content is deduped (legacy behavior)")

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.EqualValues(t, 5000, reloaded.PaidAmount)
}

func TestCreateConfirmedAlternativePaymentCreatesCashRegisterSaleMovementWhenSessionOpen(t *testing.T) {
	setupPaymentLedgerTestDB(t)
	biz := createPaymentLedgerBusiness(t, GetDBWrapper())
	openAlternativePaymentCashRegisterSession(t, biz.ID, 2500)
	bill := createPaymentLedgerBill(t, GetDBWrapper(), biz.ID, "ALT-CASH-SESSION", 10000, 0, 0, BillStatusOpen, time.Now(), time.Now(), nil)
	staffID := uint(7)

	_, applied, err := CreateConfirmedAlternativePayment(&AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "cash",
		ParticipantName: "cashier",
		Amount:          3000,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusConfirmed,
		ConfirmedBy:     "owner",
		IdempotencyKey:  "cash-session",
	}, &staffID)

	require.NoError(t, err)
	require.True(t, applied)
	movement := requireSingleCashRegisterMovement(t)
	require.Equal(t, CashRegisterMovementTypeCashSale, movement.MovementType)
	require.Equal(t, int64(3000), movement.AmountCents)
	require.Equal(t, "staff:7", movement.ActorLabel)
	require.NotNil(t, movement.ActorStaffID)
	require.Equal(t, staffID, *movement.ActorStaffID)
	require.NotNil(t, movement.AlternativePaymentID)
	require.NotNil(t, movement.BillID)
	require.Equal(t, bill.ID, *movement.BillID)
	requireCashRegisterSessionTotals(t, biz.ID, 3000, 0)
}

func TestCreateConfirmedAlternativePaymentAllowsCashWhenNoOpenCashRegisterSession(t *testing.T) {
	setupPaymentLedgerTestDB(t)
	biz := createPaymentLedgerBusiness(t, GetDBWrapper())
	bill := createPaymentLedgerBill(t, GetDBWrapper(), biz.ID, "ALT-CASH-NO-SESSION", 10000, 0, 0, BillStatusOpen, time.Now(), time.Now(), nil)

	_, applied, err := CreateConfirmedAlternativePayment(&AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "cash",
		Amount:          3000,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusConfirmed,
		IdempotencyKey:  "cash-no-session",
	}, nil)

	require.NoError(t, err)
	require.True(t, applied)
	requireCashRegisterMovementCount(t, 0)
}

func TestCreateConfirmedAlternativePaymentDoesNotCreateMovementForNonCashPayment(t *testing.T) {
	setupPaymentLedgerTestDB(t)
	biz := createPaymentLedgerBusiness(t, GetDBWrapper())
	openAlternativePaymentCashRegisterSession(t, biz.ID, 0)
	bill := createPaymentLedgerBill(t, GetDBWrapper(), biz.ID, "ALT-CARD-SESSION", 10000, 0, 0, BillStatusOpen, time.Now(), time.Now(), nil)

	_, applied, err := CreateConfirmedAlternativePayment(&AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "card",
		Amount:          3000,
		PaymentMethod:   PaymentMethodCard,
		Status:          AltPaymentStatusConfirmed,
		IdempotencyKey:  "card-session",
	}, nil)

	require.NoError(t, err)
	require.True(t, applied)
	requireCashRegisterMovementCount(t, 0)
	requireCashRegisterSessionTotals(t, biz.ID, 0, 0)
}

func TestCreateConfirmedAlternativePaymentIdempotencyDoesNotDuplicateCashRegisterMovement(t *testing.T) {
	setupPaymentLedgerTestDB(t)
	biz := createPaymentLedgerBusiness(t, GetDBWrapper())
	openAlternativePaymentCashRegisterSession(t, biz.ID, 0)
	bill := createPaymentLedgerBill(t, GetDBWrapper(), biz.ID, "ALT-CASH-IDEM-MOVEMENT", 10000, 0, 0, BillStatusOpen, time.Now(), time.Now(), nil)

	pay := func() bool {
		_, applied, err := CreateConfirmedAlternativePayment(&AlternativePayment{
			BillID:          bill.ID,
			ParticipantAddr: "cash",
			Amount:          3000,
			PaymentMethod:   PaymentMethodCash,
			Status:          AltPaymentStatusConfirmed,
			IdempotencyKey:  "same-cash-key",
		}, nil)
		require.NoError(t, err)
		return applied
	}

	require.True(t, pay())
	require.False(t, pay())
	requireCashRegisterMovementCount(t, 1)
	requireCashRegisterSessionTotals(t, biz.ID, 3000, 0)
}

func TestConfirmPendingAlternativePaymentIncludesSplitShareTipInCashRegisterMovement(t *testing.T) {
	setupPaymentLedgerTestDB(t)
	biz := createPaymentLedgerBusiness(t, GetDBWrapper())
	openAlternativePaymentCashRegisterSession(t, biz.ID, 0)
	bill := createPaymentLedgerBill(t, GetDBWrapper(), biz.ID, "ALT-CASH-SPLIT-TIP", 10000, 0, 0, BillStatusOpen, time.Now(), time.Now(), nil)
	holdExpiresAt := time.Now().Add(10 * time.Minute)
	share := BillSplitShare{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		DisplayName:    "Guest 1",
		Mode:           BillSplitModeCustom,
		AmountCents:    4000,
		TipCents:       650,
		Status:         BillSplitShareStatusHeld,
		HoldExpiresAt:  &holdExpiresAt,
	}
	require.NoError(t, db.Create(&share).Error)
	payment := AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: BillSplitAlternativePaymentParticipantAddr(share.ID),
		ParticipantName: "Guest 1",
		Amount:          share.AmountCents,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusPending,
	}
	require.NoError(t, db.Create(&payment).Error)

	confirmed, _, err := ConfirmPendingAlternativePayment(bill.ID, payment.ID, "staff:9", nil)

	require.NoError(t, err)
	require.Equal(t, int64(4000), confirmed.BillAmountCents)
	require.Equal(t, int64(650), confirmed.TipAmountCents)
	movement := requireSingleCashRegisterMovement(t)
	require.Equal(t, CashRegisterMovementTypeCashSale, movement.MovementType)
	require.Equal(t, int64(4650), movement.AmountCents)
	require.Equal(t, "staff:9", movement.ActorLabel)
	requireCashRegisterSessionTotals(t, biz.ID, 4650, 0)
}

func openAlternativePaymentCashRegisterSession(t *testing.T, businessID uint, openingFloatCents int64) CashRegisterSession {
	t.Helper()
	now := time.Now().UTC()
	session := CashRegisterSession{
		BusinessID:        businessID,
		Status:            CashRegisterSessionStatusOpen,
		OpeningFloatCents: openingFloatCents,
		ExpectedCashCents: openingFloatCents,
		OpenedByLabel:     "staff:1",
		OpenedByStaffID:   ptrUint(1),
		OpenedAt:          now,
	}
	require.NoError(t, db.Create(&session).Error)
	return session
}

func requireSingleCashRegisterMovement(t *testing.T) CashRegisterMovement {
	t.Helper()
	var movements []CashRegisterMovement
	require.NoError(t, db.Find(&movements).Error)
	require.Len(t, movements, 1)
	return movements[0]
}

func requireCashRegisterMovementCount(t *testing.T, expected int64) {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&CashRegisterMovement{}).Count(&count).Error)
	require.Equal(t, expected, count)
}

func requireCashRegisterSessionTotals(t *testing.T, businessID uint, salesCents int64, refundsCents int64) {
	t.Helper()
	var session CashRegisterSession
	require.NoError(t, db.Where("business_id = ? AND status = ?", businessID, CashRegisterSessionStatusOpen).First(&session).Error)
	require.Equal(t, salesCents, session.CashSalesCents)
	require.Equal(t, refundsCents, session.CashRefundsCents)
}
