package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupBillSplitTestDB(t testing.TB, gormLogger logger.Interface) *DB {
	t.Helper()

	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()), time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	require.NoError(t, gormDB.AutoMigrate(
		&Business{},
		&Bill{},
		&Payment{},
		&PaymentRefundDestination{},
		&AlternativePayment{},
		&BusinessMilestoneEvent{},
		&BusinessRevenueAggregate{},
		&BillHistoryEvent{},
		&BillSplitShare{},
		&CashRegisterSession{},
		&CashRegisterMovement{},
	))
	require.NoError(t, createSQLiteBillSplitBillItemsTable(gormDB))

	SetTestDB(gormDB)
	return GetDBWrapper()
}

func TestSettleBillSplitSharePersistsRefundEvidenceAtomically(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-refund-evidence", 4000)
	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID: bill.ID, GuestSessionID: "guest-evidence", Mode: BillSplitModeCustom,
		AmountCents: 4000, HoldTTL: 5 * time.Minute, Now: now,
	})
	require.NoError(t, err)
	logRef := "0xsplit-evidence:1"

	settled, _, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID: share.ID, GuestSessionID: "guest-evidence", IdempotencyKey: "split-evidence",
		Tender: "crypto", TxHash: "0xsplit-evidence", PayerAddr: "crypto_guest", Now: now.Add(time.Minute),
		RefundDestination: &PaymentRefundDestination{
			ChainID: 84532, Token: "USDC", AmountBaseUnits: 4_000_000,
			RefundAddress: "0xdead00000000000000000000000000000000beef",
			EvidenceType:  RefundEvidenceTransferLog, LogRef: &logRef, VerifiedAt: now,
		},
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.NotNil(t, settled.PaymentID)
	dest, err := GetPaymentRefundDestination(*settled.PaymentID)
	require.NoError(t, err)
	require.Equal(t, *settled.PaymentID, dest.PaymentID)

	// A retry can repair evidence missing from a pre-upgrade/crash-window split
	// settlement without reapplying the payment.
	require.NoError(t, db.GetGorm().Where("payment_id = ?", *settled.PaymentID).Delete(&PaymentRefundDestination{}).Error)
	replayed, _, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID: share.ID, GuestSessionID: "guest-evidence", IdempotencyKey: "split-evidence",
		Tender: "crypto", TxHash: "0xsplit-evidence", PayerAddr: "crypto_guest", Now: now.Add(2 * time.Minute),
		RefundDestination: &PaymentRefundDestination{
			ChainID: 84532, Token: "USDC", AmountBaseUnits: 4_000_000,
			RefundAddress: "0xdead00000000000000000000000000000000beef",
			EvidenceType:  RefundEvidenceTransferLog, LogRef: &logRef, VerifiedAt: now,
		},
	})
	require.NoError(t, err)
	require.False(t, applied)
	require.Equal(t, settled.ID, replayed.ID)
	_, err = GetPaymentRefundDestination(*settled.PaymentID)
	require.NoError(t, err)
}

// TestSettleBillSplitSharePersistsBlockEvidence pins that a crypto split-share
// settlement records block_number/block_hash on its Payment row so it is swept
// by the reorg reconciler identically to a non-split crypto payment
// (review finding #3/#5).
func TestSettleBillSplitSharePersistsBlockEvidence(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-block-evidence", 4000)
	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID: bill.ID, GuestSessionID: "guest-block", Mode: BillSplitModeCustom,
		AmountCents: 4000, HoldTTL: 5 * time.Minute, Now: now,
	})
	require.NoError(t, err)

	bn := int64(778899)
	bh := "0xsplitblockhash"
	settled, _, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID: share.ID, GuestSessionID: "guest-block", IdempotencyKey: "split-block",
		Tender: "crypto", TxHash: "0xsplit-block", PayerAddr: "crypto_guest", Now: now.Add(time.Minute),
		BlockNumber: &bn, BlockHash: &bh,
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.NotNil(t, settled.PaymentID)

	var p Payment
	require.NoError(t, db.GetGorm().First(&p, *settled.PaymentID).Error)
	require.NotNil(t, p.BlockHash, "crypto split-share settlement must persist its block hash")
	require.Equal(t, "0xsplitblockhash", *p.BlockHash)
	require.NotNil(t, p.BlockNumber)
	require.Equal(t, int64(778899), *p.BlockNumber)
}

func createSQLiteBillSplitBillItemsTable(db *gorm.DB) error {
	return db.Exec(`
CREATE TABLE bill_items (
	id text PRIMARY KEY,
	bill_id integer NOT NULL,
	menu_item_id text DEFAULT '',
	name text NOT NULL,
	price real NOT NULL,
	quantity integer NOT NULL,
	options text,
	item_type text DEFAULT 'menu_item',
	bundle_id integer,
	parent_bundle_id integer,
	source_offer_id integer,
	order_id integer,
	subtotal real NOT NULL,
	created_at datetime
)`).Error
}

func createBillSplitBill(t testing.TB, db *DB, number string, totalCents int64) *Bill {
	t.Helper()

	business := &Business{
		BusinessId:     fmt.Sprintf("split-%s-%d", number, time.Now().UnixNano()),
		Name:           "Split Test",
		OwnerAddress:   "0xsplitowner",
		SettlementAddr: "0xsettlement",
		TippingAddr:    "0xtipping",
		IsActive:       true,
	}
	require.NoError(t, db.GetGorm().Create(business).Error)

	bill := &Bill{
		BusinessID:       business.ID,
		BillNumber:       number,
		Items:            "[]",
		Subtotal:         totalCents,
		TaxAmount:        0,
		ServiceFeeAmount: 0,
		TotalAmount:      totalCents,
		PaidAmount:       0,
		Status:           BillStatusOpen,
		SettlementAddr:   business.SettlementAddr,
		TippingAddr:      business.TippingAddr,
	}
	require.NoError(t, db.GetGorm().Create(bill).Error)
	return bill
}

func createBillSplitBillWithItems(t testing.TB, db *DB, number string, subtotalCents, taxCents, serviceCents int64, items []BillItem) *Bill {
	t.Helper()

	bill := createBillSplitBill(t, db, number, subtotalCents+taxCents+serviceCents)
	require.NoError(t, db.GetGorm().Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"subtotal":           subtotalCents,
		"tax_amount":         taxCents,
		"service_fee_amount": serviceCents,
		"total_amount":       subtotalCents + taxCents + serviceCents,
	}).Error)
	bill.Subtotal = subtotalCents
	bill.TaxAmount = taxCents
	bill.ServiceFeeAmount = serviceCents
	bill.TotalAmount = subtotalCents + taxCents + serviceCents

	for i := range items {
		items[i].BillID = bill.ID
		if items[i].ID == "" {
			items[i].ID = fmt.Sprintf("item-%d", i+1)
		}
	}
	require.NoError(t, db.GetGorm().Create(&items).Error)
	return bill
}

func TestHoldBillSplitShareClampsCustomAmountToAvailableBalance(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-custom-clamp", 10000)

	firstShare, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		DisplayName:    "Guest 1",
		Mode:           BillSplitModeCustom,
		AmountCents:    6000,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.Equal(t, BillSplitShareStatusHeld, firstShare.Status)
	require.Equal(t, int64(4000), state.AvailableCents)

	secondShare, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-2",
		DisplayName:    "Guest 2",
		Mode:           BillSplitModeCustom,
		AmountCents:    5000,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.Equal(t, BillSplitShareStatusHeld, secondShare.Status)
	require.Equal(t, int64(4000), secondShare.AmountCents)
	require.Equal(t, int64(10000), state.HeldCents)
	require.Equal(t, int64(0), state.AvailableCents)

	var shareCount int64
	require.NoError(t, db.GetGorm().Model(&BillSplitShare{}).Where("bill_id = ?", bill.ID).Count(&shareCount).Error)
	require.EqualValues(t, 2, shareCount)

	var reloaded Bill
	require.NoError(t, db.GetGorm().First(&reloaded, bill.ID).Error)
	require.Equal(t, int64(0), reloaded.PaidAmount)
	require.Equal(t, BillStatusOpen, reloaded.Status)
}

func TestCreateConfirmedAlternativePaymentReleasesHeldSplitSharesWhenCashierClosesResidual(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Now().UTC()
	bill := createBillSplitBill(t, db, "split-cashier-residual", 10000)

	_, applied, err := CreateConfirmedAlternativePayment(&AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "first-payer",
		Amount:          7000,
		PaymentMethod:   PaymentMethodCash,
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)

	share, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-phone-died",
		DisplayName:    "Guest 2",
		Mode:           BillSplitModeCustom,
		CoverRemaining: true,
		IdempotencyKey: "hold-residual-before-cashier",
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3000), share.AmountCents)
	require.Equal(t, int64(3000), state.HeldCents)
	require.Equal(t, int64(0), state.AvailableCents)

	updatedBill, applied, err := CreateConfirmedAlternativePayment(&AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "cashier-override",
		Amount:          3000,
		PaymentMethod:   PaymentMethodCash,
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, BillStatusPaid, updatedBill.Status)
	require.Equal(t, int64(10000), updatedBill.PaidAmount)

	var reloadedShare BillSplitShare
	require.NoError(t, db.GetGorm().First(&reloadedShare, share.ID).Error)
	require.Equal(t, BillSplitShareStatusReleased, reloadedShare.Status)
	require.Nil(t, reloadedShare.HoldExpiresAt)
	require.NotNil(t, reloadedShare.ReleasedAt)

	state, err = GetBillSplitStateByBillID(bill.ID, now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, int64(10000), state.PaidCents)
	require.Equal(t, int64(0), state.HeldCents)
	require.Equal(t, int64(0), state.AvailableCents)
}

func TestHoldBillSplitShareCoverRemainingUsesAvailableBalance(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-cover-remaining", 10000)

	_, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		Mode:           BillSplitModeCustom,
		AmountCents:    6000,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.Equal(t, int64(4000), state.AvailableCents)

	coverShare, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-2",
		Mode:           BillSplitModeCustom,
		CoverRemaining: true,
		IdempotencyKey: "cover-remaining-1",
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.Equal(t, int64(4000), coverShare.AmountCents)
	require.Equal(t, int64(0), state.AvailableCents)
	require.Equal(t, int64(10000), state.HeldCents)

	retryShare, retryState, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-2",
		Mode:           BillSplitModeCustom,
		CoverRemaining: true,
		IdempotencyKey: "cover-remaining-1",
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.Equal(t, coverShare.ID, retryShare.ID)
	require.Equal(t, int64(4000), retryShare.AmountCents)
	require.Equal(t, int64(0), retryState.AvailableCents)

	var shareCount int64
	require.NoError(t, db.GetGorm().Model(&BillSplitShare{}).Where("bill_id = ?", bill.ID).Count(&shareCount).Error)
	require.EqualValues(t, 2, shareCount)
}

// PG-23: reference equalShareAmounts seats sum exactly; sequential re-split
// covered=1 claims sum exactly; simultaneous covered=1 previews never over-sum.
func TestCalculateEqualSplitCents_CoveredSeatsSumExact(t *testing.T) {
	totals := []int64{1, 2, 3, 7, 10, 100, 101, 999, 1000, 10001, 123456}
	for n := 2; n <= 12; n++ {
		for _, total := range totals {
			n, total := n, total
			t.Run(fmt.Sprintf("N=%d_T=%d", n, total), func(t *testing.T) {
				amounts := equalShareAmounts(total, n)
				var sum int64
				for _, a := range amounts {
					sum += a
				}
				require.Equal(t, total, sum, "reference seat amounts must sum to total")

				// Full cover always returns the whole pool.
				full, err := calculateEqualSplitCents(total, n, n)
				require.NoError(t, err)
				require.Equal(t, total, full)

				// N sequential covered=1 re-splits of remaining sum exactly.
				available := total
				remainingPeople := n
				var sequential int64
				for remainingPeople > 0 && available > 0 {
					amt, err := calculateEqualSplitCents(available, remainingPeople, 1)
					require.NoError(t, err)
					require.Greater(t, amt, int64(0))
					sequential += amt
					available -= amt
					remainingPeople--
				}
				require.Equal(t, total, sequential)
				require.Equal(t, int64(0), available)

				// Groups of covered that tile N also sum exact via re-split.
				// When total < n, some trailing seats are zero — stop once the
				// pool is exhausted rather than requiring n successful holds.
				for covered := 1; covered <= n; covered++ {
					if n%covered != 0 {
						continue
					}
					avail := total
					peopleLeft := n
					var groupSum int64
					claims := n / covered
					for c := 0; c < claims && avail > 0 && peopleLeft >= covered; c++ {
						amt, err := calculateEqualSplitCents(avail, peopleLeft, covered)
						require.NoError(t, err)
						groupSum += amt
						avail -= amt
						peopleLeft -= covered
					}
					require.Equal(t, total, groupSum)
					require.Equal(t, int64(0), avail)
				}
			})
		}
	}
}

// Simultaneous N covered=1 previews (no seats taken yet) must not over-sum.
func TestCalculateEqualSplitCents_SimultaneousCoveredOneDoesNotOverSum(t *testing.T) {
	for n := 2; n <= 12; n++ {
		for _, total := range []int64{100, 101, 1000, 10001} {
			var previewSum int64
			for i := 0; i < n; i++ {
				amt, err := calculateEqualSplitCents(total, n, 1)
				require.NoError(t, err)
				previewSum += amt
			}
			require.LessOrEqual(t, previewSum, total,
				"N simultaneous covered=1 previews must not over-sum (N=%d T=%d sum=%d)", n, total, previewSum)
		}
	}
}

func TestHoldBillSplitShareEqual_SequentialCoveredOneSumsExact(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 8, 5, 15, 0, 0, 0, time.UTC)
	const total int64 = 10001
	const people = 3
	bill := createBillSplitBill(t, db, "split-equal-exact", total)

	var held int64
	for i := 0; i < people; i++ {
		share, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
			BillID:         bill.ID,
			GuestSessionID: fmt.Sprintf("guest-%d", i+1),
			Mode:           BillSplitModeEqual,
			NumPeople:      people,
			SharesCovered:  1,
			HoldTTL:        5 * time.Minute,
			Now:            now.Add(time.Duration(i) * time.Second),
		})
		require.NoError(t, err)
		held += share.AmountCents
		require.Equal(t, held, state.HeldCents)
		require.Equal(t, total-held, state.AvailableCents)
	}
	require.Equal(t, total, held)
}

func TestHoldBillSplitShareEqualUsesAvailableBalance(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-equal-available", 10000)

	_, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		Mode:           BillSplitModeCustom,
		AmountCents:    6000,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.Equal(t, int64(4000), state.AvailableCents)

	equalShare, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-2",
		Mode:           BillSplitModeEqual,
		NumPeople:      3,
		SharesCovered:  2,
		HoldTTL:        5 * time.Minute,
		Now:            now.Add(time.Second),
	})
	require.NoError(t, err)
	// PG-23 floor re-split: floor(4000/3)*2 = 2666; remainder stays for the last seat.
	require.Equal(t, int64(2666), equalShare.AmountCents)
	require.Equal(t, int64(8666), state.HeldCents)
	require.Equal(t, int64(1334), state.AvailableCents)

	var shareCount int64
	require.NoError(t, db.GetGorm().Model(&BillSplitShare{}).Where("bill_id = ?", bill.ID).Count(&shareCount).Error)
	require.EqualValues(t, 2, shareCount)
}

func TestHoldBillSplitShareIsIdempotentForSameGuestRequest(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-hold-idempotent", 10000)

	firstShare, firstState, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		Mode:           BillSplitModeCustom,
		AmountCents:    3500,
		IdempotencyKey: "hold-request-1",
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	secondShare, secondState, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		Mode:           BillSplitModeCustom,
		AmountCents:    3500,
		IdempotencyKey: "hold-request-1",
		HoldTTL:        5 * time.Minute,
		Now:            now.Add(time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, firstShare.ID, secondShare.ID)
	require.Equal(t, int64(3500), firstState.HeldCents)
	require.Equal(t, int64(3500), secondState.HeldCents)

	var shareCount int64
	require.NoError(t, db.GetGorm().Model(&BillSplitShare{}).Where("bill_id = ?", bill.ID).Count(&shareCount).Error)
	require.EqualValues(t, 1, shareCount)
}

func TestHoldBillSplitShareItemsProratesRemainingAfterPartialPayment(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBillWithItems(t, db, "split-items-partial", 3100, 275, 124, []BillItem{
		{ID: "demo-tacos", Name: "Market Tacos", Price: 21, Quantity: 1, Subtotal: 21},
		{ID: "demo-tea", Name: "Iced Tea", Price: 5, Quantity: 2, Subtotal: 10},
	})
	require.NoError(t, db.GetGorm().Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]any{
		"paid_amount": int64(1749),
		"status":      BillStatusPartial,
	}).Error)

	tea, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-tea",
		Mode:           BillSplitModeItems,
		ClaimedItemIDs: []string{"demo-tea"},
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.Equal(t, int64(565), tea.AmountCents, "tea hold must match remaining preview 17.50×10/31")
	require.Equal(t, int64(565), state.HeldCents)
	require.Equal(t, int64(1185), state.AvailableCents)

	require.NoError(t, db.GetGorm().Model(&BillSplitShare{}).Where("id = ?", tea.ID).
		Updates(map[string]any{"status": BillSplitShareStatusReleased, "released_at": now}).Error)

	tacos, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-tacos",
		Mode:           BillSplitModeItems,
		ClaimedItemIDs: []string{"demo-tacos"},
		HoldTTL:        5 * time.Minute,
		Now:            now.Add(time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, int64(1185), tacos.AmountCents, "taco hold must fit in remaining 17.50")
	require.Equal(t, int64(1185), state.HeldCents)
	require.Equal(t, int64(565), state.AvailableCents)
}

func TestHoldBillSplitShareItemsFractionallyAllocatesTaxAndService(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBillWithItems(t, db, "split-items-fraction", 10000, 1000, 500, []BillItem{
		{ID: "shared-bottle", Name: "Shared Bottle", Price: 60, Quantity: 1, Subtotal: 60},
		{ID: "steak", Name: "Steak", Price: 40, Quantity: 1, Subtotal: 40},
	})

	share, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:           bill.ID,
		GuestSessionID:   "guest-1",
		Mode:             BillSplitModeItems,
		ClaimedItemIDs:   []string{"shared-bottle"},
		ClaimedFractions: map[string]string{"shared-bottle": "1/2"},
		HoldTTL:          5 * time.Minute,
		Now:              now,
	})
	require.NoError(t, err)

	require.Equal(t, int64(3450), share.AmountCents, "half of a $60 item plus proportional $10 tax and $5 service")
	require.Equal(t, "1/2", share.ClaimedFractions["shared-bottle"])
	require.Equal(t, int64(3450), state.HeldCents)
	require.Equal(t, int64(8050), state.AvailableCents)
}

func TestHoldBillSplitShareFractionalItemsReconcileToTotalCents(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBillWithItems(t, db, "split-items-rounding", 100, 1, 0, []BillItem{
		{ID: "shared-dessert", Name: "Shared Dessert", Price: 1, Quantity: 1, Subtotal: 1},
	})

	first, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:           bill.ID,
		GuestSessionID:   "guest-1",
		Mode:             BillSplitModeItems,
		ClaimedItemIDs:   []string{"shared-dessert"},
		ClaimedFractions: map[string]string{"shared-dessert": "1/3"},
		HoldTTL:          5 * time.Minute,
		Now:              now,
	})
	require.NoError(t, err)
	require.Equal(t, int64(33), first.AmountCents)
	require.Equal(t, int64(68), state.AvailableCents)

	second, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:           bill.ID,
		GuestSessionID:   "guest-2",
		Mode:             BillSplitModeItems,
		ClaimedItemIDs:   []string{"shared-dessert"},
		ClaimedFractions: map[string]string{"shared-dessert": "1/3"},
		HoldTTL:          5 * time.Minute,
		Now:              now.Add(time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, int64(35), second.AmountCents)
	require.Equal(t, int64(33), state.AvailableCents)

	third, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:           bill.ID,
		GuestSessionID:   "guest-3",
		Mode:             BillSplitModeItems,
		ClaimedItemIDs:   []string{"shared-dessert"},
		ClaimedFractions: map[string]string{"shared-dessert": "1/3"},
		HoldTTL:          5 * time.Minute,
		Now:              now.Add(2 * time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, int64(33), third.AmountCents)
	require.Equal(t, int64(101), state.HeldCents)
	require.Equal(t, int64(0), state.AvailableCents)
}

func TestHoldBillSplitShareItemsRejectsOverClaimedFraction(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBillWithItems(t, db, "split-items-overclaim", 10000, 1000, 500, []BillItem{
		{ID: "shared-bottle", Name: "Shared Bottle", Price: 60, Quantity: 1, Subtotal: 60},
		{ID: "steak", Name: "Steak", Price: 40, Quantity: 1, Subtotal: 40},
	})

	_, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:           bill.ID,
		GuestSessionID:   "guest-1",
		Mode:             BillSplitModeItems,
		ClaimedItemIDs:   []string{"shared-bottle"},
		ClaimedFractions: map[string]string{"shared-bottle": "1/2"},
		HoldTTL:          5 * time.Minute,
		Now:              now,
	})
	require.NoError(t, err)

	_, _, err = HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:           bill.ID,
		GuestSessionID:   "guest-2",
		Mode:             BillSplitModeItems,
		ClaimedItemIDs:   []string{"shared-bottle"},
		ClaimedFractions: map[string]string{"shared-bottle": "2/3"},
		HoldTTL:          5 * time.Minute,
		Now:              now.Add(time.Second),
	})
	require.ErrorIs(t, err, ErrSplitItemUnavailable)
}

// TestHoldBillSplitShareItemsNetsOfferDiscountLines is the regression for the
// guest item-split + auto-offer promo bug: raw positive food subtotals plus
// prorated tax/fee oversubscribed bill.TotalAmount, so the second guest's
// item hold 409'd while the discount line itself could not be held.
// Repro numbers mirror live demo tea($5)+dessert($12) with 15% off (-$2.55).
func TestHoldBillSplitShareItemsNetsOfferDiscountLines(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	// subtotal 14.45, tax 1.28, service 0.58 → total 16.31
	bill := createBillSplitBillWithItems(t, db, "split-items-discount", 1445, 128, 58, []BillItem{
		{ID: "item-tea", Name: "Iced Tea", Price: 5, Quantity: 1, Subtotal: 5, ItemType: "menu_item"},
		{ID: "item-dessert", Name: "Chocolate Tart", Price: 12, Quantity: 1, Subtotal: 12, ItemType: "menu_item"},
		{ID: "item-discount", Name: "Weekday Lunch 15% Off", Price: -2.55, Quantity: 1, Subtotal: -2.55, ItemType: "discount"},
	})

	// Pre-fix: gross food holds without netting would be 1354+564=1918 > 1631.
	dessert, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-dessert",
		DisplayName:    "Dessert",
		Mode:           BillSplitModeItems,
		ClaimedItemIDs: []string{"item-dessert"},
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	// Net dessert base = 1445 * 1200/1700 = 1020; tax 90; service 41 → 1151.
	require.Equal(t, int64(1151), dessert.AmountCents)
	require.Less(t, dessert.AmountCents, bill.TotalAmount)
	require.Equal(t, int64(1631-1151), state.AvailableCents)

	tea, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-tea",
		DisplayName:    "Tea",
		Mode:           BillSplitModeItems,
		ClaimedItemIDs: []string{"item-tea"},
		HoldTTL:        5 * time.Minute,
		Now:            now.Add(time.Second),
	})
	require.NoError(t, err, "second positive food claim must succeed after discount netting")
	require.Equal(t, int64(480), tea.AmountCents)
	require.Equal(t, int64(1631), state.HeldCents)
	require.Equal(t, int64(0), state.AvailableCents)
	require.Equal(t, bill.TotalAmount, dessert.AmountCents+tea.AmountCents)
}

func TestHoldBillSplitShareItemsRejectsDiscountLineClaim(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBillWithItems(t, db, "split-items-disc-only", 1445, 128, 58, []BillItem{
		{ID: "item-tea", Name: "Iced Tea", Price: 5, Quantity: 1, Subtotal: 5, ItemType: "menu_item"},
		{ID: "item-dessert", Name: "Chocolate Tart", Price: 12, Quantity: 1, Subtotal: 12, ItemType: "menu_item"},
		{ID: "item-discount", Name: "Weekday Lunch 15% Off", Price: -2.55, Quantity: 1, Subtotal: -2.55, ItemType: "discount"},
	})

	_, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-disc",
		Mode:           BillSplitModeItems,
		ClaimedItemIDs: []string{"item-discount"},
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.ErrorIs(t, err, ErrSplitItemUnavailable)
}

// PG-24: combo children (item_type=bundle_item) are never independently
// claimable. Claiming the parent bundle line holds the full combo amount.
func TestIsNonClaimableSplitBillItem_BundleChild(t *testing.T) {
	require.True(t, IsNonClaimableSplitBillItem("bundle_item", 500))
	require.True(t, IsNonClaimableSplitBillItem("BUNDLE_ITEM", 0))
	require.False(t, IsNonClaimableSplitBillItem("bundle", 1500))
	require.False(t, IsNonClaimableSplitBillItem("menu_item", 500))
}

func TestHoldBillSplitShareItems_ClaimParentBundleNotChildren(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	// Parent carries the combo price; children are expansion lines (0 or
	// kitchen-only). Guests must claim the parent only.
	bill := createBillSplitBillWithItems(t, db, "split-combo-parent", 1500, 150, 50, []BillItem{
		{ID: "combo-parent", Name: "Burger Combo", Price: 15, Quantity: 1, Subtotal: 15, ItemType: "bundle"},
		{ID: "combo-child-burger", Name: "Burger", Price: 0, Quantity: 1, Subtotal: 0, ItemType: "bundle_item"},
		{ID: "combo-child-fries", Name: "Fries", Price: 0, Quantity: 1, Subtotal: 0, ItemType: "bundle_item"},
	})

	_, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-child",
		Mode:           BillSplitModeItems,
		ClaimedItemIDs: []string{"combo-child-burger"},
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.ErrorIs(t, err, ErrSplitItemUnavailable)

	share, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-parent",
		Mode:           BillSplitModeItems,
		ClaimedItemIDs: []string{"combo-parent"},
		HoldTTL:        5 * time.Minute,
		Now:            now.Add(time.Second),
	})
	require.NoError(t, err)
	// Full bill remaining: 1500+150+50 = 1700 cents when parent is the only claimable base.
	require.Equal(t, int64(1700), share.AmountCents)
	require.Equal(t, int64(0), state.AvailableCents)
	require.Equal(t, []string{"combo-parent"}, share.ClaimedItemIDs)
}

func TestNetClaimableItemBaseCents_ExcludesBundleChildren(t *testing.T) {
	bases, err := netClaimableItemBaseCents([]BillItem{
		{ID: "combo-parent", Subtotal: 15, ItemType: "bundle"},
		{ID: "combo-child", Subtotal: 0, ItemType: "bundle_item"},
		{ID: "side", Subtotal: 5, ItemType: "menu_item"},
	}, 2000)
	require.NoError(t, err)
	_, hasChild := bases["combo-child"]
	require.False(t, hasChild, "bundle children must not be independently claimable")
	require.Contains(t, bases, "combo-parent")
	require.Contains(t, bases, "side")
	require.Equal(t, int64(2000), bases["combo-parent"]+bases["side"])
}

func TestNetClaimableItemBaseCentsAbsorbsDiscount(t *testing.T) {
	bases, err := netClaimableItemBaseCents([]BillItem{
		{ID: "tea", Subtotal: 5, ItemType: "menu_item"},
		{ID: "dessert", Subtotal: 12, ItemType: "menu_item"},
		{ID: "offer", Subtotal: -2.55, ItemType: "discount"},
	}, 1445)
	require.NoError(t, err)
	require.Equal(t, int64(425), bases["tea"])
	require.Equal(t, int64(1020), bases["dessert"])
	_, hasDisc := bases["offer"]
	require.False(t, hasDisc)
	require.Equal(t, int64(1445), bases["tea"]+bases["dessert"])
}

func TestReleaseExpiredBillSplitSharesFreesAvailability(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-expiry", 10000)

	_, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		Mode:           BillSplitModeEqual,
		AmountCents:    7000,
		HoldTTL:        time.Minute,
		Now:            now.Add(-2 * time.Minute),
	})
	require.NoError(t, err)

	released, states, err := ReleaseExpiredBillSplitSharesWithStates(now)
	require.NoError(t, err)
	require.EqualValues(t, 1, released)
	require.Len(t, states, 1)
	require.Equal(t, int64(10000), states[0].AvailableCents)

	share, state, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-2",
		Mode:           BillSplitModeCustom,
		AmountCents:    10000,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), state.AvailableCents)
	require.Equal(t, int64(10000), share.AmountCents)
}

func TestSettleBillSplitShareAppliesPaymentAndIsIdempotent(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-idempotent", 10000)

	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		DisplayName:    "Sara",
		Mode:           BillSplitModeCustom,
		AmountCents:    4000,
		TipCents:       500,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	settled, updatedBill, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID:        share.ID,
		GuestSessionID: "guest-1",
		IdempotencyKey: "request-1",
		Tender:         "crypto",
		TxHash:         "split-tx-1",
		PayerAddr:      "0xguest",
		Now:            now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, BillSplitShareStatusSettled, settled.Status)
	require.NotNil(t, settled.PaymentID)
	require.Equal(t, int64(4000), updatedBill.PaidAmount)
	require.Equal(t, int64(500), updatedBill.TipAmount)
	require.Equal(t, BillStatusPartial, updatedBill.Status)

	settledAgain, updatedAgain, appliedAgain, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID:        share.ID,
		GuestSessionID: "guest-1",
		IdempotencyKey: "request-1",
		Tender:         "crypto",
		TxHash:         "split-tx-1",
		PayerAddr:      "0xguest",
		Now:            now.Add(2 * time.Minute),
	})
	require.NoError(t, err)
	require.False(t, appliedAgain)
	require.Equal(t, settled.ID, settledAgain.ID)
	require.Equal(t, int64(4000), updatedAgain.PaidAmount)
	require.Equal(t, int64(500), updatedAgain.TipAmount)

	var paymentCount int64
	require.NoError(t, db.GetGorm().Model(&Payment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
	require.EqualValues(t, 1, paymentCount)
}

func TestSettleBillSplitShareCashCreatesCashRegisterSaleMovementWithTip(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-cash-caja-sale", 10000)
	openAlternativePaymentCashRegisterSession(t, bill.BusinessID, 0)

	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-cash-caja",
		DisplayName:    "Cash Guest",
		Mode:           BillSplitModeCustom,
		AmountCents:    4000,
		TipCents:       650,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	settled, paidBill, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID:        share.ID,
		GuestSessionID: "guest-cash-caja",
		IdempotencyKey: "cash-caja-sale-request",
		Tender:         "cash",
		PayerAddr:      "cashier",
		Now:            now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.NotNil(t, settled.AlternativePaymentID)
	require.Equal(t, int64(4000), paidBill.PaidAmount)
	require.Equal(t, int64(650), paidBill.TipAmount)

	var alt AlternativePayment
	require.NoError(t, db.GetGorm().First(&alt, *settled.AlternativePaymentID).Error)
	require.Equal(t, int64(4000), alt.BillAmountCents)
	require.Equal(t, int64(650), alt.TipAmountCents)

	movement := requireSingleCashRegisterMovement(t)
	require.Equal(t, CashRegisterMovementTypeCashSale, movement.MovementType)
	require.Equal(t, int64(4650), movement.AmountCents)
	require.Equal(t, "system:split_cash_payment", movement.ActorLabel)
	require.NotNil(t, movement.AlternativePaymentID)
	require.Equal(t, *settled.AlternativePaymentID, *movement.AlternativePaymentID)
	requireCashRegisterSessionTotals(t, bill.BusinessID, 4650, 0)
}

func TestRefundBillPaymentReleasesLinkedSettledSplitShare(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-refund-share", 10000)

	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		DisplayName:    "Sara",
		Mode:           BillSplitModeCustom,
		AmountCents:    4000,
		TipCents:       500,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	settled, paidBill, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID:        share.ID,
		GuestSessionID: "guest-1",
		IdempotencyKey: "refund-share-request",
		Tender:         "crypto",
		TxHash:         "refund-share-tx",
		PayerAddr:      "0xguest",
		Now:            now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.NotNil(t, settled.PaymentID)
	require.Equal(t, int64(4000), paidBill.PaidAmount)
	require.Equal(t, int64(500), paidBill.TipAmount)

	refundedBill, refundedPayment, err := RefundBillPayment(bill.ID, *settled.PaymentID, "manager", "guest requested refund")
	require.NoError(t, err)
	require.Equal(t, PaymentStatusRefunded, refundedPayment.Status)
	require.Equal(t, int64(0), refundedBill.PaidAmount)
	require.Equal(t, int64(0), refundedBill.TipAmount)
	require.Equal(t, BillStatusOpen, refundedBill.Status)

	var reloadedShare BillSplitShare
	require.NoError(t, db.GetGorm().First(&reloadedShare, settled.ID).Error)
	require.Equal(t, BillSplitShareStatusReleased, reloadedShare.Status)
	require.NotNil(t, reloadedShare.ReleasedAt)
	require.NotNil(t, reloadedShare.PaymentID)
	require.Equal(t, *settled.PaymentID, *reloadedShare.PaymentID)

	state, err := GetBillSplitStateByBillID(bill.ID, now.Add(2*time.Minute))
	require.NoError(t, err)
	require.Equal(t, int64(0), state.PaidCents)
	require.Equal(t, int64(0), state.HeldCents)
	require.Equal(t, int64(10000), state.AvailableCents)
	require.Empty(t, state.Shares)
}

func TestRefundBillAlternativePaymentReleasesLinkedSettledSplitShare(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-refund-alt-share", 10000)
	openAlternativePaymentCashRegisterSession(t, bill.BusinessID, 0)

	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-cash",
		DisplayName:    "Cash Guest",
		Mode:           BillSplitModeCustom,
		AmountCents:    4000,
		TipCents:       500,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	settled, paidBill, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID:        share.ID,
		GuestSessionID: "guest-cash",
		IdempotencyKey: "refund-alt-share-request",
		Tender:         "cash",
		PayerAddr:      "cashier",
		TipCents:       500,
		Now:            now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.NotNil(t, settled.AlternativePaymentID)
	require.Equal(t, int64(4000), paidBill.PaidAmount)
	require.Equal(t, int64(500), paidBill.TipAmount)

	refundedBill, refundedPayment, err := RefundBillAlternativePayment(bill.ID, *settled.AlternativePaymentID, "manager", "cash returned")
	require.NoError(t, err)
	require.Equal(t, AltPaymentStatusRefunded, refundedPayment.Status)
	require.Equal(t, int64(0), refundedBill.PaidAmount)
	require.Equal(t, int64(0), refundedBill.TipAmount)
	require.Equal(t, BillStatusOpen, refundedBill.Status)

	var reloadedShare BillSplitShare
	require.NoError(t, db.GetGorm().First(&reloadedShare, settled.ID).Error)
	require.Equal(t, BillSplitShareStatusReleased, reloadedShare.Status)
	require.NotNil(t, reloadedShare.ReleasedAt)
	require.NotNil(t, reloadedShare.AlternativePaymentID)
	require.Equal(t, *settled.AlternativePaymentID, *reloadedShare.AlternativePaymentID)

	state, err := GetBillSplitStateByBillID(bill.ID, now.Add(2*time.Minute))
	require.NoError(t, err)
	require.Equal(t, int64(0), state.PaidCents)
	require.Equal(t, int64(0), state.HeldCents)
	require.Equal(t, int64(10000), state.AvailableCents)
	require.Empty(t, state.Shares)

	var movements []CashRegisterMovement
	require.NoError(t, db.GetGorm().Order("id ASC").Find(&movements).Error)
	require.Len(t, movements, 2)
	require.Equal(t, CashRegisterMovementTypeCashSale, movements[0].MovementType)
	require.Equal(t, int64(4500), movements[0].AmountCents)
	require.Equal(t, CashRegisterMovementTypeCashRefund, movements[1].MovementType)
	require.Equal(t, int64(4500), movements[1].AmountCents)
	requireCashRegisterSessionTotals(t, bill.BusinessID, 4500, 4500)
}

func TestMarkBillSplitShareSettledByPaymentLinksExternalPluginPayment(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-plugin-external", 10000)

	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		DisplayName:    "Sara",
		Mode:           BillSplitModeCustom,
		AmountCents:    2500,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	payment := &Payment{
		BillID:        bill.ID,
		PayerAddr:     "plugin",
		Amount:        2500,
		TipAmount:     300,
		TxHash:        "plugin_split_external",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "plugin",
		Currency:      "USD",
	}
	require.NoError(t, db.GetGorm().Create(payment).Error)

	settled, err := MarkBillSplitShareSettledByPayment(MarkBillSplitShareSettledByPaymentInput{
		ShareID:        share.ID,
		BillID:         bill.ID,
		PaymentID:      payment.ID,
		TipCents:       300,
		Tender:         "plugin",
		IdempotencyKey: "plugin-payment-id",
		Now:            now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, BillSplitShareStatusSettled, settled.Status)
	require.NotNil(t, settled.PaymentID)
	require.Equal(t, payment.ID, *settled.PaymentID)
	require.Nil(t, settled.HoldExpiresAt)

	retry, err := MarkBillSplitShareSettledByPayment(MarkBillSplitShareSettledByPaymentInput{
		ShareID:        share.ID,
		BillID:         bill.ID,
		PaymentID:      payment.ID,
		TipCents:       300,
		Tender:         "plugin",
		IdempotencyKey: "plugin-payment-id",
		Now:            now.Add(2 * time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, settled.ID, retry.ID)
}

func TestMarkBillSplitShareSettledByPayment_RejectsSiblingShareWithSamePayment(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-sibling-payment", 10000)

	share1, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		DisplayName:    "Alice",
		Mode:           BillSplitModeCustom,
		AmountCents:    5000,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	share2, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-2",
		DisplayName:    "Bob",
		Mode:           BillSplitModeCustom,
		AmountCents:    5000,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	payment := &Payment{
		BillID:        bill.ID,
		PayerAddr:     "payer-1",
		Amount:        5000,
		TipAmount:     0,
		TxHash:        "tx-sibling-conflict",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		Currency:      "USDC",
	}
	require.NoError(t, db.GetGorm().Create(payment).Error)

	// Settle share1 with this payment — should succeed.
	settled1, err := MarkBillSplitShareSettledByPayment(MarkBillSplitShareSettledByPaymentInput{
		ShareID:        share1.ID,
		BillID:         bill.ID,
		PaymentID:      payment.ID,
		TipCents:       0,
		Tender:         "crypto",
		IdempotencyKey: "sibling-key-1",
		Now:            now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, BillSplitShareStatusSettled, settled1.Status)

	// Settle share2 with the SAME payment — must reject.
	_, err = MarkBillSplitShareSettledByPayment(MarkBillSplitShareSettledByPaymentInput{
		ShareID:        share2.ID,
		BillID:         bill.ID,
		PaymentID:      payment.ID,
		TipCents:       0,
		Tender:         "crypto",
		IdempotencyKey: "sibling-key-2",
		Now:            now.Add(2 * time.Minute),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPaymentAlreadySettledForBill)
}

// TestMarkBillSplitShareSettledByPayment_RejectsAmountMismatch (F6): the plugin
// split-settle path must enforce the same amount==share guard the cash /
// alternative-payment path applies — a payment whose bill portion does not match
// the share's owed amount must not settle the share.
func TestMarkBillSplitShareSettledByPayment_RejectsAmountMismatch(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-amount-mismatch", 10000)

	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		DisplayName:    "Mira",
		Mode:           BillSplitModeCustom,
		AmountCents:    2500,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	// Payment's bill portion (Amount) is 2400, but the share owes 2500.
	payment := &Payment{
		BillID:        bill.ID,
		PayerAddr:     "plugin",
		Amount:        2400,
		TipAmount:     0,
		TxHash:        "plugin_split_amount_mismatch",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "plugin",
		Currency:      "USD",
	}
	require.NoError(t, db.GetGorm().Create(payment).Error)

	_, err = MarkBillSplitShareSettledByPayment(MarkBillSplitShareSettledByPaymentInput{
		ShareID:        share.ID,
		BillID:         bill.ID,
		PaymentID:      payment.ID,
		TipCents:       0,
		Tender:         "plugin",
		IdempotencyKey: "amount-mismatch-key",
		Now:            now.Add(time.Minute),
	})
	require.ErrorIs(t, err, ErrSplitAmountUnavailable, "a payment that does not cover the share amount must not settle it")

	// The share must remain unsettled.
	var reloaded BillSplitShare
	require.NoError(t, db.GetGorm().First(&reloaded, share.ID).Error)
	require.NotEqual(t, BillSplitShareStatusSettled, reloaded.Status)
}

func TestSettleBillSplitShareRejectsExpiredHold(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-expired-settle", 10000)
	expiredAt := now.Add(-time.Minute)
	share := &BillSplitShare{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		Mode:           BillSplitModeEqual,
		AmountCents:    5000,
		Status:         BillSplitShareStatusHeld,
		HoldExpiresAt:  &expiredAt,
	}
	require.NoError(t, db.GetGorm().Create(share).Error)

	_, _, _, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID:        share.ID,
		GuestSessionID: "guest-1",
		IdempotencyKey: "expired",
		Tender:         "crypto",
		TxHash:         "expired-tx",
		Now:            now,
	})
	require.ErrorIs(t, err, ErrSplitHoldExpired)

	var reloaded Bill
	require.NoError(t, db.GetGorm().First(&reloaded, bill.ID).Error)
	require.Equal(t, int64(0), reloaded.PaidAmount)
}

func TestSettleBillSplitShareMixedTendersReconcileExactlyToZero(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-mixed-zero", 10000)

	cryptoShare, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		Mode:           BillSplitModeEqual,
		AmountCents:    4000,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	_, _, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID:        cryptoShare.ID,
		GuestSessionID: "guest-1",
		IdempotencyKey: "crypto-share",
		Tender:         "crypto",
		TxHash:         "mixed-crypto-tx",
		PayerAddr:      "0xguest",
		Now:            now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, applied)

	cashShare, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-2",
		Mode:           BillSplitModeCustom,
		AmountCents:    6000,
		HoldTTL:        5 * time.Minute,
		Now:            now.Add(2 * time.Minute),
	})
	require.NoError(t, err)
	settledCash, paidBill, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID:        cashShare.ID,
		GuestSessionID: "guest-2",
		IdempotencyKey: "cash-share",
		Tender:         "cash",
		PayerAddr:      "cashier",
		Now:            now.Add(3 * time.Minute),
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.NotNil(t, settledCash.AlternativePaymentID)
	require.Equal(t, int64(10000), paidBill.PaidAmount)
	require.Equal(t, BillStatusPaid, paidBill.Status)
	require.NotNil(t, paidBill.ClosedAt)
	require.Equal(t, int64(0), paidBill.TotalAmount-paidBill.PaidAmount)
}

func TestSettleBillSplitShareRejectsShareThatWouldOverpayFreshBalance(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-overpay-settle", 10000)

	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		Mode:           BillSplitModeCustom,
		AmountCents:    6000,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	_, _, err = ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "external",
		Amount:        5000,
		TxHash:        "external-payment",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}, nil)
	require.NoError(t, err)

	_, _, _, err = SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID:        share.ID,
		GuestSessionID: "guest-1",
		IdempotencyKey: "race-loser",
		Tender:         "crypto",
		TxHash:         "race-loser-tx",
		Now:            now.Add(time.Minute),
	})
	require.ErrorIs(t, err, ErrPaymentExceedsRemaining)

	var reloadedShare BillSplitShare
	require.NoError(t, db.GetGorm().First(&reloadedShare, share.ID).Error)
	require.Equal(t, BillSplitShareStatusHeld, reloadedShare.Status)
}

type billSplitSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *billSplitSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *billSplitSQLRecorder) selectCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

func (r *billSplitSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

// selectsFromTableMatching counts SELECT statements scanning the given table
// whose normalized text contains the provided substring (case-insensitive).
// Used to distinguish set-based `... IN (...)` reads from per-row `... = N`
// reads in the expired-hold sweeper access-shape test.
func (r *billSplitSQLRecorder) selectsFromTableMatching(table, substr string) int {
	count := 0
	lowSub := strings.ToLower(substr)
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if !(strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table)) {
			continue
		}
		if strings.Contains(normalized, lowSub) {
			count++
		}
	}
	return count
}

// TestReleaseExpiredBillSplitSharesWithStatesBatchesStateRebuild locks in the
// CG-4 access-shape fix: the 60s expired-hold sweeper must rebuild the SSE
// broadcast states with set-based reads (one bills WHERE id IN + one
// bill_split_shares WHERE bill_id IN) rather than the old per-bill N+1
// (loadProjectedBillForSplitTx + billSplitStateFromTx per expired bill =
// 2*M queries). It also proves the returned released count and []*BillSplitState
// stay byte-identical to the per-bill helper output.
func TestReleaseExpiredBillSplitSharesWithStatesBatchesStateRebuild(t *testing.T) {
	recorder := &billSplitSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupBillSplitTestDB(t, recorder)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)

	const m = 4
	bills := make([]*Bill, 0, m)
	for i := 0; i < m; i++ {
		bill := createBillSplitBill(t, db, fmt.Sprintf("sweep-%d", i), 10000)
		bills = append(bills, bill)

		// A still-active hold (must survive the sweep and remain in the state).
		_, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
			BillID:         bill.ID,
			GuestSessionID: fmt.Sprintf("guest-active-%d", i),
			DisplayName:    fmt.Sprintf("Active %d", i),
			Mode:           BillSplitModeCustom,
			AmountCents:    1500,
			HoldTTL:        30 * time.Minute,
			Now:            now,
		})
		require.NoError(t, err)

		// A settled share (immutable, must appear in the rebuilt state).
		settledShare, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
			BillID:         bill.ID,
			GuestSessionID: fmt.Sprintf("guest-settled-%d", i),
			DisplayName:    fmt.Sprintf("Settled %d", i),
			Mode:           BillSplitModeCustom,
			AmountCents:    3000,
			HoldTTL:        30 * time.Minute,
			Now:            now,
		})
		require.NoError(t, err)
		_, _, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
			ShareID:        settledShare.ID,
			GuestSessionID: fmt.Sprintf("guest-settled-%d", i),
			IdempotencyKey: fmt.Sprintf("sweep-settle-%d", i),
			Tender:         "crypto",
			TxHash:         fmt.Sprintf("sweep-tx-%d", i),
			PayerAddr:      "0xpayer",
			Now:            now,
		})
		require.NoError(t, err)
		require.True(t, applied)

		// Plant the EXPIRED held share LAST and via a direct insert: any later
		// hold/settle/read call on this bill runs the per-bill expired-release
		// path and would self-release it before the sweeper sees it. Inserting
		// directly keeps a genuine expired hold for the sweeper to release.
		expiredAt := now.Add(-4 * time.Minute)
		expired := &BillSplitShare{
			BillID:           bill.ID,
			GuestSessionID:   fmt.Sprintf("guest-expired-%d", i),
			DisplayName:      fmt.Sprintf("Expired %d", i),
			Mode:             BillSplitModeCustom,
			ClaimedItemIDs:   []string{},
			ClaimedFractions: map[string]string{},
			AmountCents:      2000,
			Status:           BillSplitShareStatusHeld,
			HoldExpiresAt:    &expiredAt,
			CreatedAt:        now.Add(-5 * time.Minute),
			UpdatedAt:        now.Add(-5 * time.Minute),
		}
		require.NoError(t, db.GetGorm().Create(expired).Error)
	}

	// Build the expected states by hand using the SAME per-bill helpers the old
	// loop used, inside one read transaction, BEFORE the sweep runs. We compute
	// them as-of the post-release world by applying the release first in a side
	// transaction is unnecessary: we instead assert parity against an
	// independent batched rebuild after the actual sweep below.
	expectedStates := make([]*BillSplitState, 0, m)
	err := db.GetGorm().Transaction(func(tx *gorm.DB) error {
		// Mirror the production set-based release so the per-bill helper sees the
		// same post-release rows the real sweeper will.
		if _, e := releaseExpiredBillSplitSharesTx(tx, now, nil); e != nil {
			return e
		}
		for _, bill := range bills {
			b, e := loadProjectedBillForSplitTx(tx, bill.ID, false)
			if e != nil {
				return e
			}
			st, e := billSplitStateFromTx(tx, b, now)
			if e != nil {
				return e
			}
			expectedStates = append(expectedStates, st)
		}
		// Roll back so the real sweeper still has expired rows to release.
		return errIntentionalRollback
	})
	require.ErrorIs(t, err, errIntentionalRollback)
	require.Len(t, expectedStates, m)

	recorder.statements = nil
	released, states, err := ReleaseExpiredBillSplitSharesWithStates(now)
	require.NoError(t, err)

	// One expired hold released per bill.
	require.EqualValues(t, m, released)

	// ACCESS SHAPE: exactly ONE set-based bills read and ONE set-based shares
	// read for the state rebuild — never per-bill (`... = N`) reads.
	require.Equal(t, 1, recorder.selectsFromTableMatching("bills", "in ("),
		"sweeper must batch-load bills with one WHERE id IN (...)")
	require.Equal(t, 1, recorder.selectsFromTableMatching("bill_split_shares", "bill_id in ("),
		"sweeper must batch-load shares with one WHERE bill_id IN (...)")
	require.Zero(t, recorder.selectsFromTableMatching("bills", "= 1"),
		"sweeper must not per-bill load bills")
	require.Zero(t, recorder.selectsFromTableMatching("bill_split_shares", "bill_id = 1"),
		"sweeper must not per-bill load shares")
	require.Zero(t, recorder.selectStarCount("bills"), "sweeper must project bill fields")
	require.Zero(t, recorder.selectStarCount("bill_split_shares"), "sweeper must project share fields")

	// STATE PARITY: the batched states must be byte-identical to the per-bill
	// rebuild — same order, cents math, and Shares slice.
	require.Len(t, states, m)
	for i := range expectedStates {
		exp := expectedStates[i]
		got := states[i]
		require.Equal(t, exp.BillID, got.BillID, "bill %d order/identity", i)
		require.Equal(t, exp.HeldCents, got.HeldCents, "bill %d held", i)
		require.Equal(t, exp.AvailableCents, got.AvailableCents, "bill %d available", i)
		require.Equal(t, exp.PaidCents, got.PaidCents, "bill %d paid", i)
		require.Equal(t, exp.TotalCents, got.TotalCents, "bill %d total", i)
		require.Equal(t, exp.Shares, got.Shares, "bill %d shares slice", i)
		// Full struct deep-equal for completeness.
		require.Equal(t, exp, got, "bill %d full state", i)
	}

	// Post-release, the expired hold is gone: available = total - paid(3000) -
	// activeHeld(1500) = 10000-3000-1500 = 5500.
	for i := range states {
		require.Equal(t, int64(5500), states[i].AvailableCents, "bill %d freed availability", i)
		require.Equal(t, int64(1500), states[i].HeldCents, "bill %d retains active hold", i)
		require.Equal(t, int64(3000), states[i].PaidCents, "bill %d retains settled", i)
	}
}

var errIntentionalRollback = errors.New("intentional rollback for parity snapshot")

// BenchmarkReleaseExpiredBillSplitSharesWithStates exercises the 60s sweeper
// over M expired-held bills. Before CG-4 the per-bill loop issued 2*M reads;
// after, it is a constant 4 queries regardless of M.
func BenchmarkReleaseExpiredBillSplitSharesWithStates(b *testing.B) {
	const m = 20
	db := setupBillSplitTestDB(b, logger.Default.LogMode(logger.Silent))
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	billIDs := make([]uint, 0, m)
	for i := 0; i < m; i++ {
		bill := createBillSplitBill(b, db, fmt.Sprintf("sweep-bench-%d", i), 10000)
		billIDs = append(billIDs, bill.ID)
		// A settled + an active hold for realism (untouched by the sweep).
		settled, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
			BillID: bill.ID, GuestSessionID: fmt.Sprintf("s-%d", i), Mode: BillSplitModeCustom,
			AmountCents: 3000, HoldTTL: 30 * time.Minute, Now: now,
		})
		require.NoError(b, err)
		_, _, _, err = SettleBillSplitShare(SettleBillSplitShareInput{
			ShareID: settled.ID, GuestSessionID: fmt.Sprintf("s-%d", i),
			IdempotencyKey: fmt.Sprintf("bench-settle-%d", i), Tender: "crypto",
			TxHash: fmt.Sprintf("bench-tx-%d", i), PayerAddr: "0xp", Now: now,
		})
		require.NoError(b, err)
		_, _, err = HoldBillSplitShare(HoldBillSplitShareInput{
			BillID: bill.ID, GuestSessionID: fmt.Sprintf("a-%d", i), Mode: BillSplitModeCustom,
			AmountCents: 1500, HoldTTL: 30 * time.Minute, Now: now,
		})
		require.NoError(b, err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Re-seed an expired hold on every bill so each iteration has M holds to
		// release (the release UPDATE is idempotent once rows are released).
		seedNow := now.Add(-5 * time.Minute)
		for j, billID := range billIDs {
			_, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
				BillID: billID, GuestSessionID: fmt.Sprintf("exp-%d-%d", i, j),
				Mode: BillSplitModeCustom, AmountCents: 100, HoldTTL: time.Minute, Now: seedNow,
			})
			require.NoError(b, err)
		}
		b.StartTimer()
		if _, _, err := ReleaseExpiredBillSplitSharesWithStates(now); err != nil {
			b.Fatalf("sweep failed: %v", err)
		}
	}
}

func TestGetBillSplitStateByNumberUsesProjectedReads(t *testing.T) {
	recorder := &billSplitSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupBillSplitTestDB(t, recorder)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-state-shape", 10000)
	_, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-1",
		Mode:           BillSplitModeCustom,
		AmountCents:    2500,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	recorder.statements = nil
	require.NotEmpty(t, bill.PublicToken)
	state, err := GetBillSplitStateByNumber(bill.PublicToken, now)
	require.NoError(t, err)
	require.Equal(t, int64(2500), state.HeldCents)
	require.Equal(t, int64(7500), state.AvailableCents)
	require.Len(t, state.Shares, 1)

	require.Zero(t, recorder.selectStarCount("bills"), "split state must project bill fields")
	require.Zero(t, recorder.selectStarCount("bill_split_shares"), "split state must project share fields")
	require.Zero(t, recorder.selectCount("payments"), "split state should not hydrate payments")
	require.LessOrEqual(t, recorder.selectCount("bill_split_shares"), 1, "split state should read shares in one query")
}

// TestBillSplitStateItemsAddedMidSplitEnterUnclaimedPool locks in the spec edge
// case: when a server adds items to a bill that is already being split, the new
// amount must flow into the unclaimed pool (raising available balance and the
// claimable headroom) WITHOUT mutating shares that are already settled or
// actively held. Split state derives available from the live bill total minus
// paid minus active holds, so a mid-split item addition is reflected
// immediately while prior settlements stay immutable.
func TestBillSplitStateItemsAddedMidSplitEnterUnclaimedPool(t *testing.T) {
	db := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, db, "split-mid-add", 10000)

	// Guest A settles a 4000 share — this becomes immutable.
	settledShare, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-a",
		DisplayName:    "Ana",
		Mode:           BillSplitModeCustom,
		AmountCents:    4000,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	settled, _, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID:        settledShare.ID,
		GuestSessionID: "guest-a",
		IdempotencyKey: "mid-add-settle",
		Tender:         "crypto",
		TxHash:         "mid-add-tx",
		PayerAddr:      "0xana",
		Now:            now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, applied)

	// Guest B is mid-claim with an active 3000 hold.
	heldShare, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-b",
		DisplayName:    "Bo",
		Mode:           BillSplitModeCustom,
		AmountCents:    3000,
		HoldTTL:        5 * time.Minute,
		Now:            now.Add(2 * time.Minute),
	})
	require.NoError(t, err)

	before, err := GetBillSplitStateByBillID(bill.ID, now.Add(3*time.Minute))
	require.NoError(t, err)
	require.Equal(t, int64(10000), before.TotalCents)
	require.Equal(t, int64(4000), before.PaidCents)
	require.Equal(t, int64(3000), before.HeldCents)
	require.Equal(t, int64(3000), before.AvailableCents)

	// Server adds a 2500-cent item mid-split: bill subtotal + total grow, like
	// the real AddBillItem recompute path (NetBillTotalCents).
	require.NoError(t, db.GetGorm().Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"subtotal":     int64(12500),
		"total_amount": int64(12500),
	}).Error)

	after, err := GetBillSplitStateByBillID(bill.ID, now.Add(4*time.Minute))
	require.NoError(t, err)
	require.Equal(t, int64(12500), after.TotalCents, "new item raises the live bill total")
	require.Equal(t, int64(4000), after.PaidCents, "settled share is immutable")
	require.Equal(t, int64(3000), after.HeldCents, "active hold is untouched")
	require.Equal(t, int64(5500), after.AvailableCents, "the added 2500 flows into the unclaimed pool")

	// The settled share row itself must be byte-for-byte immutable.
	var reloadedSettled BillSplitShare
	require.NoError(t, db.GetGorm().First(&reloadedSettled, settled.ID).Error)
	require.Equal(t, BillSplitShareStatusSettled, reloadedSettled.Status)
	require.Equal(t, int64(4000), reloadedSettled.AmountCents)

	// A late guest can claim the freshly-added headroom (the full 5500), proving
	// the new item is genuinely claimable and the bill cannot be over-claimed.
	lateShare, lateState, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-c",
		DisplayName:    "Cy",
		Mode:           BillSplitModeCustom,
		AmountCents:    5500,
		HoldTTL:        5 * time.Minute,
		Now:            now.Add(5 * time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, int64(5500), lateShare.AmountCents)
	require.Equal(t, int64(0), lateState.AvailableCents, "claiming the headroom drains available to zero")
	require.Equal(t, int64(8500), lateState.HeldCents, "3000 prior hold + 5500 new hold")

	// Held + settled can never exceed the live total.
	require.LessOrEqual(t, lateState.PaidCents+lateState.HeldCents, lateState.TotalCents)

	// Reload the original held share to confirm it was not disturbed by the add.
	var reloadedHeld BillSplitShare
	require.NoError(t, db.GetGorm().First(&reloadedHeld, heldShare.ID).Error)
	require.Equal(t, BillSplitShareStatusHeld, reloadedHeld.Status)
	require.Equal(t, int64(3000), reloadedHeld.AmountCents)
}

func BenchmarkGetBillSplitStateByNumberSQLite(b *testing.B) {
	db := setupBillSplitTestDB(b, logger.Default.LogMode(logger.Silent))
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(b, db, "split-state-bench", 100000)
	for i := 0; i < 75; i++ {
		_, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
			BillID:         bill.ID,
			GuestSessionID: fmt.Sprintf("guest-%d", i),
			Mode:           BillSplitModeCustom,
			AmountCents:    100,
			HoldTTL:        5 * time.Minute,
			Now:            now,
		})
		require.NoError(b, err)
	}

	require.NotEmpty(b, bill.PublicToken)
	token := bill.PublicToken
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GetBillSplitStateByNumber(token, now); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			b.Fatalf("split state failed: %v", err)
		}
	}
}
