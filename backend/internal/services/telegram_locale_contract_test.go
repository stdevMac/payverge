package services

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/locales"
)

// The Telegram-notification surface contract, mirroring the email surface
// contract (emails.TestEmailFamiliesMatchRegistryContract). Email localization
// is the canonical pattern and the Telegram tier follows it: language resolves
// on the registry's EmailFamily axis, so every family an email locale declares
// must map onto its OWN Telegram notification table and lifecycle table.
//
// Promote a locale to an email locale in locales/registry.json without
// teaching resolveTelegramLocale about its family and this fails — instead of
// operators silently receiving English Telegram notifications while their
// email is localized.
func TestTelegramTablesMatchEmailFamilyContract(t *testing.T) {
	familyExample := make(map[string]string) // EmailFamily → example canonical code
	for _, loc := range locales.AllLocales() {
		if locales.IsEmailLocale(loc.Canonical) {
			familyExample[loc.EmailFamily] = loc.Canonical
		}
	}
	if len(familyExample) == 0 {
		t.Fatal("registry declares no email locales; expected at least en/es/es-AR")
	}

	claimed := make(map[telegramLocale]string) // telegram table → claiming family
	for family, code := range familyExample {
		resolved := resolveTelegramLocale(code)
		if prev, dup := claimed[resolved]; dup {
			t.Fatalf(
				"email families %q and %q both resolve to telegram locale %q — a new email family is falling back onto another family's table; add its own table and resolveTelegramLocale case",
				prev, family, resolved,
			)
		}
		claimed[resolved] = family
		if _, ok := telegramTemplatesByLocale[resolved]; !ok {
			t.Fatalf("email family %q resolves to telegram locale %q which has no entry in telegramTemplatesByLocale", family, resolved)
		}
		if _, ok := telegramLifecycleCopyByLocale[resolved]; !ok {
			t.Fatalf("email family %q resolves to telegram locale %q which has no entry in telegramLifecycleCopyByLocale", family, resolved)
		}
	}
}

// emailChannelFamily reproduces the email channel's language→family resolution
// (emails.normalizeTemplateLanguage) using only exported helpers: lowercase +
// trim, treat the result as a family if it already IS a shipping family, else
// fold via locales.EmailFamily. This is the canonical channel; the Telegram
// channel must agree with it so an operator can never get Spanish email but
// English Telegram (or vice versa).
func emailChannelFamily(lang string) string {
	normalized := strings.ToLower(strings.TrimSpace(lang))
	switch normalized {
	case "eng", "es", "es_ar":
		return normalized
	}
	// locales.EmailFamily folds underscores/hyphens and case the same way the
	// email path does (it Looks up the canonical key); unknown → default family.
	if normalized == "" {
		return locales.Default().EmailFamily
	}
	// Normalize underscore form to the registry's hyphenated canonical so a
	// stored "es_ar"/"es-ar"/"ES-AR" resolves the way the email channel does.
	candidate := strings.ReplaceAll(normalized, "_", "-")
	if loc, ok := lookupFoldedLocale(candidate); ok {
		return loc.EmailFamily
	}
	return locales.Default().EmailFamily
}

// telegramLocaleForEmailFamily maps an email family onto the Telegram table the
// parity contract requires.
func telegramLocaleForEmailFamily(family string) telegramLocale {
	switch family {
	case "es":
		return telegramLocaleES
	case "es_ar":
		return telegramLocaleESAR
	default:
		return telegramLocaleEN
	}
}

// lookupFoldedLocale resolves a lowercased, hyphen-normalized tag to a registry
// locale by case-insensitive comparison against canonical codes (the registry
// is keyed on canonical forms like "es-AR", so a lowercased "es-ar" misses an
// exact Lookup). Used only by the test to compute the expected email family.
func lookupFoldedLocale(folded string) (locales.Locale, bool) {
	for _, loc := range locales.AllLocales() {
		if strings.EqualFold(loc.Canonical, folded) {
			return loc, true
		}
	}
	return locales.Locale{}, false
}

// TestResolveTelegramLocale_AgreesWithEmailChannel is the TG-1 regression guard:
// for the same stored business.DefaultLanguage value, the Telegram locale chosen
// must agree with the email family chosen by the email channel. Before the fix,
// resolveTelegramLocale used an EXACT registry lookup (case/form sensitive), so
// "es_ar"/"es-ar"/"ES" silently fell to English Telegram while the email channel
// localized to Spanish. The two channels must never diverge.
func TestResolveTelegramLocale_AgreesWithEmailChannel(t *testing.T) {
	inputs := []string{"es-AR", "es_ar", "es-ar", "ES-AR", "ES", "es", "en", "EN", " es-AR ", ""}
	for _, in := range inputs {
		t.Run("lang="+in, func(t *testing.T) {
			wantFamily := emailChannelFamily(in)
			want := telegramLocaleForEmailFamily(wantFamily)
			got := resolveTelegramLocale(in)
			if got != want {
				t.Fatalf("resolveTelegramLocale(%q) = %q, but email channel resolves family %q → telegram %q; channels diverged", in, got, wantFamily, want)
			}
		})
	}
}
