package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demo"

	"gorm.io/gorm"
)

// cliDemoService is the slice of demo.Service the CLI drives.
type cliDemoService interface {
	EnsureForAdmin(ctx context.Context, adminUserID uint) (*database.DemoInstance, error)
	ResetForAdmin(ctx context.Context, adminUserID uint) (*database.DemoInstance, error)
}

// newCLIDemoService builds the demo service without the S3 asset re-hoster:
// a maintenance command must not fetch stock photos over the network.
func newCLIDemoService(db *gorm.DB) cliDemoService {
	return demo.NewService(db, demo.Options{EmailDomain: os.Getenv("ADMIN_DEMO_EMAIL_DOMAIN")})
}

// runDemoSeed: `server demo seed [--owner-email E]`. Idempotent — a second
// run re-ensures the same demo instance instead of duplicating restaurants.
func runDemoSeed(ctx context.Context, env *cliEnv, args []string) error {
	fs, dbFlags := newCLIFlagSet(env, "demo seed")
	ownerEmail := fs.String("owner-email", "", "platform admin who owns the demo restaurants (default: ADMIN_EMAIL)")
	if err := parseCLIFlags(fs, args); err != nil {
		return err
	}
	email := strings.TrimSpace(*ownerEmail)
	if email == "" {
		email = strings.TrimSpace(env.getenv(config.AdminEmailEnv))
	}
	if email == "" {
		return fmt.Errorf("%w: --owner-email is required (or set ADMIN_EMAIL)", errCLIUsage)
	}
	db, err := env.open(*dbFlags)
	if err != nil {
		return err
	}
	owner, err := findCLIDemoOwner(ctx, db, email)
	if err != nil {
		return err
	}
	instance, err := env.newDemo(db).EnsureForAdmin(ctx, owner.ID)
	if err != nil {
		return fmt.Errorf("seed demo data: %w", err)
	}
	fmt.Fprintf(env.stdout, "Demo restaurants ready for %s (demo instance %d, status %s).\n", email, instance.ID, instance.Status)
	fmt.Fprintf(env.stdout, "Showroom cash drawers open for guests: %d.\n", openPublicDemoDrawers(ctx, db))
	printDemoBusinesses(ctx, env, db, instance)
	return nil
}

// runDemoReset: `server demo reset (--owner-email E | --all)`. Wraps
// demo.Service.ResetForAdmin: wipes only rows the demo seeder owns, then
// re-seeds a fresh window.
func runDemoReset(ctx context.Context, env *cliEnv, args []string) error {
	fs, dbFlags := newCLIFlagSet(env, "demo reset")
	ownerEmail := fs.String("owner-email", "", "reset this admin's demo restaurants")
	all := fs.Bool("all", false, "reset every existing demo instance")
	if err := parseCLIFlags(fs, args); err != nil {
		return err
	}
	email := strings.TrimSpace(*ownerEmail)
	if (email == "") == !*all {
		return fmt.Errorf("%w: pass exactly one of --owner-email or --all", errCLIUsage)
	}
	db, err := env.open(*dbFlags)
	if err != nil {
		return err
	}
	svc := env.newDemo(db)

	var ownerIDs []uint
	if email != "" {
		owner, err := findCLIDemoOwner(ctx, db, email)
		if err != nil {
			return err
		}
		ownerIDs = []uint{owner.ID}
	} else {
		if err := db.WithContext(ctx).Model(&database.DemoInstance{}).Order("admin_user_id ASC").Pluck("admin_user_id", &ownerIDs).Error; err != nil {
			return fmt.Errorf("list demo instances: %w", err)
		}
		if len(ownerIDs) == 0 {
			fmt.Fprintln(env.stdout, "No demo instances exist; nothing to reset. Use `server demo seed` to create one.")
			return nil
		}
	}
	var errs []error
	for _, id := range ownerIDs {
		instance, err := svc.ResetForAdmin(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("admin %d: %w", id, err))
			continue
		}
		fmt.Fprintf(env.stdout, "Demo data reset for admin user %d (demo instance %d, status %s).\n", id, instance.ID, instance.Status)
	}
	fmt.Fprintf(env.stdout, "Showroom cash drawers open for guests: %d.\n", openPublicDemoDrawers(ctx, db))
	return errors.Join(errs...)
}

// findCLIDemoOwner resolves an active platform admin by email. Demo
// restaurants are an admin showroom, so a non-admin is refused rather than
// silently promoted.
func findCLIDemoOwner(ctx context.Context, db *gorm.DB, email string) (database.User, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	var users []database.User
	if err := db.WithContext(ctx).
		Where("LOWER(TRIM(email)) = ? AND deleted_at IS NULL", normalized).
		Order("id ASC").Limit(1).Find(&users).Error; err != nil {
		return database.User{}, fmt.Errorf("look up %s: %w", email, err)
	}
	if len(users) == 0 {
		return database.User{}, fmt.Errorf("no active account with email %s (create the admin first: `server admin create`)", email)
	}
	if users[0].Role != "admin" {
		return database.User{}, fmt.Errorf("%s is not a platform admin; demo restaurants belong to an admin. Do not run `server admin create` on this account to fix that: on an existing non-admin account it takes the account over (password replaced, other logins unlinked, sessions signed out). Seed for a dedicated admin email instead", email)
	}
	return users[0], nil
}

func printDemoBusinesses(ctx context.Context, env *cliEnv, db *gorm.DB, instance *database.DemoInstance) {
	if instance == nil {
		return
	}
	var ids []uint
	for _, id := range []*uint{instance.PrimaryBusinessID, instance.SecondaryBusinessID} {
		if id != nil {
			ids = append(ids, *id)
		}
	}
	if len(ids) == 0 {
		return
	}
	var rows []struct {
		ID        uint
		Name      string
		CustomURL string
	}
	if err := db.WithContext(ctx).Model(&database.Business{}).Select("id, name, custom_url").Where("id IN ?", ids).Order("id ASC").Scan(&rows).Error; err != nil {
		return
	}
	for _, row := range rows {
		fmt.Fprintf(env.stdout, "  - %s (business id %d, /%s)\n", row.Name, row.ID, row.CustomURL)
	}
}
