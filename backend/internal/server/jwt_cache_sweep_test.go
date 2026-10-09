package server

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

func TestSweepVerifiedTokenCacheDropsExpired(t *testing.T) {
	verifiedTokenCacheMu.Lock()
	verifiedTokenCache = map[string]verifiedTokenCacheEntry{
		"expired": {claims: jwt.MapClaims{}, exp: time.Now().Add(-time.Minute).Unix()},
		"live":    {claims: jwt.MapClaims{}, exp: time.Now().Add(time.Hour).Unix()},
	}
	verifiedTokenCacheMu.Unlock()

	sweepVerifiedTokenCache(time.Now())

	verifiedTokenCacheMu.RLock()
	defer verifiedTokenCacheMu.RUnlock()
	if _, ok := verifiedTokenCache["expired"]; ok {
		t.Fatal("expired token not swept")
	}
	if _, ok := verifiedTokenCache["live"]; !ok {
		t.Fatal("live token wrongly swept")
	}
}

func TestSweepVerifiedTokenCacheKeepsZeroExp(t *testing.T) {
	// exp == 0 means "no expiry stored" — must not be swept.
	verifiedTokenCacheMu.Lock()
	verifiedTokenCache = map[string]verifiedTokenCacheEntry{
		"no-exp": {claims: jwt.MapClaims{}, exp: 0},
	}
	verifiedTokenCacheMu.Unlock()

	sweepVerifiedTokenCache(time.Now())

	verifiedTokenCacheMu.RLock()
	defer verifiedTokenCacheMu.RUnlock()
	if _, ok := verifiedTokenCache["no-exp"]; !ok {
		t.Fatal("entry with exp==0 wrongly swept")
	}
}

func TestStartVerifiedTokenCacheSweeperExitsOnContextCancel(t *testing.T) {
	// Verify the sweeper goroutine terminates when its context is cancelled.
	// We assert termination indirectly: if it did not stop, the -race detector
	// would catch concurrent writes in subsequent tests. We just confirm no
	// panic / no deadlock under a very short tick + quick cancel.
	done := make(chan struct{})
	stop := StartVerifiedTokenCacheSweeper(1 * time.Millisecond)
	go func() {
		stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sweeper stop func did not return within 2s")
	}
}
