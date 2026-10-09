package signedid

import (
	"strings"
	"testing"
)

func TestSigner_MintedIDsVerify(t *testing.T) {
	s := NewWithSecret("test", "cs_", []byte("k1"))
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id, ok := s.Mint()
		if !ok {
			t.Fatal("mint failed")
		}
		if len(id) != s.Len() || len(id) != 60 {
			t.Fatalf("unexpected length %d for %q", len(id), id)
		}
		if !s.Valid(id) {
			t.Fatalf("minted id %q did not verify", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestSigner_RejectsForgedAndForeignIDs(t *testing.T) {
	s := NewWithSecret("test", "cs_", []byte("k1"))
	id, _ := s.Mint()

	other := NewWithSecret("test", "cs_", []byte("k2"))
	otherID, _ := other.Mint()
	otherDomain := NewWithSecret("other", "cs_", []byte("k1"))
	domainID, _ := otherDomain.Mint()

	flipped := []byte(id)
	if flipped[10] == 'a' {
		flipped[10] = 'b'
	} else {
		flipped[10] = 'a'
	}

	for name, bad := range map[string]string{
		"empty":           "",
		"legacy unsigned": "cs_0123456789abcdef0123456789abcdef",
		"client local":    "local_1712345678901",
		"other key":       otherID,
		"other domain":    domainID,
		"body tampered":   string(flipped),
		"uppercase":       strings.ToUpper(id[:3]) + id[3:],
		"uppercase hex":   id[:3] + strings.ToUpper(id[3:]),
		"truncated":       id[:len(id)-1],
		"extended":        id + "0",
		"wrong separator": id[:35] + "-" + id[36:],
		"wrong prefix":    "dv_" + id[3:],
		"whitespace":      " " + id[:len(id)-1],
		"tag of zeros":    id[:36] + strings.Repeat("0", 24),
		"non-hex body":    "cs_" + strings.Repeat("g", 32) + id[35:],
	} {
		if s.Valid(bad) {
			t.Errorf("%s: %q must not verify", name, bad)
		}
	}
}

func TestSigner_EnvKeyIsStableAcrossInstances(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "stable-secret")
	a := New("concierge-session", "cs_")
	b := New("concierge-session", "cs_")
	id, ok := a.Mint()
	if !ok {
		t.Fatal("mint failed")
	}
	if !b.Valid(id) {
		t.Fatal("a second process with the same JWT_SECRET_KEY must accept the id (replicas, restarts)")
	}
}

func TestSigner_NoEnvKeyStillUnforgeable(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "")
	a := New("concierge-session", "cs_")
	b := New("concierge-session", "cs_")
	id, ok := a.Mint()
	if !ok || !a.Valid(id) {
		t.Fatal("process-random key must still mint and verify")
	}
	if b.Valid(id) {
		t.Fatal("independent random keys must not accept each other's ids")
	}
}
