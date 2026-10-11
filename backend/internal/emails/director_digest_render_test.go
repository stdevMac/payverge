package emails

import (
	"html/template"
	"strings"
	"testing"
)

// TestDirectorDigestRendersInsightsSummaryRaw documents and enforces that a
// template.HTML value placed at "insights_summary" is rendered verbatim by
// html/template — i.e. it is NOT double-escaped. This is the contract that
// allows buildInsightsSummaryHTML (services package) to emit safe pre-assembled
// markup and have it reach the recipient as real HTML.
func TestDirectorDigestRendersInsightsSummaryRaw(t *testing.T) {
	tm, err := NewTemplateManager(realTemplatesRoot(t))
	if err != nil {
		t.Fatalf("new template manager: %v", err)
	}

	// template.HTML signals to html/template that this value is already safe;
	// it must be rendered without escaping.
	safeMarkup := template.HTML("<ul><li><strong>3 items running low</strong>: Burrito, Taco, Wrap — reorder soon.</li></ul>")

	for _, lang := range []string{"en", "es", "es_ar"} {
		t.Run(lang, func(t *testing.T) {
			data := map[string]interface{}{
				"owner_name":           "Test Owner",
				"business_name":        "Test Bistro",
				"digest_date":          "May 31, 2026",
				"director_console_url": "https://app.payverge.io/business/test/dashboard",
				"insights_summary":     safeMarkup,
			}

			body, err := tm.Render(lang, "director_digest", data)
			if err != nil {
				t.Fatalf("render(%s): %v", lang, err)
			}

			// The literal tags must appear in the output — not escaped.
			for _, tag := range []string{"<ul>", "<li>", "<strong>"} {
				if !strings.Contains(body, tag) {
					t.Errorf("lang=%s: expected tag %q un-escaped in body; got excerpt:\n%s",
						lang, tag, excerpt(body, "insights"))
				}
			}

			// The escaped form must NOT appear (double-escaping would break rendering).
			if strings.Contains(body, "&lt;ul&gt;") {
				t.Errorf("lang=%s: double-escaped &lt;ul&gt; found — template.HTML contract broken", lang)
			}
		})
	}
}

// excerpt returns up to 300 chars of s, centred around the first occurrence of
// keyword, for compact failure messages.
func excerpt(s, keyword string) string {
	idx := strings.Index(strings.ToLower(s), strings.ToLower(keyword))
	if idx < 0 {
		if len(s) > 300 {
			return s[:300] + "…"
		}
		return s
	}
	start := idx - 100
	if start < 0 {
		start = 0
	}
	end := idx + 200
	if end > len(s) {
		end = len(s)
	}
	return s[start:end]
}
