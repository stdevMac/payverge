package emails

import (
	"strings"
	"testing"
)

func TestRenderSubjectFromBlock(t *testing.T) {
	tm, err := NewTemplateManager(resolveTemplatesRoot(t))
	if err != nil {
		t.Fatalf("new template manager: %v", err)
	}
	// payment_receipt has a static subject; welcome too.
	got, err := tm.renderSubject("en", "payment_receipt", map[string]interface{}{})
	if err != nil {
		t.Fatalf("renderSubject: %v", err)
	}
	if got != "Payment receipt" {
		t.Fatalf("expected %q, got %q", "Payment receipt", got)
	}

	// A template with a variable subject renders the variable.
	got, err = tm.renderSubject("en", "payverge_update", map[string]interface{}{
		"update_title": "New feature shipped",
	})
	if err != nil {
		t.Fatalf("renderSubject variable: %v", err)
	}
	if got != "New feature shipped" {
		t.Fatalf("expected variable subject, got %q", got)
	}

	// A language with no template family falls back to the English subject.
	got, err = tm.renderSubject("fr", "payment_receipt", map[string]interface{}{})
	if err != nil {
		t.Fatalf("renderSubject fallback: %v", err)
	}
	if got != "Payment receipt" {
		t.Fatalf("expected English fallback subject %q, got %q", "Payment receipt", got)
	}
}

func TestRenderSubjectDoesNotHTMLEscapeNames(t *testing.T) {
	tm, err := NewTemplateManager(resolveTemplatesRoot(t))
	if err != nil {
		t.Fatalf("new template manager: %v", err)
	}

	const venue = `Joe's Bar & Grill`
	got, err := tm.renderSubject("en", "reservation_noshow", map[string]interface{}{
		"business_name": venue,
	})
	if err != nil {
		t.Fatalf("renderSubject: %v", err)
	}
	want := "We missed you at " + venue
	if got != want {
		t.Fatalf("subject = %q, want %q", got, want)
	}

	got, err = tm.renderSubject("es", "reservation_noshow", map[string]interface{}{
		"business_name": venue,
	})
	if err != nil {
		t.Fatalf("renderSubject es: %v", err)
	}
	if strings.Contains(got, "&#") || strings.Contains(got, "&amp;") || strings.Contains(got, "&quot;") {
		t.Fatalf("Spanish subject was HTML-escaped: %q", got)
	}
	if !strings.Contains(got, venue) {
		t.Fatalf("Spanish subject %q missing venue name", got)
	}
}
