package utils

import "testing"

func TestSanitizeCSVField(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"plain text untouched", "Acme Cafe", "Acme Cafe"},
		{"equals formula neutralized", "=HYPERLINK(\"http://evil\")", "'=HYPERLINK(\"http://evil\")"},
		{"plus neutralized", "+1+1", "'+1+1"},
		{"minus/negative neutralized", "-2+3", "'-2+3"},
		{"at neutralized", "@SUM(A1)", "'@SUM(A1)"},
		{"leading tab neutralized", "\t=cmd", "'\t=cmd"},
		{"leading carriage return neutralized", "\r=cmd", "'\r=cmd"},
		{"trigger not at start is safe", "Café =not a formula", "Café =not a formula"},
		{"plus phone number is treated as text (acceptable)", "+15551234567", "'+15551234567"},
		{"multibyte first char is safe", "Ñoño", "Ñoño"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeCSVField(tc.in); got != tc.want {
				t.Fatalf("SanitizeCSVField(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
