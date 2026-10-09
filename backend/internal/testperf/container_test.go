package testperf_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

func TestStartPostgresReturnsUsableDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container test in -short")
	}
	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	var v int
	if err := pg.DB.Raw("SELECT 1").Scan(&v).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if v != 1 {
		t.Fatalf("want 1, got %d", v)
	}
}

func TestStartIsolatedPostgresReturnsUsableDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container test in -short")
	}
	ctx := context.Background()
	pg, err := testperf.StartIsolatedPostgres(ctx)
	if err != nil {
		t.Fatalf("start isolated postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	if pg.DatabaseName == "" || pg.Source == "" {
		t.Fatalf("missing harness identity: database=%q source=%q", pg.DatabaseName, pg.Source)
	}
	var databaseName string
	if err := pg.DB.Raw("SELECT current_database()").Scan(&databaseName).Error; err != nil {
		t.Fatalf("query current database: %v", err)
	}
	if databaseName != pg.DatabaseName {
		t.Fatalf("current_database() = %q, want %q", databaseName, pg.DatabaseName)
	}
}

func TestStartIsolatedPostgresCreatesAndDropsExternalChildDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container test in -short")
	}
	ctx := context.Background()
	var parent *testperf.Pg
	var err error
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) != "" ||
		strings.TrimSpace(os.Getenv("TESTPERF_DATABASE_URL")) != "" {
		parent, err = testperf.StartIsolatedPostgres(ctx)
	} else {
		t.Setenv("TEST_DATABASE_URL", "")
		parent, err = testperf.StartPostgres(ctx)
	}
	if err != nil {
		t.Fatalf("start parent postgres: %v", err)
	}
	t.Cleanup(func() { _ = parent.Terminate(ctx) })
	if err := parent.DB.Exec("CREATE TABLE parent_only_marker (id integer)").Error; err != nil {
		t.Fatalf("create parent marker: %v", err)
	}

	t.Setenv("TEST_DATABASE_URL", parent.DSN)
	child, err := testperf.StartIsolatedPostgres(ctx)
	if err != nil {
		t.Fatalf("start external child postgres: %v", err)
	}
	if child.Source != "external-child" || child.DatabaseName == parent.DatabaseName {
		t.Fatalf("child identity = source %q database %q", child.Source, child.DatabaseName)
	}
	var childMarkerCount int
	if err := child.DB.Raw(`SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename = 'parent_only_marker'`).Scan(&childMarkerCount).Error; err != nil {
		t.Fatalf("query child marker: %v", err)
	}
	if childMarkerCount != 0 {
		t.Fatalf("child database inherited %d parent-only tables", childMarkerCount)
	}

	if err := child.Terminate(ctx); err != nil {
		t.Fatalf("drop external child database: %v", err)
	}
	var parentMarkerCount int
	if err := parent.DB.Raw(`SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename = 'parent_only_marker'`).Scan(&parentMarkerCount).Error; err != nil {
		t.Fatalf("query parent marker after child cleanup: %v", err)
	}
	if parentMarkerCount != 1 {
		t.Fatalf("parent marker count = %d, want 1", parentMarkerCount)
	}
}
