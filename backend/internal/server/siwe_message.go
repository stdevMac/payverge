package server

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"

	"github.com/gin-gonic/gin"
)

// Wallet sign-in messages follow EIP-4361 (Sign-In with Ethereum), built by
// frontend/src/utils/walletAuth.ts:
//
//	${host} wants you to sign in with your Ethereum account:
//	${address}
//
//	${statement}
//
//	URI: ${origin}
//	Version: 1
//	Chain ID: #${chainId}
//	Nonce: ${challenge}
//	Issued At: ${iso8601}
//
// M-siwe: the parser used to read only the address, the nonce and the chain
// ID. A phishing page that relayed our challenge could therefore have the
// victim sign "evil.example wants you to sign in ..." and the backend would
// accept it. The domain and URI host must now name this site, and the
// Issued At / Expiration Time / Not Before fields are checked with skew.

const (
	siweHeaderSuffix = " wants you to sign in with your Ethereum account:"

	// WalletChallengeTTL is how long an issued wallet challenge stays valid.
	WalletChallengeTTL = 5 * time.Minute
	// SIWEClockSkew is the tolerance applied to client-supplied timestamps.
	SIWEClockSkew = 5 * time.Minute
)

var (
	ErrSIWEMalformed     = errors.New("malformed sign-in message")
	ErrSIWEDomain        = errors.New("sign-in message is not for this site")
	ErrSIWEStale         = errors.New("sign-in message is expired or not yet valid")
	errSIWEDuplicateLine = errors.New("duplicate sign-in message field")
)

// SIWEMessage is a parsed EIP-4361 message.
type SIWEMessage struct {
	Domain         string
	Address        string // lower-case
	URI            string
	Version        string
	ChainID        string // without a leading '#'
	Nonce          string
	IssuedAt       time.Time
	ExpirationTime *time.Time
	NotBefore      *time.Time
}

// ParseSIWEMessage parses an EIP-4361 message. Each field may appear once.
func ParseSIWEMessage(message string) (SIWEMessage, error) {
	lines := strings.Split(strings.ReplaceAll(message, "\r\n", "\n"), "\n")
	if len(lines) < 8 {
		return SIWEMessage{}, ErrSIWEMalformed
	}
	header := strings.TrimSpace(lines[0])
	if !strings.HasSuffix(header, siweHeaderSuffix) {
		return SIWEMessage{}, ErrSIWEMalformed
	}
	msg := SIWEMessage{
		Domain:  strings.TrimSpace(strings.TrimSuffix(header, siweHeaderSuffix)),
		Address: strings.ToLower(strings.TrimSpace(lines[1])),
	}

	seen := make(map[string]bool, 7)
	var issuedAt, expiration, notBefore string
	fields := []struct {
		prefix string
		dst    *string
	}{
		{"URI:", &msg.URI},
		{"Version:", &msg.Version},
		{"Chain ID:", &msg.ChainID},
		{"Nonce:", &msg.Nonce},
		{"Issued At:", &issuedAt},
		{"Expiration Time:", &expiration},
		{"Not Before:", &notBefore},
	}
	for _, line := range lines[2:] {
		trimmed := strings.TrimSpace(line)
		for _, f := range fields {
			if !strings.HasPrefix(trimmed, f.prefix) {
				continue
			}
			if seen[f.prefix] {
				return SIWEMessage{}, errSIWEDuplicateLine
			}
			seen[f.prefix] = true
			*f.dst = strings.TrimSpace(strings.TrimPrefix(trimmed, f.prefix))
			break
		}
	}
	msg.ChainID = strings.TrimPrefix(msg.ChainID, "#")

	if msg.Domain == "" || msg.Address == "" || msg.URI == "" || msg.Nonce == "" ||
		msg.ChainID == "" || issuedAt == "" {
		return SIWEMessage{}, ErrSIWEMalformed
	}
	var err error
	if msg.IssuedAt, err = time.Parse(time.RFC3339Nano, issuedAt); err != nil {
		return SIWEMessage{}, ErrSIWEMalformed
	}
	if expiration != "" {
		t, perr := time.Parse(time.RFC3339Nano, expiration)
		if perr != nil {
			return SIWEMessage{}, ErrSIWEMalformed
		}
		msg.ExpirationTime = &t
	}
	if notBefore != "" {
		t, perr := time.Parse(time.RFC3339Nano, notBefore)
		if perr != nil {
			return SIWEMessage{}, ErrSIWEMalformed
		}
		msg.NotBefore = &t
	}
	return msg, nil
}

// ValidateForSite checks that the message was produced for this site and is
// fresh at now:
//   - the domain and the URI host equal the public site host (PUBLIC_URL,
//     falling back to the frontend URL config) or a trusted CORS origin;
//   - Version is 1;
//   - Issued At lies in [now - challenge TTL - skew, now + skew];
//   - Expiration Time (if present) has not passed and Not Before (if
//     present) has been reached, both with skew.
func (m SIWEMessage) ValidateForSite(now time.Time) error {
	allowed := siweAllowedHosts()
	if _, ok := allowed[normalizeSIWEHost(m.Domain, "")]; !ok {
		return ErrSIWEDomain
	}
	u, err := url.Parse(m.URI)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ErrSIWEDomain
	}
	if _, ok := allowed[normalizeSIWEHost(u.Host, u.Scheme)]; !ok {
		return ErrSIWEDomain
	}
	if m.Version != "1" {
		return ErrSIWEMalformed
	}
	if m.IssuedAt.After(now.Add(SIWEClockSkew)) ||
		m.IssuedAt.Before(now.Add(-(WalletChallengeTTL + SIWEClockSkew))) {
		return ErrSIWEStale
	}
	if m.ExpirationTime != nil && now.After(m.ExpirationTime.Add(SIWEClockSkew)) {
		return ErrSIWEStale
	}
	if m.NotBefore != nil && now.Add(SIWEClockSkew).Before(*m.NotBefore) {
		return ErrSIWEStale
	}
	return nil
}

// ParseAndValidateSIWE parses message and validates it for this site at the
// current time. On failure it writes a 400 envelope and returns ok=false.
// Used by wallet sign-in and wallet linking.
func ParseAndValidateSIWE(c *gin.Context, message string) (SIWEMessage, bool) {
	msg, err := ParseSIWEMessage(message)
	if err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Invalid message")
		return SIWEMessage{}, false
	}
	switch err := msg.ValidateForSite(time.Now()); {
	case err == nil:
		return msg, true
	case errors.Is(err, ErrSIWEDomain):
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Sign-in message is not for this site")
	case errors.Is(err, ErrSIWEStale):
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Sign-in message is expired or not yet valid, check your device clock")
	default:
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Invalid message")
	}
	return SIWEMessage{}, false
}

var (
	siweTrustedOriginsMu sync.RWMutex
	siweTrustedOrigins   []string
)

// SetSIWETrustedOrigins registers the explicitly configured ALLOWED_ORIGINS as
// additional acceptable SIWE domains. Those origins are trusted to make
// credentialed auth mutations, so a page served from one of them is
// first-party. main.go passes the raw env list, not the resolved CORS list:
// the latter carries built-in defaults (the hosted product's domains) that a
// self-hosted instance must not accept sign-in messages for. Blank and
// unparseable entries are ignored.
func SetSIWETrustedOrigins(origins []string) {
	siweTrustedOriginsMu.Lock()
	defer siweTrustedOriginsMu.Unlock()
	siweTrustedOrigins = append([]string(nil), origins...)
}

// siwePublicURL returns the canonical public origin of this deployment via
// the instance accessor: PUBLIC_URL, else http://localhost:3000.
func siwePublicURL() string {
	return config.PublicURL()
}

func siweAllowedHosts() map[string]struct{} {
	hosts := make(map[string]struct{}, 4)
	add := func(origin string) {
		u, err := url.Parse(strings.TrimSpace(origin))
		if err != nil || u.Host == "" {
			return
		}
		hosts[normalizeSIWEHost(u.Host, u.Scheme)] = struct{}{}
	}
	add(siwePublicURL())
	siweTrustedOriginsMu.RLock()
	for _, origin := range siweTrustedOrigins {
		add(origin)
	}
	siweTrustedOriginsMu.RUnlock()
	return hosts
}

// normalizeSIWEHost lower-cases an RFC 3986 authority and drops the default
// port of scheme (browsers omit it from window.location.host). With no scheme
// (the SIWE domain line) both default ports are dropped.
func normalizeSIWEHost(host, scheme string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	switch {
	case (scheme == "https" || scheme == "") && strings.HasSuffix(host, ":443"):
		host = strings.TrimSuffix(host, ":443")
	case (scheme == "http" || scheme == "") && strings.HasSuffix(host, ":80"):
		host = strings.TrimSuffix(host, ":80")
	}
	return host
}
