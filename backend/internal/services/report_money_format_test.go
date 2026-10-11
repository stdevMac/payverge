package services

import "testing"

// Fix 5: report emails must render money in the business currency with the
// currency's real decimal count, not a hardcoded "$%.2f". JPY has no minor unit
// (must not show ".00"), BHD has three, and the symbol/code must reflect the
// business currency rather than always "$".
func TestFormatReportMoney(t *testing.T) {
	cases := []struct {
		major    float64
		currency string
		want     string
	}{
		{10, "USD", "USD 10.00"},
		{1234.5, "EUR", "EUR 1234.50"},
		{1000, "JPY", "JPY 1000"}, // zero-decimal: no ".00", not 100x off
		{10, "BHD", "BHD 10.000"}, // three-decimal
		{5, "", "USD 5.00"},       // empty currency falls back to USD
	}
	for _, c := range cases {
		got := formatReportMoney(c.major, c.currency)
		if got != c.want {
			t.Errorf("formatReportMoney(%v, %q) = %q, want %q", c.major, c.currency, got, c.want)
		}
	}
}
