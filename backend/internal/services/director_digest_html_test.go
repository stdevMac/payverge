package services

import (
	"strings"
	"testing"
)

// TestBuildInsightsSummaryHTML_EscapesItemNamesPreservesMarkup verifies that:
//  1. Trusted markup (<ul>, <li>, <strong>) is emitted as real HTML tags (not escaped).
//  2. User-controlled item names (e.g. containing "<script>") are HTML-escaped so
//     they cannot be injected as markup.
func TestBuildInsightsSummaryHTML_EscapesItemNamesPreservesMarkup(t *testing.T) {
	maliciousName := `Café <script>alert(1)</script>`

	// insightsFunc returns a single low-stock insight with a crafted item name.
	insightsFn := InsightsFunc(func(_ uint) []DigestInsight {
		return []DigestInsight{
			{
				Type: "inventory_low_stock",
				Params: map[string]interface{}{
					"count":      2,
					"item_names": []string{maliciousName},
				},
			},
		}
	})

	scheduler := &DirectorDigestScheduler{
		insightsFunc: insightsFn,
	}

	result := string(scheduler.buildInsightsSummaryHTML(1, "en"))

	// 1. Trusted structural markup must be present and UNESCAPED.
	for _, tag := range []string{"<ul", "<li", "<strong"} {
		if !strings.Contains(result, tag) {
			t.Errorf("expected trusted markup %q to be present (unescaped) in result; got:\n%s", tag, result)
		}
	}

	// 2. The raw "<script>" from the item name must NOT appear — it must be escaped.
	if strings.Contains(result, "<script>") {
		t.Errorf("raw <script> tag leaked unescaped into result; XSS vulnerability:\n%s", result)
	}

	// 3. The escaped form of the injected tag must appear, confirming the name IS
	//    included in the output (just safely).
	if !strings.Contains(result, "&lt;script&gt;") {
		t.Errorf("expected &lt;script&gt; (escaped item name) in result; got:\n%s", result)
	}
}

// TestBuildInsightsSummaryHTML_EscapesUnknownInsightType verifies that an insight
// with an unknown Type that contains HTML markup hits the default: branch and that
// the Type string is HTML-escaped before being interpolated into the output string,
// preventing injection via future external-sourced Type values.
func TestBuildInsightsSummaryHTML_EscapesUnknownInsightType(t *testing.T) {
	maliciousType := "<script>evil</script>"

	insightsFn := InsightsFunc(func(_ uint) []DigestInsight {
		return []DigestInsight{
			{
				Type:   maliciousType,
				Params: map[string]interface{}{"count": 3},
			},
		}
	})

	scheduler := &DirectorDigestScheduler{insightsFunc: insightsFn}

	// Test both language paths — the default: branch exists in EN and ES.
	for _, lang := range []string{"en", "es"} {
		result := string(scheduler.buildInsightsSummaryHTML(1, lang))

		// Raw script tag must NOT appear.
		if strings.Contains(result, "<script>") {
			t.Errorf("[lang=%s] raw <script> leaked unescaped into output (XSS); got:\n%s", lang, result)
		}

		// Escaped form must appear, confirming the type IS included (just safely).
		if !strings.Contains(result, "&lt;script&gt;") {
			t.Errorf("[lang=%s] expected &lt;script&gt; (escaped insType) in output; got:\n%s", lang, result)
		}
	}
}

// TestBuildInsightsSummaryHTML_EmptyInsightsReturnsCheckmarkParagraph verifies
// the "all good" path for completeness.
func TestBuildInsightsSummaryHTML_EmptyInsightsReturnsCheckmarkParagraph(t *testing.T) {
	scheduler := &DirectorDigestScheduler{
		insightsFunc: InsightsFunc(func(_ uint) []DigestInsight { return nil }),
	}

	result := string(scheduler.buildInsightsSummaryHTML(1, "en"))

	if !strings.Contains(result, "<p") {
		t.Errorf("expected <p> in all-good result; got: %s", result)
	}
	if !strings.Contains(result, "&#10003;") {
		t.Errorf("expected checkmark entity in all-good result; got: %s", result)
	}
}
