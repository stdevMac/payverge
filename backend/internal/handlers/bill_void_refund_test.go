package handlers

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
)

func TestRefundBillPaymentWithExternalTenderCallsPluginBeforeLedgerRefund(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}, &database.BillSplitShare{}))

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	bill.TotalAmount = 2500
	bill.PaidAmount = 2500
	bill.TipAmount = 300
	bill.Status = database.BillStatusPaid
	require.NoError(t, database.GetDB().Save(bill).Error)

	payment := &database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "plugin",
		Amount:        2500,
		TipAmount:     300,
		TxHash:        "plugin_pay_split_refund_ok",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "usd",
	}
	require.NoError(t, database.GetDB().Create(payment).Error)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "pay_split_refund_ok",
		ParticipantName: "plugin:splitrefund",
		Amount:          2800,
		PaymentMethod:   database.AlternativePaymentMethod("splitrefund"),
		Status:          database.AltPaymentStatusConfirmed,
	}).Error)

	refundPlugin := &testPaymentPlugin{name: "splitrefund"}
	registerTestPlugin(t, refundPlugin)

	handler := &PaymentHandler{}
	refundedBill, refundedPayment, err := handler.refundBillPaymentWithExternalTender(bill, payment.ID, "manager", "guest requested refund")
	require.NoError(t, err)
	require.Equal(t, 1, refundPlugin.refundCalls)
	require.Equal(t, business.ID, refundPlugin.lastRefundBizID)
	require.Equal(t, "pay_split_refund_ok", refundPlugin.lastRefundPaymentID)
	require.Equal(t, int64(2800), refundPlugin.lastRefundAmount)
	require.Equal(t, database.PaymentStatusRefunded, refundedPayment.Status)
	require.Equal(t, int64(0), refundedBill.PaidAmount)
	require.Equal(t, int64(0), refundedBill.TipAmount)
}

func TestRefundBillPaymentWithExternalTenderMarksRefundPendingBeforeProviderCall(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}, &database.BillSplitShare{}))

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	bill.TotalAmount = 2500
	bill.PaidAmount = 2500
	bill.TipAmount = 300
	bill.Status = database.BillStatusPaid
	require.NoError(t, database.GetDB().Save(bill).Error)

	payment := &database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "plugin",
		Amount:        2500,
		TipAmount:     300,
		TxHash:        "plugin_pay_split_refund_pending",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "usd",
	}
	require.NoError(t, database.GetDB().Create(payment).Error)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "pay_split_refund_pending",
		ParticipantName: "plugin:splitrefundpending",
		Amount:          2800,
		PaymentMethod:   database.AlternativePaymentMethod("splitrefundpending"),
		Status:          database.AltPaymentStatusConfirmed,
	}).Error)

	refundPlugin := &testPaymentPlugin{
		name: "splitrefundpending",
		refundHook: func() {
			var duringProviderCall database.Payment
			require.NoError(t, database.GetDB().Select("status").First(&duringProviderCall, payment.ID).Error)
			require.Equal(t, database.PaymentStatus("refund_pending"), duringProviderCall.Status)
		},
	}
	registerTestPlugin(t, refundPlugin)

	handler := &PaymentHandler{}
	refundedBill, refundedPayment, err := handler.refundBillPaymentWithExternalTender(bill, payment.ID, "manager", "guest requested refund")
	require.NoError(t, err)
	require.Equal(t, 1, refundPlugin.refundCalls)
	require.Equal(t, database.PaymentStatusRefunded, refundedPayment.Status)
	require.Equal(t, int64(0), refundedBill.PaidAmount)
}

func TestRefundBillPaymentWithExternalTenderFailureDoesNotChangeLedger(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}, &database.BillSplitShare{}))

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	bill.TotalAmount = 2500
	bill.PaidAmount = 2500
	bill.TipAmount = 300
	bill.Status = database.BillStatusPaid
	require.NoError(t, database.GetDB().Save(bill).Error)

	payment := &database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "plugin",
		Amount:        2500,
		TipAmount:     300,
		TxHash:        "plugin_pay_split_refund_fail",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "usd",
	}
	require.NoError(t, database.GetDB().Create(payment).Error)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "pay_split_refund_fail",
		ParticipantName: "plugin:splitrefundfail",
		Amount:          2800,
		PaymentMethod:   database.AlternativePaymentMethod("splitrefundfail"),
		Status:          database.AltPaymentStatusConfirmed,
	}).Error)

	refundPlugin := &testPaymentPlugin{
		name:      "splitrefundfail",
		refundErr: errors.New("provider refund unavailable"),
	}
	registerTestPlugin(t, refundPlugin)

	handler := &PaymentHandler{}
	_, _, err := handler.refundBillPaymentWithExternalTender(bill, payment.ID, "manager", "guest requested refund")
	require.ErrorIs(t, err, errPluginRefundFailed)
	require.Equal(t, 1, refundPlugin.refundCalls)

	var reloadedPayment database.Payment
	require.NoError(t, database.GetDB().First(&reloadedPayment, payment.ID).Error)
	require.Equal(t, database.PaymentStatusConfirmed, reloadedPayment.Status)

	var reloadedBill database.Bill
	require.NoError(t, database.GetDB().First(&reloadedBill, bill.ID).Error)
	require.Equal(t, int64(2500), reloadedBill.PaidAmount)
	require.Equal(t, int64(300), reloadedBill.TipAmount)
	require.Equal(t, database.BillStatusPaid, reloadedBill.Status)
}

func TestRefundBillPaymentWithExternalTenderRefusesLedgerOnlyCryptoRefund(t *testing.T) {
	for _, tc := range []struct {
		name          string
		paymentMethod string
		txHash        string
		sourceChain   string
		sourceToken   string
	}{
		{
			name:          "direct crypto",
			paymentMethod: "crypto",
			txHash:        "0xcrypto-refund-without-rail",
		},
		{
			name:          "cross chain",
			paymentMethod: "cross-chain",
			txHash:        "0xcross-chain-refund-without-rail",
			sourceChain:   "ethereum",
			sourceToken:   "USDC",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupHandlerTestDB(t)
			require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}, &database.BillSplitShare{}))

			business := createTestBusiness(t)
			bill := createTestBill(t, business.ID)
			bill.TotalAmount = 2500
			bill.PaidAmount = 2500
			bill.TipAmount = 300
			bill.Status = database.BillStatusPaid
			require.NoError(t, database.GetDB().Save(bill).Error)

			payment := &database.Payment{
				BillID:          bill.ID,
				PayerAddr:       "0xguest",
				Amount:          2500,
				TipAmount:       300,
				TxHash:          tc.txHash,
				Status:          database.PaymentStatusConfirmed,
				PaymentMethod:   tc.paymentMethod,
				SourceChain:     tc.sourceChain,
				SourceToken:     tc.sourceToken,
				SettlementChain: "base",
			}
			require.NoError(t, database.GetDB().Create(payment).Error)

			handler := &PaymentHandler{}
			_, _, err := handler.refundBillPaymentWithExternalTender(bill, payment.ID, "manager", "guest requested refund")
			require.ErrorIs(t, err, errPluginRefundUnavailable)

			var reloadedPayment database.Payment
			require.NoError(t, database.GetDB().First(&reloadedPayment, payment.ID).Error)
			require.Equal(t, database.PaymentStatusConfirmed, reloadedPayment.Status)

			var reloadedBill database.Bill
			require.NoError(t, database.GetDB().First(&reloadedBill, bill.ID).Error)
			require.Equal(t, int64(2500), reloadedBill.PaidAmount)
			require.Equal(t, int64(300), reloadedBill.TipAmount)
			require.Equal(t, database.BillStatusPaid, reloadedBill.Status)
		})
	}
}

func registerTestPlugin(t *testing.T, plugin plugins.Plugin) {
	t.Helper()
	name := plugin.GetName()
	previous, hadPrevious := plugins.GetPluginByName(name)
	plugins.GlobalRegistry.RegisterPlugin(plugin)
	t.Cleanup(func() {
		if hadPrevious {
			plugins.GlobalRegistry.RegisterPlugin(previous)
			return
		}
		plugins.GlobalRegistry.UnregisterPlugin(name)
	})
}
