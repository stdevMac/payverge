package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/guestsession"
)

func guestFiscalBill(t testing.TB, biz *Business, number string, setter *string) *Bill {
	t.Helper()
	doc, num, name, email := "CUIT", "20111111112", "Squatter SA", "squat@example.com"
	b := &Bill{
		BusinessID:                 biz.ID,
		BillNumber:                 number,
		Status:                     BillStatusOpen,
		Items:                      "[]",
		TotalAmount:                5000,
		FiscalCustomerDocType:      &doc,
		FiscalCustomerDocNumber:    &num,
		FiscalCustomerName:         &name,
		FiscalCustomerEmail:        &email,
		FiscalCustomerGuestSession: setter,
	}
	require.NoError(t, db.Create(b).Error)
	return b
}

func strp(s string) *string { return &s }

func setupGuestFiscalDB(tb testing.TB) *Business {
	tb.Helper()
	setupBillSplitTestDB(tb, nil)
	biz := &Business{Name: "Guest Fiscal Resto", OwnerAddress: "0xowner", SettlementAddr: "0xs", TippingAddr: "0xt"}
	require.NoError(tb, db.Create(biz).Error)
	return biz
}

func reloadGuestFiscalBill(t *testing.T, id uint) Bill {
	t.Helper()
	var got Bill
	require.NoError(t, db.First(&got, id).Error)
	return got
}

func TestGuestPaymentProofForSession_Ranks(t *testing.T) {
	biz := setupGuestFiscalDB(t)
	bill := guestFiscalBill(t, biz, "GFI-proof", nil)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

	proof := func(fp string) GuestPaymentProof {
		t.Helper()
		got, err := GuestPaymentProofForSession(db, bill.ID, fp, now)
		require.NoError(t, err)
		return got
	}

	require.Equal(t, GuestPaymentProofNone, proof(""), "no session, no proof")
	require.Equal(t, GuestPaymentProofNone, proof("fp-none"))

	// Pending, unexpired tender request → pending proof.
	future := now.Add(time.Hour)
	require.NoError(t, db.Create(&AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "guest", Amount: 100, PaymentMethod: PaymentMethodCash,
		Status: AltPaymentStatusPending, ExpiresAt: &future, PayerGuestSession: strp("fp-pending"),
	}).Error)
	require.Equal(t, GuestPaymentProofPending, proof("fp-pending"))

	// Expired-but-unswept pending request is not proof.
	past := now.Add(-time.Hour)
	require.NoError(t, db.Create(&AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "guest-old", Amount: 100, PaymentMethod: PaymentMethodCash,
		Status: AltPaymentStatusPending, ExpiresAt: &past, PayerGuestSession: strp("fp-expired"),
	}).Error)
	require.Equal(t, GuestPaymentProofNone, proof("fp-expired"))

	// Rejected request is not proof.
	require.NoError(t, db.Create(&AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "guest-rej", Amount: 100, PaymentMethod: PaymentMethodCash,
		Status: AltPaymentStatusRejected, PayerGuestSession: strp("fp-rejected"),
	}).Error)
	require.Equal(t, GuestPaymentProofNone, proof("fp-rejected"))

	// Confirmed crypto payment → confirmed proof.
	require.NoError(t, db.Create(&Payment{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 1000, TxHash: "0xproof-crypto",
		Status: PaymentStatusConfirmed, PayerGuestSession: strp("fp-crypto"),
	}).Error)
	require.Equal(t, GuestPaymentProofConfirmed, proof("fp-crypto"))

	// Plugin checkout tracker still pending, but its settled payments row
	// (plugin_<provider id>) is confirmed: the tracker flips only after the
	// settlement commit, so the join must already count it as confirmed.
	require.NoError(t, db.Create(&AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "pi_123", Amount: 2000, PaymentMethod: AlternativePaymentMethod("stripe"),
		Status: AltPaymentStatusPending, PayerGuestSession: strp("fp-plugin"),
	}).Error)
	require.Equal(t, GuestPaymentProofPending, proof("fp-plugin"))
	require.NoError(t, db.Create(&Payment{
		BillID: bill.ID, PayerAddr: "plugin", Amount: 2000, TxHash: "plugin_pi_123",
		Status: PaymentStatusConfirmed, PaymentMethod: "stripe",
	}).Error)
	require.Equal(t, GuestPaymentProofConfirmed, proof("fp-plugin"))

	// Proof is per bill.
	other := guestFiscalBill(t, biz, "GFI-proof-other", nil)
	got, err := GuestPaymentProofForSession(db, other.ID, "fp-crypto", now)
	require.NoError(t, err)
	require.Equal(t, GuestPaymentProofNone, got)
}

func TestClearUnpaidGuestFiscalIdentity(t *testing.T) {
	biz := setupGuestFiscalDB(t)
	seq := 0
	newBill := func(setter *string) *Bill {
		seq++
		return guestFiscalBill(t, biz, fmt.Sprintf("GFI-clear-%d", seq), setter)
	}
	pay := func(bill *Bill, fp *string, txHash string) {
		require.NoError(t, db.Create(&Payment{
			BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 5000, TxHash: txHash,
			Status: PaymentStatusConfirmed, PayerGuestSession: fp,
		}).Error)
	}

	t.Run("squatter identity dropped when another guest paid", func(t *testing.T) {
		bill := newBill(strp("fp-squatter"))
		pay(bill, strp("fp-payer"), "0xclear-1")
		cleared, err := ClearUnpaidGuestFiscalIdentity(db, bill.ID)
		require.NoError(t, err)
		require.True(t, cleared)
		got := reloadGuestFiscalBill(t, bill.ID)
		require.Nil(t, got.FiscalCustomerDocType)
		require.Nil(t, got.FiscalCustomerDocNumber)
		require.Nil(t, got.FiscalCustomerName)
		require.Nil(t, got.FiscalCustomerEmail)
		require.Nil(t, got.FiscalCustomerGuestSession)
	})

	t.Run("setter who paid keeps identity", func(t *testing.T) {
		bill := newBill(strp("fp-payer2"))
		pay(bill, strp("fp-payer2"), "0xclear-2")
		pay(bill, strp("fp-other"), "0xclear-2b")
		cleared, err := ClearUnpaidGuestFiscalIdentity(db, bill.ID)
		require.NoError(t, err)
		require.False(t, cleared)
		require.NotNil(t, reloadGuestFiscalBill(t, bill.ID).FiscalCustomerDocNumber)
	})

	t.Run("setter paid through a plugin tracker not yet flipped", func(t *testing.T) {
		bill := newBill(strp("fp-plugin-payer"))
		require.NoError(t, db.Create(&AlternativePayment{
			BillID: bill.ID, ParticipantAddr: "pi_clear_3", Amount: 5000, PaymentMethod: AlternativePaymentMethod("stripe"),
			Status: AltPaymentStatusPending, PayerGuestSession: strp("fp-plugin-payer"),
		}).Error)
		pay(bill, nil, "plugin_pi_clear_3")
		cleared, err := ClearUnpaidGuestFiscalIdentity(db, bill.ID)
		require.NoError(t, err)
		require.False(t, cleared)
	})

	t.Run("operator-set identity is never cleared", func(t *testing.T) {
		bill := newBill(nil)
		pay(bill, strp("fp-payer4"), "0xclear-4")
		cleared, err := ClearUnpaidGuestFiscalIdentity(db, bill.ID)
		require.NoError(t, err)
		require.False(t, cleared)
		require.NotNil(t, reloadGuestFiscalBill(t, bill.ID).FiscalCustomerDocNumber)
	})

	t.Run("staff-settled bill drops the squatter identity", func(t *testing.T) {
		bill := newBill(strp("fp-diner"))
		pay(bill, nil, "manual_clear_5")
		cleared, err := ClearUnpaidGuestFiscalIdentity(db, bill.ID)
		require.NoError(t, err)
		require.True(t, cleared, "setter session never paid: a staff settlement must not issue to it")
		require.Nil(t, reloadGuestFiscalBill(t, bill.ID).FiscalCustomerDocNumber)
	})

	t.Run("setter with only a pending request is not a payer", func(t *testing.T) {
		bill := newBill(strp("fp-pending6"))
		require.NoError(t, db.Create(&AlternativePayment{
			BillID: bill.ID, ParticipantAddr: "guest6", Amount: 5000, PaymentMethod: PaymentMethodCash,
			Status: AltPaymentStatusPending, PayerGuestSession: strp("fp-pending6"),
		}).Error)
		pay(bill, nil, "manual_clear_6")
		cleared, err := ClearUnpaidGuestFiscalIdentity(db, bill.ID)
		require.NoError(t, err)
		require.True(t, cleared)
	})

	t.Run("setter's confirmed cash request keeps identity on a staff-settled bill", func(t *testing.T) {
		bill := newBill(strp("fp-cash6b"))
		require.NoError(t, db.Create(&AlternativePayment{
			BillID: bill.ID, ParticipantAddr: "guest6b", Amount: 5000, PaymentMethod: PaymentMethodCash,
			Status: AltPaymentStatusConfirmed, PayerGuestSession: strp("fp-cash6b"),
		}).Error)
		cleared, err := ClearUnpaidGuestFiscalIdentity(db, bill.ID)
		require.NoError(t, err)
		require.False(t, cleared)
		require.NotNil(t, reloadGuestFiscalBill(t, bill.ID).FiscalCustomerDocNumber)
	})

	t.Run("confirmed cash request from another guest clears", func(t *testing.T) {
		bill := newBill(strp("fp-squatter7"))
		require.NoError(t, db.Create(&AlternativePayment{
			BillID: bill.ID, ParticipantAddr: "guest7", Amount: 5000, PaymentMethod: PaymentMethodCash,
			Status: AltPaymentStatusConfirmed, PayerGuestSession: strp("fp-payer7"),
		}).Error)
		cleared, err := ClearUnpaidGuestFiscalIdentity(db, bill.ID)
		require.NoError(t, err)
		require.True(t, cleared)
	})

	t.Run("only the target bill is touched", func(t *testing.T) {
		target := newBill(strp("fp-squatter8"))
		bystander := newBill(strp("fp-squatter8"))
		pay(target, strp("fp-payer8"), "0xclear-8")
		pay(bystander, strp("fp-payer8b"), "0xclear-8b")
		cleared, err := ClearUnpaidGuestFiscalIdentity(db, target.ID)
		require.NoError(t, err)
		require.True(t, cleared)
		require.NotNil(t, reloadGuestFiscalBill(t, bystander.ID).FiscalCustomerDocNumber)
	})
}

// Settling a split share stamps the share holder's session fingerprint on the
// ledger row, on both the alternative-tender and the payments branch.
func TestSettleBillSplitShare_StampsPayerGuestSession(t *testing.T) {
	for _, tender := range []string{"card", "crypto"} {
		t.Run(tender, func(t *testing.T) {
			sdb := setupBillSplitTestDB(t, nil)
			now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
			bill := createBillSplitBill(t, sdb, "split-stamp-"+tender, 4000)
			share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
				BillID: bill.ID, GuestSessionID: "guest-stamp", DisplayName: "Stamp",
				Mode: BillSplitModeCustom, AmountCents: 4000, HoldTTL: 5 * time.Minute, Now: now,
			})
			require.NoError(t, err)
			settled, _, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
				ShareID: share.ID, GuestSessionID: "guest-stamp", IdempotencyKey: "stamp-" + tender,
				Tender: tender, PayerAddr: "guest", Now: now.Add(time.Minute),
			})
			require.NoError(t, err)
			require.True(t, applied)
			want := guestsession.Fingerprint("guest-stamp")
			if tender == "card" {
				require.NotNil(t, settled.AlternativePaymentID)
				var alt AlternativePayment
				require.NoError(t, sdb.GetGorm().First(&alt, *settled.AlternativePaymentID).Error)
				require.NotNil(t, alt.PayerGuestSession)
				require.Equal(t, want, *alt.PayerGuestSession)
				return
			}
			require.NotNil(t, settled.PaymentID)
			var p Payment
			require.NoError(t, sdb.GetGorm().First(&p, *settled.PaymentID).Error)
			require.NotNil(t, p.PayerGuestSession)
			require.Equal(t, want, *p.PayerGuestSession)
		})
	}
}

// BenchmarkClearUnpaidGuestFiscalIdentity measures the issuance-time guard on
// a bill with a realistic handful of guest payments.
func BenchmarkClearUnpaidGuestFiscalIdentity(b *testing.B) {
	biz := setupGuestFiscalDB(b)
	bill := guestFiscalBill(b, biz, "GFI-bench", strp("fp-setter"))
	for i := 0; i < 4; i++ {
		require.NoError(b, db.Create(&Payment{
			BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 1000, TxHash: fmt.Sprintf("0xbench-%d", i),
			Status: PaymentStatusConfirmed, PayerGuestSession: strp(fmt.Sprintf("fp-payer-%d", i)),
		}).Error)
		require.NoError(b, db.Create(&AlternativePayment{
			BillID: bill.ID, ParticipantAddr: fmt.Sprintf("pi_bench_%d", i), Amount: 1000,
			PaymentMethod: AlternativePaymentMethod("stripe"), Status: AltPaymentStatusPending,
			PayerGuestSession: strp(fmt.Sprintf("fp-plugin-%d", i)),
		}).Error)
	}
	// The setter paid too, so the guard evaluates the full predicate and keeps
	// the identity on every iteration.
	require.NoError(b, db.Create(&Payment{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 1000, TxHash: "0xbench-setter",
		Status: PaymentStatusConfirmed, PayerGuestSession: strp("fp-setter"),
	}).Error)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cleared, err := ClearUnpaidGuestFiscalIdentity(db, bill.ID)
		if err != nil || cleared {
			b.Fatalf("cleared=%v err=%v", cleared, err)
		}
	}
}

func BenchmarkGuestPaymentProofForSession(b *testing.B) {
	biz := setupGuestFiscalDB(b)
	bill := guestFiscalBill(b, biz, "GFI-bench-proof", strp("fp-setter"))
	for i := 0; i < 4; i++ {
		require.NoError(b, db.Create(&AlternativePayment{
			BillID: bill.ID, ParticipantAddr: fmt.Sprintf("pi_proof_%d", i), Amount: 1000,
			PaymentMethod: AlternativePaymentMethod("stripe"), Status: AltPaymentStatusPending,
			PayerGuestSession: strp(fmt.Sprintf("fp-plugin-%d", i)),
		}).Error)
	}
	now := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GuestPaymentProofForSession(db, bill.ID, "fp-plugin-2", now); err != nil {
			b.Fatal(err)
		}
	}
}
