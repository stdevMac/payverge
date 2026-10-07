// Package pluginurl validates operator-supplied outbound base URLs for payment
// plugins. Operators with plugins:write can set fields like api_base_url; without
// a guard, the backend would dial whatever they provide server-side (an SSRF
// vector reaching internal services or cloud metadata, e.g. 169.254.169.254).
//
// The guard mirrors the existing s3.ValidatePublicAssetURL allowlist pattern but
// matches against a per-provider host suffix list: require https, reject IP
// literals, and require the host to be the provider's domain (or a subdomain of
// it). Because the host must end at a provider domain boundary, an attacker
// cannot point the URL at an internal IP or arbitrary host through a provider
// hostname, which also makes resolve-time RFC1918 checks unnecessary.
package pluginurl

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// ErrDisallowedBaseURL is returned when a base URL is not an https URL whose host
// is (a subdomain of) one of the allowed provider hosts.
var ErrDisallowedBaseURL = errors.New("plugin base url is not allowed")

// Provider host allowlists. The base-url override must point at the provider's
// own API domain; these cover the documented API/sandbox endpoints for each.
var (
	PayPalHosts      = []string{"paypal.com"}
	MercadoPagoHosts = []string{"mercadopago.com"}
)

// ValidateBaseURL returns nil only when rawURL is an https URL whose host equals,
// or is a subdomain of, one of allowedHostSuffixes. Empty/unparseable URLs,
// non-https schemes, IP literals, and off-allowlist hosts are rejected.
func ValidateBaseURL(rawURL string, allowedHostSuffixes []string) error {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return ErrDisallowedBaseURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return ErrDisallowedBaseURL
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return ErrDisallowedBaseURL
	}
	// Reject IP literals outright; the allowlist is domain-based, so a numeric
	// host can never be a legitimate provider endpoint.
	if net.ParseIP(host) != nil {
		return ErrDisallowedBaseURL
	}
	for _, suffix := range allowedHostSuffixes {
		suffix = strings.ToLower(strings.TrimSpace(suffix))
		if suffix == "" {
			continue
		}
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return nil
		}
	}
	return ErrDisallowedBaseURL
}
