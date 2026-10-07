package services

import "fmt"

// Order input caps (B-4). Shared by the guest and operator create paths —
// enforcement lives in ApplyPromotionsToOrder (per-line) and in the HTTP
// handlers (order-level notes). Stage 3/4 FE mirrors these values.
const (
	MaxOrderItemQuantity = 50  // max quantity per order line
	MaxOrderLines        = 60  // max request lines per order
	MaxOrderTextLen      = 500 // max runes for notes AND special_requests (each)
)

// Structured guest order-create error codes. These exact strings are the
// contract the guest frontend (Stage 3 G-1) maps to translated messages.
// They ride in the existing JSON error body as a "code" field.
const (
	// OrderErrCodeBusinessUnavailable: the server administrator suspended or
	// closed the business, so guest surfaces refuse new orders and payments.
	OrderErrCodeBusinessUnavailable = "business_unavailable"
	OrderErrCodeOrderingDisabled    = "ordering_disabled"
	// OrderErrCodeBusinessClosed is Closed Mode hours: guest UI says ordering is
	// paused; checkout must fail closed (FIND-035). Distinct from kitchen/orders toggles.
	OrderErrCodeBusinessClosed   = "business_closed"
	OrderErrCodeBillNotOpen      = "bill_not_open"
	OrderErrCodeItemUnavailable  = "item_unavailable"
	OrderErrCodeItemNotFound     = "item_not_found"
	OrderErrCodeBundleNotFound   = "bundle_not_found"
	OrderErrCodeOptionNotFound   = "option_not_found"
	OrderErrCodeQuantityExceeded = "quantity_exceeded"
	OrderErrCodeTooManyItems     = "too_many_items"
	OrderErrCodeTextTooLong      = "text_too_long"
)

// OrderValidationError is a typed sentinel carrying a machine-readable code
// alongside the human message. Pricing/validation failures inside the
// promotion engine wrap themselves in this type; HTTP handlers unwrap it via
// errors.As and emit {"error": Message, "code": Code}.
type OrderValidationError struct {
	Code    string
	Message string
}

func (e *OrderValidationError) Error() string { return e.Message }

// NewOrderValidationError builds an OrderValidationError with a formatted message.
func NewOrderValidationError(code, format string, args ...interface{}) *OrderValidationError {
	return &OrderValidationError{Code: code, Message: fmt.Sprintf(format, args...)}
}
