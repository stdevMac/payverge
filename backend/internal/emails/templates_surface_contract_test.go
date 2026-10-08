package emails

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/locales"
)

// The transactional-email surface contract: a locale ships its own template
// family if and only if its registry requiredSurfaces include "emails".
// Guest-storefront-only locales (ar/de/fr/…) intentionally fall back to English
// in TemplateManager.Render — they are not operator locales and never receive
// localized operator email.
//
// This test turns that contract into a build-time invariant. Promote a locale to
// an email locale in locales/registry.json without shipping its template family
// (or add a stray template family no email locale points at) and this fails —
// instead of silently rendering English to operators who expect localized email.
func TestEmailFamiliesMatchRegistryContract(t *testing.T) {
	wantSet := make(map[string]struct{})
	for _, loc := range locales.AllLocales() {
		if locales.IsEmailLocale(loc.Canonical) {
			wantSet[loc.EmailFamily] = struct{}{}
		}
	}
	if len(wantSet) == 0 {
		t.Fatal("registry declares no email locales; expected at least en/es/es-AR")
	}
	want := sortedKeys(wantSet)

	got := append([]string(nil), templateLanguageFamilies()...)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("templateLanguageFamilies()=%v must equal the registry's email families=%v", got, want)
	}

	templatesRoot := resolveTemplatesRoot(t)
	layoutRoot := filepath.Join(templatesRoot, "..", "layout")
	for fam := range wantSet {
		dir := filepath.Join(templatesRoot, fam)
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("email-required family %q has no template directory at %s (err=%v)", fam, dir, err)
		}
		if len(listTemplateFiles(t, dir)) == 0 {
			t.Fatalf("email-required family %q template directory %s has no .html templates", fam, dir)
		}
		// loadTemplates also needs a per-family base layout to render anything.
		layout := filepath.Join(layoutRoot, "base_"+fam+".html")
		if _, err := os.Stat(layout); err != nil {
			t.Fatalf("email-required family %q is missing base layout %s: %v", fam, layout, err)
		}
	}
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
