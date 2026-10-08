package emails

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUnsubscribeToken_RoundTrip(t *testing.T) {
	t.Setenv("UNSUBSCRIBE_TOKEN_SECRET", "test-secret-please-ignore")

	token, ok := SignUnsubscribeToken("Owner@Example.com")
	require.True(t, ok)
	require.NotEmpty(t, token)

	email, err := ParseUnsubscribeToken(token, time.Now())
	require.NoError(t, err)
	require.Equal(t, "owner@example.com", email, "email is normalized lowercase inside the token")
}

func TestUnsubscribeToken_TamperAndExpiry(t *testing.T) {
	t.Setenv("UNSUBSCRIBE_TOKEN_SECRET", "test-secret-please-ignore")

	token, ok := SignUnsubscribeToken("owner@example.com")
	require.True(t, ok)

	_, err := ParseUnsubscribeToken(token+"x", time.Now())
	require.Error(t, err, "tampered signature must be rejected")

	_, err = ParseUnsubscribeToken(strings.Replace(token, ".", "x.", 1), time.Now())
	require.Error(t, err)

	_, err = ParseUnsubscribeToken(token, time.Now().Add(366*24*time.Hour))
	require.Error(t, err, "expired token must be rejected")
}

func TestUnsubscribeToken_FallsBackToJWTSecretDomainSeparated(t *testing.T) {
	t.Setenv("UNSUBSCRIBE_TOKEN_SECRET", "")
	t.Setenv("JWT_SECRET_KEY", "jwt-secret")

	token, ok := SignUnsubscribeToken("owner@example.com")
	require.True(t, ok)
	email, err := ParseUnsubscribeToken(token, time.Now())
	require.NoError(t, err)
	require.Equal(t, "owner@example.com", email)
}

func TestUnsubscribeToken_NoSecretRefusesToSign(t *testing.T) {
	t.Setenv("UNSUBSCRIBE_TOKEN_SECRET", "")
	t.Setenv("JWT_SECRET_KEY", "")

	_, ok := SignUnsubscribeToken("owner@example.com")
	require.False(t, ok, "with no secret material, refuse to mint (emails fall back to the account link)")
}
