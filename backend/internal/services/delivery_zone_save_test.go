package services

import (
	"encoding/json"
	"testing"
)

// newDeliveryTestService creates a DeliveryService backed by an in-memory
// SQLite DB with all delivery tables migrated, and a business with no
// pre-existing delivery settings.  Mirror the pattern from
// delivery_quote_honesty_test.go.
func newDeliveryTestService(t *testing.T) (*DeliveryService, uint) {
	t.Helper()
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, t.Name())
	return svc, business.ID
}

// TestZoneSave_FirstSaveWithNewZoneSucceeds — R1:
// A first-ever save that enables in_house delivery together with a new
// active zone must succeed.  Before the fix, validateDeliverySettings ran
// BEFORE syncDeliveryZones, so the validator saw an empty zone snapshot and
// rejected the request with "requires at least one active zone".
func TestZoneSave_FirstSaveWithNewZoneSucceeds(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	enabled := true
	zones := []UpdateDeliveryZoneInput{{
		Name:          "Centro",
		DeliveryFee:   3,
		EstimatedTime: 25,
		Priority:      1,
		IsActive:      true,
		Boundaries:    json.RawMessage(`{"postal_codes":["1407*"]}`),
	}}
	dto, err := svc.UpdateDeliverySettingsDTO(businessID, UpdateDeliverySettingsInput{
		DeliveryEnabled:        &enabled,
		InHouseDeliveryEnabled: &enabled,
		Zones:                  &zones,
	})
	if err != nil {
		t.Fatalf("first zone save failed: %v", err)
	}
	if len(dto.Zones) != 1 || dto.Zones[0].Name != "Centro" {
		t.Fatalf("zone not persisted: %+v", dto.Zones)
	}
}

// TestZoneSave_NegativePlaceholderIDCreates — R2:
// The frontend zone editor uses negative placeholder ids for new zones.
// The backend must treat id <= 0 as "create", not return a 400 on unmarshal.
func TestZoneSave_NegativePlaceholderIDCreates(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	var input UpdateDeliverySettingsInput
	payload := []byte(`{
		"delivery_enabled":true,
		"in_house_delivery_enabled":true,
		"zones":[{
			"id":-1,
			"name":"Norte",
			"delivery_fee":4,
			"estimated_time":30,
			"priority":1,
			"is_active":true,
			"boundaries":{"postal_codes":["1426*"]}
		}]
	}`)
	if err := json.Unmarshal(payload, &input); err != nil {
		t.Fatalf("negative id must unmarshal without error: %v", err)
	}
	dto, err := svc.UpdateDeliverySettingsDTO(businessID, input)
	if err != nil {
		t.Fatalf("UpdateDeliverySettingsDTO with placeholder id failed: %v", err)
	}
	if len(dto.Zones) != 1 || !(dto.Zones[0].ID > 0) {
		t.Fatalf("placeholder zone not created: %+v", dto.Zones)
	}
}
