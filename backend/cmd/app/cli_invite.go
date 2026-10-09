package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"
)

const (
	cliInviteDefaultDays = 30
	cliInviteMaxDays     = 365
	cliInviteMaxUses     = 10000
	// cliInviteActor is recorded as created_by and in the audit trail, so a
	// batch minted from the server is distinguishable from one minted in the
	// admin API by a signed-in user.
	cliInviteActor = "server-cli"
)

// runInviteCreate: `server invite create [--uses N] [--days D] [--name ...]`.
// Mints a REGISTRATION_MODE=invite batch through the same service the admin
// API uses (hashed code + audit event). The plaintext code is printed once
// on stdout and never stored.
func runInviteCreate(ctx context.Context, env *cliEnv, args []string) error {
	fs, dbFlags := newCLIFlagSet(env, "invite create")
	uses := fs.Int("uses", 1, "how many accounts may sign up with this code")
	days := fs.Int("days", cliInviteDefaultDays, "days until the code expires")
	name := fs.String("name", "Self-hosted invite", "label shown in the batch history")
	owner := fs.String("owner", "", "who is accountable for the batch (default: ADMIN_EMAIL, else \"operator\")")
	reason := fs.String("reason", "Minted with `server invite create`", "why the batch exists (kept in the audit trail)")
	if err := parseCLIFlags(fs, args); err != nil {
		return err
	}
	if *uses < 1 || *uses > cliInviteMaxUses {
		return fmt.Errorf("%w: --uses must be between 1 and %d", errCLIUsage, cliInviteMaxUses)
	}
	if *days < 1 || *days > cliInviteMaxDays {
		return fmt.Errorf("%w: --days must be between 1 and %d", errCLIUsage, cliInviteMaxDays)
	}
	if strings.TrimSpace(*name) == "" || strings.TrimSpace(*reason) == "" {
		return fmt.Errorf("%w: --name and --reason must not be empty", errCLIUsage)
	}
	accountable := strings.TrimSpace(*owner)
	if accountable == "" {
		accountable = strings.TrimSpace(env.getenv(config.AdminEmailEnv))
	}
	if accountable == "" {
		accountable = "operator"
	}
	db, err := env.open(*dbFlags)
	if err != nil {
		return err
	}
	expires := time.Now().UTC().Add(time.Duration(*days) * 24 * time.Hour)
	batch, code, err := runtimecontrol.New(db).CreateInviteBatch(ctx, runtimecontrol.CreateInviteBatchInput{
		Name:      *name,
		CohortCap: *uses,
		Owner:     accountable,
		Reason:    *reason,
		Actor:     cliInviteActor,
		ExpiresAt: expires,
	})
	if errors.Is(err, runtimecontrol.ErrInvalidControlChange) {
		return fmt.Errorf("%w: invalid invite batch (name, uses, owner, reason and a future expiry are required)", errCLIUsage)
	}
	if err != nil {
		return fmt.Errorf("create invite batch: %w", err)
	}
	fmt.Fprintf(env.stdout, "Invite batch %d %q: %d use(s), expires %s.\n", batch.ID, batch.Name, batch.CohortCap, batch.ExpiresAt.Format(time.RFC3339))
	fmt.Fprintf(env.stdout, "Invite code (shown once; only its hash is stored):\n%s\n", code)
	if link := inviteLink(env.getenv("PUBLIC_URL"), code); link != "" {
		fmt.Fprintf(env.stdout, "Signup link: %s\n", link)
	}
	if mode, _ := config.ParseRegistrationMode(env.getenv(config.RegistrationModeEnv)); mode != config.RegistrationModeInvite {
		fmt.Fprintf(env.stderr, "note: REGISTRATION_MODE is %q in this environment; invite codes are only required (and consumed) in invite mode.\n", mode)
	}
	return nil
}

// inviteLink points at the instance's operator sign-in (/dashboard) with
// ?invite_code=; the signup form prefills the code from it. "/" serves the
// venue to guests, so the link must not rely on it. Empty or unparsable
// base → no link.
func inviteLink(base, code string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	u, err := url.Parse(base + "/dashboard")
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	q := u.Query()
	q.Set("invite_code", code)
	u.RawQuery = q.Encode()
	return u.String()
}
