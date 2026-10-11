// Package genesisdb gives tests outside internal/database an isolated Postgres
// database at the production schema, built the way startup builds an empty
// database: the embedded genesis baseline, then any pending numbered
// migrations. It lives apart from testperf because internal/database's own
// tests import testperf, so testperf cannot import internal/database.
package genesisdb

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// MigrationsDir is the absolute path of backend/migrations.
func MigrationsDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile = backend/internal/testperf/genesisdb/genesisdb.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "migrations")
}

// Start returns an isolated database (a child database of TEST_DATABASE_URL,
// or a Testcontainers server) at the current production schema. The caller
// terminates it.
func Start(ctx context.Context) (*testperf.Pg, error) {
	pg, err := testperf.StartIsolatedPostgres(ctx)
	if err != nil {
		return nil, err
	}
	if err := database.BootstrapGenesisSchema(pg.DB); err != nil {
		_ = pg.Terminate(ctx)
		return nil, fmt.Errorf("bootstrap genesis: %w", err)
	}
	u, err := url.Parse(pg.DSN)
	if err != nil {
		_ = pg.Terminate(ctx)
		return nil, fmt.Errorf("parse DSN: %w", err)
	}
	pass, _ := u.User.Password()
	sslmode := u.Query().Get("sslmode")
	if sslmode == "" {
		sslmode = "disable"
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	if err := database.RunMigrations(u.Hostname(), port, u.User.Username(), pass,
		strings.TrimPrefix(u.Path, "/"), sslmode, MigrationsDir()); err != nil {
		_ = pg.Terminate(ctx)
		return nil, fmt.Errorf("apply pending migrations: %w", err)
	}
	return pg, nil
}
