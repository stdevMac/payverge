package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/session"
)

func TestVerifyTokenCacheReturnsIndependentCopies(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "perf-bench-jwt-secret-key-do-not-use-in-prod")

	previousStore := session.GlobalStore
	session.GlobalStore = nil
	t.Cleanup(func() {
		session.GlobalStore = previousStore
		verifiedTokenCacheMu.Lock()
		verifiedTokenCache = make(map[string]verifiedTokenCacheEntry)
		verifiedTokenCacheMu.Unlock()
	})

	verifiedTokenCacheMu.Lock()
	verifiedTokenCache = make(map[string]verifiedTokenCacheEntry)
	verifiedTokenCacheMu.Unlock()

	token, err := GenerateUserToken(42, "user@example.com", "0x1234", "owner", 0)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	first, err := VerifyUserToken(token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	first["role"] = "mutated"

	second, err := VerifyUserToken(token)
	if err != nil {
		t.Fatalf("verify token again: %v", err)
	}

	if got := second["role"]; got != "owner" {
		t.Fatalf("want cached claims to stay isolated, got role=%v", got)
	}
}
