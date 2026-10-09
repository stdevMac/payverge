package database

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReconcileReferenceUsesGenesisAndPendingMigrations(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(thisFile), "reconcile_reference.go")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(b)
	for _, required := range []string{"BootstrapGenesisSchema(gormDB)", "RunMigrations(", "VerifySchemaAtVersion("} {
		if !strings.Contains(source, required) {
			t.Errorf("reference reconciliation missing %q", required)
		}
	}
	for _, forbidden := range []string{".RunEnsure", ".RunAnalyticsMigration("} {
		if strings.Contains(source, forbidden) {
			t.Errorf("reference reconciliation still contains legacy schema owner %q", forbidden)
		}
	}
}
