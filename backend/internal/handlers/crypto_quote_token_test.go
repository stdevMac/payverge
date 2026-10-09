package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCryptoQuoteToken_RoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	now := time.Unix(1_700_000_000, 0)
	claims := cryptoQuoteClaims{
		BillID:        7,
		LocalCents:    10_000,
		USDMicrounits: 108_000_000,
		Exp:           now.Add(30 * time.Minute).Unix(),
	}
	token := signCryptoQuote(claims, secret)
	got, err := parseCryptoQuote(token, secret, now)
	require.NoError(t, err)
	require.Equal(t, claims, got)
}

func TestCryptoQuoteToken_RejectsTamperedPayload(t *testing.T) {
	secret := []byte("test-secret")
	now := time.Unix(1_700_000_000, 0)
	token := signCryptoQuote(cryptoQuoteClaims{BillID: 1, LocalCents: 100, USDMicrounits: 1_000_000, Exp: now.Add(time.Minute).Unix()}, secret)
	// Flip the first character of the payload segment.
	parts := strings.SplitN(token, ".", 2)
	bad := flipFirstRune(parts[0]) + "." + parts[1]
	_, err := parseCryptoQuote(bad, secret, now)
	require.ErrorIs(t, err, errQuoteInvalid)
}

func TestCryptoQuoteToken_RejectsWrongSecret(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	token := signCryptoQuote(cryptoQuoteClaims{BillID: 1, LocalCents: 100, USDMicrounits: 1_000_000, Exp: now.Add(time.Minute).Unix()}, []byte("secret-a"))
	_, err := parseCryptoQuote(token, []byte("secret-b"), now)
	require.ErrorIs(t, err, errQuoteInvalid)
}

func TestCryptoQuoteToken_RejectsExpired(t *testing.T) {
	secret := []byte("test-secret")
	now := time.Unix(1_700_000_000, 0)
	token := signCryptoQuote(cryptoQuoteClaims{BillID: 1, LocalCents: 100, USDMicrounits: 1_000_000, Exp: now.Add(-time.Second).Unix()}, secret)
	_, err := parseCryptoQuote(token, secret, now)
	require.ErrorIs(t, err, errQuoteExpired)
}

func TestCryptoQuoteToken_ExpiryBoundary(t *testing.T) {
	secret := []byte("test-secret")
	now := time.Unix(1_700_000_000, 0)
	mk := func(exp int64) string {
		return signCryptoQuote(cryptoQuoteClaims{BillID: 1, LocalCents: 100, USDMicrounits: 1_000_000, Exp: exp}, secret)
	}

	// exp one second in the future: valid.
	_, err := parseCryptoQuote(mk(now.Unix()+1), secret, now)
	require.NoError(t, err)

	// exp exactly == now: expired (now >= exp).
	_, err = parseCryptoQuote(mk(now.Unix()), secret, now)
	require.ErrorIs(t, err, errQuoteExpired)

	// exp one second in the past: expired.
	_, err = parseCryptoQuote(mk(now.Unix()-1), secret, now)
	require.ErrorIs(t, err, errQuoteExpired)
}

func flipFirstRune(s string) string {
	if s == "" {
		return s
	}
	if s[0] == 'A' {
		return "B" + s[1:]
	}
	return "A" + s[1:]
}
