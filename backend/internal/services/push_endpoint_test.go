package services

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestValidatePushEndpoint(t *testing.T) {
	allowed := []string{
		"https://fcm.googleapis.com/fcm/send/abc:def",
		"https://updates.push.services.mozilla.com/wpush/v2/gAAAA",
		"https://web.push.apple.com/QGx",
		"https://api.push.apple.com/3/device/x",
		"https://wns2-bl2p.notify.windows.com/w/?token=abc",
		"https://FCM.googleapis.com:443/fcm/send/x",
	}
	for _, raw := range allowed {
		require.NoErrorf(t, ValidatePushEndpoint(raw), "should allow %s", raw)
	}

	rejected := []string{
		"",
		"http://fcm.googleapis.com/fcm/send/x", // not https
		"https://169.254.169.254/latest/meta-data/",          // metadata IP
		"https://127.0.0.1/push",                             // loopback
		"https://[::1]/push",                                 // v6 loopback
		"https://internal.payverge.local/push",               // not allowlisted
		"https://fcm.googleapis.com.evil.example/fcm/send/x", // suffix trick
		"https://evilfcm.googleapis.com/fcm/send/x",          // not exact host
		"https://notify.windows.com/x",                       // bare suffix
		"https://fcm.googleapis.com:8443/fcm/send/x",         // non-default port
		"https://user:pass@fcm.googleapis.com/fcm/send/x",    // userinfo
		"file:///etc/passwd",
		"gopher://fcm.googleapis.com/x",
	}
	for _, raw := range rejected {
		require.ErrorIsf(t, ValidatePushEndpoint(raw), ErrPushEndpointNotAllowed, "should reject %q", raw)
	}
}

// M-push: a stored endpoint outside the allowlist (legacy row) is never
// contacted.
func TestDeliverToSubscriptions_SkipsDisallowedStoredEndpoint(t *testing.T) {
	g := setupWebPushTestDB(t)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	vapidPriv, vapidPub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	svc := NewWebPushService(g, vapidPub, vapidPriv, "mailto:test@example.com")
	p256dh, auth := browserSubscriptionKeys(t)
	require.NoError(t, g.Create(&database.PushSubscription{
		UserID: 9, BusinessID: 1, Endpoint: srv.URL + "/internal", P256dhKey: p256dh, AuthKey: auth,
	}).Error)

	require.NoError(t, svc.SendPush(9, "t", "b", "/u"))
	require.Zero(t, atomic.LoadInt32(&hits), "sender must not POST to a non-allowlisted endpoint")
}

// M-push: deliveries must not follow redirects off the push service.
func TestDeliverToSubscriptions_DoesNotFollowRedirects(t *testing.T) {
	g := setupWebPushTestDB(t)
	var internalHits int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&internalHits, 1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer internal.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/admin", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	vapidPriv, vapidPub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	svc := NewWebPushService(g, vapidPub, vapidPriv, "mailto:test@example.com")
	svc.validateEndpoint = func(string) error { return nil } // reach the httptest redirector
	p256dh, auth := browserSubscriptionKeys(t)
	require.NoError(t, g.Create(&database.PushSubscription{
		UserID: 10, BusinessID: 1, Endpoint: redirector.URL + "/push", P256dhKey: p256dh, AuthKey: auth,
	}).Error)

	require.NoError(t, svc.SendPush(10, "t", "b", "/u"))
	require.Zero(t, atomic.LoadInt32(&internalHits), "redirect target must never be contacted")
}
