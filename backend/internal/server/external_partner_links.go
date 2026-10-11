package server

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/s3"
)

const MaxExternalPartnerLinks = 5

type ExternalPartnerLink struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	ProviderKey string `json:"provider_key,omitempty"`
	IconURL     string `json:"icon_url,omitempty"`
}

// NormalizeAndValidateExternalPartnerLinks validates links against the full
// provider catalog (any kind). Used by the reservation settings path.
func NormalizeAndValidateExternalPartnerLinks(raw interface{}) (json.RawMessage, error) {
	return normalizeExternalPartnerLinks(raw, nil)
}

// NormalizeAndValidateExternalPartnerLinksForKind additionally requires every
// predefined provider_key to belong to the given surface, so reservation
// platforms (opentable, resy) cannot be saved as delivery partners (#829).
// Known-but-out-of-kind rows are dropped from the normalized result rather
// than failing the save, so venues with pre-split legacy rows can still save
// their settings (#829 B6).
func NormalizeAndValidateExternalPartnerLinksForKind(raw interface{}, kind ExternalPartnerKind) (json.RawMessage, error) {
	return normalizeExternalPartnerLinks(raw, &kind)
}

func normalizeExternalPartnerLinks(raw interface{}, kind *ExternalPartnerKind) (json.RawMessage, error) {
	if raw == nil {
		return json.RawMessage("[]"), nil
	}

	var data []byte
	switch v := raw.(type) {
	case json.RawMessage:
		data = v
	case []byte:
		data = v
	default:
		marshaled, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("invalid external_partner_links payload")
		}
		data = marshaled
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage("[]"), nil
	}

	var links []ExternalPartnerLink
	if err := json.Unmarshal(data, &links); err != nil {
		return nil, fmt.Errorf("external_partner_links must be a valid array")
	}

	if len(links) > MaxExternalPartnerLinks {
		return nil, fmt.Errorf("external_partner_links supports up to %d links", MaxExternalPartnerLinks)
	}

	kept := make([]ExternalPartnerLink, 0, len(links))
	for i := range links {
		links[i].Name = strings.TrimSpace(links[i].Name)
		links[i].URL = strings.TrimSpace(links[i].URL)
		links[i].ProviderKey = strings.ToLower(strings.TrimSpace(links[i].ProviderKey))
		links[i].IconURL = strings.TrimSpace(links[i].IconURL)

		if links[i].Name == "" {
			return nil, fmt.Errorf("external_partner_links[%d].name is required", i)
		}
		if links[i].URL == "" {
			return nil, fmt.Errorf("external_partner_links[%d].url is required", i)
		}
		if !isValidHTTPURL(links[i].URL) {
			return nil, fmt.Errorf("external_partner_links[%d].url must use http or https", i)
		}

		if links[i].ProviderKey != "" {
			if !IsValidExternalProviderKey(links[i].ProviderKey) {
				return nil, fmt.Errorf("external_partner_links[%d].provider_key is not supported", i)
			}
			if kind != nil && !IsValidExternalProviderKeyForKind(links[i].ProviderKey, *kind) {
				// #829 B6: pre-split saves stranded reservation platforms
				// (opentable, resy) inside delivery partner rows. Rejecting
				// them would 400 every delivery-settings save with no way to
				// repair from the UI, so a legacy out-of-kind row is
				// tolerated and dropped instead of failing the save.
				continue
			}
			// Catalog brands render as the provider's own CTA. The URL host
			// has to be that provider; a custom icon link (no provider_key)
			// is not branded and is not checked here.
			if !externalPartnerHostAllowed(links[i].ProviderKey, links[i].URL) {
				return nil, fmt.Errorf("external_partner_links[%d].url must point to %s (host not allowed)", i, links[i].ProviderKey)
			}
		}

		// Uploaded icons are same-origin "/media/<key>" URLs on the local
		// storage driver (no PUBLIC_URL needed).
		if links[i].IconURL != "" && !isValidHTTPURL(links[i].IconURL) && !s3.IsOwnMediaURL(links[i].IconURL) {
			return nil, fmt.Errorf("external_partner_links[%d].icon_url must use http or https", i)
		}

		if links[i].ProviderKey == "" && links[i].IconURL == "" {
			return nil, fmt.Errorf("external_partner_links[%d] requires provider_key or icon_url", i)
		}

		kept = append(kept, links[i])
	}

	normalized, err := json.Marshal(kept)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize external_partner_links")
	}

	return json.RawMessage(normalized), nil
}

// ExternalPartnerLinksChanged reports whether next differs from the stored
// partner-link JSON. Both sides are normalized with kind (nil accepts every
// catalog kind, matching the reservation settings path). Null, empty, and
// whitespace-only payloads compare as an empty slice.
func ExternalPartnerLinksChanged(stored, next []byte, kind *ExternalPartnerKind) (bool, error) {
	storedNorm, err := normalizeExternalPartnerLinks(stored, kind)
	if err != nil {
		return false, err
	}
	nextNorm, err := normalizeExternalPartnerLinks(next, kind)
	if err != nil {
		return false, err
	}
	storedLinks, err := decodeExternalPartnerLinks(storedNorm)
	if err != nil {
		return false, err
	}
	nextLinks, err := decodeExternalPartnerLinks(nextNorm)
	if err != nil {
		return false, err
	}
	return !reflect.DeepEqual(storedLinks, nextLinks), nil
}

func decodeExternalPartnerLinks(raw json.RawMessage) ([]ExternalPartnerLink, error) {
	links := []ExternalPartnerLink{}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return links, nil
	}
	if err := json.Unmarshal(raw, &links); err != nil {
		return nil, err
	}
	if links == nil {
		return []ExternalPartnerLink{}, nil
	}
	return links, nil
}

func isValidHTTPURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return false
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	return parsed.Host != ""
}
