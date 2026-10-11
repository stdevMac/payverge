package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A confirmed crypto payment records the wallet it settled into, and a refund
// resolves its source wallet from that record — not from the bill, whose
// settlement_addr wallet rotation rewrites while the check is still open.
func TestApplyConfirmedPayment_RecordsSettlementWalletForRefunds(t *testing.T) {
	setupReversalTestDB(t)
	require.NoError(t, GetDB().AutoMigrate(&CryptoPaymentQuote{}))
	biz := helperBusiness(t, 0, 0)
	const oldWallet = "0x1111111111111111111111111111111111111111"
	const newWallet = "0x2222222222222222222222222222222222222222"

	bill := helperBill(t, biz, nil, 100)
	require.NoError(t, GetDB().Model(bill).Update("settlement_addr", oldWallet).Error)

	_, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "crypto_guest",
		Amount:        4000,
		TxHash:        "0xcrosschainpart",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "cross_chain",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)

	var payment Payment
	require.NoError(t, GetDB().Where("bill_id = ? AND tx_hash = ?", bill.ID, "0xcrosschainpart").First(&payment).Error)
	require.NotNil(t, payment.SettlementAddr)
	require.Equal(t, oldWallet, *payment.SettlementAddr)

	// The business rotates its payout wallet while the check is still open.
	require.NoError(t, GetDB().Model(&Bill{}).Where("id = ?", bill.ID).Update("settlement_addr", newWallet).Error)

	wallet, err := CryptoRefundSourceWallet(nil, payment.ID)
	require.NoError(t, err)
	require.Equal(t, oldWallet, wallet)

	// A non-crypto payment records no wallet, and a crypto row with nothing
	// stored is unknown rather than guessed from the bill.
	cashBill := helperBill(t, biz, nil, 100)
	_, applied, err = ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID: cashBill.ID, PayerAddr: "cash", Amount: 1000,
		Status: PaymentStatusConfirmed, PaymentMethod: "cash",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	var cash Payment
	require.NoError(t, GetDB().Where("bill_id = ?", cashBill.ID).First(&cash).Error)
	require.Nil(t, cash.SettlementAddr)

	require.NoError(t, GetDB().Model(&Payment{}).Where("id = ?", payment.ID).Update("settlement_addr", nil).Error)
	_, err = CryptoRefundSourceWallet(nil, payment.ID)
	require.ErrorIs(t, err, ErrCryptoRefundSourceWalletUnknown)
}
