package mercadopago

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Typed errors for Orders / Point terminal flows.
var (
	// ErrTerminalBusy is returned when a Point create-order call gets HTTP 409
	// (terminal already has an open order / is busy).
	ErrTerminalBusy = errors.New("mercadopago: terminal busy")
	// ErrForbidden is returned when the Orders API rejects the call with 403.
	ErrForbidden = errors.New("mercadopago: forbidden")
	// ErrUsersMeFailed is returned when GET /users/me cannot resolve the seller
	// user id (bad/expired token, network, empty response).
	ErrUsersMeFailed = errors.New("mercadopago: users/me failed")
	// ErrInsufficientTokenScope is returned when store/POS provisioning fails
	// because the access token lacks the required offline/in-store scopes.
	ErrInsufficientTokenScope = errors.New("mercadopago: insufficient token scope for store/POS provisioning")
)

// mpOrderRequest is the body for POST /v1/orders (Point or QR).
// Never include marketplace_fee / platform fee fields (zero-commission).
type mpOrderRequest struct {
	Type              string              `json:"type"` // "point" | "qr"
	ExternalReference string              `json:"external_reference"`
	ExpirationTime    string              `json:"expiration_time,omitempty"` // ISO8601 duration, e.g. "PT30M"
	Description       string              `json:"description,omitempty"`
	Transactions      mpOrderTransactions `json:"transactions"`
	Config            mpOrderConfig       `json:"config"`
}

type mpOrderTransactions struct {
	Payments []mpOrderPayment `json:"payments"`
}

type mpOrderPayment struct {
	// ID is the payment/transaction id (PAY…) returned on GET after capture.
	// Required for partial order refunds: POST /v1/orders/{id}/refund body.
	ID     string `json:"id,omitempty"`
	Amount string `json:"amount"` // decimal string, e.g. "1500.00"
}

type mpOrderConfig struct {
	Point *mpPointConfig `json:"point,omitempty"`
	QR    *mpQRConfig    `json:"qr,omitempty"`
}

type mpPointConfig struct {
	TerminalID      string `json:"terminal_id"`
	PrintOnTerminal string `json:"print_on_terminal,omitempty"`
}

type mpQRConfig struct {
	ExternalPOSID string `json:"external_pos_id"`
	Mode          string `json:"mode"` // "dynamic"
}

// mpOrder is the Orders API response (create/get).
type mpOrder struct {
	ID                string `json:"id"`     // "ORD..."
	Status            string `json:"status"` // created|processed|canceled|refunded|expired|action_required|failed
	ExternalReference string `json:"external_reference"`
	// Currency fields are optional on create responses; webhook settlement prefers
	// currency_id and falls back to currency when present on GET /v1/orders.
	CurrencyID   string `json:"currency_id,omitempty"`
	Currency     string `json:"currency,omitempty"`
	TypeResponse struct {
		QRData string `json:"qr_data"`
	} `json:"type_response"`
	Transactions mpOrderTransactions `json:"transactions"`
}

// mpTerminal is one row from GET /terminals/v1/list → data.terminals[].
type mpTerminal struct {
	ID            string `json:"id"`
	PosID         string `json:"pos_id"`
	StoreID       string `json:"store_id"`
	ExternalPosID string `json:"external_pos_id"`
	OperatingMode string `json:"operating_mode"` // PDV | STANDALONE | UNDEFINED
}

// createOrder POSTs /v1/orders. Always send X-Idempotency-Key (caller supplies).
// Never attaches marketplace/platform fees.
func (m *MercadoPagoPlugin) createOrder(businessID uint, req mpOrderRequest, idemKey string) (*mpOrder, error) {
	if strings.TrimSpace(idemKey) == "" {
		return nil, errors.New("mercadopago: X-Idempotency-Key is required for createOrder")
	}
	config, err := m.getConfig(businessID)
	if err != nil {
		return nil, err
	}

	result, err := m.makeMercadoPagoAPICallWithIdempotencyKey(
		context.Background(),
		http.MethodPost,
		"/v1/orders",
		req,
		config,
		idemKey,
	)
	if err != nil {
		return nil, mapOrdersAPIError(err, strings.EqualFold(req.Type, "point"))
	}
	return decodeMPOrder(result)
}

// getOrder GETs /v1/orders/{id}.
func (m *MercadoPagoPlugin) getOrder(businessID uint, orderID string) (*mpOrder, error) {
	config, err := m.getConfig(businessID)
	if err != nil {
		return nil, err
	}
	return m.fetchOrder(config, orderID)
}

// cancelOrder POSTs /v1/orders/{id}/cancel.
func (m *MercadoPagoPlugin) cancelOrder(businessID uint, orderID string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return errors.New("mercadopago: order id is required")
	}
	config, err := m.getConfig(businessID)
	if err != nil {
		return err
	}

	_, err = m.makeMercadoPagoAPICallWithIdempotencyKey(
		context.Background(),
		http.MethodPost,
		"/v1/orders/"+url.PathEscape(orderID)+"/cancel",
		nil,
		config,
		uuid.NewString(),
	)
	if err != nil {
		return mapOrdersAPIError(err, false)
	}
	return nil
}

// listTerminals GETs /terminals/v1/list and returns data.terminals[].
func (m *MercadoPagoPlugin) listTerminals(businessID uint) ([]mpTerminal, error) {
	config, err := m.getConfig(businessID)
	if err != nil {
		return nil, err
	}

	result, err := m.makeMercadoPagoAPICallWithIdempotencyKey(
		context.Background(),
		http.MethodGet,
		"/terminals/v1/list",
		nil,
		config,
		"",
	)
	if err != nil {
		return nil, mapOrdersAPIError(err, false)
	}

	data, _ := result["data"].(map[string]interface{})
	if data == nil {
		return nil, errors.New("mercadopago: terminals list missing data")
	}
	raw, ok := data["terminals"]
	if !ok || raw == nil {
		return []mpTerminal{}, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("mercadopago: marshal terminals: %w", err)
	}
	var terminals []mpTerminal
	if err := json.Unmarshal(b, &terminals); err != nil {
		return nil, fmt.Errorf("mercadopago: parse terminals: %w", err)
	}
	return terminals, nil
}

// setTerminalMode PATCHes /terminals/v1/setup with {terminals:[{id, operating_mode}]}.
// Mode is typically "PDV" (integration) or "STANDALONE". Terminal may reboot.
func (m *MercadoPagoPlugin) setTerminalMode(businessID uint, terminalID, mode string) error {
	terminalID = strings.TrimSpace(terminalID)
	mode = strings.TrimSpace(mode)
	if terminalID == "" {
		return errors.New("mercadopago: terminal id is required")
	}
	if mode == "" {
		return errors.New("mercadopago: operating mode is required")
	}
	config, err := m.getConfig(businessID)
	if err != nil {
		return err
	}

	body := map[string]interface{}{
		"terminals": []map[string]string{
			{"id": terminalID, "operating_mode": mode},
		},
	}
	_, err = m.makeMercadoPagoAPICallWithIdempotencyKey(
		context.Background(),
		http.MethodPatch,
		"/terminals/v1/setup",
		body,
		config,
		uuid.NewString(),
	)
	if err != nil {
		return mapOrdersAPIError(err, false)
	}
	return nil
}

// mercadoPagoOrderStatus maps Mercado Pago order status strings to Payverge
// payment statuses used by the plugin settlement path.
//
//	processed        → completed
//	canceled/failed  → cancelled
//	refunded         → refunded
//	expired          → expired
//	created / action_required / unknown → pending
func mercadoPagoOrderStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "processed":
		return "completed"
	case "canceled", "cancelled", "failed":
		return "cancelled"
	case "refunded":
		return "refunded"
	case "expired":
		return "expired"
	case "created", "action_required":
		return "pending"
	default:
		return "pending"
	}
}

func decodeMPOrder(data map[string]interface{}) (*mpOrder, error) {
	if data == nil {
		return nil, errors.New("mercadopago: empty order response")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("mercadopago: marshal order response: %w", err)
	}
	var order mpOrder
	if err := json.Unmarshal(b, &order); err != nil {
		return nil, fmt.Errorf("mercadopago: parse order response: %w", err)
	}
	if strings.TrimSpace(order.ID) == "" {
		return nil, errors.New("mercadopago: order response missing id")
	}
	return &order, nil
}

// mapOrdersAPIError turns the stringly-typed errors from
// makeMercadoPagoAPICallWithIdempotencyKey into package sentinel errors so
// callers can branch with errors.Is without changing the shared HTTP helper.
func mapOrdersAPIError(err error, isPoint bool) error {
	if err == nil {
		return nil
	}
	status, ok := mercadoPagoAPIErrorStatus(err)
	if !ok {
		return err
	}
	switch status {
	case http.StatusConflict:
		if isPoint {
			return fmt.Errorf("%w: %v", ErrTerminalBusy, err)
		}
		return err
	case http.StatusForbidden:
		return fmt.Errorf("%w: %v", ErrForbidden, err)
	default:
		return err
	}
}

// mercadoPagoAPIErrorStatus extracts the HTTP status from
// "MercadoPago API error <code>: <body>" produced by makeMercadoPagoAPICallWithIdempotencyKey.
func mercadoPagoAPIErrorStatus(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	const prefix = "MercadoPago API error "
	msg := err.Error()
	if !strings.HasPrefix(msg, prefix) {
		return 0, false
	}
	rest := msg[len(prefix):]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return 0, false
	}
	code, convErr := strconv.Atoi(strings.TrimSpace(rest[:colon]))
	if convErr != nil {
		return 0, false
	}
	return code, true
}

// ── Exported aliases for handlers outside this package (Point / QR flows) ──

// OrderRequest is the body for POST /v1/orders (Point or QR).
type OrderRequest = mpOrderRequest

// OrderTransactions groups payment lines on an order.
type OrderTransactions = mpOrderTransactions

// OrderPayment is one payment line (amount as decimal string).
type OrderPayment = mpOrderPayment

// OrderConfig holds Point or QR config for an order.
type OrderConfig = mpOrderConfig

// PointConfig is the Point terminal binding on an order.
type PointConfig = mpPointConfig

// QRConfig is the QR POS binding on an order.
type QRConfig = mpQRConfig

// Order is the Orders API response (create/get).
type Order = mpOrder

// Terminal is one row from GET /terminals/v1/list.
type Terminal = mpTerminal

// CreateOrder POSTs /v1/orders (exported for handlers).
func (m *MercadoPagoPlugin) CreateOrder(businessID uint, req OrderRequest, idemKey string) (*Order, error) {
	return m.createOrder(businessID, req, idemKey)
}

// GetOrder GETs /v1/orders/{id} (exported for handlers).
func (m *MercadoPagoPlugin) GetOrder(businessID uint, orderID string) (*Order, error) {
	return m.getOrder(businessID, orderID)
}

// CancelOrder POSTs /v1/orders/{id}/cancel (exported for handlers).
func (m *MercadoPagoPlugin) CancelOrder(businessID uint, orderID string) error {
	return m.cancelOrder(businessID, orderID)
}

// ListTerminals GETs /terminals/v1/list (exported for handlers).
func (m *MercadoPagoPlugin) ListTerminals(businessID uint) ([]Terminal, error) {
	return m.listTerminals(businessID)
}

// SetTerminalMode PATCHes /terminals/v1/setup (exported for handlers).
func (m *MercadoPagoPlugin) SetTerminalMode(businessID uint, terminalID, mode string) error {
	return m.setTerminalMode(businessID, terminalID, mode)
}

// OrderStatus maps Mercado Pago order status strings to Payverge payment statuses.
func OrderStatus(s string) string {
	return mercadoPagoOrderStatus(s)
}
