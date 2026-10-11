package database

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// autoMigrateAllowlist is the exact set of non-test production Go files
// permitted to call .AutoMigrate(. It is empty: schema changes are numbered SQL
// migrations in backend/migrations/.
var autoMigrateAllowlist = map[string]struct{}{}

// TestAutoMigrateSourceGate freezes the production GORM AutoMigrate surface.
// Walks backend/**/*.go (skipping *_test.go and vendored/generated dirs) and
// fails if .AutoMigrate( appears outside the allowlist above.
func TestAutoMigrateSourceGate(t *testing.T) {
	root := sourceGateBackendRoot(t)

	var violations []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			// Skip vendored / generated / hidden tool dirs.
			switch name {
			case "vendor", "node_modules", ".git", "testdata", "generated":
				return filepath.SkipDir
			}
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		if _, ok := autoMigrateAllowlist[rel]; ok {
			return nil
		}

		f, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		// Large generated lines are rare in .go; grow buffer just in case.
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			if strings.Contains(scanner.Text(), ".AutoMigrate(") {
				violations = append(violations,
					rel+":"+strconv.Itoa(lineNo)+": new production GORM AutoMigrate calls are prohibited; add a numbered SQL migration instead")
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			return scanErr
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk backend tree: %v", err)
	}

	if len(violations) > 0 {
		t.Fatalf("AutoMigrate source-gate violations (%d):\n  %s\n\nRule: new production GORM AutoMigrate calls are prohibited; add a numbered SQL migration instead.\nAllowlist: none",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// sourceGateBackendRoot resolves backend/ from this test file via runtime.Caller
// (independent of process working directory).
func sourceGateBackendRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = backend/internal/database/automigrate_source_gate_test.go
	// → ../.. = backend/
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("backend root %s has no go.mod: %v", root, err)
	}
	return root
}
