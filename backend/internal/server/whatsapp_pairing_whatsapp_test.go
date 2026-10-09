//go:build whatsapp

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

// installHermeticWhatsApp swaps in a manager whose next pairing attempt uses
// client, and restores the previous manager afterwards.
func installHermeticWhatsApp(t *testing.T, client *services.FakeWhatsAppClient) *services.WhatsAppManager {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := services.NewFakeDeviceStore()
	store.QueueNewClient(client)
	wm := services.NewWhatsAppManagerForHermetic(store, nil)
	prev := GetWhatsAppManager()
	SetWhatsAppManager(wm)
	t.Cleanup(func() {
		_ = wm.DisconnectBusiness(42)
		wm.Stop()
		SetWhatsAppManager(prev)
	})
	return wm
}

func callWhatsApp(t *testing.T, h gin.HandlerFunc, method string) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	c.Request = httptest.NewRequest(method, "/api/v1/inside/businesses/42/whatsapp", nil)
	h(c)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return w.Code, body
}

// The operator must be able to finish pairing: connect returns the first code,
// and GET /whatsapp/qr keeps serving whichever code whatsmeow issued last while
// the phone has not scanned yet. Before the fix the manager blocked on the
// second code and the dashboard had no way to see any code at all.
func TestWhatsAppPairing_ConnectThenPollLatestCode(t *testing.T) {
	installHermeticWhatsApp(t, services.NewFakePairingWhatsAppClient("code-A", "code-B", "code-C"))

	code, body := callWhatsApp(t, ConnectWhatsApp, http.MethodPost)
	require.Equal(t, http.StatusOK, code, body)
	require.Equal(t, "scanning", body["status"])
	require.NotEmpty(t, body["qr_code"])

	require.Eventually(t, func() bool {
		code, body := callWhatsApp(t, GetWhatsAppQR, http.MethodGet)
		return code == http.StatusOK && body["qr_code"] == "code-C" && body["status"] == "pairing"
	}, 2*time.Second, 10*time.Millisecond, "GET /whatsapp/qr never served the latest rotated code")

	code, status := callWhatsApp(t, GetWhatsAppStatus, http.MethodGet)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "pairing", status["status"])
	require.Equal(t, true, status["enabled"])
	require.Equal(t, true, status["built"])
	require.NotContains(t, status, "qr_code", "status is settings:read and must never carry the pairing code")

	code, _ = callWhatsApp(t, DisconnectWhatsApp, http.MethodPost)
	require.Equal(t, http.StatusOK, code)
	code, body = callWhatsApp(t, GetWhatsAppQR, http.MethodGet)
	require.Equal(t, http.StatusOK, code)
	require.NotContains(t, body, "qr_code", "a disconnected business has nothing to scan")
}

// A QR channel that closes without a code must fail the connect request
// instead of answering 200 with an empty qr_code.
func TestConnectWhatsApp_ChannelClosedWithoutCode(t *testing.T) {
	installHermeticWhatsApp(t, &services.FakeWhatsAppClient{})

	code, body := callWhatsApp(t, ConnectWhatsApp, http.MethodPost)
	require.Equal(t, http.StatusBadGateway, code, body)
	require.NotContains(t, body, "qr_code")
}

// Connect must not hang forever when whatsmeow never issues a code.
func TestConnectWhatsApp_TimesOutWithoutCode(t *testing.T) {
	installHermeticWhatsApp(t, services.NewFakePairingWhatsAppClient())
	prevWait := whatsAppFirstQRWait
	whatsAppFirstQRWait = 20 * time.Millisecond
	t.Cleanup(func() { whatsAppFirstQRWait = prevWait })

	code, body := callWhatsApp(t, ConnectWhatsApp, http.MethodPost)
	require.Equal(t, http.StatusGatewayTimeout, code, body)
	require.NotContains(t, body, "qr_code")
}
