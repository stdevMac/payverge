package services

import (
	"strings"
	"testing"
)

func TestResolveWaiterPrompt_EnglishOrdering(t *testing.T) {
	got, contentLocale := resolveWaiterPrompt("ordering", "en")
	if !strings.Contains(got, "data_block") {
		t.Fatalf("expected english ordering prompt to mention the data_block rule, got %q", got[:min(120, len(got))])
	}
	if contentLocale.Canonical != "en" {
		t.Fatalf("expected content locale en, got %q", contentLocale.Canonical)
	}
}

func TestResolveWaiterPrompt_UnknownLocaleFallsBackToEnContent(t *testing.T) {
	got, contentLocale := resolveWaiterPrompt("ordering", "xx")
	if contentLocale.Canonical != "en" {
		t.Fatalf("expected en fallback content locale, got %q", contentLocale.Canonical)
	}
	if got == "" {
		t.Fatalf("expected non-empty fallback content")
	}
}

func TestResolveWaiterPrompt_ReviewPendingTierThreeUsesEnContentButLocaleNativeName(t *testing.T) {
	_, contentLocale := resolveWaiterPrompt("ordering", "ja")
	if contentLocale.NativeName != "日本語" {
		t.Fatalf("expected ja NativeName anchoring 日本語, got %q", contentLocale.NativeName)
	}
}

func TestResolveWaiterPrompt_NativeSpanishFamilies(t *testing.T) {
	for _, tc := range []struct{ code, family, want string }{
		{"es", "es", "Espa\u00f1ol"},
		{"es-AR", "es_ar", "Espa\u00f1ol (Argentina)"},
	} {
		body, contentLocale := resolveWaiterPrompt("ordering", tc.code)
		if contentLocale.NativeName != tc.want {
			t.Fatalf("%s: expected NativeName %q, got %q", tc.code, tc.want, contentLocale.NativeName)
		}
		if !strings.Contains(body, "data_block") {
			t.Fatalf("%s: expected native prompt body to keep the data_block rule", tc.code)
		}
	}
	body, _ := resolveWaiterPrompt("ordering", "es-AR")
	if strings.Contains(body, "puedes") {
		t.Fatalf("es_ar prompt leaked tuteo 'puedes' (must be voseo)")
	}
	if !strings.Contains(body, "Record\u00e1") && !strings.Contains(body, "Recuerda") {
		t.Fatalf("expected a natively-authored Spanish reminder line")
	}
}

// TestWrapDataBlock_MarkerIsFreshAndBounds locks the spotlighting contract (§D):
// markers are per-request fresh, and wrapDataBlock embeds the marker exactly
// three times (the marker= attribute plus the open + close boundary lines) so a
// model can verify the boundary even under injection that tries to forge tags.
func TestWrapDataBlock_MarkerIsFreshAndBounds(t *testing.T) {
	m1, m2 := newWaiterMarker(), newWaiterMarker()
	if m1 == m2 {
		t.Fatalf("expected fresh per-request markers, both were %q", m1)
	}
	out := wrapDataBlock("about", m1, "Name: X")
	if got := strings.Count(out, m1); got != 3 {
		t.Fatalf("expected marker %q to appear 3 times (attribute + 2 bounds), got %d in %q", m1, got, out)
	}
	if !strings.Contains(out, "Name: X") {
		t.Fatalf("expected wrapped content to contain the payload, got %q", out)
	}
	if strings.Contains(out, m2) {
		t.Fatalf("wrapped block must not contain a foreign marker %q: %q", m2, out)
	}
}

func TestWaiterMarkerIsEightHexAndUnique(t *testing.T) {
	a := newWaiterMarker()
	b := newWaiterMarker()
	if len(a) != 8 {
		t.Fatalf("expected 8-hex marker, got %q (len %d)", a, len(a))
	}
	if a == b {
		t.Fatalf("expected unique markers per call, both were %q", a)
	}
	for _, r := range a {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("marker has non-hex rune %q in %q", r, a)
		}
	}
}
