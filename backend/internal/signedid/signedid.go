// Package signedid mints and verifies opaque, server-signed random ids for
// anonymous public surfaces (today, the AI-waiter device cookie). An id is
//
//	<prefix><32 hex random (128 bits)>_<24 hex HMAC-SHA256 tag (96 bits)>
//
// so a client can carry it but cannot invent one: every id the server accepts
// was minted by this process family. The key is a domain-separated derivation
// of JWT_SECRET_KEY (required in production, so ids survive restarts and are
// shared across replicas); without it a process-random key is used, which
// keeps ids unforgeable but invalidates them on restart.
package signedid

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"sync"
)

const (
	randomBytes = 16
	tagHexLen   = 24
)

// Signer mints and verifies ids for one domain. Safe for concurrent use.
type Signer struct {
	domain string
	prefix string

	once sync.Once
	key  []byte
}

// New returns a Signer whose key is derived lazily (on first use, after the
// process environment is final) from JWT_SECRET_KEY and the domain label.
func New(domain, prefix string) *Signer {
	return &Signer{domain: domain, prefix: prefix}
}

// NewWithSecret returns a Signer with an explicit secret (tests, tooling).
func NewWithSecret(domain, prefix string, secret []byte) *Signer {
	s := &Signer{domain: domain, prefix: prefix}
	s.once.Do(func() { s.key = deriveKey(domain, secret) })
	return s
}

func deriveKey(domain string, secret []byte) []byte {
	sum := sha256.Sum256(append([]byte("payverge-"+domain+"-v1|"), secret...))
	return sum[:]
}

func (s *Signer) secret() []byte {
	s.once.Do(func() {
		if v := strings.TrimSpace(os.Getenv("JWT_SECRET_KEY")); v != "" {
			s.key = deriveKey(s.domain, []byte(v))
			return
		}
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err == nil {
			s.key = deriveKey(s.domain, buf)
		}
	})
	return s.key
}

// Len is the length of every id this signer mints.
func (s *Signer) Len() int { return len(s.prefix) + 2*randomBytes + 1 + tagHexLen }

// Mint returns a fresh signed id, or ok=false when no entropy/key is available
// (crypto/rand failure); callers must fail closed rather than fall back to a
// guessable id.
func (s *Signer) Mint() (string, bool) {
	key := s.secret()
	if len(key) == 0 {
		return "", false
	}
	buf := make([]byte, randomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", false
	}
	body := hex.EncodeToString(buf)
	return s.prefix + body + "_" + s.tag(key, body), true
}

// Valid reports whether id was minted by this signer: exact shape, lowercase
// hex, and a matching tag compared in constant time.
func (s *Signer) Valid(id string) bool {
	if len(id) != s.Len() || !strings.HasPrefix(id, s.prefix) {
		return false
	}
	rest := id[len(s.prefix):]
	body, tag := rest[:2*randomBytes], rest[2*randomBytes:]
	if tag[0] != '_' || !lowerHex(body) || !lowerHex(tag[1:]) {
		return false
	}
	key := s.secret()
	if len(key) == 0 {
		return false
	}
	return hmac.Equal([]byte(tag[1:]), []byte(s.tag(key, body)))
}

func (s *Signer) tag(key []byte, body string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(s.prefix))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))[:tagHexLen]
}

func lowerHex(v string) bool {
	if v == "" {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
