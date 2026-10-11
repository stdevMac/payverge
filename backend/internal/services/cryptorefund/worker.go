// Package cryptorefund implements the noncustodial on-chain refund
// confirmation worker (Wave 4 Task 13).
//
// The backend NEVER custodies a treasury private key. Owners sign+submit via
// their connected settlement wallet/Safe, or submit a tx hash from external
// tooling. This worker only:
//  1. leases submitted/confirming payment_refunds rows
//  2. verifies an outbound USDC transfer FROM the wallet that received the
//     payment (stored quote/bill wallet) TO the verified recipient for the
//     exact base-unit amount
//  3. marks confirmed only after min confirmations
//  4. applies RefundBillPayment ledger + fiscal credit note EXACTLY ONCE
//
// Mainnet submission is gated off by default (CRYPTO_REFUND_MAINNET_ENABLED).
package cryptorefund

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/blockchain"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"gorm.io/gorm"
)

// OutboundVerifier is the read-only chain primitive the worker needs.
type OutboundVerifier interface {
	VerifyOutboundUSDCTransfer(ctx context.Context, txHash string, from string, to string, expectedAmountMicrounits int64) (blockchain.USDCTransferEvidence, error)
	MinConfirmations() uint64
}

// CreditNoteEnqueuer is satisfied by handlers.enqueueFiscalCreditNoteForRefund
// without importing the handlers package (avoids import cycles).
type CreditNoteEnqueuer func(bill *database.Bill, amountCents int64, discriminator, actor string)

// Config controls the confirmation worker.
type Config struct {
	Interval    time.Duration
	Concurrency int
	LeaseTTL    time.Duration
	// MainnetEnabled gates whether mainnet refunds are processed. When false
	// (default), mainnet submitted rows are left untouched so operators use
	// the manual workflow without the worker promising instant refunds.
	MainnetEnabled bool
	WorkerID       string
}

// DefaultConfig reads env with safe defaults. Mainnet is OFF by default.
func DefaultConfig() Config {
	cfg := Config{
		Interval:       15 * time.Second,
		Concurrency:    2,
		LeaseTTL:       2 * time.Minute,
		MainnetEnabled: false,
		WorkerID:       "crypto-refund-worker",
	}
	if v := strings.TrimSpace(os.Getenv("CRYPTO_REFUND_WORKER_INTERVAL_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Interval = time.Duration(n) * time.Second
		}
	}
	if v := strings.TrimSpace(os.Getenv("CRYPTO_REFUND_WORKER_CONCURRENCY")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Concurrency = n
		}
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("CRYPTO_REFUND_MAINNET_ENABLED"))); v == "1" || v == "true" || v == "yes" {
		cfg.MainnetEnabled = true
	}
	return cfg
}

// Worker leases and confirms noncustodial crypto refunds.
type Worker struct {
	db       *gorm.DB
	verifier OutboundVerifier
	credit   CreditNoteEnqueuer
	cfg      Config
}

// NewWorker constructs a Worker. verifier may be nil (worker no-ops process).
func NewWorker(db *gorm.DB, verifier OutboundVerifier, credit CreditNoteEnqueuer, cfg Config) *Worker {
	if cfg.Interval <= 0 {
		cfg.Interval = 15 * time.Second
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 2 * time.Minute
	}
	if strings.TrimSpace(cfg.WorkerID) == "" {
		cfg.WorkerID = "crypto-refund-worker"
	}
	return &Worker{db: db, verifier: verifier, credit: credit, cfg: cfg}
}

// Run blocks until ctx is cancelled, polling for claimable refunds.
func (w *Worker) Run(ctx context.Context) {
	if w == nil {
		return
	}
	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()
	logger.Logger.Infof("crypto refund worker started interval=%s concurrency=%d mainnet=%v",
		w.cfg.Interval, w.cfg.Concurrency, w.cfg.MainnetEnabled)

	w.safeTick(ctx)
	for {
		select {
		case <-ctx.Done():
			logger.Logger.Infof("crypto refund worker stopped")
			return
		case <-ticker.C:
			w.safeTick(ctx)
		}
	}
}

// safeTick runs one poll under SafeTick so a panic on one bad row is logged
// and the loop keeps running instead of crashing the process.
func (w *Worker) safeTick(ctx context.Context) {
	logger.SafeTick("crypto-refund-worker", func() { w.tick(ctx) })
}

func (w *Worker) tick(ctx context.Context) {
	if w.verifier == nil || w.db == nil {
		return
	}
	claimed, err := database.ClaimDueCryptoRefunds(w.db, w.cfg.WorkerID, w.cfg.Concurrency*2, time.Now().UTC(), w.cfg.LeaseTTL)
	if err != nil {
		logger.Logger.Warnf("crypto refund claim failed: %v", err)
		return
	}
	if len(claimed) == 0 {
		return
	}

	sem := make(chan struct{}, w.cfg.Concurrency)
	var wg sync.WaitGroup
	for i := range claimed {
		row := claimed[i]
		// Skip mainnet when disabled — leave in submitted/confirming for manual ops.
		if database.IsMainnetCryptoRefundChain(row.ChainID) && !w.cfg.MainnetEnabled {
			if err := database.MarkCryptoRefundConfirming(row.ID, row.Confirmations, "mainnet confirmation deferred (CRYPTO_REFUND_MAINNET_ENABLED=false)"); err != nil {
				logger.Logger.Warnf("crypto refund %d: mark confirming (mainnet deferred): %v", row.ID, err)
			}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		r := row
		logger.SafeGoNamed("crypto refund confirm", func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := w.processOne(ctx, r); err != nil {
				logger.Logger.Warnf("crypto refund %d process: %v", r.ID, err)
			}
		})
	}
	wg.Wait()
}

func (w *Worker) processOne(ctx context.Context, row database.PaymentRefund) error {
	if row.SubmittedTxHash == nil || strings.TrimSpace(*row.SubmittedTxHash) == "" {
		return database.MarkCryptoRefundFailed(row.ID, "missing submitted_tx_hash")
	}
	txHash := strings.TrimSpace(*row.SubmittedTxHash)

	// The refund must leave the wallet that received the payment, as stored at
	// payment time — not the business's live wallet, which may have rotated.
	from, err := database.CryptoRefundSourceWallet(w.db, row.PaymentID)
	if err != nil {
		if errors.Is(err, database.ErrCryptoRefundSourceWalletUnknown) {
			return database.MarkCryptoRefundFailed(row.ID, "inbound settlement wallet for payment is unknown")
		}
		// Transient read failure: stay confirming and retry on a later tick.
		w.markConfirming(row.ID, row.Confirmations, "source wallet lookup failed: "+err.Error())
		return err
	}

	ev, err := w.verifier.VerifyOutboundUSDCTransfer(ctx, txHash, from, row.VerifiedRecipient, row.AmountBaseUnits)
	if err != nil {
		if errors.Is(err, blockchain.ErrAwaitingConfirmations) {
			// Stay in confirming; re-lease later. NEVER call this "refunded".
			return database.MarkCryptoRefundConfirming(row.ID, int(ev.Confirmations), err.Error())
		}
		// Hard failure: wrong from/to/amount/token — leave ledger untouched.
		return database.MarkCryptoRefundFailed(row.ID, err.Error())
	}

	// Defense-in-depth: the verified transfer must be on the refund row's chain
	// and token. Latent today (single dialed RPC), but rejects a mismatched
	// chain/token as required by the "reject wrong chain" contract even if the
	// verifier later becomes multi-chain.
	if ev.ChainID != int64(row.ChainID) || !strings.EqualFold(ev.Token, row.Token) {
		return database.MarkCryptoRefundFailed(row.ID,
			fmt.Sprintf("verified transfer chain/token mismatch: got chain=%d token=%s want chain=%d token=%s",
				ev.ChainID, ev.Token, row.ChainID, row.Token))
	}

	confs := int(ev.Confirmations)
	if confs <= 0 {
		confs = int(w.verifier.MinConfirmations())
	}

	bill, payment, refund, err := database.ConfirmCryptoRefundAndApplyLedgerWithHook(
		row.ID,
		confs,
		"crypto_refund_worker",
		func(tx *gorm.DB, bill *database.Bill, payment *database.Payment, refund *database.PaymentRefund) error {
			return fiscal.EnqueueRefundCreditNoteInTx(
				tx,
				bill,
				payment,
				fmt.Sprintf("payment:%d", payment.ID),
				"crypto_refund_worker",
			)
		},
	)
	if err != nil {
		// Chain succeeded but DB failed — reconciliation on next tick must
		// NOT re-send (idempotent on submitted_tx_hash → confirmed). Leave in
		// confirming so we retry ledger application only.
		w.markConfirming(row.ID, confs, "ledger apply pending after chain success: "+err.Error())
		return err
	}

	// Persist the block this refund settled in so the reorg reconciler can later
	// re-read the canonical chain. Best-effort + idempotent: the
	// money is already reversed above; a missing marker only means this refund
	// won't be reorg-swept, never a double-spend. Stamp after confirm so only
	// truly-confirmed refunds become sweep candidates.
	if refund != nil && ev.BlockHash != "" {
		if err := database.RecordCryptoRefundBlockEvidence(row.ID, int64(ev.BlockNumber), ev.BlockHash, time.Now().UTC()); err != nil {
			logger.Logger.Warnf("Crypto refund %d: record block evidence: %v", row.ID, err)
		}
	}

	// Compatibility belt-and-braces hook for the existing IMP-15 seam. The
	// durable fiscal job has already committed with the ledger above; both paths
	// use the same payment discriminator, so this post-commit call is a no-op at
	// the fiscal idempotency key rather than a second credit note.
	if w.credit != nil && bill != nil && payment != nil && refund != nil && refund.LedgerAppliedAt != nil {
		w.credit(bill, payment.Amount, fmt.Sprintf("payment:%d", payment.ID), "crypto_refund_worker")
	}

	if bill != nil {
		events.GetHub().PublishJSON(row.BusinessID, "crypto_refund.confirmed", ginH{
			"refund_id":  row.ID,
			"bill_id":    row.BillID,
			"payment_id": row.PaymentID,
			"status":     "confirmed",
			"tx_hash":    txHash,
		})
		events.GetHub().PublishJSON(row.BusinessID, "bill.updated", ginH{
			"bill_id": row.BillID,
			"status":  string(bill.Status),
		})
	}
	return nil
}

// markConfirming records a retryable state; a failed write is logged, never
// swallowed, so a stuck lease is visible in the logs.
func (w *Worker) markConfirming(refundID uint, confirmations int, lastErr string) {
	if err := database.MarkCryptoRefundConfirming(refundID, confirmations, lastErr); err != nil {
		logger.Logger.Warnf("crypto refund %d: mark confirming: %v", refundID, err)
	}
}

// ginH avoids importing gin in the service package.
type ginH map[string]any

// ProcessOneForTest exposes processOne for unit tests.
func (w *Worker) ProcessOneForTest(ctx context.Context, row database.PaymentRefund) error {
	return w.processOne(ctx, row)
}
