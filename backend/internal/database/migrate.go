package database

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// RunMigrations uses golang-migrate to apply versioned SQL migrations from
// the given directory on top of the genesis baseline. It tracks applied
// versions in the schema_migrations table and fails fast if any migration
// cannot be applied, preventing the application from running against a
// partially-migrated database.
//
// An empty migrations directory is valid: the genesis baseline is the whole
// schema, so there is nothing to apply. The dirty flag is still checked.
func RunMigrations(dbHost, dbPort, dbUser, dbPassword, dbName, dbSSLMode, migrationsPath string) error {
	dsn := fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=%s",
		url.UserPassword(dbUser, dbPassword).String(), dbHost, dbPort, dbName, dbSSLMode)

	latestVersion, err := latestMigrationVersion(migrationsPath)
	if err != nil {
		return fmt.Errorf("failed to determine latest migration version: %w", err)
	}

	m, err := migrate.New(
		fmt.Sprintf("file://%s", migrationsPath),
		dsn,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize migrations: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	version, err := applyMigrations(m, latestVersion)
	if err != nil {
		return err
	}
	if latestVersion == 0 {
		logger.Logger.Info("Migrations completed (no numbered migrations beyond the genesis baseline)")
		return nil
	}
	logger.Logger.Infof("Migrations completed (version: %d)", version)
	return nil
}

// migrator is the subset of *migrate.Migrate that applyMigrations drives.
// Tests pass a fake so the post-Up version check does not need a database.
type migrator interface {
	Up() error
	Version() (uint, bool, error)
}

// applyMigrations runs everything RunMigrations does between opening the
// migrator and logging success: the pre-Up dirty check, the latestVersion==0
// short-circuit, Up (ErrNoChange is success), and the post-Up version check.
// The returned version is 0 when latestVersion is 0.
func applyMigrations(m migrator, latestVersion uint) (uint, error) {
	version, dirty, verErr := m.Version()
	action, decideErr := decideMigrationStartup(verErr, dirty)
	if decideErr != nil {
		return 0, decideErr
	}
	if action == migrationFailDirty {
		return 0, dirtyMigrationError(version)
	}

	if latestVersion == 0 {
		return 0, nil
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, fmt.Errorf("migration failed: %w", err)
	}

	version, dirty, verErr = m.Version()
	if verErr != nil && !errors.Is(verErr, migrate.ErrNilVersion) {
		return 0, fmt.Errorf("failed to read migration version: %w", verErr)
	}
	if dirty {
		return 0, dirtyMigrationError(version)
	}
	if errors.Is(verErr, migrate.ErrNilVersion) && latestVersion > 0 {
		return 0, fmt.Errorf("migration version after Up is nil and does not match latest migration file %d: %w", latestVersion, verErr)
	}
	if version != latestVersion {
		return 0, fmt.Errorf("migration version %d after Up does not match latest migration file %d", version, latestVersion)
	}
	return version, nil
}

// migrationStartupAction is what RunMigrations does, given the current
// golang-migrate state, before calling m.Up(). Extracted as a pure decision so
// it is unit-testable without a live database.
type migrationStartupAction int

const (
	// migrationApplyPending runs m.Up() to apply any pending migrations. It is
	// the path for a clean established DB and for a genesis-bootstrapped DB that
	// has no applied numbered migration yet (ErrNilVersion).
	migrationApplyPending migrationStartupAction = iota
	// migrationFailDirty refuses to continue because a prior migration half-applied.
	migrationFailDirty
)

// decideMigrationStartup chooses the startup action from golang-migrate's
// reported state. It never force-baselines: force-marking a version as applied
// without running its SQL silently skips migrations.
func decideMigrationStartup(verErr error, dirty bool) (migrationStartupAction, error) {
	switch {
	case verErr == nil:
		if dirty {
			return migrationFailDirty, nil
		}
		return migrationApplyPending, nil
	case errors.Is(verErr, migrate.ErrNilVersion):
		return migrationApplyPending, nil
	default:
		return migrationApplyPending, fmt.Errorf("failed to read migration version: %w", verErr)
	}
}

// latestMigrationVersion returns the highest NNNNNN_*.up.sql version in
// migrationsPath, or 0 when the directory holds no numbered migrations.
func latestMigrationVersion(migrationsPath string) (uint, error) {
	entries, err := os.ReadDir(migrationsPath)
	if err != nil {
		return 0, err
	}

	var latest uint
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		prefix := strings.SplitN(filepath.Base(name), "_", 2)[0]
		value, err := strconv.ParseUint(prefix, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid migration filename %q: %w", name, err)
		}
		if uint(value) > latest {
			latest = uint(value)
		}
	}

	return latest, nil
}

// LatestMigrationVersion returns the highest migration version present in
// migrationsPath (0 when there is none). Startup schema verification compares
// the live schema_migrations ledger against this — not the genesis baseline
// head — because the applied version advances past the baseline as new
// migrations land.
func LatestMigrationVersion(migrationsPath string) (int64, error) {
	v, err := latestMigrationVersion(migrationsPath)
	if err != nil {
		return 0, err
	}
	return int64(v), nil
}

// dirtyMigrationError returns a structured, actionable error for a dirty
// migration state. A dirty state means a migration's SQL started executing but
// did not complete, leaving schema_migrations with dirty=true. The database is
// now in an undefined, partially-migrated state and must not serve traffic
// until the issue is manually resolved.
//
// The error message deliberately includes the runbook path so operators have a
// concrete next step at the point of failure, with no need to search
// documentation while the server is down.
func dirtyMigrationError(version uint) error {
	return fmt.Errorf(
		"database is in dirty state at migration version %d — "+
			"the migration partially applied and did not complete; "+
			"manual recovery is required before the server can start. "+
			"See docs/runbooks/migration-dirty-recovery.md for step-by-step instructions.",
		version,
	)
}
