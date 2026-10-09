package emails

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// templateActionRe finds dot-variable references inside any {{ ... }} action span,
// e.g. {{ .business_name }}, {{ range .items }}, {{ if .show_total }}. The bare
// pipeline dot ({{ template "content" . }}) is NOT captured because a name
// character must follow the dot.
var templateActionRe = regexp.MustCompile(`{{[^}]*}}`)
var dotVarRe = regexp.MustCompile(`\.([A-Za-z_][A-Za-z0-9_]*)`)

// templateVariableSet returns the sorted unique set of dot-variable names
// referenced in a template file's raw text.
func templateVariableSet(content string) []string {
	seen := map[string]struct{}{}
	for _, action := range templateActionRe.FindAllString(content, -1) {
		for _, m := range dotVarRe.FindAllStringSubmatch(action, -1) {
			seen[m[1]] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestTemplateVariableParityAcrossLocales asserts every non-eng family
// references exactly the same template variables as eng, per template. This is
// registry-driven: a newly added email family is compared against eng with no
// edits to this test.
func TestTemplateVariableParityAcrossLocales(t *testing.T) {
	root := resolveTemplatesRoot(t)
	engDir := filepath.Join(root, "eng")
	engFiles := listTemplateFiles(t, engDir)

	for _, lang := range templateLanguageFamilies() {
		if lang == "eng" {
			continue
		}
		filenames := make([]string, 0, len(engFiles))
		for filename := range engFiles {
			filenames = append(filenames, filename)
		}
		sort.Strings(filenames)
		for _, filename := range filenames {
			engVars := templateVariableSet(readTemplate(t, filepath.Join(engDir, filename)))
			langPath := filepath.Join(root, lang, filename)
			langVars := templateVariableSet(readTemplate(t, langPath))
			if strings.Join(engVars, ",") != strings.Join(langVars, ",") {
				t.Errorf("variable drift in %s/%s\n eng: %v\n %s: %v",
					lang, filename, engVars, lang, langVars)
			}
		}
	}
}

// TestRenderedEmailHasSingleTitleInHead asserts the rendered email has its
// <title> only in <head> (valid HTML) and none after <body> — the old
// in-body-title antipattern must be gone.
func TestRenderedEmailHasSingleTitleInHead(t *testing.T) {
	root := resolveTemplatesRoot(t)
	tm, err := NewTemplateManager(root)
	if err != nil {
		t.Fatalf("new template manager: %v", err)
	}
	for _, lang := range templateLanguageFamilies() {
		for filename := range listTemplateFiles(t, filepath.Join(root, lang)) {
			name := strings.TrimSuffix(filename, ".html")
			body, err := tm.Render(lang, name, map[string]interface{}{
				"unsubscribe_url": "https://payverge.io/account",
			})
			if err != nil {
				t.Fatalf("render %s/%s: %v", lang, name, err)
			}
			lower := strings.ToLower(body)
			bodyIdx := strings.Index(lower, "<body")
			if bodyIdx < 0 {
				t.Fatalf("%s/%s rendered without <body>", lang, name)
			}
			if strings.Contains(lower[bodyIdx:], "<title>") {
				t.Fatalf("%s/%s has a <title> inside <body>", lang, name)
			}
			if strings.Count(lower[:bodyIdx], "<title>") != 1 {
				t.Fatalf("%s/%s must have exactly one <title> in <head>", lang, name)
			}
		}
	}
}

func TestFooterVariantClassifier(t *testing.T) {
	// Guests: no account.
	guests := []string{
		"payment_receipt", "thank_you_guest", "guest_feedback",
		"reservation_confirmation", "reservation_pending",
		"reservation_cancelled", "reservation_noshow", "reservation_reminder", "reservation_updated",
	}
	for _, name := range guests {
		if !isGuestTemplate(name) {
			t.Errorf("%s should be classified guest", name)
		}
		if got := footerVariant(name, MessageTypeTransactional); got != "guest" {
			t.Errorf("%s transactional footer = %q, want guest", name, got)
		}
	}

	// Operator transactional default (fail-safe for unknown names).
	for _, name := range []string{"welcome", "staff_invitation", "getting_started", "totally_new_template"} {
		if isGuestTemplate(name) {
			t.Errorf("%s should NOT be guest", name)
		}
		if got := footerVariant(name, MessageTypeTransactional); got != "operator" {
			t.Errorf("%s footer = %q, want operator", name, got)
		}
	}

	// Explicit: an entirely unknown template name is not a guest template.
	if isGuestTemplate("not_a_real_template") {
		t.Error("unknown template name must not be classified as guest")
	}

	// Broadcast wins regardless of template.
	if got := footerVariant("payverge_update", MessageTypeBroadcast); got != "broadcast" {
		t.Errorf("broadcast footer = %q, want broadcast", got)
	}
	if got := footerVariant("payment_receipt", MessageTypeBroadcast); got != "broadcast" {
		t.Errorf("broadcast over guest = %q, want broadcast", got)
	}
}

// TestGuestSetNamesAreRealTemplates: every guest-set name is a managed template.
func TestGuestSetNamesAreRealTemplates(t *testing.T) {
	managed := map[string]struct{}{}
	for _, n := range managedTemplateNames() {
		managed[n] = struct{}{}
	}
	for name := range guestTemplateNames {
		if _, ok := managed[name]; !ok {
			t.Errorf("guest-set name %q is not a managed template", name)
		}
	}
}

// TestLayoutPaletteAndBrand: layouts use the brand palette + real logo, and no
// legacy hexes remain in any layout.
func TestLayoutPaletteAndBrand(t *testing.T) {
	root := resolveTemplatesRoot(t)
	layoutDir := filepath.Join(root, "..", "layout")
	legacy := []string{"#f7f7f8", "#1f2937", "#111827", "#6b7280", "#e5e7eb"}
	for _, fam := range templateLanguageFamilies() {
		path := filepath.Join(layoutDir, "base_"+fam+".html")
		content := readTemplate(t, path)
		for _, hex := range legacy {
			if strings.Contains(content, hex) {
				t.Errorf("layout %s still contains legacy color %s", fam, hex)
			}
		}
		if !strings.Contains(content, "#1a6b6a") {
			t.Errorf("layout %s missing brand teal #1a6b6a", fam)
		}
		if !strings.Contains(content, "#faf9f6") {
			t.Errorf("layout %s missing brand cream #faf9f6", fam)
		}
		if !strings.Contains(content, "#1c1917") {
			t.Errorf("layout %s missing brand charcoal #1c1917", fam)
		}
		if !strings.Contains(content, `src="{{ .instance_logo_url }}"`) {
			t.Errorf("layout %s must load the logo from the instance (instance_logo_url)", fam)
		}
		if strings.Contains(content, "payverge.io") {
			t.Errorf("layout %s hard-codes the upstream domain", fam)
		}
		if !strings.Contains(content, `{{ template "subject" . }}`) {
			t.Errorf("layout %s head must render the subject block", fam)
		}
	}
}

func renderFooter(t *testing.T, tm *TemplateManager, lang, name, variant string) string {
	t.Helper()
	body, err := tm.Render(lang, name, map[string]interface{}{
		"footer_variant":  variant,
		"business_name":   "Cafe Test",
		"company_name":    "Payverge",
		"company_address": "123 Test St, Dubai, UAE",
		"unsubscribe_url": "https://payverge.io/account",
		"update_title":    "Update",
		"recipient_name":  "Sam",
		"dashboard_url":   "https://payverge.io/business/1/dashboard",
	})
	if err != nil {
		t.Fatalf("render %s/%s (%s): %v", lang, name, variant, err)
	}
	return body
}

// TestContentTemplatesPaletteIsBranded: no content file may carry a legacy hex.
func TestContentTemplatesPaletteIsBranded(t *testing.T) {
	root := resolveTemplatesRoot(t)
	legacy := []string{"#111827", "#f7f7f8", "#1f2937", "#6b7280", "#e5e7eb"}
	for _, lang := range templateLanguageFamilies() {
		dir := filepath.Join(root, lang)
		for filename := range listTemplateFiles(t, dir) {
			content := readTemplate(t, filepath.Join(dir, filename))
			for _, hex := range legacy {
				if strings.Contains(content, hex) {
					t.Errorf("%s/%s still contains legacy color %s", lang, filename, hex)
				}
			}
		}
	}
}

func TestFooterContentByVariant(t *testing.T) {
	root := resolveTemplatesRoot(t)
	tm, err := NewTemplateManager(root)
	if err != nil {
		t.Fatalf("new template manager: %v", err)
	}

	// Broadcast: must carry unsubscribe URL + physical address (CAN-SPAM).
	b := renderFooter(t, tm, "en", "payverge_update", "broadcast")
	if !strings.Contains(b, "/account") {
		t.Error("broadcast footer missing unsubscribe link")
	}
	if !strings.Contains(b, "123 Test St, Dubai, UAE") {
		t.Error("broadcast footer missing physical address")
	}

	// Operator: manage-preferences link + address.
	o := renderFooter(t, tm, "en", "welcome", "operator")
	if !strings.Contains(o, "/account") {
		t.Error("operator footer missing manage-preferences link")
	}
	if !strings.Contains(o, "123 Test St, Dubai, UAE") {
		t.Error("operator footer missing physical address")
	}

	// Guest: business context line, NO account/settings link, NO address.
	g := renderFooter(t, tm, "en", "payment_receipt", "guest")
	if !strings.Contains(g, "Cafe Test") {
		t.Error("guest footer missing business context line")
	}
	if strings.Contains(g, "/account") {
		t.Error("guest footer must NOT contain the account-settings link")
	}
	if strings.Contains(g, "123 Test St, Dubai, UAE") {
		t.Error("guest footer must NOT contain a physical address")
	}
}

var subjectBlockRe = regexp.MustCompile(`(?s){{\s*define\s+"subject"\s*}}(.*?){{\s*end\s*}}`)

func subjectBlockVariableSet(content string) []string {
	m := subjectBlockRe.FindStringSubmatch(content)
	if m == nil {
		return nil
	}
	return templateVariableSet(m[1])
}

// TestSubjectBlockVariableParityAcrossLocales asserts each template's subject
// block references the same variables across every locale family (eng is the
// reference). Catches subject-specific drift that the whole-file parity test
// masks when the body also references the variable.
func TestSubjectBlockVariableParityAcrossLocales(t *testing.T) {
	root := resolveTemplatesRoot(t)
	engDir := filepath.Join(root, "eng")
	engFiles := listTemplateFiles(t, engDir)
	for _, lang := range templateLanguageFamilies() {
		if lang == "eng" {
			continue
		}
		filenames := make([]string, 0, len(engFiles))
		for filename := range engFiles {
			filenames = append(filenames, filename)
		}
		sort.Strings(filenames)
		for _, filename := range filenames {
			engVars := subjectBlockVariableSet(readTemplate(t, filepath.Join(engDir, filename)))
			langVars := subjectBlockVariableSet(readTemplate(t, filepath.Join(root, lang, filename)))
			if strings.Join(engVars, ",") != strings.Join(langVars, ",") {
				t.Errorf("subject variable drift in %s/%s\n eng: %v\n %s: %v",
					lang, filename, engVars, lang, langVars)
			}
		}
	}
}

// keyStructuralParityTemplates historically diverged in content/structure across
// locales (different copy, dropped paragraphs, list-vs-prose). Their es/es_ar must
// now mirror eng's HTML structure, not just its variables. Opening-tag counts must
// match across every family.
var keyStructuralParityTemplates = []string{
	"welcome", "staff_invitation", "getting_started", "payment_receipt",
}

func TestKeyTemplatesStructuralParity(t *testing.T) {
	root := resolveTemplatesRoot(t)
	tags := []string{"p", "li", "ol", "tr", "h1", "a"}
	tagRe := map[string]*regexp.Regexp{}
	for _, tag := range tags {
		tagRe[tag] = regexp.MustCompile(`(?i)<` + tag + `[\s>]`)
	}
	for _, name := range keyStructuralParityTemplates {
		eng := readTemplate(t, filepath.Join(root, "eng", name+".html"))
		for _, lang := range templateLanguageFamilies() {
			if lang == "eng" {
				continue
			}
			other := readTemplate(t, filepath.Join(root, lang, name+".html"))
			for _, tag := range tags {
				engN := len(tagRe[tag].FindAllString(eng, -1))
				otherN := len(tagRe[tag].FindAllString(other, -1))
				if engN != otherN {
					t.Errorf("%s: <%s> count mismatch eng=%d %s=%d", name, tag, engN, lang, otherN)
				}
			}
		}
	}
}
