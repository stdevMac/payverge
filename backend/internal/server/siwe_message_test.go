package server

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logic"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type siweFixture struct {
	domain    string
	uri       string
	nonce     string
	issuedAt  string
	extraTail string
}

func buildSIWE(address string, f siweFixture) string {
	msg := fmt.Sprintf("%s wants you to sign in with your Ethereum account:\n%s\n\nSign in to Payverge.\n\nURI: %s\nVersion: 1\nChain ID: #1\nNonce: %s\nIssued At: %s",
		f.domain, address, f.uri, f.nonce, f.issuedAt)
	return msg + f.extraTail
}

func signSIWE(t *testing.T, key *ecdsa.PrivateKey, message string) string {
	t.Helper()
	prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)
	sig, err := crypto.Sign(crypto.Keccak256([]byte(prefixed)), key)
	require.NoError(t, err)
	sig[64] += 27
	return hexutil.Encode(sig)
}

func postSignIn(t *testing.T, message, signature string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, err := json.Marshal(SignInRequest{Message: message, Signature: signature})
	require.NoError(t, err)
	c.Request = httptest.NewRequest(http.MethodPost, "/signin", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	SignIn(c)
	return w
}

func issueTestChallenge(t *testing.T, address string) string {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/challenge", strings.NewReader(`{"address":"`+address+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	GenerateChallenge(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Challenge string `json:"challenge"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Challenge)
	return resp.Challenge
}

// newSIWEWallet returns a fresh key whose address already has a user row (new
// wallets otherwise hit launch-invite admission, which is not under test).
func newSIWEWallet(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	address := crypto.PubkeyToAddress(key.PublicKey).Hex()
	require.NoError(t, database.GetDB().Create(&database.User{Address: strings.ToLower(address), Email: strings.ToLower(address) + "@wallet.test", Role: "user"}).Error)
	return key, address
}

func nowISO(offset time.Duration) string {
	return time.Now().Add(offset).UTC().Format(time.RFC3339Nano)
}

// M-siwe: a signed message whose domain or URI names another site, or whose
// timestamps are out of range, must be rejected even with a live nonce and a
// valid signature (a phishing page relaying our challenge).
func TestSignIn_RejectsMessageNotForThisSiteOrStale(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRealmSwitchOwnerDB(t)
	t.Setenv("PUBLIC_URL", "https://pay.example.test")

	cases := []struct {
		name string
		f    siweFixture
		want string
	}{
		{"foreign domain", siweFixture{domain: "evil.example", uri: "https://pay.example.test", issuedAt: nowISO(0)}, "not for this site"},
		{"foreign URI host", siweFixture{domain: "pay.example.test", uri: "https://evil.example/login", issuedAt: nowISO(0)}, "not for this site"},
		{"non-http URI", siweFixture{domain: "pay.example.test", uri: "ftp://pay.example.test", issuedAt: nowISO(0)}, "not for this site"},
		{"stale issued at", siweFixture{domain: "pay.example.test", uri: "https://pay.example.test", issuedAt: nowISO(-time.Hour)}, "expired or not yet valid"},
		{"future issued at", siweFixture{domain: "pay.example.test", uri: "https://pay.example.test", issuedAt: nowISO(time.Hour)}, "expired or not yet valid"},
		{"expired", siweFixture{domain: "pay.example.test", uri: "https://pay.example.test", issuedAt: nowISO(0), extraTail: "\nExpiration Time: " + nowISO(-time.Hour)}, "expired or not yet valid"},
		{"not before in future", siweFixture{domain: "pay.example.test", uri: "https://pay.example.test", issuedAt: nowISO(0), extraTail: "\nNot Before: " + nowISO(time.Hour)}, "expired or not yet valid"},
		{"missing issued at", siweFixture{domain: "pay.example.test", uri: "https://pay.example.test", issuedAt: ""}, "Invalid message"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, address := newSIWEWallet(t)
			tc.f.nonce = issueTestChallenge(t, address)
			message := buildSIWE(address, tc.f)
			if tc.f.issuedAt == "" {
				message = strings.Replace(message, "\nIssued At: ", "", 1)
			}

			w := postSignIn(t, message, signSIWE(t, key, message))
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), tc.want)
			assert.Empty(t, liveSetCookieValue(w, "session_token"))
		})
	}

	// Control: the same flow with this site's domain and a fresh timestamp
	// signs in, so the rejections above are about the checked fields.
	key, address := newSIWEWallet(t)
	message := buildSIWE(address, siweFixture{
		domain: "pay.example.test", uri: "https://pay.example.test", nonce: issueTestChallenge(t, address), issuedAt: nowISO(-30 * time.Second),
		extraTail: "\nExpiration Time: " + nowISO(10*time.Minute),
	})
	w := postSignIn(t, message, signSIWE(t, key, message))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotEmpty(t, liveSetCookieValue(w, "session_token"))
}

// Low (challenge DoS): requesting a challenge for an address must not evict
// a challenge another client already holds for it.
func TestGenerateChallenge_DoesNotEvictPendingChallengeForSameAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRealmSwitchOwnerDB(t)
	t.Setenv("PUBLIC_URL", "https://pay.example.test")

	key, address := newSIWEWallet(t)

	victimNonce := issueTestChallenge(t, address)
	for i := 0; i < 5; i++ {
		issueTestChallenge(t, address) // attacker spamming the victim's address
	}

	message := buildSIWE(address, siweFixture{domain: "pay.example.test", uri: "https://pay.example.test", nonce: victimNonce, issuedAt: nowISO(0)})
	w := postSignIn(t, message, signSIWE(t, key, message))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The nonce is single use.
	w = postSignIn(t, message, signSIWE(t, key, message))
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid or expired challenge")
}

// A nonce issued for one address cannot be spent by another wallet.
func TestSignIn_NonceIsBoundToTheAddressItWasIssuedFor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRealmSwitchOwnerDB(t)
	t.Setenv("PUBLIC_URL", "https://pay.example.test")

	_, victim := newSIWEWallet(t)
	attackerKey, attacker := newSIWEWallet(t)

	nonce := issueTestChallenge(t, victim)
	message := buildSIWE(attacker, siweFixture{domain: "pay.example.test", uri: "https://pay.example.test", nonce: nonce, issuedAt: nowISO(0)})
	w := postSignIn(t, message, signSIWE(t, attackerKey, message))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Invalid or expired challenge")
}

func TestParseSIWEMessage_RejectsDuplicateFieldsAndMissingHeader(t *testing.T) {
	address := "0x742d35cc6635c0532925a3b8d400e4c3f2c0c1c1"
	good := buildSIWE(address, siweFixture{domain: "pay.example.test", uri: "https://pay.example.test", nonce: "abc", issuedAt: nowISO(0)})
	msg, err := ParseSIWEMessage(good)
	require.NoError(t, err)
	assert.Equal(t, "pay.example.test", msg.Domain)
	assert.Equal(t, address, msg.Address)
	assert.Equal(t, "1", msg.ChainID)
	assert.Equal(t, "abc", msg.Nonce)

	_, err = ParseSIWEMessage(good + "\nNonce: other")
	assert.Error(t, err, "a second Nonce line must not override the first")
	_, err = ParseSIWEMessage(good + "\nURI: https://evil.example")
	assert.Error(t, err, "a second URI line must not override the first")
	_, err = ParseSIWEMessage(strings.Replace(good, " wants you to sign in with your Ethereum account:", "", 1))
	assert.Error(t, err)
}

func TestSIWEAllowedHosts_PublicURLFallbackAndTrustedOrigins(t *testing.T) {
	SetSIWETrustedOrigins(nil)
	t.Cleanup(func() { SetSIWETrustedOrigins(nil) })

	t.Setenv("PUBLIC_URL", "")
	t.Setenv("FRONTEND_URL", "https://retired.example.test")
	hosts := siweAllowedHosts()
	assert.NotContains(t, hosts, "retired.example.test", "FRONTEND_URL is not an alias of PUBLIC_URL")

	t.Setenv("PUBLIC_URL", "https://Pay.Example.Test:443/")
	hosts = siweAllowedHosts()
	assert.Contains(t, hosts, "pay.example.test", "default port dropped")

	SetSIWETrustedOrigins([]string{"http://localhost:3000"})
	hosts = siweAllowedHosts()
	assert.Contains(t, hosts, "localhost:3000")

	msg := SIWEMessage{Domain: "localhost:3000", URI: "http://localhost:3000", Version: "1", IssuedAt: time.Now()}
	assert.NoError(t, msg.ValidateForSite(time.Now()))
	msg.URI = "http://localhost:3001"
	assert.ErrorIs(t, msg.ValidateForSite(time.Now()), ErrSIWEDomain, "port is part of the host")
}

// A self-hosted instance with ALLOWED_ORIGINS unset accepts SIWE messages for
// its own PUBLIC_URL only: main.go registers the raw env list, so the CORS
// layer's built-in hosted-product defaults never become SIWE domains.
func TestSIWEAllowedHosts_OnlyPublicURLWhenNoOriginsConfigured(t *testing.T) {
	t.Cleanup(func() { SetSIWETrustedOrigins(nil) })
	t.Setenv("PUBLIC_URL", "https://pay.example.test")

	SetSIWETrustedOrigins(strings.Split("", ","))
	assert.Equal(t, map[string]struct{}{"pay.example.test": {}}, siweAllowedHosts())

	foreign := SIWEMessage{Domain: "hosted.example", URI: "https://hosted.example", Version: "1", IssuedAt: time.Now()}
	assert.ErrorIs(t, foreign.ValidateForSite(time.Now()), ErrSIWEDomain)

	SetSIWETrustedOrigins(strings.Split(" https://www.pay.example.test , ,not a url", ","))
	assert.Equal(t, map[string]struct{}{"pay.example.test": {}, "www.pay.example.test": {}}, siweAllowedHosts())
}

// Low (challenge DoS): wallet challenges are stateless, so a flood of
// /auth/challenge calls from many clients stores nothing (Len stays 0, so no
// count of issued challenges can fill a pool) and cannot stop a fresh wallet
// from signing in. The previous store refused every caller once 100k
// challenges were pending.
func TestGenerateChallenge_FloodFromManyClientsDoesNotBlockSignIn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRealmSwitchOwnerDB(t)
	t.Setenv("PUBLIC_URL", "https://pay.example.test")

	for i := 0; i < 500; i++ {
		issueTestChallenge(t, fmt.Sprintf("0x%040x", i+1))
	}
	for i := 0; i < 5_000; i++ {
		_, err := ChallengeStore.Issue(fmt.Sprintf("0x%040x", i+1000), WalletChallengeTTL)
		require.NoError(t, err)
	}
	assert.Zero(t, ChallengeStore.Len(), "issuing challenges stores nothing")

	key, address := newSIWEWallet(t)
	message := buildSIWE(address, siweFixture{domain: "pay.example.test", uri: "https://pay.example.test", nonce: issueTestChallenge(t, address), issuedAt: nowISO(0)})
	w := postSignIn(t, message, signSIWE(t, key, message))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, ChallengeStore.Len(), "only the redeemed nonce is remembered")

	// A nonce this server never issued is refused even with a valid signature.
	forged := buildSIWE(address, siweFixture{domain: "pay.example.test", uri: "https://pay.example.test", nonce: strings.Repeat("ab", 40), issuedAt: nowISO(0)})
	w = postSignIn(t, forged, signSIWE(t, key, forged))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Invalid or expired challenge")
}

// A failed signature does not spend the nonce: only a verified signature
// records it, so junk sign-in attempts cannot grow server state either.
func TestSignIn_BadSignatureDoesNotSpendTheNonce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRealmSwitchOwnerDB(t)
	t.Setenv("PUBLIC_URL", "https://pay.example.test")

	key, address := newSIWEWallet(t)
	otherKey, _ := crypto.GenerateKey()
	message := buildSIWE(address, siweFixture{domain: "pay.example.test", uri: "https://pay.example.test", nonce: issueTestChallenge(t, address), issuedAt: nowISO(0)})

	w := postSignIn(t, message, signSIWE(t, otherKey, message))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Invalid signature")
	assert.Zero(t, ChallengeStore.Len())

	w = postSignIn(t, message, signSIWE(t, key, message))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// Follow-up F-2 / S-Low: when the redeemed-nonce set is full of live entries
// (a flood of throwaway-key sign-ins), a valid wallet sign-in still succeeds:
// the store evicts its oldest entries behind an expiry floor instead of
// answering 503 to everyone, and the set never grows past its cap.
func TestSignIn_SucceedsWhenChallengeStoreFull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRealmSwitchOwnerDB(t)
	t.Setenv("PUBLIC_URL", "https://pay.example.test")
	ChallengeStore.SetMaxRedeemed(1)
	t.Cleanup(func() { ChallengeStore.SetMaxRedeemed(0) })
	require.True(t, ChallengeStore.Redeem(logic.Challenge{Value: "filler-" + nowISO(0), ExpiresAt: time.Now().Add(time.Minute)}))

	key, address := newSIWEWallet(t)
	message := buildSIWE(address, siweFixture{
		domain: "pay.example.test", uri: "https://pay.example.test", nonce: issueTestChallenge(t, address), issuedAt: nowISO(-30 * time.Second),
	})
	w := postSignIn(t, message, signSIWE(t, key, message))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, ChallengeStore.Len())
}
