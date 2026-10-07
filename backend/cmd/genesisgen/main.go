// genesisgen builds the reconciled fresh-DB reference schema against a live
// Postgres DSN, then verifies schema_migrations is clean.
//
// Usage:
//
//	go run ./cmd/genesisgen -dsn='postgres://...' -migrations=/abs/path/to/migrations
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func main() {
	dsn := flag.String("dsn", "", "Postgres DSN (required), e.g. postgres://user:pass@127.0.0.1:5432/db?sslmode=disable")
	migrations := flag.String("migrations", "", "Absolute path to backend/migrations (required)")
	fingerprintOut := flag.String("fingerprint-out", "", "Optional file to write the reconciled schema's fingerprint SHA-256 to")
	flag.Parse()

	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "error: -dsn is required")
		os.Exit(1)
	}
	if *migrations == "" {
		fmt.Fprintln(os.Stderr, "error: -migrations is required")
		os.Exit(1)
	}

	version, fingerprint, err := generateGenesis(context.Background(), *dsn, *migrations)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if *fingerprintOut != "" {
		if err := os.WriteFile(*fingerprintOut, []byte(fingerprint+"\n"), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write fingerprint: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Fprintf(os.Stderr, "reconciled: version=%d fingerprint=%s\n", version, fingerprint)
	os.Exit(0)
}

func generateGenesis(ctx context.Context, dsn, migrations string) (int64, string, error) {
	var version int64
	var fingerprint string
	err := retryTransientDatabaseStartup(ctx, 30*time.Second, 250*time.Millisecond, func() error {
		gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			return fmt.Errorf("open gorm: %w", err)
		}
		sqlDB, err := gormDB.DB()
		if err != nil {
			return fmt.Errorf("sql db: %w", err)
		}
		defer func() { _ = sqlDB.Close() }()

		if err := waitForDatabase(ctx, sqlDB, 10*time.Second, 250*time.Millisecond); err != nil {
			return fmt.Errorf("wait for database: %w", err)
		}
		if err := database.ReconcileReferenceSchema(gormDB, dsn, migrations); err != nil {
			return fmt.Errorf("reconcile: %w", err)
		}

		var dirty bool
		if err := gormDB.Raw(`SELECT version, dirty FROM schema_migrations LIMIT 1`).Row().Scan(&version, &dirty); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("read schema_migrations: %w", err)
			}
			// No numbered migrations: the genesis baseline is the whole schema.
			version, dirty = 0, false
		}
		if dirty {
			return fmt.Errorf("schema_migrations is dirty (version=%d); refusing to emit baseline", version)
		}
		fingerprint, err = database.SchemaFingerprintSHA(gormDB)
		if err != nil {
			return fmt.Errorf("schema fingerprint: %w", err)
		}
		return nil
	})
	return version, fingerprint, err
}

type contextPinger interface {
	PingContext(context.Context) error
}

func waitForDatabase(ctx context.Context, db contextPinger, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		pingCtx, cancel := context.WithTimeout(ctx, interval)
		err := db.PingContext(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return fmt.Errorf("database did not become ready after %s: %w", timeout, lastErr)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("database readiness wait canceled: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
}

func retryTransientDatabaseStartup(ctx context.Context, timeout, interval time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		err := fn()
		if err == nil {
			return nil
		}
		if !isTransientDatabaseStartupError(err) {
			return err
		}
		lastErr = err
		if time.Now().After(deadline) {
			return fmt.Errorf("database startup did not stabilize after %s: %w", timeout, lastErr)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("database startup retry canceled: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
}

func isTransientDatabaseStartupError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	transient := []string{
		"unexpected eof",
		"database system is starting up",
		"connection refused",
		"connection reset by peer",
		"server closed the connection unexpectedly",
		"bad connection",
		"eof",
	}
	for _, needle := range transient {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
