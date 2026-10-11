package fiscal

import "testing"

// TestRowLockingSupportedOnlyPostgres locks F-CREDITRACE's dialect gate: the
// IssueCreditNote over-credit guard takes a SELECT ... FOR UPDATE on the original
// receipt to serialize concurrent partial credits, but FOR UPDATE is a Postgres
// feature — SQLite (used in tests) errors if the clause is rendered. The guard
// must only apply row locking on Postgres.
func TestRowLockingSupportedOnlyPostgres(t *testing.T) {
	if !rowLockingSupported("postgres") {
		t.Error("postgres must support row locking (FOR UPDATE)")
	}
	for _, d := range []string{"sqlite", "sqlite3", "mysql", ""} {
		if rowLockingSupported(d) {
			t.Errorf("dialect %q must not get FOR UPDATE", d)
		}
	}
}
