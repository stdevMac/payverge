package database

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/guestsession"
)

type BillSplitMode string

const (
	BillSplitModeEqual  BillSplitMode = "equal"
	BillSplitModeItems  BillSplitMode = "items"
	BillSplitModeCustom BillSplitMode = "custom"
)

type BillSplitShareStatus string

const (
	BillSplitShareStatusHeld     BillSplitShareStatus = "held"
	BillSplitShareStatusSettled  BillSplitShareStatus = "settled"
	BillSplitShareStatusReleased BillSplitShareStatus = "released"
	BillSplitShareStatusFailed   BillSplitShareStatus = "failed"
)

var (
	ErrInvalidSplitMode             = errors.New("invalid split mode")
	ErrInvalidSplitTender           = errors.New("invalid split tender")
	ErrInvalidIdempotencyKey        = errors.New("split idempotency key is required")
	ErrSplitAmountUnavailable       = errors.New("split amount exceeds available balance")
	ErrSplitHoldExpired             = errors.New("split hold has expired")
	ErrSplitItemUnavailable         = errors.New("split item fraction is unavailable")
	ErrSplitShareNotFound           = errors.New("split share not found")
	ErrSplitGuestMismatch           = errors.New("split share belongs to another guest session")
	ErrSplitShareAlreadyFinal       = errors.New("split share is already finalized")
	ErrSplitReceiptUnavailable      = errors.New("split share receipt is available after settlement")
	ErrPaymentAlreadySettledForBill = errors.New("payment already settled for another share on this bill")
)

type BillSplitShare struct {
	ID                       uint                 `gorm:"primaryKey" json:"id"`
	BillID                   uint                 `gorm:"index;index:idx_bill_split_shares_bill_status,priority:1;index:idx_bill_split_shares_bill_expires,priority:1;not null" json:"bill_id"`
	GuestSessionID           string               `gorm:"size:128;index;not null" json:"guest_session_id"`
	DisplayName              string               `gorm:"size:120" json:"display_name"`
	Mode                     BillSplitMode        `gorm:"size:16;not null" json:"mode"`
	ClaimedItemIDs           []string             `gorm:"serializer:json" json:"claimed_item_ids,omitempty"`
	ClaimedFractions         map[string]string    `gorm:"serializer:json" json:"claimed_fractions,omitempty"`
	AmountCents              int64                `gorm:"not null" json:"amount_cents"`
	TipCents                 int64                `gorm:"default:0" json:"tip_cents"`
	Status                   BillSplitShareStatus `gorm:"size:16;index:idx_bill_split_shares_bill_status,priority:2;not null;default:'held'" json:"status"`
	HoldExpiresAt            *time.Time           `gorm:"index:idx_bill_split_shares_bill_expires,priority:2" json:"hold_expires_at,omitempty"`
	IdempotencyKey           string               `gorm:"size:160;index:idx_bill_split_shares_hold_idempotency,priority:2" json:"idempotency_key,omitempty"`
	SettlementIdempotencyKey string               `gorm:"size:160;index:idx_bill_split_shares_settle_idempotency,priority:2" json:"settlement_idempotency_key,omitempty"`
	Tender                   string               `gorm:"size:32" json:"tender,omitempty"`
	PaymentID                *uint                `gorm:"index" json:"payment_id,omitempty"`
	AlternativePaymentID     *uint                `gorm:"index" json:"alternative_payment_id,omitempty"`
	SettledAt                *time.Time           `json:"settled_at,omitempty"`
	ReleasedAt               *time.Time           `json:"released_at,omitempty"`
	CreatedAt                time.Time            `json:"created_at"`
	UpdatedAt                time.Time            `json:"updated_at"`

	Bill               Bill                `gorm:"foreignKey:BillID" json:"-"`
	Payment            *Payment            `gorm:"foreignKey:PaymentID" json:"-"`
	AlternativePayment *AlternativePayment `gorm:"foreignKey:AlternativePaymentID" json:"-"`
}

type BillSplitShareView struct {
	ID                   uint                 `json:"id"`
	GuestSessionID       string               `json:"guest_session_id"`
	DisplayName          string               `json:"display_name"`
	Mode                 BillSplitMode        `json:"mode"`
	ClaimedItemIDs       []string             `json:"claimed_item_ids,omitempty"`
	ClaimedFractions     map[string]string    `json:"claimed_fractions,omitempty"`
	AmountCents          int64                `json:"amount_cents"`
	TipCents             int64                `json:"tip_cents"`
	Status               BillSplitShareStatus `json:"status"`
	HoldExpiresAt        *time.Time           `json:"hold_expires_at,omitempty"`
	Tender               string               `json:"tender,omitempty"`
	PaymentID            *uint                `json:"payment_id,omitempty"`
	AlternativePaymentID *uint                `json:"alternative_payment_id,omitempty"`
	SettledAt            *time.Time           `json:"settled_at,omitempty"`
	ReleasedAt           *time.Time           `json:"released_at,omitempty"`
}

type BillSplitState struct {
	BillID         uint                 `json:"-"`
	BusinessID     uint                 `json:"-"`
	BillNumber     string               `json:"bill_number"`
	Status         BillStatus           `json:"status"`
	TotalCents     int64                `json:"total_cents"`
	PaidCents      int64                `json:"paid_cents"`
	HeldCents      int64                `json:"held_cents"`
	AvailableCents int64                `json:"available_cents"`
	Shares         []BillSplitShareView `json:"shares"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

type BillSplitReceiptItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Fraction       string `json:"fraction"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	SubtotalCents  int64  `json:"subtotal_cents"`
}

type BillSplitShareReceipt struct {
	ShareID              uint                   `json:"share_id"`
	BillNumber           string                 `json:"bill_number"`
	DisplayName          string                 `json:"display_name"`
	Mode                 BillSplitMode          `json:"mode"`
	Status               BillSplitShareStatus   `json:"status"`
	Tender               string                 `json:"tender,omitempty"`
	PaymentID            *uint                  `json:"payment_id,omitempty"`
	AlternativePaymentID *uint                  `json:"alternative_payment_id,omitempty"`
	SubtotalCents        int64                  `json:"subtotal_cents"`
	TaxCents             int64                  `json:"tax_cents"`
	ServiceFeeCents      int64                  `json:"service_fee_cents"`
	AmountCents          int64                  `json:"amount_cents"`
	TipCents             int64                  `json:"tip_cents"`
	GrandTotalCents      int64                  `json:"grand_total_cents"`
	Items                []BillSplitReceiptItem `json:"items"`
	SettledAt            *time.Time             `json:"settled_at,omitempty"`
}

type billSplitShareStateRow struct {
	ID                   uint
	GuestSessionID       string
	DisplayName          string
	Mode                 BillSplitMode
	ClaimedItemIDs       string
	ClaimedFractions     string
	AmountCents          int64
	TipCents             int64
	Status               BillSplitShareStatus
	HoldExpiresAt        *time.Time
	Tender               string
	PaymentID            *uint
	AlternativePaymentID *uint
	SettledAt            *time.Time
	ReleasedAt           *time.Time
}

type HoldBillSplitShareInput struct {
	BillID           uint
	GuestSessionID   string
	DisplayName      string
	Mode             BillSplitMode
	CoverRemaining   bool
	NumPeople        int
	SharesCovered    int
	ClaimedItemIDs   []string
	ClaimedFractions map[string]string
	AmountCents      int64
	TipCents         int64
	IdempotencyKey   string
	HoldTTL          time.Duration
	Now              time.Time
}

type SettleBillSplitShareInput struct {
	ShareID           uint
	GuestSessionID    string
	IdempotencyKey    string
	Tender            string
	TxHash            string
	PayerAddr         string
	TipCents          int64
	SourceChain       string
	SourceToken       string
	LifiRouteID       string
	RefundDestination *PaymentRefundDestination
	// BlockNumber / BlockHash pin a confirmed on-chain crypto split-share
	// settlement to its block so the reorg reconciler sweeps it identically to a
	// non-split crypto payment. Nil for non-crypto / verify-only.
	BlockNumber *int64
	BlockHash   *string
	// CryptoQuoteID / CryptoQuoteExactMicrounits: see ConfirmedPaymentInput.
	// The guest crypto quote is consumed in the same transaction as the
	// split-share payment insert.
	CryptoQuoteID              uint
	CryptoQuoteExactMicrounits int64
	Now                        time.Time
}

type MarkBillSplitShareSettledByPaymentInput struct {
	ShareID        uint
	BillID         uint
	PaymentID      uint
	TipCents       int64
	Tender         string
	IdempotencyKey string
	Now            time.Time
}

type ReleaseBillSplitShareInput struct {
	ShareID        uint
	BillID         uint
	GuestSessionID string
	Now            time.Time
}

const defaultBillSplitHoldTTL = 5 * time.Minute
const billSplitAlternativePaymentParticipantAddrPrefix = "guest_split_share:"

func BillSplitAlternativePaymentParticipantAddr(shareID uint) string {
	if shareID == 0 {
		return "guest"
	}
	return fmt.Sprintf("%s%d", billSplitAlternativePaymentParticipantAddrPrefix, shareID)
}

func splitShareIDFromAlternativePaymentParticipant(value string) (uint, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, billSplitAlternativePaymentParticipantAddrPrefix) {
		raw := strings.TrimPrefix(value, billSplitAlternativePaymentParticipantAddrPrefix)
		shareID, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 32)
		return uint(shareID), err == nil && shareID > 0
	}
	for _, part := range strings.Split(value, "|") {
		key, raw, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found || key != "split_share_id" {
			continue
		}
		shareID, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 32)
		return uint(shareID), err == nil && shareID > 0
	}
	return 0, false
}

func splitShareIDFromAlternativePayment(payment AlternativePayment) (uint, bool) {
	if shareID, ok := splitShareIDFromAlternativePaymentParticipant(payment.ParticipantAddr); ok {
		return shareID, true
	}
	return splitShareIDFromAlternativePaymentParticipant(payment.ParticipantName)
}

func validBillSplitMode(mode BillSplitMode) bool {
	switch mode {
	case BillSplitModeEqual, BillSplitModeItems, BillSplitModeCustom:
		return true
	default:
		return false
	}
}

func billSplitNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

func normalizeSplitTender(tender string) (string, error) {
	tender = strings.ToLower(strings.TrimSpace(tender))
	switch tender {
	case "crypto", "cross-chain", "cross_chain", "plugin":
		if tender == "cross_chain" {
			return "cross-chain", nil
		}
		return tender, nil
	case "cashier", "cash":
		return "cash", nil
	case "card":
		return "card", nil
	case "venmo":
		return "venmo", nil
	case "other":
		return "other", nil
	default:
		return "", ErrInvalidSplitTender
	}
}

func normalizeSplitIdempotencyKey(value string) string {
	return strings.TrimSpace(value)
}

func splitSyntheticTxHash(shareID uint, idempotencyKey, tender string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", shareID, tender, idempotencyKey)))
	return "split_" + hex.EncodeToString(sum[:])[:40]
}

func parseSplitFraction(raw string) (*big.Rat, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "1"
	}
	rat, ok := new(big.Rat).SetString(raw)
	if !ok || rat.Sign() <= 0 || rat.Cmp(big.NewRat(1, 1)) > 0 {
		return nil, ErrSplitItemUnavailable
	}
	return rat, nil
}

func splitFractionToString(rat *big.Rat) string {
	if rat == nil {
		return "1"
	}
	if rat.IsInt() {
		return rat.Num().String()
	}
	return rat.RatString()
}

func prorateCentsByFraction(cents int64, fraction *big.Rat) int64 {
	if cents <= 0 || fraction == nil || fraction.Sign() <= 0 {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(cents), fraction.Num())
	denominator := fraction.Denom()
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	// Deterministic half-up rounding to whole cents for the individual claim.
	if new(big.Int).Mul(remainder, big.NewInt(2)).Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient.Int64()
}

func prorateCentsBySubtotal(cents, subtotalCents, billSubtotalCents int64) int64 {
	if cents <= 0 || subtotalCents <= 0 || billSubtotalCents <= 0 {
		return 0
	}
	numerator := cents * subtotalCents
	quotient := numerator / billSubtotalCents
	remainder := numerator % billSubtotalCents
	if remainder*2 >= billSubtotalCents {
		quotient++
	}
	return quotient
}

func addSplitFractions(existing, requested map[string]*big.Rat) map[string]*big.Rat {
	combined := make(map[string]*big.Rat, len(existing)+len(requested))
	for itemID, fraction := range existing {
		combined[itemID] = new(big.Rat).Set(fraction)
	}
	for itemID, fraction := range requested {
		if combined[itemID] == nil {
			combined[itemID] = new(big.Rat)
		}
		combined[itemID].Add(combined[itemID], fraction)
	}
	return combined
}

// equalShareMetaKey stores how many equal seats a hold consumed inside the
// existing claimed_fractions JSON column — no schema migration (PG-23).
const equalShareMetaKey = "__equal_shares__"

// equalShareAmounts is the reference exact allocator (same contract as
// splitting.allocateCentsEqual): seats 0..n-2 get floor(total/n); the last
// seat receives the remainder so the sum is exact. Used by unit tests and as
// the conceptual reference for PG-23.
func equalShareAmounts(total int64, n int) []int64 {
	out := make([]int64, n)
	if n <= 0 {
		return out
	}
	base := total / int64(n)
	for i := 0; i < n-1; i++ {
		out[i] = base
	}
	out[n-1] = total - base*int64(n-1)
	return out
}

// calculateEqualSplitCents returns the hold amount for covering sharesCovered
// of numPeople against availableCents without over-summing simultaneous
// previews (PG-23).
//
// Design (no schema): floor((available/numPeople)*sharesCovered). When the
// guest covers every remaining seat, return the full available so sequential
// re-splits (caller passes remaining people + current available) sum exactly.
// When available < numPeople, drain min(available, sharesCovered) pennies so
// tiny bills still clear.
func calculateEqualSplitCents(availableCents int64, numPeople, sharesCovered int) (int64, error) {
	if availableCents <= 0 {
		return 0, ErrSplitAmountUnavailable
	}
	if numPeople <= 0 || sharesCovered <= 0 || sharesCovered > numPeople {
		return 0, ErrInvalidPaymentAmount
	}
	if sharesCovered == numPeople {
		return availableCents, nil
	}
	amountCents := (availableCents / int64(numPeople)) * int64(sharesCovered)
	if amountCents <= 0 {
		// available < numPeople: floor share is 0. Drain up to sharesCovered
		// cents so sequential covered=1 claims can still exhaust the pool.
		if availableCents < int64(sharesCovered) {
			return availableCents, nil
		}
		return int64(sharesCovered), nil
	}
	return amountCents, nil
}

// equalSplitSeatsTakenTx counts equal-mode seats already held or settled so the
// next claim can re-split remaining balance across remaining people.
func equalSplitSeatsTakenTx(tx *gorm.DB, billID uint, now time.Time) (int, error) {
	var rows []BillSplitShare
	if err := tx.Model(&BillSplitShare{}).
		Select("id", "mode", "status", "hold_expires_at", "claimed_fractions", "amount_cents").
		Where("bill_id = ? AND mode = ?", billID, BillSplitModeEqual).
		Where("status IN ?", []BillSplitShareStatus{BillSplitShareStatusHeld, BillSplitShareStatusSettled}).
		Find(&rows).Error; err != nil {
		return 0, err
	}
	seats := 0
	for _, row := range rows {
		if row.Status == BillSplitShareStatusHeld && row.HoldExpiresAt != nil && !row.HoldExpiresAt.After(now) {
			continue
		}
		covered := 1
		if row.ClaimedFractions != nil {
			if raw, ok := row.ClaimedFractions[equalShareMetaKey]; ok {
				if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n > 0 {
					covered = n
				}
			}
		}
		seats += covered
	}
	return seats, nil
}

func sumProratedItemBaseCents(itemSubtotalByID map[string]int64, fractions map[string]*big.Rat) (int64, error) {
	baseCents := int64(0)
	for itemID, fraction := range fractions {
		itemSubtotalCents, ok := itemSubtotalByID[itemID]
		if !ok {
			return 0, ErrSplitItemUnavailable
		}
		baseCents += prorateCentsByFraction(itemSubtotalCents, fraction)
	}
	return baseCents, nil
}

// IsNonClaimableSplitBillItem reports bill lines that guests cannot claim in
// item-split mode. Offer/promo discount rows (item_type=discount) and any
// negative-subtotal lines are absorbed into positive menu bases instead of
// being held on their own — otherwise positive food holds oversubscribe the
// net bill total (see calculateItemSplitHoldAmountTx).
//
// PG-24: combo expansion children (item_type=bundle_item) are never
// independently claimable — guests claim the parent bundle line, which
// carries the combo price. Children remain on the bill for kitchen display.
func IsNonClaimableSplitBillItem(itemType string, subtotalCents int64) bool {
	trim := strings.TrimSpace(itemType)
	if strings.EqualFold(trim, "discount") || strings.EqualFold(trim, "bundle_item") {
		return true
	}
	return subtotalCents < 0
}

// netClaimableItemBaseCents folds discount lines into positive claimable bases
// so the sum of full item holds equals bill.Subtotal (and thus bill total after
// tax/fee proration). Discount IDs are omitted from the returned map.
//
// netSubtotalCents should be bill.Subtotal when known; when non-positive the
// function falls back to sum(positive)+sum(discount) from the item rows.
func netClaimableItemBaseCents(items []BillItem, netSubtotalCents int64) (map[string]int64, error) {
	type weightedItem struct {
		id     string
		weight int64
	}
	positive := make([]weightedItem, 0, len(items))
	var sumPositive, sumDiscount int64
	for _, item := range items {
		cents := int64(math.Round(item.Subtotal * 100))
		if IsNonClaimableSplitBillItem(item.ItemType, cents) {
			sumDiscount += cents
			continue
		}
		if cents <= 0 {
			continue
		}
		positive = append(positive, weightedItem{id: item.ID, weight: cents})
		sumPositive += cents
	}
	if len(positive) == 0 || sumPositive <= 0 {
		return nil, ErrSplitItemUnavailable
	}

	netTotal := netSubtotalCents
	if netTotal <= 0 {
		netTotal = sumPositive + sumDiscount
	}
	if netTotal <= 0 {
		return nil, ErrInvalidPaymentAmount
	}

	sort.Slice(positive, func(i, j int) bool {
		return positive[i].id < positive[j].id
	})

	result := make(map[string]int64, len(positive))
	if sumDiscount == 0 && netTotal == sumPositive {
		for _, item := range positive {
			result[item.id] = item.weight
		}
		return result, nil
	}

	// Allocate net subtotal onto positive lines by gross weight. Remainder goes
	// to the last id so sum(result) == netTotal exactly.
	allocated := int64(0)
	for i, item := range positive {
		if i == len(positive)-1 {
			result[item.id] = netTotal - allocated
			break
		}
		share := prorateCentsBySubtotal(netTotal, item.weight, sumPositive)
		result[item.id] = share
		allocated += share
	}
	return result, nil
}

func normalizeRequestedItemFractions(itemIDs []string, fractions map[string]string) (map[string]*big.Rat, []string, map[string]string, error) {
	requested := make(map[string]*big.Rat)
	ordered := make([]string, 0, len(itemIDs)+len(fractions))
	for _, itemID := range itemIDs {
		itemID = strings.TrimSpace(itemID)
		if itemID == "" {
			continue
		}
		rawFraction := "1"
		if fractions != nil && strings.TrimSpace(fractions[itemID]) != "" {
			rawFraction = fractions[itemID]
		}
		fraction, err := parseSplitFraction(rawFraction)
		if err != nil {
			return nil, nil, nil, err
		}
		if _, exists := requested[itemID]; !exists {
			ordered = append(ordered, itemID)
			requested[itemID] = fraction
			continue
		}
		requested[itemID].Add(requested[itemID], fraction)
		if requested[itemID].Cmp(big.NewRat(1, 1)) > 0 {
			return nil, nil, nil, ErrSplitItemUnavailable
		}
	}
	for itemID, rawFraction := range fractions {
		itemID = strings.TrimSpace(itemID)
		if itemID == "" {
			continue
		}
		if _, exists := requested[itemID]; exists {
			continue
		}
		fraction, err := parseSplitFraction(rawFraction)
		if err != nil {
			return nil, nil, nil, err
		}
		ordered = append(ordered, itemID)
		requested[itemID] = fraction
	}
	if len(requested) == 0 {
		return nil, nil, nil, ErrSplitItemUnavailable
	}
	normalized := make(map[string]string, len(requested))
	for itemID, fraction := range requested {
		normalized[itemID] = splitFractionToString(fraction)
	}
	return requested, ordered, normalized, nil
}

func releaseExpiredBillSplitSharesTx(tx *gorm.DB, now time.Time, billID *uint) (int64, error) {
	query := tx.Model(&BillSplitShare{}).
		Where("status = ? AND hold_expires_at IS NOT NULL AND hold_expires_at <= ?", BillSplitShareStatusHeld, now)
	if billID != nil {
		query = query.Where("bill_id = ?", *billID)
	}
	// FIND-062: always NULL hold_expires_at on release. Map nil is unreliable
	// across GORM/SQLite and left stale expiry timestamps on released shares.
	result := query.Updates(map[string]interface{}{
		"status":          BillSplitShareStatusReleased,
		"hold_expires_at": gorm.Expr("NULL"),
		"released_at":     now,
		"updated_at":      now,
	})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func activeHeldSplitCentsTx(tx *gorm.DB, billID uint, now time.Time) (int64, error) {
	var heldCents int64
	err := tx.Model(&BillSplitShare{}).
		Select("COALESCE(SUM(amount_cents), 0)").
		Where("bill_id = ? AND status = ? AND (hold_expires_at IS NULL OR hold_expires_at > ?)", billID, BillSplitShareStatusHeld, now).
		Scan(&heldCents).Error
	return heldCents, err
}

func releaseActiveBillSplitSharesTx(tx *gorm.DB, now time.Time, billID uint) (int64, error) {
	result := tx.Model(&BillSplitShare{}).
		Where("bill_id = ? AND status = ? AND (hold_expires_at IS NULL OR hold_expires_at > ?)", billID, BillSplitShareStatusHeld, now).
		Updates(map[string]interface{}{
			"status":          BillSplitShareStatusReleased,
			"hold_expires_at": gorm.Expr("NULL"),
			"released_at":     now,
			"updated_at":      now,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func existingItemFractionsTx(tx *gorm.DB, billID uint, now time.Time) (map[string]*big.Rat, error) {
	var rows []billSplitShareStateRow
	if err := tx.Table("bill_split_shares").
		Select("claimed_item_ids", "claimed_fractions").
		Where("bill_id = ? AND mode = ? AND (status = ? OR (status = ? AND (hold_expires_at IS NULL OR hold_expires_at > ?)))",
			billID,
			BillSplitModeItems,
			BillSplitShareStatusSettled,
			BillSplitShareStatusHeld,
			now,
		).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	totals := map[string]*big.Rat{}
	for _, row := range rows {
		itemIDs := []string{}
		if row.ClaimedItemIDs != "" && row.ClaimedItemIDs != "[]" {
			_ = json.Unmarshal([]byte(row.ClaimedItemIDs), &itemIDs)
		}
		fractions := map[string]string{}
		if row.ClaimedFractions != "" && row.ClaimedFractions != "{}" {
			_ = json.Unmarshal([]byte(row.ClaimedFractions), &fractions)
		}
		requested, _, _, err := normalizeRequestedItemFractions(itemIDs, fractions)
		if err != nil {
			return nil, err
		}
		for itemID, fraction := range requested {
			if totals[itemID] == nil {
				totals[itemID] = new(big.Rat)
			}
			totals[itemID].Add(totals[itemID], fraction)
		}
	}
	return totals, nil
}

func calculateItemSplitHoldAmountTx(tx *gorm.DB, bill Bill, input HoldBillSplitShareInput, now time.Time) (int64, []string, map[string]string, error) {
	requested, orderedItemIDs, normalizedFractions, err := normalizeRequestedItemFractions(input.ClaimedItemIDs, input.ClaimedFractions)
	if err != nil {
		return 0, nil, nil, err
	}

	// Load every line so offer discount rows can be folded into positive bases.
	// Claiming only the requested IDs (legacy) summed gross food subtotals and
	// ignored unclaimed negative discount lines, so item holds oversubscribed
	// bill.TotalAmount whenever a promo created a discount bill item.
	var items []BillItem
	if err := tx.Model(&BillItem{}).
		Select("id", "subtotal", "item_type").
		Where("bill_id = ?", bill.ID).
		Find(&items).Error; err != nil {
		return 0, nil, nil, err
	}
	itemBaseByID, err := netClaimableItemBaseCents(items, bill.Subtotal)
	if err != nil {
		return 0, nil, nil, err
	}
	for itemID := range requested {
		if _, ok := itemBaseByID[itemID]; !ok {
			// Discount lines and unknown IDs are not claimable.
			return 0, nil, nil, ErrSplitItemUnavailable
		}
	}

	existingFractions, err := existingItemFractionsTx(tx, bill.ID, now)
	if err != nil {
		return 0, nil, nil, err
	}
	// Ignore any non-claimable IDs left on legacy holds so they cannot block
	// net-base math or resurrect discount-line claims.
	for itemID := range existingFractions {
		if _, ok := itemBaseByID[itemID]; !ok {
			delete(existingFractions, itemID)
		}
	}
	for itemID, fraction := range requested {
		total := new(big.Rat).Set(fraction)
		if existingFractions[itemID] != nil {
			total.Add(total, existingFractions[itemID])
		}
		if total.Cmp(big.NewRat(1, 1)) > 0 {
			return 0, nil, nil, ErrSplitItemUnavailable
		}
	}

	combinedFractions := addSplitFractions(existingFractions, requested)

	baseBeforeCents, err := sumProratedItemBaseCents(itemBaseByID, existingFractions)
	if err != nil {
		return 0, nil, nil, err
	}
	baseAfterCents, err := sumProratedItemBaseCents(itemBaseByID, combinedFractions)
	if err != nil {
		return 0, nil, nil, err
	}
	baseCents := baseAfterCents - baseBeforeCents
	if baseCents <= 0 {
		return 0, nil, nil, ErrInvalidPaymentAmount
	}
	taxBeforeCents := prorateCentsBySubtotal(bill.TaxAmount, baseBeforeCents, bill.Subtotal)
	taxAfterCents := prorateCentsBySubtotal(bill.TaxAmount, baseAfterCents, bill.Subtotal)
	serviceBeforeCents := prorateCentsBySubtotal(bill.ServiceFeeAmount, baseBeforeCents, bill.Subtotal)
	serviceAfterCents := prorateCentsBySubtotal(bill.ServiceFeeAmount, baseAfterCents, bill.Subtotal)
	taxCents := taxAfterCents - taxBeforeCents
	serviceCents := serviceAfterCents - serviceBeforeCents
	gross := baseCents + taxCents + serviceCents
	// Partial bills: scale the full-price item allocation by remaining/total
	// so the hold matches the guest preview (available × selected / claimable)
	// and cannot oversubscribe the unpaid remainder (#521).
	if bill.TotalAmount > 0 && bill.PaidAmount > 0 {
		remaining := bill.TotalAmount - bill.PaidAmount
		if remaining < 0 {
			remaining = 0
		}
		if remaining == 0 {
			return 0, nil, nil, ErrSplitAmountUnavailable
		}
		scaled := (gross*remaining + bill.TotalAmount/2) / bill.TotalAmount
		if scaled <= 0 {
			return 0, nil, nil, ErrSplitAmountUnavailable
		}
		return scaled, orderedItemIDs, normalizedFractions, nil
	}
	return gross, orderedItemIDs, normalizedFractions, nil
}

func billSplitShareViewFromStateRow(row billSplitShareStateRow) BillSplitShareView {
	claimedItemIDs := []string{}
	if row.ClaimedItemIDs != "" && row.ClaimedItemIDs != "[]" {
		_ = json.Unmarshal([]byte(row.ClaimedItemIDs), &claimedItemIDs)
	}
	claimedFractions := map[string]string{}
	if row.ClaimedFractions != "" && row.ClaimedFractions != "{}" {
		_ = json.Unmarshal([]byte(row.ClaimedFractions), &claimedFractions)
	}
	return BillSplitShareView{
		ID:                   row.ID,
		GuestSessionID:       row.GuestSessionID,
		DisplayName:          row.DisplayName,
		Mode:                 row.Mode,
		ClaimedItemIDs:       claimedItemIDs,
		ClaimedFractions:     claimedFractions,
		AmountCents:          row.AmountCents,
		TipCents:             row.TipCents,
		Status:               row.Status,
		HoldExpiresAt:        row.HoldExpiresAt,
		Tender:               row.Tender,
		PaymentID:            row.PaymentID,
		AlternativePaymentID: row.AlternativePaymentID,
		SettledAt:            row.SettledAt,
		ReleasedAt:           row.ReleasedAt,
	}
}

// billSplitStateFromRows is the pure, in-memory tail of split-state derivation:
// given a bill projection and its held/settled share rows (already ordered
// created_at ASC, id ASC), it computes held/available cents and builds the
// BillSplitState. It performs no DB access, so it can be reused by both the
// single-bill loader (billSplitStateFromTx) and the batched expired-hold
// sweeper, keeping cents math byte-identical across paths.
func billSplitStateFromRows(bill Bill, rows []billSplitShareStateRow, now time.Time) *BillSplitState {
	heldCents := int64(0)
	views := make([]BillSplitShareView, 0, len(rows))
	for _, row := range rows {
		if row.Status == BillSplitShareStatusHeld && (row.HoldExpiresAt == nil || row.HoldExpiresAt.After(now)) {
			heldCents += row.AmountCents
		}
		views = append(views, billSplitShareViewFromStateRow(row))
	}

	available := bill.TotalAmount - bill.PaidAmount - heldCents
	if available < 0 {
		available = 0
	}
	return &BillSplitState{
		BillID:         bill.ID,
		BusinessID:     bill.BusinessID,
		BillNumber:     bill.BillNumber,
		Status:         bill.Status,
		TotalCents:     bill.TotalAmount,
		PaidCents:      bill.PaidAmount,
		HeldCents:      heldCents,
		AvailableCents: available,
		Shares:         views,
		UpdatedAt:      bill.UpdatedAt,
	}
}

func billSplitStateFromTx(tx *gorm.DB, bill Bill, now time.Time) (*BillSplitState, error) {
	var rows []billSplitShareStateRow
	if err := tx.Table("bill_split_shares").
		Select("id", "guest_session_id", "display_name", "mode", "claimed_item_ids", "claimed_fractions", "amount_cents", "tip_cents", "status", "hold_expires_at", "tender", "payment_id", "alternative_payment_id", "settled_at", "released_at").
		Where("bill_id = ? AND status IN ?", bill.ID, []BillSplitShareStatus{BillSplitShareStatusHeld, BillSplitShareStatusSettled}).
		Order("created_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return billSplitStateFromRows(bill, rows, now), nil
}

func loadProjectedBillForSplitTx(tx *gorm.DB, billID uint, lock bool) (Bill, error) {
	var bill Bill
	query := tx.Model(&Bill{}).Select(
		"id",
		"business_id",
		"bill_number",
		"subtotal",
		"tax_amount",
		"service_fee_amount",
		"total_amount",
		"paid_amount",
		"tip_amount",
		"status",
		"updated_at",
		"closed_at",
		"closed_by_staff_id",
	)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&bill, billID).Error; err != nil {
		return Bill{}, err
	}
	return bill, nil
}

// loadProjectedBillsForSplitTx batch-loads the same projection as
// loadProjectedBillForSplitTx for many bills in a single WHERE id IN (...)
// query, keyed by bill ID. Used by the expired-hold sweeper to avoid a per-bill
// N+1 when rebuilding SSE broadcast states. An empty input issues no query.
func loadProjectedBillsForSplitTx(tx *gorm.DB, billIDs []uint) (map[uint]Bill, error) {
	result := make(map[uint]Bill, len(billIDs))
	if len(billIDs) == 0 {
		return result, nil
	}
	var bills []Bill
	if err := tx.Model(&Bill{}).Select(
		"id",
		"business_id",
		"bill_number",
		"subtotal",
		"tax_amount",
		"service_fee_amount",
		"total_amount",
		"paid_amount",
		"tip_amount",
		"status",
		"updated_at",
		"closed_at",
		"closed_by_staff_id",
	).Where("id IN ?", billIDs).Find(&bills).Error; err != nil {
		return nil, err
	}
	for _, bill := range bills {
		result[bill.ID] = bill
	}
	return result, nil
}

// loadProjectedBillSplitStateRowsTx batch-loads held/settled share rows for many
// bills in a single WHERE bill_id IN (...) query, grouped by bill ID. The
// global ORDER BY (bill_id, created_at, id) preserves the per-bill created_at
// ASC, id ASC ordering that billSplitStateFromTx relies on, so each group is
// ordered identically to the single-bill read. An empty input issues no query.
func loadProjectedBillSplitStateRowsTx(tx *gorm.DB, billIDs []uint) (map[uint][]billSplitShareStateRow, error) {
	result := make(map[uint][]billSplitShareStateRow, len(billIDs))
	if len(billIDs) == 0 {
		return result, nil
	}
	type rowWithBill struct {
		BillID               uint
		ID                   uint
		GuestSessionID       string
		DisplayName          string
		Mode                 BillSplitMode
		ClaimedItemIDs       string
		ClaimedFractions     string
		AmountCents          int64
		TipCents             int64
		Status               BillSplitShareStatus
		HoldExpiresAt        *time.Time
		Tender               string
		PaymentID            *uint
		AlternativePaymentID *uint
		SettledAt            *time.Time
		ReleasedAt           *time.Time
	}
	var rows []rowWithBill
	if err := tx.Table("bill_split_shares").
		Select("bill_id", "id", "guest_session_id", "display_name", "mode", "claimed_item_ids", "claimed_fractions", "amount_cents", "tip_cents", "status", "hold_expires_at", "tender", "payment_id", "alternative_payment_id", "settled_at", "released_at").
		Where("bill_id IN ? AND status IN ?", billIDs, []BillSplitShareStatus{BillSplitShareStatusHeld, BillSplitShareStatusSettled}).
		Order("bill_id ASC, created_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.BillID] = append(result[row.BillID], billSplitShareStateRow{
			ID:                   row.ID,
			GuestSessionID:       row.GuestSessionID,
			DisplayName:          row.DisplayName,
			Mode:                 row.Mode,
			ClaimedItemIDs:       row.ClaimedItemIDs,
			ClaimedFractions:     row.ClaimedFractions,
			AmountCents:          row.AmountCents,
			TipCents:             row.TipCents,
			Status:               row.Status,
			HoldExpiresAt:        row.HoldExpiresAt,
			Tender:               row.Tender,
			PaymentID:            row.PaymentID,
			AlternativePaymentID: row.AlternativePaymentID,
			SettledAt:            row.SettledAt,
			ReleasedAt:           row.ReleasedAt,
		})
	}
	return result, nil
}

func HoldBillSplitShare(input HoldBillSplitShareInput) (*BillSplitShare, *BillSplitState, error) {
	now := billSplitNow(input.Now)
	dynamicEqualSplit := input.Mode == BillSplitModeEqual && (input.NumPeople != 0 || input.SharesCovered != 0)
	if !validBillSplitMode(input.Mode) {
		return nil, nil, ErrInvalidSplitMode
	}
	if strings.TrimSpace(input.GuestSessionID) == "" {
		return nil, nil, ErrSplitGuestMismatch
	}
	if input.CoverRemaining && input.Mode != BillSplitModeCustom {
		return nil, nil, ErrInvalidSplitMode
	}
	if input.Mode != BillSplitModeItems && !dynamicEqualSplit && input.AmountCents <= 0 && !input.CoverRemaining {
		return nil, nil, ErrInvalidPaymentAmount
	}
	if input.TipCents < 0 {
		return nil, nil, ErrInvalidTipAmount
	}
	ttl := input.HoldTTL
	if ttl <= 0 {
		ttl = defaultBillSplitHoldTTL
	}
	expiresAt := now.Add(ttl)

	var created *BillSplitShare
	var state *BillSplitState
	err := db.Transaction(func(tx *gorm.DB) error {
		bill, err := loadProjectedBillForSplitTx(tx, input.BillID, true)
		if err != nil {
			return err
		}
		if bill.Status == BillStatusPaid || bill.Status == BillStatusClosed || bill.Status == BillStatusVoided {
			return ErrBillNotPayable
		}
		if _, err := releaseExpiredBillSplitSharesTx(tx, now, &bill.ID); err != nil {
			return err
		}
		amountCents := input.AmountCents
		claimedItemIDs := input.ClaimedItemIDs
		claimedFractions := input.ClaimedFractions
		if input.Mode == BillSplitModeItems {
			var err error
			amountCents, claimedItemIDs, claimedFractions, err = calculateItemSplitHoldAmountTx(tx, bill, input, now)
			if err != nil {
				return err
			}
		}
		if idempotencyKey := normalizeSplitIdempotencyKey(input.IdempotencyKey); idempotencyKey != "" {
			var existing BillSplitShare
			err := tx.Where("bill_id = ? AND guest_session_id = ? AND idempotency_key = ? AND status IN ?",
				bill.ID,
				strings.TrimSpace(input.GuestSessionID),
				idempotencyKey,
				[]BillSplitShareStatus{BillSplitShareStatusHeld, BillSplitShareStatusSettled},
			).First(&existing).Error
			if err == nil {
				if existing.Mode != input.Mode ||
					(!input.CoverRemaining && !dynamicEqualSplit && input.Mode != BillSplitModeCustom && existing.AmountCents != amountCents) ||
					(input.Mode == BillSplitModeCustom && !input.CoverRemaining && amountCents < existing.AmountCents) {
					return ErrPaymentTxHashConflict
				}
				created = &existing
				nextState, err := billSplitStateFromTx(tx, bill, now)
				if err != nil {
					return err
				}
				state = nextState
				return nil
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		heldCents, err := activeHeldSplitCentsTx(tx, bill.ID, now)
		if err != nil {
			return err
		}
		ordinaryPendingCents, err := activeOrdinaryPendingAlternativePaymentCentsTx(tx, bill.ID, now)
		if err != nil {
			return err
		}
		available := bill.TotalAmount - bill.PaidAmount - heldCents - ordinaryPendingCents
		if dynamicEqualSplit {
			// PG-23: re-split the *current* available balance across people who
			// have not yet claimed a seat. Sequential covered=1 claims then sum
			// exactly; simultaneous previews (no seats taken) each see floor
			// amounts so N covered=1 previews no longer over-sum.
			seatsTaken, seatErr := equalSplitSeatsTakenTx(tx, bill.ID, now)
			if seatErr != nil {
				return seatErr
			}
			remainingPeople := input.NumPeople - seatsTaken
			if remainingPeople < input.SharesCovered {
				return ErrInvalidPaymentAmount
			}
			var err error
			amountCents, err = calculateEqualSplitCents(available, remainingPeople, input.SharesCovered)
			if err != nil {
				return err
			}
		}
		if input.CoverRemaining {
			amountCents = available
		}
		if input.Mode == BillSplitModeCustom && !input.CoverRemaining && amountCents > available {
			amountCents = available
		}
		if amountCents <= 0 {
			return ErrSplitAmountUnavailable
		}
		if amountCents > available {
			return ErrSplitAmountUnavailable
		}

		share := &BillSplitShare{
			BillID:           bill.ID,
			GuestSessionID:   strings.TrimSpace(input.GuestSessionID),
			DisplayName:      strings.TrimSpace(input.DisplayName),
			Mode:             input.Mode,
			ClaimedItemIDs:   claimedItemIDs,
			ClaimedFractions: claimedFractions,
			AmountCents:      amountCents,
			TipCents:         input.TipCents,
			Status:           BillSplitShareStatusHeld,
			HoldExpiresAt:    &expiresAt,
			IdempotencyKey:   normalizeSplitIdempotencyKey(input.IdempotencyKey),
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if share.ClaimedItemIDs == nil {
			share.ClaimedItemIDs = []string{}
		}
		if share.ClaimedFractions == nil {
			share.ClaimedFractions = map[string]string{}
		}
		// Persist seat count inside the existing JSON column so the next equal
		// claim can re-split remaining balance (PG-23, no schema change).
		if dynamicEqualSplit {
			share.ClaimedFractions[equalShareMetaKey] = strconv.Itoa(input.SharesCovered)
		}
		if err := tx.Create(share).Error; err != nil {
			return err
		}
		created = share
		refreshedBill, err := loadProjectedBillForSplitTx(tx, bill.ID, false)
		if err != nil {
			return err
		}
		state, err = billSplitStateFromTx(tx, refreshedBill, now)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return created, state, nil
}

func ExtendBillSplitShareHoldForGuest(shareID, billID uint, guestSessionID string, now time.Time, ttl time.Duration) (*BillSplitShare, error) {
	guestSessionID = strings.TrimSpace(guestSessionID)
	if guestSessionID == "" {
		return nil, ErrSplitGuestMismatch
	}
	return extendBillSplitShareHold(shareID, billID, guestSessionID, now, ttl)
}

func extendBillSplitShareHold(shareID, billID uint, guestSessionID string, now time.Time, ttl time.Duration) (*BillSplitShare, error) {
	now = billSplitNow(now)
	if ttl <= 0 {
		ttl = defaultBillSplitHoldTTL
	}
	guestSessionID = strings.TrimSpace(guestSessionID)
	expiresAt := now.Add(ttl)

	var share BillSplitShare
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "bill_id", "guest_session_id", "display_name", "amount_cents", "tip_cents", "status", "hold_expires_at").
			First(&share, shareID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSplitShareNotFound
			}
			return err
		}
		if share.BillID != billID {
			return ErrSplitShareNotFound
		}
		if guestSessionID != "" && share.GuestSessionID != guestSessionID {
			return ErrSplitGuestMismatch
		}
		if share.Status != BillSplitShareStatusHeld {
			return ErrSplitShareAlreadyFinal
		}
		if share.HoldExpiresAt != nil && !share.HoldExpiresAt.After(now) {
			return ErrSplitHoldExpired
		}
		if share.HoldExpiresAt == nil || !share.HoldExpiresAt.Before(expiresAt) {
			return nil
		}

		if err := tx.Model(&BillSplitShare{}).
			Where("id = ? AND bill_id = ? AND status = ?", share.ID, share.BillID, BillSplitShareStatusHeld).
			Updates(map[string]interface{}{
				"hold_expires_at": expiresAt,
				"updated_at":      now,
			}).Error; err != nil {
			return err
		}
		share.HoldExpiresAt = &expiresAt
		share.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &share, nil
}

func ReleaseBillSplitShare(input ReleaseBillSplitShareInput) (*BillSplitShare, *BillSplitState, error) {
	now := billSplitNow(input.Now)
	if input.ShareID == 0 || input.BillID == 0 {
		return nil, nil, ErrSplitShareNotFound
	}
	guestSessionID := strings.TrimSpace(input.GuestSessionID)
	if guestSessionID == "" {
		return nil, nil, ErrSplitGuestMismatch
	}

	var releasedShare *BillSplitShare
	var state *BillSplitState
	err := db.Transaction(func(tx *gorm.DB) error {
		var share BillSplitShare
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&share, input.ShareID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSplitShareNotFound
			}
			return err
		}
		if share.BillID != input.BillID {
			return ErrSplitShareNotFound
		}
		if share.GuestSessionID != guestSessionID {
			return ErrSplitGuestMismatch
		}
		if share.Status == BillSplitShareStatusSettled {
			return ErrSplitShareAlreadyFinal
		}
		if share.Status == BillSplitShareStatusReleased {
			releasedShare = &share
		} else if share.Status == BillSplitShareStatusHeld {
			share.Status = BillSplitShareStatusReleased
			share.ReleasedAt = &now
			share.HoldExpiresAt = nil
			share.UpdatedAt = now
			if err := tx.Model(&BillSplitShare{}).Where("id = ?", share.ID).Updates(map[string]interface{}{
				"status":          share.Status,
				"released_at":     share.ReleasedAt,
				"hold_expires_at": gorm.Expr("NULL"),
				"updated_at":      share.UpdatedAt,
			}).Error; err != nil {
				return err
			}
			releasedShare = &share
		} else {
			return ErrSplitShareAlreadyFinal
		}

		bill, err := loadProjectedBillForSplitTx(tx, input.BillID, false)
		if err != nil {
			return err
		}
		state, err = billSplitStateFromTx(tx, bill, now)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return releasedShare, state, nil
}

func ReleaseExpiredBillSplitSharesWithStates(now time.Time) (int64, []*BillSplitState, error) {
	released := int64(0)
	states := []*BillSplitState{}
	now = billSplitNow(now)
	err := db.Transaction(func(tx *gorm.DB) error {
		var billIDs []uint
		if err := tx.Model(&BillSplitShare{}).
			Distinct("bill_id").
			Where("status = ? AND hold_expires_at IS NOT NULL AND hold_expires_at <= ?", BillSplitShareStatusHeld, now).
			Pluck("bill_id", &billIDs).Error; err != nil {
			return err
		}
		count, err := releaseExpiredBillSplitSharesTx(tx, now, nil)
		if err != nil {
			return err
		}
		released = count

		// Rebuild the SSE broadcast states with two set-based reads instead of a
		// per-bill N+1: one bills WHERE id IN (...) + one shares WHERE bill_id IN
		// (...), grouped in Go. The release above already happened unconditionally
		// in the single set UPDATE, so the states here are a best-effort notify.
		billMap, err := loadProjectedBillsForSplitTx(tx, billIDs)
		if err != nil {
			return err
		}
		sharesByBill, err := loadProjectedBillSplitStateRowsTx(tx, billIDs)
		if err != nil {
			return err
		}
		for _, billID := range billIDs {
			bill, ok := billMap[billID]
			if !ok {
				// Anomalous: a share referencing a vanished bill. The release
				// already ran in the set UPDATE; skip the state (best-effort
				// notify) rather than aborting the whole sweep. Cannot lose money.
				continue
			}
			states = append(states, billSplitStateFromRows(bill, sharesByBill[billID], now))
		}
		return nil
	})
	return released, states, err
}

// GetBillSplitStateByNumber resolves guest split state by public_token capability.
// The parameter is the guest capability token (route :bill_token), not bill_number.
func GetBillSplitStateByNumber(token string, now time.Time) (*BillSplitState, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, gorm.ErrRecordNotFound
	}
	now = billSplitNow(now)
	var state *BillSplitState
	err := db.Transaction(func(tx *gorm.DB) error {
		var bill Bill
		if err := tx.Model(&Bill{}).
			Select("bills.id", "bills.business_id", "bills.bill_number", "bills.total_amount", "bills.paid_amount", "bills.status", "bills.updated_at").
			Joins("JOIN businesses ON businesses.id = bills.business_id").
			Where(PublicBillTokenWhere+" AND businesses.is_active = ?", token, true).
			Take(&bill).Error; err != nil {
			return err
		}
		if _, err := releaseExpiredBillSplitSharesTx(tx, now, &bill.ID); err != nil {
			return err
		}
		nextState, err := billSplitStateFromTx(tx, bill, now)
		if err != nil {
			return err
		}
		state = nextState
		return nil
	})
	return state, err
}

// GetBillSplitStateByBillID loads split state for a bill already resolved by
// an authenticated or server-side path (payment settlement SSE, etc.).
func GetBillSplitStateByBillID(billID uint, now time.Time) (*BillSplitState, error) {
	if billID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	now = billSplitNow(now)
	var state *BillSplitState
	err := db.Transaction(func(tx *gorm.DB) error {
		var bill Bill
		if err := tx.Model(&Bill{}).
			Select("bills.id", "bills.business_id", "bills.bill_number", "bills.total_amount", "bills.paid_amount", "bills.status", "bills.updated_at").
			Joins("JOIN businesses ON businesses.id = bills.business_id").
			Where("bills.id = ? AND businesses.is_active = ?", billID, true).
			Take(&bill).Error; err != nil {
			return err
		}
		if _, err := releaseExpiredBillSplitSharesTx(tx, now, &bill.ID); err != nil {
			return err
		}
		nextState, err := billSplitStateFromTx(tx, bill, now)
		if err != nil {
			return err
		}
		state = nextState
		return nil
	})
	return state, err
}

// GetGuestBillSplitSharesByNumber resolves guest shares by public_token capability.
// The first parameter is the guest capability token (route :bill_token).
func GetGuestBillSplitSharesByNumber(token string, guestSessionID string, now time.Time) ([]BillSplitShareView, error) {
	token = strings.TrimSpace(token)
	guestSessionID = strings.TrimSpace(guestSessionID)
	if token == "" {
		return nil, ErrSplitShareNotFound
	}
	if guestSessionID == "" {
		return []BillSplitShareView{}, nil
	}
	now = billSplitNow(now)

	shares := []BillSplitShareView{}
	err := db.Transaction(func(tx *gorm.DB) error {
		var bill Bill
		if err := tx.Model(&Bill{}).
			Select("bills.id").
			Joins("JOIN businesses ON businesses.id = bills.business_id").
			Where(PublicBillTokenWhere+" AND businesses.is_active = ?", token, true).
			Take(&bill).Error; err != nil {
			return err
		}
		if _, err := releaseExpiredBillSplitSharesTx(tx, now, &bill.ID); err != nil {
			return err
		}

		var rows []billSplitShareStateRow
		if err := tx.Table("bill_split_shares").
			Select("id", "guest_session_id", "display_name", "mode", "claimed_item_ids", "claimed_fractions", "amount_cents", "tip_cents", "status", "hold_expires_at", "tender", "payment_id", "alternative_payment_id", "settled_at", "released_at").
			Where("bill_id = ? AND guest_session_id = ? AND status IN ?", bill.ID, guestSessionID, []BillSplitShareStatus{BillSplitShareStatusHeld, BillSplitShareStatusSettled, BillSplitShareStatusReleased, BillSplitShareStatusFailed}).
			Order("created_at DESC, id DESC").
			Limit(20).
			Find(&rows).Error; err != nil {
			return err
		}
		shares = make([]BillSplitShareView, 0, len(rows))
		for _, row := range rows {
			shares = append(shares, billSplitShareViewFromStateRow(row))
		}
		return nil
	})
	return shares, err
}

func billSplitFloatDollarsToCents(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

// splitShareChargeBreakdown splits a share's charge into tax, service fee and
// subtotal in proportion to the bill total. The three parts always sum to
// amountCents.
func splitShareChargeBreakdown(bill Bill, amountCents int64) (taxCents, serviceCents, subtotalCents int64) {
	if amountCents <= 0 {
		return 0, 0, 0
	}
	taxCents = prorateCentsBySubtotal(bill.TaxAmount, amountCents, bill.TotalAmount)
	serviceCents = prorateCentsBySubtotal(bill.ServiceFeeAmount, amountCents, bill.TotalAmount)
	if taxCents > amountCents {
		taxCents = amountCents
	}
	if taxCents+serviceCents > amountCents {
		serviceCents = amountCents - taxCents
	}
	return taxCents, serviceCents, amountCents - taxCents - serviceCents
}

// rescaleSplitReceiptLines scales claimed-line subtotals so they sum exactly
// to subtotalCents. The rounding remainder goes to the last line.
func rescaleSplitReceiptLines(lines []BillSplitReceiptItem, subtotalCents int64) {
	if len(lines) == 0 {
		return
	}
	var lineSum int64
	for _, line := range lines {
		lineSum += line.SubtotalCents
	}
	if lineSum == subtotalCents {
		return
	}
	var assigned int64
	for i := range lines[:len(lines)-1] {
		scaled := int64(0)
		if lineSum > 0 {
			scaled = new(big.Int).Div(
				new(big.Int).Mul(big.NewInt(lines[i].SubtotalCents), big.NewInt(subtotalCents)),
				big.NewInt(lineSum),
			).Int64()
		}
		lines[i].SubtotalCents = scaled
		assigned += scaled
	}
	lines[len(lines)-1].SubtotalCents = subtotalCents - assigned
}

func buildBillSplitShareReceipt(bill Bill, items []BillItem, share BillSplitShare) (*BillSplitShareReceipt, error) {
	receiptItems := []BillSplitReceiptItem{}

	if share.Mode == BillSplitModeItems {
		itemsByID := make(map[string]BillItem, len(items))
		for _, item := range items {
			itemsByID[item.ID] = item
		}
		for _, itemID := range share.ClaimedItemIDs {
			item, ok := itemsByID[itemID]
			if !ok {
				continue
			}
			fractionText := strings.TrimSpace(share.ClaimedFractions[itemID])
			if fractionText == "" {
				fractionText = "1"
			}
			fraction, err := parseSplitFraction(fractionText)
			if err != nil {
				return nil, fmt.Errorf("invalid stored split fraction: %w", err)
			}
			lineSubtotal := prorateCentsByFraction(billSplitFloatDollarsToCents(item.Subtotal), fraction)
			receiptItems = append(receiptItems, BillSplitReceiptItem{
				ID:             item.ID,
				Name:           item.Name,
				Fraction:       splitFractionToString(fraction),
				Quantity:       item.Quantity,
				UnitPriceCents: billSplitFloatDollarsToCents(item.Price),
				SubtotalCents:  lineSubtotal,
			})
		}
	}

	// The charge is fixed (share.AmountCents); the breakdown is derived from it
	// so subtotal + tax + service always equals what the guest paid. Tax and
	// service are the share's proportion of the bill total, clamped so they
	// never exceed the charge.
	taxCents, serviceCents, subtotalCents := splitShareChargeBreakdown(bill, share.AmountCents)
	if share.Mode == BillSplitModeItems {
		rescaleSplitReceiptLines(receiptItems, subtotalCents)
	}

	return &BillSplitShareReceipt{
		ShareID:              share.ID,
		BillNumber:           bill.BillNumber,
		DisplayName:          share.DisplayName,
		Mode:                 share.Mode,
		Status:               share.Status,
		Tender:               share.Tender,
		PaymentID:            share.PaymentID,
		AlternativePaymentID: share.AlternativePaymentID,
		SubtotalCents:        subtotalCents,
		TaxCents:             taxCents,
		ServiceFeeCents:      serviceCents,
		AmountCents:          share.AmountCents,
		TipCents:             share.TipCents,
		GrandTotalCents:      share.AmountCents + share.TipCents,
		Items:                receiptItems,
		SettledAt:            share.SettledAt,
	}, nil
}

func GetGuestBillSplitShareReceipt(token string, shareID uint, guestSessionID string) (*BillSplitShareReceipt, error) {
	token = strings.TrimSpace(token)
	guestSessionID = strings.TrimSpace(guestSessionID)
	if token == "" || shareID == 0 {
		return nil, ErrSplitShareNotFound
	}
	if guestSessionID == "" {
		return nil, ErrSplitGuestMismatch
	}

	bill, items, err := GetPublicBillByToken(token)
	if err != nil {
		return nil, err
	}

	var share BillSplitShare
	if err := db.Model(&BillSplitShare{}).
		Select(
			"id",
			"bill_id",
			"guest_session_id",
			"display_name",
			"mode",
			"claimed_item_ids",
			"claimed_fractions",
			"amount_cents",
			"tip_cents",
			"status",
			"tender",
			"payment_id",
			"alternative_payment_id",
			"settled_at",
		).
		Where("id = ? AND bill_id = ?", shareID, bill.ID).
		Take(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSplitShareNotFound
		}
		return nil, fmt.Errorf("failed to load split share receipt: %w", err)
	}

	if share.GuestSessionID != guestSessionID {
		return nil, ErrSplitGuestMismatch
	}
	if share.Status != BillSplitShareStatusSettled {
		return nil, ErrSplitReceiptUnavailable
	}

	return buildBillSplitShareReceipt(*bill, items, share)
}

func SettleBillSplitShare(input SettleBillSplitShareInput) (*BillSplitShare, *Bill, bool, error) {
	now := billSplitNow(input.Now)
	idempotencyKey := normalizeSplitIdempotencyKey(input.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = normalizeSplitIdempotencyKey(input.TxHash)
	}
	if idempotencyKey == "" {
		return nil, nil, false, ErrInvalidIdempotencyKey
	}
	tender, err := normalizeSplitTender(input.Tender)
	if err != nil {
		return nil, nil, false, err
	}

	var settledShare *BillSplitShare
	var updatedBill *Bill
	applied := false

	err = db.Transaction(func(tx *gorm.DB) error {
		var share BillSplitShare
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&share, input.ShareID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSplitShareNotFound
			}
			return err
		}
		if share.GuestSessionID != strings.TrimSpace(input.GuestSessionID) {
			return ErrSplitGuestMismatch
		}
		if share.Status == BillSplitShareStatusSettled {
			if share.SettlementIdempotencyKey == idempotencyKey {
				if input.RefundDestination != nil && share.PaymentID != nil {
					dest := *input.RefundDestination
					dest.PaymentID = *share.PaymentID
					if err := persistPaymentRefundDestinationTx(tx, &dest); err != nil {
						return fmt.Errorf("failed to persist refund destination: %w", err)
					}
				}
				bill, err := loadProjectedBillForSplitTx(tx, share.BillID, false)
				if err != nil {
					return err
				}
				settledShare = &share
				updatedBill = &bill
				applied = false
				return nil
			}
			return ErrSplitShareAlreadyFinal
		}
		if share.Status != BillSplitShareStatusHeld {
			return ErrSplitShareAlreadyFinal
		}
		if share.HoldExpiresAt != nil && !share.HoldExpiresAt.After(now) {
			if err := tx.Model(&BillSplitShare{}).Where("id = ?", share.ID).Updates(map[string]interface{}{
				"status":      BillSplitShareStatusReleased,
				"released_at": now,
				"updated_at":  now,
			}).Error; err != nil {
				return err
			}
			return ErrSplitHoldExpired
		}

		tipCents := share.TipCents
		if input.TipCents > 0 {
			tipCents = input.TipCents
		}

		switch tender {
		case "cash", "card", "venmo", "other":
			bill, err := loadProjectedBillForSplitTx(tx, share.BillID, true)
			if err != nil {
				return err
			}
			if err := applyBillPaymentAmounts(tx, &bill, share.AmountCents, tipCents, nil, now); err != nil {
				return err
			}
			method := AlternativePaymentMethod(tender)
			alt := &AlternativePayment{
				BillID:          share.BillID,
				ParticipantAddr: strings.TrimSpace(input.PayerAddr),
				ParticipantName: share.DisplayName,
				Amount:          share.AmountCents,
				BillAmountCents: share.AmountCents,
				TipAmountCents:  tipCents,
				PaymentMethod:   method,
				Status:          AltPaymentStatusConfirmed,
				ConfirmedAt:     &now,
				CreatedAt:       now,
				UpdatedAt:       now,
				// The share holder's session paid this share: payment proof
				// for the guest fiscal identity binding.
				PayerGuestSession: guestsession.FingerprintPtr(share.GuestSessionID),
			}
			if alt.ParticipantAddr == "" {
				alt.ParticipantAddr = share.GuestSessionID
			}
			if err := tx.Create(alt).Error; err != nil {
				return err
			}
			if method == PaymentMethodCash {
				if err := AttachCashRegisterMovementForAlternativePaymentTx(
					tx,
					bill.BusinessID,
					*alt,
					CashRegisterMovementTypeCashSale,
					share.AmountCents+tipCents,
					CashRegisterActor{Label: "system:split_cash_payment"},
					now,
				); err != nil {
					return err
				}
			}
			previousPaidBeforeShare := bill.PaidAmount - share.AmountCents
			recognizedBillDelta := int64(0)
			if previousPaidBeforeShare <= 0 && bill.PaidAmount > 0 {
				recognizedBillDelta = 1
			}
			if err := RecordPaymentMilestonesTx(tx, bill.BusinessID, bill.ID, share.AmountCents, tipCents, recognizedBillDelta, bill.Status); err != nil {
				return err
			}
			// Fiscal transactional outbox (Wave 4): this branch flips the
			// bill to paid via applyBillPaymentAmounts directly, bypassing
			// applyConfirmedPaymentTx — without this call a bill fully
			// settled by cash/card splits never gets a factura. The hook
			// only fires on Status==paid (transition-safe: a terminal-paid
			// bill is rejected by applyBillPaymentAmounts above).
			if err := runBillPaidInTxHook(tx, &bill, nil, &alt.ID); err != nil {
				return fmt.Errorf("failed to enqueue fiscal outbox job: %w", err)
			}
			share.AlternativePaymentID = &alt.ID
			updatedBill = &bill
			applied = true
		default:
			txHash := strings.TrimSpace(input.TxHash)
			generatedTxHash := false
			if txHash == "" {
				txHash = splitSyntheticTxHash(share.ID, idempotencyKey, tender)
				generatedTxHash = true
			}
			payerAddr := strings.TrimSpace(input.PayerAddr)
			if payerAddr == "" {
				payerAddr = "split_guest"
			}
			bill, payment, wasApplied, err := applyConfirmedPaymentTx(tx, ConfirmedPaymentInput{
				BillID:            share.BillID,
				PayerAddr:         payerAddr,
				Amount:            share.AmountCents,
				TipAmount:         tipCents,
				TxHash:            txHash,
				Status:            PaymentStatusConfirmed,
				PaymentMethod:     tender,
				SourceChain:       input.SourceChain,
				SourceToken:       input.SourceToken,
				SettlementChain:   "base",
				LifiRouteID:       input.LifiRouteID,
				RefundDestination: input.RefundDestination,
				BlockNumber:       input.BlockNumber,
				BlockHash:         input.BlockHash,

				CryptoQuoteID:              input.CryptoQuoteID,
				CryptoQuoteExactMicrounits: input.CryptoQuoteExactMicrounits,
				PayerGuestSession:          guestsession.FingerprintPtr(share.GuestSessionID),
			}, nil, txHash, generatedTxHash, now)
			if err != nil {
				return err
			}
			share.PaymentID = &payment.ID
			updatedBill = bill
			applied = wasApplied
		}

		share.Status = BillSplitShareStatusSettled
		share.TipCents = tipCents
		share.Tender = tender
		share.SettlementIdempotencyKey = idempotencyKey
		share.SettledAt = &now
		share.UpdatedAt = now
		if err := tx.Model(&BillSplitShare{}).Where("id = ?", share.ID).Updates(map[string]interface{}{
			"status":                     share.Status,
			"tip_cents":                  share.TipCents,
			"tender":                     share.Tender,
			"settlement_idempotency_key": share.SettlementIdempotencyKey,
			"payment_id":                 share.PaymentID,
			"alternative_payment_id":     share.AlternativePaymentID,
			"settled_at":                 share.SettledAt,
			"updated_at":                 share.UpdatedAt,
		}).Error; err != nil {
			return err
		}
		settledShare = &share
		return nil
	})
	if err != nil {
		return nil, nil, false, err
	}
	return settledShare, updatedBill, applied, nil
}

func MarkBillSplitShareSettledByPayment(input MarkBillSplitShareSettledByPaymentInput) (*BillSplitShare, error) {
	now := billSplitNow(input.Now)
	if input.ShareID == 0 || input.BillID == 0 || input.PaymentID == 0 {
		return nil, ErrSplitShareNotFound
	}
	if input.TipCents < 0 {
		return nil, ErrInvalidTipAmount
	}
	tender := strings.TrimSpace(input.Tender)
	if tender == "" {
		tender = "plugin"
	}
	idempotencyKey := normalizeSplitIdempotencyKey(input.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("payment:%d", input.PaymentID)
	}

	var settled *BillSplitShare
	err := db.Transaction(func(tx *gorm.DB) error {
		var share BillSplitShare
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&share, input.ShareID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSplitShareNotFound
			}
			return err
		}
		if share.BillID != input.BillID {
			return ErrAlternativePaymentBillMismatch
		}
		if share.Status == BillSplitShareStatusSettled {
			if share.PaymentID != nil && *share.PaymentID == input.PaymentID {
				settled = &share
				return nil
			}
			return ErrSplitShareAlreadyFinal
		}
		if share.Status != BillSplitShareStatusHeld && share.Status != BillSplitShareStatusReleased {
			return ErrSplitShareAlreadyFinal
		}

		// F6: enforce the same amount==share guard the cash/alternative-payment
		// path applies (lockBillSplitShareForAlternativePaymentTx). The Payment's
		// bill portion (Payment.Amount excludes tip) must exactly cover the
		// share's owed amount — a payment for a different amount must not be
		// allowed to close this share (it would under/over-pay it while marking it
		// settled). Zero-amount lookups (payment missing) fall through as a
		// mismatch so we never settle a share against a phantom payment.
		var payment Payment
		if err := tx.Select("id", "amount").First(&payment, input.PaymentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSplitShareNotFound
			}
			return err
		}
		if payment.Amount != share.AmountCents {
			return ErrSplitAmountUnavailable
		}

		var siblingSettled int64
		if err := tx.Model(&BillSplitShare{}).
			Where("bill_id = ? AND payment_id = ? AND status = ? AND id <> ?",
				input.BillID, input.PaymentID, BillSplitShareStatusSettled, input.ShareID).
			Count(&siblingSettled).Error; err != nil {
			return err
		}
		if siblingSettled > 0 {
			return ErrPaymentAlreadySettledForBill
		}

		share.Status = BillSplitShareStatusSettled
		share.TipCents = input.TipCents
		share.Tender = tender
		share.PaymentID = &input.PaymentID
		share.SettlementIdempotencyKey = idempotencyKey
		share.SettledAt = &now
		share.HoldExpiresAt = nil
		share.UpdatedAt = now
		if err := tx.Model(&BillSplitShare{}).Where("id = ?", share.ID).Updates(map[string]interface{}{
			"status":                     share.Status,
			"tip_cents":                  share.TipCents,
			"tender":                     share.Tender,
			"payment_id":                 share.PaymentID,
			"settlement_idempotency_key": share.SettlementIdempotencyKey,
			"settled_at":                 share.SettledAt,
			"hold_expires_at":            gorm.Expr("NULL"),
			"updated_at":                 share.UpdatedAt,
		}).Error; err != nil {
			return err
		}
		settled = &share
		return nil
	})
	return settled, err
}

func lockBillSplitShareForAlternativePaymentTx(tx *gorm.DB, payment AlternativePayment, now time.Time) (*BillSplitShare, error) {
	shareID, ok := splitShareIDFromAlternativePayment(payment)
	if !ok {
		return nil, nil
	}
	if payment.ID == 0 || payment.BillID == 0 {
		return nil, ErrSplitShareNotFound
	}

	var share BillSplitShare
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&share, shareID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSplitShareNotFound
		}
		return nil, err
	}
	if share.BillID != payment.BillID {
		return nil, ErrAlternativePaymentBillMismatch
	}
	if share.Status == BillSplitShareStatusSettled {
		if share.AlternativePaymentID != nil && *share.AlternativePaymentID == payment.ID {
			return &share, nil
		}
		return nil, ErrSplitShareAlreadyFinal
	}
	if share.Status != BillSplitShareStatusHeld {
		return nil, ErrSplitShareAlreadyFinal
	}
	if share.HoldExpiresAt != nil && !share.HoldExpiresAt.After(now) {
		return nil, ErrSplitHoldExpired
	}
	if share.AmountCents != payment.Amount {
		return nil, ErrSplitAmountUnavailable
	}
	return &share, nil
}

func markBillSplitShareSettledByAlternativePaymentTx(tx *gorm.DB, share *BillSplitShare, payment AlternativePayment, now time.Time) error {
	if share == nil {
		return nil
	}
	if share.Status == BillSplitShareStatusSettled {
		if share.AlternativePaymentID != nil && *share.AlternativePaymentID == payment.ID {
			return nil
		}
		return ErrSplitShareAlreadyFinal
	}

	tender := strings.TrimSpace(string(payment.PaymentMethod))
	if tender == "" {
		tender = "cash"
	}
	share.Status = BillSplitShareStatusSettled
	share.Tender = tender
	share.TipCents = payment.TipAmountCents
	share.AlternativePaymentID = &payment.ID
	share.SettlementIdempotencyKey = fmt.Sprintf("alternative:%d", payment.ID)
	share.SettledAt = &now
	share.HoldExpiresAt = nil
	share.UpdatedAt = now

	return tx.Model(&BillSplitShare{}).Where("id = ?", share.ID).Updates(map[string]interface{}{
		"status":                     share.Status,
		"tender":                     share.Tender,
		"tip_cents":                  payment.TipAmountCents,
		"alternative_payment_id":     share.AlternativePaymentID,
		"settlement_idempotency_key": share.SettlementIdempotencyKey,
		"settled_at":                 share.SettledAt,
		"hold_expires_at":            gorm.Expr("NULL"),
		"updated_at":                 share.UpdatedAt,
	}).Error
}

// releaseBillSplitShareForAlternativePaymentTx gives capacity back when a
// pending alternative-payment request is cancelled, rejected, or found
// expired. The payment row is locked before this helper is called, so a
// concurrent confirmation cannot settle the same request after the release.
func releaseBillSplitShareForAlternativePaymentTx(tx *gorm.DB, payment AlternativePayment, now time.Time) error {
	shareID, ok := splitShareIDFromAlternativePayment(payment)
	if !ok {
		return nil
	}

	var share BillSplitShare
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&share, shareID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSplitShareNotFound
		}
		return err
	}
	if share.BillID != payment.BillID {
		return ErrAlternativePaymentBillMismatch
	}
	if share.Status == BillSplitShareStatusReleased {
		return nil
	}
	if share.Status != BillSplitShareStatusHeld {
		return ErrSplitShareAlreadyFinal
	}

	share.Status = BillSplitShareStatusReleased
	share.HoldExpiresAt = nil
	share.ReleasedAt = &now
	share.UpdatedAt = now
	return tx.Model(&BillSplitShare{}).
		Where("id = ? AND status = ?", share.ID, BillSplitShareStatusHeld).
		Updates(map[string]any{
			"status":          share.Status,
			"hold_expires_at": gorm.Expr("NULL"),
			"released_at":     share.ReleasedAt,
			"updated_at":      share.UpdatedAt,
		}).Error
}

func (share BillSplitShare) MarshalJSON() ([]byte, error) {
	type Shadow BillSplitShare
	return json.Marshal(&struct {
		Shadow
		Amount float64 `json:"amount"`
		Tip    float64 `json:"tip_amount"`
	}{
		Shadow: (Shadow)(share),
		Amount: centsToDollars(share.AmountCents),
		Tip:    centsToDollars(share.TipCents),
	})
}
