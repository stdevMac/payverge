package database

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/schema/genesis"
)

// operational_alerts.alert_type / resource_type are enums enforced in Go. A
// SQL CHECK with a hardcoded IN-list drifts as soon as Go grows a new value,
// and every insert of the new type then fails on a migration-created
// database. These tests keep such CHECKs out of the baseline and out of any
// future numbered migration.

func TestGenesisOperationalAlertsHaveNoEnumChecks(t *testing.T) {
	body := genesisTableBody(t, "operational_alerts")
	for _, column := range []string{"alert_type", "resource_type"} {
		require.NotRegexpf(t, `(?is)CHECK[^\n]*\b`+column+`\b[^\n]*(\bIN\s*\(|=\s*ANY)`, body,
			"genesis operational_alerts must not CHECK %s against a fixed list", column)
		require.NotContainsf(t, genesis.SchemaSQL, "operational_alerts_"+column+"_check",
			"genesis must not carry the stale %s CHECK", column)
	}
}

// TestNoStaleOperationalAlertChecksSurvive scans every numbered up migration:
// any IN-list CHECK on operational_alerts.alert_type/resource_type must be
// superseded by a later migration that drops it. This catches someone
// reintroducing an enum CHECK in a future migration.
func TestNoStaleOperationalAlertChecksSurvive(t *testing.T) {
	dir := migrationsDir(t)
	ups, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	sort.Strings(ups) // zero-padded names sort in migration order

	checkRe := map[string]*regexp.Regexp{
		"alert_type":    regexp.MustCompile(`(?is)CHECK\s*\(\s*alert_type\s+IN\s*\(`),
		"resource_type": regexp.MustCompile(`(?is)CHECK\s*\(\s*resource_type\s+IN\s*\(`),
	}
	dropRe := map[string]*regexp.Regexp{
		"alert_type":    regexp.MustCompile(`(?is)DROP\s+CONSTRAINT\s+IF\s+EXISTS\s+operational_alerts_alert_type_check`),
		"resource_type": regexp.MustCompile(`(?is)DROP\s+CONSTRAINT\s+IF\s+EXISTS\s+operational_alerts_resource_type_check`),
	}

	lastCheck := map[string]string{} // column -> migration file that last added a CHECK
	for _, path := range ups {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		sql := string(data)
		if !strings.Contains(sql, "operational_alerts") {
			continue
		}
		for column := range checkRe {
			if checkRe[column].MatchString(sql) {
				lastCheck[column] = filepath.Base(path)
			}
			if dropRe[column].MatchString(sql) {
				delete(lastCheck, column)
			}
		}
	}

	for column, file := range lastCheck {
		t.Errorf("migration %s adds an IN-list CHECK on operational_alerts.%s that no later migration drops; the enum is enforced in Go, do not re-add SQL CHECKs", file, column)
	}
}
