package services

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupWebPushTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&database.PushSubscription{}))
	return g
}

// browserSubscriptionKeys generates a valid P-256 public point + 16-byte auth
// secret, base64url-encoded the way a real browser PushSubscription does, so
// webpush-go's payload encryption succeeds and the HTTP request actually fires.
func browserSubscriptionKeys(t *testing.T) (string, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	pub := elliptic.Marshal(elliptic.P256(), priv.PublicKey.X, priv.PublicKey.Y)
	auth := make([]byte, 16)
	_, err = rand.Read(auth)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(pub), base64.RawURLEncoding.EncodeToString(auth)
}

// A 404 from the push service means the subscription no longer exists — it
// must be pruned exactly like a 410 Gone. Other non-2xx statuses must NOT
// prune (they may be transient) but must not be silently ignored either.
func TestDeliverToSubscriptions_PrunesGoneEndpointsAndKeepsTransient(t *testing.T) {
	g := setupWebPushTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gone-404":
			w.WriteHeader(http.StatusNotFound)
		case "/gone-410":
			w.WriteHeader(http.StatusGone)
		case "/transient-400":
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer server.Close()

	vapidPriv, vapidPub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	svc := NewWebPushService(g, vapidPub, vapidPriv, "mailto:test@payverge.io")
	// httptest serves plain http on 127.0.0.1, which the production allowlist
	// rejects; this test is about status handling, not endpoint policy.
	svc.validateEndpoint = func(string) error { return nil }

	p256dh, auth := browserSubscriptionKeys(t)
	userID := uint(42)
	for _, path := range []string{"/gone-404", "/gone-410", "/transient-400", "/ok"} {
		require.NoError(t, g.Create(&database.PushSubscription{
			UserID:     userID,
			BusinessID: 1,
			Endpoint:   server.URL + path,
			P256dhKey:  p256dh,
			AuthKey:    auth,
		}).Error)
	}

	require.NoError(t, svc.SendPush(userID, "title", "body", "/url"))

	var remaining []database.PushSubscription
	require.NoError(t, g.Find(&remaining).Error)
	paths := map[string]bool{}
	for _, sub := range remaining {
		paths[sub.Endpoint[len(server.URL):]] = true
	}
	require.False(t, paths["/gone-404"], "404 endpoint must be pruned")
	require.False(t, paths["/gone-410"], "410 endpoint must be pruned")
	require.True(t, paths["/transient-400"], "non-gone failure must NOT be pruned")
	require.True(t, paths["/ok"], "healthy endpoint must survive")
}

func TestEndpointHostOmitsSubscriptionToken(t *testing.T) {
	got := endpointHost("https://fcm.googleapis.com/fcm/send/very-secret-token")
	require.Equal(t, "fcm.googleapis.com", got)
	require.Equal(t, "invalid", endpointHost("not-a-url"))
}
