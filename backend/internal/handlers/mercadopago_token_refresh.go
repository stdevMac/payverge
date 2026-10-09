package handlers

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
)

const (
	// mpTokenRefreshWindow is how far ahead of token_expires_at we proactively refresh.
	mpTokenRefreshWindow = 30 * 24 * time.Hour
	// mpOAuthMaxRefreshFailures is consecutive refresh failures before flagging reauth.
	mpOAuthMaxRefreshFailures = 3
	// mpTokenRefreshHTTPTimeout bounds each provider refresh call.
	mpTokenRefreshHTTPTimeout = 20 * time.Second
)

// RefreshStats summarizes one Mercado Pago OAuth token-refresh sweep.
type RefreshStats struct {
	Refreshed int
	Skipped   int
	Failed    int
}

// RefreshMercadoPagoTokens refreshes OAuth access tokens for enabled mercadopago
// businesses whose token_expires_at is within 30 days of now.
//
// Secrets (access_token / refresh_token) are persisted via UpdateBusinessPluginConfig
// so they remain encrypted at rest. Failure counters and oauth_status use
// MergeBusinessPluginConfigFields (non-secret fields only).
//
// Mercado Pago refresh tokens are single-use: a successful refresh always
// replaces refresh_token in the same config write as the new access_token.
// After mpOAuthMaxRefreshFailures consecutive failures, oauth_status is set to
// "reauth_required" so the operator UI can prompt reconnection.
//
// db is accepted for call-site symmetry with other jobs; package-level database
// helpers (SetTestDB / GetDB) own the connection used here.
func RefreshMercadoPagoTokens(db *database.DB, client *mercadopago.OAuthClient, now time.Time) RefreshStats {
	stats := RefreshStats{}
	_ = db // connection is package-global (see GetBusinessPluginConfig)
	if client == nil {
		return stats
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	businessIDs, err := database.ListBusinessesWithPluginEnabled("mercadopago")
	if err != nil {
		log.Printf("MercadoPago token refresh: list businesses failed: %v", err)
		return stats
	}

	for _, businessID := range businessIDs {
		cfg, cfgErr := database.GetBusinessPluginConfig(businessID, "mercadopago")
		if cfgErr != nil {
			log.Printf("MercadoPago token refresh: load config business=%d: %v", businessID, cfgErr)
			stats.Failed++
			continue
		}

		mode, _ := cfg["connection_mode"].(string)
		if strings.TrimSpace(mode) != "oauth" {
			stats.Skipped++
			continue
		}

		refreshToken, _ := cfg["refresh_token"].(string)
		refreshToken = strings.TrimSpace(refreshToken)
		if refreshToken == "" {
			stats.Skipped++
			continue
		}

		if !mpTokenNeedsRefresh(cfg, now) {
			stats.Skipped++
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), mpTokenRefreshHTTPTimeout)
		tokens, refreshErr := client.Refresh(ctx, refreshToken)
		cancel()
		if refreshErr != nil {
			log.Printf("MercadoPago token refresh: refresh failed business=%d: %v", businessID, refreshErr)
			if failErr := recordMPTokenRefreshFailure(businessID, cfg); failErr != nil {
				log.Printf("MercadoPago token refresh: record failure business=%d: %v", businessID, failErr)
			}
			stats.Failed++
			continue
		}

		if persistErr := persistMPTokenRefreshSuccess(businessID, cfg, tokens, now); persistErr != nil {
			log.Printf("MercadoPago token refresh: persist failed business=%d: %v", businessID, persistErr)
			// Tokens may already be rotated at MP; still count as failed so ops notice.
			if failErr := recordMPTokenRefreshFailure(businessID, cfg); failErr != nil {
				log.Printf("MercadoPago token refresh: record failure business=%d: %v", businessID, failErr)
			}
			stats.Failed++
			continue
		}
		stats.Refreshed++
	}

	return stats
}

// mpTokenNeedsRefresh reports whether token_expires_at is missing/unparseable
// or falls within the 30-day proactive refresh window (including already expired).
func mpTokenNeedsRefresh(cfg map[string]interface{}, now time.Time) bool {
	raw, _ := cfg["token_expires_at"].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	expiresAt, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		// Tolerate RFC3339Nano or bare date if operators hand-edited config.
		if t2, err2 := time.Parse(time.RFC3339Nano, raw); err2 == nil {
			expiresAt = t2
		} else {
			return true
		}
	}
	return !expiresAt.After(now.Add(mpTokenRefreshWindow))
}

func persistMPTokenRefreshSuccess(businessID uint, cfg map[string]interface{}, tokens *mercadopago.OAuthTokens, now time.Time) error {
	if cfg == nil {
		cfg = map[string]interface{}{}
	}
	// Full-map write via UpdateBusinessPluginConfig encrypts access_token/refresh_token.
	cfg["access_token"] = tokens.AccessToken
	// MP refresh tokens are single-use — always store the new one when present.
	if rt := strings.TrimSpace(tokens.RefreshToken); rt != "" {
		cfg["refresh_token"] = rt
	}
	expiresAt := now.Add(time.Duration(tokens.ExpiresIn) * time.Second)
	if tokens.ExpiresIn <= 0 {
		expiresAt = now.Add(180 * 24 * time.Hour)
	}
	cfg["token_expires_at"] = expiresAt.Format(time.RFC3339)
	cfg["oauth_refresh_failures"] = 0
	cfg["oauth_status"] = "connected"
	if pk := strings.TrimSpace(tokens.PublicKey); pk != "" {
		cfg["public_key"] = pk
	}
	return database.UpdateBusinessPluginConfig(businessID, "mercadopago", cfg)
}

func recordMPTokenRefreshFailure(businessID uint, cfg map[string]interface{}) error {
	failures := configIntField(cfg, "oauth_refresh_failures") + 1
	fields := []database.MergeBusinessPluginConfigField{
		{Key: "oauth_refresh_failures", Value: failures},
	}
	if failures >= mpOAuthMaxRefreshFailures {
		fields = append(fields, database.MergeBusinessPluginConfigField{
			Key:   "oauth_status",
			Value: "reauth_required",
		})
	}
	return database.MergeBusinessPluginConfigFields(businessID, "mercadopago", fields...)
}

func configIntField(cfg map[string]interface{}, key string) int {
	if cfg == nil {
		return 0
	}
	v, ok := cfg[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	default:
		return 0
	}
}
