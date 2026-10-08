package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The schema, not only the app layer, refuses two active positions in one
// business whose names differ only by case or surrounding whitespace. Retired
// rows are outside the partial index, and businesses are independent.
func TestPositionsUniqueActiveName_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}
	gdb := startGenesisPostgres(t).DB
	first := seedGenesisBusiness(t, gdb, "positions-first")
	second := seedGenesisBusiness(t, gdb, "positions-second")

	require.NoError(t, gdb.Exec(`INSERT INTO positions (business_id, name, is_active) VALUES (?, 'Host', TRUE)`, first).Error)
	duplicate := gdb.Exec(`INSERT INTO positions (business_id, name, is_active) VALUES (?, ' hOsT ', TRUE)`, first).Error
	require.ErrorContains(t, duplicate, "duplicate key",
		"a folded duplicate of an active name must be rejected")
	require.NoError(t, gdb.Exec(`INSERT INTO positions (business_id, name, is_active) VALUES (?, 'Host', FALSE)`, first).Error,
		"a retired row may reuse the name — the index is partial on is_active")
	require.NoError(t, gdb.Exec(`INSERT INTO positions (business_id, name, is_active) VALUES (?, 'Host', TRUE)`, second).Error,
		"another business may use the same name")
}
