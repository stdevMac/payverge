package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/services"
)

// TestMapDeliveryActionError guards the single safe-message chokepoint that BOTH
// delivery-cancellation routes funnel their service errors through:
//   - handlers.UpdateOrderStatus (PUT /status delivery accept/reject branch), and
//   - server.CancelOrder (PATCH /businesses/:id/orders/:orderId/cancel).
//
// Both call server.MapDeliveryActionError, so proving the mapper never surfaces
// raw DB/SQL detail proves neither route leaks. The mapper:
//   - maps user-meaningful sentinels (not-found / invalid-state) to safe client
//     messages with the right status code, and
//   - collapses any other (DB-origin) error to a single generic message with no
//     raw detail surfaced.
func TestMapDeliveryActionError(t *testing.T) {
	t.Run("raw DB error does not leak to client", func(t *testing.T) {
		// A realistic raw DB error: contains table/column/SQL fragments that
		// must never reach the client (this is exactly the CancelOrder leak
		// shape that previously did `c.JSON(... rerr.Error())`).
		rawErr := fmt.Errorf("ERROR: column \"delivery_orders.driver_id\" does not exist (SQLSTATE 42703)")

		status, code, msg := MapDeliveryActionError(rawErr)

		if status != http.StatusInternalServerError {
			t.Fatalf("expected status %d for raw DB error, got %d", http.StatusInternalServerError, status)
		}
		if code != ErrCodeInternal {
			t.Fatalf("expected code %q for raw DB error, got %q", ErrCodeInternal, code)
		}
		// The client message must NOT contain any of the raw error text.
		if strings.Contains(msg, "delivery_orders") ||
			strings.Contains(msg, "driver_id") ||
			strings.Contains(msg, "SQLSTATE") ||
			strings.Contains(msg, "column") ||
			strings.Contains(strings.ToLower(msg), rawErr.Error()) {
			t.Fatalf("client message leaked raw DB error text: %q", msg)
		}
		if msg == "" {
			t.Fatalf("expected a generic client message, got empty string")
		}
	})

	t.Run("wrapped gorm error does not leak", func(t *testing.T) {
		rawErr := fmt.Errorf("save delivery: %w", gorm.ErrInvalidTransaction)

		status, code, msg := MapDeliveryActionError(rawErr)

		if status != http.StatusInternalServerError {
			t.Fatalf("expected 500 for wrapped gorm error, got %d", status)
		}
		if code != ErrCodeInternal {
			t.Fatalf("expected internal code, got %q", code)
		}
		if strings.Contains(strings.ToLower(msg), "transaction") || strings.Contains(strings.ToLower(msg), "gorm") {
			t.Fatalf("client message leaked gorm internals: %q", msg)
		}
	})

	t.Run("not-found sentinel maps to safe 404", func(t *testing.T) {
		status, code, msg := MapDeliveryActionError(services.ErrDeliveryOrderNotFound)

		if status != http.StatusNotFound {
			t.Fatalf("expected 404 for not-found sentinel, got %d", status)
		}
		if code != ErrCodeNotFound {
			t.Fatalf("expected not-found code, got %q", code)
		}
		if msg == "" {
			t.Fatalf("expected a client message for not-found, got empty")
		}
	})

	t.Run("invalid-state sentinel maps to safe 400", func(t *testing.T) {
		// Wrapped exactly like the service wraps it, to prove errors.Is matching
		// survives the added context (and the context is NOT surfaced).
		wrapped := fmt.Errorf("delivery is confirmed, only pending orders can be accepted: %w", services.ErrDeliveryInvalidState)

		status, code, msg := MapDeliveryActionError(wrapped)

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid-state sentinel, got %d", status)
		}
		if code != ErrCodeInvalidInput {
			t.Fatalf("expected invalid-input code, got %q", code)
		}
		// The wrapped state detail ("confirmed") must not leak verbatim.
		if strings.Contains(msg, "confirmed") {
			t.Fatalf("client message leaked internal state detail: %q", msg)
		}
		if !errors.Is(wrapped, services.ErrDeliveryInvalidState) {
			t.Fatalf("errors.Is should still match the wrapped sentinel")
		}
	})
}
