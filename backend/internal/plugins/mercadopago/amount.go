package mercadopago

import (
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/money"
)

// isMercadoPagoZeroDecimal reports currencies Mercado Pago treats as whole major
// units (no fractional part). CLP and other ISO zero-decimal codes use
// money.DecimalDigits; COP is 2-decimal in ISO/storage but MP Orders/refunds
// reject sub-unit COP amounts, so it is forced to whole units here.
func isMercadoPagoZeroDecimal(currency string) bool {
	code := strings.ToUpper(strings.TrimSpace(currency))
	if code == "COP" {
		return true
	}
	return money.DecimalDigits(code) == 0
}

// mercadoPagoWholeMajorUnits converts stored cents to whole major units for
// zero-decimal MP currencies by flooring toward zero (never rounding up).
// Flooring ensures charging the bill's remaining balance can never exceed
// remaining cents (half-up would leave "customer paid more than remaining" and
// permanent settlement rejection). For two-decimal currencies this path is not used.
func mercadoPagoWholeMajorUnits(cents int64) int64 {
	// Go integer division truncates toward zero.
	return cents / 100
}

// CanonicalAmountCents returns the tracker/order amount in platform cents that
// matches what Mercado Pago will charge. For zero-decimal currencies (CLP, COP,
// …) the value is floored to whole major units then re-expressed as cents so
// the charged/tracker amount never exceeds the pre-canonical remaining cents
// and webhook settlement matches the AlternativePayment row.
// Two-decimal currencies pass through unchanged.
func CanonicalAmountCents(cents int64, currency string) int64 {
	if !isMercadoPagoZeroDecimal(currency) {
		return cents
	}
	return mercadoPagoWholeMajorUnits(cents) * 100
}

// mercadoPagoMajorAmount converts stored cents to MP's major-unit float for
// Checkout Pro preferences and refunds. Zero-decimal (incl. COP) → whole number.
func mercadoPagoMajorAmount(cents int64, currency string) float64 {
	if isMercadoPagoZeroDecimal(currency) {
		return float64(mercadoPagoWholeMajorUnits(cents))
	}
	return float64(cents) / 100.0
}

// mercadoPagoDecimalAmount renders stored cents as the Orders API amount string.
// Two-decimal → "1500.00"; zero-decimal (CLP, COP) → whole "1500".
func mercadoPagoDecimalAmount(cents int64, currency string) string {
	if isMercadoPagoZeroDecimal(currency) {
		return strconv.FormatInt(mercadoPagoWholeMajorUnits(cents), 10)
	}
	return money.MajorUnitString(cents, currency)
}

// DecimalAmount is the exported alias used by Point/QR handlers.
func DecimalAmount(cents int64, currency string) string {
	return mercadoPagoDecimalAmount(cents, currency)
}

// ShouldAbsorbSettlementDust reports whether remaining bill cents after an MP
// settlement should be absorbed as a zero-decimal rounding adjustment.
// Bound: strictly positive, strictly below one major unit (100 platform cents),
// and only for MP zero-decimal currencies. Never a general payment tolerance.
func ShouldAbsorbSettlementDust(remainingCents int64, currency string) bool {
	return remainingCents > 0 && remainingCents < 100 && isMercadoPagoZeroDecimal(currency)
}
