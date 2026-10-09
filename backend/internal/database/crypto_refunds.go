package database

// Wave 4 REFUND track: durable noncustodial crypto refund repository.
//
// Money contract: amount_base_units are int64 USDC micro-units (6 decimals).
// NEVER float. The backend never custodies a treasury private key — this
// package only persists request state and verifies transitions.

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/txhash"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// ErrCryptoRefundNotFound is returned when a payment_refunds row is missing.
	ErrCryptoRefundNotFound = errors.New("crypto refund not found")
	// ErrCryptoRefundNotRefundable covers missing destination evidence, non-
	// confirmed payment, wrong method, or exhausted refundable balance.
	ErrCryptoRefundNotRefundable = errors.New("crypto payment is not refundable on-chain")
	// ErrCryptoRefundActiveExists is returned when a non-terminal refund already
	// exists for the payment (one-active-per-payment invariant).
	ErrCryptoRefundActiveExists = errors.New("an active crypto refund already exists for this payment")
	// ErrCryptoRefundInvalidTransition is a guarded state-machine violation.
	ErrCryptoRefundInvalidTransition = errors.New("invalid crypto refund status transition")
	// ErrCryptoRefundAmountExceedsBalance is amount > remaining refundable base units.
	ErrCryptoRefundAmountExceedsBalance = errors.New("refund amount exceeds remaining refundable balance")
	// ErrCryptoRefundMustBeFullAmount rejects a partial crypto refund. The ledger
	// reversal (refundBillPaymentTx) reverses the ENTIRE payment and closes it, so
	// a partial on-chain send would over-reverse the books vs. what was actually
	// refunded on-chain. Partial crypto refunds require a proportional ledger
	// reversal that is intentionally deferred; until then the requested amount must
	// equal the full remaining refundable balance.
	ErrCryptoRefundMustBeFullAmount = errors.New("partial crypto refunds are not supported; request the full remaining refundable amount")
	// ErrCryptoRefundIdempotencyConflict is a reused idempotency key with different payload.
	ErrCryptoRefundIdempotencyConflict = errors.New("crypto refund idempotency key conflict")
	// ErrCryptoRefundTxHashReused is a submitted_tx_hash already bound to another refund.
	ErrCryptoRefundTxHashReused = errors.New("refund transaction hash already used")
	// ErrCryptoRefundOverrideRequiresReason is an address override without dual-approval reason.
	ErrCryptoRefundOverrideRequiresReason = errors.New("recipient override requires an immutable audit reason")
	// ErrCryptoRefundDestinationMissing means Task 11 evidence is absent — UI
	// must say refund requires manual support, not invent a destination.
	ErrCryptoRefundDestinationMissing = errors.New("verified refund destination evidence is unavailable")
	// ErrCryptoRefundChainNotAllowlisted rejects unsupported chain/token pairs.
	ErrCryptoRefundChainNotAllowlisted = errors.New("chain/token is not allowlisted for crypto refunds")
)

// cryptoRefundTerminalStatuses free the one-active-per-payment slot.
var cryptoRefundTerminalStatuses = []PaymentRefundStatus{
	PaymentRefundStatusConfirmed,
	PaymentRefundStatusFailed,
	PaymentRefundStatusRejected,
	PaymentRefundStatusCancelled,
}

// cryptoRefundClaimableStatuses are leased by the confirmation worker.
var cryptoRefundClaimableStatuses = []PaymentRefundStatus{
	PaymentRefundStatusSubmitted,
	PaymentRefundStatusConfirming,
}

// IsCryptoRefundChainAllowlisted reports whether chainID/token may be refunded
// via the noncustodial flow. Mainnet (8453, 1) is allowlisted for *verification*
// but mainnet *submission* is gated separately by CRYPTO_REFUND_MAINNET_ENABLED.
// The testnets (Base Sepolia 84532, Sepolia 11155111) are allowlisted only
// outside production, matching guest settlement intake: production never
// accepts free-to-mint testnet USDC, so it never refunds against it either.
func IsCryptoRefundChainAllowlisted(chainID int, token string) bool {
	token = strings.ToUpper(strings.TrimSpace(token))
	if token != "USDC" {
		return false
	}
	switch chainID {
	case 8453, 1:
		return true
	case 84532, 11155111:
		return !config.IsProductionMode(false)
	default:
		return false
	}
}

// IsMainnetCryptoRefundChain reports Base mainnet or Ethereum mainnet.
func IsMainnetCryptoRefundChain(chainID int) bool {
	return chainID == 8453 || chainID == 1
}

// MaskRefundAddress returns a short operator-safe form (0x1234…abcd). Empty
// input yields empty string so the UI can render "manual support required".
func MaskRefundAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if len(addr) < 10 {
		return addr
	}
	return addr[:6] + "…" + addr[len(addr)-4:]
}

// persistPaymentRefundDestinationTx is the transaction-aware core used by
// payment settlement. Evidence is append-once: an exact retry is a no-op, but
// a different destination or proof for the same payment is rejected instead
// of being silently accepted behind the unique payment_id constraint.
func persistPaymentRefundDestinationTx(tx *gorm.DB, dest *PaymentRefundDestination) error {
	if dest == nil {
		return fmt.Errorf("refund destination is required")
	}
	if tx == nil {
		return fmt.Errorf("database transaction is required")
	}
	if dest.PaymentID == 0 {
		return fmt.Errorf("payment_id is required")
	}
	if dest.AmountBaseUnits <= 0 {
		return fmt.Errorf("amount_base_units must be positive integer base units")
	}
	addr := strings.TrimSpace(dest.RefundAddress)
	if !isValidEthereumAddress(addr) {
		return fmt.Errorf("malformed refund address")
	}
	// Never store guest ledger sentinels as refund evidence.
	lower := strings.ToLower(addr)
	if lower == "crypto_guest" || lower == "cross_chain_guest" || lower == "plugin" {
		return fmt.Errorf("refund destination must not be a ledger sentinel")
	}
	if dest.EvidenceType != RefundEvidenceTransferLog && dest.EvidenceType != RefundEvidenceWalletSignature {
		return fmt.Errorf("invalid evidence_type")
	}
	if dest.VerifiedAt.IsZero() {
		dest.VerifiedAt = time.Now().UTC()
	}
	dest.RefundAddress = commonChecksumOrRaw(addr)
	dest.Token = strings.ToUpper(strings.TrimSpace(dest.Token))
	if dest.Token == "" {
		dest.Token = "USDC"
	}

	var existing PaymentRefundDestination
	err := tx.Where("payment_id = ?", dest.PaymentID).First(&existing).Error
	if err == nil {
		if refundDestinationEvidenceMatches(&existing, dest) {
			return nil
		}
		return fmt.Errorf("refund destination conflicts with existing evidence for payment %d", dest.PaymentID)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check existing refund destination: %w", err)
	}

	result := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "payment_id"}},
		DoNothing: true,
	}).Create(dest)
	if result.Error != nil {
		err = result.Error
		return fmt.Errorf("failed to persist refund destination: %w", err)
	}
	if result.RowsAffected == 0 {
		// A concurrent writer won. ON CONFLICT keeps the transaction usable, so
		// re-read and only accept the exact same immutable proof.
		if err := tx.Where("payment_id = ?", dest.PaymentID).First(&existing).Error; err != nil {
			return fmt.Errorf("failed to load concurrent refund destination: %w", err)
		}
		if !refundDestinationEvidenceMatches(&existing, dest) {
			return fmt.Errorf("refund destination conflicts with existing evidence for payment %d", dest.PaymentID)
		}
	}
	return nil
}

func refundDestinationEvidenceMatches(a, b *PaymentRefundDestination) bool {
	if a == nil || b == nil {
		return false
	}
	return a.PaymentID == b.PaymentID &&
		a.ChainID == b.ChainID &&
		strings.EqualFold(strings.TrimSpace(a.Token), strings.TrimSpace(b.Token)) &&
		a.AmountBaseUnits == b.AmountBaseUnits &&
		strings.EqualFold(strings.TrimSpace(a.RefundAddress), strings.TrimSpace(b.RefundAddress)) &&
		a.EvidenceType == b.EvidenceType &&
		equalOptionalString(a.SignatureRef, b.SignatureRef) &&
		equalOptionalString(a.LogRef, b.LogRef)
}

func equalOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return strings.TrimSpace(*a) == strings.TrimSpace(*b)
}

// GetPaymentRefundDestination loads destination evidence for a payment.
// Returns ErrCryptoRefundDestinationMissing when none exists.
func GetPaymentRefundDestination(paymentID uint) (*PaymentRefundDestination, error) {
	var dest PaymentRefundDestination
	if err := db.Where("payment_id = ?", paymentID).First(&dest).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCryptoRefundDestinationMissing
		}
		return nil, fmt.Errorf("failed to load refund destination: %w", err)
	}
	return &dest, nil
}

// ErrCryptoRefundSourceWalletUnknown means no stored inbound recipient exists
// for the payment, so the wallet a refund must leave from cannot be proven.
var ErrCryptoRefundSourceWalletUnknown = errors.New("inbound settlement wallet for payment is unknown")

// isCryptoSettlementMethod reports whether a payment method settles on chain
// into the bill's settlement wallet (and so can be refunded on chain).
func isCryptoSettlementMethod(method string) bool {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "crypto", "cross-chain", "cross_chain":
		return true
	}
	return false
}

// CryptoRefundSourceWallet returns the wallet that received the inbound
// payment, which is the only wallet a refund for it may be sent from. It reads
// what was stored at payment time — the consumed quote's settlement_address,
// else the payment's own settlement_addr — and never the bill's or the live
// business wallet, so a refund for a payment taken before a wallet rotation verifies
// against the old wallet.
func CryptoRefundSourceWallet(gormDB *gorm.DB, paymentID uint) (string, error) {
	if gormDB == nil {
		gormDB = db
	}
	var quoteWallets []string
	if err := gormDB.Model(&CryptoPaymentQuote{}).
		Where("payment_id = ? AND settlement_address <> ''", paymentID).
		Order("id DESC").
		Limit(1).
		Pluck("settlement_address", &quoteWallets).Error; err != nil {
		return "", fmt.Errorf("failed to load payment quote wallet: %w", err)
	}
	if len(quoteWallets) == 1 && strings.TrimSpace(quoteWallets[0]) != "" {
		return strings.TrimSpace(quoteWallets[0]), nil
	}
	// The wallet recorded on the payment when it confirmed. The bill's
	// settlement_addr is not a fallback: wallet rotation rewrites it on open
	// checks, so it may name a wallet that never received these funds.
	var paymentWallets []string
	if err := gormDB.Model(&Payment{}).
		Where("id = ? AND settlement_addr IS NOT NULL AND settlement_addr <> ''", paymentID).
		Limit(1).
		Pluck("settlement_addr", &paymentWallets).Error; err != nil {
		return "", fmt.Errorf("failed to load payment settlement wallet: %w", err)
	}
	if len(paymentWallets) == 1 && strings.TrimSpace(paymentWallets[0]) != "" {
		return strings.TrimSpace(paymentWallets[0]), nil
	}
	return "", ErrCryptoRefundSourceWalletUnknown
}

// RefundableCryptoBalanceBaseUnits returns remaining refundable base units for
// a payment: destination amount minus sum of confirmed refunds.
func RefundableCryptoBalanceBaseUnits(paymentID uint) (int64, error) {
	dest, err := GetPaymentRefundDestination(paymentID)
	if err != nil {
		return 0, err
	}
	var refunded int64
	if err := db.Model(&PaymentRefund{}).
		Where("payment_id = ? AND status = ?", paymentID, PaymentRefundStatusConfirmed).
		Select("COALESCE(SUM(amount_base_units), 0)").
		Scan(&refunded).Error; err != nil {
		return 0, fmt.Errorf("failed to sum confirmed refunds: %w", err)
	}
	remaining := dest.AmountBaseUnits - refunded
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}

// CreateCryptoRefundInput is the validated request payload for RequestCryptoRefund.
type CreateCryptoRefundInput struct {
	BusinessID      uint
	BillID          uint
	PaymentID       uint
	AmountBaseUnits int64
	Reason          string
	RequestedBy     string
	IdempotencyKey  string
	// Optional override — requires OverrideReason and dual approval path.
	RecipientOverride *string
	OverrideReason    string
}

// RequestCryptoRefund creates a requested-status refund after validating the
// original confirmed on-chain payment, destination evidence, allowlist, and
// remaining balance. Enforces one active refund per payment.
func RequestCryptoRefund(input CreateCryptoRefundInput) (*PaymentRefund, error) {
	reason := strings.TrimSpace(input.Reason)
	if len(reason) < 3 {
		return nil, fmt.Errorf("refund reason is required (min 3 characters)")
	}
	idem := strings.TrimSpace(input.IdempotencyKey)
	if idem == "" {
		return nil, fmt.Errorf("idempotency_key is required")
	}
	if input.AmountBaseUnits <= 0 {
		return nil, fmt.Errorf("amount_base_units must be a positive integer")
	}
	requestedBy := strings.TrimSpace(input.RequestedBy)
	if requestedBy == "" {
		return nil, fmt.Errorf("requested_by is required")
	}

	var result *PaymentRefund
	err := db.Transaction(func(tx *gorm.DB) error {
		// Idempotency replay.
		var existing PaymentRefund
		if err := tx.Where("business_id = ? AND idempotency_key = ?", input.BusinessID, idem).
			First(&existing).Error; err == nil {
			if existing.PaymentID != input.PaymentID || existing.AmountBaseUnits != input.AmountBaseUnits {
				return ErrCryptoRefundIdempotencyConflict
			}
			result = &existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to check refund idempotency: %w", err)
		}

		var payment Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&payment, input.PaymentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("payment not found")
			}
			return fmt.Errorf("failed to lock payment: %w", err)
		}
		if payment.BillID != input.BillID {
			return fmt.Errorf("payment does not belong to this bill")
		}
		if payment.Status != PaymentStatusConfirmed {
			return ErrCryptoRefundNotRefundable
		}
		if !isCryptoSettlementMethod(payment.PaymentMethod) {
			return ErrCryptoRefundNotRefundable
		}

		var bill Bill
		if err := tx.Select("id", "business_id").First(&bill, payment.BillID).Error; err != nil {
			return fmt.Errorf("failed to load bill: %w", err)
		}
		if bill.BusinessID != input.BusinessID {
			return fmt.Errorf("bill does not belong to this business")
		}

		var dest PaymentRefundDestination
		if err := tx.Where("payment_id = ?", payment.ID).First(&dest).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCryptoRefundDestinationMissing
			}
			return fmt.Errorf("failed to load refund destination: %w", err)
		}
		if !IsCryptoRefundChainAllowlisted(dest.ChainID, dest.Token) {
			return ErrCryptoRefundChainNotAllowlisted
		}

		// Remaining balance = destination − confirmed refunds.
		var refunded int64
		if err := tx.Model(&PaymentRefund{}).
			Where("payment_id = ? AND status = ?", payment.ID, PaymentRefundStatusConfirmed).
			Select("COALESCE(SUM(amount_base_units), 0)").
			Scan(&refunded).Error; err != nil {
			return fmt.Errorf("failed to sum confirmed refunds: %w", err)
		}
		remaining := dest.AmountBaseUnits - refunded
		if remaining <= 0 {
			return ErrCryptoRefundNotRefundable
		}
		if input.AmountBaseUnits > remaining {
			return ErrCryptoRefundAmountExceedsBalance
		}
		// Full-only: the ledger reversal reverses the entire payment, so the
		// on-chain refund MUST equal the full remaining refundable balance.
		// Partial (proportional) reversal is a deferred follow-up.
		if input.AmountBaseUnits != remaining {
			return ErrCryptoRefundMustBeFullAmount
		}

		// Active refund guard (SQLite-friendly; Postgres also has partial unique).
		var activeCount int64
		if err := tx.Model(&PaymentRefund{}).
			Where("payment_id = ? AND status NOT IN ?", payment.ID, cryptoRefundTerminalStatuses).
			Count(&activeCount).Error; err != nil {
			return fmt.Errorf("failed to count active refunds: %w", err)
		}
		if activeCount > 0 {
			return ErrCryptoRefundActiveExists
		}

		recipient := dest.RefundAddress
		override := false
		var overrideReason *string
		if input.RecipientOverride != nil {
			overrideAddr := strings.TrimSpace(*input.RecipientOverride)
			if overrideAddr != "" && !strings.EqualFold(overrideAddr, dest.RefundAddress) {
				if strings.TrimSpace(input.OverrideReason) == "" {
					return ErrCryptoRefundOverrideRequiresReason
				}
				if !isValidEthereumAddress(overrideAddr) {
					return fmt.Errorf("malformed override refund address")
				}
				recipient = commonChecksumOrRaw(overrideAddr)
				override = true
				r := strings.TrimSpace(input.OverrideReason)
				overrideReason = &r
			}
		}

		now := time.Now().UTC()
		row := &PaymentRefund{
			BusinessID:        input.BusinessID,
			BillID:            input.BillID,
			PaymentID:         payment.ID,
			ChainID:           dest.ChainID,
			Token:             dest.Token,
			AmountBaseUnits:   input.AmountBaseUnits,
			VerifiedRecipient: recipient,
			RecipientOverride: override,
			OverrideReason:    overrideReason,
			Reason:            reason,
			RequestedBy:       requestedBy,
			Status:            PaymentRefundStatusRequested,
			IdempotencyKey:    idem,
			RequestedAt:       now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := tx.Create(row).Error; err != nil {
			if isUniqueConstraintError(err) {
				return ErrCryptoRefundActiveExists
			}
			return fmt.Errorf("failed to create crypto refund: %w", err)
		}
		result = row
		return nil
	})
	return result, err
}

// ApproveCryptoRefund moves requested → approved → awaiting_signature.
// Address overrides require the approver to be different from the requester
// (dual approval).
func ApproveCryptoRefund(refundID uint, businessID uint, approver string) (*PaymentRefund, error) {
	approver = strings.TrimSpace(approver)
	if approver == "" {
		return nil, fmt.Errorf("approver is required")
	}
	var result *PaymentRefund
	err := db.Transaction(func(tx *gorm.DB) error {
		var row PaymentRefund
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND business_id = ?", refundID, businessID).
			First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCryptoRefundNotFound
			}
			return err
		}
		if row.Status != PaymentRefundStatusRequested {
			return ErrCryptoRefundInvalidTransition
		}
		if row.RecipientOverride && strings.EqualFold(row.RequestedBy, approver) {
			return fmt.Errorf("recipient override requires a second distinct approver")
		}
		now := time.Now().UTC()
		updates := map[string]interface{}{
			"status":      PaymentRefundStatusAwaitingSignature,
			"approved_by": approver,
			"approved_at": now,
			"updated_at":  now,
		}
		if err := tx.Model(&PaymentRefund{}).Where("id = ? AND status = ?", row.ID, PaymentRefundStatusRequested).
			Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&row, row.ID).Error; err != nil {
			return err
		}
		result = &row
		return nil
	})
	return result, err
}

// RejectCryptoRefund moves requested → rejected. Leaves the payment ledger untouched.
func RejectCryptoRefund(refundID uint, businessID uint, rejector, reason string) (*PaymentRefund, error) {
	rejector = strings.TrimSpace(rejector)
	if rejector == "" {
		return nil, fmt.Errorf("rejector is required")
	}
	var result *PaymentRefund
	err := db.Transaction(func(tx *gorm.DB) error {
		var row PaymentRefund
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND business_id = ?", refundID, businessID).
			First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCryptoRefundNotFound
			}
			return err
		}
		if row.Status != PaymentRefundStatusRequested && row.Status != PaymentRefundStatusApproved && row.Status != PaymentRefundStatusAwaitingSignature {
			return ErrCryptoRefundInvalidTransition
		}
		now := time.Now().UTC()
		errMsg := strings.TrimSpace(reason)
		updates := map[string]interface{}{
			"status":      PaymentRefundStatusRejected,
			"rejected_by": rejector,
			"rejected_at": now,
			"updated_at":  now,
		}
		if errMsg != "" {
			updates["last_error"] = errMsg
		}
		if err := tx.Model(&PaymentRefund{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&row, row.ID).Error; err != nil {
			return err
		}
		result = &row
		return nil
	})
	return result, err
}

// SubmitCryptoRefundTxHash binds a unique on-chain tx hash after the owner
// signed externally (or via Safe). Transitions awaiting_signature|approved → submitted.
func SubmitCryptoRefundTxHash(refundID uint, businessID uint, txHash string) (*PaymentRefund, error) {
	canonical, ok := txhash.Canonical(txHash)
	if !ok {
		return nil, fmt.Errorf("submitted_tx_hash must be 0x followed by 64 hex characters")
	}
	txHash = canonical
	var result *PaymentRefund
	err := db.Transaction(func(tx *gorm.DB) error {
		var row PaymentRefund
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND business_id = ?", refundID, businessID).
			First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCryptoRefundNotFound
			}
			return err
		}
		switch row.Status {
		case PaymentRefundStatusAwaitingSignature, PaymentRefundStatusApproved, PaymentRefundStatusFailed:
			// allow re-submit after failed verification
		default:
			if row.Status == PaymentRefundStatusSubmitted || row.Status == PaymentRefundStatusConfirming {
				if row.SubmittedTxHash != nil && strings.EqualFold(*row.SubmittedTxHash, txHash) {
					result = &row
					return nil
				}
			}
			return ErrCryptoRefundInvalidTransition
		}

		// Uniqueness of submitted_tx_hash across refunds.
		var clash PaymentRefund
		if err := tx.Where("submitted_tx_hash = ? AND id <> ?", txHash, row.ID).First(&clash).Error; err == nil {
			return ErrCryptoRefundTxHashReused
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		now := time.Now().UTC()
		updates := map[string]interface{}{
			"status":            PaymentRefundStatusSubmitted,
			"submitted_tx_hash": txHash,
			"submitted_at":      now,
			"last_error":        nil,
			"updated_at":        now,
		}
		if err := tx.Model(&PaymentRefund{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			if isUniqueConstraintError(err) {
				return ErrCryptoRefundTxHashReused
			}
			return err
		}
		if err := tx.First(&row, row.ID).Error; err != nil {
			return err
		}
		result = &row
		return nil
	})
	return result, err
}

// GetCryptoRefund loads a business-scoped refund.
func GetCryptoRefund(refundID, businessID uint) (*PaymentRefund, error) {
	var row PaymentRefund
	if err := db.Where("id = ? AND business_id = ?", refundID, businessID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCryptoRefundNotFound
		}
		return nil, err
	}
	return &row, nil
}

// ListCryptoRefundsForBusiness returns recent refunds for a business.
func ListCryptoRefundsForBusiness(businessID uint, limit int) ([]PaymentRefund, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []PaymentRefund
	if err := db.Where("business_id = ?", businessID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListCryptoRefundsForBusinessPayment returns EVERY refund for one payment
// scoped to a business (tenant-safe: the business_id predicate keeps a caller
// from reading another tenant's payment refunds). Unbounded by payment — a
// single payment has at most a handful of refunds, and the operator panel needs
// them all (the old business-wide 100-cap could silently drop older refunds for
// a specific payment).
func ListCryptoRefundsForBusinessPayment(businessID, paymentID uint) ([]PaymentRefund, error) {
	var rows []PaymentRefund
	if err := db.Where("business_id = ? AND payment_id = ?", businessID, paymentID).
		Order("created_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ClaimDueCryptoRefunds leases submitted/confirming rows for the worker.
func ClaimDueCryptoRefunds(gormDB *gorm.DB, workerID string, limit int, now time.Time, leaseTTL time.Duration) ([]PaymentRefund, error) {
	if gormDB == nil {
		gormDB = db
	}
	if limit <= 0 {
		return nil, nil
	}
	if leaseTTL <= 0 {
		leaseTTL = 2 * time.Minute
	}
	leaseCutoff := now.Add(-leaseTTL)

	var claimed []PaymentRefund
	err := gormDB.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&PaymentRefund{}).
			Where(
				"status IN ? AND (locked_at IS NULL OR locked_at < ?)",
				cryptoRefundClaimableStatuses,
				leaseCutoff,
			).
			Order("updated_at ASC, id ASC").
			Limit(limit)

		if tx.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}

		var ids []uint
		if err := query.Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}

		result := tx.Model(&PaymentRefund{}).
			Where("id IN ? AND status IN ? AND (locked_at IS NULL OR locked_at < ?)", ids, cryptoRefundClaimableStatuses, leaseCutoff).
			Updates(map[string]interface{}{
				"locked_at":  now,
				"locked_by":  workerID,
				"updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return tx.Where("id IN ? AND locked_at = ? AND locked_by = ?", ids, now, workerID).
			Order("updated_at ASC, id ASC").
			Find(&claimed).Error
	})
	return claimed, err
}

// MarkCryptoRefundConfirming records partial confirmation progress.
func MarkCryptoRefundConfirming(refundID uint, confirmations int, lastErr string) error {
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"status":        PaymentRefundStatusConfirming,
		"confirmations": confirmations,
		"confirming_at": now,
		"updated_at":    now,
		"locked_at":     nil,
		"locked_by":     nil,
	}
	if strings.TrimSpace(lastErr) != "" {
		updates["last_error"] = strings.TrimSpace(lastErr)
	} else {
		updates["last_error"] = nil
	}
	return db.Model(&PaymentRefund{}).
		Where("id = ? AND status IN ?", refundID, cryptoRefundClaimableStatuses).
		Updates(updates).Error
}

// MarkCryptoRefundFailed records a hard verification failure (wrong from/to/
// amount). Does not touch the payment ledger.
func MarkCryptoRefundFailed(refundID uint, lastErr string) error {
	now := time.Now().UTC()
	return db.Model(&PaymentRefund{}).
		Where("id = ? AND status IN ?", refundID, append(cryptoRefundClaimableStatuses, PaymentRefundStatusAwaitingSignature)).
		Updates(map[string]interface{}{
			"status":     PaymentRefundStatusFailed,
			"last_error": strings.TrimSpace(lastErr),
			"failed_at":  now,
			"updated_at": now,
			"locked_at":  nil,
			"locked_by":  nil,
		}).Error
}

// RecordCryptoRefundBlockEvidence stamps the block an outbound refund tx
// confirmed in, so the reorg reconciler can later re-read the canonical chain
// and detect an orphaned refund. Best-effort and idempotent: it
// only fills the columns while still NULL/empty, scoped to the row, and treats
// RowsAffected==0 as success (a concurrent stamp already won). blockHash="" is a
// no-op — never overwrite real evidence with a blank. Evidence, not money, so it
// is a separate write from the ledger-applying confirm rather than a new
// parameter threaded through that transaction.
func RecordCryptoRefundBlockEvidence(refundID uint, blockNumber int64, blockHash string, now time.Time) error {
	blockHash = strings.TrimSpace(blockHash)
	if blockHash == "" {
		return nil
	}
	return db.Model(&PaymentRefund{}).
		Where("id = ? AND (block_hash IS NULL OR block_hash = ?)", refundID, "").
		Updates(map[string]interface{}{
			"block_number": blockNumber,
			"block_hash":   blockHash,
			"updated_at":   now.UTC(),
		}).Error
}

// CryptoRefundConfirmedInTxHook runs inside the confirm transaction. A hook
// failure rolls the ledger reversal back.
type CryptoRefundConfirmedInTxHook func(tx *gorm.DB, bill *Bill, payment *Payment, refund *PaymentRefund) error

// ConfirmCryptoRefundAndApplyLedgerWithHook marks the refund confirmed after
// on-chain verification and applies RefundBillPayment EXACTLY ONCE. If the
// ledger was already applied (reconciliation after DB failure post-chain-success),
// it only completes the refund row. Returns the updated bill when the ledger
// applied. The hook runs in the same transaction; pass nil when there is no
// side effect. The crypto worker uses the hook for the fiscal credit-note
// outbox; a hook failure rolls everything back so a later pass reconciles the
// same submitted tx hash without re-sending.
func ConfirmCryptoRefundAndApplyLedgerWithHook(refundID uint, confirmations int, actor string, hook CryptoRefundConfirmedInTxHook) (*Bill, *Payment, *PaymentRefund, error) {
	if strings.TrimSpace(actor) == "" {
		actor = "crypto_refund_worker"
	}
	var (
		resultBill    *Bill
		resultPayment *Payment
		resultRefund  *PaymentRefund
	)
	err := db.Transaction(func(tx *gorm.DB) error {
		var row PaymentRefund
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, refundID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCryptoRefundNotFound
			}
			return err
		}

		// Idempotent: already confirmed + ledger applied.
		if row.Status == PaymentRefundStatusConfirmed && row.LedgerAppliedAt != nil {
			resultRefund = &row
			return nil
		}
		if row.Status != PaymentRefundStatusSubmitted &&
			row.Status != PaymentRefundStatusConfirming &&
			!(row.Status == PaymentRefundStatusConfirmed && row.LedgerAppliedAt == nil) {
			return ErrCryptoRefundInvalidTransition
		}

		now := time.Now().UTC()

		// Apply local ledger once.
		if row.LedgerAppliedAt == nil {
			// Load payment; if already refunded (partial recovery), skip ledger.
			var payment Payment
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&payment, row.PaymentID).Error; err != nil {
				return fmt.Errorf("failed to lock payment for refund ledger: %w", err)
			}
			if payment.Status == PaymentStatusConfirmed || payment.Status == PaymentStatusRefundPending {
				// Apply the transaction-aware ledger core so the payment, bill,
				// aggregates, history, refund row, and fiscal hook share one commit.
				bill, pay, err := refundBillPaymentTx(tx, row.BillID, row.PaymentID, actor, row.Reason)
				if err != nil {
					return fmt.Errorf("ledger refund failed after on-chain confirm: %w", err)
				}
				resultBill = bill
				resultPayment = pay
			} else if payment.Status == PaymentStatusRefunded {
				// Already ledger-reversed; continue to mark refund confirmed.
				resultPayment = &payment
				var bill Bill
				if err := tx.First(&bill, row.BillID).Error; err != nil {
					return fmt.Errorf("failed to load bill for refund reconciliation: %w", err)
				}
				resultBill = &bill
			} else {
				return fmt.Errorf("%w: payment status %s", ErrPaymentNotRefundable, payment.Status)
			}
			row.LedgerAppliedAt = &now
		}

		if hook != nil && resultBill != nil && resultPayment != nil {
			if err := hook(tx, resultBill, resultPayment, &row); err != nil {
				return fmt.Errorf("crypto refund confirmed hook failed: %w", err)
			}
		}

		updates := map[string]interface{}{
			"status":            PaymentRefundStatusConfirmed,
			"confirmations":     confirmations,
			"confirmed_at":      now,
			"ledger_applied_at": row.LedgerAppliedAt,
			"last_error":        nil,
			"updated_at":        now,
			"locked_at":         nil,
			"locked_by":         nil,
		}
		if err := tx.Model(&PaymentRefund{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&row, row.ID).Error; err != nil {
			return err
		}
		resultRefund = &row
		return nil
	})
	return resultBill, resultPayment, resultRefund, err
}

// refundBillPaymentTx is the transactional core of RefundBillPayment so the
// crypto-refund confirmation path can apply ledger inside the same TX that
// marks the refund confirmed (exactly-once).
func refundBillPaymentTx(tx *gorm.DB, billID, paymentID uint, actor, reason string) (*Bill, *Payment, error) {
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		return nil, nil, fmt.Errorf("refund reason is required")
	}

	var bill Bill
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&bill, billID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("bill not found")
		}
		return nil, nil, fmt.Errorf("failed to lock bill: %w", err)
	}

	var payment Payment
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&payment, paymentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("payment not found")
		}
		return nil, nil, fmt.Errorf("failed to lock payment: %w", err)
	}
	if payment.BillID != bill.ID {
		return nil, nil, fmt.Errorf("payment does not belong to this bill")
	}
	if payment.Status != PaymentStatusConfirmed && payment.Status != PaymentStatusRefundPending {
		return nil, nil, ErrPaymentNotRefundable
	}

	var splitShareForRefund *BillSplitShare
	var linkedSplitShare BillSplitShare
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("bill_id = ? AND payment_id = ? AND status = ?", bill.ID, payment.ID, BillSplitShareStatusSettled).
		Take(&linkedSplitShare).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("failed to lock linked split share for refund: %w", err)
		}
	} else {
		splitShareForRefund = &linkedSplitShare
	}

	newPaid := bill.PaidAmount - payment.Amount
	if newPaid < 0 {
		newPaid = 0
	}
	newTip := bill.TipAmount - payment.TipAmount
	if newTip < 0 {
		newTip = 0
	}

	newStatus := BillStatusOpen
	if newPaid > billPaymentAmountTolerance {
		if newPaid >= bill.TotalAmount-billPaymentAmountTolerance {
			newStatus = BillStatusPaid
		} else {
			newStatus = BillStatusPartial
		}
	}

	now := time.Now().UTC()
	billUpdates := map[string]interface{}{
		"paid_amount": newPaid,
		"tip_amount":  newTip,
		"status":      newStatus,
		"updated_at":  now,
	}
	if newStatus == BillStatusOpen || newStatus == BillStatusPartial {
		billUpdates["closed_at"] = nil
		billUpdates["closed_by_staff_id"] = nil
	}
	if err := updateBillLifecycleTx(tx, bill.ID, billUpdates); err != nil {
		return nil, nil, fmt.Errorf("failed to update bill after refund: %w", err)
	}

	if err := tx.Model(&Payment{}).Where("id = ? AND status IN ?", payment.ID, []PaymentStatus{PaymentStatusConfirmed, PaymentStatusRefundPending}).
		Updates(map[string]interface{}{
			"status":      PaymentStatusRefunded,
			"reversed_at": &now,
			"updated_at":  now,
		}).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to mark payment refunded: %w", err)
	}

	if splitShareForRefund != nil {
		if err := tx.Model(&BillSplitShare{}).
			Where("id = ? AND status = ?", splitShareForRefund.ID, BillSplitShareStatusSettled).
			Updates(map[string]interface{}{
				"status":          BillSplitShareStatusReleased,
				"released_at":     &now,
				"hold_expires_at": nil,
				"updated_at":      now,
			}).Error; err != nil {
			return nil, nil, fmt.Errorf("failed to release split share: %w", err)
		}
	}

	recognizedBillDelta := int64(0)
	if bill.PaidAmount > 0 && newPaid == 0 {
		recognizedBillDelta = -1
	}
	if _, _, err := incrementBusinessRevenueAggregateTx(tx, bill.BusinessID, -payment.Amount, -payment.TipAmount, recognizedBillDelta); err != nil {
		return nil, nil, fmt.Errorf("failed to update revenue aggregate for refund: %w", err)
	}

	eventDetails := map[string]interface{}{
		"payment_id":      payment.ID,
		"payment_method":  payment.PaymentMethod,
		"amount":          payment.Amount,
		"tip_amount":      payment.TipAmount,
		"paid_before":     bill.PaidAmount,
		"paid_after":      newPaid,
		"status_before":   string(bill.Status),
		"status_after":    string(newStatus),
		"crypto_on_chain": true,
	}
	event := BillHistoryEvent{
		BillID:     bill.ID,
		BusinessID: bill.BusinessID,
		EventType:  BillHistoryEventPaymentRefunded,
		Actor:      actor,
		Reason:     trimmedReason,
		Details:    eventDetails,
	}
	if err := tx.Create(&event).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to record refund history: %w", err)
	}

	var refreshedBill Bill
	if err := tx.First(&refreshedBill, bill.ID).Error; err != nil {
		return nil, nil, err
	}
	var refreshedPayment Payment
	if err := tx.First(&refreshedPayment, payment.ID).Error; err != nil {
		return nil, nil, err
	}
	return &refreshedBill, &refreshedPayment, nil
}

// commonChecksumOrRaw normalizes 0x addresses to a stable lower-hex form.
// Full EIP-55 checksum is optional; EqualFold compares are used at verify time.
func commonChecksumOrRaw(addr string) string {
	addr = strings.TrimSpace(addr)
	if !strings.HasPrefix(addr, "0x") && !strings.HasPrefix(addr, "0X") {
		return addr
	}
	return "0x" + strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(addr, "0x"), "0X"))
}

// isValidEthereumAddress is a lightweight hex-address check (0x + 40 hex).
func isValidEthereumAddress(addr string) bool {
	addr = strings.TrimSpace(addr)
	if len(addr) != 42 {
		return false
	}
	if !strings.HasPrefix(addr, "0x") && !strings.HasPrefix(addr, "0X") {
		return false
	}
	for _, c := range addr[2:] {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
