package paypal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	appconfig "github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/money"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/pluginurl"
	"github.com/stdevmac/payverge/backend/internal/services"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrPayPalAuthFailed indicates PayPal rejected the supplied client credentials
// (HTTP 401 from the OAuth token endpoint). It is distinguished from transient
// or network failures so credential validation can hard-reject bad keys while
// tolerating outages.
var ErrPayPalAuthFailed = errors.New("paypal authentication failed")

// PayPalPlugin implements the PaymentPlugin interface
type PayPalPlugin struct {
	pluginService *services.PluginService
}

// NewPayPalPlugin creates a new PayPal plugin instance
func NewPayPalPlugin(pluginService *services.PluginService) *PayPalPlugin {
	return &PayPalPlugin{
		pluginService: pluginService,
	}
}

// GetName returns the plugin name
func (pp *PayPalPlugin) GetName() string {
	return "paypal"
}

// GetDisplayName returns the user-facing name
func (pp *PayPalPlugin) GetDisplayName() string {
	return "PayPal"
}

// GetDescription returns the plugin description
func (pp *PayPalPlugin) GetDescription() string {
	return "Accept PayPal payments with buyer protection and global coverage"
}

// GetCategory returns the plugin category
func (pp *PayPalPlugin) GetCategory() string {
	return "payment"
}

// GetVersion returns the plugin version
func (pp *PayPalPlugin) GetVersion() string {
	return "1.0.0"
}

// GetFeatures returns the plugin features as JSON array
func (pp *PayPalPlugin) GetFeatures() string {
	return `["PayPal Payments", "Express Checkout", "Buyer Protection", "Multi-Currency", "Mobile Optimized"]`
}

// GetConfigSchema returns the JSON schema for configuration
func (pp *PayPalPlugin) GetConfigSchema() string {
	return `{
		"type": "object",
		"required": ["client_id", "client_secret", "webhook_id", "base_url"],
		"properties": {
			"client_id": {
				"type": "string",
				"title": "Client ID",
				"description": "Your PayPal application Client ID"
			},
			"client_secret": {
				"type": "string",
				"title": "Client Secret",
				"description": "Your PayPal application Client Secret",
				"format": "password"
			},
			"environment": {
				"type": "string",
				"title": "Environment",
				"description": "PayPal environment (sandbox or live)",
				"enum": ["sandbox", "live"],
				"default": "sandbox"
			},
			"webhook_id": {
				"type": "string",
				"title": "Webhook ID",
				"description": "PayPal webhook ID for payment notification verification"
			},
			"webhook_id_previous": {
				"type": "string",
				"title": "Previous Webhook ID",
				"description": "Temporary overlap ID used while rotating PayPal webhooks"
			},
			"base_url": {
				"type": "string",
				"title": "Public API Base URL",
				"description": "Public backend origin used for PayPal return and cancel callbacks",
				"format": "uri"
			},
			"brand_name": {
				"type": "string",
				"title": "Brand Name (Optional)",
				"description": "Your brand name displayed on PayPal checkout"
			},
			"return_url": {
				"type": "string",
				"title": "Return URL",
				"description": "URL to redirect after successful payment",
				"format": "uri"
			},
			"cancel_url": {
				"type": "string",
				"title": "Cancel URL",
				"description": "URL to redirect after cancelled payment",
				"format": "uri"
			}
		}
	}`
}

// IsActive returns whether the plugin is active on the platform
func (pp *PayPalPlugin) IsActive() bool {
	return plugins.PaymentProviderStartsEnabled(pp.GetName(), appconfig.IsProductionMode(false))
}

// ValidateConfig validates the plugin configuration
func (pp *PayPalPlugin) ValidateConfig(config map[string]interface{}) error {
	clientID, ok := config["client_id"].(string)
	if !ok || clientID == "" {
		return errors.New("client_id is required")
	}

	if !isValidPayPalClientID(clientID) {
		return errors.New("invalid PayPal client ID format")
	}

	clientSecret, ok := config["client_secret"].(string)
	if !ok || clientSecret == "" {
		return errors.New("client_secret is required")
	}

	if !isValidPayPalClientSecret(clientSecret) {
		return errors.New("invalid PayPal client secret format")
	}

	webhookID, ok := config["webhook_id"].(string)
	if !ok || strings.TrimSpace(webhookID) == "" {
		return errors.New("webhook_id is required")
	}
	if !isValidPayPalWebhookID(webhookID) {
		return errors.New("invalid PayPal webhook ID format")
	}

	baseURL, ok := config["base_url"].(string)
	if !ok || strings.TrimSpace(baseURL) == "" {
		return errors.New("base_url is required")
	}
	if _, err := url.ParseRequestURI(strings.TrimSpace(baseURL)); err != nil {
		return errors.New("base_url must be an absolute URL")
	}

	// api_base_url is an optional override for PayPal's API endpoint. Guard it
	// against SSRF: an operator must not be able to point outbound API calls at
	// an internal service or cloud metadata. Empty means "use the default".
	if apiBaseURL, ok := config["api_base_url"].(string); ok && strings.TrimSpace(apiBaseURL) != "" {
		if err := pluginurl.ValidateBaseURL(apiBaseURL, pluginurl.PayPalHosts); err != nil {
			return errors.New("api_base_url must be an https PayPal endpoint")
		}
	}

	// Validate environment
	if env, exists := config["environment"]; exists {
		envStr, ok := env.(string)
		if !ok || (envStr != "sandbox" && envStr != "live") {
			return errors.New("environment must be 'sandbox' or 'live'")
		}
	}

	return nil
}

// Initialize sets up the plugin with the given configuration
func (pp *PayPalPlugin) Initialize(businessID uint, config map[string]interface{}) error {
	// Validate configuration first
	if err := pp.ValidateConfig(config); err != nil {
		return fmt.Errorf("paypal plugin initialization failed: %v", err)
	}

	// Test PayPal API connection with provided credentials
	clientID := config["client_id"].(string)
	clientSecret := config["client_secret"].(string)
	environment, ok := config["environment"].(string)
	if !ok || strings.TrimSpace(environment) == "" {
		return fmt.Errorf("paypal plugin initialization failed: environment is required (sandbox or live)")
	}

	if err := pp.testPayPalConnection(clientID, clientSecret, environment); err != nil {
		return fmt.Errorf("paypal API connection test failed: %v", err)
	}

	return nil
}

// Cleanup performs any necessary cleanup when plugin is disabled
func (pp *PayPalPlugin) Cleanup(businessID uint) error {
	log.Printf("Plugin cleanup called for business %d — webhook endpoints not automatically removed from PayPal", businessID)
	return nil
}

// ProcessPayment processes a payment using PayPal
func (pp *PayPalPlugin) ProcessPayment(businessID uint, amount int64, currency string, metadata map[string]interface{}) (string, error) {
	return "", errors.New("generic PayPal ProcessPayment is unsupported; use CreateBillPayment orders")
}

// RefundPayment refunds a captured PayPal payment. paymentID must be the
// capture id produced by PayPal's Orders capture response. amount is cents;
// amount == 0 requests a full remaining refund from PayPal.
func (pp *PayPalPlugin) RefundPayment(businessID uint, paymentID string, amount int64) error {
	captureID := strings.TrimSpace(paymentID)
	if captureID == "" {
		return errors.New("paypal capture ID is required")
	}
	if amount < 0 {
		return errors.New("paypal refund amount cannot be negative")
	}

	config, err := pp.getConfig(businessID)
	if err != nil {
		return fmt.Errorf("failed to get PayPal config: %w", err)
	}

	refundRequest := map[string]interface{}{}
	if amount > 0 {
		currency, err := pp.currencyForPayPalCapture(captureID, config)
		if err != nil {
			return err
		}
		refundRequest["amount"] = map[string]interface{}{
			// PayPal requires the value to use the currency's decimal places
			// ("1000" for JPY, "10.00" for USD); a hardcoded %.2f is rejected
			// for zero-decimal currencies.
			"value":         money.MajorUnitString(amount, currency),
			"currency_code": currency,
		}
	}

	requestID := plugins.PaymentRequestIdempotencyKey("paypal", "refund", businessID, captureID, amount)
	response, err := pp.makePayPalAPICallWithRequestID(context.Background(), http.MethodPost, fmt.Sprintf("/v2/payments/captures/%s/refund", url.PathEscape(captureID)), refundRequest, config, requestID)
	if err != nil {
		return fmt.Errorf("failed to create PayPal refund: %w", err)
	}
	refundID := strings.TrimSpace(stringFromPayPalAny(response["id"]))
	if refundID == "" {
		return errors.New("invalid PayPal refund response: missing refund ID")
	}
	// PayPal refunds can settle asynchronously. Per PayPal semantics a refund is
	// COMPLETED, PENDING, CANCELLED, or FAILED — only COMPLETED/PENDING mean the
	// money is (or will be) returned. A CANCELLED/FAILED refund must NOT be
	// recorded as done, or the platform reverses money PayPal never returned. An
	// absent status is tolerated for forward-compat with minimal responses (a
	// present refund id already proves the call itself succeeded).
	if status := strings.ToUpper(strings.TrimSpace(stringFromPayPalAny(response["status"]))); status != "" && status != "COMPLETED" && status != "PENDING" {
		return fmt.Errorf("paypal refund not successful: status %s", status)
	}
	return nil
}

// GetPaymentStatus gets the status of a payment from PayPal
func (pp *PayPalPlugin) GetPaymentStatus(businessID uint, paymentID string) (string, error) {
	// Get PayPal configuration
	config, err := pp.pluginService.GetPluginConfig(businessID, "paypal")
	if err != nil {
		return "", fmt.Errorf("failed to get paypal config: %v", err)
	}

	clientID, ok := config["client_id"].(string)
	if !ok {
		return "", errors.New("paypal client ID not configured")
	}

	clientSecret, ok := config["client_secret"].(string)
	if !ok {
		return "", errors.New("paypal client secret not configured")
	}

	environment, ok := config["environment"].(string)
	if !ok || strings.TrimSpace(environment) == "" {
		return "", errors.New("PayPal environment not configured")
	}

	// Get payment status from PayPal API
	return pp.getPayPalOrderStatus(clientID, clientSecret, environment, paymentID)
}

// Helper functions for PayPal API integration

func isValidPayPalClientID(clientID string) bool {
	// PayPal Client IDs are typically long alphanumeric strings
	return len(clientID) > 20 && len(clientID) < 200
}

func isValidPayPalClientSecret(clientSecret string) bool {
	// PayPal Client Secrets are typically long alphanumeric strings
	return len(clientSecret) > 20 && len(clientSecret) < 200
}

func isValidPayPalWebhookID(webhookID string) bool {
	webhookID = strings.TrimSpace(webhookID)
	if len(webhookID) == 0 || len(webhookID) > 50 {
		return false
	}
	for _, r := range webhookID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func (pp *PayPalPlugin) testPayPalConnection(clientID, clientSecret, environment string) error {
	return pp.validatePayPalCredentials(&PayPalConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Environment:  environment,
	})
}

// validatePayPalCredentials format-checks the credentials and performs a live
// token request against PayPal. Definitively-invalid credentials (HTTP 401) are
// rejected so a misconfigured key cannot silently enable the plugin and fail on
// the first real payment; transient/network failures are tolerated (logged, then
// allowed) so a PayPal outage does not block enabling. This mirrors the Stripe
// plugin's testStripeConnection, which rejects 401 but tolerates other failures.
func (pp *PayPalPlugin) validatePayPalCredentials(config *PayPalConfig) error {
	if !isValidPayPalClientID(config.ClientID) {
		return errors.New("invalid client ID format")
	}
	if !isValidPayPalClientSecret(config.ClientSecret) {
		return errors.New("invalid client secret format")
	}
	if config.Environment != "sandbox" && config.Environment != "live" {
		return errors.New("invalid environment, must be 'sandbox' or 'live'")
	}

	if _, err := pp.getPayPalAccessToken(context.Background(), config); err != nil {
		if errors.Is(err, ErrPayPalAuthFailed) {
			return fmt.Errorf("paypal credentials are invalid — authentication failed")
		}
		log.Printf("WARNING: PayPal connection test failed (transient): %v — plugin enabled with unverified credentials", err)
	}
	return nil
}

func (pp *PayPalPlugin) getPayPalOrderStatus(clientID, clientSecret, environment, orderID string) (string, error) {
	if orderID == "" {
		return "", errors.New("order ID is required")
	}

	config := &PayPalConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Environment:  environment,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response, err := pp.makePayPalAPICall(ctx, http.MethodGet, fmt.Sprintf("/v2/checkout/orders/%s", url.PathEscape(orderID)), nil, config)
	if err != nil {
		return "", err
	}
	status, _ := response["status"].(string)
	if strings.TrimSpace(status) == "" {
		return "", errors.New("PayPal order response missing status")
	}
	return status, nil
}

// Helper method to get PayPal configuration for a business
func (pp *PayPalPlugin) getConfig(businessID uint) (*PayPalConfig, error) {
	if pp == nil || pp.pluginService == nil {
		return nil, errors.New("paypal plugin service not configured")
	}

	config, err := pp.pluginService.GetPluginConfig(businessID, "paypal")
	if err != nil {
		return nil, fmt.Errorf("failed to get PayPal config: %w", err)
	}

	clientID, ok := config["client_id"].(string)
	if !ok || clientID == "" {
		return nil, errors.New("PayPal client_id not configured")
	}

	clientSecret, ok := config["client_secret"].(string)
	if !ok || clientSecret == "" {
		return nil, errors.New("PayPal client_secret not configured")
	}

	environment, ok := config["environment"].(string)
	if !ok || strings.TrimSpace(environment) == "" {
		return nil, errors.New("PayPal environment is required (sandbox or live)")
	}

	baseURL, _ := config["base_url"].(string)
	apiBaseURL, _ := config["api_base_url"].(string)
	webhookID, _ := config["webhook_id"].(string)

	return &PayPalConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Environment:  environment,
		BaseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIBaseURL:   strings.TrimRight(strings.TrimSpace(apiBaseURL), "/"),
		WebhookID:    strings.TrimSpace(webhookID),
	}, nil
}

func (pp *PayPalPlugin) currencyForPayPalCapture(captureID string, config *PayPalConfig) (string, error) {
	response, err := pp.makePayPalAPICall(context.Background(), http.MethodGet, fmt.Sprintf("/v2/payments/captures/%s", url.PathEscape(captureID)), nil, config)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve PayPal capture: %w", err)
	}
	amountMap, _ := response["amount"].(map[string]interface{})
	currency := strings.ToUpper(strings.TrimSpace(pluginFirstNonEmptyPayPal(
		stringFromPayPalAny(amountMap["currency_code"]),
		stringFromPayPalAny(amountMap["currency"]),
	)))
	if currency == "" {
		return "", errors.New("PayPal capture response missing currency")
	}
	return currency, nil
}

// PayPalConfig represents PayPal configuration
type PayPalConfig struct {
	ClientID     string
	ClientSecret string
	Environment  string
	BaseURL      string
	APIBaseURL   string
	WebhookID    string
}

func payPalConfigFromMap(config map[string]interface{}) (*PayPalConfig, error) {
	clientID, ok := config["client_id"].(string)
	if !ok || strings.TrimSpace(clientID) == "" {
		return nil, errors.New("PayPal client_id not configured")
	}
	clientSecret, ok := config["client_secret"].(string)
	if !ok || strings.TrimSpace(clientSecret) == "" {
		return nil, errors.New("PayPal client_secret not configured")
	}
	environment, _ := config["environment"].(string)
	if strings.TrimSpace(environment) == "" {
		return nil, errors.New("PayPal environment is required (sandbox or live)")
	}
	webhookID, _ := config["webhook_id"].(string)
	apiBaseURL, _ := config["api_base_url"].(string)
	baseURL, _ := config["base_url"].(string)
	return &PayPalConfig{
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
		Environment:  strings.TrimSpace(environment),
		BaseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIBaseURL:   strings.TrimRight(strings.TrimSpace(apiBaseURL), "/"),
		WebhookID:    strings.TrimSpace(webhookID),
	}, nil
}

func paypalAPIBaseURL(config *PayPalConfig) string {
	if config != nil && strings.TrimSpace(config.APIBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(config.APIBaseURL), "/")
	}
	if config != nil && config.Environment == "live" {
		return "https://api.paypal.com"
	}
	return "https://api.sandbox.paypal.com"
}

// callbackBaseURL resolves the public backend origin used to build PayPal
// return/cancel callbacks. It is a package var so tests can override it
// without mutating process env. Defaults to the server runtime origin
// (APP_BASE_URL, else PUBLIC_URL); the per-business base_url is never used.
var callbackBaseURL = appconfig.APIBaseURL

// paypalBrandName is the merchant label PayPal shows the payer at checkout:
// this instance's PRODUCT_NAME (at most 80 characters, inside PayPal's
// 127-character brand_name limit).
func paypalBrandName() string {
	return appconfig.ProductName()
}

// paypalCallbackBaseURL is always the deployment's own origin (APP_BASE_URL,
// else PUBLIC_URL, else the local development URL). The per-business base_url
// is deliberately ignored: a tenant must not be able to send its guests'
// PayPal returns to an arbitrary host, and a staging copy must never point at
// production.
func paypalCallbackBaseURL(_ *PayPalConfig) string {
	return strings.TrimRight(strings.TrimSpace(callbackBaseURL()), "/")
}

// Helper method to make PayPal API calls
func (pp *PayPalPlugin) makePayPalAPICall(ctx context.Context, method, endpoint string, payload interface{}, config *PayPalConfig) (map[string]interface{}, error) {
	return pp.makePayPalAPICallWithRequestID(ctx, method, endpoint, payload, config, "")
}

func (pp *PayPalPlugin) makePayPalAPICallWithRequestID(ctx context.Context, method, endpoint string, payload interface{}, config *PayPalConfig, requestID string) (map[string]interface{}, error) {
	// Get PayPal access token
	accessToken, err := pp.getPayPalAccessToken(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to get PayPal access token: %w", err)
	}

	baseURL := paypalAPIBaseURL(config)

	// Prepare request
	var reqBody io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, method, baseURL+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	if requestID != "" {
		req.Header.Set("PayPal-Request-Id", requestID)
	}

	// Make request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("PayPal API error %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// paypalCachedToken is a per-credential OAuth token with its computed expiry.
type paypalCachedToken struct {
	token     string
	expiresAt time.Time
}

// paypalTokenRefreshMargin refreshes a cached token this long before its true
// expiry so an in-flight request never races the expiry boundary.
const paypalTokenRefreshMargin = 60 * time.Second

var (
	paypalTokenCacheMu sync.Mutex
	paypalTokenCache   = map[string]paypalCachedToken{}
)

// paypalTokenCacheKey scopes a cached token to the exact credentials and API
// origin it was minted for, so rotated credentials or a different environment
// never reuse a stale token. The secret is only ever held in-process.
func paypalTokenCacheKey(config *PayPalConfig) string {
	if config == nil {
		return ""
	}
	return strings.Join([]string{config.ClientID, config.ClientSecret, paypalAPIBaseURL(config)}, "\x00")
}

func lookupPayPalToken(key string) string {
	if key == "" {
		return ""
	}
	paypalTokenCacheMu.Lock()
	defer paypalTokenCacheMu.Unlock()
	entry, ok := paypalTokenCache[key]
	if !ok {
		return ""
	}
	if time.Now().After(entry.expiresAt.Add(-paypalTokenRefreshMargin)) {
		delete(paypalTokenCache, key)
		return ""
	}
	return entry.token
}

func storePayPalToken(key, token string, expiresInSeconds int64) {
	if key == "" || token == "" || expiresInSeconds <= 0 {
		return
	}
	paypalTokenCacheMu.Lock()
	defer paypalTokenCacheMu.Unlock()
	paypalTokenCache[key] = paypalCachedToken{
		token:     token,
		expiresAt: time.Now().Add(time.Duration(expiresInSeconds) * time.Second),
	}
}

// resetPayPalTokenCache clears every cached token. Used by tests so cached
// tokens never leak between cases.
func resetPayPalTokenCache() {
	paypalTokenCacheMu.Lock()
	defer paypalTokenCacheMu.Unlock()
	paypalTokenCache = map[string]paypalCachedToken{}
}

func paypalTokenExpiresIn(tokenResp map[string]interface{}) int64 {
	switch v := tokenResp["expires_in"].(type) {
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}

// Helper method to get PayPal access token
func (pp *PayPalPlugin) getPayPalAccessToken(ctx context.Context, config *PayPalConfig) (string, error) {
	cacheKey := paypalTokenCacheKey(config)
	if token := lookupPayPalToken(cacheKey); token != "" {
		return token, nil
	}

	baseURL := paypalAPIBaseURL(config)

	// Prepare request
	data := "grant_type=client_credentials"
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/oauth2/token", strings.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(config.ClientID, config.ClientSecret)

	// Make request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	// Check status code. 401 means the client credentials themselves were
	// rejected — surface a distinct sentinel so credential validation can
	// hard-fail bad keys while tolerating transient errors (5xx, rate limits).
	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("%w (HTTP %d): %s", ErrPayPalAuthFailed, resp.StatusCode, string(body))
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("PayPal token error %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp map[string]interface{}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	accessToken, ok := tokenResp["access_token"].(string)
	if !ok {
		return "", fmt.Errorf("invalid token response: missing access_token")
	}

	// Cache only when PayPal reports a lifetime, refreshing before it lapses.
	// Responses without expires_in (test doubles, minimal stubs) are not cached.
	storePayPalToken(cacheKey, accessToken, paypalTokenExpiresIn(tokenResp))

	return accessToken, nil
}

// init registers the PayPal plugin with the global registry
func init() {
	plugins.RegisterPluginInitializer("paypal", func(pluginService *services.PluginService) {
		paypalPlugin := NewPayPalPlugin(pluginService)
		plugins.GlobalRegistry.RegisterPlugin(paypalPlugin)
	})
}

// CreateBillPayment creates a PayPal payment for a specific bill using Orders API v2
func (pp *PayPalPlugin) CreateBillPayment(businessID uint, billID uint, amount int64, currency string, metadata map[string]interface{}) (*plugins.PaymentResponse, error) {
	config, err := pp.getConfig(businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to get PayPal config: %w", err)
	}

	// Render the amount in the currency's major units with its expected
	// decimal places ("1000" for JPY, "10.00" for USD); a hardcoded %.2f is
	// rejected by PayPal for zero-decimal currencies.
	amountStr := money.MajorUnitString(amount, currency)

	callbackBase := paypalCallbackBaseURL(config)
	returnURL, err := buildPayPalCallbackURL(callbackBase, "/api/v1/webhooks/paypal/return", billID)
	if err != nil {
		return nil, err
	}
	cancelURL, err := buildPayPalCallbackURL(callbackBase, "/api/v1/webhooks/paypal/cancel", billID)
	if err != nil {
		return nil, err
	}

	// Create PayPal order request using Orders API v2
	orderRequest := map[string]interface{}{
		"intent": "CAPTURE",
		"purchase_units": []map[string]interface{}{
			{
				"reference_id": fmt.Sprintf("bill_%d", billID),
				"description":  fmt.Sprintf("Payment for Bill #%d", billID),
				"amount": map[string]interface{}{
					"currency_code": currency,
					"value":         amountStr,
				},
				"custom_id": fmt.Sprintf("bill_%d_business_%d", billID, businessID),
			},
		},
		"application_context": map[string]interface{}{
			"return_url":   returnURL,
			"cancel_url":   cancelURL,
			"brand_name":   paypalBrandName(),
			"landing_page": "BILLING",
			"user_action":  "PAY_NOW",
		},
	}

	// Make API call to PayPal Orders API v2
	requestID := plugins.PaymentRequestIdempotencyKey("paypal", "create_bill_payment", businessID, billID, amount, strings.ToUpper(currency))
	response, err := pp.makePayPalAPICallWithRequestID(context.Background(), "POST", "/v2/checkout/orders", orderRequest, config, requestID)
	if err != nil {
		return nil, fmt.Errorf("failed to create PayPal order: %w", err)
	}

	// Extract order ID and approval URL
	orderID, ok := response["id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid PayPal response: missing order ID")
	}

	var approvalURL string
	if links, ok := response["links"].([]interface{}); ok {
		for _, link := range links {
			if linkMap, ok := link.(map[string]interface{}); ok {
				if rel, ok := linkMap["rel"].(string); ok && rel == "approve" {
					if href, ok := linkMap["href"].(string); ok {
						approvalURL = href
						break
					}
				}
			}
		}
	}

	return &plugins.PaymentResponse{
		PaymentID:   orderID,
		Status:      "created",
		PaymentURL:  approvalURL,
		RedirectURL: approvalURL,
		Metadata: map[string]interface{}{
			"bill_id":     billID,
			"business_id": businessID,
			"order_id":    orderID,
		},
	}, nil
}

// HandleWebhook processes PayPal webhook callbacks
func (pp *PayPalPlugin) HandleWebhook(businessID uint, payload []byte, headers map[string]string) (*plugins.WebhookResponse, error) {
	// Parse webhook payload
	var webhookData map[string]interface{}
	if err := json.Unmarshal(payload, &webhookData); err != nil {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook payload",
		}, nil
	}

	// Extract event type
	eventType, ok := webhookData["event_type"].(string)
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing event type",
		}, nil
	}

	switch eventType {
	case "CHECKOUT.ORDER.APPROVED":
		config, err := pp.getConfig(businessID)
		if err != nil {
			return &plugins.WebhookResponse{
				Success: false,
				Message: "PayPal configuration unavailable",
			}, err
		}
		return pp.handleOrderApproved(webhookData, config)
	case "PAYMENT.CAPTURE.COMPLETED", "PAYMENT.SALE.COMPLETED":
		return pp.handlePaymentCompleted(businessID, webhookData)
	case "PAYMENT.CAPTURE.DENIED", "PAYMENT.CAPTURE.DECLINED", "PAYMENT.SALE.DENIED":
		return pp.handlePaymentFailed(businessID, webhookData)
	case "PAYMENT.CAPTURE.REFUNDED":
		return pp.handlePaymentRefunded(businessID, webhookData)
	case "PAYMENT.CAPTURE.REVERSED":
		return pp.handlePaymentLifecycle(webhookData, "reversed", "Payment reversed")
	case "CUSTOMER.DISPUTE.CREATED":
		return pp.handlePayPalDispute(webhookData)
	case "CHECKOUT.ORDER.VOIDED":
		return pp.handlePaymentLifecycle(webhookData, "expired", "PayPal order expired")
	}

	return &plugins.WebhookResponse{
		Success: true,
		Message: "Event processed",
	}, nil
}

// GetWebhookEndpoint returns the webhook endpoint path for PayPal
func (pp *PayPalPlugin) GetWebhookEndpoint() string {
	return "/api/v1/webhooks/paypal"
}

// VerifyWebhookSignature is retained for the generic PaymentPlugin interface.
// PayPal's official verification requires multiple transmission headers and an
// API call to /v1/notifications/verify-webhook-signature, so callers should use
// VerifyWebhookRequest for PayPal webhooks.
func (pp *PayPalPlugin) VerifyWebhookSignature(payload []byte, signature string, secret string) bool {
	log.Printf("WARNING: PayPal VerifyWebhookSignature called directly — use VerifyWebhookRequest instead for proper PayPal verification")
	return false
}

func (pp *PayPalPlugin) VerifyWebhookRequest(request plugins.WebhookVerificationRequest) error {
	config, err := payPalConfigFromMap(request.Config)
	if err != nil {
		return err
	}
	if strings.TrimSpace(config.WebhookID) == "" {
		return errors.New("PayPal webhook_id not configured")
	}

	var webhookEvent map[string]interface{}
	if err := json.Unmarshal(request.Payload, &webhookEvent); err != nil {
		return fmt.Errorf("invalid PayPal webhook payload: %w", err)
	}

	transmissionID := paypalHeader(request.Headers, "PayPal-Transmission-Id")
	transmissionTime := paypalHeader(request.Headers, "PayPal-Transmission-Time")
	certURL := paypalHeader(request.Headers, "PayPal-Cert-Url")
	authAlgo := paypalHeader(request.Headers, "PayPal-Auth-Algo")
	transmissionSig := paypalHeader(request.Headers, "PayPal-Transmission-Sig")
	if transmissionID == "" || transmissionTime == "" || certURL == "" || authAlgo == "" || transmissionSig == "" {
		return errors.New("missing PayPal webhook verification headers")
	}

	webhookIDs := []string{config.WebhookID, strings.TrimSpace(stringFromPayPalAny(request.Config["webhook_id_previous"]))}
	lastStatus := ""
	var lastErr error
	for _, webhookID := range webhookIDs {
		if webhookID == "" {
			continue
		}
		verifyRequest := map[string]interface{}{
			"auth_algo":         authAlgo,
			"cert_url":          certURL,
			"transmission_id":   transmissionID,
			"transmission_sig":  transmissionSig,
			"transmission_time": transmissionTime,
			"webhook_id":        webhookID,
			"webhook_event":     webhookEvent,
		}
		verifyResponse, verifyErr := pp.makePayPalAPICall(context.Background(), http.MethodPost, "/v1/notifications/verify-webhook-signature", verifyRequest, config)
		if verifyErr != nil {
			lastErr = verifyErr
			continue
		}
		lastStatus, _ = verifyResponse["verification_status"].(string)
		if strings.ToUpper(strings.TrimSpace(lastStatus)) == "SUCCESS" {
			return nil
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("PayPal webhook verification failed: %s", lastStatus)
}

// Helper method to handle payment completed webhook
func (pp *PayPalPlugin) handlePaymentCompleted(businessID uint, webhookData map[string]interface{}) (*plugins.WebhookResponse, error) {
	resource, ok := webhookData["resource"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook resource",
		}, nil
	}

	return paypalCaptureWebhookResponse(resource, "completed")
}

func (pp *PayPalPlugin) handleOrderApproved(webhookData map[string]interface{}, config *PayPalConfig) (*plugins.WebhookResponse, error) {
	resource, ok := webhookData["resource"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook resource",
		}, nil
	}

	orderID := strings.TrimSpace(stringFromPayPalAny(resource["id"]))
	if orderID == "" {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing PayPal order ID",
		}, nil
	}

	captureResponse, err := pp.capturePayPalOrder(orderID, config)
	if err != nil {
		return nil, err
	}
	return paypalOrderCaptureResponse(captureResponse)
}

func (pp *PayPalPlugin) CapturePaymentReturn(businessID uint, billID uint, paymentID string) (*plugins.WebhookResponse, error) {
	orderID := strings.TrimSpace(paymentID)
	if orderID == "" {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing PayPal order ID",
		}, nil
	}
	config, err := pp.getConfig(businessID)
	if err != nil {
		return nil, err
	}
	captureResponse, err := pp.capturePayPalOrder(orderID, config)
	if err != nil {
		return nil, err
	}
	response, err := paypalOrderCaptureResponse(captureResponse)
	if err != nil || response == nil {
		return response, err
	}
	if response.BillID == 0 {
		response.BillID = billID
	}
	if response.BillID != billID {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "PayPal capture bill mismatch",
		}, nil
	}
	return response, nil
}

// Helper method to handle payment failed webhook
func (pp *PayPalPlugin) handlePaymentFailed(businessID uint, webhookData map[string]interface{}) (*plugins.WebhookResponse, error) {
	resource, ok := webhookData["resource"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook resource",
		}, nil
	}

	paymentID := pluginFirstNonEmptyPayPal(
		stringFromPayPalAny(resource["id"]),
		stringFromPayPalAny(resource["parent_payment"]),
	)
	billID, _ := payPalBillBusinessFromResource(resource)

	return &plugins.WebhookResponse{
		PaymentID: paymentID,
		BillID:    billID,
		Status:    "failed",
		Success:   true,
		Message:   "Payment failed",
	}, nil
}

func (pp *PayPalPlugin) handlePaymentRefunded(businessID uint, webhookData map[string]interface{}) (*plugins.WebhookResponse, error) {
	resource, ok := webhookData["resource"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook resource",
		}, nil
	}
	response, err := paypalCaptureWebhookResponse(resource, "refunded")
	if err != nil {
		return response, err
	}
	refundID := response.PaymentID
	captureID := paypalRefundCaptureID(resource)
	if captureID == "" {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing PayPal capture ID for refund",
		}, nil
	}
	response.PaymentID = captureID
	response.TransactionID = refundID
	response.Message = "Payment refunded"
	return response, nil
}

func (pp *PayPalPlugin) handlePaymentLifecycle(webhookData map[string]interface{}, status, message string) (*plugins.WebhookResponse, error) {
	resource, ok := webhookData["resource"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{Success: false, Message: "Invalid webhook resource"}, nil
	}
	paymentID := pluginFirstNonEmptyPayPal(
		stringFromPayPalAny(resource["capture_id"]),
		stringFromPayPalAny(resource["id"]),
		stringFromPayPalAny(resource["parent_payment"]),
	)
	if paymentID == "" {
		return &plugins.WebhookResponse{Success: false, Message: "Missing PayPal payment ID"}, nil
	}
	billID, _ := payPalBillBusinessFromResource(resource)
	return &plugins.WebhookResponse{
		PaymentID: paymentID,
		BillID:    billID,
		Status:    status,
		Success:   true,
		Message:   message,
	}, nil
}

func (pp *PayPalPlugin) handlePayPalDispute(webhookData map[string]interface{}) (*plugins.WebhookResponse, error) {
	resource, ok := webhookData["resource"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{Success: false, Message: "Invalid webhook resource"}, nil
	}
	captureID := payPalDisputeCaptureID(resource)
	if captureID == "" {
		return &plugins.WebhookResponse{Success: false, Message: "Missing disputed PayPal capture ID"}, nil
	}
	billID, _ := payPalBillBusinessFromResource(resource)
	return &plugins.WebhookResponse{
		PaymentID:     captureID,
		BillID:        billID,
		Status:        "disputed",
		TransactionID: strings.TrimSpace(stringFromPayPalAny(resource["id"])),
		Success:       true,
		Message:       "PayPal dispute opened",
	}, nil
}

func payPalDisputeCaptureID(resource map[string]interface{}) string {
	if transactions, ok := resource["disputed_transactions"].([]interface{}); ok {
		for _, value := range transactions {
			transaction, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			if captureID := strings.TrimSpace(stringFromPayPalAny(transaction["seller_transaction_id"])); captureID != "" {
				return captureID
			}
		}
	}
	if supplementary, ok := resource["supplementary_data"].(map[string]interface{}); ok {
		if related, ok := supplementary["related_ids"].(map[string]interface{}); ok {
			return strings.TrimSpace(stringFromPayPalAny(related["capture_id"]))
		}
	}
	return ""
}

func (pp *PayPalPlugin) capturePayPalOrder(orderID string, config *PayPalConfig) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/v2/checkout/orders/%s/capture", url.PathEscape(orderID))
	requestID := plugins.PaymentRequestIdempotencyKey("paypal", "capture_order", orderID)
	response, err := pp.makePayPalAPICallWithRequestID(context.Background(), http.MethodPost, endpoint, map[string]interface{}{}, config, requestID)
	if err != nil {
		// A CHECKOUT.ORDER.APPROVED webhook can arrive for an order that was
		// already captured — via the synchronous return-capture path, or a
		// webhook redelivery. PayPal answers the duplicate capture with 422
		// ORDER_ALREADY_CAPTURED. Treat that as a no-op success by fetching the
		// already-completed order (same purchase_units/payments/captures shape),
		// so the webhook acks instead of 500-looping forever and flipping the
		// plugin health badge to a false "Webhook error".
		if isPayPalOrderAlreadyCaptured(err) {
			if existing, getErr := pp.getPayPalOrderDetails(orderID, config); getErr == nil {
				return existing, nil
			}
		}
		return nil, err
	}
	return response, nil
}

// getPayPalOrderDetails fetches the full order (including a completed order's
// captures) so an already-captured order can be resolved without re-capturing.
func (pp *PayPalPlugin) getPayPalOrderDetails(orderID string, config *PayPalConfig) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/v2/checkout/orders/%s", url.PathEscape(orderID))
	return pp.makePayPalAPICall(context.Background(), http.MethodGet, endpoint, nil, config)
}

// isPayPalOrderAlreadyCaptured reports whether a capture error is PayPal's
// 422 ORDER_ALREADY_CAPTURED, the idempotent "this order is already captured"
// signal (makePayPalAPICall surfaces the raw body in the error string).
func isPayPalOrderAlreadyCaptured(err error) bool {
	return err != nil && strings.Contains(err.Error(), "ORDER_ALREADY_CAPTURED")
}

func buildPayPalCallbackURL(baseURL, callbackPath string, billID uint) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", errors.New("PayPal base_url is required for callback URLs")
	}
	parsedBase, err := url.Parse(baseURL)
	if err != nil || parsedBase.Scheme == "" || parsedBase.Host == "" {
		return "", errors.New("PayPal base_url must be an absolute URL")
	}
	if !strings.HasPrefix(callbackPath, "/") {
		callbackPath = "/" + callbackPath
	}
	callbackURL := baseURL + callbackPath
	parsed, err := url.Parse(callbackURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("bill_id", strconv.FormatUint(uint64(billID), 10))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func paypalOrderCaptureResponse(captureResponse map[string]interface{}) (*plugins.WebhookResponse, error) {
	purchaseUnits, _ := captureResponse["purchase_units"].([]interface{})
	for _, unitValue := range purchaseUnits {
		unit, ok := unitValue.(map[string]interface{})
		if !ok {
			continue
		}
		payments, _ := unit["payments"].(map[string]interface{})
		captures, _ := payments["captures"].([]interface{})
		for _, captureValue := range captures {
			capture, ok := captureValue.(map[string]interface{})
			if !ok {
				continue
			}
			if _, exists := capture["custom_id"]; !exists {
				if customID := stringFromPayPalAny(unit["custom_id"]); customID != "" {
					capture["custom_id"] = customID
				}
			}
			if _, exists := capture["invoice_id"]; !exists {
				if invoiceID := stringFromPayPalAny(unit["invoice_id"]); invoiceID != "" {
					capture["invoice_id"] = invoiceID
				}
			}
			return paypalCaptureWebhookResponse(capture, "completed")
		}
	}
	return &plugins.WebhookResponse{
		Success: false,
		Message: "PayPal capture response missing capture",
	}, nil
}

func paypalCaptureWebhookResponse(resource map[string]interface{}, status string) (*plugins.WebhookResponse, error) {
	paymentID := pluginFirstNonEmptyPayPal(
		stringFromPayPalAny(resource["id"]),
		stringFromPayPalAny(resource["parent_payment"]),
	)
	amountMap, _ := resource["amount"].(map[string]interface{})
	amountText := pluginFirstNonEmptyPayPal(
		stringFromPayPalAny(amountMap["value"]),
		stringFromPayPalAny(amountMap["total"]),
	)
	currency := strings.ToUpper(pluginFirstNonEmptyPayPal(
		stringFromPayPalAny(amountMap["currency_code"]),
		stringFromPayPalAny(amountMap["currency"]),
	))
	amountCents, err := paypalAmountStringToCents(amountText)
	if err != nil {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid PayPal capture amount",
		}, nil
	}
	billID, _ := payPalBillBusinessFromResource(resource)
	if paymentID == "" {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing PayPal capture ID",
		}, nil
	}
	resp := &plugins.WebhookResponse{
		PaymentID:     paymentID,
		BillID:        billID,
		Status:        status,
		Amount:        amountCents,
		Currency:      currency,
		TransactionID: paymentID,
		Success:       true,
		Message:       "Payment completed successfully",
	}
	orderID := paypalOrderIDFromResource(resource)
	if orderID == "" {
		orderID = strings.TrimSpace(stringFromPayPalAny(resource["parent_payment"]))
	}
	if orderID != "" && orderID != paymentID {
		resp.Metadata = map[string]interface{}{"provider_tracker_id": orderID}
	}
	// For refund events PayPal sets Amount to the refunded portion (not the
	// original capture), so declare it explicitly so the handler does not have
	// to infer partial-vs-full from Amount's per-provider semantics.
	if status == "refunded" && amountCents > 0 {
		resp.RefundedAmountCents = &amountCents
	}
	return resp, nil
}

func payPalBillBusinessFromResource(resource map[string]interface{}) (uint, uint) {
	candidates := []string{
		stringFromPayPalAny(resource["custom_id"]),
		stringFromPayPalAny(resource["custom"]),
		stringFromPayPalAny(resource["invoice_id"]),
		stringFromPayPalAny(resource["reference_id"]),
	}
	if units, ok := resource["purchase_units"].([]interface{}); ok {
		for _, unitValue := range units {
			unit, ok := unitValue.(map[string]interface{})
			if !ok {
				continue
			}
			candidates = append(candidates,
				stringFromPayPalAny(unit["custom_id"]),
				stringFromPayPalAny(unit["invoice_id"]),
				stringFromPayPalAny(unit["reference_id"]),
			)
		}
	}
	// A dispute resource carries no custom_id of its own: the purchase unit's
	// custom_id is echoed as "custom" on each disputed transaction.
	if transactions, ok := resource["disputed_transactions"].([]interface{}); ok {
		for _, value := range transactions {
			transaction, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			candidates = append(candidates,
				stringFromPayPalAny(transaction["custom"]),
				stringFromPayPalAny(transaction["invoice_number"]),
			)
		}
	}
	for _, candidate := range candidates {
		if billID, businessID, ok := parsePayPalBillBusinessReference(candidate); ok {
			return billID, businessID
		}
	}
	return 0, 0
}

// paypalOrderIDFromResource extracts the originating checkout order id from a
// capture resource. PayPal confirms captures under a different id than the
// order the tracker row was keyed with; the handler uses this to rekey.
func paypalOrderIDFromResource(resource map[string]interface{}) string {
	supplementary, _ := resource["supplementary_data"].(map[string]interface{})
	if supplementary == nil {
		return ""
	}
	related, _ := supplementary["related_ids"].(map[string]interface{})
	if related == nil {
		return ""
	}
	return strings.TrimSpace(stringFromPayPalAny(related["order_id"]))
}

func paypalRefundCaptureID(resource map[string]interface{}) string {
	candidates := []string{
		stringFromPayPalAny(resource["capture_id"]),
		stringFromPayPalAny(resource["parent_payment"]),
		stringFromPayPalAny(resource["sale_id"]),
	}
	if supplementaryData, ok := resource["supplementary_data"].(map[string]interface{}); ok {
		if relatedIDs, ok := supplementaryData["related_ids"].(map[string]interface{}); ok {
			candidates = append(candidates, stringFromPayPalAny(relatedIDs["capture_id"]))
		}
	}
	return pluginFirstNonEmptyPayPal(candidates...)
}

func parsePayPalBillBusinessReference(reference string) (uint, uint, bool) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return 0, 0, false
	}
	parts := strings.Split(reference, "_")
	if len(parts) >= 2 && parts[0] == "bill" {
		billID, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return 0, 0, false
		}
		var businessID uint64
		if len(parts) >= 4 && parts[2] == "business" {
			parsedBusinessID, err := strconv.ParseUint(parts[3], 10, 32)
			if err == nil {
				businessID = parsedBusinessID
			}
		}
		return uint(billID), uint(businessID), true
	}
	return 0, 0, false
}

func paypalAmountStringToCents(amount string) (int64, error) {
	return money.ParseDollarStringToCents(amount)
}

func paypalHeader(headers map[string]string, key string) string {
	if value, ok := headers[key]; ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	for headerKey, value := range headers {
		if strings.EqualFold(headerKey, key) && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringFromPayPalAny(value interface{}) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func pluginFirstNonEmptyPayPal(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// Ensure PayPalPlugin implements PaymentPlugin interface
var _ plugins.PaymentPlugin = (*PayPalPlugin)(nil)

// Ensure PayPalPlugin implements WebhookRequestVerifier. PayPal's
// VerifyWebhookSignature intentionally returns false (its real verification
// needs the full request via VerifyWebhookRequest). The webhook handler routes
// to VerifyWebhookRequest only when this interface is satisfied; if the method
// signature ever drifted, the type assertion would silently fail and every
// PayPal webhook would be rejected. This assertion makes that drift a build error.
var _ plugins.WebhookRequestVerifier = (*PayPalPlugin)(nil)
