package geodefaults

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"
)

// The frontend mirrors this package's countryTable in
// frontend/src/lib/geoDefaults.ts (COUNTRY_OPTIONS) because Next.js inlines
// the signup preview + registration_data seed at build time. If the two ever
// drift, the funnel previews one currency/timezone and the backend seeds
// another. This guard parses the frontend constant and fails the moment the
// mirrors disagree (same approach as services' plan-pricing drift test).
func TestFrontendCountryOptionsMatchBackendTable(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	// backend/internal/geodefaults -> repo root
	repoRoot := filepath.Join(filepath.Dir(currentFile), "..", "..", "..")
	fePath := filepath.Join(repoRoot, "frontend", "src", "lib", "geoDefaults.ts")

	raw, err := os.ReadFile(fePath)
	if err != nil {
		t.Fatalf("read frontend geoDefaults.ts (%s): %v", fePath, err)
	}

	entryRe := regexp.MustCompile(`\{ code: "([A-Z]{2})", name: "[^"]+", currency: "([A-Z]{3})", timezone: "([^"]+)" \}`)
	matches := entryRe.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatal("parsed zero COUNTRY_OPTIONS entries; the regex or the file shape changed — fix the parser, do not delete this guard")
	}

	feCodes := map[string]bool{}
	for _, m := range matches {
		code, currency, timezone := m[1], m[2], m[3]
		feCodes[code] = true

		be, ok := countryTable[code]
		if !ok {
			t.Errorf("frontend lists %q but backend countryTable does not", code)
			continue
		}
		if be.Currency != currency {
			t.Errorf("%s currency drift: frontend %q vs backend %q", code, currency, be.Currency)
		}
		if be.Timezone != timezone {
			t.Errorf("%s timezone drift: frontend %q vs backend %q", code, timezone, be.Timezone)
		}
		if _, err := time.LoadLocation(timezone); err != nil {
			t.Errorf("%s timezone %q is not a loadable IANA zone: %v", code, timezone, err)
		}
	}

	for code := range countryTable {
		if !feCodes[code] {
			t.Errorf("backend countryTable lists %q but frontend COUNTRY_OPTIONS does not", code)
		}
	}
}
