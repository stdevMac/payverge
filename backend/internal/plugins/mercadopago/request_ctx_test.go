package mercadopago

import (
	"context"
	"strings"
	"testing"
)

// TestMakeRequestHonorsContextCancel proves the MercadoPago HTTP helper threads
// the caller's context: a pre-cancelled ctx must abort before the network call
// completes, returning a context error (EXT-3). With http.NewRequest the ctx was
// dropped and the call always ran the full 30s Timeout regardless of caller.
func TestMakeRequestHonorsContextCancel(t *testing.T) {
	mp := &MercadoPagoPlugin{}
	cfg := &MercadoPagoConfig{AccessToken: "test-token"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel up front

	_, err := mp.makeMercadoPagoAPICall(ctx, "GET", "/v1/payments/search", nil, cfg)
	if err == nil {
		t.Fatalf("expected error from cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context-canceled error, got: %v", err)
	}
}
