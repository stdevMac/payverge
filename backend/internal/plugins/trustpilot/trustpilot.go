package trustpilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// TrustpilotPlugin implements the MarketingPlugin interface
type TrustpilotPlugin struct {
	pluginService *services.PluginService
}

// NewTrustpilotPlugin creates a new Trustpilot plugin instance
func NewTrustpilotPlugin(pluginService *services.PluginService) *TrustpilotPlugin {
	return &TrustpilotPlugin{
		pluginService: pluginService,
	}
}

// GetName returns the plugin name
func (tp *TrustpilotPlugin) GetName() string {
	return "trustpilot"
}

// GetDisplayName returns the user-facing name
func (tp *TrustpilotPlugin) GetDisplayName() string {
	return "Trustpilot Reviews"
}

// GetDescription returns the plugin description
func (tp *TrustpilotPlugin) GetDescription() string {
	return "Collect customer reviews and build trust with simple business profile integration"
}

// GetCategory returns the plugin category
func (tp *TrustpilotPlugin) GetCategory() string {
	return "marketing"
}

// GetVersion returns the plugin version
func (tp *TrustpilotPlugin) GetVersion() string {
	return "1.0.0"
}

// GetFeatures returns the features as JSON string. Only the implemented
// review-link generator and profile link are advertised — Trustpilot review
// display, widgets, and invitation sending are not built.
func (tp *TrustpilotPlugin) GetFeatures() string {
	return `["Review Link Generator", "Business Profile Link"]`
}

// IsActive returns whether the plugin is active on the platform
func (tp *TrustpilotPlugin) IsActive() bool {
	return true // Trustpilot plugin is always active
}

// TrustpilotConfig represents the Trustpilot configuration
type TrustpilotConfig struct {
	BusinessName  string `json:"business_name" validate:"required"`
	TrustpilotURL string `json:"trustpilot_url" validate:"required,url"`
}

// GetConfigSchema returns the JSON schema for configuration
func (tp *TrustpilotPlugin) GetConfigSchema() string {
	return `{
		"type": "object",
		"required": ["business_name", "trustpilot_url"],
		"properties": {
			"business_name": {
				"type": "string",
				"title": "Business Name",
				"description": "Your business name as it appears on Trustpilot"
			},
			"trustpilot_url": {
				"type": "string",
				"format": "uri",
				"title": "Trustpilot Profile URL",
				"description": "Your Trustpilot business profile URL"
			}
		}
	}`
}

// ValidateConfig validates the plugin configuration
func (tp *TrustpilotPlugin) ValidateConfig(config map[string]interface{}) error {
	// Check required fields
	businessName, ok := config["business_name"].(string)
	if !ok || businessName == "" {
		return errors.New("business_name is required")
	}

	trustpilotURL, ok := config["trustpilot_url"].(string)
	if !ok || trustpilotURL == "" {
		return errors.New("trustpilot_url is required")
	}

	parsedURL, err := url.Parse(trustpilotURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return errors.New("trustpilot_url must be a valid URL")
	}
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return errors.New("trustpilot_url must use http or https")
	}
	if !isTrustpilotHost(parsedURL.Hostname()) {
		return errors.New("trustpilot_url must be a Trustpilot URL")
	}

	return nil
}

// isTrustpilotHost reports whether host is trustpilot.com or a subdomain of it.
// A plain strings.HasSuffix(host, "trustpilot.com") is unsafe: it also matches
// lookalike domains like "evil-trustpilot.com" or "nottrustpilot.com". Matching
// on an exact host or a dot-boundary suffix (".trustpilot.com") closes that gap.
func isTrustpilotHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "trustpilot.com" || strings.HasSuffix(host, ".trustpilot.com")
}

// Initialize sets up the plugin
func (tp *TrustpilotPlugin) Initialize(businessID uint, config map[string]interface{}) error {
	// Validate configuration
	if err := tp.ValidateConfig(config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Parse configuration
	var trustpilotConfig TrustpilotConfig
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := json.Unmarshal(configJSON, &trustpilotConfig); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	return nil
}

// IsEnabled checks if the plugin is enabled for a business
func (tp *TrustpilotPlugin) IsEnabled(businessID uint) bool {
	return tp.pluginService.IsPluginActive(businessID, tp.GetName())
}

// GetConfig retrieves the plugin configuration for a business
func (tp *TrustpilotPlugin) GetConfig(businessID uint) (map[string]interface{}, error) {
	config, err := tp.pluginService.GetPluginConfig(businessID, tp.GetName())
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}
	return config, nil
}

// GetTrustpilotURL returns the configured Trustpilot URL
func (tp *TrustpilotPlugin) GetTrustpilotURL(businessID uint) (string, error) {
	config, err := tp.GetConfig(businessID)
	if err != nil {
		return "", err
	}

	trustpilotURL, ok := config["trustpilot_url"].(string)
	if !ok {
		return "", errors.New("trustpilot_url not found in config")
	}

	return trustpilotURL, nil
}

// GenerateReviewLink generates a direct review link for customers
func (tp *TrustpilotPlugin) GenerateReviewLink(businessID uint) (string, error) {
	trustpilotURL, err := tp.GetTrustpilotURL(businessID)
	if err != nil {
		return "", err
	}

	parsedURL, err := url.Parse(trustpilotURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", errors.New("invalid trustpilot_url")
	}

	segments := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		return "", errors.New("trustpilot_url must include a business profile path")
	}
	if segments[0] == "evaluate" || segments[0] == "evaluate-link" {
		parsedURL.RawQuery = ""
		parsedURL.Fragment = ""
		return parsedURL.String(), nil
	}
	if segments[0] != "review" || len(segments) < 2 || strings.TrimSpace(segments[1]) == "" {
		return "", errors.New("trustpilot_url must be a Trustpilot review profile URL")
	}

	parsedURL.Path = "/evaluate/" + strings.Join(segments[1:], "/")
	parsedURL.RawQuery = ""
	parsedURL.Fragment = ""
	return parsedURL.String(), nil
}

// GetStatus returns the current status of the Trustpilot integration
func (tp *TrustpilotPlugin) GetStatus(businessID uint) (map[string]interface{}, error) {
	if !tp.IsEnabled(businessID) {
		return map[string]interface{}{
			"enabled": false,
			"status":  "disabled",
		}, nil
	}

	config, err := tp.GetConfig(businessID)
	if err != nil {
		return map[string]interface{}{
			"enabled": true,
			"status":  "error",
			"error":   err.Error(),
		}, nil
	}

	businessName, _ := config["business_name"].(string)
	trustpilotURL, _ := config["trustpilot_url"].(string)

	return map[string]interface{}{
		"enabled":        true,
		"status":         "active",
		"business_name":  businessName,
		"trustpilot_url": trustpilotURL,
		"configured":     businessName != "" && trustpilotURL != "",
	}, nil
}

// Cleanup performs any necessary cleanup when plugin is disabled
func (tp *TrustpilotPlugin) Cleanup(businessID uint) error {
	// For Trustpilot, no special cleanup is needed
	// Configuration will be preserved in case they re-enable
	return nil
}

// init registers the Trustpilot plugin with the global registry
func init() {
	plugins.RegisterPluginInitializer("trustpilot", func(pluginService *services.PluginService) {
		trustpilotPlugin := NewTrustpilotPlugin(pluginService)
		plugins.GlobalRegistry.RegisterPlugin(trustpilotPlugin)
	})
}

// Ensure TrustpilotPlugin implements the Plugin interface
var _ plugins.Plugin = (*TrustpilotPlugin)(nil)
