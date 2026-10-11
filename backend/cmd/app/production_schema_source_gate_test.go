package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestProductionStartupHasNoAdHocDDL keeps production schema ownership in the
// embedded genesis baseline plus numbered SQL migrations. Reference-schema and
// unit-test builders may still use GORM AutoMigrate, but cmd/app must not call
// any schema-mutating compatibility helper after connecting.
func TestProductionStartupHasNoAdHocDDL(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	mainPath := filepath.Join(filepath.Dir(thisFile), "main.go")
	source, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read %s: %v", mainPath, err)
	}

	forbidden := []string{
		".RunEnsure",
		"auth.RunAuthMigration(",
		"auth.UpdateUserSchema(",
		".RunAnalyticsMigration(",
		".RunDropBlogTablesMigration(",
		".RunLoyaltyProgramMigration(",
	}
	for _, needle := range forbidden {
		if strings.Contains(string(source), needle) {
			t.Errorf("production startup still contains schema-mutating call %q", needle)
		}
	}
	for _, required := range []string{"case database.DBLegacy:", "refusing startup"} {
		if !strings.Contains(string(source), required) {
			t.Errorf("production startup missing inconsistent-database guard %q", required)
		}
	}
}
