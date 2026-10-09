package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// stubPlacesService satisfies the GooglePlacesService interface and always
// returns the configured error. Used to verify that the public diner-page
// handlers fail-soft (200 + error_code) rather than surfacing a 500 when the
// upstream Google Places API is unhealthy.
type stubPlacesService struct{ failErr error }

func (s *stubPlacesService) GetPlaceReviews(placeID, language string) ([]any, error) {
	return nil, s.failErr
}

func (s *stubPlacesService) GetPlaceDetails(placeID string) (any, error) {
	return nil, s.failErr
}

func TestGetPublicBusinessGoogleReviews_FailSoftOnUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "demo-core-kitchen", true, true)

	prev := googlePlacesService
	googlePlacesService = &stubPlacesService{failErr: errors.New("upstream 503")}
	t.Cleanup(func() { googlePlacesService = prev })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetPublicBusinessGoogleReviews(c)

	require.Equal(t, http.StatusOK, w.Code, "expected fail-soft 200, got %d (%s)", w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"reviews":[]`)
	require.Contains(t, w.Body.String(), `"error_code"`)
	require.Contains(t, w.Body.String(), `"upstream_unavailable"`)
}

func TestGetPublicBusinessGoogleReviews_FailSoftWhenServiceMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "demo-core-no-svc", true, true)

	prev := googlePlacesService
	googlePlacesService = nil
	t.Cleanup(func() { googlePlacesService = prev })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetPublicBusinessGoogleReviews(c)

	require.Equal(t, http.StatusOK, w.Code, "expected fail-soft 200, got %d (%s)", w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"reviews":[]`)
	require.Contains(t, w.Body.String(), `"places_not_configured"`)
}

func TestGetPublicBusinessGoogleDetails_FailSoftOnUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "demo-core-kitchen-2", true, true)

	prev := googlePlacesService
	googlePlacesService = &stubPlacesService{failErr: errors.New("upstream 503")}
	t.Cleanup(func() { googlePlacesService = prev })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetPublicBusinessGoogleDetails(c)

	require.Equal(t, http.StatusOK, w.Code, "expected fail-soft 200, got %d (%s)", w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"place_details":null`)
	require.Contains(t, w.Body.String(), `"error_code"`)
	require.Contains(t, w.Body.String(), `"upstream_unavailable"`)
}

func TestGetPublicBusinessGoogleDetails_FailSoftWhenServiceMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "demo-core-details-no-svc", true, true)

	prev := googlePlacesService
	googlePlacesService = nil
	t.Cleanup(func() { googlePlacesService = prev })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetPublicBusinessGoogleDetails(c)

	require.Equal(t, http.StatusOK, w.Code, "expected fail-soft 200, got %d (%s)", w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"place_details":null`)
	require.Contains(t, w.Body.String(), `"places_not_configured"`)
}
