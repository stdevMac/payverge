package services

import (
	"encoding/json"
	"fmt"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// PluginService provides runtime plugin functionality
type PluginService struct {
	db *database.DB
}

// NewPluginService creates a new plugin service
func NewPluginService(db *database.DB) *PluginService {
	return &PluginService{
		db: db,
	}
}

// IsPluginActive checks if a plugin is active for a business
func (ps *PluginService) IsPluginActive(businessID uint, pluginName string) bool {
	enabled, err := database.IsPluginEnabledForBusiness(businessID, pluginName)
	if err != nil {
		return false
	}
	return enabled
}

// EnableDefaultPaymentPlugins is intentionally a no-op.
//
// Product lock (BA dinner wedge): crypto/USDC must stay selective and must never
// door-lead a restaurant's payment setup. Mercado Pago and other real checkout
// rails are operator-enabled under Plugins; auto-enabling wallet rails made the
// marketplace read as "crypto-only" for new venues. Kept as a stable API so
// older call sites / tests can still invoke it safely.
func (ps *PluginService) EnableDefaultPaymentPlugins(businessID uint) error {
	_ = businessID
	return nil
}

// GetPluginConfig returns the configuration for a business plugin
func (ps *PluginService) GetPluginConfig(businessID uint, pluginName string) (map[string]interface{}, error) {
	return database.GetBusinessPluginConfig(businessID, pluginName)
}

// UpdatePluginConfig updates the configuration for a business plugin
func (ps *PluginService) UpdatePluginConfig(businessID uint, pluginName string, config map[string]interface{}) error {
	return database.UpdateBusinessPluginConfig(businessID, pluginName, config)
}

// MergePluginConfigFields atomically merges a few NON-SECRET fields into a
// business plugin's config without a whole-blob read-modify-write, so a
// concurrent operator edit (e.g. a reconnect that changes chat_id) is not
// clobbered by a stale in-memory copy (N-5). Only for plaintext status fields.
func (ps *PluginService) MergePluginConfigFields(businessID uint, pluginName string, fields ...database.MergeBusinessPluginConfigField) error {
	return database.MergeBusinessPluginConfigFields(businessID, pluginName, fields...)
}

// GetPluginConfigString returns a specific config value as string
func (ps *PluginService) GetPluginConfigString(businessID uint, pluginName, key string) (string, error) {
	config, err := ps.GetPluginConfig(businessID, pluginName)
	if err != nil {
		return "", err
	}

	if value, exists := config[key]; exists {
		if str, ok := value.(string); ok {
			return str, nil
		}
		return fmt.Sprintf("%v", value), nil
	}

	return "", fmt.Errorf("config key '%s' not found", key)
}

// Plugin-specific helper methods

// ValidatePluginConfig validates plugin configuration against schema
func (ps *PluginService) ValidatePluginConfig(pluginName string, config map[string]interface{}) error {
	plugin, err := database.GetPluginByName(pluginName)
	if err != nil {
		return fmt.Errorf("plugin not found: %w", err)
	}

	if plugin.ConfigSchema == "" {
		return nil // No schema to validate against
	}

	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(plugin.ConfigSchema), &schema); err != nil {
		return fmt.Errorf("invalid plugin schema: %w", err)
	}

	// Basic validation - check required fields
	if required, exists := schema["required"]; exists {
		if requiredFields, ok := required.([]interface{}); ok {
			for _, field := range requiredFields {
				if fieldName, ok := field.(string); ok {
					if _, exists := config[fieldName]; !exists {
						return fmt.Errorf("required field '%s' is missing", fieldName)
					}
				}
			}
		}
	}

	return nil
}
