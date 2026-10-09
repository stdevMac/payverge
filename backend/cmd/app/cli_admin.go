package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/auth"
	"github.com/stdevmac/payverge/backend/internal/auththrottle"
	"github.com/stdevmac/payverge/backend/internal/session"
)

// passwordLoginThrottleKind matches the kind AuthHandler.Login records, so a
// reset also lifts a lockout the operator may have tripped while locked out.
const passwordLoginThrottleKind = auth.PasswordLoginThrottleKind

type adminCredentialFlags struct {
	email         string
	passwordStdin bool
}

// parseAdminCredentialArgs parses --email/--password-stdin plus DB flags and
// reads + validates the password BEFORE any database work.
func parseAdminCredentialArgs(env *cliEnv, name string, args []string) (adminCredentialFlags, cliDBConfig, string, error) {
	fs, dbFlags := newCLIFlagSet(env, name)
	var f adminCredentialFlags
	fs.StringVar(&f.email, "email", "", "account email (required)")
	fs.BoolVar(&f.passwordStdin, "password-stdin", false, "read the new password from stdin (required; never pass it as an argument)")
	if err := parseCLIFlags(fs, args); err != nil {
		return f, cliDBConfig{}, "", err
	}
	f.email = strings.TrimSpace(f.email)
	if f.email == "" {
		return f, cliDBConfig{}, "", fmt.Errorf("%w: --email is required", errCLIUsage)
	}
	if !f.passwordStdin {
		return f, cliDBConfig{}, "", fmt.Errorf("%w: --password-stdin is required (passwords are never accepted as arguments)", errCLIUsage)
	}
	password, err := readPasswordFromStdin(env.stdin)
	if err != nil {
		return f, cliDBConfig{}, "", err
	}
	if err := auth.ValidateAdminPassword(password); err != nil {
		return f, cliDBConfig{}, "", err
	}
	return f, *dbFlags, password, nil
}

// runAdminCreate: `server admin create --email E --password-stdin`.
// Same semantics as ADMIN_EMAIL/ADMIN_PASSWORD at boot: creates a verified
// admin with an email login; leaves an existing admin's password alone; takes
// an existing non-admin account over (password replaced, other sign-in methods
// unlinked, sessions revoked) or refuses it when a wallet is linked.
func runAdminCreate(ctx context.Context, env *cliEnv, args []string) error {
	f, dbFlags, password, err := parseAdminCredentialArgs(env, "admin create", args)
	if err != nil {
		return err
	}
	db, err := env.open(dbFlags)
	if err != nil {
		return err
	}
	res, err := auth.EnsureBootstrapAdmin(ctx, db, f.email, password)
	if errors.Is(err, auth.ErrBootstrapAdminWalletLinked) {
		return fmt.Errorf("%s belongs to an existing account with a linked wallet; it is never promoted automatically and nothing was changed. Create the admin with an email no account uses yet", f.email)
	}
	if errors.Is(err, auth.ErrAdminEmailCredentialInUse) {
		return errCredentialHeldByOtherAccount(f.email)
	}
	if err != nil {
		return err
	}
	printReleasedDeadCredentials(env, f.email, res.ReleasedDeadCredentials)
	switch {
	case res.Created:
		fmt.Fprintf(env.stdout, "Created platform admin %s (user id %d). Sign in with this email and password.\n", f.email, res.UserID)
	case res.TookOver:
		fmt.Fprintf(env.stdout, "Took over existing account %s (user id %d) as platform admin: its password is now the one you supplied, %d other sign-in method(s) were unlinked and all its sessions were signed out.\n", f.email, res.UserID, res.UnlinkedSignInMethods)
	default:
		fmt.Fprintf(env.stdout, "%s (user id %d) is already a platform admin. Password NOT changed; use `server admin reset-password` to set one.\n", f.email, res.UserID)
	}
	return nil
}

// runAdminResetPassword: `server admin reset-password --email E --password-stdin`.
// Works for any active account (the operator owns the database). The email
// login is (re)created and verified, every operator session is revoked and
// the password-login lockout is cleared.
func runAdminResetPassword(ctx context.Context, env *cliEnv, args []string) error {
	f, dbFlags, password, err := parseAdminCredentialArgs(env, "admin reset-password", args)
	if err != nil {
		return err
	}
	db, err := env.open(dbFlags)
	if err != nil {
		return err
	}
	res, err := auth.SetEmailPassword(ctx, db, f.email, password)
	if errors.Is(err, auth.ErrAdminUserNotFound) {
		return fmt.Errorf("no active account with email %s (create one with `server admin create`)", f.email)
	}
	if errors.Is(err, auth.ErrAdminEmailCredentialInUse) {
		return errCredentialHeldByOtherAccount(f.email)
	}
	if err != nil {
		return err
	}
	printReleasedDeadCredentials(env, f.email, res.ReleasedDeadCredentials)
	var followUp []error
	if err := session.NewStore(db).RevokeAllLinkedOperatorSessions(res.UserID, res.Addresses, session.RevocationReasonSecurityReset); err != nil {
		followUp = append(followUp, fmt.Errorf("revoke existing sessions: %w", err))
	}
	if err := auththrottle.New(db, auththrottle.Config{}).Clear(f.email, passwordLoginThrottleKind); err != nil {
		followUp = append(followUp, fmt.Errorf("clear login lockout: %w", err))
	}
	if res.CredentialCreated {
		fmt.Fprintf(env.stdout, "Added an email/password login for %s (user id %d).\n", f.email, res.UserID)
	} else {
		fmt.Fprintf(env.stdout, "Password updated for %s (user id %d).\n", f.email, res.UserID)
	}
	if len(followUp) > 0 {
		return fmt.Errorf("password was changed, but: %w", errors.Join(followUp...))
	}
	fmt.Fprintln(env.stdout, "All existing sessions for this account were signed out.")
	return nil
}

func errCredentialHeldByOtherAccount(email string) error {
	return fmt.Errorf("another active account holds an email/password login for %s, so this account could not sign in with it; nothing was changed. Use an email no other account signs in with", email)
}

// printReleasedDeadCredentials reports logins removed because a deleted
// account still held them for this address and they shadowed the new one.
func printReleasedDeadCredentials(env *cliEnv, email string, n int) {
	if n > 0 {
		fmt.Fprintf(env.stdout, "Removed %d email/password login(s) for %s left by a deleted account; they would have blocked sign-in.\n", n, email)
	}
}
