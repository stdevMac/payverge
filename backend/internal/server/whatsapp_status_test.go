package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/services"
)

// GET /businesses/:id/whatsapp/status is mounted in every build so the AI
// Waiter dashboard can tell "not compiled in" (built=false: hide the channel)
// from "compiled in but off" (built=true, enabled=false). `requested` mirrors
// WHATSAPP_ENABLED so a tagged, requested build whose manager failed to start
// is not reported as "not turned on". expectWhatsAppBuilt is pinned per build
// tag in whatsapp_build_{,no}whatsapp_test.go.
func TestGetWhatsAppStatus_ReportsBuildWithoutManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := GetWhatsAppManager()
	SetWhatsAppManager(nil)
	t.Cleanup(func() { SetWhatsAppManager(prev) })

	require.Equal(t, expectWhatsAppBuilt, services.WhatsAppBuilt)

	for _, tc := range []struct {
		env       string
		requested bool
	}{
		{env: "", requested: false},
		{env: "false", requested: false},
		{env: "true", requested: true},
		{env: "TRUE", requested: true},
	} {
		t.Run("WHATSAPP_ENABLED="+tc.env, func(t *testing.T) {
			t.Setenv(services.WhatsAppEnabledEnv, tc.env)

			body := getWhatsAppJSON(t, GetWhatsAppStatus, "42", http.StatusOK)
			require.Equal(t, map[string]any{
				"status":    "disconnected",
				"enabled":   false,
				"built":     expectWhatsAppBuilt,
				"requested": tc.requested,
			}, body)
		})
	}
}

// The pairing-code route answers like connect/disconnect when the manager is
// absent; it never invents a code.
func TestGetWhatsAppQR_WithoutManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := GetWhatsAppManager()
	SetWhatsAppManager(nil)
	t.Cleanup(func() { SetWhatsAppManager(prev) })

	body := getWhatsAppJSON(t, GetWhatsAppQR, "42", http.StatusInternalServerError)
	require.NotContains(t, body, "qr_code")

	getWhatsAppJSON(t, GetWhatsAppQR, "abc", http.StatusBadRequest)
}

func getWhatsAppJSON(t *testing.T, h gin.HandlerFunc, id string, wantCode int) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: id}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/inside/businesses/"+id+"/whatsapp", nil)
	h(c)
	require.Equal(t, wantCode, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func TestGetWhatsAppStatus_RejectsNonNumericBusinessID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/inside/businesses/abc/whatsapp/status", nil)
	GetWhatsAppStatus(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// POST /whatsapp/connect must answer (code or 504) before the dashboard's
// shared axios instance gives up at 30s (frontend/src/api/tools/instance.ts);
// with the two equal, a code that arrived near the deadline was lost to a
// client-side abort. Keep at least 10s of headroom for the network and proxy.
func TestWhatsAppFirstQRWaitBeatsDashboardTimeout(t *testing.T) {
	const dashboardRequestTimeout = 30 * time.Second
	require.LessOrEqual(t, whatsAppFirstQRWait, dashboardRequestTimeout-10*time.Second)
	require.Greater(t, whatsAppFirstQRWait, 5*time.Second, "too short to wait for a slow first code")
}
