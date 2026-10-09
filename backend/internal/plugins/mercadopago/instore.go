package mercadopago

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"

	appconfig "github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
)

// Config keys persisted after first store/POS provisioning.
const (
	configKeyMPUserID        = "mp_user_id"
	configKeyMPStoreID       = "mp_store_id"
	configKeyMPExternalPOSID = "mp_external_pos_id"
)

// EnsureStoreAndPOS is the exported wrapper for handlers (QR charge flow).
func (m *MercadoPagoPlugin) EnsureStoreAndPOS(businessID uint) (externalPOSID string, err error) {
	return m.ensureStoreAndPOS(businessID)
}

// ensureStoreAndPOS returns the business's Mercado Pago external POS id for
// dynamic QR orders. Idempotent:
//
//  1. If config already has mp_external_pos_id, return it with zero HTTP.
//  2. Otherwise create a store (POST /users/{mp_user_id}/stores) and POS
//     (POST /pos), then persist mp_store_id + mp_external_pos_id.
//  3. On 409 / "already exists", look up GET /pos?external_id=... and persist.
//
// Zero commission: no marketplace fees are involved in store/POS provisioning.
func (m *MercadoPagoPlugin) ensureStoreAndPOS(businessID uint) (externalPOSID string, err error) {
	if m == nil || m.pluginService == nil {
		return "", errors.New("mercadopago plugin service not configured")
	}

	rawConfig, err := m.pluginService.GetPluginConfig(businessID, m.GetName())
	if err != nil {
		return "", fmt.Errorf("mercadopago: get config: %w", err)
	}
	if existing := strings.TrimSpace(configString(rawConfig, configKeyMPExternalPOSID)); existing != "" {
		return existing, nil
	}

	config, err := m.configFromMap(rawConfig)
	if err != nil {
		return "", err
	}

	mpUserID := strings.TrimSpace(configString(rawConfig, configKeyMPUserID))
	if mpUserID == "" {
		// Manual-mode merchants never get mp_user_id from OAuth; resolve via
		// GET /users/me with the configured access token and persist it (F9).
		resolved, resolveErr := m.resolveAndPersistMPUserID(businessID, config)
		if resolveErr != nil {
			return "", resolveErr
		}
		mpUserID = resolved
	}

	storeExternalID := fmt.Sprintf("payverge-store-%d", businessID)
	// POS external_id is stricter than the store's: MP rejects anything
	// non-alphanumeric ("external_id must be alphanumeric"), so no hyphens.
	posExternalID := fmt.Sprintf("PAYVERGEPOS%d", businessID)

	// The MP store is named after the business; this instance's
	// PRODUCT_NAME is only the fallback for a nameless business.
	storeName := appconfig.ProductName()
	if biz, bizErr := database.GetBusinessByID(businessID); bizErr == nil && biz != nil {
		if n := strings.TrimSpace(biz.Name); n != "" {
			storeName = n
		}
	}

	// Prefer a previously persisted store id (partial progress) so we only create POS.
	storeID := strings.TrimSpace(configString(rawConfig, configKeyMPStoreID))
	if storeID == "" {
		storeID, err = m.createOrResolveStore(businessID, config, mpUserID, storeName, storeExternalID)
		if err != nil {
			return "", err
		}
	}

	if err := m.createOrResolvePOS(businessID, config, storeID, storeExternalID, posExternalID); err != nil {
		return "", err
	}

	if err := m.pluginService.MergePluginConfigFields(businessID, m.GetName(),
		database.MergeBusinessPluginConfigField{Key: configKeyMPStoreID, Value: storeID},
		database.MergeBusinessPluginConfigField{Key: configKeyMPExternalPOSID, Value: posExternalID},
	); err != nil {
		return "", fmt.Errorf("mercadopago: persist store/POS ids: %w", err)
	}
	return posExternalID, nil
}

// resolveAndPersistMPUserID calls GET /users/me and merges mp_user_id into the
// business plugin config. Returns an actionable error when the call fails
// (bad token, network, empty id).
func (m *MercadoPagoPlugin) resolveAndPersistMPUserID(businessID uint, config *MercadoPagoConfig) (string, error) {
	if config == nil || strings.TrimSpace(config.AccessToken) == "" {
		return "", errors.New("mercadopago: access token is required to resolve mp_user_id via /users/me")
	}
	result, err := m.makeMercadoPagoAPICallWithIdempotencyKey(
		context.Background(),
		http.MethodGet,
		"/users/me",
		nil,
		config,
		"",
	)
	if err != nil {
		return "", fmt.Errorf("%w: check access token: %v", ErrUsersMeFailed, err)
	}
	userID := stringFromMercadoPagoAny(result["id"])
	if userID == "" {
		// id may be a JSON number.
		if n, ok := result["id"].(float64); ok && n > 0 {
			userID = strconv.FormatInt(int64(n), 10)
		}
	}
	if userID == "" {
		return "", fmt.Errorf("%w: response missing user id; cannot provision store/POS", ErrUsersMeFailed)
	}
	if err := m.pluginService.MergePluginConfigFields(businessID, m.GetName(),
		database.MergeBusinessPluginConfigField{Key: configKeyMPUserID, Value: userID},
	); err != nil {
		return "", fmt.Errorf("mercadopago: persist mp_user_id: %w", err)
	}
	return userID, nil
}

func (m *MercadoPagoPlugin) createOrResolveStore(
	businessID uint,
	config *MercadoPagoConfig,
	mpUserID, storeName, storeExternalID string,
) (string, error) {
	body := map[string]interface{}{
		"name":        storeName,
		"external_id": storeExternalID,
		// Minimal AR-friendly location; MP requires location on many sites.
		// city_name is validated against MP's fixed AR city list, which covers
		// Buenos Aires *province* localities but not the city "Buenos Aires"
		// itself — use La Plata (the provincial capital) with its coordinates.
		"location": map[string]interface{}{
			"street_number": "0",
			"street_name":   "Payverge",
			"city_name":     "La Plata",
			"state_name":    "Buenos Aires",
			"latitude":      -34.9215,
			"longitude":     -57.9545,
		},
	}
	path := "/users/" + url.PathEscape(mpUserID) + "/stores"
	result, err := m.makeMercadoPagoAPICallWithIdempotencyKey(
		context.Background(),
		http.MethodPost,
		path,
		body,
		config,
		uuid.NewString(),
	)
	if err != nil {
		if isMercadoPagoAlreadyExists(err) {
			// Store already exists under our external_id. We only need the numeric
			// store id for POS create; resolve via POS lookup later if needed.
			// Try GET /users/{id}/stores/search?external_id=... when available,
			// otherwise surface a clear error — POS path can still use external_store_id.
			storeID, resolveErr := m.lookupStoreIDByExternalID(config, mpUserID, storeExternalID)
			if resolveErr != nil {
				return "", fmt.Errorf("mercadopago: store already exists but lookup failed: %w", resolveErr)
			}
			return storeID, nil
		}
		if status, ok := mercadoPagoAPIErrorStatus(err); ok && (status == http.StatusForbidden || status == http.StatusUnauthorized) {
			return "", fmt.Errorf("%w: create store: %v", ErrInsufficientTokenScope, err)
		}
		return "", fmt.Errorf("mercadopago: create store: %w", err)
	}
	storeID := stringFromMercadoPagoAny(result["id"])
	if storeID == "" {
		return "", errors.New("mercadopago: create store response missing id")
	}
	_ = businessID // reserved for future logging / metrics
	return storeID, nil
}

func (m *MercadoPagoPlugin) createOrResolvePOS(
	businessID uint,
	config *MercadoPagoConfig,
	storeID, storeExternalID, posExternalID string,
) error {
	body := map[string]interface{}{
		"name":              appconfig.ProductName(),
		"fixed_amount":      false,
		"store_id":          parseStoreIDForBody(storeID),
		"external_store_id": storeExternalID,
		"external_id":       posExternalID,
	}
	_, err := m.makeMercadoPagoAPICallWithIdempotencyKey(
		context.Background(),
		http.MethodPost,
		"/pos",
		body,
		config,
		uuid.NewString(),
	)
	if err != nil {
		if isMercadoPagoAlreadyExists(err) {
			// Confirm the existing POS is addressable by external_id.
			if _, lookupErr := m.lookupPOSByExternalID(config, posExternalID); lookupErr != nil {
				return fmt.Errorf("mercadopago: POS already exists but lookup failed: %w", lookupErr)
			}
			return nil
		}
		if status, ok := mercadoPagoAPIErrorStatus(err); ok && (status == http.StatusForbidden || status == http.StatusUnauthorized) {
			return fmt.Errorf("%w: create POS: %v", ErrInsufficientTokenScope, err)
		}
		return fmt.Errorf("mercadopago: create POS: %w", err)
	}
	_ = businessID
	return nil
}

func (m *MercadoPagoPlugin) lookupPOSByExternalID(config *MercadoPagoConfig, externalID string) (map[string]interface{}, error) {
	q := url.Values{}
	q.Set("external_id", externalID)
	path := "/pos?" + q.Encode()
	result, err := m.makeMercadoPagoAPICallWithIdempotencyKey(
		context.Background(),
		http.MethodGet,
		path,
		nil,
		config,
		"",
	)
	if err != nil {
		return nil, err
	}
	// Response may be a single object, or {results: [...]}, or {paging, results}.
	if id := stringFromMercadoPagoAny(result["id"]); id != "" || stringFromMercadoPagoAny(result["external_id"]) != "" {
		return result, nil
	}
	if results, ok := result["results"].([]interface{}); ok {
		for _, raw := range results {
			row, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if stringFromMercadoPagoAny(row["external_id"]) == externalID || stringFromMercadoPagoAny(row["id"]) != "" {
				return row, nil
			}
		}
	}
	return nil, fmt.Errorf("mercadopago: POS not found for external_id %q", externalID)
}

func (m *MercadoPagoPlugin) lookupStoreIDByExternalID(config *MercadoPagoConfig, mpUserID, storeExternalID string) (string, error) {
	// MP search endpoint for stores by external_id.
	q := url.Values{}
	q.Set("external_id", storeExternalID)
	path := "/users/" + url.PathEscape(mpUserID) + "/stores/search?" + q.Encode()
	result, err := m.makeMercadoPagoAPICallWithIdempotencyKey(
		context.Background(),
		http.MethodGet,
		path,
		nil,
		config,
		"",
	)
	if err != nil {
		return "", err
	}
	if id := stringFromMercadoPagoAny(result["id"]); id != "" {
		return id, nil
	}
	if results, ok := result["results"].([]interface{}); ok {
		for _, raw := range results {
			row, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if stringFromMercadoPagoAny(row["external_id"]) == storeExternalID {
				if id := stringFromMercadoPagoAny(row["id"]); id != "" {
					return id, nil
				}
			}
			// Some responses put the match first without filtering client-side.
			if id := stringFromMercadoPagoAny(row["id"]); id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("mercadopago: store not found for external_id %q", storeExternalID)
}

// isMercadoPagoAlreadyExists detects 409 Conflict and common "already exists"
// 400 bodies so provisioning can recover via GET lookup.
func isMercadoPagoAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	status, ok := mercadoPagoAPIErrorStatus(err)
	if ok && status == http.StatusConflict {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "already_exists") ||
		// Store create duplicates surface as 400 bad_request with
		// "external id '...' is already assigned to this user".
		strings.Contains(msg, "already assigned") ||
		strings.Contains(msg, "duplicated") ||
		strings.Contains(msg, "duplicate") {
		return true
	}
	// 400 with those phrases is covered above; also treat bare 400 as "maybe exists"
	// only when the body mentions external_id conflicts.
	if ok && status == http.StatusBadRequest {
		if strings.Contains(msg, "external_id") && (strings.Contains(msg, "exist") || strings.Contains(msg, "unique")) {
			return true
		}
	}
	return false
}

func configString(cfg map[string]interface{}, key string) string {
	if cfg == nil {
		return ""
	}
	return stringFromMercadoPagoAny(cfg[key])
}

// parseStoreIDForBody prefers a JSON number when the store id is numeric so the
// body matches Mercado Pago's documented store_id type; falls back to string.
func parseStoreIDForBody(storeID string) interface{} {
	storeID = strings.TrimSpace(storeID)
	if n, err := strconv.ParseInt(storeID, 10, 64); err == nil {
		return n
	}
	return storeID
}
