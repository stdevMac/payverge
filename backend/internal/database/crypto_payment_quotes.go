package database

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/txhash"
)

// Guest crypto payer binding.
//
// A USDC Transfer log proves "some wallet paid this venue wallet N
// micro-USDC"; it does not say which bill the money was for. Every guest
// crypto quote is therefore persisted with a server-chosen offset of
// 1..CryptoQuoteMaxOffsetMicrounits micro-USDC (always below one cent) on top
// of the converted amount, so no two ACTIVE quotes for one receiving wallet
// share an exact amount. Settlement then requires the on-chain value to equal
// the quote's exact amount and consumes the quote (active -> consumed) in the
// same transaction as the payment insert, so a transfer someone else made for
// a different quote can never settle this one, and a quote settles at most
// once.

type CryptoPaymentQuoteStatus string

const (
	CryptoPaymentQuoteStatusActive   CryptoPaymentQuoteStatus = "active"
	CryptoPaymentQuoteStatusConsumed CryptoPaymentQuoteStatus = "consumed"
	CryptoPaymentQuoteStatusExpired  CryptoPaymentQuoteStatus = "expired"
)

const (
	// CryptoQuoteMaxOffsetMicrounits bounds the unique offset. USDC has six
	// decimals, so 9999 micro-USDC is just under one cent: the guest never pays
	// a visible extra cent and the ledger (which records cents) is unchanged.
	CryptoQuoteMaxOffsetMicrounits int64 = 9999
	// CryptoQuoteMaxActivePerBill caps live quotes per bill. Each quote holds
	// one slot of the wallet's offset space for its TTL; the cap keeps one bill
	// (or one scripted client) from draining that space.
	CryptoQuoteMaxActivePerBill int64 = 40
	// CryptoQuoteMaxActivePerClient caps the live quotes one client (by
	// IssueCryptoPaymentQuoteInput.ClientKey) holds on one bill, below the
	// per-bill cap, so anyone holding the bill link cannot take every slot
	// from one address and lock the other payers out for a quote TTL. The
	// guest UI mints one quote per pay attempt, and a table behind one venue
	// NAT shares an address, so it leaves room for several payers and
	// retries.
	CryptoQuoteMaxActivePerClient int64 = 12
	// CryptoQuoteAmountReuseCooldown keeps an exact amount out of circulation
	// for a while after its quote stopped being active. A transfer sent for an
	// expired quote right before it expired would otherwise match a fresh quote
	// that reused the amount within the quote's block-time skew window.
	CryptoQuoteAmountReuseCooldown = 10 * time.Minute
	// cryptoQuoteInsertAttempts bounds retries when a concurrent issuer took
	// the same exact amount between our read and our insert. Each failed
	// attempt implies another issuer succeeded, so with the random start
	// offset a retry is rare and a run of eight is practically impossible.
	cryptoQuoteInsertAttempts = 8
)

var (
	// ErrCryptoQuoteNotFound: the quote id carried by a signed token has no row.
	ErrCryptoQuoteNotFound = errors.New("crypto payment quote not found")
	// ErrCryptoQuoteUnavailable: the quote is no longer active (consumed,
	// expired) at the moment settlement tried to consume it.
	ErrCryptoQuoteUnavailable = errors.New("crypto payment quote is no longer active")
	// ErrCryptoQuoteExpired narrows ErrCryptoQuoteUnavailable: the quote ran
	// past its TTL (or was swept to expired) between the handler's check and
	// the consume. Consume errors wrap both sentinels, so callers matching
	// ErrCryptoQuoteUnavailable keep working and the guest-facing code can
	// tell an expired quote from one another transfer already consumed.
	ErrCryptoQuoteExpired = errors.New("crypto payment quote expired")
	// ErrCryptoQuoteLimitReached: the bill already holds the maximum number of
	// active quotes.
	ErrCryptoQuoteLimitReached = errors.New("too many active crypto payment quotes for this bill")
	// ErrCryptoQuoteClientLimitReached: the requesting client already holds
	// CryptoQuoteMaxActivePerClient active quotes on this bill.
	ErrCryptoQuoteClientLimitReached = errors.New("too many active crypto payment quotes from this client for this bill")
	// ErrCryptoQuoteAmountsExhausted: every exact amount near this base amount
	// is held by another live quote for the same wallet.
	ErrCryptoQuoteAmountsExhausted = errors.New("no unique crypto payment amount available")
)

// CryptoQuoteOffsetSource returns a pseudo-random int in [0, n). The offset
// does not need to be secret (it is shown to the guest and lands on-chain); it
// only needs to spread concurrent issuers so they rarely collide. Tests swap it
// to force collisions.
var CryptoQuoteOffsetSource = func(n int) int { return rand.IntN(n) }

// CryptoPaymentQuote is one server-issued guest crypto quote. ExactMicrounits
// is the only amount a transfer may carry to settle it.
type CryptoPaymentQuote struct {
	ID                uint                     `gorm:"primaryKey" json:"id"`
	BillID            uint                     `gorm:"not null;index:idx_crypto_payment_quotes_bill_status,priority:1" json:"bill_id"`
	BusinessID        uint                     `gorm:"not null;index:idx_crypto_payment_quotes_business_id" json:"business_id"`
	ChainID           int64                    `gorm:"not null;uniqueIndex:idx_crypto_payment_quotes_active_amount,where:status = 'active',priority:1;index:idx_crypto_payment_quotes_wallet_amount,priority:1" json:"chain_id"`
	SettlementAddress string                   `gorm:"size:42;not null;uniqueIndex:idx_crypto_payment_quotes_active_amount,where:status = 'active',priority:2;index:idx_crypto_payment_quotes_wallet_amount,priority:2" json:"settlement_address"`
	ExactMicrounits   int64                    `gorm:"not null;uniqueIndex:idx_crypto_payment_quotes_active_amount,where:status = 'active',priority:3;index:idx_crypto_payment_quotes_wallet_amount,priority:3" json:"exact_microunits"`
	PaymentMethod     string                   `gorm:"size:32;not null" json:"payment_method"`
	BaseMicrounits    int64                    `gorm:"not null" json:"base_microunits"`
	OffsetMicrounits  int64                    `gorm:"not null" json:"offset_microunits"`
	Status            CryptoPaymentQuoteStatus `gorm:"size:16;not null;default:active;index:idx_crypto_payment_quotes_bill_status,priority:2" json:"status"`
	IssuedAt          time.Time                `gorm:"not null" json:"issued_at"`
	ExpiresAt         time.Time                `gorm:"not null;index:idx_crypto_payment_quotes_wallet_amount,priority:4" json:"expires_at"`
	ConsumedAt        *time.Time               `json:"consumed_at,omitempty"`
	ConsumedTxHash    *string                  `gorm:"size:66" json:"consumed_tx_hash,omitempty"`
	PaymentID         *uint                    `gorm:"index:idx_crypto_payment_quotes_payment_id,where:payment_id IS NOT NULL" json:"payment_id,omitempty"`
	// ClientKey is an opaque keyed hash of the requesting client (never a raw
	// IP), held only while the quote is active; see CryptoQuoteMaxActivePerClient.
	ClientKey *string   `gorm:"size:64" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (CryptoPaymentQuote) TableName() string { return "crypto_payment_quotes" }

// IssueCryptoPaymentQuoteInput describes the quote to persist. BaseMicrounits
// is the converted bill amount before the unique offset.
type IssueCryptoPaymentQuoteInput struct {
	BillID            uint
	BusinessID        uint
	SettlementAddress string
	ChainID           int64
	PaymentMethod     string
	BaseMicrounits    int64
	IssuedAt          time.Time
	ExpiresAt         time.Time
	// ClientKey identifies the requesting client for the per-client cap. It
	// must be an opaque 32-hex-char keyed hash, never a raw address; empty
	// skips the per-client cap (non-guest callers).
	ClientKey string
}

// NormalizeCryptoQuoteAddress is the single spelling quote rows use for a
// wallet: lowercase, trimmed. Uniqueness and lookups both key on it.
func NormalizeCryptoQuoteAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

// IssueCryptoPaymentQuote reserves a unique exact amount for one guest quote.
// It runs as short independent statements (no long transaction): the partial
// unique index on (chain_id, settlement_address, exact_microunits) WHERE
// status = 'active' is what guarantees uniqueness under concurrency; the read
// of taken amounts only makes a collision unlikely.
func IssueCryptoPaymentQuote(in IssueCryptoPaymentQuoteInput) (*CryptoPaymentQuote, error) {
	return issueCryptoPaymentQuote(db, in)
}

func issueCryptoPaymentQuote(conn *gorm.DB, in IssueCryptoPaymentQuoteInput) (*CryptoPaymentQuote, error) {
	if conn == nil {
		return nil, errors.New("database unavailable")
	}
	address := NormalizeCryptoQuoteAddress(in.SettlementAddress)
	if in.BillID == 0 || in.BusinessID == 0 || address == "" || in.ChainID == 0 || in.BaseMicrounits <= 0 {
		return nil, errors.New("invalid crypto payment quote input")
	}
	now := in.IssuedAt.UTC().Truncate(time.Second)
	expiresAt := in.ExpiresAt.UTC().Truncate(time.Second)
	if !expiresAt.After(now) {
		return nil, errors.New("crypto payment quote must expire after it is issued")
	}
	var clientKey *string
	if key := strings.TrimSpace(in.ClientKey); key != "" {
		clientKey = &key
	}

	// Free the wallet's lapsed slots so they stop counting against the
	// partial unique index. The status is a literal (not a bind parameter) so
	// the planner can prove the partial-index predicate and scan only the
	// wallet's active rows, not its whole quote history.
	if err := conn.Model(&CryptoPaymentQuote{}).
		Where("chain_id = ? AND settlement_address = ? AND status = '"+string(CryptoPaymentQuoteStatusActive)+"' AND expires_at <= ?",
			in.ChainID, address, now).
		Updates(map[string]any{"status": CryptoPaymentQuoteStatusExpired, "client_key": nil, "updated_at": now}).Error; err != nil {
		return nil, fmt.Errorf("expire stale crypto quotes: %w", err)
	}

	// One read answers both caps: the bill's live quotes and, of those, the
	// ones this client holds. NULL client keys never match.
	// Scanned straight into two ints (Row, not Scan into a struct) so the
	// folded count costs no more than the plain COUNT it replaced.
	var activeForBill, activeForClient int64
	clientKeyArg := ""
	if clientKey != nil {
		clientKeyArg = *clientKey
	}
	if err := conn.Model(&CryptoPaymentQuote{}).
		Select("COUNT(*), COALESCE(SUM(CASE WHEN client_key = ? THEN 1 ELSE 0 END), 0)", clientKeyArg).
		Where("bill_id = ? AND status = ? AND expires_at > ?", in.BillID, CryptoPaymentQuoteStatusActive, now).
		Row().Scan(&activeForBill, &activeForClient); err != nil {
		return nil, fmt.Errorf("count active crypto quotes: %w", err)
	}
	if clientKey != nil && activeForClient >= CryptoQuoteMaxActivePerClient {
		return nil, ErrCryptoQuoteClientLimitReached
	}
	if activeForBill >= CryptoQuoteMaxActivePerBill {
		return nil, ErrCryptoQuoteLimitReached
	}

	low := in.BaseMicrounits + 1
	high := in.BaseMicrounits + CryptoQuoteMaxOffsetMicrounits
	reuseCutoff := now.Add(-CryptoQuoteAmountReuseCooldown)
	for attempt := 0; attempt < cryptoQuoteInsertAttempts; attempt++ {
		// Re-read on every attempt: a unique violation means another issuer
		// committed an amount since our last read, possibly more than one.
		var takenAmounts []int64
		if err := conn.Model(&CryptoPaymentQuote{}).
			Where("chain_id = ? AND settlement_address = ? AND exact_microunits BETWEEN ? AND ? AND expires_at > ?",
				in.ChainID, address, low, high, reuseCutoff).
			Pluck("exact_microunits", &takenAmounts).Error; err != nil {
			return nil, fmt.Errorf("load taken crypto quote amounts: %w", err)
		}
		taken := make(map[int64]struct{}, len(takenAmounts))
		for _, amount := range takenAmounts {
			taken[amount-in.BaseMicrounits] = struct{}{}
		}
		offset, ok := pickFreeCryptoQuoteOffset(taken)
		if !ok {
			return nil, ErrCryptoQuoteAmountsExhausted
		}
		quote := &CryptoPaymentQuote{
			BillID:            in.BillID,
			BusinessID:        in.BusinessID,
			ChainID:           in.ChainID,
			SettlementAddress: address,
			PaymentMethod:     strings.TrimSpace(in.PaymentMethod),
			BaseMicrounits:    in.BaseMicrounits,
			OffsetMicrounits:  offset,
			ExactMicrounits:   in.BaseMicrounits + offset,
			Status:            CryptoPaymentQuoteStatusActive,
			IssuedAt:          now,
			ExpiresAt:         expiresAt,
			ClientKey:         clientKey,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		err := conn.Create(quote).Error
		if err == nil {
			return quote, nil
		}
		if !isUniqueConstraintError(err) {
			return nil, fmt.Errorf("create crypto quote: %w", err)
		}
		// A concurrent issuer won this exact amount; never hand it out twice.
	}
	return nil, ErrCryptoQuoteAmountsExhausted
}

// pickFreeCryptoQuoteOffset starts at a random offset and probes forward
// (wrapping) for the first one not in taken. Offsets are 1..max.
func pickFreeCryptoQuoteOffset(taken map[int64]struct{}) (int64, bool) {
	span := CryptoQuoteMaxOffsetMicrounits
	if int64(len(taken)) >= span {
		return 0, false
	}
	start := int64(CryptoQuoteOffsetSource(int(span)))
	if start < 0 || start >= span {
		start = 0
	}
	for i := int64(0); i < span; i++ {
		offset := (start+i)%span + 1
		if _, used := taken[offset]; !used {
			return offset, true
		}
	}
	return 0, false
}

// GetCryptoPaymentQuote loads one quote row by id.
func GetCryptoPaymentQuote(id uint) (*CryptoPaymentQuote, error) {
	if db == nil {
		return nil, errors.New("database unavailable")
	}
	if id == 0 {
		return nil, ErrCryptoQuoteNotFound
	}
	var quote CryptoPaymentQuote
	if err := db.Where("id = ?", id).Take(&quote).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCryptoQuoteNotFound
		}
		return nil, err
	}
	return &quote, nil
}

// PaymentTxHashHolder names the payment (and its bill) that already records an
// on-chain transfer. It is deliberately narrow: the guest crypto handler only
// needs to know which bill a replayed transfer already paid, so it can tell
// operators "this transfer already paid bill X" instead of filing the replay
// as a wrong-amount payment someone might settle or refund by hand.
type PaymentTxHashHolder struct {
	PaymentID  uint
	BillID     uint
	BusinessID uint
	BillNumber string
}

// FindPaymentTxHashHolder returns the payment already recorded under txHash
// (canonicalized the same way payments.tx_hash is written), or nil when no
// payment holds it. BusinessID and BillNumber are zero/empty if the bill row
// is gone. It is a single indexed lookup on payments.tx_hash; callers run it
// only on refusal paths, never on the settlement happy path.
func FindPaymentTxHashHolder(txHash string) (*PaymentTxHashHolder, error) {
	if db == nil {
		return nil, errors.New("database unavailable")
	}
	ref := txhash.NormalizeReference(txHash)
	if ref == "" {
		return nil, nil
	}
	var rows []PaymentTxHashHolder
	err := db.Table("payments").
		Select("payments.id AS payment_id, payments.bill_id AS bill_id, COALESCE(bills.business_id, 0) AS business_id, COALESCE(bills.bill_number, '') AS bill_number").
		Joins("LEFT JOIN bills ON bills.id = payments.bill_id").
		Where("payments.tx_hash = ?", ref).
		Limit(1).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("find payment by tx hash: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// consumeCryptoPaymentQuoteTx flips an active, unexpired quote to consumed
// inside the settlement transaction. The conditional UPDATE is the single-use
// guarantee: of two settlements racing on one quote, exactly one sees a row
// affected; the other gets ErrCryptoQuoteUnavailable and its whole
// transaction (payment insert included) rolls back.
func consumeCryptoPaymentQuoteTx(tx *gorm.DB, quoteID uint, billID uint, exactMicrounits int64, txHash string, paymentID uint, now time.Time) error {
	nowUTC := now.UTC()
	res := tx.Model(&CryptoPaymentQuote{}).
		Where("id = ? AND bill_id = ? AND exact_microunits = ? AND status = ? AND expires_at > ?",
			quoteID, billID, exactMicrounits, CryptoPaymentQuoteStatusActive, nowUTC).
		Updates(map[string]any{
			"status":           CryptoPaymentQuoteStatusConsumed,
			"consumed_at":      nowUTC,
			"consumed_tx_hash": txHash,
			"payment_id":       paymentID,
			"client_key":       nil, // only live quotes keep the client key
			"updated_at":       nowUTC,
		})
	if res.Error != nil {
		return fmt.Errorf("consume crypto quote: %w", res.Error)
	}
	if res.RowsAffected != 1 {
		return cryptoQuoteConsumeRefusal(tx, quoteID, billID, exactMicrounits, nowUTC)
	}
	return nil
}

// cryptoQuoteConsumeRefusal explains a consume that matched no row. A quote
// for this bill and amount that is still active past its expiry, or already
// swept to expired, is reported as ErrCryptoQuoteExpired (wrapped in
// ErrCryptoQuoteUnavailable). Anything else (consumed by another transfer,
// another bill or amount, missing) stays plain ErrCryptoQuoteUnavailable.
func cryptoQuoteConsumeRefusal(tx *gorm.DB, quoteID uint, billID uint, exactMicrounits int64, now time.Time) error {
	var row struct {
		Status    CryptoPaymentQuoteStatus
		ExpiresAt time.Time
	}
	err := tx.Model(&CryptoPaymentQuote{}).
		Select("status", "expires_at").
		Where("id = ? AND bill_id = ? AND exact_microunits = ?", quoteID, billID, exactMicrounits).
		Take(&row).Error
	if err == nil {
		expired := row.Status == CryptoPaymentQuoteStatusExpired ||
			(row.Status == CryptoPaymentQuoteStatusActive && !row.ExpiresAt.After(now))
		if expired {
			return fmt.Errorf("%w: %w: quote %d", ErrCryptoQuoteUnavailable, ErrCryptoQuoteExpired, quoteID)
		}
	}
	return fmt.Errorf("%w: quote %d", ErrCryptoQuoteUnavailable, quoteID)
}
