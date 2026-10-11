package server

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// Match only the KIT_ORDER array declaration, not the KitId union type or the
// KITS record, so a type-only edit elsewhere in the file can't accidentally
// satisfy (or spuriously fail) this check.
var kitOrderPattern = regexp.MustCompile(`(?s)export const KIT_ORDER: KitId\[\]\s*=\s*\[(.*?)\];`)

var kitIDPattern = regexp.MustCompile(`"([^"]+)"`)

var (
	errKitOrderNotFound = errors.New("could not find `export const KIT_ORDER: KitId[] = [...]`")
	errKitOrderNoIDs    = errors.New("parsed zero kit ids out of KIT_ORDER")
)

// TestMarketingKitsWhitelistParity guards against the backend's marketingKits
// whitelist (marketing_activity_handlers.go) drifting from the frontend's
// KIT_ORDER (artDirection/kits.ts). Today only a code comment on each side
// asks a human to keep them in sync; if a future wave adds a kit on one side
// without the matching change on the other, posting silently 400s on a kit id
// that looks valid to the caller. This test parses the frontend source
// directly (a small regex, not a TypeScript parser) rather than duplicating
// the list in Go, so there is exactly one place a drift can hide: the two
// source files themselves.
//
// Caching caveat — this Go test can report a stale PASS: kits.ts lives outside
// the Go module, so Go's build/test cache does not hash it as a dependency.
// ANY run without `-count=1` can replay a cached PASS recorded before a
// frontend edit, including CI's own `cd backend && go test -race ./...` in
// .github/workflows/ci-gates.yml — that workflow uses actions/setup-go, whose
// `cache` input defaults to true and which restores GOCACHE (where Go stores
// test results) on a key derived from backend/go.sum, and go.sum does not
// change when kits.ts changes. So CI is not a cold cache and must not be
// relied on to re-run this.
//
// Practical consequence: if you change kits.ts, re-run this test explicitly
// with `-count=1` (`go test ./internal/server/ -run TestMarketingKits
// -count=1`). The guard that actually gates CI is the mirrored Jest test at
// frontend/src/components/business/Marketing/artDirection/kits.parity.test.ts,
// which has no equivalent stale-cache problem; this Go test is the secondary
// one.
func TestMarketingKitsWhitelistParity(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed to resolve this test file's own path")
	}
	// Anchor to this test file's location rather than the process cwd. Plain
	// cwd-relative filepath.Join works under ordinary `go test` (Go sets cwd
	// to the package directory) but breaks under a pre-compiled test binary
	// (`go test -c` then run from elsewhere), which CI can do.
	kitsPath := filepath.Join(
		filepath.Dir(thisFile),
		"..", "..", "..",
		"frontend", "src", "components", "business", "Marketing", "artDirection", "kits.ts",
	)

	raw, err := os.ReadFile(kitsPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("frontend checkout not present at %s; skipping kit whitelist parity check", kitsPath)
		}
		t.Fatalf("reading %s: %v", kitsPath, err)
	}

	frontendKits, err := parseKitOrderIDs(string(raw))
	if err != nil {
		t.Fatalf(
			"%v in %s; this test's regex needs updating to match the current file shape",
			err, kitsPath,
		)
	}

	if !stringSlicesEqualInOrder(marketingKits, frontendKits) {
		t.Fatalf(
			"backend/internal/server/marketing_activity_handlers.go marketingKits and "+
				"frontend/src/components/business/Marketing/artDirection/kits.ts KIT_ORDER are out of sync "+
				"(order matters). Update both files together.\n"+
				"backend marketingKits:  %v\n"+
				"frontend KIT_ORDER:     %v",
			marketingKits, frontendKits,
		)
	}
}

// parseKitOrderIDs extracts the kit ids from kits.ts's KIT_ORDER declaration.
// It returns an error — never an empty-but-successful result — for any shape
// it does not recognize, so a frontend refactor surfaces as a loud failure
// rather than a vacuously passing parity check.
func parseKitOrderIDs(src string) ([]string, error) {
	match := kitOrderPattern.FindStringSubmatch(src)
	if match == nil {
		return nil, errKitOrderNotFound
	}

	idMatches := kitIDPattern.FindAllStringSubmatch(stripSourceComments(match[1]), -1)
	if len(idMatches) == 0 {
		return nil, errKitOrderNoIDs
	}

	ids := make([]string, 0, len(idMatches))
	for _, m := range idMatches {
		ids = append(ids, m[1])
	}
	return ids, nil
}

// stripSourceComments removes `//` line comments and `/* */` block comments
// while leaving double-quoted string literals intact. Without it a parked line
// inside the literal — `// TODO: add "neon" next wave` — scrapes as a real id
// and reports a confidently wrong "out of sync" drift that would send whoever
// hits it looking for a backend change that was never missing.
func stripSourceComments(src string) string {
	var out strings.Builder
	out.Grow(len(src))
	inString := false

	for i := 0; i < len(src); i++ {
		char := src[i]

		if inString {
			if char == '\\' && i+1 < len(src) {
				out.WriteByte(char)
				i++
				out.WriteByte(src[i])
				continue
			}
			if char == '"' {
				inString = false
			}
			out.WriteByte(char)
			continue
		}

		if char == '"' {
			inString = true
			out.WriteByte(char)
			continue
		}

		if char == '/' && i+1 < len(src) {
			if src[i+1] == '/' {
				for i < len(src) && src[i] != '\n' {
					i++
				}
				// Keep the newline so line structure (and any following code)
				// survives; the loop's i++ steps past it.
				if i < len(src) {
					out.WriteByte('\n')
				}
				continue
			}
			if src[i+1] == '*' {
				end := strings.Index(src[i+2:], "*/")
				if end < 0 {
					// Unterminated block comment: everything after it is
					// commented out.
					return out.String()
				}
				i += 2 + end + 1
				continue
			}
		}

		out.WriteByte(char)
	}

	return out.String()
}

func TestMarketingKitsParseKitOrderIDsIgnoresCommentedIDs(t *testing.T) {
	src := `export const KIT_ORDER: KitId[] = [
  "editorial",
  // TODO: add "neon" next wave
  "bold",
  /* parked: "sepia" */
  "minimal", // keep last for now
];`

	got, err := parseKitOrderIDs(src)
	if err != nil {
		t.Fatalf("parseKitOrderIDs returned an unexpected error: %v", err)
	}
	if !stringSlicesEqualInOrder(got, []string{"editorial", "bold", "minimal"}) {
		t.Fatalf("commented-out ids leaked into the parsed list: %v", got)
	}
}

// TestMarketingKitsParseKitOrderIDsFailsLoudly pins the shapes this guard must
// keep rejecting outright. Every one of these is a plausible frontend refactor
// that the regex cannot read; each must return an error so the parity test
// t.Fatalf's, rather than parsing an empty or partial list and passing.
func TestMarketingKitsParseKitOrderIDsFailsLoudly(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"readonly modifier", `export const KIT_ORDER: readonly KitId[] = ["editorial", "bold"];`},
		{"as const", `export const KIT_ORDER: KitId[] = ["editorial", "bold"] as const;`},
		{"generic Array", `export const KIT_ORDER: Array<KitId> = ["editorial", "bold"];`},
		{"satisfies", `export const KIT_ORDER = ["editorial", "bold"] satisfies KitId[];`},
		{"derived from KITS", `export const KIT_ORDER: KitId[] = Object.keys(KITS) as KitId[];`},
		{"spaces removed", `export const KIT_ORDER:KitId[]=["editorial","bold"];`},
		{"single quotes", `export const KIT_ORDER: KitId[] = ['editorial', 'bold'];`},
		{"ids hoisted to constants", `export const KIT_ORDER: KitId[] = [EDITORIAL, BOLD];`},
		{"every id commented out", "export const KIT_ORDER: KitId[] = [\n  // \"editorial\",\n];"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ids, err := parseKitOrderIDs(tc.src)
			if err == nil {
				t.Fatalf("expected a loud parse failure, got ids %v", ids)
			}
		})
	}
}

func stringSlicesEqualInOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
