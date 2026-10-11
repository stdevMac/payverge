package database

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestApplyConfirmedPayment_DedupesEVMTxHashAcrossSpellings pins the ledger
// side of the guest USDC replay fix: an EVM transaction hash is one ledger key
// regardless of hex casing or 0x prefix, so a second bill cannot be settled by
// re-spelling an already-recorded transfer.
func TestApplyConfirmedPayment_DedupesEVMTxHashAcrossSpellings(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	body := "5c504ed432cb51138bcf09aa5e8a410dd4a1e204ef84bfed1be16dfba1b22060"

	newBill := func(number string) *Bill {
		b := &Bill{BusinessID: biz.ID, BillNumber: number, Status: BillStatusOpen, Items: "[]", TotalAmount: 3000}
		require.NoError(t, db.Create(b).Error)
		return b
	}
	first := newBill("B-evm-canon-1")
	_, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID: first.ID, PayerAddr: "crypto_guest", Amount: 3000,
		TxHash: "0x" + body, Status: PaymentStatusConfirmed, PaymentMethod: "crypto",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)

	for i, variant := range []string{"0x" + strings.ToUpper(body), body, "0X" + body} {
		bill := newBill("B-evm-canon-variant-" + string(rune('a'+i)))
		_, _, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
			BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 3000,
			TxHash: variant, Status: PaymentStatusConfirmed, PaymentMethod: "crypto",
		}, nil)
		require.Errorf(t, err, "variant %q must not settle a second bill", variant)
		require.Truef(t, errors.Is(err, ErrPaymentTxHashConflict), "variant %q: want tx hash conflict, got %v", variant, err)

		var reloaded Bill
		require.NoError(t, db.First(&reloaded, bill.ID).Error)
		require.Equal(t, int64(0), reloaded.PaidAmount)
	}

	var stored Payment
	require.NoError(t, db.Where("bill_id = ?", first.ID).First(&stored).Error)
	require.Equal(t, "0x"+body, stored.TxHash, "EVM hashes are stored in canonical lowercase 0x form")
}

// TestApplyConfirmedPayment_NonEVMReferencesAreNotCaseFolded guards the scope
// of canonicalization: provider references (Stripe ids, manual refs) are
// case-sensitive identifiers and must be stored untouched.
func TestApplyConfirmedPayment_NonEVMReferencesAreNotCaseFolded(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 100)
	_, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID: bill.ID, PayerAddr: "stripe", Amount: 1000,
		TxHash: "pi_3AbCdEfGh", Status: PaymentStatusConfirmed, PaymentMethod: "stripe",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	var stored Payment
	require.NoError(t, db.Where("bill_id = ?", bill.ID).First(&stored).Error)
	require.Equal(t, "pi_3AbCdEfGh", stored.TxHash)
}
