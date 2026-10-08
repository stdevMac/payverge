package money

import (
	"strconv"
	"strings"
)

// Payverge stores every monetary amount as an int64 in "cents" — major units
// multiplied by 100 — regardless of currency. Payment providers, however,
// expect amounts in the currency's *actual* minor unit, which is not always
// 1/100 of the major unit: zero-decimal currencies (JPY, KRW, …) have no minor
// unit at all, and a few currencies (BHD, KWD, …) use three decimals. Passing
// the raw ×100 "cents" to a provider therefore over- or under-charges by a
// factor of 100 for those currencies (e.g. Stripe would read ¥1000, stored as
// 100000, as ¥100000). These helpers convert at the provider boundary.

// zeroDecimalCurrencies have no minor unit (ISO 4217 / provider zero-decimal
// set). An amount in these currencies is a whole number of major units.
var zeroDecimalCurrencies = map[string]struct{}{
	"BIF": {}, "CLP": {}, "DJF": {}, "GNF": {}, "ISK": {}, "JPY": {},
	"KMF": {}, "KRW": {}, "MGA": {}, "PYG": {}, "RWF": {}, "UGX": {},
	"VND": {}, "VUV": {}, "XAF": {}, "XOF": {}, "XPF": {},
}

// threeDecimalCurrencies use three decimal places (1/1000 of the major unit).
var threeDecimalCurrencies = map[string]struct{}{
	"BHD": {}, "IQD": {}, "JOD": {}, "KWD": {}, "LYD": {}, "OMR": {}, "TND": {},
}

// DecimalDigits returns the number of decimal places a currency's minor unit
// uses: 0 for zero-decimal currencies, 3 for three-decimal currencies, and 2
// for everything else (the default for unknown/empty codes).
func DecimalDigits(code string) int {
	c := strings.ToUpper(strings.TrimSpace(code))
	if _, ok := zeroDecimalCurrencies[c]; ok {
		return 0
	}
	if _, ok := threeDecimalCurrencies[c]; ok {
		return 3
	}
	return 2
}

// MinorUnits converts a stored "cents" amount (major*100) into the currency's
// actual minor unit. For 2-decimal currencies it is the identity. For
// zero-decimal currencies it divides by 100 (rounded to the nearest whole minor
// unit, since sub-unit precision can't be charged). For three-decimal
// currencies it multiplies by 10.
func MinorUnits(cents int64, code string) int64 {
	switch DecimalDigits(code) {
	case 0:
		return roundDiv(cents, 100)
	case 3:
		return cents * 10
	default: // 2 decimals: stored cents already are the minor unit.
		return cents
	}
}

// FromMinorUnits is the inverse of MinorUnits: it converts a provider's actual
// minor-unit amount (e.g. what Stripe reports in webhooks and status polling)
// back into the platform's stored "cents" (major*100). For 2-decimal currencies
// it is the identity. For zero-decimal currencies (JPY, KRW, …) it multiplies by
// 100 (¥1000 -> 100000). For three-decimal currencies (BHD, KWD, …) it divides
// by 10 (1500 mils -> 150 cents). It uses the same currency-decimals table as
// MinorUnits, so the two round-trip for whole-unit amounts.
func FromMinorUnits(minor int64, code string) int64 {
	switch DecimalDigits(code) {
	case 0:
		return minor * 100
	case 3:
		return roundDiv(minor, 10)
	default: // 2 decimals: provider minor unit already is the stored cent.
		return minor
	}
}

// MajorUnitString renders a stored "cents" amount as a decimal string in major
// units with the currency's provider-expected number of decimal places (e.g.
// "10.00" for USD, "1000" for JPY, "10.000" for BHD). It is derived from
// MinorUnits so it always agrees with the integer minor-unit amount.
func MajorUnitString(cents int64, code string) string {
	digits := DecimalDigits(code)
	minor := MinorUnits(cents, code)

	neg := minor < 0
	if neg {
		minor = -minor
	}
	s := strconv.FormatInt(minor, 10)

	var out string
	if digits == 0 {
		out = s
	} else {
		for len(s) <= digits {
			s = "0" + s
		}
		out = s[:len(s)-digits] + "." + s[len(s)-digits:]
	}
	if neg {
		out = "-" + out
	}
	return out
}

// roundDiv divides a by b rounding half away from zero (b > 0).
func roundDiv(a, b int64) int64 {
	if a < 0 {
		return -((-a + b/2) / b)
	}
	return (a + b/2) / b
}
