package locales

import (
	"sort"
	"strings"
)

type Locale struct {
	Canonical                 string
	PathSegment               string
	SourceFolder              string
	DisplayName               string
	NativeName                string
	Direction                 string
	EmailFamily               string
	PromptFamily              string
	TranslationProviderTarget string
	PublicRoutes              string
	OperatorLocale            bool
	GuestLocale               bool
	Publishable               bool
	RequiredSurfaces          []string
}

func Default() Locale {
	return cloneLocale(generatedRegistry[defaultLocale])
}

func Lookup(code string) (Locale, bool) {
	locale, ok := generatedRegistry[strings.TrimSpace(code)]
	if !ok {
		return Locale{}, false
	}

	return cloneLocale(locale), true
}

func MustLookup(code string) Locale {
	locale, ok := Lookup(code)
	if !ok {
		return Default()
	}

	return locale
}

// IsSupported reports whether the code matches any locale in the registry,
// regardless of role. Prefer the role-specific predicates below at call sites
// that gate behavior — IsSupported conflates operator/dashboard locales with
// guest/storefront locales and once cost us the full guest-language set.
func IsSupported(code string) bool {
	_, ok := Lookup(code)

	return ok
}

// IsOperatorLocale reports whether the code is shippable to operators in the
// business dashboard (full message bundles, emails, prompts). Use this for
// auth cookies, route validation, and operator UI gating.
func IsOperatorLocale(code string) bool {
	locale, ok := Lookup(code)

	return ok && locale.OperatorLocale
}

// IsGuestLocale reports whether the code is shippable to guests on the public
// storefront (guest-messages JSON ships). Use this for menu translation
// targeting and guest UI gating.
func IsGuestLocale(code string) bool {
	locale, ok := Lookup(code)

	return ok && locale.GuestLocale
}

// IsPromptLocale reports whether the locale has human-reviewed prompt assets
// and can be used for AI prompt selection.
func IsPromptLocale(code string) bool {
	return requiresSurface(code, "prompts")
}

// IsEmailLocale reports whether the locale ships its own transactional email
// template family (its requiredSurfaces include "emails"). Locales without it
// are guest-storefront-only targets and intentionally fall back to English
// email — see TemplateManager.Render. Use this to keep templateLanguageFamilies
// and the on-disk template tree provably aligned with the registry contract.
func IsEmailLocale(code string) bool {
	return requiresSurface(code, "emails")
}

func requiresSurface(code, surface string) bool {
	locale, ok := Lookup(code)
	if !ok {
		return false
	}

	for _, declared := range locale.RequiredSurfaces {
		if declared == surface {
			return true
		}
	}

	return false
}

// GuestLocales returns every locale that ships guest-storefront translations,
// sorted by canonical code. Backend services seed BusinessLanguage / supported
// language rows from this set so the canonical registry is the single source.
func GuestLocales() []Locale {
	locales := make([]Locale, 0, len(generatedRegistry))
	for _, locale := range generatedRegistry {
		if locale.GuestLocale {
			locales = append(locales, cloneLocale(locale))
		}
	}

	sort.Slice(locales, func(i, j int) bool {
		return locales[i].Canonical < locales[j].Canonical
	})

	return locales
}

func AllLocales() []Locale {
	locales := make([]Locale, 0, len(generatedRegistry))
	for _, locale := range generatedRegistry {
		locales = append(locales, cloneLocale(locale))
	}

	sort.Slice(locales, func(i, j int) bool {
		return locales[i].Canonical < locales[j].Canonical
	})

	return locales
}

func EmailFamily(code string) string {
	return MustLookup(code).EmailFamily
}

func TranslationProviderTarget(code string) string {
	return MustLookup(code).TranslationProviderTarget
}

// TranslationFallbackChain returns stored-translation language codes to try
// when reading menu/content rows, most specific first. Guest chrome stays on
// the requested locale (es-AR); missing rows reuse the base-language copy
// (es) instead of falling through to English source text.
func TranslationFallbackChain(code string) []string {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}
	chain := []string{code}
	if i := strings.LastIndex(code, "-"); i > 0 {
		base := code[:i]
		if base != "" && base != code && IsGuestLocale(base) {
			chain = append(chain, base)
		}
	}
	return chain
}

func PublishableLocales() []Locale {
	locales := make([]Locale, 0, len(generatedRegistry))
	for _, locale := range generatedRegistry {
		if locale.Publishable {
			locales = append(locales, cloneLocale(locale))
		}
	}

	sort.Slice(locales, func(i, j int) bool {
		return locales[i].Canonical < locales[j].Canonical
	})

	return locales
}

func cloneLocale(locale Locale) Locale {
	locale.RequiredSurfaces = append([]string(nil), locale.RequiredSurfaces...)

	return locale
}
