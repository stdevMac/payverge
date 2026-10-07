package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/auth"
	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"github.com/stretchr/testify/require"
)

func TestDemoStartupPlanDefaultsOff(t *testing.T) {
	cases := []struct {
		name       string
		automation string
		demoData   bool
		want       demoStartupPlan
	}{
		{"self-host default: nothing", "", false, demoStartupPlan{}},
		{"DEMO_DATA seeds the owner and keeps it moving", "", true, demoStartupPlan{Append: true, SeedOwner: true}},
		{"hosted opt-in", "true", false, demoStartupPlan{Append: true, EnsureAllAdmins: true}},
		{"hosted opt-in + DEMO_DATA", "on", true, demoStartupPlan{Append: true, EnsureAllAdmins: true, SeedOwner: true}},
		{"explicit off wins over append", "false", true, demoStartupPlan{SeedOwner: true}},
		{"unknown value is off", "maybe", false, demoStartupPlan{}},
		{"whitespace is unset", "  ", false, demoStartupPlan{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, demoStartupPlanFrom(tc.automation, tc.demoData))
		})
	}
	t.Setenv("ADMIN_DEMO_AUTOMATION_ENABLED", "")
	t.Setenv("DEMO_DATA", "")
	require.Equal(t, demoStartupPlan{}, demoStartupPlanFromEnv(), "a fresh self-host install runs no demo jobs and needs no S3")
}

func TestEnsureEnvBootstrapAdmin(t *testing.T) {
	db := newCLITestDB(t)
	env := func(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }
	ctx := context.Background()

	require.Zero(t, ensureEnvBootstrapAdmin(ctx, db, env(nil)))
	require.Zero(t, ensureEnvBootstrapAdmin(ctx, db, env(map[string]string{"ADMIN_EMAIL": "owner@example.com"})), "email alone is not enough")
	require.Zero(t, ensureEnvBootstrapAdmin(ctx, db, env(map[string]string{"ADMIN_EMAIL": "owner@example.com", "ADMIN_PASSWORD": "changeme1234"})), "weak password is refused")
	var n int64
	require.NoError(t, db.Model(&database.User{}).Count(&n).Error)
	require.Zero(t, n)

	vars := map[string]string{"ADMIN_EMAIL": "owner@example.com", "ADMIN_PASSWORD": cliTestPassword}
	id := ensureEnvBootstrapAdmin(ctx, db, env(vars))
	require.NotZero(t, id)
	require.Equal(t, id, ensureEnvBootstrapAdmin(ctx, db, env(vars)), "second boot resolves the same admin")
	require.NoError(t, db.Model(&database.User{}).Count(&n).Error)
	require.Equal(t, int64(1), n)
	var cred auth.UserAuth
	require.NoError(t, db.Where("user_id = ?", id).First(&cred).Error)
	require.True(t, auth.CheckPassword(cliTestPassword, cred.PasswordHash))

	// After install.sh removes ADMIN_PASSWORD, ADMIN_EMAIL alone resolves
	// the existing admin instead of warning that only one is set.
	require.Equal(t, id, ensureEnvBootstrapAdmin(ctx, db, env(map[string]string{"ADMIN_EMAIL": " Owner@Example.com "})))
	require.Zero(t, ensureEnvBootstrapAdmin(ctx, db, env(map[string]string{"ADMIN_EMAIL": "someone-else@example.com"})), "an unknown address is still skipped")
	require.NoError(t, db.Model(&database.User{}).Count(&n).Error)
	require.Equal(t, int64(1), n)
}

func TestEnsureEnvBootstrapAdminTakesOverPreRegisteredAccount(t *testing.T) {
	db := newCLITestDB(t)
	env := func(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }
	hash, err := auth.HashPassword("squatter-chose-this-1")
	require.NoError(t, err)
	squatter := database.User{Email: "owner@example.com", Role: "user", AuthMethod: "email"}
	require.NoError(t, db.Create(&squatter).Error)
	require.NoError(t, db.Create(&auth.UserAuth{UserID: squatter.ID, Provider: "email", ProviderUserID: squatter.Email, PasswordHash: hash}).Error)

	vars := map[string]string{"ADMIN_EMAIL": "owner@example.com", "ADMIN_PASSWORD": cliTestPassword}
	require.Equal(t, squatter.ID, ensureEnvBootstrapAdmin(context.Background(), db, env(vars)))
	var cred auth.UserAuth
	require.NoError(t, db.Where("user_id = ? AND provider = ?", squatter.ID, "email").First(&cred).Error)
	require.True(t, auth.CheckPassword(cliTestPassword, cred.PasswordHash), "ADMIN_PASSWORD replaces a pre-registered password")
	require.False(t, auth.CheckPassword("squatter-chose-this-1", cred.PasswordHash))
	require.True(t, cred.EmailVerified)

	// Wallet-linked accounts are refused: no admin id, nothing promoted.
	require.NoError(t, db.Create(&database.User{Email: "wallet@example.com", Role: "user", Address: "0xabc0000000000000000000000000000000000004"}).Error)
	require.Zero(t, ensureEnvBootstrapAdmin(context.Background(), db, env(map[string]string{"ADMIN_EMAIL": "wallet@example.com", "ADMIN_PASSWORD": cliTestPassword})))
	var walletUser database.User
	require.NoError(t, db.Where("email = ?", "wallet@example.com").First(&walletUser).Error)
	require.Equal(t, "user", walletUser.Role)
}

type fakeStartupDemo struct {
	ensured   []uint
	ensureAll int
	err       error
}

func (f *fakeStartupDemo) EnsureForAdmin(_ context.Context, id uint) (*database.DemoInstance, error) {
	f.ensured = append(f.ensured, id)
	return &database.DemoInstance{AdminUserID: id}, f.err
}

func (f *fakeStartupDemo) EnsureAllAdmins(context.Context) error {
	f.ensureAll++
	return f.err
}

func TestRunDemoStartupEnsure(t *testing.T) {
	db := newCLITestDB(t)
	ctx := context.Background()
	assetCalls := 0
	assets := func() (int, error) { assetCalls++; return 0, errors.New("no bucket") }

	// Off: nothing runs, not even the S3 asset self-heal.
	f := &fakeStartupDemo{}
	runDemoStartupEnsure(ctx, db, f, demoStartupPlan{Append: true}, 7, assets)
	require.Empty(t, f.ensured)
	require.Zero(t, f.ensureAll)
	require.Zero(t, assetCalls)

	// DEMO_DATA with a bootstrap admin: only that admin is seeded; a broken
	// bucket does not block seeding.
	runDemoStartupEnsure(ctx, db, f, demoStartupPlan{SeedOwner: true}, 7, assets)
	require.Equal(t, []uint{7}, f.ensured)
	require.Zero(t, f.ensureAll)
	require.Equal(t, 1, assetCalls)

	// DEMO_DATA without ADMIN_EMAIL: falls back to the oldest admin, or
	// warns when there is none.
	f = &fakeStartupDemo{}
	runDemoStartupEnsure(ctx, db, f, demoStartupPlan{SeedOwner: true}, 0, nil)
	require.Empty(t, f.ensured, "no admin, nothing to seed")
	first := database.User{Email: "first@example.com", Role: "admin"}
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Create(&database.User{Email: "second@example.com", Role: "admin"}).Error)
	runDemoStartupEnsure(ctx, db, f, demoStartupPlan{SeedOwner: true}, 0, nil)
	require.Equal(t, []uint{first.ID}, f.ensured)

	// Hosted automation keeps the every-admin ensure.
	f = &fakeStartupDemo{}
	runDemoStartupEnsure(ctx, db, f, demoStartupPlan{Append: true, EnsureAllAdmins: true, SeedOwner: true}, 7, nil)
	require.Equal(t, 1, f.ensureAll)
	require.Empty(t, f.ensured, "EnsureAllAdmins already covers the owner")
}

// ADM-4: an invalid REGISTRATION_MODE fails closed everywhere, but in
// production it stops startup instead of silently locking signup.
func TestCheckRegistrationModeAtStartup(t *testing.T) {
	logger.InitLogger()
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == config.RegistrationModeEnv {
				return v
			}
			return ""
		}
	}
	for _, valid := range []string{"", "invite", "open", "closed", " Open "} {
		require.NoError(t, checkRegistrationModeAtStartup(true, env(valid)), valid)
		require.NoError(t, checkRegistrationModeAtStartup(false, env(valid)), valid)
	}
	for _, typo := range []string{"opne", "invites", "public", "true"} {
		err := checkRegistrationModeAtStartup(true, env(typo))
		require.Error(t, err, typo)
		require.Contains(t, err.Error(), "refusing to start in production")
		require.NoError(t, checkRegistrationModeAtStartup(false, env(typo)), "development keeps the fail-closed log-only behaviour")
	}
}
