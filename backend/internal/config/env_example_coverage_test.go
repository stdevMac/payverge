package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// envExampleAllowlist is the only os.Getenv / os.LookupEnv literals excused
// from the repo-root .env.example. Each one is test, perf, or CI tooling,
// not a knob of a running server.
var envExampleAllowlist = map[string]struct{}{
	// Postgres DSN for backend/perf/seed (and the showcase-seed compose
	// profile, which builds the URL itself). The server uses DB_*.
	"DATABASE_URL": {},
	// Integration tests and the perf harness. cmd/app never reads these.
	"TEST_DATABASE_URL":     {},
	"TESTPERF_DATABASE_URL": {},
	// backend/cmd/email-synthetic deliverability probe, not the server.
	// The server takes --templates-dir instead of EMAIL_TEMPLATES_DIR.
	"EMAIL_SYNTHETIC_MODE":      {},
	"EMAIL_SYNTHETIC_RECIPIENT": {},
	"EMAIL_TEMPLATES_DIR":       {},
}

// directEnvReadRE matches a string literal passed directly to os.Getenv or
// os.LookupEnv. Indirect reads (os.Getenv(key), intEnv("X", n)) are out of
// scope: the call site does not pass the literal to os.Getenv.
var directEnvReadRE = regexp.MustCompile(`os\.(?:Getenv|LookupEnv)\(\s*"([A-Z][A-Z0-9_]*)"\s*\)`)

func TestEnvExampleCoversDirectOSGetenv(t *testing.T) {
	envPath := repoRootEnvExample(t)
	envText, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	found := scanDirectEnvReads(t, filepath.Join(filepath.Dir(envPath), "backend"))

	var missing []string
	for key, srcs := range found {
		if _, ok := envExampleAllowlist[key]; ok {
			continue
		}
		if !envExampleMentions(string(envText), key) {
			missing = append(missing, fmt.Sprintf("%s (read in %s)", key, strings.Join(srcs, ", ")))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf(".env.example is missing operator env keys:\n%s", strings.Join(missing, "\n"))
	}

	var stale []string
	for key := range envExampleAllowlist {
		if _, ok := found[key]; !ok {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("env example allowlist has keys no non-test backend file reads: %s", strings.Join(stale, ", "))
	}
}

// repoRootEnvExample walks up from the test working directory until it finds
// the monorepo-root .env.example. backend/.env.example is a stub that points
// at that file, so a directory counts only when it also contains backend/go.mod.
func repoRootEnvExample(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		candidate := filepath.Join(dir, ".env.example")
		if _, err := os.Stat(candidate); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
				return candidate
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo-root .env.example not found")
		}
		dir = parent
	}
}

func scanDirectEnvReads(t *testing.T, backend string) map[string][]string {
	t.Helper()
	found := map[string][]string{}
	err := filepath.WalkDir(backend, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(backend, path)
		if err != nil {
			return err
		}
		for _, m := range directEnvReadRE.FindAllSubmatch(body, -1) {
			key := string(m[1])
			found[key] = append(found[key], rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 {
		t.Fatal("scan found no os.Getenv/os.LookupEnv literals under backend/")
	}
	for key, srcs := range found {
		sort.Strings(srcs)
		found[key] = uniqueStrings(srcs)
	}
	return found
}

func envExampleMentions(text, key string) bool {
	return regexp.MustCompile(`(?:^|[^A-Za-z0-9_])` + regexp.QuoteMeta(key) + `(?:[^A-Za-z0-9_]|$)`).MatchString(text)
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
