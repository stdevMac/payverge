package auth

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/logic"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// M-siwe: wallet linking verifies the same EIP-4361 message as sign-in, so it
// must also reject a message for another site and consume the challenge
// atomically (it used to Get, then Delete).
func TestLinkWallet_ValidatesSIWEDomainAndConsumesNonceOnce(t *testing.T) {
	t.Setenv("PUBLIC_URL", "https://pay.example.test")
	prevStore := server.ChallengeStore
	server.ChallengeStore = logic.NewChallengeStore()
	t.Cleanup(func() { server.ChallengeStore = prevStore })

	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	address := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())

	issue := func() string {
		challenge, err := server.ChallengeStore.Issue(address, 5*time.Minute)
		require.NoError(t, err)
		return challenge.Value
	}
	build := func(domain, nonce string) (string, string) {
		message := fmt.Sprintf("%s wants you to sign in with your Ethereum account:\n%s\n\nLink this wallet to your Payverge account.\n\nURI: https://%s\nVersion: 1\nChain ID: #1\nNonce: %s\nIssued At: %s",
			domain, address, domain, nonce, time.Now().UTC().Format(time.RFC3339))
		prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)
		sig, err := crypto.Sign(crypto.Keccak256([]byte(prefixed)), key)
		require.NoError(t, err)
		sig[64] += 27
		return message, hexutil.Encode(sig)
	}
	link := func(message, signature string) (int, string) {
		h, _ := newAuthEnvelopeHandler(t)
		w, c := postJSON(t, LinkWalletRequest{Address: address, Message: message, Signature: signature})
		c.Set("user_id", uint(7))
		h.LinkWallet(c)
		return w.Code, w.Body.String()
	}

	message, signature := build("evil.example", issue())
	code, body := link(message, signature)
	assert.Equal(t, http.StatusBadRequest, code, body)
	assert.Contains(t, body, "not for this site")

	message, signature = build("pay.example.test", issue())
	code, body = link(message, signature)
	require.Equal(t, http.StatusOK, code, body)

	code, body = link(message, signature)
	assert.Equal(t, http.StatusBadRequest, code, body)
	assert.Contains(t, body, "Invalid or expired challenge", "the nonce is single use")
}
