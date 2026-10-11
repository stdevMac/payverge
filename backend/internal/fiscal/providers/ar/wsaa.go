package ar

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.mozilla.org/pkcs7"
	"golang.org/x/sync/singleflight"
)

// loginTicketRequest models the WSAA LTR. AFIP requires generationTime in the
// recent past and expirationTime in the near future; uniqueId distinguishes
// concurrent requests within the same second.
type loginTicketRequest struct {
	XMLName xml.Name `xml:"loginTicketRequest"`
	Version string   `xml:"version,attr"`
	Header  struct {
		UniqueID       int64  `xml:"uniqueId"`
		GenerationTime string `xml:"generationTime"`
		ExpirationTime string `xml:"expirationTime"`
	} `xml:"header"`
	Service string `xml:"service"`
}

func buildLoginTicketRequest(service string, generatedAt time.Time, ttl time.Duration) (string, int64, error) {
	if service == "" {
		return "", 0, fmt.Errorf("service is required")
	}
	gen := generatedAt.UTC()
	uniqueID := gen.Unix()
	var ltr loginTicketRequest
	ltr.Version = "1.0"
	ltr.Header.UniqueID = uniqueID
	ltr.Header.GenerationTime = gen.Format("2006-01-02T15:04:05Z07:00")
	ltr.Header.ExpirationTime = gen.Add(ttl).Format("2006-01-02T15:04:05Z07:00")
	ltr.Service = service
	out, err := xml.Marshal(&ltr)
	if err != nil {
		return "", 0, fmt.Errorf("marshal LTR: %w", err)
	}
	return xml.Header + string(out), uniqueID, nil
}

// WSAACredentials holds the short-lived token/sign pair returned by WSAA.
type WSAACredentials struct {
	Token     string
	Sign      string
	ExpiresAt time.Time
}

// signLoginTicketRequest CMS-signs the raw LTR XML with the operator's cert+key
// and returns the base64-encoded DER blob expected by WSAA's loginCms operation.
func signLoginTicketRequest(ltr []byte, cert *x509.Certificate, key *rsa.PrivateKey) (string, error) {
	signed, err := pkcs7.NewSignedData(ltr)
	if err != nil {
		return "", fmt.Errorf("new signed data: %w", err)
	}
	if err := signed.AddSigner(cert, key, pkcs7.SignerInfoConfig{}); err != nil {
		return "", fmt.Errorf("add signer: %w", err)
	}
	der, err := signed.Finish()
	if err != nil {
		return "", fmt.Errorf("finish CMS: %w", err)
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

const loginCmsEnvelope = `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:wsaa="https://wsaa.afip.gov.ar/ws/services/LoginCms">
<soapenv:Body><wsaa:loginCms><wsaa:in0>%s</wsaa:in0></wsaa:loginCms></soapenv:Body></soapenv:Envelope>`

// parseLoginCmsResponse extracts WSAACredentials from the SOAP loginCmsResponse body.
// WSAA nests an XML-escaped loginTicketResponse inside the loginCmsReturn element.
func parseLoginCmsResponse(body []byte) (*WSAACredentials, error) {
	var soap struct {
		Return string `xml:"Body>loginCmsResponse>loginCmsReturn"`
	}
	if err := xml.Unmarshal(body, &soap); err != nil {
		return nil, fmt.Errorf("unmarshal soap: %w", err)
	}
	if soap.Return == "" {
		return nil, fmt.Errorf("loginCmsReturn empty (WSAA fault?): %s", string(body))
	}
	var ltr struct {
		Token      string `xml:"credentials>token"`
		Sign       string `xml:"credentials>sign"`
		Expiration string `xml:"header>expirationTime"`
	}
	if err := xml.Unmarshal([]byte(soap.Return), &ltr); err != nil {
		return nil, fmt.Errorf("unmarshal ticket: %w", err)
	}
	exp, err := time.Parse("2006-01-02T15:04:05Z07:00", ltr.Expiration)
	if err != nil {
		return nil, fmt.Errorf("parse expiry %q: %w", ltr.Expiration, err)
	}
	return &WSAACredentials{Token: ltr.Token, Sign: ltr.Sign, ExpiresAt: exp}, nil
}

// WSAAClient authenticates against AFIP's WSAA and caches credentials until
// they are within 5 minutes of expiry.
//
// Concurrent cache misses coalesce into a single SOAP call via singleflight;
// the cache mutex is held only for the brief read/write of the cached value,
// never across the network round-trip.
type WSAAClient struct {
	endpoint string
	cert     *x509.Certificate
	key      *rsa.PrivateKey
	http     *http.Client
	now      func() time.Time

	// fetch performs the real (or injected-test) SOAP login. It is called at
	// most once per cache miss regardless of concurrent callers.
	fetch func(ctx context.Context) (*WSAACredentials, error)

	mu    sync.Mutex
	cache *WSAACredentials
	sf    singleflight.Group
}

// NewWSAAClient creates a client that will call the given WSAA endpoint URL.
func NewWSAAClient(endpoint string, cert *x509.Certificate, key *rsa.PrivateKey) *WSAAClient {
	c := &WSAAClient{
		endpoint: endpoint,
		cert:     cert,
		key:      key,
		http:     &http.Client{Timeout: 20 * time.Second},
		now:      time.Now,
	}
	c.fetch = c.soapLogin
	return c
}

// Authenticate returns a valid WSAACredentials, re-using the in-memory cache
// when the token has more than 5 minutes of remaining lifetime.
//
// Concurrent cache misses coalesce into a single fetch invocation. The shared
// fetch runs under a context detached from the caller that started it, with
// its own 30s timeout, so cancelling that caller does not fail everyone else
// waiting on the same login. Each caller still returns promptly with its own
// ctx.Err() when its context is cancelled; the fetch continues and fills the
// cache. The cache mutex is held only briefly for the read and write — never
// across the network.
func (c *WSAAClient) Authenticate(ctx context.Context) (*WSAACredentials, error) {
	// Fast path: return cached token without touching the network.
	c.mu.Lock()
	if c.cache != nil && c.now().Add(5*time.Minute).Before(c.cache.ExpiresAt) {
		creds := c.cache
		c.mu.Unlock()
		return creds, nil
	}
	c.mu.Unlock()

	// Slow path: at most one SOAP fetch per endpoint. DoChan gives every caller
	// its own result channel so a cancelled caller can leave without abandoning
	// the shared fetch. The cache mutex is NOT held during this call.
	ch := c.sf.DoChan(c.endpoint, func() (interface{}, error) {
		// Re-check: a caller that missed the cache just before the previous
		// flight stored its result must not start a second WSAA login.
		c.mu.Lock()
		if c.cache != nil && c.now().Add(5*time.Minute).Before(c.cache.ExpiresAt) {
			creds := c.cache
			c.mu.Unlock()
			return creds, nil
		}
		c.mu.Unlock()
		// WithoutCancel drops the leader's cancellation/deadline. The leader's
		// ctx is only a values parent; a 30s timeout bounds the shared login.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		fetched, ferr := c.fetch(fctx)
		if ferr != nil {
			return nil, ferr
		}
		c.mu.Lock()
		c.cache = fetched
		c.mu.Unlock()
		return fetched, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.(*WSAACredentials), nil
	}
}

// loginTicketRequest builds the WSAA login ticket XML. generationTime is five
// minutes behind c.now() and the ticket lives 15 minutes from that instant
// (expirationTime = now+10m) so a clock-skewed AFIP still accepts it.
func (c *WSAAClient) loginTicketRequest() (string, error) {
	now := c.now()
	ltr, _, err := buildLoginTicketRequest("wsfe", now.Add(-5*time.Minute), 15*time.Minute)
	return ltr, err
}

// soapLogin is the default fetch implementation: builds, signs, and submits the
// WSAA loginCms SOAP request over the network.
func (c *WSAAClient) soapLogin(ctx context.Context) (*WSAACredentials, error) {
	ltr, err := c.loginTicketRequest()
	if err != nil {
		return nil, err
	}
	signed, err := signLoginTicketRequest([]byte(ltr), c.cert, c.key)
	if err != nil {
		return nil, err
	}
	reqBody := fmt.Sprintf(loginCmsEnvelope, signed)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBufferString(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wsaa call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wsaa read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wsaa HTTP %d: %s", resp.StatusCode, truncateForError(body, 200))
	}
	return parseLoginCmsResponse(body)
}

// truncateForError returns up to n bytes of b as a string, suitable for
// embedding in error messages without blowing up log lines.
func truncateForError(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n])
	}
	return string(b)
}
