package database

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPendingAlternativePaymentConfirmationVersusRejection_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}
	pg := startGenesisPostgres(t)

	previousDB := db
	SetTestDB(pg.DB)
	t.Cleanup(func() { SetTestDB(previousDB) })

	business := Business{
		BusinessId:   "alternative-resolution-race",
		OwnerAddress: "0x1111111111111111111111111111111111111111",
		Name:         "Resolution race",
		IsActive:     true,
	}
	require.NoError(t, pg.DB.Create(&business).Error)
	bill := Bill{
		BusinessID: business.ID, BillNumber: "ALT-RESOLVE-RACE", Status: BillStatusOpen,
		Items: "[]", TotalAmount: 1_000,
	}
	require.NoError(t, pg.DB.Create(&bill).Error)
	expiresAt := time.Now().UTC().Add(time.Minute)
	payment := AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "guest", Amount: 500,
		PaymentMethod: PaymentMethodCard, Status: AltPaymentStatusPending, ExpiresAt: &expiresAt,
	}
	require.NoError(t, pg.DB.Create(&payment).Error)

	start := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, _, errs[0] = ConfirmPendingAlternativePayment(bill.ID, payment.ID, "owner", nil)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, errs[1] = RejectPendingAlternativePayment(bill.ID, payment.ID, "owner", "not received", time.Now().UTC())
	}()
	close(start)
	wg.Wait()

	successes := 0
	for _, raceErr := range errs {
		if raceErr == nil {
			successes++
			continue
		}
		require.True(t,
			errors.Is(raceErr, ErrAlternativePaymentRequestNotPending) || errors.Is(raceErr, ErrAlternativePaymentAlreadyConfirmed),
			"losing transition must report an already-terminal request, got %v", raceErr,
		)
	}
	require.Equal(t, 1, successes, "exactly one terminal transition may win")

	var storedPayment AlternativePayment
	require.NoError(t, pg.DB.First(&storedPayment, payment.ID).Error)
	var storedBill Bill
	require.NoError(t, pg.DB.First(&storedBill, bill.ID).Error)
	switch storedPayment.Status {
	case AltPaymentStatusConfirmed:
		require.Equal(t, int64(500), storedBill.PaidAmount)
		require.Nil(t, storedPayment.ResolvedAt)
	case AltPaymentStatusRejected:
		require.Zero(t, storedBill.PaidAmount)
		require.NotNil(t, storedPayment.ResolvedAt)
	default:
		t.Fatalf("unexpected terminal status %q", storedPayment.Status)
	}
}
