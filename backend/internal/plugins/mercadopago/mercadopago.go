package mercadopago

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	appconfig "github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/money"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/pluginurl"
	"github.com/stdevmac/payverge/backend/internal/plugins/webhookhmac"
	"github.com/stdevmac/payverge/backend/internal/services"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MercadoPagoPlugin implements the PaymentPlugin interface
type MercadoPagoPlugin struct {
	pluginService *services.PluginService
}

// NewMercadoPagoPlugin creates a new MercadoPago plugin instance
func NewMercadoPagoPlugin(pluginService *services.PluginService) *MercadoPagoPlugin {
	return &MercadoPagoPlugin{
		pluginService: pluginService,
	}
}

// GetName returns the plugin name
func (mp *MercadoPagoPlugin) GetName() string {
	return "mercadopago"
}

// GetDisplayName returns the user-facing name
func (mp *MercadoPagoPlugin) GetDisplayName() string {
	return "MercadoPago"
}

// GetDescription returns the plugin description
func (mp *MercadoPagoPlugin) GetDescription() string {
	return "Accept payments across Latin America with local payment methods, installments, and fraud protection"
}

// GetCategory returns the plugin category
func (mp *MercadoPagoPlugin) GetCategory() string {
	return "payment"
}

// GetVersion returns the plugin version
func (mp *MercadoPagoPlugin) GetVersion() string {
	return "1.0.0"
}

// GetFeatures returns the features as JSON string
func (mp *MercadoPagoPlugin) GetFeatures() string {
	return `["Credit & Debit Cards", "Bank Transfers", "Cash Payments", "Installments", "Local Payment Methods", "Fraud Protection", "Multi-country Support"]`
}

// IsActive returns whether the plugin is active on the platform.
// Mercado Pago is the dinner-wedge card rail: it stays in the operator catalog
// even when the production kill switch blocks unconfigured live charges.
// Stripe/PayPal remain env-gated. Demo venues enable this plugin as the
// default card path (#202). Unlike Stripe/PayPal it therefore defaults to
// active when PAYMENT_PROVIDER_MERCADOPAGO_ENABLED is unset (in every mode),
// but an explicit value is honored: anything other than "true" turns it off,
// so self-hosters can disable it from the environment.
func (mp *MercadoPagoPlugin) IsActive() bool {
	if strings.TrimSpace(os.Getenv("PAYMENT_PROVIDER_MERCADOPAGO_ENABLED")) == "" {
		return true
	}
	return plugins.PaymentProviderStartsEnabled(mp.GetName(), appconfig.IsProductionMode(false))
}

// GetConfigSchema returns the JSON schema for configuration
func (mp *MercadoPagoPlugin) GetConfigSchema() string {
	// required is only access_token so oauth configs pass ValidatePluginConfig's
	// schema check; manual-mode extras are enforced in ValidateConfig.
	return `{
		"type": "object",
		"required": ["access_token"],
		"properties": {
			"access_token": {
				"type": "string",
				"title": "Access Token",
				"description": "Your MercadoPago access token (production credentials start with APP_USR-)",
				"format": "password"
			},
			"public_key": {
				"type": "string",
				"title": "Public Key",
				"description": "Your MercadoPago public key (production credentials start with APP_USR-)"
			},
			"webhook_secret": {
				"type": "string",
				"title": "Webhook Secret",
				"description": "Secret key generated in MercadoPago Webhooks configuration",
				"format": "password"
			},
			"webhook_secret_previous": {
				"type": "string",
				"title": "Previous Webhook Secret",
				"description": "Temporary overlap key used while rotating webhook signing secrets",
				"format": "password"
			},
			"base_url": {
				"type": "string",
				"title": "Public API Base URL",
				"description": "Public backend origin used for MercadoPago webhook callbacks",
				"format": "uri"
			},
			"connection_mode": {
				"type": "string",
				"title": "Connection Mode",
				"description": "oauth or manual",
				"enum": ["oauth", "manual"]
			},
			"refresh_token": {
				"type": "string",
				"title": "Refresh Token",
				"format": "password"
			},
			"country": {
				"type": "string",
				"title": "Country",
				"description": "Operating country for local payment methods",
				"enum": ["AR", "BR", "CL", "CO", "MX", "PE", "UY"],
				"default": "AR"
			},
			"auto_return": {
				"type": "string",
				"title": "Auto Return",
				"description": "Automatic return behavior after payment",
				"enum": ["approved", "all"],
				"default": "approved"
			},
			"installments": {
				"type": "integer",
				"title": "Max Installments",
				"description": "Maximum number of installments allowed",
				"minimum": 1,
				"maximum": 24,
				"default": 12
			}
		}
	}`
}

// ValidateConfig validates the plugin configuration
func (mp *MercadoPagoPlugin) ValidateConfig(config map[string]interface{}) error {
	accessToken, ok := config["access_token"].(string)
	if !ok || accessToken == "" {
		return errors.New("access_token is required")
	}

	if !mp.isValidAccessToken(accessToken) {
		return errors.New("invalid MercadoPago access token format")
	}

	// Production must never accept TEST- credentials: they route to MP sandbox
	// and can "settle" real bills with public test cards (no real money moves).
	if appconfig.IsProductionMode(false) {
		if strings.HasPrefix(strings.TrimSpace(accessToken), "TEST-") {
			return errors.New("TEST- access tokens are not allowed in production; use APP_USR- production credentials")
		}
		if pk, _ := config["public_key"].(string); strings.HasPrefix(strings.TrimSpace(pk), "TEST-") {
			return errors.New("TEST- public keys are not allowed in production; use APP_USR- production credentials")
		}
		if resolveMercadoPagoEnvironment(config) == "sandbox" {
			return errors.New("MercadoPago sandbox environment is not allowed in production")
		}
	}

	connectionMode, _ := config["connection_mode"].(string)
	oauthMode := strings.EqualFold(strings.TrimSpace(connectionMode), "oauth")

	if oauthMode {
		refresh, _ := config["refresh_token"].(string)
		if strings.TrimSpace(refresh) == "" {
			return errors.New("refresh_token is required for oauth connection mode")
		}
		// base_url / webhook_secret optional (APP_BASE_URL + env webhook secret).
		if publicKey, ok := config["public_key"].(string); ok && strings.TrimSpace(publicKey) != "" {
			if !mp.isValidPublicKey(publicKey) {
				return errors.New("invalid MercadoPago public key format")
			}
		}
	} else {
		publicKey, ok := config["public_key"].(string)
		if !ok || publicKey == "" {
			return errors.New("public_key is required")
		}
		if !mp.isValidPublicKey(publicKey) {
			return errors.New("invalid MercadoPago public key format")
		}

		webhookSecret, ok := config["webhook_secret"].(string)
		if !ok || strings.TrimSpace(webhookSecret) == "" {
			return errors.New("webhook_secret is required")
		}

		baseURL, ok := config["base_url"].(string)
		if !ok || strings.TrimSpace(baseURL) == "" {
			return errors.New("base_url is required")
		}
		if _, err := url.ParseRequestURI(strings.TrimSpace(baseURL)); err != nil {
			return errors.New("base_url must be an absolute URL")
		}
	}

	// api_base_url is an optional override for MercadoPago's API endpoint. Guard
	// it against SSRF so an operator cannot point outbound API calls at an
	// internal service or cloud metadata. Empty means "use the default".
	if apiBaseURL, ok := config["api_base_url"].(string); ok && strings.TrimSpace(apiBaseURL) != "" {
		if err := pluginurl.ValidateBaseURL(apiBaseURL, pluginurl.MercadoPagoHosts); err != nil {
			return errors.New("api_base_url must be an https MercadoPago endpoint")
		}
	}

	// Validate country if provided
	if country, exists := config["country"].(string); exists {
		validCountries := []string{"AR", "BR", "CL", "CO", "MX", "PE", "UY"}
		isValid := false
		for _, validCountry := range validCountries {
			if normalizeMercadoPagoCountry(country) == validCountry {
				isValid = true
				break
			}
		}
		if !isValid {
			return errors.New("invalid country code")
		}
	}

	return nil
}

// refuseSandboxPaymentInProduction blocks guest/staff payment creation when the
// business is configured for MP sandbox while this server runs in production.
// Prefer refuseSandboxConfigInProduction from configFromMap so all charge paths
// share one gate; this helper remains for explicit call sites and tests.
func refuseSandboxPaymentInProduction(environment string) error {
	return refuseSandboxConfigInProduction(environment, "", "")
}

// refuseSandboxConfigInProduction refuses sandbox environment and TEST-
// credentials when the server is in production mode. Used by configFromMap so
// Point, QR, instore, return-capture, refunds, and Checkout Pro share one gate.
func refuseSandboxConfigInProduction(environment, accessToken, publicKey string) error {
	if !appconfig.IsProductionMode(false) {
		return nil
	}
	if strings.HasPrefix(strings.TrimSpace(accessToken), "TEST-") {
		return errors.New("TEST- access tokens are not allowed in production; use APP_USR- production credentials")
	}
	if strings.HasPrefix(strings.TrimSpace(publicKey), "TEST-") {
		return errors.New("TEST- public keys are not allowed in production; use APP_USR- production credentials")
	}
	if strings.EqualFold(strings.TrimSpace(environment), "sandbox") {
		return errors.New("MercadoPago sandbox payments are not allowed in production")
	}
	return nil
}

func normalizeMercadoPagoCountry(country string) string {
	switch strings.ToLower(strings.TrimSpace(country)) {
	case "argentina", "ar":
		return "AR"
	case "brazil", "br", "brasil":
		return "BR"
	case "chile", "cl":
		return "CL"
	case "colombia", "co":
		return "CO"
	case "mexico", "mx", "méxico":
		return "MX"
	case "peru", "pe", "perú":
		return "PE"
	case "uruguay", "uy":
		return "UY"
	default:
		return strings.ToUpper(strings.TrimSpace(country))
	}
}

// Initialize sets up the plugin with the given configuration
func (mp *MercadoPagoPlugin) Initialize(businessID uint, config map[string]interface{}) error {
	// Validate configuration first
	if err := mp.ValidateConfig(config); err != nil {
		return fmt.Errorf("mercadopago plugin initialization failed: %v", err)
	}

	// Test MercadoPago API connection with provided credentials, honoring the
	// configured api_base_url override rather than a hardcoded production host.
	accessToken := config["access_token"].(string)
	apiBaseURL := mercadoPagoAPIBaseURL(&MercadoPagoConfig{APIBaseURL: strings.TrimSpace(stringFromMercadoPagoAny(config["api_base_url"]))})
	if err := mp.testMercadoPagoConnection(accessToken, apiBaseURL); err != nil {
		return fmt.Errorf("mercadopago API connection test failed: %v", err)
	}

	return nil
}

// Cleanup performs any necessary cleanup when plugin is disabled
func (mp *MercadoPagoPlugin) Cleanup(businessID uint) error {
	log.Printf("Plugin cleanup called for business %d — webhook endpoints not automatically removed from MercadoPago", businessID)
	return nil
}

// ProcessPayment is unsupported for the generic MercadoPago plugin interface;
// use CreateBillPayment, which builds a Checkout Pro preference.
func (mp *MercadoPagoPlugin) ProcessPayment(businessID uint, amount int64, currency string, metadata map[string]interface{}) (string, error) {
	return "", errors.New("generic MercadoPago ProcessPayment is unsupported; use CreateBillPayment preferences")
}

// RefundPayment creates a MercadoPago refund for a captured payment. amount is
// cents; amount == 0 requests a full remaining refund from MercadoPago.
// ORD… ids from the Orders API (Point/QR) must use POST /v1/orders/{id}/refund —
// the payments-refund endpoint 404s for order ids.
func (mp *MercadoPagoPlugin) RefundPayment(businessID uint, paymentID string, amount int64) error {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return errors.New("mercadopago payment ID is required")
	}
	if amount < 0 {
		return errors.New("mercadopago refund amount cannot be negative")
	}

	config, err := mp.getConfig(businessID)
	if err != nil {
		return fmt.Errorf("failed to get MercadoPago config: %w", err)
	}

	if isMercadoPagoOrderID(paymentID) {
		return mp.refundOrder(businessID, paymentID, amount, config)
	}

	refundRequest := map[string]interface{}{}
	if amount > 0 {
		// The refund amount must be sent in the payment's currency with the
		// currency's decimal places. Zero-decimal currencies (CLP, COP) reject
		// sub-unit decimals, so resolve the currency from the payment and floor
		// accordingly rather than blindly dividing by 100.
		info, err := mp.fetchPaymentInfo(config, paymentID)
		if err != nil {
			return fmt.Errorf("failed to resolve MercadoPago payment currency for refund: %w", err)
		}
		refundRequest["amount"] = mercadoPagoMajorAmount(amount, info.CurrencyID)
	}

	idempotencyKey := plugins.PaymentRequestIdempotencyKey("mercadopago", "refund", businessID, paymentID, amount)
	response, err := mp.makeMercadoPagoAPICallWithIdempotencyKey(context.Background(), http.MethodPost, fmt.Sprintf("/v1/payments/%s/refunds", url.PathEscape(paymentID)), refundRequest, config, idempotencyKey)
	if err != nil {
		return fmt.Errorf("failed to create MercadoPago refund: %w", err)
	}
	refundID := strings.TrimSpace(stringFromMercadoPagoAny(response["id"]))
	if refundID == "" {
		return errors.New("invalid MercadoPago refund response: missing refund ID")
	}
	// A MercadoPago refund can come back rejected/cancelled — recording those as
	// complete would reverse money that was never returned. Only approved (and
	// still-settling) states count. An absent status is tolerated for minimal
	// responses (the refund id already proves the call itself succeeded).
	if status := strings.ToLower(strings.TrimSpace(stringFromMercadoPagoAny(response["status"]))); status != "" {
		switch status {
		case "approved", "pending", "in_process":
			// success or still settling
		default: // rejected, cancelled, …
			return fmt.Errorf("mercadopago refund not successful: status %s", status)
		}
	}
	return nil
}

// isMercadoPagoOrderID reports whether id is an Orders API order id (Point/QR).
// Payment ids are numeric; order ids are prefixed ORD… (see MP Orders API).
func isMercadoPagoOrderID(id string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(id)), "ORD")
}

// refundOrder POSTs /v1/orders/{id}/refund for Point/QR captures. amount is
// platform cents; amount == 0 means full remaining refund (empty body).
// Partial refunds use the verified MP contract (2026-08-10):
//
//	{"transactions":[{"id":"<PAY…>","amount":"<decimal string>"}]}
//
// Currency and payment id come from GET /v1/orders/{id}; never guess ARS.
// Multi-payment orders reject partial refunds (see buildPartialOrderRefundBody).
// Never routes ORD ids to /v1/payments/{id}/refunds (that endpoint 404s).
func (mp *MercadoPagoPlugin) refundOrder(businessID uint, orderID string, amount int64, config *MercadoPagoConfig) error {
	var body interface{}
	if amount > 0 {
		order, err := mp.fetchOrder(config, orderID)
		if err != nil {
			return fmt.Errorf("failed to fetch MercadoPago order for partial refund: %w", err)
		}
		body, err = buildPartialOrderRefundBody(order, amount)
		if err != nil {
			return err
		}
	}
	// amount == 0: body stays nil → empty POST body (full remaining refund).

	idempotencyKey := plugins.PaymentRequestIdempotencyKey("mercadopago", "order_refund", businessID, orderID, amount)
	response, err := mp.makeMercadoPagoAPICallWithIdempotencyKey(
		context.Background(),
		http.MethodPost,
		"/v1/orders/"+url.PathEscape(orderID)+"/refund",
		body,
		config,
		idempotencyKey,
	)
	if err != nil {
		return fmt.Errorf("failed to create MercadoPago order refund: %w", err)
	}
	if response == nil {
		return errors.New("invalid MercadoPago order refund response: empty body")
	}
	// Only states that prove the refund went through (or is still settling)
	// count. Anything else — failed, rejected, canceled, action_required, or no
	// status at all — leaves the payment refund_pending instead of recording
	// money that may never have been returned.
	status := strings.ToLower(strings.TrimSpace(stringFromMercadoPagoAny(response["status"])))
	switch status {
	case "refunded", "partially_refunded", "processed", "approved", "pending", "in_process":
		return nil
	case "":
		return errors.New("invalid MercadoPago order refund response: missing status")
	default:
		return fmt.Errorf("mercadopago order refund not successful: status %s", status)
	}
}

// buildPartialOrderRefundBody builds the MP Orders partial-refund JSON body.
// Policy for multi-payment orders: reject with a clear error. Point/QR charges
// in this product create a single payment line; proportional split across
// multiple PAY ids is ambiguous against available per-transaction balances
// and is not supported.
func buildPartialOrderRefundBody(order *mpOrder, amountCents int64) (map[string]interface{}, error) {
	if order == nil {
		return nil, errors.New("mercadopago: order required for partial refund")
	}
	payments := order.Transactions.Payments
	if len(payments) == 0 {
		return nil, errors.New("mercadopago: order has no payments to refund")
	}
	if len(payments) > 1 {
		return nil, errors.New("mercadopago: multi-payment order partial refund is not supported; refund the full order or each payment in Mercado Pago")
	}
	payID := strings.TrimSpace(payments[0].ID)
	if payID == "" {
		return nil, errors.New("mercadopago: order payment missing transaction id")
	}
	currency := orderCurrencyCode(order)
	if currency == "" {
		return nil, errors.New("mercadopago: order currency unknown; cannot format partial refund amount")
	}
	amountStr := mercadoPagoDecimalAmount(amountCents, currency)
	return map[string]interface{}{
		"transactions": []map[string]interface{}{
			{"id": payID, "amount": amountStr},
		},
	}, nil
}

// orderCurrencyCode prefers currency_id then currency from the order payload.
func orderCurrencyCode(order *mpOrder) string {
	if order == nil {
		return ""
	}
	if c := strings.ToUpper(strings.TrimSpace(order.CurrencyID)); c != "" {
		return c
	}
	return strings.ToUpper(strings.TrimSpace(order.Currency))
}

// Compile-time check: MercadoPago supports guest return capture (PayPal-style).
var _ plugins.PaymentReturnCapturer = (*MercadoPagoPlugin)(nil)

// CapturePaymentReturn fetches the payment by ID after Checkout Pro redirect and
// maps it to the same WebhookResponse shape as handlePaymentUpdated so the
// return handler can settle tracked payments immediately.
func (m *MercadoPagoPlugin) CapturePaymentReturn(businessID uint, billID uint, paymentID string) (*plugins.WebhookResponse, error) {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing MercadoPago payment ID",
		}, nil
	}
	config, err := m.getConfig(businessID)
	if err != nil {
		return nil, err
	}
	paymentInfo, err := m.fetchPaymentInfo(config, paymentID)
	if err != nil {
		return nil, err
	}
	refBillID, refBusinessID, ok := parseMercadoPagoBillBusinessReference(paymentInfo.ExternalReference)
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing MercadoPago external reference",
		}, nil
	}
	if refBusinessID != businessID {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "MercadoPago payment business mismatch",
		}, nil
	}
	if refBillID != billID {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "MercadoPago payment bill mismatch",
		}, nil
	}
	expectedRef := fmt.Sprintf("bill_%d_business_%d", billID, businessID)
	if strings.TrimSpace(paymentInfo.ExternalReference) != expectedRef {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "MercadoPago external reference mismatch",
		}, nil
	}
	status := mercadoPagoPaymentStatus(paymentInfo.Status)
	amountCents, err := money.Float64ToCents(paymentInfo.TransactionAmount)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction amount: %w", err)
	}
	// rejected/cancelled payments: Success false so the handler does not settle.
	if status == "cancelled" || status == "rejected" || strings.EqualFold(paymentInfo.Status, "rejected") {
		return &plugins.WebhookResponse{
			PaymentID: paymentID,
			BillID:    billID,
			Status:    status,
			Amount:    amountCents,
			Currency:  strings.ToUpper(paymentInfo.CurrencyID),
			Metadata:  paymentInfo.Metadata,
			Success:   false,
			Message:   "Payment not approved",
		}, nil
	}
	resp := &plugins.WebhookResponse{
		PaymentID:     paymentID,
		BillID:        billID,
		Status:        status,
		Amount:        amountCents,
		Currency:      strings.ToUpper(paymentInfo.CurrencyID),
		TransactionID: paymentID,
		Metadata:      paymentInfo.Metadata,
		Success:       true,
		Message:       "Payment status updated",
	}
	if status == "refunded" {
		resp.RefundedAmountCents = mercadoPagoRefundedCents(paymentInfo)
	}
	applyMercadoPagoCumulativeRefund(resp, paymentInfo)
	return resp, nil
}

// GetPaymentStatus gets the status of a payment from MercadoPago.
// ORD-prefixed IDs are Point/QR Orders API orders (via getOrder); other IDs use
// the classic payments resource. Config is resolved through the shared guarded
// constructor (sandbox/TEST- refusal in production) — the last remaining bypass.
func (mp *MercadoPagoPlugin) GetPaymentStatus(businessID uint, paymentID string) (string, error) {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return "", errors.New("mercadopago payment ID is required")
	}
	if strings.HasPrefix(strings.ToUpper(paymentID), "ORD") {
		order, err := mp.getOrder(businessID, paymentID)
		if err != nil {
			return "", err
		}
		return mercadoPagoOrderStatus(order.Status), nil
	}

	config, err := mp.getConfig(businessID)
	if err != nil {
		return "", fmt.Errorf("failed to get mercadopago config: %w", err)
	}
	return mp.getPaymentInfo(config.AccessToken, mercadoPagoAPIBaseURL(config), paymentID)
}

// MercadoPago API Integration

// PaymentPreferenceRequest represents a MercadoPago payment preference request
type PaymentPreferenceRequest struct {
	Items              []PreferenceItem `json:"items"`
	Payer              *Payer           `json:"payer,omitempty"`
	BackURLs           *BackURLs        `json:"back_urls,omitempty"`
	AutoReturn         string           `json:"auto_return,omitempty"`
	PaymentMethods     *PaymentMethods  `json:"payment_methods,omitempty"`
	NotificationURL    string           `json:"notification_url,omitempty"`
	ExternalReference  string           `json:"external_reference,omitempty"`
	Metadata           map[string]any   `json:"metadata,omitempty"`
	Expires            bool             `json:"expires,omitempty"`
	ExpirationDateFrom string           `json:"expiration_date_from,omitempty"`
	ExpirationDateTo   string           `json:"expiration_date_to,omitempty"`
}

type PreferenceItem struct {
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	CurrencyID  string  `json:"currency_id"`
}

type Payer struct {
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
}

type BackURLs struct {
	Success string `json:"success,omitempty"`
	Failure string `json:"failure,omitempty"`
	Pending string `json:"pending,omitempty"`
}

type PaymentMethods struct {
	ExcludedPaymentMethods []ExcludedPaymentMethod `json:"excluded_payment_methods,omitempty"`
	ExcludedPaymentTypes   []ExcludedPaymentType   `json:"excluded_payment_types,omitempty"`
	Installments           int                     `json:"installments,omitempty"`
}

type ExcludedPaymentMethod struct {
	ID string `json:"id"`
}

type ExcludedPaymentType struct {
	ID string `json:"id"`
}

// PaymentPreferenceResponse represents a MercadoPago payment preference response
type PaymentPreferenceResponse struct {
	ID                string `json:"id"`
	InitPoint         string `json:"init_point"`
	SandboxInitPoint  string `json:"sandbox_init_point"`
	DateCreated       string `json:"date_created"`
	OperationType     string `json:"operation_type"`
	AdditionalInfo    string `json:"additional_info"`
	ExternalReference string `json:"external_reference"`
}

// PaymentInfo represents payment information from MercadoPago
type PaymentInfo struct {
	ID                int64   `json:"id"`
	Status            string  `json:"status"`
	StatusDetail      string  `json:"status_detail"`
	OperationType     string  `json:"operation_type"`
	CurrencyID        string  `json:"currency_id"`
	TransactionAmount float64 `json:"transaction_amount"`
	// TransactionAmountRefunded is the cumulative amount refunded so far. It
	// lets a partial refund/chargeback be detected from data rather than assumed
	// to be a full reversal.
	TransactionAmountRefunded float64        `json:"transaction_amount_refunded"`
	DateCreated               string         `json:"date_created"`
	DateLastUpdated           string         `json:"date_last_updated"`
	ExternalReference         string         `json:"external_reference"`
	Description               string         `json:"description"`
	PaymentMethodID           string         `json:"payment_method_id"`
	PaymentTypeID             string         `json:"payment_type_id"`
	Metadata                  map[string]any `json:"metadata"`
}

// Helper functions for MercadoPago API integration

func (mp *MercadoPagoPlugin) isValidAccessToken(token string) bool {
	return len(token) > 20 && (strings.HasPrefix(token, "APP_USR-") || strings.HasPrefix(token, "TEST-"))
}

func (mp *MercadoPagoPlugin) isValidPublicKey(key string) bool {
	return len(key) > 20 && (strings.HasPrefix(key, "APP_USR-") || strings.HasPrefix(key, "TEST-"))
}

// resolveMercadoPagoEnvironment returns sandbox or production.
//
// In production mode, APP_USR- is authoritative over a mislabeled
// environment=sandbox (that mislabel used to hard-fail every webhook/refund via
// refuseSandboxConfigInProduction). Outside production, APP_USR + sandbox is a
// valid Mercado Pago test-app setup and the label is honored.
// Genuine TEST- tokens always resolve as sandbox. Defaults to production.
func resolveMercadoPagoEnvironment(config map[string]interface{}) string {
	token, _ := config["access_token"].(string)
	token = strings.TrimSpace(token)
	label := ""
	if env, _ := config["environment"].(string); env != "" {
		switch strings.ToLower(strings.TrimSpace(env)) {
		case "sandbox":
			label = "sandbox"
		case "production":
			label = "production"
		}
	}
	if strings.HasPrefix(token, "TEST-") {
		return "sandbox"
	}
	if appconfig.IsProductionMode(false) && strings.HasPrefix(token, "APP_USR-") {
		if label == "sandbox" {
			log.Printf("MercadoPago: access_token is APP_USR- but environment=sandbox in production; treating as production")
		}
		return "production"
	}
	if label != "" {
		return label
	}
	if strings.HasPrefix(token, "APP_USR-") {
		return "production"
	}
	return "production"
}

func (mp *MercadoPagoPlugin) testMercadoPagoConnection(accessToken, apiBaseURL string) error {
	// Test API connection by getting user information against the configured host.
	url := strings.TrimRight(strings.TrimSpace(apiBaseURL), "/") + "/users/me"

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API test failed with status: %d", resp.StatusCode)
	}

	return nil
}

func (mp *MercadoPagoPlugin) getPaymentInfo(accessToken, apiBaseURL, paymentID string) (string, error) {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return "", errors.New("payment ID is required")
	}
	paymentURL := fmt.Sprintf("%s/v1/payments/%s", strings.TrimRight(strings.TrimSpace(apiBaseURL), "/"), url.PathEscape(paymentID))
	req, err := http.NewRequestWithContext(context.Background(), "GET", paymentURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var paymentInfo PaymentInfo
	if err := json.Unmarshal(body, &paymentInfo); err != nil {
		return "", err
	}

	return paymentInfo.Status, nil
}

// init registers the MercadoPago plugin with the global registry
func init() {
	plugins.RegisterPluginInitializer("mercadopago", func(pluginService *services.PluginService) {
		mercadoPagoPlugin := NewMercadoPagoPlugin(pluginService)
		plugins.GlobalRegistry.RegisterPlugin(mercadoPagoPlugin)
	})
}

func mercadoPagoBackURLs(metadata map[string]interface{}) (*BackURLs, error) {
	if metadata == nil {
		return nil, errors.New("return_url and cancel_url are required")
	}
	successURL, _ := metadata["return_url"].(string)
	cancelURL, _ := metadata["cancel_url"].(string)
	successURL = strings.TrimSpace(successURL)
	cancelURL = strings.TrimSpace(cancelURL)
	if successURL == "" || cancelURL == "" {
		return nil, errors.New("return_url and cancel_url are required")
	}
	return &BackURLs{
		Success: successURL,
		Failure: cancelURL,
		Pending: cancelURL,
	}, nil
}

// CreateBillPayment creates a MercadoPago Checkout Pro preference for a specific bill
func (m *MercadoPagoPlugin) CreateBillPayment(businessID uint, billID uint, amount int64, currency string, metadata map[string]interface{}) (*plugins.PaymentResponse, error) {
	config, err := m.getConfig(businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to get MercadoPago config: %w", err)
	}
	if err := refuseSandboxPaymentInProduction(config.Environment); err != nil {
		return nil, err
	}

	// Canonicalize once so the preference major-unit amount and the tracker
	// amount_cents match for zero-decimal currencies (CLP/COP floor to whole
	// major units). Guest initiation already rejects non-representable amounts
	// via currencyCanRepresentCents; this keeps tracker/preference aligned.
	canonicalCents := CanonicalAmountCents(amount, currency)
	if canonicalCents <= 0 {
		return nil, errors.New("charge amount must be at least one major unit for this currency")
	}

	// Convert amount from stored cents to MercadoPago's major-unit amount,
	// honoring the currency's decimal places so zero-decimal currencies (CLP,
	// COP) are sent as whole numbers rather than sub-unit decimals MercadoPago
	// rejects.
	amountFloat := mercadoPagoMajorAmount(canonicalCents, currency)

	backURLs, err := mercadoPagoBackURLs(metadata)
	if err != nil {
		return nil, err
	}
	// Point success/pending back to the backend return-capture route so the
	// guest is confirmed instantly; keep failure on the guest bill page.
	if returnCapture, err := mercadoPagoReturnCaptureURL(config, businessID, billID); err == nil {
		backURLs.Success = returnCapture
		backURLs.Pending = returnCapture
	}
	notificationURL, err := mercadoPagoWebhookURL(config, businessID)
	if err != nil {
		return nil, err
	}
	trackerID := "mp_tracker_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	preferenceMetadata := make(map[string]any)
	for key, value := range metadata {
		if key == "return_url" || key == "cancel_url" {
			continue
		}
		preferenceMetadata[key] = value
	}
	preferenceMetadata["payverge_tracker_id"] = trackerID

	// Create MercadoPago Checkout Pro preference request
	preferenceRequest := PaymentPreferenceRequest{
		Items: []PreferenceItem{
			{
				Title:       fmt.Sprintf("Bill #%d Payment", billID),
				Description: fmt.Sprintf("Payment for Bill #%d", billID),
				Quantity:    1,
				UnitPrice:   amountFloat,
				CurrencyID:  currency,
			},
		},
		BackURLs:          backURLs,
		AutoReturn:        config.AutoReturn,
		PaymentMethods:    &PaymentMethods{Installments: config.Installments},
		ExternalReference: fmt.Sprintf("bill_%d_business_%d", billID, businessID),
		NotificationURL:   notificationURL,
		Metadata:          preferenceMetadata,
	}

	// Make API call to MercadoPago Checkout Pro
	idempotencyKey := plugins.PaymentRequestIdempotencyKey("mercadopago", "create_bill_payment", businessID, billID, canonicalCents, strings.ToUpper(currency))
	response, err := m.makeMercadoPagoAPICallWithIdempotencyKey(context.Background(), "POST", "/checkout/preferences", preferenceRequest, config, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create MercadoPago preference: %w", err)
	}

	// Extract preference details
	preferenceID, ok := response["id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid MercadoPago response: missing preference ID")
	}

	// Get the checkout URL - use sandbox or production based on config
	var checkoutURL string
	if config.Environment == "sandbox" {
		if sandboxURL, ok := response["sandbox_init_point"].(string); ok {
			checkoutURL = sandboxURL
		}
	} else {
		if initURL, ok := response["init_point"].(string); ok {
			checkoutURL = initURL
		}
	}

	if checkoutURL == "" {
		return nil, fmt.Errorf("invalid MercadoPago response: missing checkout URL")
	}

	return &plugins.PaymentResponse{
		PaymentID:   trackerID,
		PaymentURL:  checkoutURL,
		RedirectURL: checkoutURL,
		Status:      "pending",
		Metadata: map[string]interface{}{
			"preference_id":       preferenceID,
			"payverge_tracker_id": trackerID,
			"bill_id":             billID,
			"business_id":         businessID,
			// amount_cents is the same canonical value the preference charged so
			// AlternativePayment.Amount matches webhook settlement for CLP/COP.
			"amount_cents": canonicalCents,
		},
	}, nil
}

// HandleWebhook processes MercadoPago webhook callbacks
func (m *MercadoPagoPlugin) HandleWebhook(businessID uint, payload []byte, headers map[string]string) (*plugins.WebhookResponse, error) {
	// Parse webhook payload
	var webhookData map[string]interface{}
	if err := json.Unmarshal(payload, &webhookData); err != nil {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook payload",
		}, nil
	}

	// Extract action and data
	action, ok := webhookData["action"].(string)
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing action",
		}, nil
	}

	// Handle payment updates (Checkout Pro / Payments API)
	if action == "payment.updated" {
		config, err := m.getConfig(businessID)
		if err != nil {
			return &plugins.WebhookResponse{
				Success: false,
				Message: "MercadoPago configuration unavailable",
			}, err
		}
		return m.handlePaymentUpdated(webhookData, config)
	}

	// Orders topic (Point / QR): fetch authoritative order; never trust payload status/amount.
	if strings.HasPrefix(action, "order.") {
		if !isSupportedMercadoPagoOrderWebhookAction(action) {
			log.Printf("Unsupported MercadoPago order webhook action %q for business %d", action, businessID)
			metrics.PaymentWebhookUnsupportedActions.WithLabelValues(m.GetName(), action).Inc()
			return &plugins.WebhookResponse{
				Success: true,
				Message: fmt.Sprintf("Unsupported MercadoPago webhook action: %s", action),
			}, nil
		}
		config, err := m.getConfig(businessID)
		if err != nil {
			return &plugins.WebhookResponse{
				Success: false,
				Message: "MercadoPago configuration unavailable",
			}, err
		}
		return m.handleOrderUpdated(businessID, webhookData, config)
	}

	log.Printf("Unsupported MercadoPago webhook action %q for business %d", action, businessID)
	metrics.PaymentWebhookUnsupportedActions.WithLabelValues(m.GetName(), action).Inc()

	return &plugins.WebhookResponse{
		Success: true,
		Message: fmt.Sprintf("Unsupported MercadoPago webhook action: %s", action),
	}, nil
}

// isSupportedMercadoPagoOrderWebhookAction reports whether the orders-topic
// action should be settled (via GET /v1/orders) rather than acknowledged as a no-op.
func isSupportedMercadoPagoOrderWebhookAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "order.processed",
		"order.canceled",
		"order.cancelled",
		"order.refunded",
		"order.expired",
		"order.failed",
		"order.action_required":
		return true
	default:
		return false
	}
}

// GetWebhookEndpoint returns the webhook endpoint path for MercadoPago
func (m *MercadoPagoPlugin) GetWebhookEndpoint() string {
	return "/api/v1/webhooks/mercadopago"
}

// mercadoPagoWebhookClockSkewLimit is the maximum allowed age of a webhook timestamp.
const mercadoPagoWebhookClockSkewLimit = 5 * time.Minute

// VerifyWebhookSignature is retained for the generic PaymentPlugin interface.
// MercadoPago's official signature uses the request query data.id and
// x-request-id header, so callers should use VerifyWebhookRequest.
func (m *MercadoPagoPlugin) VerifyWebhookSignature(payload []byte, signature string, secret string) bool {
	return false
}

func (m *MercadoPagoPlugin) VerifyWebhookRequest(request plugins.WebhookVerificationRequest) error {
	// Prefer business-config secrets, then fall back to the platform app secret
	// (MERCADOPAGO_WEBHOOK_SECRET). OAuth-connected merchants typically have no
	// per-business webhook_secret; MP signs with the application secret from the
	// developer panel.
	secrets := make([]string, 0, 4)
	appendMPWebhookSecret := func(raw string) {
		s := strings.TrimSpace(raw)
		if s == "" {
			return
		}
		for _, existing := range secrets {
			if existing == s {
				return
			}
		}
		secrets = append(secrets, s)
	}
	appendMPWebhookSecret(stringFromMercadoPagoAny(request.Config["webhook_secret"]))
	appendMPWebhookSecret(stringFromMercadoPagoAny(request.Config["webhook_secret_previous"]))
	appendMPWebhookSecret(os.Getenv("MERCADOPAGO_WEBHOOK_SECRET"))
	appendMPWebhookSecret(os.Getenv("MERCADOPAGO_WEBHOOK_SECRET_PREVIOUS"))
	if len(secrets) == 0 {
		return errors.New("MercadoPago webhook_secret not configured")
	}
	signature := mercadoPagoHeader(request.Headers, "x-signature")
	requestID := mercadoPagoHeader(request.Headers, "x-request-id")
	if signature == "" {
		return errors.New("missing x-signature header")
	}
	if requestID == "" {
		return errors.New("missing x-request-id header")
	}

	timestamp, signatures, err := parseMercadoPagoSignature(signature)
	if err != nil {
		return err
	}
	if timestamp == "" || len(signatures) == 0 {
		return errors.New("invalid MercadoPago signature header")
	}
	if err := validateMercadoPagoTimestamp(timestamp); err != nil {
		return err
	}

	dataID := request.Query.Get("data.id")
	if dataID == "" {
		dataID = request.Query.Get("id")
	}
	if dataID == "" {
		dataID = mercadoPagoDataIDFromPayload(request.Payload)
	}
	if dataID == "" {
		return errors.New("missing MercadoPago data.id")
	}
	dataID = mercadoPagoSignatureDataID(dataID)

	manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%s;", dataID, requestID, timestamp)
	for _, secret := range secrets {
		if secret != "" && webhookhmac.VerifyHexSHA256(secret, []byte(manifest), signatures) {
			return nil
		}
	}
	return errors.New("MercadoPago webhook signature verification failed")
}

func mercadoPagoSignatureDataID(dataID string) string {
	return strings.ToLower(strings.TrimSpace(dataID))
}

func parseMercadoPagoSignature(signature string) (string, []string, error) {
	var timestamp string
	signatures := make([]string, 0)
	for _, part := range strings.Split(signature, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		switch key {
		case "ts":
			timestamp = value
		case "v1":
			if value != "" {
				signatures = append(signatures, value)
			}
		}
	}
	if timestamp == "" {
		return "", nil, errors.New("missing MercadoPago signature timestamp")
	}
	return timestamp, signatures, nil
}

func validateMercadoPagoTimestamp(timestamp string) error {
	parsed, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid MercadoPago timestamp: %w", err)
	}
	var sentAt time.Time
	if parsed > 1_000_000_000_000 {
		sentAt = time.UnixMilli(parsed)
	} else {
		sentAt = time.Unix(parsed, 0)
	}
	delta := time.Since(sentAt)
	if delta > mercadoPagoWebhookClockSkewLimit || delta < -mercadoPagoWebhookClockSkewLimit {
		return errors.New("MercadoPago webhook timestamp outside tolerance")
	}
	return nil
}

func mercadoPagoDataIDFromPayload(payload []byte) string {
	var webhookData map[string]interface{}
	if err := json.Unmarshal(payload, &webhookData); err != nil {
		return ""
	}
	if data, ok := webhookData["data"].(map[string]interface{}); ok {
		return stringFromMercadoPagoAny(data["id"])
	}
	return stringFromMercadoPagoAny(webhookData["id"])
}

// Helper method to get MercadoPago configuration for a business
func (m *MercadoPagoPlugin) getConfig(businessID uint) (*MercadoPagoConfig, error) {
	if m == nil || m.pluginService == nil {
		return nil, errors.New("mercadopago plugin service not configured")
	}

	config, err := m.pluginService.GetPluginConfig(businessID, "mercadopago")
	if err != nil {
		return nil, fmt.Errorf("failed to get MercadoPago config: %w", err)
	}

	return m.configFromMap(config)
}

// configFromMap builds MercadoPagoConfig from a raw plugin config map.
// Missing environment is derived (TEST- token → sandbox, else production).
// In production mode, sandbox/TEST- configs are refused here so every path that
// resolves config (Point, QR, instore, capture, refund, Checkout Pro) is gated.
func (m *MercadoPagoPlugin) configFromMap(config map[string]interface{}) (*MercadoPagoConfig, error) {
	if config == nil {
		return nil, errors.New("MercadoPago config is empty")
	}

	accessToken, ok := config["access_token"].(string)
	if !ok || accessToken == "" {
		return nil, errors.New("MercadoPago access_token not configured")
	}

	publicKey, _ := config["public_key"].(string)
	connectionMode, _ := config["connection_mode"].(string)
	oauthMode := strings.EqualFold(strings.TrimSpace(connectionMode), "oauth")
	if !oauthMode && strings.TrimSpace(publicKey) == "" {
		return nil, errors.New("MercadoPago public_key not configured")
	}

	baseURL, _ := config["base_url"].(string)
	apiBaseURL, _ := config["api_base_url"].(string)
	webhookSecret, _ := config["webhook_secret"].(string)
	environment := resolveMercadoPagoEnvironment(config)
	if err := refuseSandboxConfigInProduction(environment, accessToken, publicKey); err != nil {
		return nil, err
	}

	return &MercadoPagoConfig{
		AccessToken:   accessToken,
		PublicKey:     publicKey,
		Environment:   environment,
		BaseURL:       strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIBaseURL:    strings.TrimRight(strings.TrimSpace(apiBaseURL), "/"),
		WebhookSecret: strings.TrimSpace(webhookSecret),
		AutoReturn:    normalizedAutoReturn(config),
		Installments:  configInstallments(config),
	}, nil
}

// MercadoPagoConfig represents MercadoPago configuration
type MercadoPagoConfig struct {
	AccessToken   string
	PublicKey     string
	Environment   string
	BaseURL       string
	APIBaseURL    string
	WebhookSecret string
	AutoReturn    string
	Installments  int
}

// normalizedAutoReturn returns "all" only when config auto_return is "all"; else "approved".
func normalizedAutoReturn(config map[string]interface{}) string {
	if config == nil {
		return "approved"
	}
	if v, _ := config["auto_return"].(string); strings.EqualFold(strings.TrimSpace(v), "all") {
		return "all"
	}
	return "approved"
}

// configInstallments parses installments from string/float/int config, default 12, clamp 1..24.
func configInstallments(config map[string]interface{}) int {
	const defaultInstallments = 12
	if config == nil {
		return defaultInstallments
	}
	raw, ok := config["installments"]
	if !ok || raw == nil {
		return defaultInstallments
	}
	var n int
	switch v := raw.(type) {
	case int:
		n = v
	case int64:
		n = int(v)
	case float64:
		n = int(v)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return defaultInstallments
		}
		n = parsed
	default:
		return defaultInstallments
	}
	if n < 1 {
		return 1
	}
	if n > 24 {
		return 24
	}
	return n
}

func mercadoPagoAPIBaseURL(config *MercadoPagoConfig) string {
	if config != nil && strings.TrimSpace(config.APIBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(config.APIBaseURL), "/")
	}
	return "https://api.mercadopago.com"
}

// callbackBaseURL resolves the public backend origin used to build the
// MercadoPago IPN/webhook and return callbacks. Package var so tests override
// without env mutation. It is always the deployment's own origin
// (APP_BASE_URL, else PUBLIC_URL, else the local development URL); the
// per-business base_url is ignored so a tenant cannot redirect its guests to
// an arbitrary host and staging never points IPNs at production.
var callbackBaseURL = appconfig.APIBaseURL

func mercadoPagoCallbackBaseURL(_ *MercadoPagoConfig) string {
	return strings.TrimRight(strings.TrimSpace(callbackBaseURL()), "/")
}

// Helper method to make MercadoPago API calls
func (m *MercadoPagoPlugin) makeMercadoPagoAPICall(ctx context.Context, method, endpoint string, payload interface{}, config *MercadoPagoConfig) (map[string]interface{}, error) {
	return m.makeMercadoPagoAPICallWithIdempotencyKey(ctx, method, endpoint, payload, config, "")
}

func (m *MercadoPagoPlugin) makeMercadoPagoAPICallWithIdempotencyKey(ctx context.Context, method, endpoint string, payload interface{}, config *MercadoPagoConfig, idempotencyKey string) (map[string]interface{}, error) {
	baseURL := mercadoPagoAPIBaseURL(config)

	// Prepare request body
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
	req.Header.Set("Authorization", "Bearer "+config.AccessToken)
	if idempotencyKey != "" {
		req.Header.Set("X-Idempotency-Key", idempotencyKey)
	}

	// Charge/create paths hold a per-bill lock; keep HTTP well under pool
	// pressure while still above typical MP create-order latency (Point/QR).
	// Status/refund calls also share this client — 15s is ample for those.
	client := &http.Client{Timeout: 15 * time.Second}
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
		return nil, fmt.Errorf("MercadoPago API error %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (m *MercadoPagoPlugin) fetchPaymentInfo(config *MercadoPagoConfig, paymentID string) (*PaymentInfo, error) {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return nil, errors.New("payment ID is required")
	}
	paymentURL := fmt.Sprintf("%s/v1/payments/%s", mercadoPagoAPIBaseURL(config), url.PathEscape(paymentID))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, paymentURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+config.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MercadoPago payment lookup failed with status %d: %s", resp.StatusCode, string(body))
	}

	var paymentInfo PaymentInfo
	if err := json.Unmarshal(body, &paymentInfo); err != nil {
		return nil, err
	}
	return &paymentInfo, nil
}

func mercadoPagoWebhookURL(config *MercadoPagoConfig, businessID uint) (string, error) {
	base := mercadoPagoCallbackBaseURL(config)
	if base == "" {
		return "", errors.New("mercadopago callback base URL is required (set APP_BASE_URL or base_url)")
	}
	parsedBase, err := url.Parse(base)
	if err != nil || parsedBase.Scheme == "" || parsedBase.Host == "" {
		return "", errors.New("mercadopago callback base URL must be an absolute URL")
	}
	webhookURL := strings.TrimRight(base, "/") + "/api/v1/webhooks/mercadopago"
	parsed, err := url.Parse(webhookURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("business_id", strconv.FormatUint(uint64(businessID), 10))
	query.Set("source_news", "webhooks")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// mercadoPagoReturnCaptureURL is the Checkout Pro back_urls success/pending target.
func mercadoPagoReturnCaptureURL(config *MercadoPagoConfig, businessID, billID uint) (string, error) {
	base := mercadoPagoCallbackBaseURL(config)
	if base == "" {
		return "", errors.New("mercadopago callback base URL is required (set APP_BASE_URL or base_url)")
	}
	parsedBase, err := url.Parse(base)
	if err != nil || parsedBase.Scheme == "" || parsedBase.Host == "" {
		return "", errors.New("mercadopago callback base URL must be an absolute URL")
	}
	returnURL := strings.TrimRight(base, "/") + "/api/v1/webhooks/mercadopago/return"
	parsed, err := url.Parse(returnURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("business_id", strconv.FormatUint(uint64(businessID), 10))
	query.Set("bill_id", strconv.FormatUint(uint64(billID), 10))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func mercadoPagoHeader(headers map[string]string, key string) string {
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

func stringFromMercadoPagoAny(value interface{}) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case float32:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func parseMercadoPagoBillBusinessReference(reference string) (uint, uint, bool) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return 0, 0, false
	}
	parts := strings.Split(reference, "_")
	if len(parts) < 2 || parts[0] != "bill" {
		return 0, 0, false
	}
	billID, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return 0, 0, false
	}
	var businessID uint64
	if len(parts) >= 4 && parts[2] == "business" {
		if parsedBusinessID, err := strconv.ParseUint(parts[3], 10, 32); err == nil {
			businessID = parsedBusinessID
		}
	}
	return uint(billID), uint(businessID), true
}

func mercadoPagoPaymentStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "approved", "accredited":
		return "completed"
	case "refunded":
		return "refunded"
	case "charged_back":
		return "reversed"
	case "in_mediation":
		return "disputed"
	case "rejected", "cancelled":
		return "cancelled"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

// Helper method to handle payment updated webhook
func (m *MercadoPagoPlugin) handlePaymentUpdated(webhookData map[string]interface{}, config *MercadoPagoConfig) (*plugins.WebhookResponse, error) {
	data, ok := webhookData["data"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook data",
		}, nil
	}

	paymentID := stringFromMercadoPagoAny(data["id"])
	if paymentID == "" {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing MercadoPago payment ID",
		}, nil
	}

	paymentInfo, err := m.fetchPaymentInfo(config, paymentID)
	if err != nil {
		return nil, err
	}
	billID, _, ok := parseMercadoPagoBillBusinessReference(paymentInfo.ExternalReference)
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing MercadoPago external reference",
		}, nil
	}
	status := mercadoPagoPaymentStatus(paymentInfo.Status)
	amountCents, err := money.Float64ToCents(paymentInfo.TransactionAmount)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction amount: %w", err)
	}
	resp := &plugins.WebhookResponse{
		PaymentID:     paymentID,
		BillID:        billID,
		Status:        status,
		Amount:        amountCents,
		Currency:      strings.ToUpper(paymentInfo.CurrencyID),
		TransactionID: paymentID,
		Metadata:      paymentInfo.Metadata,
		Success:       true,
		Message:       "Payment status updated",
	}
	// MercadoPago's webhook Amount/TransactionAmount is the ORIGINAL CAPTURE, not
	// the refunded portion. On a refund, surface the cumulative
	// refunded amount (transaction_amount_refunded) so isPartialPluginRefund can
	// distinguish a partial from a full reversal from data rather than assuming
	// full — which would over-reverse a partial chargeback.
	if status == "refunded" {
		resp.RefundedAmountCents = mercadoPagoRefundedCents(paymentInfo)
	}
	applyMercadoPagoCumulativeRefund(resp, paymentInfo)
	return resp, nil
}

// handleOrderUpdated processes orders-topic webhooks (Point / in-store QR).
// Always GET /v1/orders/{id}; never trust payload status/amount for settlement.
// Zero commission: this path never attaches marketplace_fee (Orders client
// already omits fee fields on create).
func (m *MercadoPagoPlugin) handleOrderUpdated(businessID uint, webhookData map[string]interface{}, config *MercadoPagoConfig) (*plugins.WebhookResponse, error) {
	data, ok := webhookData["data"].(map[string]interface{})
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Invalid webhook data",
		}, nil
	}

	orderID := stringFromMercadoPagoAny(data["id"])
	if orderID == "" {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing MercadoPago order ID",
		}, nil
	}

	order, err := m.fetchOrder(config, orderID)
	if err != nil {
		return nil, err
	}

	billID, refBusinessID, ok := parseMercadoPagoBillBusinessReference(order.ExternalReference)
	if !ok {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "Missing MercadoPago external reference",
		}, nil
	}
	if businessID > 0 && refBusinessID > 0 && refBusinessID != businessID {
		return &plugins.WebhookResponse{
			Success: false,
			Message: "MercadoPago order business mismatch",
		}, nil
	}

	status := mercadoPagoOrderStatus(order.Status)
	amountCents, err := mercadoPagoOrderAmountCents(order)
	if err != nil {
		return nil, err
	}
	currency := strings.ToUpper(strings.TrimSpace(order.CurrencyID))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(order.Currency))
	}

	return &plugins.WebhookResponse{
		PaymentID:     order.ID,
		BillID:        billID,
		Status:        status,
		Amount:        amountCents,
		Currency:      currency,
		TransactionID: order.ID,
		Metadata: map[string]interface{}{
			// Tracker rows for Point/QR store ParticipantAddr = order id.
			"payverge_tracker_id": order.ID,
		},
		Success: true,
		Message: "Order status updated",
	}, nil
}

// fetchOrder GETs /v1/orders/{id} using the supplied config (webhook path).
func (m *MercadoPagoPlugin) fetchOrder(config *MercadoPagoConfig, orderID string) (*mpOrder, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, errors.New("mercadopago: order id is required")
	}
	if config == nil {
		return nil, errors.New("mercadopago: config is required")
	}
	result, err := m.makeMercadoPagoAPICallWithIdempotencyKey(
		context.Background(),
		http.MethodGet,
		"/v1/orders/"+url.PathEscape(orderID),
		nil,
		config,
		"",
	)
	if err != nil {
		return nil, mapOrdersAPIError(err, false)
	}
	return decodeMPOrder(result)
}

// mercadoPagoOrderAmountCents reads transactions.payments[0].amount (decimal string).
func mercadoPagoOrderAmountCents(order *mpOrder) (int64, error) {
	if order == nil || len(order.Transactions.Payments) == 0 {
		return 0, nil
	}
	raw := strings.TrimSpace(order.Transactions.Payments[0].Amount)
	if raw == "" {
		return 0, nil
	}
	cents, err := money.ParseDollarStringToCents(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid order payment amount %q: %w", raw, err)
	}
	return cents, nil
}

// mercadoPagoRefundedCents returns the cumulative refunded amount in cents when
// the payment carries one, or nil when nothing is recorded as refunded (callers
// then fall back to the legacy full-reversal heuristic).
func mercadoPagoRefundedCents(info *PaymentInfo) *int64 {
	if info == nil || info.TransactionAmountRefunded <= 0 {
		return nil
	}
	cents, err := money.Float64ToCents(info.TransactionAmountRefunded)
	if err != nil || cents <= 0 {
		return nil
	}
	return &cents
}

// applyMercadoPagoCumulativeRefund declares transaction_amount_refunded as the
// cumulative refunded total. A partial refund keeps the payment "approved"
// (status_detail partially_refunded), so without this the handler never saw
// it; with it the handler reverses only the not-yet-applied delta, on both
// "completed" and "refunded" responses.
func applyMercadoPagoCumulativeRefund(resp *plugins.WebhookResponse, info *PaymentInfo) {
	if resp == nil || (resp.Status != "completed" && resp.Status != "refunded") {
		return
	}
	resp.RefundedCumulativeCents = mercadoPagoRefundedCents(info)
}

// Ensure MercadoPagoPlugin implements PaymentPlugin interface
var _ plugins.PaymentPlugin = (*MercadoPagoPlugin)(nil)

// Ensure MercadoPagoPlugin implements WebhookRequestVerifier. Its
// VerifyWebhookSignature intentionally returns false (real verification uses the
// request query data.id + x-request-id via VerifyWebhookRequest). The webhook
// handler routes to VerifyWebhookRequest only when this interface is satisfied;
// if the method signature ever drifted, the type assertion would silently fail
// and every MercadoPago webhook would be rejected. This assertion makes that
// drift a build error.
var _ plugins.WebhookRequestVerifier = (*MercadoPagoPlugin)(nil)
