package main

import (
	"context"

	"github.com/stdevmac/payverge/backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

// newGuestCatalogReadLimiter throttles the public, unauthenticated catalog
// reads a guest page makes once per visit (translated menu, enabled payment
// plugins). It keeps its own bucket but the same per-client budget as the
// guest table reads (GUEST_TABLE_READ_RATE_LIMIT_REQUESTS_PER_MINUTE,
// middleware.GuestTableReadRequestsPerMinute), which already covers a full
// table of diners behind one venue NAT. Before this, both routes had no route
// limiter and could be scraped or used to enumerate businesses freely.
func newGuestCatalogReadLimiter() gin.HandlerFunc {
	return middleware.NewSimpleRateLimiter(context.Background(),
		intEnv("GUEST_TABLE_READ_RATE_LIMIT_REQUESTS_PER_MINUTE", middleware.GuestTableReadRequestsPerMinute)).RateLimit()
}
