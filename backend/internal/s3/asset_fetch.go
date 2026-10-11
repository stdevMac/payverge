package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// publicHost is the host of the configured public bucket/CDN, captured at
// init. It is the allowlist used to keep server-side asset fetches pointed at
// our own storage (SSRF guard). Same-origin /media URLs are handled separately
// (IsOwnMediaURL) and never fetched over the network. Guarded by stateMu.
var publicHost string

// ErrDisallowedAssetURL is returned when a URL does not point at our own public
// asset bucket.
var ErrDisallowedAssetURL = errors.New("url is not an allowed public asset url")

// maxAssetBytes caps how much we will download for an enhancement source image.
const maxAssetBytes = 12 << 20 // 12 MiB

// PublicHost returns the configured public bucket/CDN host (empty for the
// local driver unless legacy S3_PUBLIC_BASE_URL rows are still trusted).
func PublicHost() string {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return publicHost
}

func setPublicHost(host string) {
	stateMu.Lock()
	publicHost = host
	stateMu.Unlock()
}

// configurePublicHost derives the public bucket host from the S3 config. This
// must match the host in the URLs that the upload helpers return and that the
// frontend serves (next.config.mjs remotePatterns), or the SSRF guard would
// reject every real stored image URL.
//
// When publicBaseURL is set (e.g. Cloudflare R2 served via a custom domain like
// "https://media.example.com"), the public host is that domain — the write
// endpoint and the read host differ there, and the upload helpers return
// "<publicBaseURL>/<key>", so the guard must allowlist the read host.
//
// Otherwise (legacy rows written before /media existed) the S3 client used
// virtual-host addressing: the object URL host is "<bucket>.<endpoint-host>"
// for a custom endpoint (e.g. iDrive e2: "<bucket>.j5f7.c18.e2-3.dev"), or the
// AWS virtual-hosted form when no custom endpoint is configured.
func configurePublicHost(bucketName, region, endpointURL, publicBaseURL string) {
	configurePublicHostStyle(bucketName, region, endpointURL, publicBaseURL, false)
}

// configurePublicHostStyle is configurePublicHost with path-style awareness:
// with S3_FORCE_PATH_STYLE the object host is the endpoint host itself.
func configurePublicHostStyle(bucketName, region, endpointURL, publicBaseURL string, pathStyle bool) {
	if publicBaseURL != "" {
		if u, err := url.Parse(publicBaseURL); err == nil && u.Host != "" {
			setPublicHost(u.Host)
			return
		}
	}
	if endpointURL != "" {
		if u, err := url.Parse(endpointURL); err == nil && u.Host != "" {
			if pathStyle {
				setPublicHost(u.Host)
			} else {
				setPublicHost(bucketName + "." + u.Host)
			}
			return
		}
	}
	setPublicHost(fmt.Sprintf("%s.s3.%s.amazonaws.com", bucketName, region))
}

// ValidatePublicAssetURL ensures rawURL is an https URL whose host equals
// allowedHost. Pure (host passed in) so it is unit-testable without s3.Init.
func ValidatePublicAssetURL(rawURL, allowedHost string) error {
	if allowedHost == "" {
		return ErrDisallowedAssetURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host != allowedHost {
		return ErrDisallowedAssetURL
	}
	return nil
}

// newAssetHTTPClient builds the HTTP client used for server-side asset fetches.
// It refuses to follow redirects: only the initial URL is allowlist-validated,
// so a 30x to another host would bypass the SSRF guard. Refusing redirects makes
// any 30x surface as a non-200 status, which DownloadPublicAsset then rejects.
func newAssetHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type assetHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func downloadPublicAsset(ctx context.Context, rawURL, allowedHost string, client assetHTTPDoer) ([]byte, string, error) {
	if err := ValidatePublicAssetURL(rawURL, allowedHost); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch asset: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("asset fetch returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read asset: %w", err)
	}
	if len(data) > maxAssetBytes {
		return nil, "", fmt.Errorf("asset exceeds %d byte cap", maxAssetBytes)
	}
	return data, normalizeContentType(resp.Header.Get("Content-Type")), nil
}

// DownloadPublicAsset returns the bytes and Content-Type of one of our own
// public assets, capped at 12 MiB. Same-origin media URLs ("/media/<key>" or
// "${PUBLIC_URL}/media/<key>") are read straight from the public store with no
// network hop; anything else must be an https URL on the configured public
// bucket/CDN host and is fetched with a timeout and without redirects.
func DownloadPublicAsset(ctx context.Context, rawURL string) ([]byte, string, error) {
	if key, ok := ownMediaKey(rawURL); ok {
		if IsProtectedMediaKey(key) {
			return nil, "", ErrDisallowedAssetURL
		}
		data, info, err := ReadPublicObject(ctx, key, maxAssetBytes)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read asset: %w", err)
		}
		return data, normalizeContentType(info.ContentType), nil
	}
	return downloadPublicAsset(ctx, rawURL, PublicHost(), newAssetHTTPClient())
}

func normalizeContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = strings.TrimSpace(contentType[:i])
	}
	return contentType
}
