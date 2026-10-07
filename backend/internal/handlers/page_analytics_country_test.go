package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/analytics"
)

func TestCountryFromCDNHeader(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "ar", want: "AR"},
		{in: "XX", want: ""},
		{in: "T1", want: ""},
		{in: "ARG", want: ""},
		{in: "", want: ""},
		{in: "1A", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, countryFromCDNHeader(tt.in))
		})
	}
}

// blockingTransport fails the test if TrackPageView opens an outbound request.
type blockingTransport struct {
	t *testing.T
}

func (b blockingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	b.t.Errorf("unexpected outbound HTTP to %s", req.URL.Host)
	return nil, errors.New("unexpected outbound HTTP")
}

func TestTrackPageView_CountryFromCDNHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPageAnalyticsTestDB(t)

	orig := http.DefaultTransport
	http.DefaultTransport = blockingTransport{t: t}
	t.Cleanup(func() { http.DefaultTransport = orig })

	service := analytics.NewPageAnalyticsService(db)
	handler := NewPageAnalyticsHandler(service)
	router := gin.New()
	router.POST("/analytics/page-view", handler.TrackPageView)

	payload, err := json.Marshal(map[string]any{
		"session_id": "sess-cdn-de",
		"page":       "/menu",
		"city":       "client-city",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/analytics/page-view", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("CF-IPCountry", "DE")
	req.RemoteAddr = "203.0.113.7:1234"

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var country, city string
	require.NoError(t, db.GetGorm().Raw(
		"SELECT country, city FROM page_views WHERE session_id = ?", "sess-cdn-de",
	).Row().Scan(&country, &city))
	assert.Equal(t, "DE", country)
	assert.Equal(t, "client-city", city)
}
