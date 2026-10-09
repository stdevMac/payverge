package database

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/schema/genesis"
)

// TestGenesisSessionResetSQL covers the session GUCs the embedded pg_dump
// sets before any DDL. client_encoding, standard_conforming_strings, and the
// search_path set_config are restored elsewhere and are not part of the RESET.
func TestGenesisSessionResetSQL(t *testing.T) {
	for _, name := range []string{
		"statement_timeout",
		"lock_timeout",
		"idle_in_transaction_session_timeout",
		"row_security",
	} {
		if !strings.Contains(genesisSessionResetSQL, "RESET "+name) {
			t.Errorf("genesisSessionResetSQL missing RESET %s\nSQL: %s", name, genesisSessionResetSQL)
		}
	}

	excludedSET := map[string]bool{
		"client_encoding":             true,
		"standard_conforming_strings": true,
	}

	foundSET := false
	for _, line := range genesisPreambleLines(t, genesis.SchemaSQL) {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "set_config") && strings.Contains(trimmed, "search_path") {
			continue
		}
		if !strings.HasPrefix(trimmed, "SET ") {
			t.Fatalf("unexpected preamble line %q", trimmed)
		}
		name := genesisSetName(trimmed)
		if name == "" {
			t.Fatalf("could not parse SET line %q", trimmed)
		}
		if excludedSET[name] {
			continue
		}
		foundSET = true
		if !strings.Contains(genesisSessionResetSQL, "RESET "+name) {
			t.Errorf("preamble SET %s has no matching RESET in %s", name, genesisSessionResetSQL)
		}
	}
	if !foundSET {
		t.Fatal("found no SET lines at the top of the embedded genesis schema")
	}
}

// genesisPreambleLines returns the leading session-setup lines of a pg_dump:
// SET commands and the search_path set_config, stopping at the first other
// statement. Blank lines and comments are skipped.
func genesisPreambleLines(t *testing.T, schema string) []string {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(schema, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		if strings.HasPrefix(trimmed, "SET ") ||
			(strings.Contains(trimmed, "set_config") && strings.Contains(trimmed, "search_path")) {
			lines = append(lines, line)
			continue
		}
		break
	}
	if len(lines) == 0 {
		t.Fatal("embedded genesis schema has an empty session preamble")
	}
	return lines
}

func genesisSetName(setLine string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(setLine), "SET "))
	name := rest
	if i := strings.IndexAny(rest, " =;"); i >= 0 {
		name = rest[:i]
	}
	return strings.TrimSpace(name)
}
