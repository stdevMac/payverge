package perfgate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoBackendRoot walks up from this test file to the backend module root
// (the dir containing go.mod).
func repoBackendRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from test dir")
		}
		dir = parent
	}
}

// treePrefixesFromMakefile parses the bench-ci recipe in the Makefile and
// returns the `./tree/...` prefixes (trimmed of the trailing "...") that are
// covered by the recipe's go test invocation.
func treePrefixesFromMakefile(t *testing.T, root string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	// Slice out the bench-ci recipe (from "bench-ci:" to the next blank-after-tab block).
	body := string(raw)
	start := strings.Index(body, "\nbench-ci:")
	if start < 0 {
		t.Fatal("bench-ci target not found in Makefile")
	}
	var prefixes []string
	for _, line := range strings.Split(body[start:], "\n") {
		// Strip leading whitespace and trailing backslash-continuation and spaces.
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, "\\")
		line = strings.TrimSpace(line)
		// Match ./some/tree/... patterns (with or without a trailing space before \)
		if strings.HasPrefix(line, "./") &&
			(strings.Contains(line, "internal/") || strings.Contains(line, "perf/bench")) {
			// The field may be "./internal/server/..." — grab just the path token.
			token := strings.Fields(line)[0]
			if strings.HasSuffix(token, "/...") {
				prefixes = append(prefixes, strings.TrimSuffix(token, "..."))
			}
		}
	}
	return prefixes
}

// TestBenchCICoversEveryPerfTestTree asserts that every directory containing a
// *_perf_test.go file is covered by a tree argument in the bench-ci Makefile
// recipe. This prevents benchmark-bearing packages from silently being excluded
// from CI perf runs.
func TestBenchCICoversEveryPerfTestTree(t *testing.T) {
	root := repoBackendRoot(t)
	prefixes := treePrefixesFromMakefile(t, root)

	if len(prefixes) == 0 {
		t.Fatal("no bench-ci tree prefixes found in Makefile — recipe may have changed format")
	}

	var missing []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(d.Name(), "_perf_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		treeRel := "./" + filepath.ToSlash(rel) + "/" // e.g. ./internal/services/
		covered := false
		for _, p := range prefixes {
			if strings.HasPrefix(treeRel, p) {
				covered = true
				break
			}
		}
		if !covered {
			missing = append(missing, treeRel)
		}
		return nil
	})
	if len(missing) > 0 {
		t.Fatalf("perf-test dirs not covered by bench-ci trees: %v\n\nAdd the missing trees to the bench-ci recipe in Makefile.", missing)
	}
}
