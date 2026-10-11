package cryptorefund

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/blockchain"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type mockOutbound struct {
	ev  blockchain.USDCTransferEvidence
	err error
}

func (m *mockOutbound) VerifyOutboundUSDCTransfer(ctx context.Context, txHash string, from string, to string, expectedAmountMicrounits int64) (blockchain.USDCTransferEvidence, error) {
	return m.ev, m.err
}

func (m *mockOutbound) MinConfirmations() uint64 { return 3 }

func setupWorkerDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf("file:cr_worker_%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	// package-level db used by database helpers
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.PaymentRefundDestination{},
		&database.PaymentRefund{},
		&database.CryptoPaymentQuote{},
		&database.BillHistoryEvent{},
		&database.BillSplitShare{},
		&database.BusinessRevenueAggregate{},
		&database.BusinessMilestoneEvent{},
		&database.BusinessFiscalSettings{},
		&database.FiscalReceipt{},
		&database.FiscalJob{},
		&database.FiscalAuditEvent{},
	))
}

func seedAuthorizedFiscalReceipt(t *testing.T, db *gorm.DB, biz *database.Business, bill *database.Bill, payment *database.Payment) *database.FiscalReceipt {
	t.Helper()
	settings := &database.BusinessFiscalSettings{
		BusinessID: biz.ID, Country: "AR", Provider: "arca",
		Mode: database.FiscalModeAutomaticNonBlocking, Environment: "sandbox", SetupStatus: "validated",
	}
	require.NoError(t, db.Create(settings).Error)
	receiptNumber := "1"
	receipt := &database.FiscalReceipt{
		BusinessID: biz.ID, SettingsID: settings.ID, BillID: bill.ID, PaymentID: &payment.ID,
		Country: "AR", Provider: "arca", Action: "issue_receipt", ReceiptType: "factura_b",
		ReceiptNumber: &receiptNumber, TotalAmountCents: bill.TotalAmount, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}
	require.NoError(t, db.Create(receipt).Error)
	return receipt
}

func TestWorker_ConfirmsAndAppliesLedgerOnce(t *testing.T) {
	setupWorkerDB(t)
	db := database.GetDB()

	biz := &database.Business{Name: "W", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &database.Bill{
		BusinessID: biz.ID, BillNumber: "BW1", TotalAmount: 2500, PaidAmount: 2500,
		Status: database.BillStatusPaid, SettlementAddr: biz.SettlementAddr,
	}
	require.NoError(t, db.Create(bill).Error)
	pay := &database.Payment{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 2500,
		TxHash: "0xworkerpay1", Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		SettlementAddr: &bill.SettlementAddr,
	}
	require.NoError(t, db.Create(pay).Error)
	require.NoError(t, database.GetDB().Create(&database.PaymentRefundDestination{
		PaymentID: pay.ID, ChainID: 84532, Token: "USDC", AmountBaseUnits: 5_000_000,
		RefundAddress: "0xdead00000000000000000000000000000000beef",
		EvidenceType:  database.RefundEvidenceTransferLog, VerifiedAt: time.Now().UTC(),
	}).Error)

	row, err := database.RequestCryptoRefund(database.CreateCryptoRefundInput{
		BusinessID: biz.ID, BillID: bill.ID, PaymentID: pay.ID,
		AmountBaseUnits: 5_000_000, Reason: "worker test",
		RequestedBy: "owner", IdempotencyKey: "w-1",
	})
	require.NoError(t, err)
	_, err = database.ApproveCryptoRefund(row.ID, biz.ID, "owner")
	require.NoError(t, err)
	_, err = database.SubmitCryptoRefundTxHash(row.ID, biz.ID, "0x6e96999b14078c2fb4d3d21e19c68878248ff3266691e766e07d237a8a328f87")
	require.NoError(t, err)

	// Reload submitted row
	row, err = database.GetCryptoRefund(row.ID, biz.ID)
	require.NoError(t, err)

	var creditCalls int
	w := NewWorker(db, &mockOutbound{
		ev: blockchain.USDCTransferEvidence{
			From: biz.SettlementAddr, To: row.VerifiedRecipient,
			AmountBaseUnits: 5_000_000, Confirmations: 3, TxHash: "0x6e96999b14078c2fb4d3d21e19c68878248ff3266691e766e07d237a8a328f87",
			ChainID: 84532, Token: "USDC",
		},
	}, func(b *database.Bill, amountCents int64, disc, actor string) {
		creditCalls++
		require.Equal(t, int64(2500), amountCents)
		require.Contains(t, disc, "payment:")
	}, Config{MainnetEnabled: true, WorkerID: "t", LeaseTTL: time.Minute})

	require.NoError(t, w.ProcessOneForTest(context.Background(), *row))

	var payReloaded database.Payment
	require.NoError(t, db.First(&payReloaded, pay.ID).Error)
	require.Equal(t, database.PaymentStatusRefunded, payReloaded.Status)

	var billReloaded database.Bill
	require.NoError(t, db.First(&billReloaded, bill.ID).Error)
	require.Equal(t, int64(0), billReloaded.PaidAmount)

	refund, err := database.GetCryptoRefund(row.ID, biz.ID)
	require.NoError(t, err)
	require.Equal(t, database.PaymentRefundStatusConfirmed, refund.Status)
	require.Equal(t, 1, creditCalls)

	// Second process is idempotent — no second credit note, no double ledger.
	require.NoError(t, w.ProcessOneForTest(context.Background(), *refund))
	require.Equal(t, 1, creditCalls, "credit note must fire exactly once")
}

// TestWorker_ConfirmPersistsBlockEvidence pins that a confirmed refund records
// the block it settled in, so the reorg reconciler has the durable input it
// needs to later re-verify against the canonical chain.
func TestWorker_ConfirmPersistsBlockEvidence(t *testing.T) {
	setupWorkerDB(t)
	db := database.GetDB()

	biz := &database.Business{Name: "WB", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &database.Bill{
		BusinessID: biz.ID, BillNumber: "BW2", TotalAmount: 2500, PaidAmount: 2500,
		Status: database.BillStatusPaid, SettlementAddr: biz.SettlementAddr,
	}
	require.NoError(t, db.Create(bill).Error)
	pay := &database.Payment{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 2500,
		TxHash: "0xworkerpay2", Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		SettlementAddr: &bill.SettlementAddr,
	}
	require.NoError(t, db.Create(pay).Error)
	require.NoError(t, database.GetDB().Create(&database.PaymentRefundDestination{
		PaymentID: pay.ID, ChainID: 84532, Token: "USDC", AmountBaseUnits: 5_000_000,
		RefundAddress: "0xdead00000000000000000000000000000000beef",
		EvidenceType:  database.RefundEvidenceTransferLog, VerifiedAt: time.Now().UTC(),
	}).Error)

	row, err := database.RequestCryptoRefund(database.CreateCryptoRefundInput{
		BusinessID: biz.ID, BillID: bill.ID, PaymentID: pay.ID,
		AmountBaseUnits: 5_000_000, Reason: "block evidence", RequestedBy: "owner", IdempotencyKey: "w-2",
	})
	require.NoError(t, err)
	_, err = database.ApproveCryptoRefund(row.ID, biz.ID, "owner")
	require.NoError(t, err)
	_, err = database.SubmitCryptoRefundTxHash(row.ID, biz.ID, "0x03aec143897721d6db6e191e77007a1d02edc7e4593be5e20cd0c200abc19dca")
	require.NoError(t, err)
	row, err = database.GetCryptoRefund(row.ID, biz.ID)
	require.NoError(t, err)

	w := NewWorker(db, &mockOutbound{
		ev: blockchain.USDCTransferEvidence{
			From: biz.SettlementAddr, To: row.VerifiedRecipient,
			AmountBaseUnits: 5_000_000, Confirmations: 3, TxHash: "0x03aec143897721d6db6e191e77007a1d02edc7e4593be5e20cd0c200abc19dca",
			ChainID: 84532, Token: "USDC",
			BlockNumber: 987654, BlockHash: "0xfeedface",
		},
	}, nil, Config{MainnetEnabled: true, WorkerID: "t", LeaseTTL: time.Minute})

	require.NoError(t, w.ProcessOneForTest(context.Background(), *row))

	refund, err := database.GetCryptoRefund(row.ID, biz.ID)
	require.NoError(t, err)
	require.Equal(t, database.PaymentRefundStatusConfirmed, refund.Status)
	require.NotNil(t, refund.BlockHash, "confirmed refund must persist its settlement block hash")
	require.Equal(t, "0xfeedface", *refund.BlockHash)
	require.NotNil(t, refund.BlockNumber)
	require.Equal(t, int64(987654), *refund.BlockNumber)
}

func TestWorker_ConfirmationPersistsFiscalCreditJobBeforeCommit(t *testing.T) {
	setupWorkerDB(t)
	db := database.GetDB()
	biz := &database.Business{Name: "Fiscal W", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &database.Bill{BusinessID: biz.ID, BillNumber: "BWF1", TotalAmount: 2500, PaidAmount: 2500, Status: database.BillStatusPaid, SettlementAddr: biz.SettlementAddr}
	require.NoError(t, db.Create(bill).Error)
	pay := &database.Payment{BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 2500, TxHash: "0xfiscalpay", Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto", SettlementAddr: &bill.SettlementAddr}
	require.NoError(t, db.Create(pay).Error)
	seedAuthorizedFiscalReceipt(t, db, biz, bill, pay)
	require.NoError(t, database.GetDB().Create(&database.PaymentRefundDestination{
		PaymentID: pay.ID, ChainID: 84532, Token: "USDC", AmountBaseUnits: 5_000_000,
		RefundAddress: "0xdead00000000000000000000000000000000beef", EvidenceType: database.RefundEvidenceTransferLog,
	}).Error)
	row, err := database.RequestCryptoRefund(database.CreateCryptoRefundInput{
		BusinessID: biz.ID, BillID: bill.ID, PaymentID: pay.ID, AmountBaseUnits: 5_000_000,
		Reason: "fiscal durability", RequestedBy: "owner", IdempotencyKey: "fiscal-durable",
	})
	require.NoError(t, err)
	_, err = database.ApproveCryptoRefund(row.ID, biz.ID, "owner")
	require.NoError(t, err)
	row, err = database.SubmitCryptoRefundTxHash(row.ID, biz.ID, "0x7cd0e92cff676adde8beb4588e03ac0a86bf15ba84c99d0406457db72e82c901")
	require.NoError(t, err)

	w := NewWorker(db, &mockOutbound{ev: blockchain.USDCTransferEvidence{
		From: biz.SettlementAddr, To: row.VerifiedRecipient, AmountBaseUnits: 5_000_000,
		Confirmations: 3, TxHash: "0x7cd0e92cff676adde8beb4588e03ac0a86bf15ba84c99d0406457db72e82c901", ChainID: 84532, Token: "USDC",
	}}, nil, Config{MainnetEnabled: true, WorkerID: "fiscal-test"})
	require.NoError(t, w.ProcessOneForTest(context.Background(), *row))

	var jobs []database.FiscalJob
	require.NoError(t, db.Where("bill_id = ? AND action = ?", bill.ID, "credit_note").Find(&jobs).Error)
	require.Len(t, jobs, 1)
	require.Equal(t, database.FiscalStatusPending, jobs[0].Status)
	require.Equal(t, int64(2500), *jobs[0].CreditAmountCents)
}

func TestWorker_FiscalOutboxFailureRollsBackLedgerAndRetryReconciles(t *testing.T) {
	setupWorkerDB(t)
	db := database.GetDB()
	biz := &database.Business{Name: "Fiscal Retry W", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &database.Bill{BusinessID: biz.ID, BillNumber: "BWF2", TotalAmount: 2500, PaidAmount: 2500, Status: database.BillStatusPaid, SettlementAddr: biz.SettlementAddr}
	require.NoError(t, db.Create(bill).Error)
	pay := &database.Payment{BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 2500, TxHash: "0xfiscalpay2", Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto", SettlementAddr: &bill.SettlementAddr}
	require.NoError(t, db.Create(pay).Error)
	seedAuthorizedFiscalReceipt(t, db, biz, bill, pay)
	require.NoError(t, database.GetDB().Create(&database.PaymentRefundDestination{
		PaymentID: pay.ID, ChainID: 84532, Token: "USDC", AmountBaseUnits: 5_000_000,
		RefundAddress: "0xdead00000000000000000000000000000000beef", EvidenceType: database.RefundEvidenceTransferLog,
	}).Error)
	row, err := database.RequestCryptoRefund(database.CreateCryptoRefundInput{
		BusinessID: biz.ID, BillID: bill.ID, PaymentID: pay.ID, AmountBaseUnits: 5_000_000,
		Reason: "fiscal retry", RequestedBy: "owner", IdempotencyKey: "fiscal-retry",
	})
	require.NoError(t, err)
	_, err = database.ApproveCryptoRefund(row.ID, biz.ID, "owner")
	require.NoError(t, err)
	row, err = database.SubmitCryptoRefundTxHash(row.ID, biz.ID, "0x59df872085cd46ad1a17390100f3d2046fcfa21f3ba0e196d9b214b5aca9464b")
	require.NoError(t, err)

	w := NewWorker(db, &mockOutbound{ev: blockchain.USDCTransferEvidence{
		From: biz.SettlementAddr, To: row.VerifiedRecipient, AmountBaseUnits: 5_000_000,
		Confirmations: 3, TxHash: "0x59df872085cd46ad1a17390100f3d2046fcfa21f3ba0e196d9b214b5aca9464b", ChainID: 84532, Token: "USDC",
	}}, nil, Config{MainnetEnabled: true, WorkerID: "fiscal-retry-test"})
	callbackName := "test:block-fiscal-credit-job"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "fiscal_jobs" {
			tx.AddError(errors.New("simulated fiscal outbox write failure"))
		}
	}))
	err = w.ProcessOneForTest(context.Background(), *row)
	require.Error(t, err)
	require.NoError(t, db.Callback().Create().Remove(callbackName))

	var payAfterFailure database.Payment
	require.NoError(t, db.First(&payAfterFailure, pay.ID).Error)
	require.Equal(t, database.PaymentStatusConfirmed, payAfterFailure.Status)
	refundAfterFailure, err := database.GetCryptoRefund(row.ID, biz.ID)
	require.NoError(t, err)
	require.Equal(t, database.PaymentRefundStatusConfirming, refundAfterFailure.Status)
	require.Nil(t, refundAfterFailure.LedgerAppliedAt)

	require.NoError(t, w.ProcessOneForTest(context.Background(), *refundAfterFailure))
	var jobCount int64
	require.NoError(t, db.Model(&database.FiscalJob{}).Where("bill_id = ? AND action = ?", bill.ID, "credit_note").Count(&jobCount).Error)
	require.EqualValues(t, 1, jobCount)
	var payAfterRetry database.Payment
	require.NoError(t, db.First(&payAfterRetry, pay.ID).Error)
	require.Equal(t, database.PaymentStatusRefunded, payAfterRetry.Status)
}

func TestWorker_AlreadyFullyCreditedFiscalReceiptDoesNotBlockLedgerReconciliation(t *testing.T) {
	setupWorkerDB(t)
	db := database.GetDB()
	biz := &database.Business{Name: "Overcredited W", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &database.Bill{BusinessID: biz.ID, BillNumber: "BWF3", TotalAmount: 2500, PaidAmount: 2500, Status: database.BillStatusPaid, SettlementAddr: biz.SettlementAddr}
	require.NoError(t, db.Create(bill).Error)
	pay := &database.Payment{BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 2500, TxHash: "0xfiscalpay3", Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto", SettlementAddr: &bill.SettlementAddr}
	require.NoError(t, db.Create(pay).Error)
	receipt := seedAuthorizedFiscalReceipt(t, db, biz, bill, pay)
	require.NoError(t, db.Create(&database.FiscalJob{
		BusinessID: biz.ID, SettingsID: receipt.SettingsID, ReceiptID: &receipt.ID, BillID: bill.ID,
		Action: "credit_note", IdempotencyKey: "existing-full-credit", Status: database.FiscalStatusAuthorized,
		MaxAttempts: 5, CreatedBy: "operator",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.PaymentRefundDestination{
		PaymentID: pay.ID, ChainID: 84532, Token: "USDC", AmountBaseUnits: 5_000_000,
		RefundAddress: "0xdead00000000000000000000000000000000beef", EvidenceType: database.RefundEvidenceTransferLog,
	}).Error)
	row, err := database.RequestCryptoRefund(database.CreateCryptoRefundInput{
		BusinessID: biz.ID, BillID: bill.ID, PaymentID: pay.ID, AmountBaseUnits: 5_000_000,
		Reason: "already tax credited", RequestedBy: "owner", IdempotencyKey: "fiscal-overcredit",
	})
	require.NoError(t, err)
	_, err = database.ApproveCryptoRefund(row.ID, biz.ID, "owner")
	require.NoError(t, err)
	row, err = database.SubmitCryptoRefundTxHash(row.ID, biz.ID, "0xff1ca578d3f22740bb70debfe8cfc413557ca053bec21aa159b84cc234c2f78c")
	require.NoError(t, err)
	w := NewWorker(db, &mockOutbound{ev: blockchain.USDCTransferEvidence{
		From: biz.SettlementAddr, To: row.VerifiedRecipient, AmountBaseUnits: 5_000_000,
		Confirmations: 3, TxHash: "0xff1ca578d3f22740bb70debfe8cfc413557ca053bec21aa159b84cc234c2f78c", ChainID: 84532, Token: "USDC",
	}}, nil, Config{MainnetEnabled: true, WorkerID: "fiscal-overcredit-test"})

	require.NoError(t, w.ProcessOneForTest(context.Background(), *row))
	var reloadedPayment database.Payment
	require.NoError(t, db.First(&reloadedPayment, pay.ID).Error)
	require.Equal(t, database.PaymentStatusRefunded, reloadedPayment.Status)
	var audit database.FiscalAuditEvent
	require.NoError(t, db.Where("business_id = ? AND event_type = ?", biz.ID, "refund_credit_note_not_enqueued").First(&audit).Error)
	require.Contains(t, audit.Message, "already fully credited")
	confirmed, err := database.GetCryptoRefund(row.ID, biz.ID)
	require.NoError(t, err)
	require.NoError(t, w.ProcessOneForTest(context.Background(), *confirmed))
	var auditCount int64
	require.NoError(t, db.Model(&database.FiscalAuditEvent{}).
		Where("business_id = ? AND event_type = ?", biz.ID, "refund_credit_note_not_enqueued").
		Count(&auditCount).Error)
	require.EqualValues(t, 1, auditCount, "confirmed refund replay must not duplicate the skip audit")
}

func TestWorker_PartiallyCreditedFiscalReceiptBlocksLedgerUntilReconciled(t *testing.T) {
	setupWorkerDB(t)
	db := database.GetDB()
	biz := &database.Business{Name: "Partially Credited W", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &database.Bill{BusinessID: biz.ID, BillNumber: "BWF4", TotalAmount: 2500, PaidAmount: 2500, Status: database.BillStatusPaid, SettlementAddr: biz.SettlementAddr}
	require.NoError(t, db.Create(bill).Error)
	pay := &database.Payment{BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 2500, TxHash: "0xfiscalpay4", Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto", SettlementAddr: &bill.SettlementAddr}
	require.NoError(t, db.Create(pay).Error)
	receipt := seedAuthorizedFiscalReceipt(t, db, biz, bill, pay)
	partialAmount := int64(1000)
	require.NoError(t, db.Create(&database.FiscalJob{
		BusinessID: biz.ID, SettingsID: receipt.SettingsID, ReceiptID: &receipt.ID, BillID: bill.ID,
		Action: "credit_note", IdempotencyKey: "existing-partial-credit", CreditAmountCents: &partialAmount,
		Status: database.FiscalStatusAuthorized, MaxAttempts: 5, CreatedBy: "operator",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.PaymentRefundDestination{
		PaymentID: pay.ID, ChainID: 84532, Token: "USDC", AmountBaseUnits: 5_000_000,
		RefundAddress: "0xdead00000000000000000000000000000000beef", EvidenceType: database.RefundEvidenceTransferLog,
	}).Error)
	row, err := database.RequestCryptoRefund(database.CreateCryptoRefundInput{
		BusinessID: biz.ID, BillID: bill.ID, PaymentID: pay.ID, AmountBaseUnits: 5_000_000,
		Reason: "partial fiscal credit", RequestedBy: "owner", IdempotencyKey: "fiscal-partial-credit",
	})
	require.NoError(t, err)
	_, err = database.ApproveCryptoRefund(row.ID, biz.ID, "owner")
	require.NoError(t, err)
	row, err = database.SubmitCryptoRefundTxHash(row.ID, biz.ID, "0xc04bf7844793a9bf3ebec875c5a1226b53a1995e229a9e4018d7536c3e5c5026")
	require.NoError(t, err)
	w := NewWorker(db, &mockOutbound{ev: blockchain.USDCTransferEvidence{
		From: biz.SettlementAddr, To: row.VerifiedRecipient, AmountBaseUnits: 5_000_000,
		Confirmations: 3, TxHash: "0xc04bf7844793a9bf3ebec875c5a1226b53a1995e229a9e4018d7536c3e5c5026", ChainID: 84532, Token: "USDC",
	}}, nil, Config{MainnetEnabled: true, WorkerID: "fiscal-partial-credit-test"})

	err = w.ProcessOneForTest(context.Background(), *row)
	require.Error(t, err)
	require.Contains(t, err.Error(), "credit note would exceed")
	var reloadedPayment database.Payment
	require.NoError(t, db.First(&reloadedPayment, pay.ID).Error)
	require.Equal(t, database.PaymentStatusConfirmed, reloadedPayment.Status)
	var auditCount int64
	require.NoError(t, db.Model(&database.FiscalAuditEvent{}).
		Where("business_id = ? AND event_type = ?", biz.ID, "refund_credit_note_not_enqueued").Count(&auditCount).Error)
	require.Zero(t, auditCount)
}

func TestWorker_AwaitingConfirmationsDoesNotMarkRefunded(t *testing.T) {
	setupWorkerDB(t)
	db := database.GetDB()

	biz := &database.Business{Name: "W2", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &database.Bill{
		BusinessID: biz.ID, BillNumber: "BW2", TotalAmount: 1000, PaidAmount: 1000,
		Status: database.BillStatusPaid, SettlementAddr: biz.SettlementAddr,
	}
	require.NoError(t, db.Create(bill).Error)
	pay := &database.Payment{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 1000,
		TxHash: "0xworkerpay2", Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		SettlementAddr: &bill.SettlementAddr,
	}
	require.NoError(t, db.Create(pay).Error)
	require.NoError(t, database.GetDB().Create(&database.PaymentRefundDestination{
		PaymentID: pay.ID, ChainID: 84532, Token: "USDC", AmountBaseUnits: 1_000_000,
		RefundAddress: "0xdead00000000000000000000000000000000beef",
		EvidenceType:  database.RefundEvidenceTransferLog, VerifiedAt: time.Now().UTC(),
	}).Error)
	row, err := database.RequestCryptoRefund(database.CreateCryptoRefundInput{
		BusinessID: biz.ID, BillID: bill.ID, PaymentID: pay.ID,
		AmountBaseUnits: 1_000_000, Reason: "await conf",
		RequestedBy: "owner", IdempotencyKey: "w-2",
	})
	require.NoError(t, err)
	_, err = database.ApproveCryptoRefund(row.ID, biz.ID, "owner")
	require.NoError(t, err)
	_, err = database.SubmitCryptoRefundTxHash(row.ID, biz.ID, "0x03aec143897721d6db6e191e77007a1d02edc7e4593be5e20cd0c200abc19dca")
	require.NoError(t, err)
	row, _ = database.GetCryptoRefund(row.ID, biz.ID)

	w := NewWorker(db, &mockOutbound{
		err: fmt.Errorf("%w: 1/3 confirmations", blockchain.ErrAwaitingConfirmations),
	}, nil, Config{MainnetEnabled: true, WorkerID: "t"})

	require.NoError(t, w.ProcessOneForTest(context.Background(), *row))

	refund, err := database.GetCryptoRefund(row.ID, biz.ID)
	require.NoError(t, err)
	require.Equal(t, database.PaymentRefundStatusConfirming, refund.Status)
	require.NotEqual(t, database.PaymentRefundStatusConfirmed, refund.Status)

	var payReloaded database.Payment
	require.NoError(t, db.First(&payReloaded, pay.ID).Error)
	require.Equal(t, database.PaymentStatusConfirmed, payReloaded.Status, "ledger must stay confirmed until on-chain confirmed")
}

func TestWorker_WrongFromFailsWithoutLedgerChange(t *testing.T) {
	setupWorkerDB(t)
	db := database.GetDB()

	biz := &database.Business{Name: "W3", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, db.Create(biz).Error)
	bill := &database.Bill{
		BusinessID: biz.ID, BillNumber: "BW3", TotalAmount: 1000, PaidAmount: 1000,
		Status: database.BillStatusPaid, SettlementAddr: biz.SettlementAddr,
	}
	require.NoError(t, db.Create(bill).Error)
	pay := &database.Payment{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 1000,
		TxHash: "0xworkerpay3", Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		SettlementAddr: &bill.SettlementAddr,
	}
	require.NoError(t, db.Create(pay).Error)
	require.NoError(t, database.GetDB().Create(&database.PaymentRefundDestination{
		PaymentID: pay.ID, ChainID: 84532, Token: "USDC", AmountBaseUnits: 1_000_000,
		RefundAddress: "0xdead00000000000000000000000000000000beef",
		EvidenceType:  database.RefundEvidenceTransferLog, VerifiedAt: time.Now().UTC(),
	}).Error)
	row, err := database.RequestCryptoRefund(database.CreateCryptoRefundInput{
		BusinessID: biz.ID, BillID: bill.ID, PaymentID: pay.ID,
		AmountBaseUnits: 1_000_000, Reason: "bad from",
		RequestedBy: "owner", IdempotencyKey: "w-3",
	})
	require.NoError(t, err)
	_, err = database.ApproveCryptoRefund(row.ID, biz.ID, "owner")
	require.NoError(t, err)
	_, err = database.SubmitCryptoRefundTxHash(row.ID, biz.ID, "0x8bc4f8a1271ac88f3ab91be7880112fee9d05e8281b4ae610267e787e6f6c20b")
	require.NoError(t, err)
	row, _ = database.GetCryptoRefund(row.ID, biz.ID)

	w := NewWorker(db, &mockOutbound{
		err: errors.New("no matching USDC transfer found for transaction"),
	}, nil, Config{MainnetEnabled: true, WorkerID: "t"})

	require.NoError(t, w.ProcessOneForTest(context.Background(), *row))

	refund, err := database.GetCryptoRefund(row.ID, biz.ID)
	require.NoError(t, err)
	require.Equal(t, database.PaymentRefundStatusFailed, refund.Status)

	var payReloaded database.Payment
	require.NoError(t, db.First(&payReloaded, pay.ID).Error)
	require.Equal(t, database.PaymentStatusConfirmed, payReloaded.Status)
}

func TestDefaultConfig_MainnetOffByDefault(t *testing.T) {
	t.Setenv("CRYPTO_REFUND_MAINNET_ENABLED", "")
	cfg := DefaultConfig()
	require.False(t, cfg.MainnetEnabled, "mainnet must be OFF by default")
}

// fromCheckingOutbound only confirms a transfer sent from wantFrom, like the
// real verifier does with the Transfer log's sender.
type fromCheckingOutbound struct {
	wantFrom string
	gotFrom  string
	ev       blockchain.USDCTransferEvidence
}

func (m *fromCheckingOutbound) VerifyOutboundUSDCTransfer(ctx context.Context, txHash string, from string, to string, expectedAmountMicrounits int64) (blockchain.USDCTransferEvidence, error) {
	m.gotFrom = from
	if !strings.EqualFold(from, m.wantFrom) {
		return blockchain.USDCTransferEvidence{}, errors.New("no matching USDC transfer found for transaction")
	}
	return m.ev, nil
}

func (m *fromCheckingOutbound) MinConfirmations() uint64 { return 1 }

// seedRotatedWalletRefund pays a bill into oldWallet, then rotates the
// business to newWallet, and returns a submitted refund for that payment.
func seedRotatedWalletRefund(t *testing.T, withQuote bool) (*database.Business, database.PaymentRefund) {
	t.Helper()
	db := database.GetDB()
	const oldWallet = "0x1111111111111111111111111111111111111111"
	const newWallet = "0x2222222222222222222222222222222222222222"
	biz := &database.Business{Name: "Rotated", SettlementAddr: oldWallet}
	require.NoError(t, db.Create(biz).Error)
	bill := &database.Bill{
		BusinessID: biz.ID, BillNumber: "BROT", TotalAmount: 2500, PaidAmount: 2500,
		Status: database.BillStatusPaid, SettlementAddr: oldWallet,
	}
	require.NoError(t, db.Create(bill).Error)
	pay := &database.Payment{
		BillID: bill.ID, PayerAddr: "crypto_guest", Amount: 2500,
		TxHash: "0xrotpay", Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
	}
	if !withQuote {
		// Quote-less payments carry the wallet they settled into on the row.
		wallet := oldWallet
		pay.SettlementAddr = &wallet
	}
	require.NoError(t, db.Create(pay).Error)
	if withQuote {
		now := time.Now().UTC()
		require.NoError(t, db.Create(&database.CryptoPaymentQuote{
			BillID: bill.ID, BusinessID: biz.ID, ChainID: 84532, SettlementAddress: oldWallet,
			ExactMicrounits: 5_000_000, PaymentMethod: "usdc_payment", BaseMicrounits: 5_000_000,
			Status: database.CryptoPaymentQuoteStatusConsumed, IssuedAt: now, ExpiresAt: now.Add(time.Minute),
			PaymentID: &pay.ID,
		}).Error)
	}
	// Rotation rewrites open checks' settlement_addr: the stored snapshot must
	// win over the bill row as well as over the live business wallet.
	require.NoError(t, db.Model(bill).Update("settlement_addr", newWallet).Error)
	require.NoError(t, db.Create(&database.PaymentRefundDestination{
		PaymentID: pay.ID, ChainID: 84532, Token: "USDC", AmountBaseUnits: 5_000_000,
		RefundAddress: "0xdead00000000000000000000000000000000beef",
		EvidenceType:  database.RefundEvidenceTransferLog, VerifiedAt: time.Now().UTC(),
	}).Error)
	row, err := database.RequestCryptoRefund(database.CreateCryptoRefundInput{
		BusinessID: biz.ID, BillID: bill.ID, PaymentID: pay.ID,
		AmountBaseUnits: 5_000_000, Reason: "rotation", RequestedBy: "owner", IdempotencyKey: "rot-1",
	})
	require.NoError(t, err)
	_, err = database.ApproveCryptoRefund(row.ID, biz.ID, "owner")
	require.NoError(t, err)
	_, err = database.SubmitCryptoRefundTxHash(row.ID, biz.ID, "0x6e96999b14078c2fb4d3d21e19c68878248ff3266691e766e07d237a8a328f87")
	require.NoError(t, err)

	// Rotate the live payout wallet after the payment.
	require.NoError(t, db.Model(biz).Update("settlement_addr", newWallet).Error)

	row, err = database.GetCryptoRefund(row.ID, biz.ID)
	require.NoError(t, err)
	return biz, *row
}

func TestWorker_RefundAfterWalletRotationVerifiesAgainstPaymentWallet(t *testing.T) {
	for _, tc := range []struct {
		name      string
		withQuote bool
	}{{"quote snapshot", true}, {"payment snapshot", false}} {
		t.Run(tc.name, func(t *testing.T) {
			setupWorkerDB(t)
			biz, row := seedRotatedWalletRefund(t, tc.withQuote)
			ev := blockchain.USDCTransferEvidence{
				To: row.VerifiedRecipient, AmountBaseUnits: 5_000_000, Confirmations: 1,
				TxHash: *row.SubmittedTxHash, ChainID: 84532, Token: "USDC",
			}

			// A refund sent from the new (live) wallet does not match the payment.
			wrong := &fromCheckingOutbound{wantFrom: "0x2222222222222222222222222222222222222222", ev: ev}
			w := NewWorker(database.GetDB(), wrong, nil, Config{MainnetEnabled: true, WorkerID: "t", LeaseTTL: time.Minute})
			require.NoError(t, w.ProcessOneForTest(context.Background(), row))
			require.Equal(t, "0x1111111111111111111111111111111111111111", wrong.gotFrom)
			failed, err := database.GetCryptoRefund(row.ID, biz.ID)
			require.NoError(t, err)
			require.Equal(t, database.PaymentRefundStatusFailed, failed.Status)
		})
	}
}

func TestWorker_OldWalletRefundConfirmsAfterRotation(t *testing.T) {
	setupWorkerDB(t)
	biz, row := seedRotatedWalletRefund(t, true)
	right := &fromCheckingOutbound{
		wantFrom: "0x1111111111111111111111111111111111111111",
		ev: blockchain.USDCTransferEvidence{
			From: "0x1111111111111111111111111111111111111111", To: row.VerifiedRecipient,
			AmountBaseUnits: 5_000_000, Confirmations: 1, TxHash: *row.SubmittedTxHash,
			ChainID: 84532, Token: "USDC",
		},
	}
	w := NewWorker(database.GetDB(), right, nil, Config{MainnetEnabled: true, WorkerID: "t", LeaseTTL: time.Minute})
	require.NoError(t, w.ProcessOneForTest(context.Background(), row))
	confirmed, err := database.GetCryptoRefund(row.ID, biz.ID)
	require.NoError(t, err)
	require.Equal(t, database.PaymentRefundStatusConfirmed, confirmed.Status)
}
