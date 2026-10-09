package utils

import "testing"

func TestNormalizeOperatorLang(t *testing.T) {
	cases := map[string]string{
		"en": "en", "es": "es", "es-AR": "es", "es_ar": "es", "ES-AR": "es", "fr": "en",
	}
	for in, want := range cases {
		if got := NormalizeOperatorLang(in); got != want {
			t.Fatalf("NormalizeOperatorLang(%q)=%q want %q", in, got, want)
		}
	}
}
