package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// #829: OpenTable and Resy are reservation platforms, not couriers. The
// shared external-partner catalog must carry a kind so the delivery settings
// path rejects them while reservation booking links keep them.
func TestExternalPartnerCatalog_KindsSplitDeliveryFromReservation(t *testing.T) {
	wantDelivery := []string{"careem", "deliveroo", "doordash", "talabat", "ubereats", "zomato"}
	for _, key := range wantDelivery {
		if !IsValidExternalProviderKeyForKind(key, ExternalPartnerKindDelivery) {
			t.Errorf("expected %q to be a delivery provider", key)
		}
	}
	for _, key := range []string{"opentable", "resy"} {
		if IsValidExternalProviderKeyForKind(key, ExternalPartnerKindDelivery) {
			t.Errorf("reservation platform %q must not be a valid delivery provider", key)
		}
		if !IsValidExternalProviderKeyForKind(key, ExternalPartnerKindReservation) {
			t.Errorf("expected %q to stay valid for reservations", key)
		}
		// The kind split must not orphan the key from the general catalog.
		if !IsValidExternalProviderKey(key) {
			t.Errorf("expected %q to remain a known provider key", key)
		}
	}
}

// #829 B6: venues that saved OpenTable/Resy under delivery partners before the
// kind split must still be able to save delivery settings — the out-of-kind
// legacy rows are silently dropped, never a 400 the operator cannot fix from
// the UI.
func TestNormalizeAndValidateExternalPartnerLinksForKind_DeliveryDropsReservationProviders(t *testing.T) {
	for _, key := range []string{"opentable", "resy"} {
		input := []map[string]interface{}{
			{
				"name":         strings.ToUpper(key[:1]) + key[1:],
				"url":          "https://www." + key + ".com/r/sample",
				"provider_key": key,
			},
		}
		normalized, err := NormalizeAndValidateExternalPartnerLinksForKind(input, ExternalPartnerKindDelivery)
		if err != nil {
			t.Fatalf("legacy %q row must be tolerated on the delivery path, got: %v", key, err)
		}
		var links []ExternalPartnerLink
		if err := json.Unmarshal(normalized, &links); err != nil {
			t.Fatalf("normalized payload must stay a valid array: %v", err)
		}
		if len(links) != 0 {
			t.Fatalf("expected legacy %q row to be dropped, got %d links", key, len(links))
		}
	}
}

// #829 B6: a legacy mixed payload (real courier + stranded reservation row)
// saves successfully and keeps only the couriers.
func TestNormalizeAndValidateExternalPartnerLinksForKind_DeliveryKeepsCouriersDropsLegacyRows(t *testing.T) {
	input := []map[string]interface{}{
		{
			"name":         "Uber Eats",
			"url":          "https://www.ubereats.com/store/123",
			"provider_key": "ubereats",
		},
		{
			"name":         "OpenTable",
			"url":          "https://www.opentable.com/r/sample",
			"provider_key": "opentable",
		},
		{
			"name":     "Local Courier",
			"url":      "https://courier.example.com/order",
			"icon_url": "https://cdn.example.com/icons/courier.png",
		},
	}
	normalized, err := NormalizeAndValidateExternalPartnerLinksForKind(input, ExternalPartnerKindDelivery)
	if err != nil {
		t.Fatalf("mixed legacy payload must save on the delivery path, got: %v", err)
	}
	var links []ExternalPartnerLink
	if err := json.Unmarshal(normalized, &links); err != nil {
		t.Fatalf("normalized payload must stay a valid array: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 surviving links, got %d: %+v", len(links), links)
	}
	if links[0].ProviderKey != "ubereats" {
		t.Errorf("expected first surviving link to be ubereats, got %q", links[0].ProviderKey)
	}
	if links[1].Name != "Local Courier" {
		t.Errorf("expected custom-icon courier to survive, got %q", links[1].Name)
	}
	for _, link := range links {
		if link.ProviderKey == "opentable" {
			t.Errorf("legacy reservation row must not survive the delivery save")
		}
	}
}

func TestNormalizeAndValidateExternalPartnerLinksForKind_DeliveryAcceptsCouriersAndCustomIcons(t *testing.T) {
	input := []map[string]interface{}{
		{
			"name":         "Uber Eats",
			"url":          "https://www.ubereats.com/store/123",
			"provider_key": "ubereats",
		},
		{
			"name":     "Local Courier",
			"url":      "https://courier.example.com/order",
			"icon_url": "https://cdn.example.com/icons/courier.png",
		},
	}
	if _, err := NormalizeAndValidateExternalPartnerLinksForKind(input, ExternalPartnerKindDelivery); err != nil {
		t.Fatalf("expected couriers and custom-icon partners to stay valid, got: %v", err)
	}
}

func TestExternalPartnerCatalog_EveryKeyHasAllowedHost(t *testing.T) {
	if len(externalPartnerAllowedHosts) != len(ExternalPartnerProviderCatalog) {
		t.Fatalf("allowed-host map has %d keys, catalog has %d", len(externalPartnerAllowedHosts), len(ExternalPartnerProviderCatalog))
	}
	for key := range ExternalPartnerProviderCatalog {
		hosts := externalPartnerAllowedHosts[key]
		if len(hosts) == 0 {
			t.Errorf("catalog provider %q has no allowed host", key)
			continue
		}
		for _, host := range hosts {
			if strings.TrimSpace(host) == "" {
				t.Errorf("catalog provider %q has an empty allowed host", key)
			}
		}
	}
}

// A legacy reservation row on the delivery path is dropped even when its host
// would fail the catalog allowlist. The host check applies only to rows the
// kind filter keeps.
func TestNormalizeAndValidateExternalPartnerLinksForKind_DropsOutOfKindBeforeHostCheck(t *testing.T) {
	normalized, err := NormalizeAndValidateExternalPartnerLinksForKind([]map[string]interface{}{
		{
			"name":         "OpenTable",
			"url":          "https://attacker.example/pay",
			"provider_key": "opentable",
		},
	}, ExternalPartnerKindDelivery)
	if err != nil {
		t.Fatalf("out-of-kind legacy row must be dropped, not rejected for host: %v", err)
	}
	var links []ExternalPartnerLink
	if err := json.Unmarshal(normalized, &links); err != nil {
		t.Fatalf("normalized payload must stay a valid array: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected legacy row to be dropped, got %+v", links)
	}
}

// The unrestricted validator (reservation settings path) must keep accepting
// OpenTable/Resy — the split only tightens the delivery route.
func TestNormalizeAndValidateExternalPartnerLinks_StillAcceptsReservationPlatforms(t *testing.T) {
	input := []map[string]interface{}{
		{
			"name":         "OpenTable",
			"url":          "https://www.opentable.com/r/sample",
			"provider_key": "opentable",
		},
	}
	if _, err := NormalizeAndValidateExternalPartnerLinks(input); err != nil {
		t.Fatalf("expected opentable to remain valid on the unrestricted path, got: %v", err)
	}
}
