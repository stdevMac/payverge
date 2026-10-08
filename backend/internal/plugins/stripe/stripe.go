package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	appconfig "github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/money"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/webhookhmac"
	"github.com/stdevmac/payverge/backend/internal/services"
)

const stripePluginWebhookClockSkewLimit = 5 * time.Minute

// stripeAPIVersion pins every Stripe API call this plugin makes to a fixed API
// shape so response parsing stays stable across Stripe's rolling upgrades.
const stripeAPIVersion = "2024-06-20"

// StripePlugin implements the PaymentPlugin interface
type StripePlugin struct {
	pluginService *services.PluginService
}

// NewStripePlugin creates a new Stripe plugin instance
func NewStripePlugin(pluginService *services.PluginService) *StripePlugin {
	return &StripePlugin{
		pluginService: pluginService,
	}
}

// GetName returns the plugin name
func (sp *StripePlugin) GetName() string {
	return "stripe"
}

// GetDisplayName returns the user-facing name
func (sp *StripePlugin) GetDisplayName() string {
	return "Stripe"
}

// GetDescription returns the plugin description
func (sp *StripePlugin) GetDescription() string {
	return "Accept credit card payments through Stripe with advanced fraud protection and global reach"
}

// GetCategory returns the plugin category
func (sp *StripePlugin) GetCategory() string {
	return "payment"
}

// GetVersion returns the plugin version
func (sp *StripePlugin) GetVersion() string {
	return "1.0.0"
}

// GetFeatures returns the plugin features as JSON array
func (sp *StripePlugin) GetFeatures() string {
	return `["Credit Card Processing", "Fraud Detection", "Global Payments", "Mobile Payments"]`
}

// GetConfigSchema returns the JSON schema for configuration.
// Manual mode uses secret_key + publishable_key; OAuth mode uses stripe_user_id
// (validated in ValidateConfig). Neither is unconditionally required in the
// schema so OAuth-connected businesses can enable without merchant keys.
func (sp *StripePlugin) GetConfigSchema() string {
	return `{
		"type": "object",
		"required": [],
		"properties": {
			"connection_mode": {
				"type": "string",
				"title": "Connection Mode",
				"description": "oauth or manual",
				"enum": ["oauth", "manual"]
			},
			"stripe_user_id": {
				"type": "string",
				"title": "Stripe Connected Account ID",
				"description": "Connected account id (acct_…) from Connect OAuth"
			},
			"secret_key": {
				"type": "string",
				"title": "Secret Key",
				"description": "Your Stripe secret key (starts with sk_) — manual mode",
				"format": "password"
			},
			"publishable_key": {
				"type": "string", 
				"title": "Publishable Key",
				"description": "Your Stripe publishable key (starts with pk_)"
			},
			"webhook_endpoint": {
				"type": "string",
				"title": "Webhook Endpoint",
				"description": "Webhook URL for payment notifications",
				"format": "uri"
			},
			"webhook_secret": {
				"type": "string",
				"title": "Webhook Signing Secret",
				"description": "Stripe webhook signing secret (starts with whsec_)",
				"format": "password"
			},
			"webhook_secret_previous": {
				"type": "string",
				"title": "Previous Webhook Signing Secret",
				"description": "Temporary overlap key used while rotating webhook signing secrets",
				"format": "password"
			},
			"auto_capture": {
				"type": "boolean",
				"title": "Auto Capture Payments",
				"description": "Automatically capture payments when authorized",
				"default": true
			},
			"access_token": {
				"type": "string",
				"title": "OAuth Access Token",
				"format": "password"
			},
			"refresh_token": {
				"type": "string",
				"title": "OAuth Refresh Token",
				"format": "password"
			}
		}
	}`
}

// IsActive returns whether the plugin is active on the platform
func (sp *StripePlugin) IsActive() bool {
	return plugins.PaymentProviderStartsEnabled(sp.GetName(), appconfig.IsProductionMode(false))
}

// stripeGrantOwnedKeys are the facts that only a completed Connect grant may
// establish. They are written by the OAuth callback and by the deauthorization
// webhook, never by a config save.
var stripeGrantOwnedKeys = []string{
	"stripe_user_id",
	"oauth_status",
	"live_mode",
	"access_token",
	"refresh_token",
}

// NormalizeConfig pins the OAuth grant facts to what the server stored, so a
// config save can never claim a connection that never happened.
//
// A dashboard save carries whatever the operator's browser sent. Accepting
// connection_mode/stripe_user_id/oauth_status from it would let anyone with
// plugin-write permission declare "oauth, acct_SOMEONE_ELSE, connected":
// Payverge would then charge through the platform key with a Stripe-Account
// header for an account the business does not own, and — because Connect
// webhooks route on stripe_user_id — that account's events would be delivered
// to this business. ValidateConfig cannot catch it; an acct_-prefixed string is
// all it ever sees.
//
// The operator keeps two legitimate moves: connecting (via the OAuth callback,
// which writes these keys itself and does not pass through here) and switching
// back to manual keys, which drops the grant instead of silently keeping it.
func (sp *StripePlugin) NormalizeConfig(businessID uint, config map[string]interface{}) (map[string]interface{}, error) {
	normalized := make(map[string]interface{}, len(config)+len(stripeGrantOwnedKeys))
	for key, value := range config {
		normalized[key] = value
	}
	// Whatever the request said about the grant is discarded up front; only the
	// stored grant can put these back.
	for _, key := range stripeGrantOwnedKeys {
		delete(normalized, key)
	}

	existing, err := sp.storedConfigForNormalize(businessID)
	if err != nil {
		return nil, err
	}

	grantedAccount := ""
	if strings.EqualFold(stripeConfigString(existing, "connection_mode"), "oauth") {
		grantedAccount = stripeConfigString(existing, "stripe_user_id")
	}
	requestedMode := strings.ToLower(stripeConfigString(normalized, "connection_mode"))

	if grantedAccount != "" && (requestedMode == "" || requestedMode == "oauth") {
		// A grant is on file and the save did not ask to leave it: restore every
		// server-owned value verbatim. A save that omits connection_mode (the UI
		// does not always send it) must not disconnect the merchant.
		for _, key := range stripeGrantOwnedKeys {
			if value, ok := existing[key]; ok {
				normalized[key] = value
			}
		}
		normalized["connection_mode"] = "oauth"
		return normalized, nil
	}

	// Either no grant is on file, or the operator explicitly switched to manual.
	// Claiming OAuth without a grant falls back to manual so ValidateConfig
	// demands real merchant keys instead of trusting the claim.
	if requestedMode == "" || requestedMode == "oauth" {
		delete(normalized, "connection_mode")
	}
	return normalized, nil
}

// storedConfigForNormalize reads the config already persisted for this business,
// including a disabled row — an operator editing a disabled plugin must not
// lose their Connect grant.
func (sp *StripePlugin) storedConfigForNormalize(businessID uint) (map[string]interface{}, error) {
	if businessID == 0 {
		return nil, nil
	}
	existing, _, _, err := database.GetBusinessPluginConfigState(businessID, sp.GetName())
	if err != nil {
		if errors.Is(err, database.ErrBusinessPluginNotEnabled) {
			return nil, nil
		}
		return nil, fmt.Errorf("load existing stripe config: %w", err)
	}
	return existing, nil
}

// stripeConfigString reads a config value as a trimmed string, treating absent
// and nil alike so fmt.Sprint's "<nil>" never leaks into a comparison.
func stripeConfigString(config map[string]interface{}, key string) string {
	if config == nil {
		return ""
	}
	value, ok := config[key]
	if !ok || value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// ValidateConfig validates the plugin configuration for manual or OAuth mode.
//
// The OAuth branch trusts connection_mode/stripe_user_id/oauth_status because
// NormalizeConfig has already replaced them with the stored grant; nothing an
// operator submits reaches here.
func (sp *StripePlugin) ValidateConfig(config map[string]interface{}) error {
	mode := strings.ToLower(strings.TrimSpace(fmt.Sprint(config["connection_mode"])))
	if mode == "<nil>" {
		mode = ""
	}

	if mode == "oauth" {
		status := strings.ToLower(strings.TrimSpace(fmt.Sprint(config["oauth_status"])))
		if status == "reauth_required" || status == "disconnected" {
			return errors.New("stripe OAuth connection requires reauth")
		}
		userID := strings.TrimSpace(fmt.Sprint(config["stripe_user_id"]))
		if userID == "" || userID == "<nil>" {
			return errors.New("stripe_user_id is required for OAuth connection mode")
		}
		if !strings.HasPrefix(userID, "acct_") {
			return errors.New("invalid stripe_user_id format (expected acct_…)")
		}
		if webhookSecret, ok := config["webhook_secret"].(string); ok && strings.TrimSpace(webhookSecret) != "" {
			if !strings.HasPrefix(strings.TrimSpace(webhookSecret), "whsec_") {
				return errors.New("invalid Stripe webhook secret format")
			}
		}
		return nil
	}

	// Manual (default / legacy): require merchant keys.
	secretKey, ok := config["secret_key"].(string)
	if !ok || secretKey == "" {
		return errors.New("secret_key is required")
	}

	if !isValidStripeSecretKey(secretKey) {
		return errors.New("invalid Stripe secret key format")
	}

	publishableKey, ok := config["publishable_key"].(string)
	if !ok || publishableKey == "" {
		return errors.New("publishable_key is required")
	}

	if !isValidStripePublishableKey(publishableKey) {
		return errors.New("invalid Stripe publishable key format")
	}

	if webhookSecret, ok := config["webhook_secret"].(string); ok && strings.TrimSpace(webhookSecret) != "" {
		if !strings.HasPrefix(strings.TrimSpace(webhookSecret), "whsec_") {
			return errors.New("invalid Stripe webhook secret format")
		}
	}

	return nil
}

// Initialize sets up the plugin with the given configuration
func (sp *StripePlugin) Initialize(businessID uint, config map[string]interface{}) error {
	// Validate configuration first
	if err := sp.ValidateConfig(config); err != nil {
		return fmt.Errorf("stripe plugin initialization failed: %v", err)
	}

	mode := strings.ToLower(strings.TrimSpace(fmt.Sprint(config["connection_mode"])))
	if mode == "oauth" {
		// OAuth: platform secret + connected account; reject test livemode in prod
		// is enforced at OAuth callback. Optionally probe platform key.
		platformKey := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
		if platformKey == "" {
			return fmt.Errorf("stripe OAuth requires platform STRIPE_SECRET_KEY")
		}
		if appconfig.IsProductionMode(false) && strings.HasPrefix(platformKey, "sk_test_") {
			return fmt.Errorf("test-mode platform Stripe key (sk_test_) is not allowed in production mode")
		}
		if err := sp.testStripeConnection(platformKey); err != nil {
			return fmt.Errorf("stripe API connection test failed: %v", err)
		}
		return nil
	}

	// Reject test-mode keys in production
	secretKey := config["secret_key"].(string)
	if appconfig.IsProductionMode(false) && strings.HasPrefix(secretKey, "sk_test_") {
		return fmt.Errorf("test-mode Stripe keys (sk_test_) are not allowed in production mode")
	}

	// Test Stripe API connection with provided keys
	if err := sp.testStripeConnection(secretKey); err != nil {
		return fmt.Errorf("stripe API connection test failed: %v", err)
	}

	return nil
}

// Cleanup performs any necessary cleanup when plugin is disabled
func (sp *StripePlugin) Cleanup(businessID uint) error {
	log.Printf("Plugin cleanup called for business %d — webhook endpoints not automatically removed from Stripe", businessID)
	return nil
}

// ProcessPayment processes a payment using Stripe
func (sp *StripePlugin) ProcessPayment(businessID uint, amount int64, currency string, metadata map[string]interface{}) (string, error) {
	return "", errors.New("generic Stripe ProcessPayment is unsupported; use CreateBillPayment checkout sessions")
}

// RefundPayment creates a Stripe refund for a Checkout Session or PaymentIntent.
// Checkout-based bill payments store the Checkout Session ID locally, while
// Stripe's refund API expects a PaymentIntent, so cs_* IDs are resolved first.
// amount is cents; amount == 0 requests a full remaining refund from Stripe.
func (sp *StripePlugin) RefundPayment(businessID uint, paymentID string, amount int64) error {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return errors.New("stripe payment ID is required")
	}
	if amount < 0 {
		return errors.New("stripe refund amount cannot be negative")
	}

	config, err := sp.getConfig(businessID)
	if err != nil {
		return fmt.Errorf("failed to get Stripe config: %w", err)
	}

	paymentIntentID := paymentID
	currency := ""
	isCheckout := strings.HasPrefix(paymentID, "cs_")
	if isCheckout {
		paymentIntentID, currency, err = sp.paymentIntentForCheckoutSession(paymentID, config)
		if err != nil {
			return err
		}
	}

	refundRequest := map[string]interface{}{
		"payment_intent": paymentIntentID,
	}
	if amount > 0 {
		// A partial refund amount must be expressed in the currency's smallest
		// unit, matching how the charge was created. The checkout session
		// already carries the currency for free; for a bare PaymentIntent we
		// resolve it so a zero-decimal refund (e.g. JPY) isn't sent 100× too
		// large and rejected.
		if !isCheckout {
			currency, err = sp.currencyForPaymentIntent(paymentIntentID, config)
			if err != nil {
				return err
			}
		}
		refundRequest["amount"] = money.MinorUnits(amount, currency)
	}

	// Deterministic idempotency key on the (payment intent, amount) tuple so a
	// network-level retry of this POST cannot issue a second refund.
	idempotencyKey := fmt.Sprintf("refund_%s_%d", paymentIntentID, amount)
	response, err := sp.makeStripeAPICall(context.Background(), "POST", "/v1/refunds", refundRequest, config, idempotencyKey)
	if err != nil {
		return fmt.Errorf("failed to create Stripe refund: %w", err)
	}
	refundID, _ := response["id"].(string)
	if strings.TrimSpace(refundID) == "" {
		return errors.New("invalid Stripe refund response: missing refund ID")
	}
	// A refund object is returned even when it did not succeed. Only "succeeded"
	// and "pending" (async settlement) count as success; "failed"/"canceled"
	// (and any other terminal state) must surface as an error so callers don't
	// mark money returned that Stripe never returned.
	status := strings.TrimSpace(strings.ToLower(fmt.Sprint(response["status"])))
	switch status {
	case "succeeded", "pending", "":
		// "" tolerates Stripe responses that omit status; the refund ID is proof
		// the refund was accepted.
		return nil
	default:
		return fmt.Errorf("stripe refund %s did not succeed: status %q", refundID, status)
	}
}

// GetPaymentStatus gets the status of a payment from Stripe
func (sp *StripePlugin) GetPaymentStatus(businessID uint, paymentID string) (string, error) {
	if strings.TrimSpace(paymentID) == "" {
		return "", errors.New("stripe checkout session ID is required")
	}
	config, err := sp.getConfig(businessID)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response, err := sp.makeStripeAPICall(ctx, http.MethodGet, "/v1/checkout/sessions/"+url.PathEscape(paymentID), nil, config, "")
	if err != nil {
		return "", err
	}
	return stripeCheckoutReconciliationStatus(response)
}

func stripeCheckoutReconciliationStatus(response map[string]interface{}) (string, error) {
	paymentStatus := strings.ToLower(strings.TrimSpace(fmt.Sprint(response["payment_status"])))
	sessionStatus := strings.ToLower(strings.TrimSpace(fmt.Sprint(response["status"])))
	switch {
	case paymentStatus == "paid" || paymentStatus == "no_payment_required":
		return "completed", nil
	case sessionStatus == "expired":
		return "expired", nil
	case sessionStatus == "open" || sessionStatus == "complete" || paymentStatus == "unpaid":
		return "pending", nil
	default:
		return "", errors.New("stripe checkout session response missing recognized status")
	}
}

// Helper functions for Stripe API integration

func isValidStripeSecretKey(key string) bool {
	return len(key) > 7 && (key[:3] == "sk_" || key[:8] == "rk_live_" || key[:8] == "rk_test_")
}

func isValidStripePublishableKey(key string) bool {
	return len(key) > 7 && (key[:3] == "pk_")
}

func (sp *StripePlugin) testStripeConnection(secretKey string) error {
	if !isValidStripeSecretKey(secretKey) {
		return errors.New("invalid secret key format")
	}

	req, err := http.NewRequestWithContext(context.Background(), "GET", "https://api.stripe.com/v1/account", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(secretKey, "")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("WARNING: Stripe connection test failed (network): %v — plugin enabled with unverified credentials", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return errors.New("stripe API key is invalid — authentication failed")
	}
	if resp.StatusCode >= 400 {
		log.Printf("WARNING: Stripe connection test returned HTTP %d — plugin enabled with unverified credentials", resp.StatusCode)
	}
	return nil
}

// init registers the Stripe plugin with the global registry
func init() {
	plugins.RegisterPluginInitializer("stripe", func(pluginService *services.PluginService) {
		stripePlugin := NewStripePlugin(pluginService)
		plugins.GlobalRegistry.RegisterPlugin(stripePlugin)
	})
}

func stripeCheckoutRedirectURLs(metadata map[string]interface{}) (string, string, error) {
	if metadata == nil {
		return "", "", errors.New("return_url and cancel_url are required")
	}
	successURL, _ := metadata["return_url"].(string)
	cancelURL, _ := metadata["cancel_url"].(string)
	successURL = strings.TrimSpace(successURL)
	cancelURL = strings.TrimSpace(cancelURL)
	if successURL == "" || cancelURL == "" {
		return "", "", errors.New("return_url and cancel_url are required")
	}
	return successURL, cancelURL, nil
}

// CreateBillPayment creates a Stripe Checkout Session for a specific bill
func (s *StripePlugin) CreateBillPayment(businessID uint, billID uint, amount int64, currency string, metadata map[string]interface{}) (*plugins.PaymentResponse, error) {
	config, err := s.getConfig(businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to get Stripe config: %w", err)
	}

	// Render the amount in the currency's major units for the display
	// description (currency-aware: "1000" for JPY, "10.00" for USD).
	amountStr := money.MajorUnitString(amount, currency)

	successURL, cancelURL, err := stripeCheckoutRedirectURLs(metadata)
	if err != nil {
		return nil, err
	}

	checkoutMetadata := buildStripeCheckoutMetadata(businessID, billID, metadata)

	// Create Stripe Checkout Session request
	sessionRequest := map[string]interface{}{
		"mode": "payment",
		"line_items": []map[string]interface{}{
			{
				"price_data": map[string]interface{}{
					"currency": strings.ToLower(currency),
					"product_data": map[string]interface{}{
						"name":        fmt.Sprintf("Bill #%d Payment", billID),
						"description": fmt.Sprintf("Payment for Bill #%d - %s %s", billID, amountStr, currency),
					},
					// Stripe expects the currency's smallest unit. That is NOT
					// the stored ×100 "cents" for zero/three-decimal currencies
					// (e.g. ¥1000 is 1000, not 100000), so convert.
					"unit_amount": money.MinorUnits(amount, currency),
				},
				"quantity": 1,
			},
		},
		"success_url": successURL,
		"cancel_url":  cancelURL,
		"metadata":    checkoutMetadata,
		"payment_intent_data": map[string]interface{}{
			"metadata": checkoutMetadata,
		},
	}

	// Make API call to Stripe Checkout Sessions API. The idempotency key is
	// derived from the stable (business, bill, amount) tuple so a retried request
	// reuses the same checkout session instead of creating a duplicate.
	idempotencyKey := fmt.Sprintf("checkout_%d_%d_%d", businessID, billID, amount)
	response, err := s.makeStripeAPICall(context.Background(), "POST", "/v1/checkout/sessions", sessionRequest, config, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stripe checkout session: %w", err)
	}

	// Extract session details
	sessionID, ok := response["id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid Stripe response: missing session ID")
	}

	checkoutURL, ok := response["url"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid Stripe response: missing checkout URL")
	}

	return &plugins.PaymentResponse{
		PaymentID:   sessionID,
		PaymentURL:  checkoutURL,
		RedirectURL: checkoutURL,
		Status:      "pending",
		Metadata: map[string]interface{}{
			"session_id":  sessionID,
			"bill_id":     billID,
			"business_id": businessID,
		},
	}, nil
}

// HandleWebhook processes Stripe webhook callbacks
func (s *StripePlugin) HandleWebhook(businessID uint, payload []byte, headers map[string]string) (*plugins.WebhookResponse, error) {
	// Parse webhook payload
	var webhookData map[string]interface{}
	if err := json.Unmarshal(payload, &webhookData); err != nil {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook payload",
		}, nil
	}

	// Extract event type
	eventType, ok := webhookData["type"].(string)
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing event type",
		}, nil
	}

	// Handle checkout session completed (payment succeeded) and async settlement.
	// async_payment_succeeded fires when a delayed-method payment (bank debit, OXXO,
	// Boleto, BLIK) eventually settles after the initial checkout.session.completed
	// event was acked as pending.
	if eventType == "checkout.session.completed" ||
		eventType == "checkout.session.async_payment_succeeded" {
		return s.handleCheckoutCompleted(businessID, webhookData)
	}

	// async_payment_failed fires when a delayed-method payment does not settle.
	if eventType == "checkout.session.async_payment_failed" {
		return s.handleCheckoutAsyncFailed(businessID, webhookData)
	}
	if eventType == "checkout.session.expired" {
		return s.handleCheckoutExpired(businessID, webhookData)
	}

	// Stripe's charge/refund and dispute resources identify the original
	// settlement by payment_intent. Keep that identity all the way through the
	// handler so a lifecycle event updates the same local Payment row created by
	// Checkout completion.
	switch eventType {
	case "charge.refunded":
		return s.handleChargeLifecycle(webhookData, "refunded", "Charge refunded")
	case "charge.dispute.created":
		return s.handleChargeLifecycle(webhookData, "disputed", "Charge disputed")
	case "charge.dispute.funds_withdrawn":
		return s.handleChargeLifecycle(webhookData, "reversed", "Dispute funds withdrawn")
	case "charge.dispute.funds_reinstated":
		return s.handleChargeLifecycle(webhookData, "dispute_reinstated", "Dispute funds reinstated")
	case "charge.dispute.closed":
		return s.handleDisputeClosed(webhookData)
	}

	if eventType == "payment_intent.succeeded" {
		metrics.PaymentWebhookUnsupportedActions.WithLabelValues(s.GetName(), eventType).Inc()
		return &plugins.WebhookResponse{
			Status:  "unsupported",
			Success: true,
			Message: "PaymentIntent succeeded is unsupported because Stripe plugin only creates Checkout Sessions",
		}, nil
	}

	// Handle payment failed
	if eventType == "payment_intent.payment_failed" {
		return s.handlePaymentFailed(businessID, webhookData)
	}

	// Connect deauthorization: mark OAuth link as needing reauth. Primary path
	// is the platform webhook handler (business may be unresolved); this branch
	// covers the case where account was already mapped to a business.
	if eventType == "account.application.deauthorized" {
		return &plugins.WebhookResponse{
			Success: true,
			Message: "Account deauthorized — OAuth reauth required",
			Status:  "deauthorized",
		}, nil
	}

	return &plugins.WebhookResponse{
		Success: true,
		Message: "Event processed",
	}, nil
}

// GetWebhookEndpoint returns the webhook endpoint path for Stripe
func (s *StripePlugin) GetWebhookEndpoint() string {
	return "/api/v1/webhooks/stripe"
}

// VerifyWebhookSignature verifies Stripe webhook signature
func (s *StripePlugin) VerifyWebhookSignature(payload []byte, signature string, secret string) bool {
	signature = strings.TrimSpace(signature)
	secret = strings.TrimSpace(secret)
	if signature == "" || secret == "" {
		return false
	}

	timestamp, signatures := parseStripeSignatureHeader(signature)
	if timestamp == 0 || len(signatures) == 0 {
		return false
	}

	now := time.Now().Unix()
	if delta := now - timestamp; delta > int64(stripePluginWebhookClockSkewLimit.Seconds()) || delta < -int64(stripePluginWebhookClockSkewLimit.Seconds()) {
		return false
	}

	signedPayload := fmt.Sprintf("%d.%s", timestamp, string(payload))
	return webhookhmac.VerifyHexSHA256(secret, []byte(signedPayload), signatures)
}

func parseStripeSignatureHeader(header string) (int64, []string) {
	var timestamp int64
	signatures := make([]string, 0)

	parts := strings.Split(header, ",")
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		switch key {
		case "t":
			if ts, err := strconv.ParseInt(value, 10, 64); err == nil {
				timestamp = ts
			}
		case "v1":
			if value != "" {
				signatures = append(signatures, value)
			}
		}
	}

	return timestamp, signatures
}

func buildStripeCheckoutMetadata(businessID uint, billID uint, metadata map[string]interface{}) map[string]interface{} {
	checkoutMetadata := map[string]interface{}{
		"bill_id":     fmt.Sprintf("%d", billID),
		"business_id": fmt.Sprintf("%d", businessID),
	}

	for _, key := range []string{"tip_amount_cents", "bill_amount_cents"} {
		if value, ok := metadata[key]; ok {
			if encoded := stripeMetadataString(value); encoded != "" {
				checkoutMetadata[key] = encoded
			}
		}
	}

	return checkoutMetadata
}

func stripeMetadataString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
	case float32:
		if typed == float32(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
	}

	return ""
}

func extractStripeMetadataInt64(metadata map[string]interface{}, key string) (int64, bool) {
	value, ok := metadata[key]
	if !ok {
		return 0, false
	}

	switch typed := value.(type) {
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed, true
		}
	case float64:
		return int64(typed), true
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	}

	return 0, false
}

// Helper method to get Stripe configuration for a business
func (s *StripePlugin) getConfig(businessID uint) (*StripeConfig, error) {
	if s == nil || s.pluginService == nil {
		return nil, errors.New("stripe plugin service not configured")
	}

	config, err := s.pluginService.GetPluginConfig(businessID, "stripe")
	if err != nil {
		return nil, fmt.Errorf("failed to get Stripe config: %w", err)
	}

	return s.configFromMap(config)
}

// configFromMap builds StripeConfig from a raw plugin config map.
// OAuth mode uses the platform STRIPE_SECRET_KEY + Stripe-Account header;
// manual/legacy uses the merchant secret_key.
func (s *StripePlugin) configFromMap(config map[string]interface{}) (*StripeConfig, error) {
	if config == nil {
		return nil, errors.New("stripe config is empty")
	}
	mode := strings.ToLower(strings.TrimSpace(fmt.Sprint(config["connection_mode"])))
	if mode == "<nil>" || mode == "" {
		mode = "manual"
	}

	webhookSecret, _ := config["webhook_secret"].(string)
	publishableKey, _ := config["publishable_key"].(string)

	if mode == "oauth" {
		status := strings.ToLower(strings.TrimSpace(fmt.Sprint(config["oauth_status"])))
		if status == "reauth_required" || status == "disconnected" {
			return nil, errors.New("stripe OAuth connection requires reauth")
		}
		userID := strings.TrimSpace(fmt.Sprint(config["stripe_user_id"]))
		if userID == "" || userID == "<nil>" {
			return nil, errors.New("stripe_user_id not configured for OAuth mode")
		}
		platformKey := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
		if platformKey == "" {
			return nil, errors.New("platform STRIPE_SECRET_KEY not configured for OAuth mode")
		}
		if appconfig.IsProductionMode(false) && strings.HasPrefix(platformKey, "sk_test_") {
			return nil, errors.New("test-mode platform Stripe key is not allowed in production")
		}
		return &StripeConfig{
			SecretKey:      platformKey,
			PublishableKey: publishableKey,
			WebhookSecret:  webhookSecret,
			ConnectionMode: "oauth",
			StripeUserID:   userID,
		}, nil
	}

	secretKey, ok := config["secret_key"].(string)
	if !ok || secretKey == "" {
		return nil, errors.New("stripe secret_key not configured")
	}
	if publishableKey == "" {
		return nil, errors.New("stripe publishable_key not configured")
	}

	return &StripeConfig{
		SecretKey:      secretKey,
		PublishableKey: publishableKey,
		WebhookSecret:  webhookSecret,
		ConnectionMode: "manual",
	}, nil
}

// StripeConfig represents Stripe configuration for API calls.
// When ConnectionMode is "oauth", SecretKey is the platform key and
// StripeUserID is sent as the Stripe-Account header.
type StripeConfig struct {
	SecretKey      string
	PublishableKey string
	WebhookSecret  string
	ConnectionMode string // "oauth" | "manual"
	StripeUserID   string // acct_… when oauth
	// APIBaseURL overrides api.stripe.com for tests only.
	APIBaseURL string
}

func (s *StripePlugin) paymentIntentForCheckoutSession(sessionID string, config *StripeConfig) (string, string, error) {
	response, err := s.makeStripeAPICall(context.Background(), "GET", "/v1/checkout/sessions/"+url.PathEscape(sessionID), nil, config, "")
	if err != nil {
		return "", "", fmt.Errorf("failed to retrieve Stripe checkout session: %w", err)
	}
	paymentIntentID, _ := response["payment_intent"].(string)
	if strings.TrimSpace(paymentIntentID) == "" {
		return "", "", errors.New("Stripe checkout session is missing payment_intent")
	}
	currency, _ := response["currency"].(string)
	return strings.TrimSpace(paymentIntentID), strings.TrimSpace(currency), nil
}

// currencyForPaymentIntent fetches a PaymentIntent's currency so a partial
// refund amount can be converted to the currency's smallest unit. Only used on
// the bare-PaymentIntent refund path (the checkout-session path gets the
// currency for free from the session lookup).
func (s *StripePlugin) currencyForPaymentIntent(paymentIntentID string, config *StripeConfig) (string, error) {
	response, err := s.makeStripeAPICall(context.Background(), "GET", "/v1/payment_intents/"+url.PathEscape(paymentIntentID), nil, config, "")
	if err != nil {
		return "", fmt.Errorf("failed to retrieve Stripe payment intent: %w", err)
	}
	currency, _ := response["currency"].(string)
	return strings.TrimSpace(currency), nil
}

// Helper method to make Stripe API calls. idempotencyKey, when non-empty, is
// sent as the Idempotency-Key header so a network-level retry of a mutating POST
// (refund/checkout creation) cannot execute the operation twice.
//
// OAuth mode authenticates with the platform secret and sets Stripe-Account.
// Direct charges only — callers must never attach application_fee_amount.
func (s *StripePlugin) makeStripeAPICall(ctx context.Context, method, endpoint string, payload interface{}, config *StripeConfig, idempotencyKey string) (map[string]interface{}, error) {
	if config == nil {
		return nil, errors.New("stripe config is required")
	}
	// Stripe API base URL (tests may override via config.APIBaseURL)
	baseURL := "https://api.stripe.com"
	if override := strings.TrimRight(strings.TrimSpace(config.APIBaseURL), "/"); override != "" {
		baseURL = override
	}

	// Prepare request body. Strip any application fee keys so OAuth direct
	// charges never take a platform commission even if a caller injects them.
	if payload != nil {
		if m, ok := payload.(map[string]interface{}); ok {
			delete(m, "application_fee_amount")
			delete(m, "application_fee")
			if pi, ok := m["payment_intent_data"].(map[string]interface{}); ok {
				delete(pi, "application_fee_amount")
			}
		}
	}

	var reqBody io.Reader
	if payload != nil {
		// Convert payload to form-encoded data (Stripe's preferred format)
		formData := s.convertToFormData(payload)
		reqBody = strings.NewReader(formData)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, method, baseURL+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+config.SecretKey)
	req.Header.Set("Stripe-Version", stripeAPIVersion)
	if strings.EqualFold(config.ConnectionMode, "oauth") && strings.TrimSpace(config.StripeUserID) != "" {
		req.Header.Set("Stripe-Account", strings.TrimSpace(config.StripeUserID))
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
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
		return nil, fmt.Errorf("stripe API error %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// Helper method to convert payload to form-encoded data for Stripe API
func (s *StripePlugin) convertToFormData(payload interface{}) string {
	var formParts []string

	if data, ok := payload.(map[string]interface{}); ok {
		for key, value := range data {
			formParts = append(formParts, s.encodeFormField(key, value)...)
		}
	}

	result := strings.Join(formParts, "&")
	return result
}

// Helper method to encode form fields recursively
func (s *StripePlugin) encodeFormField(prefix string, value interface{}) []string {
	var parts []string

	switch v := value.(type) {
	case string:
		parts = append(parts, fmt.Sprintf("%s=%s", url.QueryEscape(prefix), url.QueryEscape(v)))
	case int, int64, float64:
		parts = append(parts, fmt.Sprintf("%s=%s", url.QueryEscape(prefix), url.QueryEscape(fmt.Sprintf("%v", v))))
	case map[string]interface{}:
		for k, val := range v {
			fieldName := fmt.Sprintf("%s[%s]", prefix, k)
			parts = append(parts, s.encodeFormField(fieldName, val)...)
		}
	case []interface{}:
		for i, val := range v {
			fieldName := fmt.Sprintf("%s[%d]", prefix, i)
			parts = append(parts, s.encodeFormField(fieldName, val)...)
		}
	case []map[string]interface{}:
		for i, val := range v {
			fieldName := fmt.Sprintf("%s[%d]", prefix, i)
			parts = append(parts, s.encodeFormField(fieldName, val)...)
		}
	}

	return parts
}

// Helper method to handle checkout session completed webhook
func (s *StripePlugin) handleCheckoutCompleted(businessID uint, webhookData map[string]interface{}) (*plugins.WebhookResponse, error) {
	// Extract checkout session details from webhook
	data, ok := webhookData["data"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook data",
		}, nil
	}

	object, ok := data["object"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook object",
		}, nil
	}

	sessionID, _ := object["id"].(string)
	paymentIntentID := strings.TrimSpace(fmt.Sprint(object["payment_intent"]))
	if paymentIntentID == "<nil>" {
		paymentIntentID = ""
	}
	amountTotal, _ := object["amount_total"].(float64)
	currency, _ := object["currency"].(string)

	// Gate completion on payment_status. Async methods (bank debit, OXXO, Boleto,
	// BLIK) fire checkout.session.completed with payment_status "unpaid" BEFORE
	// money settles. Marking the bill paid here would record a payment backed by
	// nothing if a later async_payment_failed arrives. Ack (Success:true) so Stripe
	// does not retry-loop; the checkout.session.async_payment_succeeded event will
	// complete settlement.
	paymentStatus, _ := object["payment_status"].(string)
	if paymentStatus != "paid" && paymentStatus != "no_payment_required" {
		return &plugins.WebhookResponse{
			PaymentID: sessionID,
			Status:    "pending",
			Success:   true,
			Message:   "Checkout session not yet paid (async method) — awaiting settlement",
		}, nil
	}

	// Extract bill ID from metadata
	var billID uint
	responseMetadata := make(map[string]interface{})
	if metadata, ok := object["metadata"].(map[string]interface{}); ok {
		if billIDStr, ok := metadata["bill_id"].(string); ok {
			if id, err := strconv.ParseUint(billIDStr, 10, 32); err == nil {
				billID = uint(id)
			}
		}
		if value, ok := extractStripeMetadataInt64(metadata, "tip_amount_cents"); ok {
			responseMetadata["tip_amount_cents"] = value
		}
		if value, ok := extractStripeMetadataInt64(metadata, "bill_amount_cents"); ok {
			responseMetadata["bill_amount_cents"] = value
		}
	}
	settlementID := sessionID
	if paymentIntentID != "" {
		settlementID = paymentIntentID
		if sessionID != "" && sessionID != paymentIntentID {
			responseMetadata["provider_tracker_id"] = sessionID
		}
	}

	return &plugins.WebhookResponse{
		PaymentID: settlementID,
		BillID:    billID,
		Status:    "completed",
		// Stripe reports amount_total in the currency's provider minor unit,
		// which is NOT the platform's ×100 "cents" for zero-decimal (JPY/KRW) or
		// three-decimal (BHD/KWD) currencies. Convert back to stored cents so
		// settlement's breakdown check matches. The tip/bill_amount_cents in
		// metadata below are OUR values (written in cents, echoed verbatim by
		// Stripe) and must NOT be re-converted.
		Amount:        money.FromMinorUnits(int64(amountTotal), currency),
		Currency:      strings.ToUpper(currency),
		TransactionID: settlementID,
		Metadata:      responseMetadata,
		Success:       true,
		Message:       "Payment completed successfully",
	}, nil
}

// Helper method to handle payment failed webhook
func (s *StripePlugin) handlePaymentFailed(businessID uint, webhookData map[string]interface{}) (*plugins.WebhookResponse, error) {
	data, ok := webhookData["data"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook data",
		}, nil
	}

	object, ok := data["object"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook object",
		}, nil
	}

	paymentID, _ := object["id"].(string)

	return &plugins.WebhookResponse{
		PaymentID: paymentID,
		Status:    "failed",
		Success:   true,
		Message:   "Payment failed",
	}, nil
}

// handleCheckoutAsyncFailed maps a failed async checkout (bank debit / OXXO /
// Boleto / BLIK that did not settle) to a "failed" response so the bill is not
// left marked paid.
func (s *StripePlugin) handleCheckoutAsyncFailed(businessID uint, webhookData map[string]interface{}) (*plugins.WebhookResponse, error) {
	data, _ := webhookData["data"].(map[string]interface{})
	object, _ := data["object"].(map[string]interface{})
	sessionID, _ := object["id"].(string)
	return &plugins.WebhookResponse{
		PaymentID: sessionID,
		Status:    "failed",
		Success:   true,
		Message:   "Async payment failed",
	}, nil
}

func (s *StripePlugin) handleCheckoutExpired(businessID uint, webhookData map[string]interface{}) (*plugins.WebhookResponse, error) {
	data, _ := webhookData["data"].(map[string]interface{})
	object, _ := data["object"].(map[string]interface{})
	sessionID := strings.TrimSpace(fmt.Sprint(object["id"]))
	if sessionID == "<nil>" {
		sessionID = ""
	}
	return &plugins.WebhookResponse{
		PaymentID: sessionID,
		Status:    "expired",
		Success:   true,
		Message:   "Checkout session expired",
	}, nil
}

func (s *StripePlugin) handleChargeLifecycle(webhookData map[string]interface{}, status, message string) (*plugins.WebhookResponse, error) {
	data, ok := webhookData["data"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{Success: false, Message: "Invalid webhook data"}, nil
	}
	object, ok := data["object"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{Success: false, Message: "Invalid webhook object"}, nil
	}
	paymentIntentID := strings.TrimSpace(fmt.Sprint(object["payment_intent"]))
	if paymentIntentID == "" || paymentIntentID == "<nil>" {
		return &plugins.WebhookResponse{Success: false, Message: "Missing Stripe payment intent"}, nil
	}

	var billID uint
	if metadata, ok := object["metadata"].(map[string]interface{}); ok {
		if rawBillID := strings.TrimSpace(fmt.Sprint(metadata["bill_id"])); rawBillID != "" && rawBillID != "<nil>" {
			if parsed, err := strconv.ParseUint(rawBillID, 10, 32); err == nil {
				billID = uint(parsed)
			}
		}
	}
	currency := strings.TrimSpace(fmt.Sprint(object["currency"]))
	if currency == "<nil>" {
		currency = ""
	}
	providerAmount, _ := object["amount_refunded"].(float64)
	if providerAmount == 0 {
		providerAmount, _ = object["amount"].(float64)
	}
	response := &plugins.WebhookResponse{
		PaymentID:     paymentIntentID,
		BillID:        billID,
		Status:        status,
		Amount:        money.FromMinorUnits(int64(providerAmount), currency),
		Currency:      strings.ToUpper(currency),
		TransactionID: strings.TrimSpace(fmt.Sprint(object["id"])),
		Success:       true,
		Message:       message,
	}
	// Stripe totals are cumulative: charge.amount_refunded is the running
	// refunded total and dispute.amount is the disputed (withdrawn) amount.
	// Declare them so the handler moves the ledger by the delta only. A
	// missing amount keeps the legacy full-reversal path.
	switch status {
	case "refunded":
		if refunded, ok := object["amount_refunded"].(float64); ok && refunded > 0 {
			cumulative := money.FromMinorUnits(int64(refunded), currency)
			response.RefundedCumulativeCents = &cumulative
		}
	case "reversed":
		if disputed, ok := object["amount"].(float64); ok && disputed > 0 {
			withdrawn := money.FromMinorUnits(int64(disputed), currency)
			response.DisputedCents = &withdrawn
		}
	case "dispute_reinstated":
		zero := int64(0)
		response.DisputedCents = &zero
	}
	return response, nil
}

// handleDisputeClosed maps charge.dispute.closed: a won (or inquiry-only
// warning_closed) dispute reinstates the withdrawn amount; a lost one keeps
// the withdrawal, idempotent with an earlier funds_withdrawn.
func (s *StripePlugin) handleDisputeClosed(webhookData map[string]interface{}) (*plugins.WebhookResponse, error) {
	disputeStatus := ""
	if data, ok := webhookData["data"].(map[string]interface{}); ok {
		if object, ok := data["object"].(map[string]interface{}); ok {
			disputeStatus = strings.TrimSpace(fmt.Sprint(object["status"]))
		}
	}
	switch disputeStatus {
	case "won", "warning_closed":
		return s.handleChargeLifecycle(webhookData, "dispute_reinstated", "Dispute closed in merchant's favor")
	case "lost":
		response, err := s.handleChargeLifecycle(webhookData, "reversed", "Dispute lost")
		if err == nil && response != nil && response.Success && response.DisputedCents == nil {
			// Without an amount a lost close must not fall back to a full
			// reversal of a possibly larger capture; funds_withdrawn already
			// carried the withdrawal.
			return &plugins.WebhookResponse{Status: "unsupported", Success: true, Message: "Dispute lost without an amount"}, nil
		}
		return response, err
	default:
		return &plugins.WebhookResponse{Status: "unsupported", Success: true, Message: "Dispute closed with status " + disputeStatus}, nil
	}
}

// Ensure StripePlugin implements PaymentPlugin interface
var _ plugins.PaymentPlugin = (*StripePlugin)(nil)
