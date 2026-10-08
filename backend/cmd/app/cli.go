package main

// Maintenance subcommands for the server binary.
//
// The production image is distroless (no shell), so first-admin recovery and
// demo data have to live inside /app/server itself:
//
//	docker compose exec -T backend /app/server admin reset-password --email you@example.com --password-stdin
//
// main() dispatches here BEFORE flag.Parse, so `server --db-host …` keeps
// starting the HTTP server exactly as before, while any other leading bare
// word is a command (an unknown one exits 2 with usage). A subcommand opens the
// database, refuses to touch a schema that is not at this binary's migration
// head, does its one job and exits. It never starts HTTP, schedulers or
// workers, and it never runs migrations (start the server once for that).

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"gorm.io/gorm"
)

const (
	cliExitOK    = 0
	cliExitError = 1
	cliExitUsage = 2

	// cliMaxPasswordBytes bounds what --password-stdin reads, so a mistaken
	// `< /dev/urandom` cannot exhaust memory.
	cliMaxPasswordBytes = 4096
)

// errCLIUsage marks an argument error: the command prints usage and exits 2.
var errCLIUsage = errors.New("usage")

// cliDBConfig is the connection a subcommand opens. The password only ever
// comes from DB_PASSWORD (never argv, which is world-readable in /proc).
type cliDBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// cliDBOpener opens and verifies the database. Tests inject a SQLite opener.
type cliDBOpener func(cfg cliDBConfig) (*gorm.DB, error)

// cliEnv is everything a subcommand may touch.
type cliEnv struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	getenv func(string) string
	openDB cliDBOpener
	// serverArgs returns the running server's argv (PID 1 in the container)
	// so `docker compose exec` inherits its --db-* flags. Nil disables it.
	serverArgs func() []string
	// newDemo builds the demo service; tests pin the clock and window.
	newDemo func(db *gorm.DB) cliDemoService
}

type cliCommand struct {
	group   string
	name    string
	usage   string
	summary string
	run     func(ctx context.Context, env *cliEnv, args []string) error
}

func cliCommands() []cliCommand {
	return []cliCommand{
		{group: "admin", name: "create", usage: "admin create --email EMAIL --password-stdin", summary: "Create a platform admin; an existing admin is left alone, an existing non-admin account is taken over (password replaced, other logins unlinked, sessions signed out)", run: runAdminCreate},
		{group: "admin", name: "reset-password", usage: "admin reset-password --email EMAIL --password-stdin", summary: "Set a user's email/password login, verify it, sign out every session and clear the login lockout", run: runAdminResetPassword},
		{group: "invite", name: "create", usage: "invite create [--uses N] [--days D] [--name TEXT]", summary: "Mint a signup invite code for REGISTRATION_MODE=invite (printed once; only its hash is stored)", run: runInviteCreate},
		{group: "settings", name: "image-limits", usage: "settings image-limits [--daily N] [--monthly-alert N]", summary: "Show or set the AI image fair-use limits (per-business daily cap and monthly alert threshold)", run: runSettingsImageLimits},
		{group: "demo", name: "seed", usage: "demo seed [--owner-email EMAIL]", summary: "Seed the demo restaurants for a platform admin (idempotent; defaults to ADMIN_EMAIL)", run: runDemoSeed},
		{group: "demo", name: "reset", usage: "demo reset (--owner-email EMAIL | --all)", summary: "Wipe and re-seed demo restaurants (only rows the demo seeder owns)", run: runDemoReset},
	}
}

// isCLIInvocation reports whether argv belongs to the maintenance CLI: any
// leading bare word does. Only no arguments, or flags first, start the server.
//
// flag.Parse stops at the first bare word, so before this a typo such as
// `server amdin create` (or `server serve`) silently booted a second full
// server (migrations, workers) inside `docker compose exec`. A bare word that
// names no command group now fails with usage instead.
func isCLIInvocation(args []string) bool {
	return len(args) > 0 && !strings.HasPrefix(args[0], "-")
}

// isCLIGroup reports whether word names a command group.
func isCLIGroup(word string) bool {
	for _, cmd := range cliCommands() {
		if word == cmd.group {
			return true
		}
	}
	return false
}

// runCLI is main()'s entry point. handled=false means "start the server".
func runCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) (handled bool, exitCode int) {
	if !isCLIInvocation(args) {
		return false, cliExitOK
	}
	logger.InitLogger()
	// Keep stdout for the command's own result lines.
	logger.Logger.SetOutput(stderr)
	env := &cliEnv{
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		getenv:     os.Getenv,
		openDB:     openCLIDatabase,
		serverArgs: readPID1Args,
		newDemo:    newCLIDemoService,
	}
	return true, runCLIWith(context.Background(), env, args)
}

func runCLIWith(ctx context.Context, env *cliEnv, args []string) int {
	if args[0] == "help" {
		printCLIUsage(env.stderr, "")
		return cliExitOK
	}
	if !isCLIGroup(args[0]) {
		if commandNameShape.MatchString(args[0]) {
			fmt.Fprintf(env.stderr, "error: unknown command %q (server flags start with -)\n", args[0])
		} else {
			// Could be a password typed in the wrong place: never echo it.
			fmt.Fprintln(env.stderr, "error: unknown command (value not shown; server flags start with -)")
		}
		printCLIUsage(env.stderr, "")
		return cliExitUsage
	}
	if len(args) < 2 || isHelpArg(args[1]) {
		printCLIUsage(env.stderr, args[0])
		if len(args) >= 2 {
			return cliExitOK
		}
		return cliExitUsage
	}
	for _, cmd := range cliCommands() {
		if cmd.group != args[0] || cmd.name != args[1] {
			continue
		}
		err := cmd.run(ctx, env, args[2:])
		switch {
		case err == nil:
			return cliExitOK
		case errors.Is(err, flag.ErrHelp):
			return cliExitOK
		case errors.Is(err, errCLIUsage):
			fmt.Fprintf(env.stderr, "error: %v\nusage: server %s\n", err, cmd.usage)
			return cliExitUsage
		default:
			fmt.Fprintf(env.stderr, "error: %v\n", err)
			return cliExitError
		}
	}
	if commandNameShape.MatchString(args[1]) {
		fmt.Fprintf(env.stderr, "error: unknown command %q\n", args[0]+" "+args[1])
	} else {
		// Could be a password typed in the wrong place: never echo it.
		fmt.Fprintf(env.stderr, "error: unknown %s command (value not shown)\n", args[0])
	}
	printCLIUsage(env.stderr, args[0])
	return cliExitUsage
}

// commandNameShape is what a subcommand name looks like; anything else is
// not echoed back in errors.
var commandNameShape = regexp.MustCompile(`^[a-z][a-z-]{0,31}$`)

func isHelpArg(arg string) bool {
	switch arg {
	case "-h", "-help", "--help", "help":
		return true
	}
	return false
}

func printCLIUsage(w io.Writer, group string) {
	fmt.Fprintln(w, "Payverge maintenance commands (no HTTP server is started):")
	for _, cmd := range cliCommands() {
		if group != "" && cmd.group != group {
			continue
		}
		fmt.Fprintf(w, "  server %-48s %s\n", cmd.usage, cmd.summary)
	}
	fmt.Fprintln(w, "\nDatabase: --db-host --db-port --db-user --db-name --db-sslmode, else DB_HOST/DB_PORT/DB_USER/DB_NAME/DB_SSLMODE,")
	fmt.Fprintln(w, "else the running server's flags (inside the container). The password is read from DB_PASSWORD only.")
}

// newCLIFlagSet builds a quiet FlagSet whose errors surface as usage errors.
// The flag package's own messages echo raw argument values, and an operator
// who pastes a password where a flag belongs must never see it printed back,
// so its output is discarded and parseCLIFlags reports a redacted message.
func newCLIFlagSet(env *cliEnv, name string) (*flag.FlagSet, *cliDBConfig) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {
		fmt.Fprintf(env.stderr, "flags for server %s:\n", name)
		fs.SetOutput(env.stderr)
		fs.PrintDefaults()
		fs.SetOutput(io.Discard)
	}
	db := &cliDBConfig{}
	fs.StringVar(&db.Host, "db-host", "", "PostgreSQL host (env DB_HOST)")
	fs.StringVar(&db.Port, "db-port", "", "PostgreSQL port (env DB_PORT)")
	fs.StringVar(&db.User, "db-user", "", "PostgreSQL user (env DB_USER)")
	fs.StringVar(&db.Name, "db-name", "", "PostgreSQL database (env DB_NAME)")
	fs.StringVar(&db.SSLMode, "db-sslmode", "", "PostgreSQL sslmode (env DB_SSLMODE)")
	return fs, db
}

func parseCLIFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return fmt.Errorf("%w: %s", errCLIUsage, redactFlagError(fs, err))
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("%w: unexpected positional argument (value not shown; passwords are read from stdin only)", errCLIUsage)
	}
	return nil
}

// redactFlagError rewrites a flag.Parse error without the offending value.
// Only names of flags this FlagSet defines are echoed.
func redactFlagError(fs *flag.FlagSet, err error) string {
	msg := err.Error()
	defined := ""
	if m := flagNameInError.FindStringSubmatch(msg); m != nil && fs.Lookup(m[1]) != nil {
		defined = "--" + m[1]
	}
	switch {
	case strings.HasPrefix(msg, "flag needs an argument") && defined != "":
		return defined + " needs a value"
	case strings.HasPrefix(msg, "invalid ") && defined != "":
		return "invalid value for " + defined + " (value not shown)"
	case strings.HasPrefix(msg, "flag provided but not defined"):
		return "unknown flag (value not shown; see --help)"
	default:
		return "malformed flags (value not shown; see --help)"
	}
}

var flagNameInError = regexp.MustCompile(`-([a-z][a-z0-9-]*)(?::|$)`)

// resolveCLIDBConfig fills unset fields: flag > env > running server's flags
// > the server's own defaults. Inside the container the server receives its
// DB settings as argv (compose `command:`), not env, so without the PID 1
// fallback `docker compose exec` would dial localhost.
func resolveCLIDBConfig(env *cliEnv, flags cliDBConfig) cliDBConfig {
	inherited := map[string]string{}
	if env.serverArgs != nil {
		inherited = parseServerDBFlags(env.serverArgs())
	}
	pick := func(flagValue, envKey, serverFlag, fallback string) string {
		if v := strings.TrimSpace(flagValue); v != "" {
			return v
		}
		if v := strings.TrimSpace(env.getenv(envKey)); v != "" {
			return v
		}
		if v := strings.TrimSpace(inherited[serverFlag]); v != "" {
			return v
		}
		return fallback
	}
	return cliDBConfig{
		Host:     pick(flags.Host, "DB_HOST", "db-host", "localhost"),
		Port:     pick(flags.Port, "DB_PORT", "db-port", "5432"),
		User:     pick(flags.User, "DB_USER", "db-user", "payverge"),
		Name:     pick(flags.Name, "DB_NAME", "db-name", "payverge"),
		SSLMode:  pick(flags.SSLMode, "DB_SSLMODE", "db-sslmode", "require"),
		Password: strings.TrimSpace(env.getenv("DB_PASSWORD")),
	}
}

// parseServerDBFlags extracts --db-* values (both "--k v" and "--k=v", one
// or two dashes) from a server argv. --db-password is deliberately ignored.
func parseServerDBFlags(argv []string) map[string]string {
	out := map[string]string{}
	if len(argv) == 0 || filepath.Base(argv[0]) != "server" {
		return out
	}
	wanted := map[string]bool{"db-host": true, "db-port": true, "db-user": true, "db-name": true, "db-sslmode": true}
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		value, hasValue := "", false
		if k, v, ok := strings.Cut(name, "="); ok {
			name, value, hasValue = k, v, true
		}
		if !wanted[name] {
			continue
		}
		if !hasValue && i+1 < len(argv) {
			value = argv[i+1]
			i++
		}
		out[name] = value
	}
	return out
}

// readPID1Args returns PID 1's argv when this process is not PID 1 itself
// (i.e. we were started by `docker exec` next to the server). Best effort.
func readPID1Args() []string {
	if os.Getpid() == 1 {
		return nil
	}
	raw, err := os.ReadFile("/proc/1/cmdline")
	if err != nil || len(raw) == 0 {
		return nil
	}
	return strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
}

// openCLIDatabase connects with the server's own pool settings and refuses a
// schema that is not exactly at this binary's migration head.
func openCLIDatabase(cfg cliDBConfig) (*gorm.DB, error) {
	database.InitDB(database.NewConfig(cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode))
	db := database.GetDB()
	head, err := database.LatestMigrationVersion("migrations")
	if err != nil {
		return nil, fmt.Errorf("resolve migration head (run from the server's working directory, /app in the image): %w", err)
	}
	if err := database.VerifySchemaAtVersion(db, head); err != nil {
		return nil, fmt.Errorf("database schema is not at this binary's migration head; start the server once so it migrates, then retry: %w", err)
	}
	return db, nil
}

func (env *cliEnv) open(flags cliDBConfig) (*gorm.DB, error) {
	return env.openDB(resolveCLIDBConfig(env, flags))
}

// readPasswordFromStdin reads one line (or everything up to EOF) and strips
// only the line terminator, so passphrases keep inner and edge spaces.
func readPasswordFromStdin(r io.Reader) (string, error) {
	if r == nil {
		return "", fmt.Errorf("%w: --password-stdin needs a password on stdin", errCLIUsage)
	}
	reader := bufio.NewReader(io.LimitReader(r, cliMaxPasswordBytes+1))
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	if len(line) > cliMaxPasswordBytes {
		return "", fmt.Errorf("%w: password on stdin is longer than %d bytes", errCLIUsage, cliMaxPasswordBytes)
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	if line == "" {
		return "", fmt.Errorf("%w: no password on stdin (pipe it in: printf '%%s' \"$PW\" | server … --password-stdin)", errCLIUsage)
	}
	return line, nil
}
