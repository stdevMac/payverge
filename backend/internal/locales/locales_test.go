package locales

import "testing"

func TestLookupReturnsSpanishArgentinaMetadata(t *testing.T) {
	locale, ok := Lookup("es-AR")
	if !ok {
		t.Fatal("expected es-AR to be supported")
	}

	if locale.PathSegment != "es-ar" {
		t.Fatalf("expected es-AR path segment es-ar, got %q", locale.PathSegment)
	}

	if locale.EmailFamily != "es_ar" {
		t.Fatalf("expected es-AR email family es_ar, got %q", locale.EmailFamily)
	}

	if locale.PromptFamily != "es_ar" {
		t.Fatalf("expected es-AR prompt family es_ar, got %q", locale.PromptFamily)
	}

	if !containsString(locale.RequiredSurfaces, "frontend") {
		t.Fatal("expected es-AR required surfaces to include frontend")
	}

	if !containsString(locale.RequiredSurfaces, "backendValidation") {
		t.Fatal("expected es-AR required surfaces to include backendValidation")
	}
}

func TestUnknownLocaleIsUnsupported(t *testing.T) {
	// "xx-not-real" is intentionally absent from the registry. Real codes like
	// fr/de/etc. are now registered guest locales and would be supported.
	if IsSupported("xx-not-real") {
		t.Fatal("expected xx-not-real to be unsupported")
	}
}

// The role-set expectations below are DERIVED from locales/registry.json (the
// single source of truth) via registryRoles, not hand-maintained literals.
// Adding a locale to the registry therefore does not require touching these
// tests; and because the expectations come from the JSON while the predicates
// read the generated registry, a divergence between registry.json and the
// generated Go map still fails here (in addition to the dedicated drift test).

func TestIsOperatorLocaleOnlyMatchesDashboardLocales(t *testing.T) {
	roles := registryRoles(t)

	for code := range roles.operator {
		if !IsOperatorLocale(code) {
			t.Fatalf("expected %s to be an operator locale", code)
		}
	}

	for code := range roles.all {
		if roles.operator[code] {
			continue
		}
		if IsOperatorLocale(code) {
			t.Fatalf("expected %s NOT to be an operator locale (guest-only menu target)", code)
		}
	}
}

func TestIsGuestLocaleMatchesAllRegistryGuestLocales(t *testing.T) {
	roles := registryRoles(t)

	for code := range roles.guest {
		if !IsGuestLocale(code) {
			t.Fatalf("expected %s to be a guest locale (menu translation target)", code)
		}
	}

	if IsGuestLocale("xx-not-real") {
		t.Fatal("expected xx-not-real NOT to be a guest locale")
	}
}

func TestIsPromptLocaleOnlyMatchesPromptBackedLocales(t *testing.T) {
	roles := registryRoles(t)

	for code := range roles.prompts {
		if !IsPromptLocale(code) {
			t.Fatalf("expected %s to be a prompt-backed locale", code)
		}
	}

	for code := range roles.all {
		if roles.prompts[code] {
			continue
		}
		if IsPromptLocale(code) {
			t.Fatalf("expected %s NOT to be a prompt-backed locale", code)
		}
	}

	if IsPromptLocale("xx-not-real") {
		t.Fatal("expected xx-not-real NOT to be a prompt-backed locale")
	}
}

func TestIsEmailLocaleOnlyMatchesEmailBackedLocales(t *testing.T) {
	// Only operator locales declare the "emails" surface; every other locale is
	// a guest-storefront-only target and intentionally falls back to English
	// transactional email (no per-locale template family ships for them).
	roles := registryRoles(t)

	for code := range roles.emails {
		if !IsEmailLocale(code) {
			t.Fatalf("expected %s to be an email-backed locale", code)
		}
	}

	for code := range roles.all {
		if roles.emails[code] {
			continue
		}
		if IsEmailLocale(code) {
			t.Fatalf("expected %s NOT to be an email-backed locale (guest-only storefront target)", code)
		}
	}

	if IsEmailLocale("xx-not-real") {
		t.Fatal("expected xx-not-real NOT to be an email-backed locale")
	}
}

// Regression pin for the 2026-05-14 fix that conflated IsSupported (operator
// only) with menu-translation-target gating, silently dropping support for 18
// guest languages. The 2026-05-23 follow-up restored the remaining 5
// historical codes (vi/pl/sv/da/no) and added a guest bundle for es-AR.
//
// The expected set is DERIVED from registry.json so adding/removing a guest
// locale never requires editing this literal — it just has to stay aligned
// with the registry, which is the contract this pins.
func TestGuestLocalesIncludesFullSet(t *testing.T) {
	roles := registryRoles(t)

	guests := GuestLocales()
	if len(guests) != len(roles.guest) {
		t.Fatalf("expected %d guest locales (per registry.json), got %d", len(roles.guest), len(guests))
	}

	have := make(map[string]bool, len(guests))
	for _, locale := range guests {
		have[locale.Canonical] = true
	}

	for code := range roles.guest {
		if !have[code] {
			t.Fatalf("expected GuestLocales() to include %s (declared guestLocale in registry.json)", code)
		}
	}
}

func TestTranslationProviderTarget(t *testing.T) {
	// es-AR's menu-translation target MUST be "es", not "es-AR". Google Cloud
	// Translation v2 (NMT) accepts "es" but rejects region-qualified Spanish
	// like "es-AR"/"es-419" (zh-CN/zh-TW are the only region variants it takes).
	// With "es-AR" the API 400s, the error is swallowed in translateTextWithSource
	// (returns source text), and untranslated menu items get persisted as the
	// es-AR translation. Menu item names are nouns/prose — neutral "es" is the
	// correct target; the Rioplatense voseo lives in the static UI bundle, not in
	// machine-translated menu content.
	if got := TranslationProviderTarget("es-AR"); got != "es" {
		t.Fatalf("es-AR translation provider target must be \"es\" (Google rejects \"es-AR\"), got %q", got)
	}
	// zh stays region-qualified — Chinese is the supported exception.
	if got := TranslationProviderTarget("zh"); got != "zh-CN" {
		t.Fatalf("expected zh-CN translation provider target, got %q", got)
	}
}

func TestTranslationFallbackChain(t *testing.T) {
	// Documented guest menu read chain: es-AR → es. Argentine chrome stays
	// es-AR; missing es-AR translation rows reuse existing es copy instead of
	// falling through to English source text.
	got := TranslationFallbackChain("es-AR")
	if len(got) != 2 || got[0] != "es-AR" || got[1] != "es" {
		t.Fatalf("es-AR fallback chain must be [es-AR es], got %#v", got)
	}
	if got := TranslationFallbackChain("es"); len(got) != 1 || got[0] != "es" {
		t.Fatalf("es fallback chain must stay exact [es], got %#v", got)
	}
	if got := TranslationFallbackChain("en"); len(got) != 1 || got[0] != "en" {
		t.Fatalf("en fallback chain must stay exact [en], got %#v", got)
	}
	if got := TranslationFallbackChain("  "); len(got) != 0 {
		t.Fatalf("blank language has no fallback chain, got %#v", got)
	}
}

func TestLookupReturnsClonedRequiredSurfaces(t *testing.T) {
	restore := preserveGeneratedRequiredSurfaces("es-AR")
	defer restore()

	locale, ok := Lookup("es-AR")
	if !ok {
		t.Fatal("expected es-AR to be supported")
	}

	if len(locale.RequiredSurfaces) == 0 {
		t.Fatal("expected es-AR required surfaces to be populated")
	}

	original := locale.RequiredSurfaces[0]
	locale.RequiredSurfaces[0] = "mutated"

	next, ok := Lookup("es-AR")
	if !ok {
		t.Fatal("expected es-AR to remain supported")
	}

	if next.RequiredSurfaces[0] != original {
		t.Fatalf("expected lookup required surfaces to remain immutable, got %q", next.RequiredSurfaces[0])
	}
}

func TestAllLocalesReturnsDraftAndPublishableLocales(t *testing.T) {
	allLocales := AllLocales()

	if _, ok := findLocale(allLocales, "en"); !ok {
		t.Fatal("expected all locales to include en")
	}
	if _, ok := findLocale(allLocales, "es"); !ok {
		t.Fatal("expected all locales to include es")
	}
	if _, ok := findLocale(allLocales, "es-AR"); !ok {
		t.Fatal("expected all locales to include draft es-AR")
	}
}

func TestPublishableLocalesReturnsClonedRequiredSurfaces(t *testing.T) {
	locales := PublishableLocales()
	if len(locales) == 0 {
		t.Fatal("expected publishable locales")
	}

	locale := locales[0]
	restore := preserveGeneratedRequiredSurfaces(locale.Canonical)
	defer restore()

	if len(locale.RequiredSurfaces) == 0 {
		t.Fatalf("expected %s required surfaces to be populated", locale.Canonical)
	}

	original := locale.RequiredSurfaces[0]
	locale.RequiredSurfaces[0] = "mutated"

	next, ok := findLocale(PublishableLocales(), locale.Canonical)
	if !ok {
		t.Fatalf("expected %s to remain publishable", locale.Canonical)
	}

	if next.RequiredSurfaces[0] != original {
		t.Fatalf("expected publishable locale required surfaces to remain immutable, got %q", next.RequiredSurfaces[0])
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

func findLocale(locales []Locale, canonical string) (Locale, bool) {
	for _, locale := range locales {
		if locale.Canonical == canonical {
			return locale, true
		}
	}

	return Locale{}, false
}

func preserveGeneratedRequiredSurfaces(canonical string) func() {
	locale := generatedRegistry[canonical]
	original := append([]string(nil), locale.RequiredSurfaces...)

	return func() {
		locale := generatedRegistry[canonical]
		locale.RequiredSurfaces = append([]string(nil), original...)
		generatedRegistry[canonical] = locale
	}
}
