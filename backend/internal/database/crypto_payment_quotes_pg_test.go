package database

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestCryptoPaymentQuotes_Postgres runs on the genesis schema in real Postgres
// and exercises the two races the payer binding depends on with genuinely
// concurrent connections: issuers colliding on one exact amount (partial
// unique index) and settlements racing to consume one quote (conditional
// UPDATE).
func TestCryptoPaymentQuotes_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}
	gdb := startGenesisPostgres(t).DB
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(16)

	prev := db
	SetTestDB(gdb)
	t.Cleanup(func() { SetTestDB(prev) })

	businessID := seedGenesisBusiness(t, gdb, "crypto-quotes")
	billSeq := 0
	newBill := func() *Bill {
		billSeq++
		bill := &Bill{BusinessID: businessID, BillNumber: fmt.Sprintf("CQ-%d", billSeq), Status: BillStatusOpen, Items: "[]"}
		require.NoError(t, gdb.Create(bill).Error)
		return bill
	}

	t.Run("checks_reject_malformed_rows", func(t *testing.T) {
		bill := newBill()
		now := time.Now().UTC()
		base := CryptoPaymentQuote{
			BillID: bill.ID, BusinessID: businessID, ChainID: 8453,
			SettlementAddress: NormalizeCryptoQuoteAddress(quoteTestWallet), PaymentMethod: "usdc_payment",
			BaseMicrounits: 1_000_000, OffsetMicrounits: 5, ExactMicrounits: 1_000_005,
			Status: CryptoPaymentQuoteStatusExpired, IssuedAt: now, ExpiresAt: now.Add(time.Minute),
		}
		mixedCase := base
		mixedCase.SettlementAddress = "0xABCDEF1111111111111111111111111111111111"
		require.Error(t, gdb.Create(&mixedCase).Error, "address must be stored lowercase")
		zeroOffset := base
		zeroOffset.OffsetMicrounits, zeroOffset.ExactMicrounits = 0, base.BaseMicrounits
		require.Error(t, gdb.Create(&zeroOffset).Error, "offset 0 would equal the unbound amount")
		skewed := base
		skewed.ExactMicrounits = base.ExactMicrounits + 1
		require.Error(t, gdb.Create(&skewed).Error, "exact must equal base + offset")
		consumedNoHash := base
		consumedNoHash.Status = CryptoPaymentQuoteStatusConsumed
		require.Error(t, gdb.Create(&consumedNoHash).Error, "consumed rows carry their tx hash")
		rawIP := base
		rawIPKey := "203.0.113.9"
		rawIP.ClientKey = &rawIPKey
		require.Error(t, gdb.Create(&rawIP).Error, "client_key holds a keyed hash, never a raw IP")
		ok := base
		hashedKey := quoteClientA
		ok.ClientKey = &hashedKey
		require.NoError(t, gdb.Create(&ok).Error)
	})

	t.Run("concurrent_issuers_get_distinct_amounts", func(t *testing.T) {
		fixedCryptoQuoteOffsets(t)
		now := time.Now().UTC()
		bills := make([]*Bill, cryptoQuoteInsertAttempts)
		for i := range bills {
			bills[i] = newBill()
		}
		results := make([]*CryptoPaymentQuote, len(bills))
		errs := make([]error, len(bills))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i, bill := range bills {
			wg.Add(1)
			go func(i int, bill *Bill) {
				defer wg.Done()
				<-start
				results[i], errs[i] = IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 42_000_000, now))
			}(i, bill)
		}
		close(start)
		wg.Wait()
		seen := map[int64]bool{}
		for i := range bills {
			require.NoError(t, errs[i])
			require.Falsef(t, seen[results[i].ExactMicrounits], "exact amount %d issued twice", results[i].ExactMicrounits)
			seen[results[i].ExactMicrounits] = true
		}
	})

	t.Run("per_client_cap_counts_only_that_clients_live_quotes", func(t *testing.T) {
		bill := newBill()
		now := time.Now().UTC()
		withKey := func(key string) IssueCryptoPaymentQuoteInput {
			in := quoteInput(bill, quoteTestWallet, 13_000_000, now)
			in.ClientKey = key
			return in
		}
		for i := int64(0); i < CryptoQuoteMaxActivePerClient; i++ {
			_, err := IssueCryptoPaymentQuote(withKey(quoteClientA))
			require.NoError(t, err)
		}
		_, err := IssueCryptoPaymentQuote(withKey(quoteClientA))
		require.ErrorIs(t, err, ErrCryptoQuoteClientLimitReached)
		_, err = IssueCryptoPaymentQuote(withKey(quoteClientB))
		require.NoError(t, err, "another client on the bill still gets a quote")
		_, err = IssueCryptoPaymentQuote(withKey(""))
		require.NoError(t, err, "a request without a client key is only bound by the per-bill cap")
	})

	t.Run("racing_settlements_consume_once", func(t *testing.T) {
		bill := newBill()
		q, err := IssueCryptoPaymentQuote(quoteInput(bill, quoteTestWallet, 77_000_000, time.Now()))
		require.NoError(t, err)
		paymentIDs := make([]uint, 2)
		for i := range paymentIDs {
			require.NoError(t, gdb.Raw(`INSERT INTO payments (bill_id, payer_addr, amount) VALUES (?, '', 0) RETURNING id`, bill.ID).Scan(&paymentIDs[i]).Error)
		}
		hashes := []string{quoteTxA, quoteTxB}
		errs := make([]error, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range errs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				errs[i] = gdb.Transaction(func(tx *gorm.DB) error {
					return consumeCryptoPaymentQuoteTx(tx, q.ID, bill.ID, q.ExactMicrounits, hashes[i], paymentIDs[i], time.Now())
				})
			}(i)
		}
		close(start)
		wg.Wait()
		wins := 0
		for _, err := range errs {
			if err == nil {
				wins++
				continue
			}
			require.True(t, errors.Is(err, ErrCryptoQuoteUnavailable), "loser must see ErrCryptoQuoteUnavailable, got %v", err)
		}
		require.Equal(t, 1, wins, "exactly one settlement consumes the quote")
	})

}
