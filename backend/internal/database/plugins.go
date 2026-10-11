package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/security"

	"gorm.io/gorm"
)

// Plugin represents a platform plugin that can be activated by businesses
type Plugin struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	Name                string    `gorm:"uniqueIndex;not null" json:"name"`           // Internal name (e.g., "stripe", "mercadopago")
	DisplayName         string    `gorm:"not null" json:"display_name"`               // User-facing name (e.g., "Stripe Integration")
	Description         string    `gorm:"type:text" json:"description"`               // Short plugin description (deprecated, use Message)
	Message             string    `gorm:"type:text" json:"message"`                   // Enhanced user message explaining what they can achieve
	Image               string    `json:"image"`                                      // Plugin icon/logo URL
	IsActive            bool      `gorm:"default:true" json:"is_active"`              // Platform-level active status
	AdminActiveOverride bool      `gorm:"default:false" json:"admin_active_override"` // When true, registry sync must not overwrite is_active
	ComingSoon          bool      `gorm:"default:false" json:"coming_soon"`           // Whether plugin is coming soon
	Category            string    `gorm:"not null" json:"category"`                   // payment, analytics, integration, etc.
	Version             string    `gorm:"default:'1.0.0'" json:"version"`             // Plugin version
	Features            string    `gorm:"type:text" json:"features"`                  // JSON array of features
	ConfigSchema        string    `gorm:"type:text" json:"config_schema"`             // JSON schema for plugin configuration
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

	// Relationships
	Translations []PluginTranslation `gorm:"foreignKey:PluginID" json:"translations,omitempty"`
}

// BusinessPlugin represents a business's subscription to a plugin
type BusinessPlugin struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	BusinessID uint      `gorm:"index;not null" json:"business_id"`
	PluginID   uint      `gorm:"index;not null" json:"plugin_id"`
	IsEnabled  bool      `gorm:"default:false" json:"is_enabled"` // Business-level enabled status
	Config     string    `gorm:"type:text" json:"config"`         // JSON configuration for this business
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	// Health, written best-effort by the webhook handler so operators can see a
	// plugin failing without reading server logs. LastStatus is "", "ok", or
	// "error". Empty health fields are omitempty so list polls do not invent
	// blank errors / idle status on healthy rows (FIND-059).
	LastStatus    string     `gorm:"type:text;not null;default:''" json:"last_status,omitempty"`
	LastError     string     `gorm:"type:text;not null;default:''" json:"last_error,omitempty"`
	LastErrorAt   *time.Time `json:"last_error_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`

	// Relationships
	Business Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
	Plugin   Plugin   `gorm:"foreignKey:PluginID" json:"plugin,omitempty"`
}

// PluginTranslation represents translations for plugin content
type PluginTranslation struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	PluginID     uint      `gorm:"index;not null" json:"plugin_id"`
	LanguageCode string    `gorm:"size:16;not null" json:"language_code"` // e.g., "en", "es", "es-AR"
	FieldName    string    `gorm:"size:50;not null" json:"field_name"`    // "message", "description"
	Content      string    `gorm:"type:text;not null" json:"content"`     // Translated content
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Relationships
	Plugin Plugin `gorm:"foreignKey:PluginID" json:"plugin,omitempty"`
}

// Plugin Categories
const (
	PluginCategoryPayment     = "payment"
	PluginCategoryAnalytics   = "analytics"
	PluginCategoryIntegration = "integration"
	PluginCategoryReporting   = "reporting"
	PluginCategoryMarketing   = "marketing"
)

var ErrBusinessPluginNotEnabled = errors.New("business plugin not enabled")

// Plugin database operations

// CreatePlugin creates a new plugin (admin only)
func CreatePlugin(plugin Plugin) (*Plugin, error) {
	if err := db.Create(&plugin).Error; err != nil {
		return nil, fmt.Errorf("failed to create plugin: %w", err)
	}
	return &plugin, nil
}

// GetAllPlugins returns all plugins with optional filtering
func GetAllPlugins(activeOnly bool) ([]Plugin, error) {
	var plugins []Plugin
	query := db.Model(&Plugin{})

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	if err := query.Order("category, display_name").Find(&plugins).Error; err != nil {
		return nil, fmt.Errorf("failed to get plugins: %w", err)
	}
	return plugins, nil
}

// GetPluginByID returns a plugin by ID
func GetPluginByID(id uint) (*Plugin, error) {
	var plugin Plugin
	if err := db.First(&plugin, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("plugin not found")
		}
		return nil, fmt.Errorf("failed to get plugin: %w", err)
	}
	return &plugin, nil
}

// GetPluginByName returns a plugin by name
func GetPluginByName(name string) (*Plugin, error) {
	var plugin Plugin
	if err := db.Where("name = ?", name).First(&plugin).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("plugin not found")
		}
		return nil, fmt.Errorf("failed to get plugin: %w", err)
	}
	return &plugin, nil
}

// UpdatePlugin updates a plugin (admin only)
func UpdatePlugin(id uint, updates Plugin) (*Plugin, error) {
	var plugin Plugin
	if err := db.First(&plugin, id).Error; err != nil {
		return nil, fmt.Errorf("plugin not found")
	}

	if err := db.Model(&plugin).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}
	return &plugin, nil
}

// UpdatePluginForSync updates a plugin row from the authoritative Go registry
// (SyncPluginsToDatabase). Unlike UpdatePlugin — which keeps partial-update
// semantics for the admin PATCH endpoint by relying on GORM's zero-value skip —
// this path always writes the registry-managed columns via an explicit Select,
// so that a zero value propagates. This matters most for is_active: GORM's
// Updates(struct) silently skips is_active=false, which meant a plugin whose Go
// IsActive() returns false (e.g. an env-disabled plugin) could never be
// deactivated in the catalog and stayed purchasable. Name/timestamps are left
// out on purpose — Name is the immutable natural key the sync matches on, and
// Message/ComingSoon are not owned by the registry (the seed manages those).
func UpdatePluginForSync(id uint, updates Plugin) (*Plugin, error) {
	var plugin Plugin
	if err := db.First(&plugin, id).Error; err != nil {
		return nil, fmt.Errorf("plugin not found")
	}

	cols := []string{
		"display_name", "description", "image",
		"category", "version", "features", "config_schema",
	}
	if !plugin.AdminActiveOverride {
		cols = append(cols, "is_active")
	}

	if err := db.Model(&plugin).Select(cols).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to sync plugin: %w", err)
	}
	return GetPluginByID(id)
}

// TogglePluginActive toggles the is_active status of a plugin
func TogglePluginActive(id uint) (*Plugin, error) {
	var plugin Plugin
	if err := db.First(&plugin, id).Error; err != nil {
		return nil, fmt.Errorf("plugin not found")
	}

	newStatus := !plugin.IsActive
	if err := db.Model(&plugin).Updates(map[string]interface{}{
		"is_active":             newStatus,
		"admin_active_override": true,
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to toggle plugin status: %w", err)
	}

	plugin.IsActive = newStatus
	plugin.AdminActiveOverride = true
	return GetPluginByID(id)
}

// Business Plugin operations

// GetBusinessPlugins returns business plugin relationships with plugin data
func GetBusinessPlugins(businessID uint) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	// Only return business plugins that are actually enabled/configured
	query := `
		SELECT 
			bp.id,
			bp.business_id,
			bp.plugin_id,
			bp.is_enabled,
			bp.created_at,
			bp.updated_at,
			bp.last_status,
			bp.last_error,
			bp.last_error_at,
			bp.last_success_at,
			p.name,
			p.display_name,
			p.description,
			p.image,
			p.category,
			p.version,
			p.features,
			p.config_schema
		FROM business_plugins bp
		INNER JOIN plugins p ON bp.plugin_id = p.id
		WHERE bp.business_id = ? AND p.is_active = true
		ORDER BY p.category, p.display_name
	`

	if err := db.Raw(query, businessID).Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to get business plugins: %w", err)
	}

	// GORM map scans always materialize string columns (including '') and null
	// timestamps as present keys. Drop idle health noise so the operator list
	// wire only carries real status/errors (FIND-059).
	for _, row := range results {
		scrubBusinessPluginHealthFields(row)
	}

	return results, nil
}

// HasEnabledPaymentPlugin reports whether the business has any enabled
// payment-category plugin. It is an existence check only — features,
// config_schema, and the per-business config blob stay unselected so the
// public delivery-settings payment-mode probe does not hydrate those JSON
// columns (#567).
func HasEnabledPaymentPlugin(conn *gorm.DB, businessID uint) (bool, error) {
	if conn == nil {
		conn = db
	}
	if conn == nil {
		return false, fmt.Errorf("database not initialized")
	}
	var exists bool
	err := conn.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM business_plugins bp
			INNER JOIN plugins p ON p.id = bp.plugin_id
			WHERE bp.business_id = ?
			  AND p.is_active = ?
			  AND p.category = ?
			  AND bp.is_enabled = ?
		)
	`, businessID, true, "payment", true).Scan(&exists).Error
	if err != nil {
		return false, err
	}
	return exists, nil
}

// scrubBusinessPluginHealthFields removes empty health keys from a
// GetBusinessPlugins projection row so JSON omitempty semantics apply even
// though the row is a map[string]interface{} (FIND-059).
// BusinessPluginRowEnabled reports whether a GetBusinessPlugins projection row
// is enabled. Map scans hand back bool on Postgres and int64 (0/1) on the
// SQLite test driver.
func BusinessPluginRowEnabled(row map[string]interface{}) bool {
	switch v := row["is_enabled"].(type) {
	case bool:
		return v
	case int64:
		return v != 0
	case int:
		return v != 0
	case float64:
		return v != 0
	}
	return false
}

func scrubBusinessPluginHealthFields(row map[string]interface{}) {
	if row == nil {
		return
	}
	// #795: a disabled rail must not read as ok/ready. Stale success health
	// from when the rail was enabled (or a decorative seed stamp) is dropped;
	// a recorded error stays — it is a factual failure record, not a
	// readiness claim.
	if !BusinessPluginRowEnabled(row) {
		if s, ok := row["last_status"].(string); ok && s == "ok" {
			delete(row, "last_status")
		}
		delete(row, "last_success_at")
	}
	if s, ok := row["last_error"].(string); ok && strings.TrimSpace(s) == "" {
		delete(row, "last_error")
	}
	if s, ok := row["last_status"].(string); ok && strings.TrimSpace(s) == "" {
		delete(row, "last_status")
	}
	if v, ok := row["last_error_at"]; ok && v == nil {
		delete(row, "last_error_at")
	}
	if v, ok := row["last_success_at"]; ok && v == nil {
		delete(row, "last_success_at")
	}
}

// EnableBusinessPlugin enables a plugin for a business
func EnableBusinessPlugin(businessID, pluginID uint, config map[string]interface{}) error {
	// Encrypt credential-bearing fields (Stripe secret_key, PayPal/MercadoPago
	// secrets, etc.) before they are written to the text column so a DB dump
	// can't reveal payment-provider secrets. No-op (plaintext) when no key is
	// provisioned, so behavior is unchanged until encryption is opted into. (B1)
	config, err := security.EncryptConfigSecrets(config)
	if err != nil {
		return fmt.Errorf("failed to encrypt plugin config secrets: %w", err)
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	businessPlugin := BusinessPlugin{
		BusinessID: businessID,
		PluginID:   pluginID,
		IsEnabled:  true,
		Config:     string(configJSON),
	}

	// Upsert - update if exists, create if not
	if err := db.Where("business_id = ? AND plugin_id = ?", businessID, pluginID).
		Assign(BusinessPlugin{IsEnabled: true, Config: string(configJSON)}).
		FirstOrCreate(&businessPlugin).Error; err != nil {
		return fmt.Errorf("failed to enable plugin: %w", err)
	}

	return nil
}

// DisableBusinessPlugin disables a plugin for a business
func DisableBusinessPlugin(businessID, pluginID uint) error {
	if err := db.Model(&BusinessPlugin{}).
		Where("business_id = ? AND plugin_id = ?", businessID, pluginID).
		Update("is_enabled", false).Error; err != nil {
		return fmt.Errorf("failed to disable plugin: %w", err)
	}
	return nil
}

// RecordBusinessPluginWebhookSuccess records a successful webhook on the
// (business, plugin-by-name) row. It is a single keyed UPDATE (no extra read)
// so it adds negligible cost to the webhook hot path, and is best-effort: a
// failure to record health must never fail webhook processing, so callers log
// and continue. A no-op (plugin not enabled for the business) is not an error.
func RecordBusinessPluginWebhookSuccess(businessID uint, pluginName string) error {
	now := time.Now()
	// Clear last_error_at with the error text so the operator list does not
	// show a stale failure timestamp beside an empty/omitted last_error
	// (FIND-059 honesty).
	return db.Model(&BusinessPlugin{}).
		Where("business_id = ? AND plugin_id = (SELECT id FROM plugins WHERE name = ?)", businessID, pluginName).
		Updates(map[string]interface{}{
			"last_status":     "ok",
			"last_success_at": now,
			"last_error":      "",
			"last_error_at":   nil,
		}).Error
}

// RecordBusinessPluginWebhookFailure records a failed webhook on the
// (business, plugin-by-name) row. Same hot-path/best-effort contract as
// RecordBusinessPluginWebhookSuccess.
func RecordBusinessPluginWebhookFailure(businessID uint, pluginName, errMsg string) error {
	now := time.Now()
	return db.Model(&BusinessPlugin{}).
		Where("business_id = ? AND plugin_id = (SELECT id FROM plugins WHERE name = ?)", businessID, pluginName).
		Updates(map[string]interface{}{
			"last_status":   "error",
			"last_error":    errMsg,
			"last_error_at": now,
		}).Error
}

// IsPluginEnabledForBusiness checks if a plugin is enabled for a business
func IsPluginEnabledForBusiness(businessID uint, pluginName string) (bool, error) {
	var count int64

	query := `
		SELECT COUNT(*) 
		FROM business_plugins bp
		JOIN plugins p ON bp.plugin_id = p.id
		WHERE bp.business_id = ? AND p.name = ? AND bp.is_enabled = true AND p.is_active = true
	`

	if err := db.Raw(query, businessID, pluginName).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check plugin status: %w", err)
	}

	return count > 0, nil
}

// ListBusinessesWithPluginEnabled returns business IDs that have the named
// plugin enabled (and the catalog plugin row is active). Used by background
// jobs such as Mercado Pago OAuth token refresh.
func ListBusinessesWithPluginEnabled(pluginName string) ([]uint, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var ids []uint
	err := db.Model(&BusinessPlugin{}).
		Joins("JOIN plugins ON plugins.id = business_plugins.plugin_id").
		Where("plugins.name = ? AND business_plugins.is_enabled = ? AND plugins.is_active = ?", pluginName, true, true).
		Pluck("business_plugins.business_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list businesses with plugin %q enabled: %w", pluginName, err)
	}
	if ids == nil {
		ids = []uint{}
	}
	return ids, nil
}

// GetBusinessPluginConfig returns the configuration for a business plugin
func GetBusinessPluginConfig(businessID uint, pluginName string) (map[string]interface{}, error) {
	var row struct {
		Config string `gorm:"column:config"`
	}

	query := `
		SELECT COALESCE(bp.config, '{}') as config
		FROM business_plugins bp
		JOIN plugins p ON bp.plugin_id = p.id
		WHERE bp.business_id = ? AND p.name = ? AND bp.is_enabled = true AND p.is_active = true
	`

	dbResult := db.Raw(query, businessID, pluginName).Scan(&row)
	if dbResult.Error != nil {
		return nil, fmt.Errorf("failed to get plugin config: %w", dbResult.Error)
	}
	if dbResult.RowsAffected == 0 {
		return nil, ErrBusinessPluginNotEnabled
	}

	config := strings.TrimSpace(row.Config)
	if config == "" {
		config = "{}"
	}

	var configMap map[string]interface{}
	if err := json.Unmarshal([]byte(config), &configMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Decrypt credential fields so callers (payment plugins, save-side secret
	// preservation) see plaintext exactly as before encryption-at-rest. Legacy
	// plaintext rows pass through untouched. (B1)
	configMap, err := security.DecryptConfigSecrets(configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt plugin config secrets: %w", err)
	}

	return configMap, nil
}

// GetBusinessPaymentPluginConfigs returns name→parsed-config for the business's
// enabled+active plugins among pluginNames, in ONE query — replacing the
// per-plugin IsPluginEnabledForBusiness + GetBusinessPluginConfig pair in the
// guest payment-plugin listing loop. A plugin absent from the result map was
// either not enabled/active or had a malformed config blob (logged + skipped),
// matching the old per-plugin "not configured / config error → skip" behavior.
// An empty pluginNames slice short-circuits with no query.
//
// SECURITY: the returned maps are consumed in-memory for the enabled-gate only
// and must never be written back into the guest response — bp.config holds
// payment-plugin secrets (API keys / credentials). Because this path reads only
// the "enabled" flag (evalPaymentPluginConfigEnabled), it deliberately does NOT
// decrypt secret fields: leaving them as v1: ciphertext keeps this guest hot
// path lean and avoids materializing plaintext credentials it never needs. (B1)
func GetBusinessPaymentPluginConfigs(businessID uint, pluginNames []string) (map[string]map[string]interface{}, error) {
	result := make(map[string]map[string]interface{})
	if len(pluginNames) == 0 {
		return result, nil
	}

	var rows []struct {
		Name   string `gorm:"column:name"`
		Config string `gorm:"column:config"`
	}

	query := `
		SELECT p.name, COALESCE(bp.config, '{}') as config
		FROM business_plugins bp
		JOIN plugins p ON bp.plugin_id = p.id
		WHERE bp.business_id = ? AND p.name IN ? AND bp.is_enabled = true AND p.is_active = true
	`

	if err := db.Raw(query, businessID, pluginNames).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get business payment plugin configs: %w", err)
	}

	for _, row := range rows {
		config := strings.TrimSpace(row.Config)
		if config == "" {
			config = "{}"
		}

		var configMap map[string]interface{}
		if err := json.Unmarshal([]byte(config), &configMap); err != nil {
			// Malformed config blob: omit so the caller skips the plugin,
			// matching the old per-plugin config-error path.
			log.Printf("skipping payment plugin %q for business %d: malformed config: %v", row.Name, businessID, err)
			continue
		}
		result[row.Name] = configMap
	}

	return result, nil
}

func GetBusinessPluginConfigState(businessID uint, pluginName string) (map[string]interface{}, bool, bool, error) {
	var row struct {
		Config           string `gorm:"column:config"`
		IsEnabled        bool   `gorm:"column:is_enabled"`
		PlatformIsActive bool   `gorm:"column:platform_is_active"`
	}

	query := `
		SELECT COALESCE(bp.config, '{}') as config, bp.is_enabled, p.is_active AS platform_is_active
		FROM business_plugins bp
		JOIN plugins p ON bp.plugin_id = p.id
		WHERE bp.business_id = ? AND p.name = ?
	`

	dbResult := db.Raw(query, businessID, pluginName).Scan(&row)
	if dbResult.Error != nil {
		return nil, false, false, fmt.Errorf("failed to get plugin config state: %w", dbResult.Error)
	}
	if dbResult.RowsAffected == 0 {
		return nil, false, false, ErrBusinessPluginNotEnabled
	}

	config := strings.TrimSpace(row.Config)
	if config == "" {
		config = "{}"
	}

	var configMap map[string]interface{}
	if err := json.Unmarshal([]byte(config), &configMap); err != nil {
		return nil, false, false, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Decrypt credential fields so callers see plaintext (see GetBusinessPluginConfig). (B1)
	configMap, err := security.DecryptConfigSecrets(configMap)
	if err != nil {
		return nil, false, false, fmt.Errorf("failed to decrypt plugin config secrets: %w", err)
	}

	return configMap, row.IsEnabled, row.PlatformIsActive, nil
}

// UpdateBusinessPluginConfig updates the configuration for a business plugin
func UpdateBusinessPluginConfig(businessID uint, pluginName string, config map[string]interface{}) error {
	// Encrypt credential-bearing fields before persisting (see EnableBusinessPlugin). (B1)
	config, err := security.EncryptConfigSecrets(config)
	if err != nil {
		return fmt.Errorf("failed to encrypt plugin config secrets: %w", err)
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	query := `
		UPDATE business_plugins 
		SET config = ?, updated_at = ?
		FROM plugins p
		WHERE business_plugins.plugin_id = p.id 
		AND business_plugins.business_id = ? 
		AND p.name = ?
	`

	if err := db.Exec(query, string(configJSON), time.Now(), businessID, pluginName).Error; err != nil {
		return fmt.Errorf("failed to update plugin config: %w", err)
	}

	return nil
}

// MergeBusinessPluginConfigField is a single field to merge into a business
// plugin's stored config blob.
type MergeBusinessPluginConfigField struct {
	Key   string
	Value interface{}
}

// MergeBusinessPluginConfigFields atomically merges a small set of NON-SECRET
// fields into a business plugin's config blob without a read-modify-write of the
// whole config in application memory. It re-reads the CURRENT stored config
// inside a row-locked transaction and overwrites only the given keys, so a
// concurrent operator edit/reconnect (e.g. a new chat_id) committed just before
// this write is preserved instead of being clobbered by a stale in-memory copy
// (N-5). It deliberately bypasses secret encryption/decryption: it must only be
// used for plaintext status fields (last_sent_at, last_error, failure_count, …),
// never credential fields — the surrounding encrypted secret values in the blob
// pass through untouched.
func MergeBusinessPluginConfigFields(businessID uint, pluginName string, fields ...MergeBusinessPluginConfigField) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if len(fields) == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var row struct {
			ID     uint   `gorm:"column:id"`
			Config string `gorm:"column:config"`
		}
		// Lock the business_plugins row for the read-merge-write. On Postgres this
		// serializes concurrent merges; SQLite (tests) serializes writers anyway.
		err := tx.Raw(`
			SELECT bp.id, COALESCE(bp.config, '{}') AS config
			FROM business_plugins bp
			JOIN plugins p ON bp.plugin_id = p.id
			WHERE bp.business_id = ? AND p.name = ?
			FOR UPDATE
		`, businessID, pluginName).Scan(&row)
		if err.Error != nil {
			// SQLite rejects "FOR UPDATE"; retry without the lock clause so tests
			// (and any non-locking engine) still merge correctly.
			if isUnsupportedLockError(err.Error) {
				err = tx.Raw(`
					SELECT bp.id, COALESCE(bp.config, '{}') AS config
					FROM business_plugins bp
					JOIN plugins p ON bp.plugin_id = p.id
					WHERE bp.business_id = ? AND p.name = ?
				`, businessID, pluginName).Scan(&row)
			}
			if err.Error != nil {
				return fmt.Errorf("failed to load plugin config for merge: %w", err.Error)
			}
		}
		if row.ID == 0 {
			return ErrBusinessPluginNotEnabled
		}

		configStr := strings.TrimSpace(row.Config)
		if configStr == "" {
			configStr = "{}"
		}
		var configMap map[string]interface{}
		if e := json.Unmarshal([]byte(configStr), &configMap); e != nil {
			return fmt.Errorf("failed to unmarshal plugin config for merge: %w", e)
		}
		if configMap == nil {
			configMap = map[string]interface{}{}
		}
		for _, f := range fields {
			if f.Value == nil {
				delete(configMap, f.Key)
				continue
			}
			configMap[f.Key] = f.Value
		}
		merged, e := json.Marshal(configMap)
		if e != nil {
			return fmt.Errorf("failed to marshal merged plugin config: %w", e)
		}
		if e := tx.Exec(`UPDATE business_plugins SET config = ?, updated_at = ? WHERE id = ?`,
			string(merged), time.Now(), row.ID).Error; e != nil {
			return fmt.Errorf("failed to write merged plugin config: %w", e)
		}
		return nil
	})
}

// isUnsupportedLockError reports whether a DB error is the "FOR UPDATE not
// supported" class raised by engines like SQLite, so callers can retry without
// the lock clause.
func isUnsupportedLockError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "for update") ||
		strings.Contains(msg, `near "for"`) ||
		strings.Contains(msg, "syntax error")
}

func CountConnectedTelegramBusinesses() (int64, error) {
	type telegramConnectionCandidate struct {
		Config string `gorm:"column:config"`
	}

	var candidates []telegramConnectionCandidate
	query := `
		SELECT COALESCE(bp.config, '{}') AS config
		FROM business_plugins bp
		JOIN plugins p ON bp.plugin_id = p.id
		WHERE p.name = 'telegram' AND bp.is_enabled = true AND p.is_active = true
	`

	if err := db.Raw(query).Scan(&candidates).Error; err != nil {
		return 0, fmt.Errorf("failed to count telegram business connections: %w", err)
	}

	var count int64
	for _, candidate := range candidates {
		config := strings.TrimSpace(candidate.Config)
		if config == "" {
			config = "{}"
		}

		var configMap map[string]interface{}
		if err := json.Unmarshal([]byte(config), &configMap); err != nil {
			continue
		}

		connected, _ := configMap["is_connected"].(bool)
		if !connected {
			continue
		}
		if strings.TrimSpace(fmt.Sprintf("%v", configMap["chat_id"])) == "" {
			continue
		}
		count++
	}

	return count, nil
}

// Plugin Translation operations

// CreateOrUpdatePluginTranslation creates or updates a plugin translation
func CreateOrUpdatePluginTranslation(pluginID uint, languageCode, fieldName, content string) error {
	translation := PluginTranslation{
		PluginID:     pluginID,
		LanguageCode: languageCode,
		FieldName:    fieldName,
		Content:      content,
	}

	// Upsert - update if exists, create if not
	if err := db.Where("plugin_id = ? AND language_code = ? AND field_name = ?", pluginID, languageCode, fieldName).
		Assign(PluginTranslation{Content: content}).
		FirstOrCreate(&translation).Error; err != nil {
		return fmt.Errorf("failed to create/update plugin translation: %w", err)
	}

	return nil
}

// GetPluginsWithTranslations returns all plugins with their translations
func GetPluginsWithTranslations(activeOnly bool, languageCode string) ([]Plugin, error) {
	var plugins []Plugin
	query := db.Model(&Plugin{})

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	// Preload translations only for the exact canonical language requested.
	// Regional marketplace copy must not silently fall back to a base language.
	if languageCode != "" {
		query = query.Preload("Translations", "language_code = ?", languageCode)
	} else {
		query = query.Preload("Translations")
	}

	if err := query.Order("category, display_name").Find(&plugins).Error; err != nil {
		return nil, fmt.Errorf("failed to get plugins with translations: %w", err)
	}
	return plugins, nil
}
