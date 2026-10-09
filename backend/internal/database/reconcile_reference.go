package database

import (
	"fmt"
	"net/url"
	"strings"

	"gorm.io/gorm"
)

// ReconcileReferenceSchema builds the next production baseline from the
// currently embedded genesis artifact plus every pending numbered migration.
// This mirrors production schema startup and, unlike the retired legacy
// AutoMigrate/ensure path, cannot force-skip a newly added migration.
//
// dsn must be a postgres URL (postgres://user:pass@host:port/db?sslmode=...).
// migrationsDir is the absolute path to backend/migrations.
func ReconcileReferenceSchema(gormDB *gorm.DB, dsn, migrationsDir string) error {
	if gormDB == nil {
		return fmt.Errorf("ReconcileReferenceSchema: gormDB is nil")
	}
	if err := BootstrapGenesisSchema(gormDB); err != nil {
		return fmt.Errorf("BootstrapGenesisSchema: %w", err)
	}

	host, port, user, pass, name, sslmode, err := parsePostgresDSN(dsn)
	if err != nil {
		return fmt.Errorf("parse DSN: %w", err)
	}
	if err := RunMigrations(host, port, user, pass, name, sslmode, migrationsDir); err != nil {
		return fmt.Errorf("RunMigrations: %w", err)
	}
	head, err := LatestMigrationVersion(migrationsDir)
	if err != nil {
		return fmt.Errorf("LatestMigrationVersion: %w", err)
	}
	if err := VerifySchemaAtVersion(gormDB, head); err != nil {
		return fmt.Errorf("VerifySchemaAtVersion: %w", err)
	}
	return nil
}

// parsePostgresDSN extracts connection fields from a postgres:// URL for RunMigrations.
func parsePostgresDSN(dsn string) (host, port, user, pass, name, sslmode string, err error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("url.Parse: %w", err)
	}
	host = u.Hostname()
	port = u.Port()
	if port == "" {
		port = "5432"
	}
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}
	name = strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return "", "", "", "", "", "", fmt.Errorf("database name missing in DSN path")
	}
	sslmode = u.Query().Get("sslmode")
	if sslmode == "" {
		sslmode = "disable"
	}
	return host, port, user, pass, name, sslmode, nil
}
