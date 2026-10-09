package database

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAlternativePaymentRequestIntegrityAndConcurrency_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}
	pg := startGenesisPostgres(t)
	gdb := pg.DB
	require.Equal(t, uint(1), seedGenesisBusiness(t, gdb, "alt-request-integrity"))

	require.Equal(t, uint(1), createOpenTestBill(t, gdb, "ALT-REQ-1"))
	service := &AlternativePaymentService{repo: NewRepository[AlternativePayment](gdb)}
	now := time.Now().UTC()
	requests := []*AlternativePayment{
		{
			BillID: 1, ParticipantAddr: "guest", ParticipantName: "A", Amount: 700, BillAmountCents: 700,
			PaymentMethod: PaymentMethodCash, IdempotencyKeyHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			PayloadHash: "1111111111111111111111111111111111111111111111111111111111111111",
		},
		{
			BillID: 1, ParticipantAddr: "guest", ParticipantName: "B", Amount: 700, BillAmountCents: 700,
			PaymentMethod: PaymentMethodCash, IdempotencyKeyHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			PayloadHash: "2222222222222222222222222222222222222222222222222222222222222222",
		},
	}

	start := make(chan struct{})
	errs := make([]error, len(requests))
	var wg sync.WaitGroup
	for i := range requests {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, _, errs[index] = service.CreatePendingRequest(requests[index], now, "", 0, nil)
		}(i)
	}
	close(start)
	wg.Wait()

	var successful, capacityConflicts int
	for _, requestErr := range errs {
		switch {
		case requestErr == nil:
			successful++
		case errors.Is(requestErr, ErrPaymentExceedsRemaining):
			capacityConflicts++
		default:
			require.NoError(t, requestErr)
		}
	}
	require.Equal(t, 1, successful)
	require.Equal(t, 1, capacityConflicts)

	var count, reserved int64
	require.NoError(t, gdb.Model(&AlternativePayment{}).Where("bill_id = ? AND status = ?", 1, AltPaymentStatusPending).Count(&count).Error)
	require.NoError(t, gdb.Model(&AlternativePayment{}).Select("COALESCE(SUM(amount), 0)").Where("bill_id = ? AND status = ?", 1, AltPaymentStatusPending).Scan(&reserved).Error)
	require.Equal(t, int64(1), count)
	require.Equal(t, int64(700), reserved, "the bill row lock must prevent concurrent over-reservation")

	replayBillID := createOpenTestBill(t, gdb, "ALT-REQ-2")
	sameKeyRequests := []*AlternativePayment{
		{
			BillID: replayBillID, ParticipantAddr: "guest", ParticipantName: "Retry", Amount: 500, BillAmountCents: 500,
			PaymentMethod: PaymentMethodCash, IdempotencyKeyHash: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			PayloadHash: "3333333333333333333333333333333333333333333333333333333333333333",
		},
		{
			BillID: replayBillID, ParticipantAddr: "guest", ParticipantName: "Retry", Amount: 500, BillAmountCents: 500,
			PaymentMethod: PaymentMethodCash, IdempotencyKeyHash: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			PayloadHash: "3333333333333333333333333333333333333333333333333333333333333333",
		},
	}
	type replayResult struct {
		id       uint
		replayed bool
		err      error
	}
	replayResults := make([]replayResult, len(sameKeyRequests))
	start = make(chan struct{})
	wg = sync.WaitGroup{}
	for i := range sameKeyRequests {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			stored, replayed, createErr := service.CreatePendingRequest(sameKeyRequests[index], now, "", 0, nil)
			replayResults[index].err = createErr
			replayResults[index].replayed = replayed
			if stored != nil {
				replayResults[index].id = stored.ID
			}
		}(i)
	}
	close(start)
	wg.Wait()

	require.NoError(t, replayResults[0].err)
	require.NoError(t, replayResults[1].err)
	require.NotZero(t, replayResults[0].id)
	require.Equal(t, replayResults[0].id, replayResults[1].id)
	require.NotEqual(t, replayResults[0].replayed, replayResults[1].replayed, "one concurrent caller creates and the other replays")
	require.NoError(t, gdb.Model(&AlternativePayment{}).Where("bill_id = ?", replayBillID).Count(&count).Error)
	require.Equal(t, int64(1), count, "concurrent exact retries must persist one request")

	previousDB := db
	SetTestDB(gdb)
	t.Cleanup(func() { SetTestDB(previousDB) })

	closeRaceBillID := createOpenTestBill(t, gdb, "ALT-REQ-3")
	requestInserted := make(chan struct{})
	releaseRequest := make(chan struct{})
	require.NoError(t, gdb.Callback().Create().After("gorm:create").Register("test:pause_alt_request_before_commit", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "alternative_payments" {
			close(requestInserted)
			<-releaseRequest
		}
	}))
	defer gdb.Callback().Create().Remove("test:pause_alt_request_before_commit")

	closeRacePayment := &AlternativePayment{
		BillID: closeRaceBillID, ParticipantAddr: "guest", ParticipantName: "Close race", Amount: 500, BillAmountCents: 500,
		PaymentMethod: PaymentMethodCash, IdempotencyKeyHash: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		PayloadHash: "4444444444444444444444444444444444444444444444444444444444444444",
	}
	requestDone := make(chan error, 1)
	go func() {
		_, _, createErr := service.CreatePendingRequest(closeRacePayment, now, "", 0, nil)
		requestDone <- createErr
	}()
	<-requestInserted
	closeDone := make(chan error, 1)
	go func() { closeDone <- CloseBill(closeRaceBillID) }()
	close(releaseRequest)
	require.NoError(t, <-requestDone)
	require.ErrorIs(t, <-closeDone, ErrBillHasUnpaidRemaining)
	require.NoError(t, gdb.Model(&AlternativePayment{}).
		Where("bill_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", closeRaceBillID, AltPaymentStatusPending, now).
		Count(&count).Error)
	require.Equal(t, int64(1), count, "unpaid remaining keeps the bill open, so the racing request stays reserved")

	require.NoError(t, gdb.Callback().Create().Remove("test:pause_alt_request_before_commit"))
	splitRaceBillID := createOpenTestBill(t, gdb, "ALT-REQ-4")
	splitRaceRequest := &AlternativePayment{
		BillID: splitRaceBillID, ParticipantAddr: "guest", ParticipantName: "Ordinary", Amount: 700, BillAmountCents: 700,
		PaymentMethod: PaymentMethodCash, IdempotencyKeyHash: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		PayloadHash: "5555555555555555555555555555555555555555555555555555555555555555",
	}
	start = make(chan struct{})
	var ordinaryErr, splitErr error
	wg = sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, _, ordinaryErr = service.CreatePendingRequest(splitRaceRequest, now, "", 0, nil)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, _, splitErr = HoldBillSplitShare(HoldBillSplitShareInput{
			BillID: splitRaceBillID, GuestSessionID: "split-racer", DisplayName: "Split",
			Mode: BillSplitModeCustom, AmountCents: 700, IdempotencyKey: "split-race-hold", Now: now,
		})
	}()
	close(start)
	wg.Wait()
	require.True(t, ordinaryErr == nil || splitErr == nil, "at least one competing reservation should succeed: ordinary=%v split=%v", ordinaryErr, splitErr)
	var held int64
	require.NoError(t, gdb.Model(&BillSplitShare{}).Select("COALESCE(SUM(amount_cents), 0)").
		Where("bill_id = ? AND status = ?", splitRaceBillID, BillSplitShareStatusHeld).Scan(&held).Error)
	require.NoError(t, gdb.Model(&AlternativePayment{}).Select("COALESCE(SUM(amount), 0)").
		Where("bill_id = ? AND status = ?", splitRaceBillID, AltPaymentStatusPending).Scan(&reserved).Error)
	require.LessOrEqual(t, held+reserved, int64(1000), "split holds and ordinary requests share one capacity budget")
	if ordinaryErr == nil && splitErr == nil {
		require.Equal(t, int64(1000), held+reserved, "a custom split may be capped to the residual capacity, never over-reserved")
	}
}

func createOpenTestBill(t *testing.T, gdb *gorm.DB, number string) uint {
	t.Helper()
	bill := Bill{BusinessID: 1, BillNumber: number, Status: BillStatusOpen, Items: "[]", TotalAmount: 1000}
	require.NoError(t, gdb.Create(&bill).Error)
	return bill.ID
}
