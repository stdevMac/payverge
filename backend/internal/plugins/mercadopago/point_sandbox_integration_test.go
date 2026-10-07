//go:build mercadopago_sandbox
// +build mercadopago_sandbox

package mercadopago

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Virtual Point terminal id used by Mercado Pago's sandbox simulator.
// Orders API expects {POI_TYPE}__{SERIAL}; serial SBX0000001 is MP's standard
// virtual device, valid with any poi_type for the country.
const mercadoPagoSandboxPointTerminalID = "NEWLAND_N950__SBX0000001"

// TestPointSandboxIntegration_ProcessedAndCanceled exercises the Point Orders
// path against a live Mercado Pago sandbox account: create an order on the
// virtual terminal, simulate terminal events, poll until the order reaches a
// terminal status, and assert mercadoPagoOrderStatus mapping.
//
// Requires MERCADOPAGO_SANDBOX_ACCESS_TOKEN. Skips cleanly when unset so local
// `go test -tags mercadopago_sandbox` does not fail without credentials.
func TestPointSandboxIntegration_ProcessedAndCanceled(t *testing.T) {
	accessToken := strings.TrimSpace(os.Getenv("MERCADOPAGO_SANDBOX_ACCESS_TOKEN"))
	if accessToken == "" {
		t.Skip("set MERCADOPAGO_SANDBOX_ACCESS_TOKEN to run Point sandbox integration tests")
	}

	config := &MercadoPagoConfig{
		AccessToken: accessToken,
		Environment: "sandbox",
	}
	plugin := &MercadoPagoPlugin{}
	ctx := context.Background()

	// --- Order 1: simulate processed → mapped completed ---
	processedOrderID := createSandboxPointOrder(t, ctx, plugin, config, "point-sandbox-processed")
	simulateSandboxOrderEvent(t, ctx, plugin, config, processedOrderID, "processed")
	finalProcessed := pollSandboxOrderStatus(t, ctx, plugin, config, processedOrderID, "processed", 40*time.Second)
	if got := mercadoPagoOrderStatus(finalProcessed); got != "completed" {
		t.Fatalf("processed order mapped status: got %q want completed (raw status %q)", got, finalProcessed)
	}

	// --- Order 2: simulate canceled → mapped cancelled ---
	canceledOrderID := createSandboxPointOrder(t, ctx, plugin, config, "point-sandbox-canceled")
	simulateSandboxOrderEvent(t, ctx, plugin, config, canceledOrderID, "canceled")
	finalCanceled := pollSandboxOrderStatus(t, ctx, plugin, config, canceledOrderID, "canceled", 40*time.Second)
	if got := mercadoPagoOrderStatus(finalCanceled); got != "cancelled" {
		t.Fatalf("canceled order mapped status: got %q want cancelled (raw status %q)", got, finalCanceled)
	}
}

func createSandboxPointOrder(t *testing.T, ctx context.Context, plugin *MercadoPagoPlugin, config *MercadoPagoConfig, refSuffix string) string {
	t.Helper()
	// ARS 100.00 — decimal string per Orders API (zero commission: no marketplace_fee).
	body := map[string]interface{}{
		"type":               "point",
		"external_reference": fmt.Sprintf("payverge_%s_%d", refSuffix, time.Now().UnixNano()),
		"description":        "Payverge Point sandbox integration",
		"transactions": map[string]interface{}{
			"payments": []map[string]string{
				{"amount": "100.00"},
			},
		},
		"config": map[string]interface{}{
			"point": map[string]string{
				"terminal_id": mercadoPagoSandboxPointTerminalID,
			},
		},
	}
	resp, err := plugin.makeMercadoPagoAPICallWithIdempotencyKey(
		ctx,
		http.MethodPost,
		"/v1/orders",
		body,
		config,
		uuid.NewString(),
	)
	if err != nil {
		t.Fatalf("create Point sandbox order (%s): %v", refSuffix, err)
	}
	orderID := strings.TrimSpace(stringFromMercadoPagoAny(resp["id"]))
	if orderID == "" {
		t.Fatalf("Point sandbox order response missing id: %#v", resp)
	}
	t.Cleanup(func() { cancelSandboxOrderBestEffort(t, plugin, config, orderID) })
	return orderID
}

// cancelSandboxOrderBestEffort clears a leftover order off the virtual
// terminal queue so a failed run cannot 409 the next one
// (already_queued_order_on_terminal). Errors are ignored: an order that
// already reached a terminal status rejects the transition harmlessly.
func cancelSandboxOrderBestEffort(t *testing.T, plugin *MercadoPagoPlugin, config *MercadoPagoConfig, orderID string) {
	t.Helper()
	_, _ = plugin.makeMercadoPagoAPICallWithIdempotencyKey(
		context.Background(),
		http.MethodPost,
		"/v1/orders/"+orderID+"/events",
		map[string]string{"status": "canceled"},
		config,
		uuid.NewString(),
	)
}

func simulateSandboxOrderEvent(t *testing.T, ctx context.Context, plugin *MercadoPagoPlugin, config *MercadoPagoConfig, orderID, eventType string) {
	t.Helper()
	path := "/v1/orders/" + orderID + "/events"
	_, err := plugin.makeMercadoPagoAPICallWithIdempotencyKey(
		ctx,
		http.MethodPost,
		path,
		map[string]string{"status": eventType},
		config,
		uuid.NewString(),
	)
	// The sandbox simulator acknowledges events with 200 and an empty body,
	// which the JSON helper reports as a parse error; the event still lands.
	if err != nil && !strings.Contains(err.Error(), "unexpected end of JSON input") {
		t.Fatalf("simulate order event type=%s order=%s: %v", eventType, orderID, err)
	}
}

func pollSandboxOrderStatus(t *testing.T, ctx context.Context, plugin *MercadoPagoPlugin, config *MercadoPagoConfig, orderID, wantStatus string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastStatus string
	for time.Now().Before(deadline) {
		resp, err := plugin.makeMercadoPagoAPICall(ctx, http.MethodGet, "/v1/orders/"+orderID, nil, config)
		if err != nil {
			t.Logf("poll getOrder %s: %v", orderID, err)
			time.Sleep(1 * time.Second)
			continue
		}
		lastStatus = strings.ToLower(strings.TrimSpace(stringFromMercadoPagoAny(resp["status"])))
		if lastStatus == strings.ToLower(wantStatus) {
			return lastStatus
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("order %s did not reach status %q within %s (last status %q)", orderID, wantStatus, timeout, lastStatus)
	return lastStatus
}
