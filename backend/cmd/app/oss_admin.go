package main

// Self-host first-admin and demo-data startup wiring (plan A1.7). main.go
// only calls bootstrapPlatformAdmins, demoStartupPlanFromEnv and
// startDemoStartupEnsure; everything else lives here.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/auth"
	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demo"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"gorm.io/gorm"
)

// bootstrapPlatformAdmins runs every configured first-admin path and returns
// the ADMIN_EMAIL admin's user id (0 when ADMIN_EMAIL/ADMIN_PASSWORD are not
// set or were rejected).
//
// The bootstrap admin is written directly, never through signup admission,
// so REGISTRATION_MODE=closed|invite cannot lock the operator out.
func bootstrapPlatformAdmins(ctx context.Context, db *gorm.DB) uint {
	if err := checkRegistrationModeAtStartup(config.IsProductionMode(false), os.Getenv); err != nil {
		logger.Logger.Fatal(err.Error())
	}
	adminID := ensureEnvBootstrapAdmin(ctx, db, os.Getenv)

	if adminID == 0 {
		hintWhenNoPlatformAdmin(ctx, db)
	}
	return adminID
}

// checkRegistrationModeAtStartup validates REGISTRATION_MODE. An unknown
// value always resolves to "closed" (a typo can never open signup), but in
// production that silent lock-out is itself a misconfiguration. The
// production preflight (config.ValidateProduction, registration_mode.invalid)
// already refuses it before the database is touched; this is the backstop
// for any path that skips preflight. Outside production it only logs the
// error.
func checkRegistrationModeAtStartup(production bool, getenv func(string) string) error {
	mode, err := config.ParseRegistrationMode(getenv(config.RegistrationModeEnv))
	if err != nil {
		if production {
			return fmt.Errorf("%v — refusing to start in production: set REGISTRATION_MODE to invite, open or closed (or unset it for the default, %s)", err, config.DefaultRegistrationMode)
		}
		logger.Logger.Errorf("%v — signup is CLOSED until REGISTRATION_MODE is one of invite, open, closed", err)
	}
	logger.Logger.Infof("Registration mode: %s", mode)
	return nil
}

// ensureEnvBootstrapAdmin applies ADMIN_EMAIL/ADMIN_PASSWORD. It never logs
// the password, and a rejected password leaves the database untouched.
func ensureEnvBootstrapAdmin(ctx context.Context, db *gorm.DB, getenv func(string) string) uint {
	email := strings.TrimSpace(getenv(config.AdminEmailEnv))
	password := getenv(config.AdminPasswordEnv)
	if email == "" && password == "" {
		return 0
	}
	if email != "" && password == "" {
		// install.sh removes a generated ADMIN_PASSWORD after the first start
		// and the docs say to keep ADMIN_EMAIL, so this is the normal state of
		// every installed instance. Only warn when that admin does not exist.
		if id, ok := existingPlatformAdminID(ctx, db, email); ok {
			logger.Logger.Infof("Bootstrap admin %s already exists; %s is not needed", logger.RedactEmail(email), config.AdminPasswordEnv)
			return id
		}
		logger.Logger.Warnf("Bootstrap admin skipped: %s is set but no platform admin has that address; set %s too (only one is set)", config.AdminEmailEnv, config.AdminPasswordEnv)
		return 0
	}
	if email == "" || password == "" {
		logger.Logger.Warnf("Bootstrap admin skipped: set both %s and %s (only one is set)", config.AdminEmailEnv, config.AdminPasswordEnv)
		return 0
	}
	res, err := auth.EnsureBootstrapAdmin(ctx, db, email, password)
	if err != nil {
		switch {
		case isAdminPasswordPolicyError(err):
			logger.Logger.Errorf("Bootstrap admin NOT created: %s rejected: %v. Choose a unique passphrase of at least %d characters and restart.", config.AdminPasswordEnv, err, auth.AdminPasswordMinLength)
		case errors.Is(err, auth.ErrBootstrapAdminWalletLinked):
			logger.Logger.Errorf("Bootstrap admin NOT applied: %s belongs to an existing non-admin account with a linked wallet, which is never promoted automatically. Nothing was changed. Set %s to an address no account uses yet and restart.", config.AdminEmailEnv, config.AdminEmailEnv)
		case errors.Is(err, auth.ErrAdminEmailCredentialInUse):
			logger.Logger.Errorf("Bootstrap admin NOT applied: another active account holds an email/password login for %s, so an admin with that address could not sign in. Nothing was changed. Set %s to an address no account uses yet and restart.", config.AdminEmailEnv, config.AdminEmailEnv)
		default:
			logger.Logger.Errorf("Bootstrap admin for %s failed: %v", logger.RedactEmail(email), err)
		}
		return 0
	}
	switch {
	case res.Created:
		logger.Logger.Infof("Bootstrap admin %s created (user id %d)", logger.RedactEmail(email), res.UserID)
	case res.TookOver:
		// Warn, not Info: an account someone else may have registered was
		// re-keyed. Its old password and sign-in links no longer work.
		logger.Logger.Warnf("Bootstrap admin %s matched an existing non-admin account (user id %d); took it over: password set from %s, %d other sign-in method(s) unlinked, all sessions signed out, promoted to admin.",
			logger.RedactEmail(email), res.UserID, config.AdminPasswordEnv, res.UnlinkedSignInMethods)
	}
	if res.ReleasedDeadCredentials > 0 {
		logger.Logger.Warnf("Bootstrap admin %s: removed %d email/password login(s) for this address left by a deleted account; they would have blocked the admin's sign-in.",
			logger.RedactEmail(email), res.ReleasedDeadCredentials)
	}
	if res.PasswordIgnored {
		logger.Logger.Infof("%s ignored: %s already exists and its password is never overwritten at boot. Use `/app/server admin reset-password --email … --password-stdin` to change it, then remove %s from the environment.",
			config.AdminPasswordEnv, logger.RedactEmail(email), config.AdminPasswordEnv)
	}
	return res.UserID
}

// existingPlatformAdminID returns the id of the active platform admin with
// this email, if there is one.
func existingPlatformAdminID(ctx context.Context, db *gorm.DB, email string) (uint, bool) {
	var users []database.User
	err := db.WithContext(ctx).Select("id").
		Where("LOWER(TRIM(email)) = ? AND role = ? AND deleted_at IS NULL", auth.NormalizeEmail(email), string(structs.RoleAdmin)).
		Order("id ASC").Limit(1).Find(&users).Error
	if err != nil || len(users) == 0 {
		return 0, false
	}
	return users[0].ID, true
}

func isAdminPasswordPolicyError(err error) bool {
	return errors.Is(err, auth.ErrAdminPasswordTooShort) ||
		errors.Is(err, auth.ErrAdminPasswordTooLong) ||
		errors.Is(err, auth.ErrAdminPasswordWeak)
}

func hintWhenNoPlatformAdmin(ctx context.Context, db *gorm.DB) {
	var admins int64
	if err := db.WithContext(ctx).Model(&database.User{}).Where("role = ? AND deleted_at IS NULL", string(structs.RoleAdmin)).Count(&admins).Error; err != nil || admins > 0 {
		return
	}
	logger.Logger.Warnf("No platform admin exists yet. Set %s and %s, or run `/app/server admin create --email you@example.com --password-stdin`.", config.AdminEmailEnv, config.AdminPasswordEnv)
}

// demoStartupPlan says which demo-data jobs this boot runs.
type demoStartupPlan struct {
	// Append runs the hourly AppendDueDays scheduler, which only advances
	// demo instances that already exist.
	Append bool
	// EnsureAllAdmins gives every platform admin a showroom (the upstream
	// hosted-instance behaviour; needs ADMIN_DEMO_AUTOMATION_ENABLED=true).
	EnsureAllAdmins bool
	// SeedOwner seeds one showroom owned by the bootstrap admin (DEMO_DATA).
	SeedOwner bool
}

// demoStartupPlanFromEnv: demo automation is OFF unless asked for.
//
//   - ADMIN_DEMO_AUTOMATION_ENABLED set → it alone decides the hourly append
//     and the every-admin ensure (truthy: 1/true/yes/on).
//   - unset → the hourly append follows DEMO_DATA, so a seeded showroom keeps
//     moving; no admin other than the owner ever gets one.
func demoStartupPlanFromEnv() demoStartupPlan {
	return demoStartupPlanFrom(os.Getenv("ADMIN_DEMO_AUTOMATION_ENABLED"), config.DemoDataEnabled())
}

func demoStartupPlanFrom(automationRaw string, demoData bool) demoStartupPlan {
	automationRaw = strings.ToLower(strings.TrimSpace(automationRaw))
	if automationRaw == "" {
		return demoStartupPlan{Append: demoData, SeedOwner: demoData}
	}
	hosted := false
	switch automationRaw {
	case "1", "true", "yes", "on":
		hosted = true
	}
	return demoStartupPlan{Append: hosted, EnsureAllAdmins: hosted, SeedOwner: demoData}
}

// demoStartupEnsurer is the slice of demo.Service the startup ensure needs.
type demoStartupEnsurer interface {
	EnsureForAdmin(ctx context.Context, adminUserID uint) (*database.DemoInstance, error)
	EnsureAllAdmins(ctx context.Context) error
}

// startDemoStartupEnsure runs the boot-time demo ensure in the background.
// It must be called after s3.Init: EnsureSeedAssets needs the S3 client.
func startDemoStartupEnsure(svc demoStartupEnsurer, plan demoStartupPlan, ownerID uint) {
	if !plan.EnsureAllAdmins && !plan.SeedOwner {
		return
	}
	logger.SafeGo(func() {
		runDemoStartupEnsure(context.Background(), database.GetDB(), svc, plan, ownerID, func() (int, error) {
			return demo.EnsureSeedAssets(nil, nil)
		})
	})
}

func runDemoStartupEnsure(ctx context.Context, db *gorm.DB, svc demoStartupEnsurer, plan demoStartupPlan, ownerID uint, ensureAssets func() (int, error)) {
	if !plan.EnsureAllAdmins && !plan.SeedOwner {
		return
	}
	// Self-heal the demo photography first: seeded menus reference
	// demo-arg/assets/... keys that ship embedded in the binary. Non-fatal —
	// a broken bucket must not block demo seeding or startup.
	if ensureAssets != nil {
		if uploaded, err := ensureAssets(); err != nil {
			logger.Logger.Warnf("Demo seed asset ensure incomplete (%d uploaded): %v", uploaded, err)
		} else if uploaded > 0 {
			logger.Logger.Infof("Demo seed assets: uploaded %d missing object(s) to public bucket", uploaded)
		}
	}
	if plan.EnsureAllAdmins {
		if err := svc.EnsureAllAdmins(ctx); err != nil {
			logger.Logger.Warnf("Admin demo startup ensure failed: %v", err)
		} else {
			logger.Logger.Info("Admin demo startup ensure completed")
		}
		return
	}
	owner, err := resolveDemoOwner(ctx, db, ownerID)
	if err != nil {
		logger.Logger.Warnf("DEMO_DATA=true but no demo owner: %v", err)
		return
	}
	if _, err := svc.EnsureForAdmin(ctx, owner); err != nil {
		logger.Logger.Warnf("DEMO_DATA seed for admin %d failed: %v", owner, err)
		return
	}
	logger.Logger.Infof("DEMO_DATA: demo restaurants ready for admin user %d", owner)
	// Open the showroom drawers so a guest can pay at the counter before any
	// operator has visited Caja. A drawer an operator closed stays closed.
	logger.Logger.Infof("DEMO_DATA: %d showroom cash drawer(s) open for guests", openPublicDemoDrawers(ctx, db))
}

// resolveDemoOwner prefers the ADMIN_EMAIL admin and falls back to the oldest
// platform admin (e.g. one created with `server admin create`).
func resolveDemoOwner(ctx context.Context, db *gorm.DB, ownerID uint) (uint, error) {
	if ownerID != 0 {
		return ownerID, nil
	}
	var ids []uint
	if err := db.WithContext(ctx).Model(&database.User{}).
		Where("role = ? AND deleted_at IS NULL", string(structs.RoleAdmin)).
		Order("id ASC").Limit(1).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, fmt.Errorf("no platform admin exists; set %s/%s or run `/app/server demo seed --owner-email …` after creating one", config.AdminEmailEnv, config.AdminPasswordEnv)
	}
	return ids[0], nil
}
