package utils

// SanitizeCSVField neutralizes spreadsheet formula injection (CWE-1236) for a
// single CSV cell value.
//
// Excel, Google Sheets, and LibreOffice interpret a cell whose first character
// is one of = + - @ as a formula, and treat a leading TAB or CR as whitespace
// that can precede such a trigger. An attacker who controls a free-text field
// that later lands in an exported CSV (e.g. a customer name or note entered at
// loyalty enrollment / checkout) can therefore inject a formula that runs in
// the *operator's* spreadsheet when they open the export — enabling data
// exfiltration (=HYPERLINK / =WEBSERVICE) or, on legacy configs, DDE command
// execution. The CSV layer's own quoting does NOT prevent this: the spreadsheet
// strips the surrounding quotes and still sees the leading trigger.
//
// The fix is the OWASP-recommended one: prefix a dangerous cell with a single
// apostrophe, which forces the spreadsheet to treat the value as text. The
// apostrophe is hidden on display, so the visible value is unchanged.
//
// Apply this only to free-text / externally-influenced string fields. Do not
// apply it to code-formatted numerics (counts, money) — those are never
// attacker-controlled and a legitimate leading "-" on a value the code itself
// produced should stay numeric.
func SanitizeCSVField(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}
