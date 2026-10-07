package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// irreversibleMarker is the exact annotation that must appear in any .down.sql
// file that lacks real revert DDL. The marker documents that the migration was
// intentionally left without a rollback path.
const irreversibleMarker = "-- IRREVERSIBLE"

// hasRealDDL returns true if the file content contains at least one non-blank,
// non-comment line — i.e. actual SQL statements (not just comment lines).
func hasRealDDL(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		return true
	}
	return false
}

// hasIrreversibleMarker returns true if the file contains the canonical
// IRREVERSIBLE annotation on its own comment line.
func hasIrreversibleMarker(content string) bool {
	return strings.Contains(content, irreversibleMarker)
}

// TestDownMigrationPolicy asserts that every .down.sql in backend/migrations/
// either contains real revert DDL (non-blank, non-comment SQL) or carries the
// explicit "-- IRREVERSIBLE" annotation. Placeholder files that silently no-op
// without documentation are rejected — they mislead operators who attempt a
// rollback and expect the down migration to undo the schema change.
func TestDownMigrationPolicy(t *testing.T) {
	dir := migrationsDir(t)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("cannot read migrations dir %s: %v", dir, err)
	}

	var violations []string

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".down.sql") {
			continue
		}

		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("cannot read %s: %v", name, err)
			continue
		}
		content := string(raw)

		if hasRealDDL(content) || hasIrreversibleMarker(content) {
			continue
		}

		violations = append(violations, name)
	}

	assert.Empty(t, violations,
		"the following .down.sql files contain no real revert DDL and no %q annotation.\n"+
			"Either add a proper rollback statement or add a %q comment line to document\n"+
			"that the migration cannot be safely reversed:\n",
		irreversibleMarker,
		irreversibleMarker,
	)
}

// TestIrreversibleMarkerIsExactString guards the canonical marker string from
// accidental typos or reformatting across refactors.
func TestIrreversibleMarkerIsExactString(t *testing.T) {
	const want = "-- IRREVERSIBLE"
	if irreversibleMarker != want {
		t.Errorf("irreversibleMarker = %q; want %q", irreversibleMarker, want)
	}
}
