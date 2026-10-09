package server

import (
	"net/url"
	"strings"
)

// ExternalPartnerKind is the surface a predefined provider belongs to.
type ExternalPartnerKind string

const (
	ExternalPartnerKindDelivery    ExternalPartnerKind = "delivery"
	ExternalPartnerKindReservation ExternalPartnerKind = "reservation"
)

// ExternalPartnerProviderCatalog maps provider keys allowed for predefined
// icons to the surface they belong to. OpenTable and Resy are reservation
// platforms, not couriers (#829): the delivery settings path must not accept
// them, while reservation booking links keep them.
var ExternalPartnerProviderCatalog = map[string]ExternalPartnerKind{
	"careem":    ExternalPartnerKindDelivery,
	"deliveroo": ExternalPartnerKindDelivery,
	"doordash":  ExternalPartnerKindDelivery,
	"opentable": ExternalPartnerKindReservation,
	"resy":      ExternalPartnerKindReservation,
	"talabat":   ExternalPartnerKindDelivery,
	"ubereats":  ExternalPartnerKindDelivery,
	"zomato":    ExternalPartnerKindDelivery,
}

// externalPartnerAllowedHosts is the hostname allowlist for each catalog key.
// A branded storefront CTA ("Open Uber Eats") may only target that provider.
// A host matches when it equals a listed domain or is a subdomain of one.
var externalPartnerAllowedHosts = map[string][]string{
	"ubereats": {"ubereats.com", "uber.com"},
	"doordash": {"doordash.com"},
	"deliveroo": {
		"deliveroo.com", "deliveroo.co.uk", "deliveroo.ae", "deliveroo.fr", "deliveroo.be",
		"deliveroo.ie", "deliveroo.it", "deliveroo.nl", "deliveroo.es", "deliveroo.de",
		"deliveroo.hk", "deliveroo.com.sg", "deliveroo.com.kw", "deliveroo.com.qa", "deliveroo.com.au",
	},
	"talabat": {"talabat.com"},
	"careem":  {"careem.com"},
	"zomato":  {"zomato.com"},
	"opentable": {
		"opentable.com", "opentable.co.uk", "opentable.de", "opentable.ca", "opentable.com.au",
		"opentable.jp", "opentable.com.mx", "opentable.ie", "opentable.nl", "opentable.es", "opentable.it",
	},
	"resy": {"resy.com"},
}

// externalPartnerHostAllowed reports whether rawURL's host is the catalog
// provider or a subdomain of one. The comparison is case-insensitive and
// ignores any port. A catalog key with no listed domains is never allowed.
func externalPartnerHostAllowed(providerKey, rawURL string) bool {
	domains := externalPartnerAllowedHosts[providerKey]
	if len(domains) == 0 {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" {
			continue
		}
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// IsValidExternalProviderKey reports whether the key is in the catalog at all
// (any kind). The reservation settings path uses this unrestricted check.
func IsValidExternalProviderKey(key string) bool {
	_, ok := ExternalPartnerProviderCatalog[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

// IsValidExternalProviderKeyForKind reports whether the key is in the catalog
// AND belongs to the given surface.
func IsValidExternalProviderKeyForKind(key string, kind ExternalPartnerKind) bool {
	got, ok := ExternalPartnerProviderCatalog[strings.ToLower(strings.TrimSpace(key))]
	return ok && got == kind
}
