package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/services"
)

// The public business lookup resolves by custom_url slug only. Numeric
// /b/<id> links were never minted by this release, so a numeric segment that
// is not a slug is a 404 rather than a by-ID lookup.
func setupBusinessPublicSlugOnlyTest(t *testing.T) {
	t.Helper()
	setupPublicBusinessHandlerTestDB(t)
	// The slug lookup is cached (pricing cache); disable it so per-test
	// businesses can't bleed across tests through the cache.
	services.DisablePricingCacheForTest(t)
}

func performGetBusinessByCustomURL(t *testing.T, segment string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: segment}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetBusinessByCustomURL(c)
	return w
}

func TestBusinessPublicLookup_NumericIDIsNotAnAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBusinessPublicSlugOnlyTest(t)
	business := createPublicBusinessRouteTestBusiness(t, "slug-only-published", true, true)

	w := performGetBusinessByCustomURL(t, fmt.Sprintf("%d", business.ID))

	assert.Equal(t, http.StatusNotFound, w.Code, "a published business must not resolve by its numeric ID")
}

func TestBusinessPublicLookup_NumericSlugResolves(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBusinessPublicSlugOnlyTest(t)
	// validateCustomURL allows digits-only slugs; they resolve as slugs.
	slugOwner := createPublicBusinessRouteTestBusiness(t, "246810", true, true)

	w := performGetBusinessByCustomURL(t, "246810")

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(slugOwner.ID), body["id"])
}

func TestBusinessPublicLookup_UnknownSlugIs404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBusinessPublicSlugOnlyTest(t)

	assert.Equal(t, http.StatusNotFound, performGetBusinessByCustomURL(t, "no-such-slug").Code)
	assert.Equal(t, http.StatusNotFound, performGetBusinessByCustomURL(t, "999999").Code)
}
