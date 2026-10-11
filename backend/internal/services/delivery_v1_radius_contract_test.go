package services

import (
	"encoding/json"
	"strings"
	"testing"
)

// L3-39 cleanup: max_delivery_radius has no operator UI and is no longer
// enforced (customer_address_service stopped distance-gating), so it must not
// linger on the v1 wire contract either. A field that is readable and writable
// but has no effect is worse than no field: operators (or an integrator) can
// set it and reasonably expect a radius to be honoured.
//
// The DB column and the GORM model keep the value — this is a contract-level
// removal, not a schema change.
func TestDeliverySettingsV1_ContractOmitsDeliveryRadius(t *testing.T) {
	raw, err := json.Marshal(DeliverySettingsDTO{})
	if err != nil {
		t.Fatalf("marshal DTO: %v", err)
	}
	if strings.Contains(string(raw), "delivery_radius") {
		t.Fatalf("DeliverySettingsDTO must not expose delivery_radius, got %s", raw)
	}

	var in UpdateDeliverySettingsInput
	if err := json.Unmarshal([]byte(`{"delivery_radius": 42}`), &in); err != nil {
		t.Fatalf("unmarshal update input: %v", err)
	}
	inRaw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal update input: %v", err)
	}
	if strings.Contains(string(inRaw), "delivery_radius") {
		t.Fatalf("UpdateDeliverySettingsInput must not accept delivery_radius, got %s", inRaw)
	}
}
