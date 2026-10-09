package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// setupTestDB creates an in-memory SQLite database for service tests and
// installs it as the database package's global handle.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("ALLOWED_REDIRECT_DOMAINS", "payverge.io,www.payverge.io")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("Failed to get test sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	err = db.AutoMigrate(
		&database.Business{},
		&database.PlatformSettings{},
		&database.WebhookEvent{},
		&database.Plugin{},
		&database.BusinessPlugin{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	database.SetTestDB(db)
	return db
}

// captureEmailProvider records every EmailMessage the server is asked to send
// so a test can assert on which template/body was used. When fail is true it
// still records the attempt then returns a hard, non-retryable error.
type captureEmailProvider struct {
	sent []emails.EmailMessage
	fail bool
}

func (p *captureEmailProvider) Send(_ context.Context, msg emails.EmailMessage) error {
	p.sent = append(p.sent, msg)
	if p.fail {
		return errors.New("not authorized to send emails from notifications@payverge.io")
	}
	return nil
}

// servicesTemplatesRoot resolves backend/email/templates relative to this file.
func servicesTemplatesRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to resolve current file path")
	return filepath.Join(filepath.Dir(currentFile), "..", "..", "email", "templates")
}

// installCaptureEmailServer swaps EmailServerInstance for a real
// template-backed server whose transport records messages, restoring the
// original on cleanup.
func installCaptureEmailServer(t *testing.T) *captureEmailProvider {
	t.Helper()
	provider := &captureEmailProvider{}
	orig := emails.EmailServerInstance
	_, err := emails.NewEmailServer(provider, "notifications@payverge.io", "updates@payverge.io", servicesTemplatesRoot(t))
	require.NoError(t, err)
	t.Cleanup(func() { emails.EmailServerInstance = orig })
	return provider
}

// checkerSQLRecorder captures emitted SQL so scheduler column projections can
// be asserted at the query layer.
type checkerSQLRecorder struct {
	gormlogger.Interface
	statements []string
}

func (r *checkerSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

// businessSelectStarCount counts SELECT * statements against businesses.
func (r *checkerSQLRecorder) businessSelectStarCount() int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		if strings.HasPrefix(normalized, "select *") && strings.Contains(normalized, "businesses") {
			count++
		}
	}
	return count
}
