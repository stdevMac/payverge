package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Rotating the settlement wallet must move every still-payable bill to the
// new recipient and expire quotes signed for the old one, atomically with the
// business save. Terminal bills keep the wallet they settled to.
func TestUpdateBusinessExceptDesignRotatingWallets_MovesOpenBillsAndExpiresQuotes(t *testing.T) {
	gdb := setupCryptoQuoteTestDB(t)
	now := time.Now().UTC()

	open := createBillSplitBill(t, gdb, "rotate-open", 5000)
	var business Business
	require.NoError(t, gdb.GetGorm().First(&business, open.BusinessID).Error)
	require.NoError(t, gdb.GetGorm().Model(&Business{}).Where("id = ?", business.ID).
		Update("settlement_addr", quoteTestWallet).Error)
	business.SettlementAddr = quoteTestWallet

	partial := &Bill{BusinessID: business.ID, BillNumber: "rotate-partial", Items: "[]", TotalAmount: 5000, PaidAmount: 1000,
		Status: BillStatusPartial, SettlementAddr: quoteTestWallet, TippingAddr: business.TippingAddr}
	closed := &Bill{BusinessID: business.ID, BillNumber: "rotate-closed", Items: "[]", TotalAmount: 5000, PaidAmount: 5000,
		Status: BillStatusPaid, SettlementAddr: quoteTestWallet, TippingAddr: business.TippingAddr}
	require.NoError(t, gdb.GetGorm().Create(partial).Error)
	require.NoError(t, gdb.GetGorm().Create(closed).Error)
	require.NoError(t, gdb.GetGorm().Model(open).Update("settlement_addr", quoteTestWallet).Error)

	quote, err := IssueCryptoPaymentQuote(quoteInput(open, quoteTestWallet, 50_000_000, now))
	require.NoError(t, err)
	require.Equal(t, CryptoPaymentQuoteStatusActive, quote.Status)

	const rotated = "0x2222222222222222222222222222222222222222"
	business.SettlementAddr = rotated
	require.NoError(t, UpdateBusinessExceptDesignRotatingWallets(&business, true, false))

	addrOf := func(id uint) (string, string) {
		var row Bill
		require.NoError(t, gdb.GetGorm().Select("settlement_addr", "tipping_addr").First(&row, id).Error)
		return row.SettlementAddr, row.TippingAddr
	}
	s, tip := addrOf(open.ID)
	require.Equal(t, rotated, s)
	require.Equal(t, business.TippingAddr, tip, "a settlement rotation must not touch tipping_addr")
	s, _ = addrOf(partial.ID)
	require.Equal(t, rotated, s)
	s, _ = addrOf(closed.ID)
	require.Equal(t, quoteTestWallet, s, "terminal bills keep the wallet they settled to")

	var reloaded CryptoPaymentQuote
	require.NoError(t, gdb.GetGorm().First(&reloaded, quote.ID).Error)
	require.Equal(t, CryptoPaymentQuoteStatusExpired, reloaded.Status)
	require.Nil(t, reloaded.ClientKey)
}

// A save that does not rotate a wallet leaves bills and quotes alone.
func TestUpdateBusinessExceptDesign_NoRotationKeepsQuotesActive(t *testing.T) {
	gdb := setupCryptoQuoteTestDB(t)
	open := createBillSplitBill(t, gdb, "norotate-open", 5000)
	var business Business
	require.NoError(t, gdb.GetGorm().First(&business, open.BusinessID).Error)

	quote, err := IssueCryptoPaymentQuote(quoteInput(open, quoteTestWallet, 50_000_000, time.Now().UTC()))
	require.NoError(t, err)

	business.Name = "Renamed"
	require.NoError(t, UpdateBusinessExceptDesign(&business))

	var reloaded CryptoPaymentQuote
	require.NoError(t, gdb.GetGorm().First(&reloaded, quote.ID).Error)
	require.Equal(t, CryptoPaymentQuoteStatusActive, reloaded.Status)
}
