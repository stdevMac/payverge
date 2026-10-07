package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/auth"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/session"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const cliTestPassword = "Tr4ffic-Lantern-Basil-91"

// newCLITestDB is a SQLite stand-in for the migrated Postgres schema with the
// tables the admin subcommands touch (test-only AutoMigrate).
func newCLITestDB(t *testing.T, models ...interface{}) *gorm.DB {
	t.Helper()
	logger.InitLogger()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	base := []interface{}{&database.User{}, &auth.UserAuth{}, &session.UserSession{}, &database.AuthAttempt{}}
	require.NoError(t, db.AutoMigrate(append(base, models...)...))
	database.InitTestDB(db)
	return db
}

type cliHarness struct {
	env    *cliEnv
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	opened int
	gotCfg cliDBConfig
}

func newCLIHarness(t *testing.T, db *gorm.DB, stdin string, envVars map[string]string) *cliHarness {
	t.Helper()
	h := &cliHarness{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	h.env = &cliEnv{
		stdin:  strings.NewReader(stdin),
		stdout: h.stdout,
		stderr: h.stderr,
		getenv: func(k string) string { return envVars[k] },
		openDB: func(cfg cliDBConfig) (*gorm.DB, error) {
			h.opened++
			h.gotCfg = cfg
			if db == nil {
				return nil, errors.New("no database in this test")
			}
			return db, nil
		},
		newDemo: newCLIDemoService,
	}
	return h
}

func (h *cliHarness) run(args ...string) int {
	return runCLIWith(context.Background(), h.env, args)
}

func TestIsCLIInvocation(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"admin"}, {"admin", "create"}, {"demo", "seed"}, {"demo", "reset", "--all"}, {"serve"}, {"amdin", "create"}} {
		require.True(t, isCLIInvocation(args), args)
	}
	for _, args := range [][]string{nil, {}, {"--production"}, {"--db-host", "postgres"}, {"--rpc-url=http://127.0.0.1:8545"}} {
		require.False(t, isCLIInvocation(args), args)
	}
	handled, code := runCLI([]string{"--db-host", "postgres"}, nil, nil, nil)
	require.False(t, handled, "server flags must fall through to the HTTP server")
	require.Zero(t, code)
}

// The distroless image has no shell, so the subcommands only work if main()
// dispatches them BEFORE flag.Parse rejects "admin" as an unknown argument.
func TestMainDispatchesCLIBeforeFlagParse(t *testing.T) {
	src, err := os.ReadFile("main.go")
	require.NoError(t, err)
	body := string(src)
	dispatch := strings.Index(body, "runCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)")
	parse := strings.Index(body, "flag.Parse()")
	require.Positive(t, dispatch, "main.go must call runCLI")
	require.Positive(t, parse)
	require.Less(t, dispatch, parse, "runCLI must run before flag.Parse")
	require.Contains(t, body, "bootstrapPlatformAdmins(context.Background(), database.GetDB())")
	require.Contains(t, body, "startDemoStartupEnsure(adminDemoService, demoPlan, bootstrapAdminID)")
	require.NotContains(t, body, `!strings.EqualFold(strings.TrimSpace(os.Getenv("ADMIN_DEMO_AUTOMATION_ENABLED")), "false")`,
		"demo automation must not default on (it needs S3 and seeds every admin)")
}

func TestCLIUsageAndUnknownCommands(t *testing.T) {
	h := newCLIHarness(t, nil, "", nil)
	require.Equal(t, cliExitOK, h.run("help"))
	require.Contains(t, h.stderr.String(), "admin reset-password")
	require.Contains(t, h.stderr.String(), "demo seed")

	h = newCLIHarness(t, nil, "", nil)
	require.Equal(t, cliExitUsage, h.run("admin"))
	require.Contains(t, h.stderr.String(), "admin reset-password")

	h = newCLIHarness(t, nil, "", nil)
	require.Equal(t, cliExitOK, h.run("demo", "--help"))
	require.Contains(t, h.stderr.String(), "demo seed")
	require.NotContains(t, h.stderr.String(), "admin create")

	h = newCLIHarness(t, nil, "", nil)
	require.Equal(t, cliExitUsage, h.run("admin", "delete"))
	require.Contains(t, h.stderr.String(), `unknown command "admin delete"`)

	// An unknown bare word must not boot the server: it is a usage error.
	h = newCLIHarness(t, nil, "", nil)
	require.Equal(t, cliExitUsage, h.run("serve"))
	require.Contains(t, h.stderr.String(), `unknown command "serve"`)
	require.Contains(t, h.stderr.String(), "admin reset-password", "usage lists every command")

	h = newCLIHarness(t, nil, "", nil)
	require.Equal(t, cliExitUsage, h.run("amdin", "create", "--email", "a@example.com"))
	require.Contains(t, h.stderr.String(), `unknown command "amdin"`)

	h = newCLIHarness(t, nil, "", nil)
	require.Equal(t, cliExitUsage, h.run(cliTestPassword))
	require.Contains(t, h.stderr.String(), "unknown command (value not shown")
	require.NotContains(t, h.stderr.String(), cliTestPassword)

	handled, code := runCLI([]string{"serve"}, nil, io.Discard, io.Discard)
	require.True(t, handled, "a bare word is never a server start")
	require.Equal(t, cliExitUsage, code)

	h = newCLIHarness(t, nil, "", nil)
	require.Equal(t, cliExitUsage, h.run("admin", cliTestPassword))
	require.Contains(t, h.stderr.String(), "unknown admin command (value not shown)")
	require.NotContains(t, h.stderr.String(), cliTestPassword)
}

func TestReadPasswordFromStdin(t *testing.T) {
	cases := map[string]string{
		"pass phrase with spaces\n": "pass phrase with spaces",
		"crlf-terminated-pass\r\n":  "crlf-terminated-pass",
		"no-newline-at-eof":         "no-newline-at-eof",
		" edge spaces kept \n":      " edge spaces kept ",
		"first-line\nsecond-line\n": "first-line",
	}
	for in, want := range cases {
		got, err := readPasswordFromStdin(strings.NewReader(in))
		require.NoError(t, err, in)
		require.Equal(t, want, got)
	}
	_, err := readPasswordFromStdin(strings.NewReader("\n"))
	require.ErrorIs(t, err, errCLIUsage)
	_, err = readPasswordFromStdin(strings.NewReader(strings.Repeat("x", cliMaxPasswordBytes+10)))
	require.ErrorIs(t, err, errCLIUsage)
}

func TestParseServerDBFlags(t *testing.T) {
	got := parseServerDBFlags([]string{"/app/server", "--production", "--db-host", "postgres", "--db-port=6543", "-db-user", "pv", "--db-name", "pvdb", "--db-sslmode", "disable", "--db-password", "secret", "--s3-bucket", "b"})
	require.Equal(t, map[string]string{"db-host": "postgres", "db-port": "6543", "db-user": "pv", "db-name": "pvdb", "db-sslmode": "disable"}, got)
	require.Empty(t, parseServerDBFlags([]string{"/usr/bin/postgres", "--db-host", "x"}), "only a payverge server argv is trusted")
	require.Empty(t, parseServerDBFlags(nil))
}

func TestResolveCLIDBConfigPrecedence(t *testing.T) {
	env := &cliEnv{
		getenv: func(k string) string {
			return map[string]string{"DB_HOST": "env-host", "DB_PASSWORD": "env-secret", "DB_PORT": ""}[k]
		},
		serverArgs: func() []string {
			return []string{"/app/server", "--db-host", "pid1-host", "--db-port", "7000", "--db-sslmode", "disable", "--db-password", "argv-secret"}
		},
	}
	got := resolveCLIDBConfig(env, cliDBConfig{User: "flag-user"})
	require.Equal(t, cliDBConfig{
		Host: "env-host", Port: "7000", User: "flag-user", Name: "payverge", SSLMode: "disable", Password: "env-secret",
	}, got, "flag > env > running server argv > defaults; password only from DB_PASSWORD")

	env.serverArgs = nil
	env.getenv = func(string) string { return "" }
	require.Equal(t, cliDBConfig{Host: "localhost", Port: "5432", User: "payverge", Name: "payverge", SSLMode: "require"}, resolveCLIDBConfig(env, cliDBConfig{}))
}

func TestAdminCreateRejectsBadInputBeforeTouchingTheDatabase(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		code  int
		msg   string
	}{
		{"missing email", cliTestPassword, []string{"admin", "create", "--password-stdin"}, cliExitUsage, "--email is required"},
		{"password as argument", cliTestPassword, []string{"admin", "create", "--email", "a@example.com"}, cliExitUsage, "--password-stdin is required"},
		{"stray positional", cliTestPassword, []string{"admin", "create", "--email", "a@example.com", "--password-stdin", cliTestPassword}, cliExitUsage, "unexpected positional argument"},
		{"password as bool flag value", cliTestPassword, []string{"admin", "create", "--email", "a@example.com", "--password-stdin=" + cliTestPassword}, cliExitUsage, "invalid value for --password-stdin"},
		{"password pasted as a flag", cliTestPassword, []string{"admin", "create", "--email", "a@example.com", "-" + cliTestPassword}, cliExitUsage, "unknown flag"},
		{"flag missing its value", cliTestPassword, []string{"admin", "create", "--password-stdin", "--email"}, cliExitUsage, "--email needs a value"},
		{"empty stdin", "", []string{"admin", "create", "--email", "a@example.com", "--password-stdin"}, cliExitUsage, "no password on stdin"},
		{"weak password", "password1234\n", []string{"admin", "create", "--email", "a@example.com", "--password-stdin"}, cliExitError, "known default"},
		{"short password", "short\n", []string{"admin", "reset-password", "--email", "a@example.com", "--password-stdin"}, cliExitError, "at least 12"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newCLIHarness(t, nil, tc.stdin, nil)
			require.Equal(t, tc.code, h.run(tc.args...), h.stderr.String())
			require.Contains(t, h.stderr.String(), tc.msg)
			require.Zero(t, h.opened, "input errors must not open the database")
			require.NotContains(t, h.stderr.String()+h.stdout.String(), cliTestPassword, "the password is never echoed")
		})
	}
}

func TestAdminCreateCreatesThenIsIdempotent(t *testing.T) {
	db := newCLITestDB(t)
	h := newCLIHarness(t, db, cliTestPassword+"\n", nil)
	require.Equal(t, cliExitOK, h.run("admin", "create", "--email", "Owner@Example.com", "--password-stdin"), h.stderr.String())
	require.Contains(t, h.stdout.String(), "Created platform admin")

	var user database.User
	require.NoError(t, db.Where("email = ?", "owner@example.com").First(&user).Error)
	require.Equal(t, "admin", user.Role)
	require.True(t, user.EmailVerified)
	var cred auth.UserAuth
	require.NoError(t, db.Where("user_id = ? AND provider = ?", user.ID, "email").First(&cred).Error)
	require.True(t, auth.CheckPassword(cliTestPassword, cred.PasswordHash))

	h = newCLIHarness(t, db, "Another-Strong-pass-77\n", nil)
	require.Equal(t, cliExitOK, h.run("admin", "create", "--email", "owner@example.com", "--password-stdin"), h.stderr.String())
	require.Contains(t, h.stdout.String(), "already a platform admin")
	require.Contains(t, h.stdout.String(), "reset-password")
	var users int64
	require.NoError(t, db.Model(&database.User{}).Where("email = ?", "owner@example.com").Count(&users).Error)
	require.Equal(t, int64(1), users)
	require.NoError(t, db.Where("user_id = ? AND provider = ?", user.ID, "email").First(&cred).Error)
	require.True(t, auth.CheckPassword(cliTestPassword, cred.PasswordHash), "admin create never overwrites a password")
}

func TestAdminCreateTakesOverExistingNonAdminAccount(t *testing.T) {
	db := newCLITestDB(t)
	// Someone registered the operator's email first and linked a Google login.
	hash, err := auth.HashPassword("squatter-chose-this-1")
	require.NoError(t, err)
	squatter := database.User{Email: "manager@example.com", Role: "user", AuthMethod: "email"}
	require.NoError(t, db.Create(&squatter).Error)
	require.NoError(t, db.Create(&auth.UserAuth{UserID: squatter.ID, Provider: "email", ProviderUserID: squatter.Email, PasswordHash: hash}).Error)
	require.NoError(t, db.Create(&auth.UserAuth{UserID: squatter.ID, Provider: "google", ProviderUserID: "sub-x"}).Error)
	uid := squatter.ID
	require.NoError(t, db.Create(&session.UserSession{UserID: &uid, SessionToken: "squatter-tok", Provider: "email"}).Error)

	h := newCLIHarness(t, db, cliTestPassword, nil)
	require.Equal(t, cliExitOK, h.run("admin", "create", "--email", "manager@example.com", "--password-stdin"), h.stderr.String())
	require.Contains(t, h.stdout.String(), "Took over existing account")
	require.Contains(t, h.stdout.String(), "1 other sign-in method(s) were unlinked")
	require.NotContains(t, h.stdout.String()+h.stderr.String(), cliTestPassword)

	var user database.User
	require.NoError(t, db.Where("email = ?", "manager@example.com").First(&user).Error)
	require.Equal(t, "admin", user.Role)
	require.True(t, user.EmailVerified)
	var creds []auth.UserAuth
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&creds).Error)
	require.Len(t, creds, 1)
	require.Equal(t, "email", creds[0].Provider)
	require.True(t, auth.CheckPassword(cliTestPassword, creds[0].PasswordHash), "the operator's password replaces the registrant's")
	var sess session.UserSession
	require.NoError(t, db.Where("session_token = ?", "squatter-tok").First(&sess).Error)
	require.True(t, sess.Revoked)
}

func TestAdminCreateRefusesWalletLinkedAccount(t *testing.T) {
	db := newCLITestDB(t)
	require.NoError(t, db.Create(&database.User{Email: "wallet@example.com", Role: "user", AuthMethod: "wallet", Address: "0xabc0000000000000000000000000000000000003"}).Error)
	h := newCLIHarness(t, db, cliTestPassword, nil)
	require.Equal(t, cliExitError, h.run("admin", "create", "--email", "wallet@example.com", "--password-stdin"))
	require.Contains(t, h.stderr.String(), "linked wallet")
	require.Contains(t, h.stderr.String(), "nothing was changed")
	var user database.User
	require.NoError(t, db.Where("email = ?", "wallet@example.com").First(&user).Error)
	require.Equal(t, "user", user.Role)
	var creds int64
	require.NoError(t, db.Model(&auth.UserAuth{}).Where("user_id = ?", user.ID).Count(&creds).Error)
	require.Zero(t, creds)
}

func TestAdminResetPasswordResetsRevokesAndUnlocks(t *testing.T) {
	db := newCLITestDB(t)
	h := newCLIHarness(t, db, cliTestPassword, nil)
	require.Equal(t, cliExitOK, h.run("admin", "create", "--email", "owner@example.com", "--password-stdin"))
	var user database.User
	require.NoError(t, db.Where("email = ?", "owner@example.com").First(&user).Error)

	uid := user.ID
	require.NoError(t, db.Create(&session.UserSession{UserID: &uid, SessionToken: "tok-1", Provider: "email"}).Error)
	require.NoError(t, db.Create(&database.AuthAttempt{Principal: "owner@example.com", Kind: passwordLoginThrottleKind, Count: 9}).Error)

	const newPassword = "Fresh-Recovery-Phrase-2026"
	h = newCLIHarness(t, db, newPassword+"\n", map[string]string{"DB_HOST": "postgres"})
	require.Equal(t, cliExitOK, h.run("admin", "reset-password", "--email", "OWNER@example.com", "--password-stdin"), h.stderr.String())
	require.Equal(t, "postgres", h.gotCfg.Host)
	require.Contains(t, h.stdout.String(), "Password updated")
	require.Contains(t, h.stdout.String(), "signed out")
	require.NotContains(t, h.stdout.String()+h.stderr.String(), newPassword)

	var cred auth.UserAuth
	require.NoError(t, db.Where("user_id = ? AND provider = ?", uid, "email").First(&cred).Error)
	require.True(t, auth.CheckPassword(newPassword, cred.PasswordHash))
	require.False(t, auth.CheckPassword(cliTestPassword, cred.PasswordHash))

	var sess session.UserSession
	require.NoError(t, db.Where("session_token = ?", "tok-1").First(&sess).Error)
	require.True(t, sess.Revoked)
	require.Equal(t, string(session.RevocationReasonSecurityReset), sess.RevocationReason)
	var attempts int64
	require.NoError(t, db.Model(&database.AuthAttempt{}).Count(&attempts).Error)
	require.Zero(t, attempts, "the login lockout is cleared")
}

func TestAdminResetPasswordUnknownEmailFails(t *testing.T) {
	db := newCLITestDB(t)
	h := newCLIHarness(t, db, cliTestPassword, nil)
	require.Equal(t, cliExitError, h.run("admin", "reset-password", "--email", "ghost@example.com", "--password-stdin"))
	require.Contains(t, h.stderr.String(), "no active account with email ghost@example.com")
}

func TestAdminCreateReleasesDeletedAccountsLogin(t *testing.T) {
	db := newCLITestDB(t)
	// Production's active-email index is partial (deleted_at IS NULL).
	require.NoError(t, db.Exec("DROP INDEX IF EXISTS idx_users_email").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_users_email_lower_active ON users (lower(trim(email))) WHERE email IS NOT NULL AND trim(email) <> '' AND deleted_at IS NULL").Error)
	now := time.Now()
	gone := database.User{Email: "owner@example.com", Role: "user", AuthMethod: "email", EmailVerified: true, DeletedAt: &now}
	require.NoError(t, db.Create(&gone).Error)
	oldHash, err := auth.HashPassword("deleted-account-pass-1")
	require.NoError(t, err)
	require.NoError(t, db.Create(&auth.UserAuth{UserID: gone.ID, Provider: "email", ProviderUserID: gone.Email, PasswordHash: oldHash, EmailVerified: true}).Error)

	h := newCLIHarness(t, db, cliTestPassword, nil)
	require.Equal(t, cliExitOK, h.run("admin", "create", "--email", "owner@example.com", "--password-stdin"), h.stderr.String())
	require.Contains(t, h.stdout.String(), "Removed 1 email/password login(s) for owner@example.com left by a deleted account")
	require.Contains(t, h.stdout.String(), "Created platform admin")

	authRec, err := auth.NewAuthService(db).LoginWithEmail("owner@example.com", cliTestPassword)
	require.NoError(t, err, "the new admin signs in with the password it was created with")
	require.NotEqual(t, gone.ID, authRec.UserID)
}

func TestAdminResetPasswordRefusesLoginHeldByAnotherLiveAccount(t *testing.T) {
	db := newCLITestDB(t)
	other := database.User{Email: "renamed@example.com", Role: "user", AuthMethod: "email", EmailVerified: true}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&auth.UserAuth{UserID: other.ID, Provider: "email", ProviderUserID: "owner@example.com", PasswordHash: "theirs"}).Error)
	owner := database.User{Email: "owner@example.com", Role: "admin", AuthMethod: "google", EmailVerified: true}
	require.NoError(t, db.Create(&owner).Error)

	h := newCLIHarness(t, db, cliTestPassword, nil)
	require.Equal(t, cliExitError, h.run("admin", "reset-password", "--email", "owner@example.com", "--password-stdin"))
	require.Contains(t, h.stderr.String(), "another active account holds an email/password login for owner@example.com")
	require.NotContains(t, h.stdout.String(), "Password updated")
	var creds int64
	require.NoError(t, db.Model(&auth.UserAuth{}).Where("user_id = ?", owner.ID).Count(&creds).Error)
	require.Zero(t, creds, "nothing is written")

	// admin create refuses the same way for an address it would create.
	require.NoError(t, db.Create(&auth.UserAuth{UserID: other.ID, Provider: "email", ProviderUserID: "fresh@example.com", PasswordHash: "theirs"}).Error)
	h = newCLIHarness(t, db, cliTestPassword, nil)
	require.Equal(t, cliExitError, h.run("admin", "create", "--email", "fresh@example.com", "--password-stdin"))
	require.Contains(t, h.stderr.String(), "another active account holds an email/password login for fresh@example.com")
}
