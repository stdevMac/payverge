package database

import (
	"os"
	"regexp"
	"testing"
)

// golang-migrate refuses to initialize a source directory that contains two
// files with the same version number ("duplicate migration file"), and
// RunMigrations is a fatal production startup path — a duplicate pair merged
// from two branches would prevent the backend from booting on its next
// deploy. This happened with 000200 (alt_payments_participant_addr_idx vs
// rewrite_pending_bill_numbers); the former was renumbered to 000202.
func TestMigrationVersionsAreUnique(t *testing.T) {
	dir := migrationsDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir %s: %v", dir, err)
	}
	pattern := regexp.MustCompile(`^(\d+)_.*\.(up|down)\.sql$`)
	seen := map[string]string{}
	for _, entry := range entries {
		match := pattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		key := match[1] + ":" + match[2]
		if prev, dup := seen[key]; dup {
			t.Errorf("duplicate migration version %s: %s and %s", match[1], prev, entry.Name())
		}
		seen[key] = entry.Name()
	}
}
