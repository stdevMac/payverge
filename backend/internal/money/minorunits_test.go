package money

import "testing"

func TestDecimalDigits(t *testing.T) {
	cases := map[string]int{
		"USD": 2, "usd": 2, "EUR": 2, "ARS": 2, "BRL": 2, " gbp ": 2,
		"JPY": 0, "jpy": 0, "KRW": 0, "CLP": 0, "VND": 0, "ISK": 0,
		"BHD": 3, "KWD": 3, "OMR": 3,
		"": 2, "ZZZ": 2, // default / unknown
	}
	for code, want := range cases {
		if got := DecimalDigits(code); got != want {
			t.Errorf("DecimalDigits(%q) = %d, want %d", code, got, want)
		}
	}
}

// MinorUnits converts the platform's universal "cents" (major*100) into the
// provider's actual minor unit for the currency.
func TestMinorUnits(t *testing.T) {
	cases := []struct {
		cents int64
		code  string
		want  int64
	}{
		{1000, "USD", 1000},     // $10.00 -> 1000 cents
		{5, "USD", 5},           // $0.05 -> 5 cents
		{100000, "JPY", 1000},   // ¥1000 stored as 100000 -> 1000 yen (NOT 100000)
		{250000, "KRW", 2500},   // ₩2500 -> 2500 won
		{8250, "JPY", 83},       // ¥82.50 stored -> rounds to nearest whole yen
		{0, "JPY", 0},           // zero stays zero
		{1000, "BHD", 10000},    // 10.000 BHD stored as 1000 -> 10000 fils (3-decimal: ×10)
		{1000, "", 1000},        // unknown/empty -> 2-decimal identity
		{-100000, "JPY", -1000}, // negative preserved
	}
	for _, tc := range cases {
		if got := MinorUnits(tc.cents, tc.code); got != tc.want {
			t.Errorf("MinorUnits(%d, %q) = %d, want %d", tc.cents, tc.code, got, tc.want)
		}
	}
}

// FromMinorUnits is the inverse of MinorUnits: it converts a provider's
// smallest-unit amount (what Stripe reports in webhooks / status polling) back
// into the platform's universal "cents" (major*100).
func TestFromMinorUnits(t *testing.T) {
	cases := []struct {
		minor int64
		code  string
		want  int64
	}{
		{1000, "USD", 1000},     // $10.00 stays 1000 cents
		{5, "USD", 5},           // $0.05 stays 5 cents
		{1000, "JPY", 100000},   // ¥1000 (Stripe minor) -> 100000 stored cents
		{2500, "KRW", 250000},   // ₩2500 -> 250000 stored cents
		{0, "JPY", 0},           // zero stays zero
		{1500, "BHD", 150},      // 1.500 BHD = 1500 mils (Stripe) -> 150 stored cents
		{10000, "BHD", 1000},    // 10.000 BHD = 10000 mils -> 1000 stored cents
		{1000, "", 1000},        // unknown/empty -> 2-decimal identity
		{-1000, "JPY", -100000}, // negative preserved
	}
	for _, tc := range cases {
		if got := FromMinorUnits(tc.minor, tc.code); got != tc.want {
			t.Errorf("FromMinorUnits(%d, %q) = %d, want %d", tc.minor, tc.code, got, tc.want)
		}
	}
}

// FromMinorUnits must invert MinorUnits for whole-unit amounts (round-trip).
func TestMinorUnitsRoundTrip(t *testing.T) {
	for _, code := range []string{"USD", "JPY", "KRW", "BHD", "KWD", "EUR"} {
		for _, cents := range []int64{0, 100, 1000, 250000} {
			minor := MinorUnits(cents, code)
			if got := FromMinorUnits(minor, code); got != cents {
				t.Errorf("round-trip %s: FromMinorUnits(MinorUnits(%d)) = %d, want %d", code, cents, got, cents)
			}
		}
	}
}

// MajorUnitString renders an amount in major units with the currency's
// provider-expected number of decimal places (PayPal `value`, display).
func TestMajorUnitString(t *testing.T) {
	cases := []struct {
		cents int64
		code  string
		want  string
	}{
		{1000, "USD", "10.00"},
		{5, "USD", "0.05"},
		{250050, "USD", "2500.50"},
		{100000, "JPY", "1000"}, // no decimal point for zero-decimal currencies
		{8250, "JPY", "83"},     // rounded to whole yen, consistent with MinorUnits
		{2500, "KRW", "25"},
		{1000, "BHD", "10.000"},
		{0, "USD", "0.00"},
		{-1000, "USD", "-10.00"},
	}
	for _, tc := range cases {
		if got := MajorUnitString(tc.cents, tc.code); got != tc.want {
			t.Errorf("MajorUnitString(%d, %q) = %q, want %q", tc.cents, tc.code, got, tc.want)
		}
	}
}
