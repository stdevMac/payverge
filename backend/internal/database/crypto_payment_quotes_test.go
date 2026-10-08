package database

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const quoteTestWallet = "0x1111111111111111111111111111111111111111"

func setupCryptoQuoteTestDB(t *testing.T) *DB {
	t.Helper()
	wrapper := setupBillSplitTestDB(t, nil)
	require.NoError(t, wrapper.GetGorm().AutoMigrate(&CryptoPaymentQuote{}))
	return wrapper
}

// fixedCryptoQuoteOffsets forces every issuer to start probing at offset 1, so
// the uniqueness logic (not luck) is what keeps amounts apart.
func fixedCryptoQuoteOffsets(t *testing.T) {
	t.Helper()
	prev := CryptoQuoteOffsetSource
	CryptoQuoteOffsetSource = func(int) int { return 0 }
	t.Cleanup(func() { CryptoQuoteOffsetSource = prev })
}

func quoteInput(bill *Bill, wallet string, base int64, issuedAt time.Time) IssueCryptoPaymentQuoteInput {
	return IssueCryptoPaymentQuoteInput{
		BillID:            bill.ID,
		BusinessID:        bill.BusinessID,
		SettlementAddress: wallet,
		ChainID:           8453,
		PaymentMethod:     "usdc_payment",
		BaseMicrounits:    base,
		IssuedAt:          issuedAt,
		ExpiresAt:         issuedAt.Add(30 * time.Minute),
	}
}

func TestIssueCryptoPaymentQuote_SameWalletNeverSharesAnExactAmount(t *testing.T) {
	db := setupCryptoQuoteTestDB(t)
	fixedCryptoQuoteOffsets(t)
	now := time.Now().UTC()
	billA := createBillSplitBill(t, db, "quote-unique-a", 5000)
	billB := createBillSplitBill(t, db, "quote-unique-b", 5000)

	seen := map[int64]bool{}
	for i, bill := range []*Bill{billA, billB, billA} {
		// Mixed-case spelling of the same wallet must land on the same key.
		wallet := quoteTestWallet
		if i == 1 {
			wallet = "0x1111111111111111111111111111111111111111"
		}
		q, err := IssueCryptoPaymentQuote(quoteInput(bill, wallet, 50_000_000, now))
		require.NoError(t, err)
		require.GreaterOrEqual(t, q.OffsetMicrounits, int64(1))
		require.LessOrEqual(t, q.OffsetMicrounits, CryptoQuoteMaxOffsetMicrounits)
		require.Equal(t, q.BaseMicrounits+q.OffsetMicrounits, q.ExactMicrounits)
		require.Equal(t, NormalizeCryptoQuoteAddress(quoteTestWallet), q.SettlementAddress)
		require.Falsef(t, seen[q.ExactMicrounits], "exact amount %d issued twice", q.ExactMicrounits)
		seen[q.ExactMicrounits] = true
	}

	// A different receiving wallet has its own offset space.
	other, err := IssueCryptoPaymentQuote(quoteInput(billA, "0x2222222222222222222222222222222222222222", 50_000_000, now))
	require.NoError(t, err)
	require.Equal(t, int64(1), other.OffsetMicrounits)
}

// TestIssueCryptoPaymentQuote_ConcurrentIssuersGetDistinctAmounts is the
// "(d) concurrent quotes to the same wallet" case at the storage layer. With
// the start offset pinned every issuer collides on purpose, which is the worst
// case for the retry loop: each failed attempt means another issuer won, so
// cryptoQuoteInsertAttempts issuers must all still succeed.
func TestIssueCryptoPaymentQuote_ConcurrentIssuersGetDistinctAmounts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pinned  bool
		issuers int
	}{
		{name: "pinned_start_worst_case", pinned: true, issuers: cryptoQuoteInsertAttempts},
		{name: "random_start", pinned: false, issuers: 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupCryptoQuoteTestDB(t)
			if tc.pinned {
				fixedCryptoQuoteOffsets(t)
			}
			now := time.Now().UTC()
			bills := make([]*Bill, tc.issuers)
			for i := range bills {
				bills[i] = createBillSplitBill(t, db, fmt.Sprintf("quote-concurrent-%d", i), 2500)
			}

			var wg sync.WaitGroup
			results := make([]*CryptoPaymentQuote, len(bills))
			errs := make([]error, len(bills))
			for i, bill := range bills {
				wg.Add(1)
				go func(i int, bill *Bill) {
					defer wg.Done()
					results[i], errs[i] = IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 25_000_000, now))
				}(i, bill)
			}
			wg.Wait()

			seen := map[int64]bool{}
			for i := range bills {
				require.NoError(t, errs[i])
				require.Falsef(t, seen[results[i].ExactMicrounits], "exact amount %d issued twice", results[i].ExactMicrounits)
				seen[results[i].ExactMicrounits] = true
			}
		})
	}
}

// TestIssueCryptoPaymentQuote_RetriesWhenARacingIssuerTakesTheAmount covers
// the window between reading taken amounts and inserting: a concurrent issuer
// that wins the same exact amount makes the partial unique index reject our
// insert, and we must move to another amount instead of failing or sharing.
func TestIssueCryptoPaymentQuote_RetriesWhenARacingIssuerTakesTheAmount(t *testing.T) {
	db := setupCryptoQuoteTestDB(t)
	now := time.Now().UTC()
	bill := createBillSplitBill(t, db, "quote-race", 1000)
	rival := createBillSplitBill(t, db, "quote-race-rival", 1000)

	prev := CryptoQuoteOffsetSource
	t.Cleanup(func() { CryptoQuoteOffsetSource = prev })
	raced := false
	CryptoQuoteOffsetSource = func(int) int {
		if !raced {
			raced = true
			// The rival commits offset 1 after our taken-amount read.
			require.NoError(t, db.GetGorm().Create(&CryptoPaymentQuote{
				BillID: rival.ID, BusinessID: rival.BusinessID, ChainID: 8453,
				SettlementAddress: NormalizeCryptoQuoteAddress(quoteTestWallet), PaymentMethod: "usdc_payment",
				BaseMicrounits: 10_000_000, OffsetMicrounits: 1, ExactMicrounits: 10_000_001,
				Status: CryptoPaymentQuoteStatusActive, IssuedAt: now, ExpiresAt: now.Add(30 * time.Minute),
			}).Error)
		}
		return 0
	}

	q, err := IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 10_000_000, now))
	require.NoError(t, err)
	require.True(t, raced)
	require.Equal(t, int64(10_000_002), q.ExactMicrounits)
}

func TestIssueCryptoPaymentQuote_PartialUniqueIndexRejectsDuplicateActiveAmount(t *testing.T) {
	db := setupCryptoQuoteTestDB(t)
	now := time.Now().UTC()
	bill := createBillSplitBill(t, db, "quote-index", 1000)
	row := func(status CryptoPaymentQuoteStatus) *CryptoPaymentQuote {
		return &CryptoPaymentQuote{
			BillID: bill.ID, BusinessID: bill.BusinessID, ChainID: 8453,
			SettlementAddress: NormalizeCryptoQuoteAddress(quoteTestWallet), PaymentMethod: "usdc_payment",
			BaseMicrounits: 10_000_000, OffsetMicrounits: 7, ExactMicrounits: 10_000_007,
			Status: status, IssuedAt: now, ExpiresAt: now.Add(30 * time.Minute),
		}
	}
	require.NoError(t, db.GetGorm().Create(row(CryptoPaymentQuoteStatusActive)).Error)
	err := db.GetGorm().Create(row(CryptoPaymentQuoteStatusActive)).Error
	require.Error(t, err)
	require.True(t, isUniqueConstraintError(err), "want unique violation, got %v", err)
	// Only ACTIVE rows hold an amount; history rows do not.
	require.NoError(t, db.GetGorm().Create(row(CryptoPaymentQuoteStatusConsumed)).Error)
	require.NoError(t, db.GetGorm().Create(row(CryptoPaymentQuoteStatusExpired)).Error)
}

func TestIssueCryptoPaymentQuote_PerBillCap(t *testing.T) {
	db := setupCryptoQuoteTestDB(t)
	now := time.Now().UTC()
	bill := createBillSplitBill(t, db, "quote-cap", 1000)
	for i := int64(0); i < CryptoQuoteMaxActivePerBill; i++ {
		_, err := IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 10_000_000, now))
		require.NoError(t, err)
	}
	_, err := IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 10_000_000, now))
	require.ErrorIs(t, err, ErrCryptoQuoteLimitReached)

	// Once those quotes lapse the bill can quote again.
	_, err = IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 10_000_000, now.Add(31*time.Minute)))
	require.NoError(t, err)
}

const (
	quoteClientA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	quoteClientB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// Review c1 L3: one client holding the bill link cannot take every quote slot
// of the bill. It stops at the per-client cap while other payers, and callers
// without a client key, still get quotes.
func TestIssueCryptoPaymentQuote_PerClientCapLeavesSlotsForOtherPayers(t *testing.T) {
	require.Less(t, CryptoQuoteMaxActivePerClient, CryptoQuoteMaxActivePerBill,
		"the per-client cap must sit below the per-bill cap or it protects nobody")
	db := setupCryptoQuoteTestDB(t)
	now := time.Now().UTC()
	bill := createBillSplitBill(t, db, "quote-client-cap", 1000)
	withClient := func(key string, at time.Time) IssueCryptoPaymentQuoteInput {
		in := quoteInput(bill, quoteTestWallet, 10_000_000, at)
		in.ClientKey = key
		return in
	}

	for i := int64(0); i < CryptoQuoteMaxActivePerClient; i++ {
		q, err := IssueCryptoPaymentQuote(withClient(quoteClientA, now))
		require.NoError(t, err)
		require.NotNil(t, q.ClientKey)
		require.Equal(t, quoteClientA, *q.ClientKey)
	}
	_, err := IssueCryptoPaymentQuote(withClient(quoteClientA, now))
	require.ErrorIs(t, err, ErrCryptoQuoteClientLimitReached)
	require.NotErrorIs(t, err, ErrCryptoQuoteLimitReached, "the bill itself still has room")

	// Another payer on the same bill is unaffected, and so is a caller
	// without a client key (counted only against the per-bill cap).
	_, err = IssueCryptoPaymentQuote(withClient(quoteClientB, now))
	require.NoError(t, err)
	_, err = IssueCryptoPaymentQuote(withClient("", now))
	require.NoError(t, err)

	// Once the client's quotes lapse it can quote again.
	_, err = IssueCryptoPaymentQuote(withClient(quoteClientA, now.Add(31*time.Minute)))
	require.NoError(t, err)
}

// The client key exists only to count live quotes, so it is cleared when a
// quote is consumed or swept to expired.
func TestCryptoPaymentQuote_ClientKeyClearedWhenQuoteStopsBeingActive(t *testing.T) {
	db := setupCryptoQuoteTestDB(t)
	now := time.Now().UTC()
	bill := createBillSplitBill(t, db, "quote-client-key-clear", 3000)
	other := createBillSplitBill(t, db, "quote-client-key-clear-2", 3000)

	in := quoteInput(bill, quoteTestWallet, 30_000_000, now)
	in.ClientKey = quoteClientA
	paid, err := IssueCryptoPaymentQuote(in)
	require.NoError(t, err)
	_, applied, err := ApplyConfirmedPayment(quotedPayment(bill, paid, quoteTxA), nil)
	require.NoError(t, err)
	require.True(t, applied)
	consumed, err := GetCryptoPaymentQuote(paid.ID)
	require.NoError(t, err)
	require.Equal(t, CryptoPaymentQuoteStatusConsumed, consumed.Status)
	require.Nil(t, consumed.ClientKey, "a consumed quote keeps no client key")

	in = quoteInput(other, quoteTestWallet, 10_000_000, now)
	in.ClientKey = quoteClientA
	lapsed, err := IssueCryptoPaymentQuote(in)
	require.NoError(t, err)
	// The next issuance for the wallet sweeps the lapsed quote.
	_, err = IssueCryptoPaymentQuote(quoteInput(other, quoteTestWallet, 10_000_000, now.Add(31*time.Minute)))
	require.NoError(t, err)
	swept, err := GetCryptoPaymentQuote(lapsed.ID)
	require.NoError(t, err)
	require.Equal(t, CryptoPaymentQuoteStatusExpired, swept.Status)
	require.Nil(t, swept.ClientKey, "an expired quote keeps no client key")
}

func TestIssueCryptoPaymentQuote_ExpiredAmountHonoursReuseCooldown(t *testing.T) {
	db := setupCryptoQuoteTestDB(t)
	fixedCryptoQuoteOffsets(t)
	t0 := time.Now().UTC().Truncate(time.Second)
	bill := createBillSplitBill(t, db, "quote-cooldown", 1000)

	first, err := IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 10_000_000, t0))
	require.NoError(t, err)
	require.Equal(t, int64(1), first.OffsetMicrounits)

	// Just after expiry: the stale row is flipped to expired, but its amount
	// is still cooling down and must not be handed out again.
	second, err := IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 10_000_000, t0.Add(31*time.Minute)))
	require.NoError(t, err)
	require.Equal(t, int64(2), second.OffsetMicrounits)
	reloaded, err := GetCryptoPaymentQuote(first.ID)
	require.NoError(t, err)
	require.Equal(t, CryptoPaymentQuoteStatusExpired, reloaded.Status)

	// After the cooldown the amount is free again.
	third, err := IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 10_000_000, t0.Add(30*time.Minute+CryptoQuoteAmountReuseCooldown+time.Minute)))
	require.NoError(t, err)
	require.Equal(t, int64(1), third.OffsetMicrounits)
}

func TestIssueCryptoPaymentQuote_ExhaustedOffsetSpace(t *testing.T) {
	taken := make(map[int64]struct{}, CryptoQuoteMaxOffsetMicrounits)
	for i := int64(1); i <= CryptoQuoteMaxOffsetMicrounits; i++ {
		taken[i] = struct{}{}
	}
	_, ok := pickFreeCryptoQuoteOffset(taken)
	require.False(t, ok)
	delete(taken, 4242)
	offset, ok := pickFreeCryptoQuoteOffset(taken)
	require.True(t, ok)
	require.Equal(t, int64(4242), offset)
}

func TestGetCryptoPaymentQuote_Missing(t *testing.T) {
	setupCryptoQuoteTestDB(t)
	_, err := GetCryptoPaymentQuote(999)
	require.ErrorIs(t, err, ErrCryptoQuoteNotFound)
	_, err = GetCryptoPaymentQuote(0)
	require.ErrorIs(t, err, ErrCryptoQuoteNotFound)
}

func quotedPayment(bill *Bill, q *CryptoPaymentQuote, txHash string) ConfirmedPaymentInput {
	return ConfirmedPaymentInput{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: bill.TotalAmount,
		TxHash: txHash, Status: PaymentStatusConfirmed, PaymentMethod: "usdc_payment",
		CryptoQuoteID: q.ID, CryptoQuoteExactMicrounits: q.ExactMicrounits,
	}
}

const (
	quoteTxA = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	quoteTxB = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// TestApplyConfirmedPayment_ConsumesQuoteOnce is the "(b) a quote cannot be
// used twice" case at the ledger layer: the second settlement loses the
// conditional UPDATE and its payment insert rolls back with it.
func TestApplyConfirmedPayment_ConsumesQuoteOnce(t *testing.T) {
	db := setupCryptoQuoteTestDB(t)
	bill := createBillSplitBill(t, db, "quote-consume", 3000)
	q, err := IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 30_000_000, time.Now()))
	require.NoError(t, err)

	_, applied, err := ApplyConfirmedPayment(quotedPayment(bill, q, quoteTxA), nil)
	require.NoError(t, err)
	require.True(t, applied)

	consumed, err := GetCryptoPaymentQuote(q.ID)
	require.NoError(t, err)
	require.Equal(t, CryptoPaymentQuoteStatusConsumed, consumed.Status)
	require.NotNil(t, consumed.ConsumedTxHash)
	require.Equal(t, quoteTxA, *consumed.ConsumedTxHash)
	require.NotNil(t, consumed.PaymentID)
	require.NotNil(t, consumed.ConsumedAt)

	// Exact retry of the same transfer is idempotent and does not re-consume.
	_, applied, err = ApplyConfirmedPayment(quotedPayment(bill, q, quoteTxA), nil)
	require.NoError(t, err)
	require.False(t, applied)

	// A different transfer cannot reuse the consumed quote; nothing is written.
	reopened := createBillSplitBill(t, db, "quote-consume-again", 3000)
	reuse := quotedPayment(reopened, q, quoteTxB)
	_, _, err = ApplyConfirmedPayment(reuse, nil)
	require.ErrorIs(t, err, ErrCryptoQuoteUnavailable)
	require.NotErrorIs(t, err, ErrCryptoQuoteExpired, "a consumed quote is not an expired one")
	var count int64
	require.NoError(t, db.GetGorm().Model(&Payment{}).Where("tx_hash = ?", quoteTxB).Count(&count).Error)
	require.Zero(t, count, "losing settlement must roll back its payment insert")
	var reloaded Bill
	require.NoError(t, db.GetGorm().First(&reloaded, reopened.ID).Error)
	require.Zero(t, reloaded.PaidAmount)
}

// TestApplyConfirmedPayment_ExpiredQuoteRollsBack is the "(c) an expired quote
// cannot settle" case at the ledger layer.
func TestApplyConfirmedPayment_ExpiredQuoteRollsBack(t *testing.T) {
	db := setupCryptoQuoteTestDB(t)
	bill := createBillSplitBill(t, db, "quote-expired", 3000)
	issued := time.Now().Add(-45 * time.Minute)
	q, err := IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 30_000_000, issued))
	require.NoError(t, err)

	_, _, err = ApplyConfirmedPayment(quotedPayment(bill, q, quoteTxA), nil)
	require.ErrorIs(t, err, ErrCryptoQuoteUnavailable)
	// Review c1 L4: expiry is reported as expiry, not as "used".
	require.ErrorIs(t, err, ErrCryptoQuoteExpired)
	var count int64
	require.NoError(t, db.GetGorm().Model(&Payment{}).Where("bill_id = ?", bill.ID).Count(&count).Error)
	require.Zero(t, count)
	reloaded, err := GetCryptoPaymentQuote(q.ID)
	require.NoError(t, err)
	require.Equal(t, CryptoPaymentQuoteStatusActive, reloaded.Status, "rolled back with the payment")

	// A quote already swept to "expired" by another issuer is also expiry.
	require.NoError(t, db.GetGorm().Model(&CryptoPaymentQuote{}).Where("id = ?", q.ID).
		Update("status", CryptoPaymentQuoteStatusExpired).Error)
	_, _, err = ApplyConfirmedPayment(quotedPayment(bill, q, quoteTxA), nil)
	require.ErrorIs(t, err, ErrCryptoQuoteExpired)
}

func TestApplyConfirmedPayment_QuoteBoundToBillAndAmount(t *testing.T) {
	db := setupCryptoQuoteTestDB(t)
	bill := createBillSplitBill(t, db, "quote-bound", 3000)
	other := createBillSplitBill(t, db, "quote-bound-other", 3000)
	q, err := IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 30_000_000, time.Now()))
	require.NoError(t, err)

	wrongBill := quotedPayment(other, q, quoteTxA)
	_, _, err = ApplyConfirmedPayment(wrongBill, nil)
	require.ErrorIs(t, err, ErrCryptoQuoteUnavailable)
	require.NotErrorIs(t, err, ErrCryptoQuoteExpired)

	wrongAmount := quotedPayment(bill, q, quoteTxB)
	wrongAmount.CryptoQuoteExactMicrounits = q.ExactMicrounits + 1
	_, _, err = ApplyConfirmedPayment(wrongAmount, nil)
	require.ErrorIs(t, err, ErrCryptoQuoteUnavailable)

	var count int64
	require.NoError(t, db.GetGorm().Model(&Payment{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestSettleBillSplitShare_ConsumesQuoteAtomically(t *testing.T) {
	db := setupCryptoQuoteTestDB(t)
	now := time.Now()
	bill := createBillSplitBill(t, db, "quote-split", 4000)
	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID: bill.ID, GuestSessionID: "guest-quote", Mode: BillSplitModeCustom,
		AmountCents: 2000, HoldTTL: 5 * time.Minute, Now: now,
	})
	require.NoError(t, err)
	q, err := IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 20_000_000, now))
	require.NoError(t, err)

	settled, _, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID: share.ID, GuestSessionID: "guest-quote", IdempotencyKey: "split-quote",
		Tender: "crypto", TxHash: quoteTxA, PayerAddr: "crypto_guest", Now: now,
		CryptoQuoteID: q.ID, CryptoQuoteExactMicrounits: q.ExactMicrounits,
	})
	require.NoError(t, err)
	require.True(t, applied)
	consumed, err := GetCryptoPaymentQuote(q.ID)
	require.NoError(t, err)
	require.Equal(t, CryptoPaymentQuoteStatusConsumed, consumed.Status)
	require.Equal(t, *settled.PaymentID, *consumed.PaymentID)

	// A second share cannot settle on the consumed quote.
	share2, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID: bill.ID, GuestSessionID: "guest-quote-2", Mode: BillSplitModeCustom,
		AmountCents: 2000, HoldTTL: 5 * time.Minute, Now: now,
	})
	require.NoError(t, err)
	_, _, _, err = SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID: share2.ID, GuestSessionID: "guest-quote-2", IdempotencyKey: "split-quote-2",
		Tender: "crypto", TxHash: quoteTxB, PayerAddr: "crypto_guest", Now: now,
		CryptoQuoteID: q.ID, CryptoQuoteExactMicrounits: q.ExactMicrounits,
	})
	require.True(t, errors.Is(err, ErrCryptoQuoteUnavailable), "got %v", err)
	var count int64
	require.NoError(t, db.GetGorm().Model(&Payment{}).Where("tx_hash = ?", quoteTxB).Count(&count).Error)
	require.Zero(t, count)
}

// TestFindPaymentTxHashHolder names the bill that already recorded a transfer,
// whatever spelling the replay uses, so the guest crypto handler can report a
// replayed transfer as already recorded rather than as a wrong-amount payment.
func TestFindPaymentTxHashHolder(t *testing.T) {
	db := setupCryptoQuoteTestDB(t)
	now := time.Now()
	bill := createBillSplitBill(t, db, "quote-holder", 3000)
	q, err := IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 30_000_000, now))
	require.NoError(t, err)
	payment, applied, err := ApplyConfirmedPayment(quotedPayment(bill, q, quoteTxA), nil)
	require.NoError(t, err)
	require.True(t, applied)

	for _, spelling := range []string{quoteTxA, "0X" + strings.ToUpper(quoteTxA[2:]), quoteTxA[2:], "  " + quoteTxA + "\n"} {
		holder, err := FindPaymentTxHashHolder(spelling)
		require.NoError(t, err, spelling)
		require.NotNil(t, holder, spelling)
		require.Equal(t, payment.ID, holder.PaymentID)
		require.Equal(t, bill.ID, holder.BillID)
		require.Equal(t, bill.BusinessID, holder.BusinessID)
		require.Equal(t, bill.BillNumber, holder.BillNumber)
	}

	// A split share records its transfer on payments too.
	splitBill := createBillSplitBill(t, db, "quote-holder-split", 4000)
	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID: splitBill.ID, GuestSessionID: "guest-holder", Mode: BillSplitModeCustom,
		AmountCents: 2000, HoldTTL: 5 * time.Minute, Now: now,
	})
	require.NoError(t, err)
	splitQuote, err := IssueCryptoPaymentQuote(quoteInput(splitBill, quoteTestWallet, 20_000_000, now))
	require.NoError(t, err)
	_, _, applied, err = SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID: share.ID, GuestSessionID: "guest-holder", IdempotencyKey: "split-holder",
		Tender: "crypto", TxHash: quoteTxB, PayerAddr: "crypto_guest", Now: now,
		CryptoQuoteID: splitQuote.ID, CryptoQuoteExactMicrounits: splitQuote.ExactMicrounits,
	})
	require.NoError(t, err)
	require.True(t, applied)
	holder, err := FindPaymentTxHashHolder(quoteTxB)
	require.NoError(t, err)
	require.NotNil(t, holder)
	require.Equal(t, splitBill.ID, holder.BillID)
	require.Equal(t, splitBill.BillNumber, holder.BillNumber)

	for _, unknown := range []string{"0x" + strings.Repeat("c", 64), "", "   "} {
		holder, err := FindPaymentTxHashHolder(unknown)
		require.NoError(t, err)
		require.Nilf(t, holder, "%q must not match any payment", unknown)
	}
}
