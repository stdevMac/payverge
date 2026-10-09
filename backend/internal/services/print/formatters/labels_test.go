package formatters

import "testing"

// resolvePrintLang is the seam that maps a raw business locale tag onto the
// three label bundles we ship (en, es, es-AR). These cases assert the
// case-insensitive / separator-tolerant resolution and the English fallback.
func TestResolvePrintLang(t *testing.T) {
	cases := []struct {
		in   string
		want printLang
	}{
		{"en", printLangEN},
		{"EN", printLangEN},
		{"", printLangEN},
		{"fr", printLangEN},    // unknown → English (no regression)
		{"de-DE", printLangEN}, // unknown → English
		{"es", printLangES},
		{"ES", printLangES},
		{"es-AR", printLangESAR},
		{"es_ar", printLangESAR},
		{"es-ar", printLangESAR},
		{"ES_AR", printLangESAR},
	}
	for _, c := range cases {
		if got := resolvePrintLang(c.in); got != c.want {
			t.Errorf("resolvePrintLang(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCanonicalPrintLanguage(t *testing.T) {
	cases := map[string]string{
		"":      "en",
		"en":    "en",
		"EN":    "en",
		"fr":    "en", // no label bundle → English (existing tier contract)
		"es-MX": "en", // non-registry regional Spanish collapses to en (registry exact-match seam)
		"es":    "es",
		"ES":    "es",
		"es-AR": "es-AR",
		"es_ar": "es-AR",
		"es-ar": "es-AR",
	}
	for in, want := range cases {
		if got := CanonicalPrintLanguage(in); got != want {
			t.Errorf("CanonicalPrintLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPrintLabels_BundlesComplete guards that every shipped locale has a
// non-empty value for every label key, so a missing translation is a test
// failure rather than a blank label on a printed ticket.
func TestPrintLabels_BundlesComplete(t *testing.T) {
	for lang, b := range printLabelBundles {
		v := b
		fields := map[string]string{
			"Bill": v.Bill, "Table": v.Table, "Date": v.Date,
			"Subtotal": v.Subtotal, "Tax": v.Tax, "Service": v.Service,
			"Receipt": v.Receipt, "Tip": v.Tip, "TotalPaid": v.TotalPaid, "Total": v.Total,
			"Method": v.Method, "Txn": v.Txn, "Paid": v.Paid,
			"ThankYou": v.ThankYou, "PreBill": v.PreBill,
			"Kitchen": v.Kitchen, "OrderNum": v.OrderNum, "Allergens": v.Allergens,
			"EndOfTicket": v.EndOfTicket,
		}
		for name, val := range fields {
			if val == "" {
				t.Errorf("bundle %q missing label %q", lang, name)
			}
		}
	}
}
