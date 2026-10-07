package testperf

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Pg is a started Postgres container plus a connected GORM handle.
type Pg struct {
	container     testcontainers.Container
	cleanup       func(context.Context) error
	terminateOnce sync.Once
	terminateErr  error
	DB            *gorm.DB
	DSN           string
	DatabaseName  string
	Source        string
}

type contextPinger interface {
	PingContext(context.Context) error
}

// StartPostgres spins a postgres:15-alpine container and returns a *gorm.DB
// ready for benchmarks. Caller is responsible for applying migrations and
// loading fixtures.
//
// We use tcpostgres.Run (the post-v0.32 API); RunContainer is deprecated.
// The image is the first positional arg rather than testcontainers.WithImage.
func postgresHarnessURL() string {
	if v := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("TESTPERF_DATABASE_URL"))
}

func StartPostgres(ctx context.Context) (*Pg, error) {
	if postgresHarnessURL() != "" {
		return StartIsolatedPostgres(ctx)
	}
	c, err := tcpostgres.Run(ctx,
		"postgres:15-alpine",
		tcpostgres.WithDatabase("bench"),
		tcpostgres.WithUsername("bench"),
		tcpostgres.WithPassword("bench"),
		testcontainers.WithWaitStrategy(
			// Postgres logs "ready to accept connections" twice — once during
			// init bootstrap, once after the restart for the real server. Wait
			// for the second occurrence so the DB is actually accepting conns.
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("run container: %w", err)
	}
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, fmt.Errorf("conn string: %w", err)
	}
	// Mirror the production GORM config so benches measure the same path.
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, fmt.Errorf("gorm open: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, fmt.Errorf("sql db: %w", err)
	}
	if err := waitForPostgresPing(ctx, sqlDB, 10*time.Second, 100*time.Millisecond); err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	return &Pg{
		container:    c,
		DB:           db,
		DSN:          dsn,
		DatabaseName: "bench",
		Source:       "testcontainers",
	}, nil
}

var safeDatabaseName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

func isolatedDatabaseDSN(parentDSN, databaseName string) (string, error) {
	if !safeDatabaseName.MatchString(databaseName) {
		return "", fmt.Errorf("unsafe isolated database name %q", databaseName)
	}
	parsed, err := url.Parse(strings.TrimSpace(parentDSN))
	if err != nil {
		return "", fmt.Errorf("parse TEST_DATABASE_URL: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", fmt.Errorf("TEST_DATABASE_URL must use postgres:// or postgresql://")
	}
	if parsed.Host == "" || parsed.User == nil {
		return "", fmt.Errorf("TEST_DATABASE_URL must include host and user")
	}
	parsed.Path = "/" + databaseName
	return parsed.String(), nil
}

func randomDatabaseName() (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate isolated database name: %w", err)
	}
	return "pv_test_" + hex.EncodeToString(suffix[:]), nil
}

// StartIsolatedPostgres returns a database that the caller may migrate, lock,
// truncate, or drop without affecting another test. When TEST_DATABASE_URL is
// set, it creates a child database on that server. Otherwise it starts a
// dedicated PostgreSQL 15 Testcontainers instance.
func StartIsolatedPostgres(ctx context.Context) (*Pg, error) {
	parentDSN := postgresHarnessURL()
	if parentDSN == "" {
		return StartPostgres(ctx)
	}

	databaseName, err := randomDatabaseName()
	if err != nil {
		return nil, err
	}
	adminDSN, err := isolatedDatabaseDSN(parentDSN, "postgres")
	if err != nil {
		return nil, err
	}
	childDSN, err := isolatedDatabaseDSN(parentDSN, databaseName)
	if err != nil {
		return nil, err
	}

	admin, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open postgres admin database: %w", err)
	}
	adminSQL, err := admin.DB()
	if err != nil {
		return nil, fmt.Errorf("get postgres admin handle: %w", err)
	}
	if err := waitForPostgresPing(ctx, adminSQL, 10*time.Second, 100*time.Millisecond); err != nil {
		_ = adminSQL.Close()
		return nil, err
	}

	quotedName := pq.QuoteIdentifier(databaseName)
	if err := admin.Exec("CREATE DATABASE " + quotedName).Error; err != nil {
		_ = adminSQL.Close()
		return nil, fmt.Errorf("create isolated database: %w", err)
	}
	dropChild := func() {
		_ = admin.Exec("DROP DATABASE IF EXISTS " + quotedName + " WITH (FORCE)").Error
		_ = adminSQL.Close()
	}

	child, err := gorm.Open(postgres.Open(childDSN), &gorm.Config{})
	if err != nil {
		dropChild()
		return nil, fmt.Errorf("open isolated database: %w", err)
	}
	childSQL, err := child.DB()
	if err != nil {
		dropChild()
		return nil, fmt.Errorf("get isolated database handle: %w", err)
	}
	if err := waitForPostgresPing(ctx, childSQL, 10*time.Second, 100*time.Millisecond); err != nil {
		_ = childSQL.Close()
		dropChild()
		return nil, err
	}

	return &Pg{
		DB:           child,
		DSN:          childDSN,
		DatabaseName: databaseName,
		Source:       "external-child",
		cleanup: func(context.Context) error {
			if err := childSQL.Close(); err != nil {
				return fmt.Errorf("close isolated database: %w", err)
			}
			if err := admin.Exec("DROP DATABASE IF EXISTS " + quotedName + " WITH (FORCE)").Error; err != nil {
				_ = adminSQL.Close()
				return fmt.Errorf("drop isolated database: %w", err)
			}
			return adminSQL.Close()
		},
	}, nil
}

func waitForPostgresPing(ctx context.Context, db contextPinger, timeout, interval time.Duration) error {
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
			return fmt.Errorf("postgres did not become ready after %s: %w", timeout, lastErr)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("postgres readiness wait canceled: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
}

// Terminate stops the container. Safe to call multiple times.
func (p *Pg) Terminate(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.terminateOnce.Do(func() {
		if p.cleanup != nil {
			p.terminateErr = p.cleanup(ctx)
			return
		}
		if p.DB != nil {
			if sqlDB, err := p.DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		if p.container != nil {
			p.terminateErr = p.container.Terminate(ctx)
		}
	})
	return p.terminateErr
}
