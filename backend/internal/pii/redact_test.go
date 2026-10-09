package pii

import "testing"

func TestRedact(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// --- MUST NOT touch (the audit's §4-A regression set) ---
		{"iso date range", "Revenue from 2026-05-30 to 2026-06-06 grew 12%", "Revenue from 2026-05-30 to 2026-06-06 grew 12%"},
		{"week of date", "Week of 2026-06-01: 145 orders", "Week of 2026-06-01: 145 orders"},
		{"space grouped quantity", "sold 1 234 567 units", "sold 1 234 567 units"},
		{"hash id", "Order #100234567", "Order #100234567"},
		{"comma grouped quantity", "sold 1,234,567 units", "sold 1,234,567 units"},
		{"currency amount", "total was $1,234.50 today", "total was $1,234.50 today"},
		{"percentage", "conversion rose 12% week over week", "conversion rose 12% week over week"},
		{"plain integer run", "table 12345678 served", "table 12345678 served"},
		{"single iso date", "as of 2026-06-06 we shipped", "as of 2026-06-06 we shipped"},
		// audit §4-A shape: a >=9-digit grouped quantity preceded by a word that
		// merely CONTAINS a former phone-keyword substring ("was" ⊃ "wa"). A
		// substring Contains match would wrongly redact this; the word-boundary
		// keyword test in isRedactablePhone MUST leave it untouched (contract C3:
		// "MUST NOT touch space/comma-grouped quantities").
		{"keyword-substring word + grouped quantity", "it was 123 456 789 transactions", "it was 123 456 789 transactions"},
		{"software + grouped quantity", "our software processed 987 654 321 events", "our software processed 987 654 321 events"},
		{"award + grouped quantity", "the award covered 100 200 300 customers", "the award covered 100 200 300 customers"},
		// paren-led accounting figure (>=9 digits, leads with "(", no phone intent):
		{"parenthesized accounting figure", "(1 234 567 890) net loss", "(1 234 567 890) net loss"},

		// --- MUST redact (phones) ---
		{"us paren phone with keyword", "Call +1 (555) 123-4567", "Call [redacted-phone]"},
		{"plus e164", "+5491155551234", "[redacted-phone]"},
		{"spanish keyword phone", "teléfono: 11 5555 1234", "teléfono: [redacted-phone]"},
		{"phone keyword english", "phone 555 867 5309 now", "phone [redacted-phone] now"},
		{"whatsapp keyword", "whatsapp 5491133334444 today", "whatsapp [redacted-phone] today"},

		// --- MUST redact (emails) ---
		{"plain email", "reach me at jane.doe@example.com please", "reach me at [redacted-email] please"},
		{"email plus tag", "ops+alerts@payverge.io bounced", "[redacted-email] bounced"},

		// --- mixed: email redacted, date untouched ---
		{"email and date", "on 2026-06-06 email a@b.co", "on 2026-06-06 email [redacted-email]"},

		// --- grouped quantity that LOOKS phone-ish but has no context signal: untouched ---
		{"bare grouped no context", "processed 12 345 678 records", "processed 12 345 678 records"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Redact(tc.in); got != tc.want {
				t.Fatalf("Redact(%q)\n  got  %q\n  want %q", tc.in, got, tc.want)
			}
		})
	}
}

func BenchmarkRedact(b *testing.B) {
	const para = "On 2026-06-06 we sold 1,234,567 units ($1,234.50 each, up 12%); " +
		"call +1 (555) 123-4567 or email ops+alerts@payverge.io, ref Order #100234567, " +
		"teléfono 11 5555 1234, processed 12 345 678 records this week."
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Redact(para)
	}
}
