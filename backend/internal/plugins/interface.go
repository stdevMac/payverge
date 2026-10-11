package plugins

import (
	"net/url"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Plugin represents a plugin interface that all plugins must implement
type Plugin interface {
	// GetName returns the plugin name (e.g., "stripe")
	GetName() string

	// GetDisplayName returns the user-facing name
	GetDisplayName() string

	// GetDescription returns the plugin description
	GetDescription() string

	// GetCategory returns the plugin category
	GetCategory() string

	// GetVersion returns the plugin version
	GetVersion() string

	// GetFeatures returns the plugin features as JSON array
	GetFeatures() string

	// GetConfigSchema returns the JSON schema for configuration
	GetConfigSchema() string

	// IsActive returns whether the plugin is active on the platform
	IsActive() bool

	// ValidateConfig validates the plugin configuration
	ValidateConfig(config map[string]interface{}) error

	// Initialize sets up the plugin with the given configuration. Plugins that
	// hold credentials should verify them here (e.g. a test API call) so a bad
	// secret fails fast at save time rather than at first use; Stripe, PayPal,
	// and MercadoPago do this in their Initialize implementations.
	Initialize(businessID uint, config map[string]interface{}) error

	// Cleanup performs any necessary cleanup when plugin is disabled
	Cleanup(businessID uint) error
}

// PaymentPlugin represents a payment processing plugin
type PaymentPlugin interface {
	Plugin

	// ProcessPayment processes a payment using this plugin
	ProcessPayment(businessID uint, amount int64, currency string, metadata map[string]interface{}) (string, error)

	// RefundPayment refunds a payment
	RefundPayment(businessID uint, paymentID string, amount int64) error

	// GetPaymentStatus gets the status of a payment
	GetPaymentStatus(businessID uint, paymentID string) (string, error)

	// CreateBillPayment creates a payment for a specific bill
	CreateBillPayment(businessID uint, billID uint, amount int64, currency string, metadata map[string]interface{}) (*PaymentResponse, error)

	// HandleWebhook processes webhook callbacks from the payment provider
	HandleWebhook(businessID uint, payload []byte, headers map[string]string) (*WebhookResponse, error)

	// GetWebhookEndpoint returns the webhook endpoint path for this plugin
	GetWebhookEndpoint() string

	// VerifyWebhookSignature verifies the webhook signature
	VerifyWebhookSignature(payload []byte, signature string, secret string) bool
}

// WebhookVerificationRequest carries the full provider callback context for
// plugins whose signature schemes need more than one header and a raw body.
type WebhookVerificationRequest struct {
	Payload    []byte
	Headers    map[string]string
	Query      url.Values
	BusinessID uint
	Config     map[string]interface{}
}

// WebhookRequestVerifier is implemented by payment plugins whose official
// verification flow cannot be represented by VerifyWebhookSignature.
type WebhookRequestVerifier interface {
	VerifyWebhookRequest(request WebhookVerificationRequest) error
}

// PaymentReturnCapturer is implemented by redirect-based payment providers
// that require a server-side capture call after buyer approval.
type PaymentReturnCapturer interface {
	CapturePaymentReturn(businessID uint, billID uint, paymentID string) (*WebhookResponse, error)
}

// ConfigNormalizer lets plugins inject server-owned config values and encrypt
// secrets before the config is persisted.
type ConfigNormalizer interface {
	NormalizeConfig(businessID uint, config map[string]interface{}) (map[string]interface{}, error)
}

// PublicConfigProvider lets plugins mask sensitive config before dashboard
// responses return it to the browser.
type PublicConfigProvider interface {
	PublicConfig(config map[string]interface{}) map[string]interface{}
}

// AlternativePaymentTracker lets a plugin connect its provider intent row to
// the generic AlternativePayment row created by plugin_handlers.
type AlternativePaymentTracker interface {
	AttachAlternativePayment(businessID uint, paymentID string, alternativePaymentID uint) error
}

type PaymentStatusDetails struct {
	Status   string                 `json:"status"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// DetailedPaymentStatusProvider returns provider-specific metadata for
// non-redirect payment flows such as QR/address stablecoin checkout.
type DetailedPaymentStatusProvider interface {
	GetPaymentStatusDetails(businessID uint, paymentID string) (*PaymentStatusDetails, error)
}

// PaymentResponse represents the response from creating a payment
type PaymentResponse struct {
	PaymentID   string                 `json:"payment_id"`
	Status      string                 `json:"status"`
	PaymentURL  string                 `json:"payment_url,omitempty"`
	RedirectURL string                 `json:"redirect_url,omitempty"`
	QRCode      string                 `json:"qr_code,omitempty"`
	ExpiresAt   *int64                 `json:"expires_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// WebhookResponse represents the response from processing a webhook
type WebhookResponse struct {
	PaymentID     string                 `json:"payment_id"`
	BillID        uint                   `json:"bill_id"`
	Status        string                 `json:"status"`
	Amount        int64                  `json:"amount"`
	Currency      string                 `json:"currency"`
	TransactionID string                 `json:"transaction_id,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Success       bool                   `json:"success"`
	Message       string                 `json:"message,omitempty"`
	// RefundedAmountCents, when non-nil on a "refunded" event, is the amount the
	// PROVIDER actually refunded (int64 cents), declared explicitly by the plugin
	// so partial-vs-full is a property the plugin states rather than one the
	// handler infers from the per-provider meaning of Amount. nil preserves the
	// legacy Amount-heuristic behavior for providers not yet updated.
	RefundedAmountCents *int64 `json:"refunded_amount_cents,omitempty"`
	// RefundedCumulativeCents is the provider's CUMULATIVE refunded total for
	// the capture (Stripe amount_refunded, MercadoPago
	// transaction_amount_refunded). The handler reverses only the delta over
	// what the payment already absorbed, so replays are no-ops. It may ride on
	// a "refunded" or a "completed" (partially refunded capture) response.
	RefundedCumulativeCents *int64 `json:"refunded_cumulative_cents,omitempty"`
	// DisputedCents is the amount the provider currently holds back for a
	// dispute (0 once reinstated). It rides on "reversed" (funds withdrawn)
	// and "dispute_reinstated"; the handler moves the ledger by the change.
	DisputedCents *int64 `json:"disputed_cents,omitempty"`
}

// IntegrationPlugin represents a third-party integration plugin. The signatures
// mirror the concrete TelegramPlugin contract (a typed notification + a
// per-business connection-status map) so the interface is an enforced contract
// rather than aspirational documentation; see the compile-time assertion in the
// telegram package.
type IntegrationPlugin interface {
	Plugin

	// SendNotification sends a typed notification through this integration.
	SendNotification(businessID uint, notificationType string, message string, data map[string]interface{}) error

	// GetConnectionStatus reports the integration's per-business connection state.
	GetConnectionStatus(businessID uint) (map[string]interface{}, error)
}

// ReportingPlugin represents a reporting/analytics plugin
type ReportingPlugin interface {
	Plugin

	// GenerateReport generates a report using this plugin
	GenerateReport(businessID uint, reportType string, params map[string]interface{}) ([]byte, error)

	// GetReportTypes returns available report types
	GetReportTypes() []string
}

// MarketingPlugin represents a marketing/CRM plugin
type MarketingPlugin interface {
	Plugin

	// CreateCampaign creates a marketing campaign
	CreateCampaign(businessID uint, campaignData map[string]interface{}) (string, error)

	// GetCampaignStatus gets the status of a campaign
	GetCampaignStatus(businessID uint, campaignID string) (string, error)
}

// PluginRegistry manages all available plugins
type PluginRegistry struct {
	plugins map[string]Plugin
}

// NewPluginRegistry creates a new plugin registry
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins: make(map[string]Plugin),
	}
}

// RegisterPlugin registers a plugin in the registry
func (pr *PluginRegistry) RegisterPlugin(plugin Plugin) {
	pr.plugins[plugin.GetName()] = plugin
}

// UnregisterPlugin removes a plugin from the registry.
func (pr *PluginRegistry) UnregisterPlugin(name string) {
	delete(pr.plugins, name)
}

// GetPlugin gets a plugin by name
func (pr *PluginRegistry) GetPlugin(name string) (Plugin, bool) {
	plugin, exists := pr.plugins[name]
	return plugin, exists
}

// GetAllPlugins returns all registered plugins
func (pr *PluginRegistry) GetAllPlugins() map[string]Plugin {
	return pr.plugins
}

// ConvertToDBPlugin converts a Plugin interface to database.Plugin
func (pr *PluginRegistry) ConvertToDBPlugin(plugin Plugin) database.Plugin {
	return database.Plugin{
		Name:         plugin.GetName(),
		DisplayName:  plugin.GetDisplayName(),
		Description:  plugin.GetDescription(),
		Image:        "/images/plugins/" + plugin.GetName() + "-logo.png",
		Category:     plugin.GetCategory(),
		Version:      plugin.GetVersion(),
		Features:     plugin.GetFeatures(),
		ConfigSchema: plugin.GetConfigSchema(),
		IsActive:     plugin.IsActive(),
	}
}

// Global plugin registry instance
var GlobalRegistry = NewPluginRegistry()
