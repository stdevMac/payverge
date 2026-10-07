package main

import (
	"os"
	"strings"
	"testing"
)

func TestGuestPaymentRoutesAreRateLimitedAndLegacyWebhookRemoved(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}
	source := string(src)

	if strings.Contains(source, `publicRoutes.POST("/payments/webhook"`) {
		t.Fatalf("legacy unauthenticated /payments/webhook must not be wired")
	}

	required := []string{
		`guestOrderRateLimiter.RateLimit(), paymentHandler.RequestAlternativePayment`,
		`guestOrderRateLimiter.RateLimit(), paymentHandler.ProcessCryptoPayment`,
		`guestOrderRateLimiter.RateLimit(), paymentHandler.IssueCryptoQuote`,
		`guestOrderRateLimiter.RateLimit(), paymentHandler.ProcessCrossChainPayment`,
	}
	for _, snippet := range required {
		if !strings.Contains(source, snippet) {
			t.Fatalf("guest payment route missing rate limiter: %s", snippet)
		}
	}
}
