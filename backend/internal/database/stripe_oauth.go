package database

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// stripeOAuthAccountRow is the narrow projection the account lookup reads: the
// tenant id plus the raw config text, never a hydrated/decrypted config map.
type stripeOAuthAccountRow struct {
	BusinessID uint   `gorm:"column:business_id"`
	Config     string `gorm:"column:config"`
}

// stripeConfigString reads a plaintext string field out of a raw config map.
func stripeConfigString(cfg map[string]interface{}, key string) string {
	value, ok := cfg[key]
	if !ok || value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// FindBusinessIDsByStripeUserID returns enabled businesses whose Stripe plugin
// config has connection_mode=oauth and stripe_user_id matching acctID.
// Used for Connect webhook routing (account field → tenant).
//
// This runs on the webhook hot path for every Connect-delivered event, so it is
// deliberately ONE query with a narrow projection and NO secret decryption:
// connection_mode and stripe_user_id are non-secret keys
// (security.IsSecretConfigKey), so they are stored as plaintext inside the
// config JSON and can be read without touching the AES layer. The previous
// shape issued one GetBusinessPluginConfig call per Stripe-enabled business,
// each with its own query plus an AES decrypt of every secret field, which grew
// webhook latency linearly with tenant count. See
// BenchmarkFindBusinessIDsByStripeUserID for the before/after numbers.
func FindBusinessIDsByStripeUserID(acctID string) ([]uint, error) {
	acctID = strings.TrimSpace(acctID)
	if acctID == "" {
		return nil, nil
	}
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}

	var rows []stripeOAuthAccountRow
	err := db.Model(&BusinessPlugin{}).
		Select("business_plugins.business_id AS business_id, COALESCE(business_plugins.config, '{}') AS config").
		Joins("JOIN plugins ON plugins.id = business_plugins.plugin_id").
		Where("plugins.name = ? AND business_plugins.is_enabled = ? AND plugins.is_active = ?", "stripe", true, true).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list stripe connect accounts: %w", err)
	}

	var matched []uint
	for _, row := range rows {
		raw := strings.TrimSpace(row.Config)
		if raw == "" {
			continue
		}
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			continue
		}
		if !strings.EqualFold(stripeConfigString(cfg, "connection_mode"), "oauth") {
			continue
		}
		if stripeConfigString(cfg, "stripe_user_id") == acctID {
			matched = append(matched, row.BusinessID)
		}
	}
	return matched, nil
}

// FindBusinessIDsByStripeUserIDExcluding is FindBusinessIDsByStripeUserID minus
// one business. The OAuth callback uses it to reject connecting a Stripe
// account that another Payverge business already owns: two businesses sharing
// one acct_… makes Connect webhook routing ambiguous for every event that does
// not carry our own checkout metadata.
func FindBusinessIDsByStripeUserIDExcluding(acctID string, excludeBusinessID uint) ([]uint, error) {
	ids, err := FindBusinessIDsByStripeUserID(acctID)
	if err != nil {
		return nil, err
	}
	var out []uint
	for _, id := range ids {
		if id == excludeBusinessID {
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

// MarkStripeOAuthDeauthorized sets oauth_status=reauth_required for the
// OAuth-connected Stripe plugins matching the given connected account id.
// Returns the number of business rows updated.
//
// businessIDs, when non-empty, is the already-resolved tenant set for acctID —
// the webhook path passes it so the account→business lookup runs once per
// delivery instead of twice.
func MarkStripeOAuthDeauthorized(acctID string, businessIDs ...uint) (int, error) {
	acctID = strings.TrimSpace(acctID)
	if acctID == "" {
		return 0, nil
	}
	ids := businessIDs
	if len(ids) == 0 {
		resolved, err := FindBusinessIDsByStripeUserID(acctID)
		if err != nil {
			return 0, err
		}
		ids = resolved
	}
	updated := 0
	for _, bizID := range ids {
		cfg, _, _, err := GetBusinessPluginConfigState(bizID, "stripe")
		if err != nil {
			continue
		}
		if cfg == nil {
			cfg = map[string]interface{}{}
		}
		// Only ever touch a business that actually owns this account. Callers
		// resolve the tenant set themselves, so this is a defence-in-depth
		// check against a caller passing an unrelated business id.
		if !strings.EqualFold(stripeConfigString(cfg, "connection_mode"), "oauth") ||
			stripeConfigString(cfg, "stripe_user_id") != acctID {
			continue
		}
		cfg["oauth_status"] = "reauth_required"
		// Hide the card option from guests. Without this the plugin row stays
		// enabled and evalPaymentPluginConfigEnabled (which reads only
		// config["enabled"]) keeps offering "Pay with card" — every diner then
		// gets a failed checkout, because configFromMap rejects a
		// reauth_required OAuth config. The operator UI reads is_enabled on the
		// row (untouched), so the reconnect CTA still renders.
		cfg["enabled"] = false
		// Keep stripe_user_id so reconnect can detect account change; clear
		// live tokens so we do not keep using a revoked link.
		delete(cfg, "access_token")
		delete(cfg, "refresh_token")
		// Re-encrypt + persist. Plugin stays enabled so the operator UI can
		// show reauth CTA; charges will fail ValidateConfig/configFromMap.
		if err := UpdateBusinessPluginConfig(bizID, "stripe", cfg); err != nil {
			return updated, fmt.Errorf("mark deauth business %d: %w", bizID, err)
		}
		updated++
	}
	return updated, nil
}
