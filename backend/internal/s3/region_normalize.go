package s3

import (
	"log"
	"net/url"
	"strings"
)

// r2EndpointHostSuffix identifies a Cloudflare R2 write endpoint. R2 URLs look
// like https://<account-id>.r2.cloudflarestorage.com — the account-id prefix
// varies, so we match on the host suffix.
const r2EndpointHostSuffix = ".r2.cloudflarestorage.com"

// r2AcceptedRegions are the only region values Cloudflare R2 accepts on the
// wire. Anything else (e.g. an AWS region like "us-west-2") triggers an
// InvalidRegionName 400.
var r2AcceptedRegions = map[string]struct{}{
	"auto": {},
	"wnam": {},
	"enam": {},
	"weur": {},
	"eeur": {},
	"apac": {},
	"oc":   {},
}

// normalizeRegionForEndpoint guards against R2 rejecting an AWS region name.
// When the configured write endpoint is a Cloudflare R2 host and the region is
// not one of R2's accepted values, it returns "auto" (and logs the coercion);
// otherwise it returns the region unchanged. Non-R2 endpoints (real AWS with an
// empty endpoint, MinIO, iDrive e2, etc.) always pass the region through.
func normalizeRegionForEndpoint(region, endpointURL string) string {
	if !isR2Endpoint(endpointURL) {
		return region
	}
	if _, ok := r2AcceptedRegions[strings.ToLower(strings.TrimSpace(region))]; ok {
		return region
	}
	log.Printf("s3: endpoint %q is Cloudflare R2 but region %q is not an accepted R2 region; normalizing region to \"auto\"", endpointURL, region)
	return "auto"
}

// NormalizeRegionForEndpoint exposes the production storage normalization to
// operational commands so their signed requests exercise the same region as
// the application clients.
func NormalizeRegionForEndpoint(region, endpointURL string) string {
	return normalizeRegionForEndpoint(region, endpointURL)
}

// isR2Endpoint reports whether endpointURL points at a Cloudflare R2 host. It
// tolerates a missing scheme (some callers pass a bare host).
func isR2Endpoint(endpointURL string) bool {
	trimmed := strings.TrimSpace(endpointURL)
	if trimmed == "" {
		return false
	}
	host := trimmed
	if u, err := url.Parse(trimmed); err == nil && u.Host != "" {
		host = u.Host
	} else {
		// No scheme (e.g. "acct.r2.cloudflarestorage.com" or with a path):
		// take the authority up to the first slash.
		if i := strings.IndexByte(host, '/'); i >= 0 {
			host = host[:i]
		}
	}
	host = strings.ToLower(host)
	// Drop any :port suffix before matching.
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	return strings.HasSuffix(host, r2EndpointHostSuffix)
}
