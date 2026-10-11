package ar

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/xml"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newSelfSignedCert generates a minimal self-signed cert+key for testing.
func newSelfSignedCert(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	return cert, key
}

// loginCmsFixtureWithCredentials builds a minimal WSAA SOAP loginCmsResponse.
// The inner loginTicketResponse is XML-escaped and placed as text content of
// loginCmsReturn, matching real WSAA behaviour.
func loginCmsFixtureWithCredentials(token, sign, expiry string) string {
	inner := "<loginTicketResponse>" +
		"<header><expirationTime>" + expiry + "</expirationTime></header>" +
		"<credentials><token>" + token + "</token><sign>" + sign + "</sign></credentials>" +
		"</loginTicketResponse>"
	// xml.EscapeText writes to a writer; use encoding/xml Marshal to produce escaped text.
	type wrapper struct {
		XMLName xml.Name `xml:"loginCmsReturn"`
		Content string   `xml:",chardata"`
	}
	b, _ := xml.Marshal(wrapper{Content: inner})
	// b is <loginCmsReturn>ESCAPED_INNER</loginCmsReturn>
	// We only need the inner text, so trim the tags.
	escapedInner := string(b)
	escapedInner = strings.TrimPrefix(escapedInner, "<loginCmsReturn>")
	escapedInner = strings.TrimSuffix(escapedInner, "</loginCmsReturn>")

	return `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">` +
		`<soapenv:Body>` +
		`<loginCmsResponse>` +
		`<loginCmsReturn>` + escapedInner + `</loginCmsReturn>` +
		`</loginCmsResponse>` +
		`</soapenv:Body>` +
		`</soapenv:Envelope>`
}

// ---------------------------------------------------------------------------
// Existing tests (Task 1)
// ---------------------------------------------------------------------------

func TestBuildLoginTicketRequest(t *testing.T) {
	gen := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	ltrXML, uniqueID, err := buildLoginTicketRequest("wsfe", gen, 10*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uniqueID == 0 {
		t.Fatal("uniqueId must be non-zero")
	}
	if !strings.HasPrefix(ltrXML, "<?xml") {
		t.Errorf("missing XML header preamble: %s", ltrXML)
	}
	if !strings.Contains(ltrXML, "<service>wsfe</service>") {
		t.Errorf("missing service tag: %s", ltrXML)
	}
	if !strings.Contains(ltrXML, "<generationTime>2026-06-19T12:00:00") {
		t.Errorf("missing/incorrect generationTime: %s", ltrXML)
	}
	if !strings.Contains(ltrXML, "<expirationTime>2026-06-19T12:10:00") {
		t.Errorf("missing/incorrect expirationTime: %s", ltrXML)
	}
}

func TestLoginTicketRequestSkewsClock(t *testing.T) {
	cert, key := newSelfSignedCert(t)
	c := NewWSAAClient("https://wsaa.test.example/login", cert, key)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	ltr, err := c.loginTicketRequest()
	if err != nil {
		t.Fatalf("loginTicketRequest: %v", err)
	}
	if !strings.Contains(ltr, "<generationTime>2026-06-19T11:55:00Z</generationTime>") {
		t.Errorf("generationTime should be 5 minutes before c.now(), got %s", ltr)
	}
	if !strings.Contains(ltr, "<expirationTime>2026-06-19T12:10:00Z</expirationTime>") {
		t.Errorf("expirationTime should be 10 minutes after c.now(), got %s", ltr)
	}
}

func TestBuildLoginTicketRequest_EmptyService(t *testing.T) {
	gen := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	if _, _, err := buildLoginTicketRequest("", gen, 10*time.Minute); err == nil {
		t.Fatal("expected error for empty service")
	}
}

// ---------------------------------------------------------------------------
// Task 2 tests
// ---------------------------------------------------------------------------

func TestSignLoginTicketRequest(t *testing.T) {
	cert, key := newSelfSignedCert(t)
	signed, err := signLoginTicketRequest([]byte("<loginTicketRequest/>"), cert, key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if len(signed) == 0 {
		t.Fatal("signed CMS must be non-empty")
	}
}

func TestParseLoginCmsResponse(t *testing.T) {
	raw := loginCmsFixtureWithCredentials("TOKEN123", "SIGN456", "2026-06-19T23:59:59-03:00")
	creds, err := parseLoginCmsResponse([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if creds.Token != "TOKEN123" || creds.Sign != "SIGN456" {
		t.Errorf("got token=%q sign=%q", creds.Token, creds.Sign)
	}
	if creds.ExpiresAt.IsZero() {
		t.Error("expiry must be parsed")
	}
}

func TestWSAAAuthenticateUsesCache(t *testing.T) {
	cert, key := newSelfSignedCert(t)

	var hits atomic.Int32
	future := time.Now().Add(2 * time.Hour).Format("2006-01-02T15:04:05-07:00")
	fixture := loginCmsFixtureWithCredentials("CACHED_TOKEN", "CACHED_SIGN", future)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer srv.Close()

	client := NewWSAAClient(srv.URL, cert, key)

	// First call — should hit the server.
	creds1, err := client.Authenticate(t.Context())
	if err != nil {
		t.Fatalf("first Authenticate: %v", err)
	}
	if creds1.Token != "CACHED_TOKEN" {
		t.Errorf("unexpected token: %q", creds1.Token)
	}
	if hits.Load() != 1 {
		t.Errorf("expected 1 server hit after first call, got %d", hits.Load())
	}

	// Second call — should return from cache, no additional server hit.
	creds2, err := client.Authenticate(t.Context())
	if err != nil {
		t.Fatalf("second Authenticate: %v", err)
	}
	if creds2.Token != "CACHED_TOKEN" {
		t.Errorf("unexpected token on second call: %q", creds2.Token)
	}
	if hits.Load() != 1 {
		t.Errorf("expected cache hit (still 1 server hit), got %d", hits.Load())
	}
}

// TestWSAAAuthenticateCacheEviction proves that the 5-minute boundary causes a
// cache miss: when now() is within 5 minutes of expiry the client must fetch a
// fresh token rather than returning the cached one.
func TestWSAAAuthenticateCacheEviction(t *testing.T) {
	cert, key := newSelfSignedCert(t)

	// frozen "now" — mutable so we can advance it between calls.
	frozenNow := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)

	// Expiry is 4 minutes after the initial frozen now: 12:04:00Z.
	// That means now()+5min = 12:05:00Z > 12:04:00Z → cache is stale immediately.
	expiry := frozenNow.Add(4 * time.Minute).Format("2006-01-02T15:04:05Z")
	fixture := loginCmsFixtureWithCredentials("EVICT_TOKEN", "EVICT_SIGN", expiry)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fixture))
	}))
	defer srv.Close()

	client := NewWSAAClient(srv.URL, cert, key)
	// Override the clock to a frozen value via the mutable closure.
	client.now = func() time.Time { return frozenNow }

	// First call: now=12:00:00, expiry=12:04:00, now+5min=12:05:00 > expiry → stale → hit server.
	if _, err := client.Authenticate(t.Context()); err != nil {
		t.Fatalf("first Authenticate: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("expected 1 server hit after first call, got %d", hits.Load())
	}

	// Advance frozen now to 12:01:00 — still within 5 min of expiry (12:04:00).
	// now+5min = 12:06:00 > 12:04:00 → still stale → must hit server again.
	frozenNow = frozenNow.Add(time.Minute)

	if _, err := client.Authenticate(t.Context()); err != nil {
		t.Fatalf("second Authenticate: %v", err)
	}
	if hits.Load() != 2 {
		t.Errorf("expected 2 server hits after cache eviction, got %d", hits.Load())
	}
}
