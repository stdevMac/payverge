package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeAndValidateExternalPartnerLinks_AcceptsValidProviderKey(t *testing.T) {
	input := []map[string]interface{}{
		{
			"name":         "Talabat",
			"url":          "https://www.talabat.com/store/123",
			"provider_key": "talabat",
		},
	}

	normalized, err := NormalizeAndValidateExternalPartnerLinks(input)
	if err != nil {
		t.Fatalf("expected valid payload, got error: %v", err)
	}

	var links []ExternalPartnerLink
	if err := json.Unmarshal(normalized, &links); err != nil {
		t.Fatalf("failed to unmarshal normalized links: %v", err)
	}

	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].ProviderKey != "talabat" {
		t.Fatalf("expected provider_key talabat, got %q", links[0].ProviderKey)
	}
}

func TestNormalizeAndValidateExternalPartnerLinks_AcceptsValidIconURL(t *testing.T) {
	input := []map[string]interface{}{
		{
			"name":     "Custom Partner",
			"url":      "https://partners.example.com/order",
			"icon_url": "https://cdn.example.com/icons/partner.png",
		},
	}

	normalized, err := NormalizeAndValidateExternalPartnerLinks(input)
	if err != nil {
		t.Fatalf("expected valid payload, got error: %v", err)
	}

	var links []ExternalPartnerLink
	if err := json.Unmarshal(normalized, &links); err != nil {
		t.Fatalf("failed to unmarshal normalized links: %v", err)
	}

	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].IconURL != "https://cdn.example.com/icons/partner.png" {
		t.Fatalf("expected icon_url to be preserved, got %q", links[0].IconURL)
	}
}

func TestNormalizeAndValidateExternalPartnerLinks_RejectsInvalidURL(t *testing.T) {
	input := []map[string]interface{}{
		{
			"name":         "Invalid URL Partner",
			"url":          "talabat.com/store/123",
			"provider_key": "talabat",
		},
	}

	_, err := NormalizeAndValidateExternalPartnerLinks(input)
	if err == nil {
		t.Fatal("expected error for invalid url")
	}
	if !strings.Contains(err.Error(), ".url must use http or https") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeAndValidateExternalPartnerLinks_RejectsInvalidIconURL(t *testing.T) {
	input := []map[string]interface{}{
		{
			"name":         "Invalid Icon URL Partner",
			"url":          "https://www.talabat.com/store/123",
			"provider_key": "talabat",
			"icon_url":     "ftp://cdn.example.com/icon.png",
		},
	}

	_, err := NormalizeAndValidateExternalPartnerLinks(input)
	if err == nil {
		t.Fatal("expected error for invalid icon_url")
	}
	if !strings.Contains(err.Error(), ".icon_url must use http or https") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeAndValidateExternalPartnerLinks_RejectsUnknownProviderKey(t *testing.T) {
	input := []map[string]interface{}{
		{
			"name":         "Unknown Provider",
			"url":          "https://example.com/order",
			"provider_key": "unknown_provider",
		},
	}

	_, err := NormalizeAndValidateExternalPartnerLinks(input)
	if err == nil {
		t.Fatal("expected error for unknown provider key")
	}
	if !strings.Contains(err.Error(), ".provider_key is not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeAndValidateExternalPartnerLinks_RejectsOffCatalogHost(t *testing.T) {
	_, err := NormalizeAndValidateExternalPartnerLinks([]map[string]interface{}{
		{
			"name":         "Uber Eats",
			"url":          "https://attacker.example/pay",
			"provider_key": "ubereats",
		},
	})
	if err == nil {
		t.Fatal("expected error for off-catalog host")
	}
	if !strings.Contains(err.Error(), "external_partner_links[0].url must point to ubereats (host not allowed)") {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := NormalizeAndValidateExternalPartnerLinks([]map[string]interface{}{
		{
			"name":         "Uber Eats",
			"url":          "https://www.ubereats.com/store/x",
			"provider_key": "ubereats",
		},
	}); err != nil {
		t.Fatalf("expected provider host to pass, got %v", err)
	}

	_, err = NormalizeAndValidateExternalPartnerLinks([]map[string]interface{}{
		{
			"name":         "Uber Eats",
			"url":          "https://ubereats.com.attacker.example/",
			"provider_key": "ubereats",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "host not allowed") {
		t.Fatalf("expected subdomain spoof to be rejected, got %v", err)
	}

	if _, err := NormalizeAndValidateExternalPartnerLinks([]map[string]interface{}{
		{
			"name":     "Custom Partner",
			"url":      "https://attacker.example/pay",
			"icon_url": "https://cdn.example.com/icons/partner.png",
		},
	}); err != nil {
		t.Fatalf("custom icon link must keep an arbitrary host, got %v", err)
	}
}
