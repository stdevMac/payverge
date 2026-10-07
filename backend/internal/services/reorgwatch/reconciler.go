// Package reorgwatch reconciles already-confirmed on-chain crypto records
// (inbound USDC payments and outbound crypto refunds) against the canonical
// chain, so a blockchain reorganization that orphans a record deeper than the
// one-shot confirmation gate does not go undetected.
//
// Scope and safety posture. The one-shot N-confirmation gate at settlement time
// (internal/blockchain) accepts a transfer once it is buried `USDC_MIN_CONFIRM-
// ATIONS` deep and never looks again. A reorg deeper than that depth can orphan
// a payment already credited to a bill, or a refund already reversed in the
// ledger. This reconciler re-reads the receipt for recently-confirmed records
// within a bounded finality window and compares the stored block hash to the
// canonical one.
//
// It deliberately DETECTS and ALERTS rather than silently auto-reversing the
// ledger: a false positive (a flaky RPC transiently reporting a live tx as
// missing) that automatically flipped a paid bill to unpaid, or re-opened a
// reversed refund, would be a larger blast radius than the rare deep reorg it
// guards against. On a suspected orphan it raises an operator-facing operational
// alert (once per episode) and stamps `reorg_suspected_at`; operators remediate
// via the existing audited void/refund paths. Auto-remediation is a deliberate,
// risk-weighted follow-up, not an oversight.
package reorgwatch

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// CanonicalChecker re-reads the current canonical chain state for a tx hash.
// *blockchain.BlockchainService satisfies this via its CanonicalBlock method.
// found=false means the tx is not in the canonical chain (dropped/orphaned); a
// non-nil err means the read itself failed (treated as "unknown", never a
// reorg) so a flaky node cannot masquerade as a reorganization.
type CanonicalChecker interface {
	CanonicalBlock(ctx context.Context, txHash string) (blockHash string, blockNumber uint64, found bool, err error)
}

// RecordKind distinguishes the two financially-material on-chain surfaces.
type RecordKind string

const (
	KindPayment RecordKind = "payment"
	KindRefund  RecordKind = "refund"
)

// ReorgAlert is the payload handed to the Alerter when a confirmed record is
// suspected to have been orphaned by a reorg.
type ReorgAlert struct {
	Kind            RecordKind
	BusinessID      uint
	RecordID        uint // payments.id or payment_refunds.id
	BillID          uint
	PaymentID       uint
	TxHash          string
	StoredBlockHash string
	Detail          string
}

// Alerter is notified when the reconciler suspects a reorg orphaned a confirmed
// record. It is an interface so the reconciler stays out of the events /
// operational_alerts import graph and is unit-testable with a fake. The
// production implementation raises an operational alert (which publishes the
// already-permission-mapped alert.created SSE frame).
type Alerter interface {
	ReorgSuspected(ctx context.Context, ev ReorgAlert)
}

// Config bounds the sweep. Zero values fall back to conservative defaults.
type Config struct {
	// FinalityWindow: only re-check records confirmed within this window. Past
	// it, a reorg is effectively impossible on the target chain, so the record
	// ages out of the sweep. Keeps the working set tiny and bounded.
	FinalityWindow time.Duration
	// Cooldown: minimum interval between re-checks of the same record.
	Cooldown time.Duration
	// SuspicionGrace: how long a record must remain continuously missing (across
	// multiple sweeps) before it escalates from "watching" to an operator alert.
	// This is the guard against a transient NotFound — a load-balanced/lagging/
	// pruned RPC replica can return ethereum.NotFound for a genuinely-canonical
	// tx; requiring the miss to persist across the grace window (several
	// cooldown-spaced re-reads) means one flaky read self-heals silently instead
	// of firing a spurious alert. Must be a small multiple of Cooldown.
	SuspicionGrace time.Duration
	// BatchSize: max records re-checked per table per sweep.
	BatchSize int
}

func (c Config) withDefaults() Config {
	if c.FinalityWindow <= 0 {
		c.FinalityWindow = 30 * time.Minute
	}
	if c.Cooldown <= 0 {
		c.Cooldown = 2 * time.Minute
	}
	if c.SuspicionGrace <= 0 {
		c.SuspicionGrace = 5 * time.Minute
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	return c
}

// Reconciler re-verifies confirmed on-chain records against the canonical chain.
type Reconciler struct {
	db      *gorm.DB
	checker CanonicalChecker
	alerter Alerter
	cfg     Config
	now     func() time.Time
}

// New builds a Reconciler. checker and alerter must be non-nil for Sweep to do
// any work (Sweep is a safe no-op otherwise).
func New(db *gorm.DB, checker CanonicalChecker, alerter Alerter, cfg Config) *Reconciler {
	return &Reconciler{
		db:      db,
		checker: checker,
		alerter: alerter,
		cfg:     cfg.withDefaults(),
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// SweepStats reports what a single sweep did (for logging/observability/tests).
type SweepStats struct {
	Checked        int
	Canonical      int
	ReIncluded     int
	Suspected      int // first observed miss: flagged + watching, NOT yet alerted
	Orphaned       int // missing past the grace window: alerted this sweep
	StillSuspected int // repeat miss still inside the grace window (watching)
	Errors         int // transient checker errors (row left for next sweep)
}

// reorgCandidate is the minimal projection the sweep needs per record.
type reorgCandidate struct {
	ID               uint
	BusinessID       uint
	BillID           uint
	PaymentID        uint
	TxHash           string
	BlockHash        string
	ReorgSuspectedAt *time.Time
}

// Sweep re-verifies both tables once. Safe no-op when unconfigured.
func (r *Reconciler) Sweep(ctx context.Context) (SweepStats, error) {
	var stats SweepStats
	if r == nil || r.db == nil || r.checker == nil {
		return stats, nil
	}
	now := r.now()
	windowStart := now.Add(-r.cfg.FinalityWindow)
	cooldownCutoff := now.Add(-r.cfg.Cooldown)

	payCands, err := r.paymentCandidates(windowStart, cooldownCutoff)
	if err != nil {
		return stats, err
	}
	r.reconcile(ctx, KindPayment, "payments", payCands, now, &stats)

	refCands, err := r.refundCandidates(windowStart, cooldownCutoff)
	if err != nil {
		return stats, err
	}
	r.reconcile(ctx, KindRefund, "payment_refunds", refCands, now, &stats)

	return stats, nil
}

// paymentCandidates selects recently-confirmed crypto payments carrying block
// evidence that are due for a re-check. The join to bills supplies business_id
// (payments has no business_id column). The WHERE mirrors the partial index
// idx_payments_reorg_watch so the scan stays cheap.
func (r *Reconciler) paymentCandidates(windowStart, cooldownCutoff time.Time) ([]reorgCandidate, error) {
	var cands []reorgCandidate
	err := r.db.
		Table("payments AS p").
		Select("p.id AS id, b.business_id AS business_id, p.bill_id AS bill_id, p.id AS payment_id, p.tx_hash AS tx_hash, p.block_hash AS block_hash, p.reorg_suspected_at AS reorg_suspected_at").
		Joins("JOIN bills b ON b.id = p.bill_id").
		Where("p.status = ? AND p.block_hash IS NOT NULL AND p.confirmed_at IS NOT NULL AND p.confirmed_at > ? AND (p.reorg_checked_at IS NULL OR p.reorg_checked_at < ?)",
			database.PaymentStatusConfirmed, windowStart, cooldownCutoff).
		Order("p.id ASC").
		Limit(r.cfg.BatchSize).
		Scan(&cands).Error
	return cands, err
}

// refundCandidates selects recently-confirmed crypto refunds carrying block
// evidence that are due for a re-check. payment_refunds already carries
// business_id/bill_id/payment_id, so no join is needed.
func (r *Reconciler) refundCandidates(windowStart, cooldownCutoff time.Time) ([]reorgCandidate, error) {
	var cands []reorgCandidate
	err := r.db.
		Table("payment_refunds").
		Select("id AS id, business_id AS business_id, bill_id AS bill_id, payment_id AS payment_id, submitted_tx_hash AS tx_hash, block_hash AS block_hash, reorg_suspected_at AS reorg_suspected_at").
		Where("status = ? AND block_hash IS NOT NULL AND submitted_tx_hash IS NOT NULL AND confirmed_at IS NOT NULL AND confirmed_at > ? AND (reorg_checked_at IS NULL OR reorg_checked_at < ?)",
			database.PaymentRefundStatusConfirmed, windowStart, cooldownCutoff).
		Order("id ASC").
		Limit(r.cfg.BatchSize).
		Scan(&cands).Error
	return cands, err
}

func (r *Reconciler) reconcile(ctx context.Context, kind RecordKind, table string, cands []reorgCandidate, now time.Time, stats *SweepStats) {
	for _, c := range cands {
		if err := ctx.Err(); err != nil {
			return
		}
		txHash := strings.TrimSpace(c.TxHash)
		if txHash == "" {
			continue
		}
		stats.Checked++

		hash, number, found, err := r.checker.CanonicalBlock(ctx, txHash)
		if err != nil {
			// Transient/unknown: never treat as a reorg. Leave the row untouched
			// (reorg_checked_at not advanced) so the next sweep retries it.
			stats.Errors++
			continue
		}

		switch {
		case !found:
			// Orphaned: the tx is no longer in the canonical chain.
			r.handleOrphan(ctx, kind, table, c, now, stats)
		case strings.EqualFold(strings.TrimSpace(hash), strings.TrimSpace(c.BlockHash)):
			// Canonical and unchanged: advance the check clock, clear any prior
			// suspicion (it recovered).
			r.updateRow(table, c.ID, map[string]interface{}{
				"reorg_checked_at":   now,
				"reorg_suspected_at": nil,
			})
			stats.Canonical++
		default:
			// Present but re-mined in a different block: the SAME tx (identical
			// effects) is still on-chain, so no financial loss — record the new
			// canonical block and clear suspicion. Not alertable.
			r.updateRow(table, c.ID, map[string]interface{}{
				"block_hash":         hash,
				"block_number":       int64(number),
				"reorg_checked_at":   now,
				"reorg_suspected_at": nil,
			})
			stats.ReIncluded++
		}
	}
}

func (r *Reconciler) handleOrphan(ctx context.Context, kind RecordKind, table string, c reorgCandidate, now time.Time, stats *SweepStats) {
	if c.ReorgSuspectedAt == nil {
		// First observed miss. This is NOT yet an alert: a load-balanced/lagging/
		// pruned RPC replica routinely returns NotFound for a genuinely-canonical
		// tx. Mark the record under suspicion and WATCH it. If the next
		// cooldown-spaced re-read sees it canonical again, suspicion clears with no
		// alert (reconcile's canonical branch), so a transient miss self-heals.
		r.updateRow(table, c.ID, map[string]interface{}{
			"reorg_suspected_at": now,
			"reorg_checked_at":   now,
		})
		stats.Suspected++
		return
	}

	// Repeat miss — advance the check clock regardless.
	r.updateRow(table, c.ID, map[string]interface{}{"reorg_checked_at": now})

	// Escalate to an operator alert only once the record has stayed missing across
	// the grace window (several cooldown-spaced re-reads), which a transient
	// replica lag does not survive. We (re-)raise the alert on EVERY qualifying
	// sweep rather than once: CreateReorgSuspectedAlert is Upsert-idempotent, so
	// this both dedups to a single alert row AND retries an alert whose write
	// previously failed — no orphan signal is ever permanently lost.
	if now.Sub(*c.ReorgSuspectedAt) < r.cfg.SuspicionGrace {
		stats.StillSuspected++
		return
	}
	stats.Orphaned++
	if r.alerter != nil {
		r.alerter.ReorgSuspected(ctx, ReorgAlert{
			Kind:            kind,
			BusinessID:      c.BusinessID,
			RecordID:        c.ID,
			BillID:          c.BillID,
			PaymentID:       c.PaymentID,
			TxHash:          c.TxHash,
			StoredBlockHash: c.BlockHash,
			Detail:          "confirmed on-chain " + string(kind) + " transaction has been absent from the canonical chain across the reorg grace window (suspected reorg); verify settlement before treating funds as final",
		})
	}
}

func (r *Reconciler) updateRow(table string, id uint, updates map[string]interface{}) {
	if err := r.db.Table(table).Where("id = ?", id).Updates(updates).Error; err != nil {
		logger.Logger.Warnf("reorgwatch: update %s id=%d: %v", table, id, err)
	}
}

// Run drives Sweep on a ticker until ctx is cancelled. A no-op (returns
// immediately) when the reconciler is unconfigured, so main.go can start it
// unconditionally and let a missing RPC verifier disable it safely.
func (r *Reconciler) Run(ctx context.Context, interval time.Duration) {
	if r == nil || r.db == nil || r.checker == nil {
		logger.Logger.Infof("reorgwatch: disabled (no blockchain verifier)")
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	logger.Logger.Infof("reorgwatch: started interval=%s finality_window=%s cooldown=%s grace=%s batch=%d",
		interval, r.cfg.FinalityWindow, r.cfg.Cooldown, r.cfg.SuspicionGrace, r.cfg.BatchSize)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Logger.Infof("reorgwatch: stopped")
			return
		case <-ticker.C:
			r.safeSweep(ctx)
		}
	}
}

// safeSweep runs one Sweep under SafeTick: a panic on one bad row is logged
// and the ticker loop keeps running instead of crashing the process.
func (r *Reconciler) safeSweep(ctx context.Context) {
	logger.SafeTick("reorgwatch-sweep", func() {
		stats, err := r.Sweep(ctx)
		if err != nil {
			if ctx.Err() == nil {
				logger.Logger.Warnf("reorgwatch: sweep failed: %v", err)
			}
			return
		}
		if stats.Orphaned > 0 || stats.Suspected > 0 || stats.ReIncluded > 0 || stats.Errors > 0 {
			logger.Logger.Warnf("reorgwatch: sweep checked=%d canonical=%d reincluded=%d suspected=%d orphaned=%d still_suspected=%d errors=%d",
				stats.Checked, stats.Canonical, stats.ReIncluded, stats.Suspected, stats.Orphaned, stats.StillSuspected, stats.Errors)
		}
	})
}
