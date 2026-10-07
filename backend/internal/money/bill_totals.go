package money

import (
	"math"
	"strings"
)

// CentsFromMajor converts a major-unit amount received from the JSON/API
// boundary into the platform's canonical integer-cent representation.
func CentsFromMajor(value float64) int64 {
	return int64(math.Round(value * 100))
}

// MajorFromCents converts canonical cents back to the two-decimal API shape.
// Keeping this conversion at the boundary prevents repeated float arithmetic
// from becoming part of bill-total calculations.
func MajorFromCents(cents int64) float64 {
	return float64(cents) / 100
}

// PercentageCents applies ratePercent (8.875 means 8.875%) to a cent amount.
// Rounding rule: half-up once on the final cent (18637.5 → 18638). The rate
// is kept to millipercent so 8.875 is not snapped to 8.88 first (#228/#537).
func PercentageCents(cents int64, ratePercent float64) int64 {
	milliPercent := int64(math.Round(ratePercent * 1000))
	return (cents*milliPercent + 50000) / 100000
}

// BillTotalsCents returns tax, service fee, and gross total in integer cents.
func BillTotalsCents(subtotalCents int64, taxRate, serviceFeeRate float64) (int64, int64, int64) {
	if subtotalCents < 0 {
		subtotalCents = 0
	}
	taxCents := PercentageCents(subtotalCents, taxRate)
	serviceFeeCents := PercentageCents(subtotalCents, serviceFeeRate)
	return taxCents, serviceFeeCents, subtotalCents + taxCents + serviceFeeCents
}

// OptionSurcharge is the minimal option shape needed for line-subtotal math.
// Callers pass PriceChange in major units (dollars), matching MenuItemOption.
type OptionSurcharge struct {
	PriceChange float64
}

// OptionTotalCents sums option surcharges in integer cents.
func OptionTotalCents(options []OptionSurcharge) int64 {
	var total int64
	for _, option := range options {
		total += CentsFromMajor(option.PriceChange)
	}
	return total
}

// IsInformationalBillLine reports whether a bill/order line is priced only for
// display (bundle children) and must never contribute to bill money totals.
// Discount lines are billable (negative) and are NOT informational.
func IsInformationalBillLine(itemType string) bool {
	return strings.EqualFold(strings.TrimSpace(itemType), "bundle_item")
}

// LineSubtotalCents is the single source of truth for one bill line's money
// contribution before tax/service:
//   - bundle_item → always 0 (price lives on the parent bundle line)
//   - discount → trust the provided subtotal (may be negative)
//   - otherwise → (unit price + option surcharges) × quantity
//
// existingSubtotalMajor is only used for discount lines (and as a fallback when
// quantity is non-positive so we do not invent money from a bad qty).
func LineSubtotalCents(itemType string, unitPriceMajor float64, quantity int, options []OptionSurcharge, existingSubtotalMajor float64) int64 {
	if IsInformationalBillLine(itemType) {
		return 0
	}
	if strings.EqualFold(strings.TrimSpace(itemType), "discount") {
		return CentsFromMajor(existingSubtotalMajor)
	}
	if quantity <= 0 {
		return CentsFromMajor(existingSubtotalMajor)
	}
	unitCents := CentsFromMajor(unitPriceMajor) + OptionTotalCents(options)
	return unitCents * int64(quantity)
}
