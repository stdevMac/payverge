package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// stripeConnectTenantCount is how many Stripe-enabled businesses the lookup
// fixtures seed. It is deliberately larger than a toy fixture: the legacy shape
// issued one query plus one AES decrypt per business, so its cost is a function
// of this number while the current shape is not.
const stripeConnectTenantCount = 200

// setupStripeConnectLookupDB seeds stripeConnectTenantCount Stripe-enabled
// businesses — a mix of OAuth-connected and manual-key merchants, each carrying
// realistic encrypted secrets — and points the package-level db at them. It
// returns the acct_ id of the single business the webhook path must resolve.
func setupStripeConnectLookupDB(t testing.TB) (targetAcct string, targetBusinessID uint) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open in-memory database")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	SetTestDB(gormDB)
	require.NoError(t, db.AutoMigrate(&Business{}, &Plugin{}, &BusinessPlugin{}))

	plugin := &Plugin{Name: "stripe", DisplayName: "Stripe", Category: PluginCategoryPayment, IsActive: true}
	require.NoError(t, db.Create(plugin).Error)

	for i := 0; i < stripeConnectTenantCount; i++ {
		business := &Business{
			BusinessId:   fmt.Sprintf("stripe-lookup-%d", i),
			Name:         fmt.Sprintf("Stripe Lookup Biz %d", i),
			OwnerAddress: fmt.Sprintf("0xowner%040d", i),
			IsActive:     true,
		}
		require.NoError(t, db.Create(business).Error)

		cfg := map[string]interface{}{
			"connection_mode": "oauth",
			"stripe_user_id":  fmt.Sprintf("acct_lookup_%d", i),
			"oauth_status":    "connected",
			"live_mode":       true,
			"publishable_key": fmt.Sprintf("pk_live_lookup_%040d", i),
			// Secret keys are AES-encrypted at rest; the legacy per-business
			// hydration paid a decrypt for each one of these on every webhook.
			"access_token":  fmt.Sprintf("sk_live_access_%040d", i),
			"refresh_token": fmt.Sprintf("rt_live_refresh_%040d", i),
		}
		// Every fourth merchant is on manual keys, so the lookup also has to skip
		// non-OAuth rows.
		if i%4 == 3 {
			cfg = map[string]interface{}{
				"connection_mode": "manual",
				"secret_key":      fmt.Sprintf("sk_live_manual_%040d", i),
				"publishable_key": fmt.Sprintf("pk_live_manual_%040d", i),
				"webhook_secret":  fmt.Sprintf("whsec_manual_%040d", i),
			}
		}
		require.NoError(t, EnableBusinessPlugin(business.ID, plugin.ID, cfg))

		// The target is the LAST OAuth merchant, so the legacy scan pays its full
		// per-tenant cost before finding it.
		if i%4 != 3 {
			targetAcct = fmt.Sprint(cfg["stripe_user_id"])
			targetBusinessID = business.ID
		}
	}
	return targetAcct, targetBusinessID
}

// newStripeLookupQueryCounter counts every statement issued against the
// package-level db. Query, Row and Raw are all counted: a chain ending in Scan
// executes the Row callbacks, while the per-business hydration it replaced went
// through the Query callbacks — counting only one of them would let a
// regression hide in the other.
func newStripeLookupQueryCounter(t testing.TB) (count *int, cleanup func()) {
	t.Helper()
	n := 0
	bump := func(tx *gorm.DB) { n++ }
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("stripe_lookup_count", bump))
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("stripe_lookup_count_row", bump))
	require.NoError(t, db.Callback().Raw().After("gorm:raw").Register("stripe_lookup_count_raw", bump))
	return &n, func() {
		_ = db.Callback().Query().Remove("stripe_lookup_count")
		_ = db.Callback().Row().Remove("stripe_lookup_count_row")
		_ = db.Callback().Raw().Remove("stripe_lookup_count_raw")
	}
}

// TestFindBusinessIDsByStripeUserIDIsSingleQuery is the access-shape guard for
// the Connect webhook hot path. Resolving account → tenant runs on EVERY
// Connect-delivered event, so it must cost the same at 10 tenants and at 10,000:
//   - exactly ONE query, no per-business config load, and
//   - no secret decryption (connection_mode and stripe_user_id are non-secret
//     keys stored as plaintext inside the config JSON).
func TestFindBusinessIDsByStripeUserIDIsSingleQuery(t *testing.T) {
	acct, businessID := setupStripeConnectLookupDB(t)

	count, cleanup := newStripeLookupQueryCounter(t)
	defer cleanup()

	ids, err := FindBusinessIDsByStripeUserID(acct)
	require.NoError(t, err)

	assert.Equal(t, 1, *count,
		"account→tenant lookup must issue exactly ONE query regardless of tenant count")
	assert.Equal(t, []uint{businessID}, ids)
}

// TestFindBusinessIDsByStripeUserIDIgnoresManualAndDisabled proves the narrow
// projection did not lose the filters the hydrated scan applied.
func TestFindBusinessIDsByStripeUserIDIgnoresManualAndDisabled(t *testing.T) {
	acct, businessID := setupStripeConnectLookupDB(t)

	// A manual-key merchant that happens to store the same account id must never
	// be routed to: manual merchants verify with their own webhook secret and are
	// not part of the Connect topology.
	impostor := &Business{BusinessId: "stripe-impostor", Name: "Impostor", IsActive: true}
	require.NoError(t, db.Create(impostor).Error)
	var plugin Plugin
	require.NoError(t, db.Where("name = ?", "stripe").First(&plugin).Error)
	require.NoError(t, EnableBusinessPlugin(impostor.ID, plugin.ID, map[string]interface{}{
		"connection_mode": "manual",
		"stripe_user_id":  acct,
		"secret_key":      "sk_live_impostor_key_value",
	}))

	// A disabled OAuth row for the same account must also drop out.
	disabled := &Business{BusinessId: "stripe-disabled", Name: "Disabled", IsActive: true}
	require.NoError(t, db.Create(disabled).Error)
	require.NoError(t, EnableBusinessPlugin(disabled.ID, plugin.ID, map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  acct,
		"oauth_status":    "connected",
	}))
	require.NoError(t, db.Model(&BusinessPlugin{}).
		Where("business_id = ? AND plugin_id = ?", disabled.ID, plugin.ID).
		Update("is_enabled", false).Error)

	ids, err := FindBusinessIDsByStripeUserID(acct)
	require.NoError(t, err)
	assert.Equal(t, []uint{businessID}, ids)

	excluded, err := FindBusinessIDsByStripeUserIDExcluding(acct, businessID)
	require.NoError(t, err)
	assert.Empty(t, excluded, "excluding the owner must leave no other claimant")
}

// findBusinessIDsByStripeUserIDLegacy reproduces the pre-fix lookup: list every
// Stripe-enabled business, then load and DECRYPT each one's config until the
// account matches. Kept only as the benchmark baseline.
func findBusinessIDsByStripeUserIDLegacy(acctID string) ([]uint, error) {
	acctID = strings.TrimSpace(acctID)
	if acctID == "" {
		return nil, nil
	}
	ids, err := ListBusinessesWithPluginEnabled("stripe")
	if err != nil {
		return nil, err
	}
	var matched []uint
	for _, bizID := range ids {
		cfg, err := GetBusinessPluginConfig(bizID, "stripe")
		if err != nil {
			continue
		}
		if strings.ToLower(strings.TrimSpace(fmt.Sprint(cfg["connection_mode"]))) != "oauth" {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(cfg["stripe_user_id"])) == acctID {
			matched = append(matched, bizID)
		}
	}
	return matched, nil
}

// BenchmarkFindBusinessIDsByStripeUserID_Before is the Connect-webhook baseline:
// one query plus one AES decrypt per Stripe-enabled business, per event.
func BenchmarkFindBusinessIDsByStripeUserID_Before(b *testing.B) {
	acct, businessID := setupStripeConnectLookupDB(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ids, err := findBusinessIDsByStripeUserIDLegacy(acct)
		if err != nil {
			b.Fatal(err)
		}
		if len(ids) != 1 || ids[0] != businessID {
			b.Fatalf("unexpected lookup result: %v", ids)
		}
	}
}

// BenchmarkFindBusinessIDsByStripeUserID_After is the single-query, no-decrypt
// lookup that ships.
func BenchmarkFindBusinessIDsByStripeUserID_After(b *testing.B) {
	acct, businessID := setupStripeConnectLookupDB(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ids, err := FindBusinessIDsByStripeUserID(acct)
		if err != nil {
			b.Fatal(err)
		}
		if len(ids) != 1 || ids[0] != businessID {
			b.Fatalf("unexpected lookup result: %v", ids)
		}
	}
}
