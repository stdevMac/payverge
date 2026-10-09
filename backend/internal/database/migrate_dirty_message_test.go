package database

import (
	"fmt"
	"strings"
	"testing"
)

// TestDirtyMigrationError verifies that dirtyMigrationError produces an
// actionable, consistent message that:
//   - names the dirty version number so operators know exactly which migration
//     to inspect in schema_migrations,
//   - states the problem clearly ("dirty state"), and
//   - points to the recovery runbook so operators have a concrete next step
//     without needing to search documentation.
func TestDirtyMigrationError(t *testing.T) {
	const dirtyVersion = uint(42)
	err := dirtyMigrationError(dirtyVersion)
	if err == nil {
		t.Fatal("dirtyMigrationError returned nil; expected a non-nil error")
	}

	msg := err.Error()

	// The message must include the version number so operators know which
	// schema_migrations row to inspect or force.
	if !strings.Contains(msg, "42") {
		t.Errorf("error message does not contain dirty version (42): %q", msg)
	}

	// The message must communicate the root problem clearly.
	if !strings.Contains(msg, "dirty") {
		t.Errorf("error message does not mention 'dirty': %q", msg)
	}

	// The runbook path is the contract: it must appear verbatim so operators
	// can locate the doc without guessing.
	const runbookPath = "docs/runbooks/migration-dirty-recovery.md"
	if !strings.Contains(msg, runbookPath) {
		t.Errorf("error message does not reference the recovery runbook (%s): %q", runbookPath, msg)
	}
}

// TestDirtyMigrationErrorDifferentVersions guards against a regression where
// the version is hard-coded or the format string is accidentally swapped.
func TestDirtyMigrationErrorDifferentVersions(t *testing.T) {
	for _, v := range []uint{1, 63, 90, 999} {
		err := dirtyMigrationError(v)
		if err == nil {
			t.Fatalf("dirtyMigrationError(%d) returned nil", v)
		}
		msg := err.Error()
		needle := fmt.Sprintf("%d", v)
		if !strings.Contains(msg, needle) {
			t.Errorf("version %d not found in error message: %q", v, msg)
		}
	}
}
