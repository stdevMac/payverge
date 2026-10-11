package locales

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
)

// registryFileEntry mirrors a single locale object in locales/registry.json.
// It is intentionally a separate, hand-written shape (not the generated Locale)
// so this test re-derives the expected registry FROM the JSON source of truth
// rather than from any generated artifact — catching codegen drift even when
// the TypeScript toolchain never ran.
type registryFileEntry struct {
	Canonical                 string   `json:"canonical"`
	PathSegment               string   `json:"pathSegment"`
	SourceFolder              string   `json:"sourceFolder"`
	DisplayName               string   `json:"displayName"`
	NativeName                string   `json:"nativeName"`
	Direction                 string   `json:"direction"`
	EmailFamily               string   `json:"emailFamily"`
	PromptFamily              string   `json:"promptFamily"`
	TranslationProviderTarget string   `json:"translationProviderTarget"`
	PublicRoutes              string   `json:"publicRoutes"`
	OperatorLocale            bool     `json:"operatorLocale"`
	GuestLocale               bool     `json:"guestLocale"`
	Publishable               bool     `json:"publishable"`
	RequiredSurfaces          []string `json:"requiredSurfaces"`
}

type registryFile struct {
	Version       int                          `json:"version"`
	DefaultLocale string                       `json:"defaultLocale"`
	Locales       map[string]registryFileEntry `json:"locales"`
}

// TestGeneratedRegistryMatchesRegistryJSON is the Go-side drift guard for the
// i18n codegen (frontend/scripts/i18n/generate.ts). It reads locales/registry.json,
// the canonical single source of truth, and asserts the in-code generatedRegistry
// map (registry_generated.go, "// Code generated ... DO NOT EDIT.") agrees with
// it field-for-field for every locale. If someone edits registry.json without
// re-running `npm run i18n:generate`, this fails on the backend side even without
// the TS toolchain.
func TestGeneratedRegistryMatchesRegistryJSON(t *testing.T) {
	parsed := loadRegistryJSON(t)

	if parsed.DefaultLocale != defaultLocale {
		t.Fatalf(
			"defaultLocale drift: registry.json=%q generated=%q (run `npm run i18n:generate`)",
			parsed.DefaultLocale, defaultLocale,
		)
	}

	if len(parsed.Locales) != len(generatedRegistry) {
		t.Fatalf(
			"locale count drift: registry.json=%d generated=%d (run `npm run i18n:generate`)",
			len(parsed.Locales), len(generatedRegistry),
		)
	}

	for canonical, fileEntry := range parsed.Locales {
		generated, ok := generatedRegistry[canonical]
		if !ok {
			t.Errorf(
				"registry.json defines locale %q but generatedRegistry does not (run `npm run i18n:generate`)",
				canonical,
			)
			continue
		}

		expected := Locale{
			Canonical:                 fileEntry.Canonical,
			PathSegment:               fileEntry.PathSegment,
			SourceFolder:              fileEntry.SourceFolder,
			DisplayName:               fileEntry.DisplayName,
			NativeName:                fileEntry.NativeName,
			Direction:                 fileEntry.Direction,
			EmailFamily:               fileEntry.EmailFamily,
			PromptFamily:              fileEntry.PromptFamily,
			TranslationProviderTarget: fileEntry.TranslationProviderTarget,
			PublicRoutes:              fileEntry.PublicRoutes,
			OperatorLocale:            fileEntry.OperatorLocale,
			GuestLocale:               fileEntry.GuestLocale,
			Publishable:               fileEntry.Publishable,
			RequiredSurfaces:          fileEntry.RequiredSurfaces,
		}

		if !reflect.DeepEqual(generated, expected) {
			t.Errorf(
				"generatedRegistry[%q] drifted from registry.json (run `npm run i18n:generate`):\n  generated: %+v\n  registry:  %+v",
				canonical, generated, expected,
			)
		}
	}

	// Catch generated locales that registry.json no longer declares.
	for canonical := range generatedRegistry {
		if _, ok := parsed.Locales[canonical]; !ok {
			t.Errorf(
				"generatedRegistry defines locale %q absent from registry.json (run `npm run i18n:generate`)",
				canonical,
			)
		}
	}
}

// TestGeneratedRegistryRoleSetsMatchRegistryJSON re-derives the operator / guest
// / publishable / email / prompt / storefront role sets directly from
// registry.json and asserts the generated registry's role predicates agree. This
// makes a forgotten regenerate (or a hand-edited generated file) fail on the
// backend side independent of any hardcoded literal.
func TestGeneratedRegistryRoleSetsMatchRegistryJSON(t *testing.T) {
	roles := registryRoles(t)

	expected := map[string]map[string]bool{
		"operator":    roles.operator,
		"guest":       roles.guest,
		"publishable": roles.publishable,
		"emails":      roles.emails,
		"prompts":     roles.prompts,
		"storefront":  roles.storefront,
	}

	actual := map[string][]string{
		"operator":    canonicalsOf(AllLocales(), func(l Locale) bool { return l.OperatorLocale }),
		"guest":       canonicalsOf(GuestLocales(), func(Locale) bool { return true }),
		"publishable": canonicalsOf(PublishableLocales(), func(Locale) bool { return true }),
		"emails":      canonicalsOf(AllLocales(), func(l Locale) bool { return IsEmailLocale(l.Canonical) }),
		"prompts":     canonicalsOf(AllLocales(), func(l Locale) bool { return IsPromptLocale(l.Canonical) }),
		"storefront":  canonicalsOf(AllLocales(), func(l Locale) bool { return requiresSurface(l.Canonical, "guestStorefront") }),
	}

	for role, want := range expected {
		got := sortedSet(actual[role])
		wantSorted := sortedKeys(want)
		if !reflect.DeepEqual(got, wantSorted) {
			t.Errorf(
				"%s role set drifted from registry.json (run `npm run i18n:generate`):\n  generated: %v\n  registry:  %v",
				role, got, wantSorted,
			)
		}
	}
}

// registryRoleSets carries the per-role canonical-code sets derived directly
// from locales/registry.json so tests can assert behavior against the source of
// truth instead of hand-maintained literals. Shared by locales_test.go.
type registryRoleSets struct {
	all         map[string]bool
	operator    map[string]bool
	guest       map[string]bool
	publishable map[string]bool
	emails      map[string]bool
	prompts     map[string]bool
	storefront  map[string]bool
}

// registryRoles parses locales/registry.json and projects every role set used
// by the locales tests. Deriving these from the JSON (rather than duplicating
// the 21-locale lists) means adding a locale is a one-line registry edit, not a
// five-place test edit.
func registryRoles(t *testing.T) registryRoleSets {
	t.Helper()

	parsed := loadRegistryJSON(t)

	roles := registryRoleSets{
		all:         map[string]bool{},
		operator:    map[string]bool{},
		guest:       map[string]bool{},
		publishable: map[string]bool{},
		emails:      map[string]bool{},
		prompts:     map[string]bool{},
		storefront:  map[string]bool{},
	}

	for canonical, entry := range parsed.Locales {
		roles.all[canonical] = true
		if entry.OperatorLocale {
			roles.operator[canonical] = true
		}
		if entry.GuestLocale {
			roles.guest[canonical] = true
		}
		if entry.Publishable {
			roles.publishable[canonical] = true
		}
		for _, surface := range entry.RequiredSurfaces {
			switch surface {
			case "emails":
				roles.emails[canonical] = true
			case "prompts":
				roles.prompts[canonical] = true
			case "guestStorefront":
				roles.storefront[canonical] = true
			}
		}
	}

	return roles
}

func loadRegistryJSON(t *testing.T) registryFile {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve current test file path")
	}

	// backend/internal/locales/registry_drift_test.go -> repo root is three
	// directories up (locales -> internal -> backend -> repo root).
	repoRoot := filepath.Join(filepath.Dir(currentFile), "..", "..", "..")
	registryPath := filepath.Join(repoRoot, "locales", "registry.json")

	raw, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("read registry.json (%s): %v", registryPath, err)
	}

	var parsed registryFile
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse registry.json: %v", err)
	}

	if parsed.Version != 1 {
		t.Fatalf("unexpected registry.json version: %d", parsed.Version)
	}

	if len(parsed.Locales) == 0 {
		t.Fatal("registry.json declared no locales")
	}

	return parsed
}

func canonicalsOf(locales []Locale, keep func(Locale) bool) []string {
	out := make([]string, 0, len(locales))
	for _, locale := range locales {
		if keep(locale) {
			out = append(out, locale.Canonical)
		}
	}

	return out
}

func sortedSet(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)

	return out
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)

	return out
}
