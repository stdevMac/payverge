package stripe

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// setupStripeNormalizeDB points the database package at a fresh in-memory
// SQLite and registers the stripe catalog row.
func setupStripeNormalizeDB(t *testing.T) *database.Plugin {
	t.Helper()

	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()), time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Plugin{}, &database.BusinessPlugin{}))

	plugin := &database.Plugin{
		Name: "stripe", DisplayName: "Stripe",
		Category: database.PluginCategoryPayment, IsActive: true,
	}
	require.NoError(t, gormDB.Create(plugin).Error)
	return plugin
}

func newStripeNormalizeBusiness(t *testing.T, name string) *database.Business {
	t.Helper()
	business := &database.Business{
		BusinessId: fmt.Sprintf("%s-%d", name, time.Now().UnixNano()),
		Name:       name,
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

// TestNormalizeConfig_RejectsForgedOAuthClaim is the config-save half of the
// Connect tenant-isolation guarantee. ValidateConfig's OAuth branch only checks
// that stripe_user_id looks like an acct_ id, so without normalization anyone
// with plugin-write permission could declare a connection to an account they do
// not own — and Connect webhooks for that account would then route here.
func TestNormalizeConfig_RejectsForgedOAuthClaim(t *testing.T) {
	setupStripeNormalizeDB(t)
	business := newStripeNormalizeBusiness(t, "ForgedClaim")

	plugin := NewStripePlugin(services.NewPluginService(database.GetDBWrapper()))
	normalized, err := plugin.NormalizeConfig(business.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_someone_elses_account",
		"oauth_status":    "connected",
		"live_mode":       true,
		"access_token":    "sk_live_not_ours",
	})
	require.NoError(t, err)

	assert.NotContains(t, normalized, "stripe_user_id")
	assert.NotContains(t, normalized, "oauth_status")
	assert.NotContains(t, normalized, "live_mode")
	assert.NotContains(t, normalized, "access_token")
	assert.NotContains(t, normalized, "connection_mode",
		"a claim with no grant on file must fall back to manual validation")

	// And the fallback is fail-closed: manual mode demands real merchant keys.
	assert.Error(t, plugin.ValidateConfig(normalized))
}

// TestNormalizeConfig_KeepsGrantOnUnrelatedSave: an ordinary settings save must
// neither disconnect the merchant nor let the request restate the grant.
func TestNormalizeConfig_KeepsGrantOnUnrelatedSave(t *testing.T) {
	pluginRow := setupStripeNormalizeDB(t)
	business := newStripeNormalizeBusiness(t, "ConnectedMerchant")
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRow.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_real_grant",
		"oauth_status":    "connected",
		"live_mode":       true,
		"access_token":    "sk_live_real_grant_token",
	}))

	plugin := NewStripePlugin(services.NewPluginService(database.GetDBWrapper()))

	// Save that says nothing about the connection.
	normalized, err := plugin.NormalizeConfig(business.ID, map[string]interface{}{"auto_capture": false})
	require.NoError(t, err)
	assert.Equal(t, "oauth", normalized["connection_mode"])
	assert.Equal(t, "acct_real_grant", normalized["stripe_user_id"])
	assert.Equal(t, "connected", normalized["oauth_status"])
	assert.Equal(t, "sk_live_real_grant_token", normalized["access_token"])
	assert.Equal(t, false, normalized["auto_capture"])
	assert.NoError(t, plugin.ValidateConfig(normalized))

	// Save that tries to repoint the existing grant at another account.
	hijacked, err := plugin.NormalizeConfig(business.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_attacker",
		"oauth_status":    "connected",
	})
	require.NoError(t, err)
	assert.Equal(t, "acct_real_grant", hijacked["stripe_user_id"],
		"a config save must never repoint the connected account")
}

// TestNormalizeConfig_ReauthStatusSurvivesSave: after a deauthorization the
// operator must not be able to clear reauth_required by saving the form — the
// grant is gone on Stripe's side and charges would fail.
func TestNormalizeConfig_ReauthStatusSurvivesSave(t *testing.T) {
	pluginRow := setupStripeNormalizeDB(t)
	business := newStripeNormalizeBusiness(t, "DeauthorizedMerchant")
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRow.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_revoked",
		"oauth_status":    "reauth_required",
		"enabled":         false,
	}))

	plugin := NewStripePlugin(services.NewPluginService(database.GetDBWrapper()))
	normalized, err := plugin.NormalizeConfig(business.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"oauth_status":    "connected",
		"enabled":         true,
	})
	require.NoError(t, err)

	assert.Equal(t, "reauth_required", normalized["oauth_status"])
	assert.Error(t, plugin.ValidateConfig(normalized),
		"a revoked connection must not validate until the merchant reconnects")
}

// TestNormalizeConfig_AllowsSwitchBackToManualKeys: pinning the grant must not
// trap the operator in OAuth mode. Explicitly saving manual keys drops the
// grant instead of silently keeping it.
func TestNormalizeConfig_AllowsSwitchBackToManualKeys(t *testing.T) {
	pluginRow := setupStripeNormalizeDB(t)
	business := newStripeNormalizeBusiness(t, "SwitchingBackMerchant")
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRow.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  "acct_leaving",
		"oauth_status":    "connected",
		"access_token":    "sk_live_leaving_token",
	}))

	plugin := NewStripePlugin(services.NewPluginService(database.GetDBWrapper()))
	normalized, err := plugin.NormalizeConfig(business.ID, map[string]interface{}{
		"connection_mode": "manual",
		"secret_key":      "sk_live_merchant_own_key_value",
		"publishable_key": "pk_live_merchant_own_key_value",
	})
	require.NoError(t, err)

	assert.Equal(t, "manual", normalized["connection_mode"])
	assert.NotContains(t, normalized, "stripe_user_id")
	assert.NotContains(t, normalized, "access_token")
	assert.NoError(t, plugin.ValidateConfig(normalized))
}

// TestNormalizeConfig_FirstManualSetupUnaffected: the plain manual onboarding
// path (no row yet) must pass through untouched.
func TestNormalizeConfig_FirstManualSetupUnaffected(t *testing.T) {
	setupStripeNormalizeDB(t)
	business := newStripeNormalizeBusiness(t, "FirstManualSetup")

	plugin := NewStripePlugin(services.NewPluginService(database.GetDBWrapper()))
	normalized, err := plugin.NormalizeConfig(business.ID, map[string]interface{}{
		"secret_key":      "sk_live_first_setup_key_value",
		"publishable_key": "pk_live_first_setup_key_value",
		"webhook_secret":  "whsec_first_setup_value",
	})
	require.NoError(t, err)

	assert.Equal(t, "sk_live_first_setup_key_value", normalized["secret_key"])
	assert.Equal(t, "whsec_first_setup_value", normalized["webhook_secret"])
	assert.NoError(t, plugin.ValidateConfig(normalized))
}
