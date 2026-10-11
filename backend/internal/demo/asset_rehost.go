package demo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/s3"
)

// AssetRehoster copies a third-party demo image into storage we control and
// hands back the URL we own.
//
// Demo gallery photos shipped as Unsplash URLs. That is two problems on a
// surface built to sell: the stock CDN is outside our control and has 503'd
// mid-demo, and "Mejorar esta foto" is permanently dead on those rows because
// the AI-cleanup SSRF allowlist refuses stock hosts by design
// (see frontend cleanupPhotoEligibility.ts). Re-hosting at seed time fixes
// both without loosening the allowlist. (L4-20)
//
// Rehost may do network I/O and must never be called inside a transaction.
// Hosted is the cache-only lookup that seeding uses while a transaction is
// open.
type AssetRehoster interface {
	Rehost(ctx context.Context, sourceURL string) (string, error)
	Hosted(sourceURL string) (string, bool)
}

const (
	rehostFolder      = "demo-arg/assets"
	rehostMaxBytes    = 8 << 20 // a demo hero is ~200KB; 8MB is a generous ceiling
	rehostFetchBudget = 20 * time.Second
)

// uploadFunc matches s3.UploadBytes so tests can supply their own sink.
type uploadFunc func(data []byte, name string, folderPath string, contentType string) (string, error)

// S3AssetRehoster fetches a source image once and uploads it to the public
// bucket under a key derived from the source URL, so repeat seeds across
// restarts converge on the same object instead of littering the bucket.
type S3AssetRehoster struct {
	upload uploadFunc
	client *http.Client

	mu     sync.Mutex
	hosted map[string]string
}

func NewS3AssetRehoster(upload uploadFunc) *S3AssetRehoster {
	if upload == nil {
		upload = s3.UploadBytes
	}
	return &S3AssetRehoster{
		upload: upload,
		client: rehostHTTPClient(),
		hosted: map[string]string{},
	}
}

// rehostHTTPClient mirrors the bounded client the s3 package uses for its own
// outbound calls: a demo seed must not hang on a stalled CDN.
func rehostHTTPClient() *http.Client {
	return &http.Client{
		Timeout: rehostFetchBudget,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}
}

func (r *S3AssetRehoster) Hosted(sourceURL string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	hosted, ok := r.hosted[sourceURL]
	return hosted, ok
}

func (r *S3AssetRehoster) Rehost(ctx context.Context, sourceURL string) (string, error) {
	if hosted, ok := r.Hosted(sourceURL); ok {
		return hosted, nil
	}

	data, contentType, err := r.fetch(ctx, sourceURL)
	if err != nil {
		return "", err
	}

	location, err := r.upload(data, rehostKey(sourceURL, contentType), rehostFolder, contentType)
	if err != nil {
		return "", fmt.Errorf("upload demo asset: %w", err)
	}
	if strings.TrimSpace(location) == "" {
		return "", fmt.Errorf("upload demo asset: empty location for %s", sourceURL)
	}

	r.mu.Lock()
	r.hosted[sourceURL] = location
	r.mu.Unlock()
	return location, nil
}

func (r *S3AssetRehoster) fetch(ctx context.Context, sourceURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build demo asset request: %w", err)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch demo asset: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("fetch demo asset %s: status %d", sourceURL, resp.StatusCode)
	}

	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("fetch demo asset %s: content-type %q is not an image", sourceURL, contentType)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, rehostMaxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read demo asset: %w", err)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("fetch demo asset %s: empty body", sourceURL)
	}
	if len(data) > rehostMaxBytes {
		return nil, "", fmt.Errorf("fetch demo asset %s: larger than %d bytes", sourceURL, rehostMaxBytes)
	}
	return data, contentType, nil
}

// rehostKey is deterministic in the source URL so re-seeding overwrites the
// same object rather than accumulating copies.
func rehostKey(sourceURL, contentType string) string {
	sum := sha256.Sum256([]byte(sourceURL))
	return hex.EncodeToString(sum[:])[:16] + rehostExtension(sourceURL, contentType)
}

func rehostExtension(sourceURL, contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	}
	if parsed, err := url.Parse(sourceURL); err == nil {
		if ext := strings.ToLower(path.Ext(parsed.Path)); ext != "" && len(ext) <= 5 {
			return ext
		}
	}
	return ".jpg"
}
