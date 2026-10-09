package database

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupCryptoRefundTestDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf("file:crypto_refund_%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	db = gormDB
	require.NoError(t, gormDB.AutoMigrate(
		&Business{},
		&Table{},
		&Bill{},
		&Payment{},
		&PaymentRefundDestination{},
		&PaymentRefund{},
		&BillHistoryEvent{},
		&BillSplitShare{},
		&BusinessRevenueAggregate{},
		&BusinessMilestoneEvent{},
	))
}

func seedCryptoPayment(t *testing.T, amountBaseUnits int64) (businessID, billID, paymentID uint) {
	t.Helper()
	biz := &Business{Name: "Refund Biz", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &Bill{
		BusinessID:     biz.ID,
		BillNumber:     fmt.Sprintf("B-%d", time.Now().UnixNano()),
		TotalAmount:    2500,
		PaidAmount:     2500,
		TipAmount:      0,
		Status:         BillStatusPaid,
		SettlementAddr: biz.SettlementAddr,
	}
	require.NoError(t, db.Create(bill).Error)
	pay := &Payment{
		BillID:          bill.ID,
		PayerAddr:       "crypto_guest",
		Amount:          2500,
		TipAmount:       0,
		TxHash:          fmt.Sprintf("0xpay%d", time.Now().UnixNano()),
		Status:          PaymentStatusConfirmed,
		PaymentMethod:   "crypto",
		SettlementChain: "base",
	}
	require.NoError(t, db.Create(pay).Error)
	addr := "0xdead00000000000000000000000000000000beef"
	require.NoError(t, persistPaymentRefundDestinationTx(db, &PaymentRefundDestination{
		PaymentID:       pay.ID,
		ChainID:         84532,
		Token:           "USDC",
		AmountBaseUnits: amountBaseUnits,
		RefundAddress:   addr,
		EvidenceType:    RefundEvidenceTransferLog,
		VerifiedAt:      time.Now().UTC(),
	}))
	return biz.ID, bill.ID, pay.ID
}

func TestPersistPaymentRefundDestination_RejectsSentinelAndMalformed(t *testing.T) {
	setupCryptoRefundTestDB(t)
	_, _, paymentID := seedCryptoPayment(t, 5_000_000)

	err := persistPaymentRefundDestinationTx(db, &PaymentRefundDestination{
		PaymentID:       paymentID + 999,
		ChainID:         84532,
		Token:           "USDC",
		AmountBaseUnits: 1,
		RefundAddress:   "crypto_guest",
		EvidenceType:    RefundEvidenceTransferLog,
		VerifiedAt:      time.Now().UTC(),
	})
	require.Error(t, err)

	err = persistPaymentRefundDestinationTx(db, &PaymentRefundDestination{
		PaymentID:       paymentID + 998,
		ChainID:         84532,
		Token:           "USDC",
		AmountBaseUnits: 1,
		RefundAddress:   "not-an-address",
		EvidenceType:    RefundEvidenceTransferLog,
		VerifiedAt:      time.Now().UTC(),
	})
	require.Error(t, err)
}

func TestPersistPaymentRefundDestination_RejectsConflictingEvidence(t *testing.T) {
	setupCryptoRefundTestDB(t)
	_, _, paymentID := seedCryptoPayment(t, 5_000_000)

	existing, err := GetPaymentRefundDestination(paymentID)
	require.NoError(t, err)
	require.Equal(t, "0xdead00000000000000000000000000000000beef", existing.RefundAddress)

	err = persistPaymentRefundDestinationTx(db, &PaymentRefundDestination{
		PaymentID:       paymentID,
		ChainID:         existing.ChainID,
		Token:           existing.Token,
		AmountBaseUnits: existing.AmountBaseUnits,
		RefundAddress:   "0x2222222222222222222222222222222222222222",
		EvidenceType:    existing.EvidenceType,
		VerifiedAt:      time.Now().UTC(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "conflicts with existing evidence")

	reloaded, err := GetPaymentRefundDestination(paymentID)
	require.NoError(t, err)
	require.Equal(t, existing.RefundAddress, reloaded.RefundAddress)
}

func TestApplyConfirmedPayment_PersistsRefundEvidenceAtomically(t *testing.T) {
	setupCryptoRefundTestDB(t)
	biz := &Business{Name: "Atomic Evidence", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &Bill{
		BusinessID: biz.ID, BillNumber: "B-atomic-evidence", TotalAmount: 2500,
		Status: BillStatusOpen, SettlementAddr: biz.SettlementAddr,
	}
	require.NoError(t, db.Create(bill).Error)
	logRef := "0xatomic:0"

	_, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 2500,
		TxHash: "0xatomic", Status: PaymentStatusConfirmed, PaymentMethod: "crypto",
		RefundDestination: &PaymentRefundDestination{
			ChainID: 84532, Token: "USDC", AmountBaseUnits: 5_000_000,
			RefundAddress: "0xdead00000000000000000000000000000000beef",
			EvidenceType:  RefundEvidenceTransferLog, LogRef: &logRef, VerifiedAt: time.Now().UTC(),
		},
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)

	payment, err := GetPaymentByTxHash("0xatomic")
	require.NoError(t, err)
	dest, err := GetPaymentRefundDestination(payment.ID)
	require.NoError(t, err)
	require.Equal(t, payment.ID, dest.PaymentID)
	require.Equal(t, "0xdead00000000000000000000000000000000beef", dest.RefundAddress)

	// An idempotent settlement replay re-validates the same immutable proof and
	// remains a no-op rather than producing a duplicate row.
	_, applied, err = ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 2500,
		TxHash: "0xatomic", Status: PaymentStatusConfirmed, PaymentMethod: "crypto",
		RefundDestination: &PaymentRefundDestination{
			ChainID: 84532, Token: "USDC", AmountBaseUnits: 5_000_000,
			RefundAddress: "0xdead00000000000000000000000000000000beef",
			EvidenceType:  RefundEvidenceTransferLog, LogRef: &logRef, VerifiedAt: time.Now().UTC(),
		},
	}, nil)
	require.NoError(t, err)
	require.False(t, applied)
	var destinationCount int64
	require.NoError(t, db.Model(&PaymentRefundDestination{}).Where("payment_id = ?", payment.ID).Count(&destinationCount).Error)
	require.EqualValues(t, 1, destinationCount)
}

func TestApplyConfirmedPayment_ReplayBackfillsPreviouslyMissingRefundEvidence(t *testing.T) {
	setupCryptoRefundTestDB(t)
	biz := &Business{Name: "Evidence Repair", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &Bill{BusinessID: biz.ID, BillNumber: "B-evidence-repair", TotalAmount: 2500, Status: BillStatusOpen}
	require.NoError(t, db.Create(bill).Error)
	base := ConfirmedPaymentInput{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 2500,
		TxHash: "0xevidencerepair", Status: PaymentStatusConfirmed, PaymentMethod: "crypto",
	}
	_, applied, err := ApplyConfirmedPayment(base, nil)
	require.NoError(t, err)
	require.True(t, applied)
	payment, err := GetPaymentByTxHash(base.TxHash)
	require.NoError(t, err)
	_, err = GetPaymentRefundDestination(payment.ID)
	require.ErrorIs(t, err, ErrCryptoRefundDestinationMissing)

	base.RefundDestination = &PaymentRefundDestination{
		ChainID: 84532, Token: "USDC", AmountBaseUnits: 5_000_000,
		RefundAddress: "0xdead00000000000000000000000000000000beef",
		EvidenceType:  RefundEvidenceTransferLog, VerifiedAt: time.Now().UTC(),
	}
	_, applied, err = ApplyConfirmedPayment(base, nil)
	require.NoError(t, err)
	require.False(t, applied)
	_, err = GetPaymentRefundDestination(payment.ID)
	require.NoError(t, err)
}

func TestApplyConfirmedPayment_InvalidRefundEvidenceRollsBackPayment(t *testing.T) {
	setupCryptoRefundTestDB(t)
	biz := &Business{Name: "Atomic Evidence Rollback", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &Bill{
		BusinessID: biz.ID, BillNumber: "B-atomic-evidence-rollback", TotalAmount: 2500,
		Status: BillStatusOpen, SettlementAddr: biz.SettlementAddr,
	}
	require.NoError(t, db.Create(bill).Error)

	_, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 2500,
		TxHash: "0xatomicbad", Status: PaymentStatusConfirmed, PaymentMethod: "crypto",
		RefundDestination: &PaymentRefundDestination{
			ChainID: 84532, Token: "USDC", AmountBaseUnits: 5_000_000,
			RefundAddress: "not-an-address", EvidenceType: RefundEvidenceTransferLog,
		},
	}, nil)
	require.Error(t, err)
	require.False(t, applied)

	var paymentCount int64
	require.NoError(t, db.Model(&Payment{}).Where("tx_hash = ?", "0xatomicbad").Count(&paymentCount).Error)
	require.Zero(t, paymentCount)
	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.Zero(t, reloaded.PaidAmount)
	require.Equal(t, BillStatusOpen, reloaded.Status)
}

func TestRequestCryptoRefund_IdempotentAndOneActive(t *testing.T) {
	setupCryptoRefundTestDB(t)
	bizID, billID, payID := seedCryptoPayment(t, 5_000_000)

	r1, err := RequestCryptoRefund(CreateCryptoRefundInput{
		BusinessID: bizID, BillID: billID, PaymentID: payID,
		AmountBaseUnits: 5_000_000, Reason: "guest complaint",
		RequestedBy: "owner", IdempotencyKey: "idem-1",
	})
	require.NoError(t, err)
	require.Equal(t, PaymentRefundStatusRequested, r1.Status)

	// Same idempotency key → same row.
	r1b, err := RequestCryptoRefund(CreateCryptoRefundInput{
		BusinessID: bizID, BillID: billID, PaymentID: payID,
		AmountBaseUnits: 5_000_000, Reason: "guest complaint",
		RequestedBy: "owner", IdempotencyKey: "idem-1",
	})
	require.NoError(t, err)
	require.Equal(t, r1.ID, r1b.ID)

	// A second active request is blocked even at the full amount.
	_, err = RequestCryptoRefund(CreateCryptoRefundInput{
		BusinessID: bizID, BillID: billID, PaymentID: payID,
		AmountBaseUnits: 5_000_000, Reason: "second attempt",
		RequestedBy: "owner", IdempotencyKey: "idem-2",
	})
	require.ErrorIs(t, err, ErrCryptoRefundActiveExists)
}

func TestRequestCryptoRefund_ConcurrentDoubleRequestOnlyOneWins(t *testing.T) {
	setupCryptoRefundTestDB(t)
	bizID, billID, payID := seedCryptoPayment(t, 5_000_000)

	var wins int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := RequestCryptoRefund(CreateCryptoRefundInput{
				BusinessID: bizID, BillID: billID, PaymentID: payID,
				AmountBaseUnits: 5_000_000, Reason: "concurrent",
				RequestedBy: "owner", IdempotencyKey: fmt.Sprintf("c-%d", i),
			})
			if err == nil {
				atomic.AddInt32(&wins, 1)
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, int32(1), wins, "exactly one concurrent request must win")
}

func TestRequestCryptoRefund_FullOnlyBalance(t *testing.T) {
	setupCryptoRefundTestDB(t)
	bizID, billID, payID := seedCryptoPayment(t, 10_000_000)

	// Partial request is rejected: the ledger only does full reversal, so a
	// partial on-chain send would over-reverse the books. Full-only is enforced.
	_, err := RequestCryptoRefund(CreateCryptoRefundInput{
		BusinessID: bizID, BillID: billID, PaymentID: payID,
		AmountBaseUnits: 4_000_000, Reason: "partial refund",
		RequestedBy: "owner", IdempotencyKey: "partial-1",
	})
	require.ErrorIs(t, err, ErrCryptoRefundMustBeFullAmount)

	// Over-balance rejected (distinct error).
	_, err = RequestCryptoRefund(CreateCryptoRefundInput{
		BusinessID: bizID, BillID: billID, PaymentID: payID,
		AmountBaseUnits: 11_000_000, Reason: "too much",
		RequestedBy: "owner", IdempotencyKey: "over",
	})
	require.ErrorIs(t, err, ErrCryptoRefundAmountExceedsBalance)

	// Full remaining balance is accepted.
	r, err := RequestCryptoRefund(CreateCryptoRefundInput{
		BusinessID: bizID, BillID: billID, PaymentID: payID,
		AmountBaseUnits: 10_000_000, Reason: "full refund",
		RequestedBy: "owner", IdempotencyKey: "full",
	})
	require.NoError(t, err)
	require.Equal(t, int64(10_000_000), r.AmountBaseUnits)
}

func TestRequestCryptoRefund_MissingEvidenceRequiresManualSupport(t *testing.T) {
	setupCryptoRefundTestDB(t)
	biz := &Business{Name: "No Dest", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &Bill{BusinessID: biz.ID, BillNumber: "B-nodest", TotalAmount: 1000, PaidAmount: 1000, Status: BillStatusPaid}
	require.NoError(t, db.Create(bill).Error)
	pay := &Payment{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 1000,
		TxHash: "0xnodest", Status: PaymentStatusConfirmed, PaymentMethod: "crypto",
	}
	require.NoError(t, db.Create(pay).Error)

	_, err := RequestCryptoRefund(CreateCryptoRefundInput{
		BusinessID: biz.ID, BillID: bill.ID, PaymentID: pay.ID,
		AmountBaseUnits: 1_000_000, Reason: "want refund",
		RequestedBy: "owner", IdempotencyKey: "no-dest",
	})
	require.ErrorIs(t, err, ErrCryptoRefundDestinationMissing)
}

func TestRejectCryptoRefund_LeavesLedgerUntouched(t *testing.T) {
	setupCryptoRefundTestDB(t)
	bizID, billID, payID := seedCryptoPayment(t, 5_000_000)
	r, err := RequestCryptoRefund(CreateCryptoRefundInput{
		BusinessID: bizID, BillID: billID, PaymentID: payID,
		AmountBaseUnits: 5_000_000, Reason: "request",
		RequestedBy: "owner", IdempotencyKey: "rej-1",
	})
	require.NoError(t, err)
	_, err = RejectCryptoRefund(r.ID, bizID, "owner", "nope")
	require.NoError(t, err)

	var pay Payment
	require.NoError(t, db.First(&pay, payID).Error)
	require.Equal(t, PaymentStatusConfirmed, pay.Status)
	var bill Bill
	require.NoError(t, db.First(&bill, billID).Error)
	require.Equal(t, int64(2500), bill.PaidAmount)
}

func TestApproveOverrideRequiresSecondApprover(t *testing.T) {
	setupCryptoRefundTestDB(t)
	bizID, billID, payID := seedCryptoPayment(t, 5_000_000)
	override := "0x2222222222222222222222222222222222222222"
	r, err := RequestCryptoRefund(CreateCryptoRefundInput{
		BusinessID: bizID, BillID: billID, PaymentID: payID,
		AmountBaseUnits: 5_000_000, Reason: "override path",
		RequestedBy: "owner-a", IdempotencyKey: "ov-1",
		RecipientOverride: &override, OverrideReason: "guest lost wallet, KYC verified",
	})
	require.NoError(t, err)
	require.True(t, r.RecipientOverride)

	_, err = ApproveCryptoRefund(r.ID, bizID, "owner-a")
	require.Error(t, err)
	require.Contains(t, err.Error(), "second distinct")

	approved, err := ApproveCryptoRefund(r.ID, bizID, "owner-b")
	require.NoError(t, err)
	require.Equal(t, PaymentRefundStatusAwaitingSignature, approved.Status)
}

func TestSubmitTxHashUniqueAndConfirmAppliesLedgerOnce(t *testing.T) {
	setupCryptoRefundTestDB(t)
	bizID, billID, payID := seedCryptoPayment(t, 5_000_000)
	r, err := RequestCryptoRefund(CreateCryptoRefundInput{
		BusinessID: bizID, BillID: billID, PaymentID: payID,
		AmountBaseUnits: 5_000_000, Reason: "on-chain refund",
		RequestedBy: "owner", IdempotencyKey: "sub-1",
	})
	require.NoError(t, err)
	_, err = ApproveCryptoRefund(r.ID, bizID, "owner")
	require.NoError(t, err)

	submitted, err := SubmitCryptoRefundTxHash(r.ID, bizID, "0xe15dd03ab99f5506fe7df79d6f90a30ed0d383ccdcf5f280f4dd9d1abff6c669")
	require.NoError(t, err)
	require.Equal(t, PaymentRefundStatusSubmitted, submitted.Status)

	// Reuse same hash on another refund must fail.
	// First complete this one to free active slot, then try reuse.
	bill, pay, refund, err := ConfirmCryptoRefundAndApplyLedgerWithHook(r.ID, 3, "worker", nil)
	require.NoError(t, err)
	require.NotNil(t, bill)
	require.Equal(t, PaymentStatusRefunded, pay.Status)
	require.Equal(t, PaymentRefundStatusConfirmed, refund.Status)
	require.NotNil(t, refund.LedgerAppliedAt)
	require.Equal(t, int64(0), bill.PaidAmount)

	// Idempotent re-confirm does not double-apply.
	_, pay2, refund2, err := ConfirmCryptoRefundAndApplyLedgerWithHook(r.ID, 5, "worker", nil)
	require.NoError(t, err)
	require.Equal(t, PaymentRefundStatusConfirmed, refund2.Status)
	if pay2 != nil {
		require.Equal(t, PaymentStatusRefunded, pay2.Status)
	}
}

// TestSubmitCryptoRefundTxHash_CanonicalSpellingParity pins the owner refund
// submit path to the same tx-hash rules as guest payments: every
// spelling of one transfer is stored as 0x + 64 lowercase hex, an idempotent
// re-submit in another spelling is accepted, a second refund cannot claim the
// same transfer by changing case, and lenient spellings go-ethereum would
// still resolve (no 0x prefix, padding, trailing bytes) are refused outright.
func TestSubmitCryptoRefundTxHash_CanonicalSpellingParity(t *testing.T) {
	setupCryptoRefundTestDB(t)
	const hexBody = "e15dd03ab99f5506fe7df79d6f90a30ed0d383ccdcf5f280f4dd9d1abff6c669"
	canonical := "0x" + hexBody

	approvedRefund := func(key string) *PaymentRefund {
		t.Helper()
		bizID, billID, payID := seedCryptoPayment(t, 5_000_000)
		// seedCryptoPayment leaves the public business_id empty; give each
		// seeded business its own so the next seed does not collide.
		require.NoError(t, db.Model(&Business{}).Where("id = ?", bizID).Update("business_id", key).Error)
		r, err := RequestCryptoRefund(CreateCryptoRefundInput{
			BusinessID: bizID, BillID: billID, PaymentID: payID,
			AmountBaseUnits: 5_000_000, Reason: "on-chain refund",
			RequestedBy: "owner", IdempotencyKey: key,
		})
		require.NoError(t, err)
		approved, err := ApproveCryptoRefund(r.ID, bizID, "owner")
		require.NoError(t, err)
		return approved
	}
	first := approvedRefund("parity-a")
	second := approvedRefund("parity-b")

	submitted, err := SubmitCryptoRefundTxHash(first.ID, first.BusinessID, " 0X"+strings.ToUpper(hexBody)+" ")
	require.NoError(t, err)
	require.NotNil(t, submitted.SubmittedTxHash)
	require.Equal(t, canonical, *submitted.SubmittedTxHash, "an uppercase spelling is stored canonical")

	again, err := SubmitCryptoRefundTxHash(first.ID, first.BusinessID, "0x"+strings.ToUpper(hexBody[:32])+hexBody[32:])
	require.NoError(t, err, "re-submitting the same transfer in another spelling is idempotent")
	require.Equal(t, PaymentRefundStatusSubmitted, again.Status)
	require.Equal(t, canonical, *again.SubmittedTxHash)

	for _, spelling := range []string{canonical, "0X" + strings.ToUpper(hexBody), "0x" + strings.ToUpper(hexBody[:32]) + hexBody[32:]} {
		_, err := SubmitCryptoRefundTxHash(second.ID, second.BusinessID, spelling)
		require.Truef(t, errors.Is(err, ErrCryptoRefundTxHashReused), "spelling %q must be rejected as the same transfer, got %v", spelling, err)
	}
	for _, malformed := range []string{hexBody, strings.ToUpper(hexBody), "0x00" + hexBody, "0x" + hexBody[:63], "0x" + hexBody + "00", "0xzz" + hexBody[2:], ""} {
		_, err := SubmitCryptoRefundTxHash(second.ID, second.BusinessID, malformed)
		require.Errorf(t, err, "malformed hash %q must be rejected", malformed)
	}
	reloaded, err := GetCryptoRefund(second.ID, second.BusinessID)
	require.NoError(t, err)
	require.Equal(t, PaymentRefundStatusAwaitingSignature, reloaded.Status, "rejected submits leave the second refund untouched")
	require.Nil(t, reloaded.SubmittedTxHash)

	var stored []string
	require.NoError(t, db.Model(&PaymentRefund{}).Where("submitted_tx_hash IS NOT NULL").Pluck("submitted_tx_hash", &stored).Error)
	require.Equal(t, []string{canonical}, stored)
}

func TestMaskRefundAddress(t *testing.T) {
	require.Equal(t, "0xdead…beef", MaskRefundAddress("0xdead00000000000000000000000000000000beef"))
	require.Equal(t, "", MaskRefundAddress(""))
}

// setRefundTestProductionMode pins the production switch for one test; the
// testnet refund allowlist reads it through config.IsProductionMode.
func setRefundTestProductionMode(t *testing.T, production bool) {
	t.Helper()
	t.Setenv("APP_ENV", "")
	if production {
		t.Setenv("ENV", "production")
	} else {
		t.Setenv("ENV", "development")
	}
}

func TestIsCryptoRefundChainAllowlisted_TestnetsDevelopmentOnly(t *testing.T) {
	setRefundTestProductionMode(t, false)
	for _, chain := range []int{8453, 1, 84532, 11155111} {
		require.Truef(t, IsCryptoRefundChainAllowlisted(chain, "usdc"), "development must allowlist chain %d", chain)
	}
	require.False(t, IsCryptoRefundChainAllowlisted(8453, "DAI"))
	require.False(t, IsCryptoRefundChainAllowlisted(137, "USDC"))

	setRefundTestProductionMode(t, true)
	require.True(t, IsCryptoRefundChainAllowlisted(8453, "USDC"))
	require.True(t, IsCryptoRefundChainAllowlisted(1, "USDC"))
	require.False(t, IsCryptoRefundChainAllowlisted(84532, "USDC"), "production must never refund Base Sepolia USDC")
	require.False(t, IsCryptoRefundChainAllowlisted(11155111, "USDC"), "production must never refund Sepolia USDC")
}

func TestRequestCryptoRefund_RefusesTestnetEvidenceInProduction(t *testing.T) {
	setupCryptoRefundTestDB(t)
	// seedCryptoPayment records Base Sepolia (84532) refund evidence.
	bizID, billID, payID := seedCryptoPayment(t, 5_000_000)

	setRefundTestProductionMode(t, true)
	_, err := RequestCryptoRefund(CreateCryptoRefundInput{
		BusinessID: bizID, BillID: billID, PaymentID: payID,
		AmountBaseUnits: 5_000_000, Reason: "testnet in prod",
		RequestedBy: "owner", IdempotencyKey: "prod-sepolia",
	})
	require.ErrorIs(t, err, ErrCryptoRefundChainNotAllowlisted)

	var count int64
	require.NoError(t, db.Model(&PaymentRefund{}).Where("payment_id = ?", payID).Count(&count).Error)
	require.Zero(t, count, "a refused testnet refund must not persist a request row")
}
