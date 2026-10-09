package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicGuestCreateOrderRoute_SitsBesideQuote(t *testing.T) {
	body, err := os.ReadFile("main.go")
	require.NoError(t, err)
	src := string(body)
	assert.Contains(t, src,
		`publicRoutes.POST("/businesses/:business_id/delivery/quote", deliveryQuoteRateLimit, deliveryHandler.QuoteDelivery)`)
	assert.Contains(t, src,
		`publicRoutes.POST("/businesses/:business_id/delivery/orders", deliveryQuoteRateLimit, deliveryHandler.GuestDeliveryCheckout)`,
		"storefront create-order must share the quote path prefix, not 404")
	assert.NotContains(t, src, `"/businesses/:business_id/guest-delivery-checkout"`,
		"delivery/orders is the single storefront create-order route")
}
