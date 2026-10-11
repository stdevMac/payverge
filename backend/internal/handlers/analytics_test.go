package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestResolveBusinessLocation_KnownZone(t *testing.T) {
	business := &database.Business{Timezone: "America/New_York"}

	loc := resolveBusinessLocation(business)

	assert.NotNil(t, loc)
	assert.NotEqual(t, time.UTC, loc, "known zone should not resolve to UTC")
	assert.Equal(t, "America/New_York", loc.String())
}

func TestResolveBusinessLocation_EmptyTimezone(t *testing.T) {
	business := &database.Business{Timezone: ""}

	loc := resolveBusinessLocation(business)

	assert.Equal(t, time.UTC, loc)
}

func TestResolveBusinessLocation_NilBusiness(t *testing.T) {
	loc := resolveBusinessLocation(nil)

	assert.Equal(t, time.UTC, loc)
}

func TestResolveBusinessLocation_UnknownZoneNegativeCached(t *testing.T) {
	business := &database.Business{Timezone: "Not/AZone"}

	loc := resolveBusinessLocation(business)
	assert.Equal(t, time.UTC, loc, "unknown zone should resolve to UTC")

	// Verify the negative cache entry is stored as UTC so we don't keep
	// re-invoking time.LoadLocation for the same broken value.
	cached, ok := tzCache.Load("Not/AZone")
	assert.True(t, ok, "unknown zone should be negative-cached")
	assert.Equal(t, time.UTC, cached.(*time.Location))
}

func TestResolveBusinessLocation_CachedPointerReused(t *testing.T) {
	business := &database.Business{Timezone: "America/Los_Angeles"}

	first := resolveBusinessLocation(business)
	second := resolveBusinessLocation(business)

	// Pointer equality proves we hit the cache on the second call
	// rather than calling time.LoadLocation again (which returns a
	// freshly-allocated *time.Location each time).
	assert.Same(t, first, second, "second lookup should return the cached pointer")
}

func TestAnalyticsHandler_GetLiveBillsAcceptsBusinessSlug(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	business.BusinessId = fmt.Sprintf("analytics-slug-%d", time.Now().UnixNano())
	business.OwnerAddress = "0xAnalyticsOwner"
	require.NoError(t, database.GetDB().Save(business).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("address", business.OwnerAddress)

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	handler.GetLiveBills(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"success":true`)
}
