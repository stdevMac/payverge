package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStreamIPLimiterCapsPerIP(t *testing.T) {
	limiter := newStreamIPLimiter(5)
	releases := make([]func(), 0, 5)
	for i := 0; i < 5; i++ {
		release, ok := limiter.acquire("1.1.1.1")
		require.True(t, ok)
		require.NotNil(t, release)
		releases = append(releases, release)
	}
	_, ok := limiter.acquire("1.1.1.1")
	require.False(t, ok)

	_, ok = limiter.acquire("2.2.2.2")
	require.True(t, ok)

	releases[0]()
	release, ok := limiter.acquire("1.1.1.1")
	require.True(t, ok)
	require.NotNil(t, release)

	// Releasing the same reservation twice must not free a second slot.
	releases[0]()
	_, ok = limiter.acquire("1.1.1.1")
	require.False(t, ok)
	release()
}

func TestGuestTableEventsPerIPCap(t *testing.T) {
	previous := guestTableStreamIPLimiter
	guestTableStreamIPLimiter = newStreamIPLimiter(5)
	t.Cleanup(func() { guestTableStreamIPLimiter = previous })

	for i := 0; i < 5; i++ {
		release, ok := guestTableStreamIPLimiter.acquire("203.0.113.5")
		require.True(t, ok)
		t.Cleanup(release)
	}

	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)
	business := createSensitiveGuestTableBusiness(t, "IPCAP1")
	table := createGuestPublicTable(t, business.ID, "IPCAP1")

	router := gin.New()
	router.GET("/guest/table/:code/events", GuestTableEvents)

	blocked := httptest.NewRequest(http.MethodGet, "/guest/table/"+table.TableCode+"/events", nil)
	blocked.RemoteAddr = "203.0.113.5:1234"
	blockedW := httptest.NewRecorder()
	router.ServeHTTP(blockedW, blocked)
	require.Equal(t, http.StatusTooManyRequests, blockedW.Code, blockedW.Body.String())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	other := httptest.NewRequest(http.MethodGet, "/guest/table/"+table.TableCode+"/events", nil)
	other.RemoteAddr = "198.51.100.8:4321"
	other = other.WithContext(ctx)
	otherW := httptest.NewRecorder()
	router.ServeHTTP(otherW, other)
	require.Equal(t, http.StatusOK, otherW.Code, otherW.Body.String())
	require.True(t, strings.HasPrefix(otherW.Body.String(), "event: connected"), otherW.Body.String())
}
