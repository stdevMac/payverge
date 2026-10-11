package services

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func validZones() database.JSONRawMessage {
	zones := []map[string]interface{}{
		{
			"name":                 "Downtown",
			"boundaries":           map[string]interface{}{"postal_codes": []string{"10001"}, "cities": []string{}},
			"priority":             1,
			"estimated_time":       30,
			"delivery_fee":         4.50,
			"minimum_order_amount": 15.00,
			"is_active":            true,
		},
	}
	raw, _ := json.Marshal(zones)
	return raw
}

func validPartners() database.JSONRawMessage {
	links := []map[string]string{
		{"name": "Talabat", "url": "https://talabat.com/x", "provider_key": "talabat"},
	}
	raw, _ := json.Marshal(links)
	return raw
}

func TestValidateDeliverySettings(t *testing.T) {
	svc := &DeliveryService{}
	cases := []struct {
		name    string
		mutate  func(s *database.DeliverySettings)
		wantErr string // substring of expected error; "" means accept
	}{
		{
			name: "Valid/InHouseOnly",
			mutate: func(s *database.DeliverySettings) {
				s.DeliveryEnabled, s.InHouseDeliveryEnabled = true, true
				s.DeliveryZones = validZones()
			},
			wantErr: "",
		},
		{
			name: "Valid/PartnerOnly",
			mutate: func(s *database.DeliverySettings) {
				s.DeliveryEnabled, s.ThirdPartyEnabled = true, true
				s.ExternalPartnerLinks = validPartners()
			},
			wantErr: "",
		},
		{
			name: "Valid/InHouseAndPartner",
			mutate: func(s *database.DeliverySettings) {
				s.DeliveryEnabled = true
				s.InHouseDeliveryEnabled, s.ThirdPartyEnabled = true, true
				s.DeliveryZones = validZones()
				s.ExternalPartnerLinks = validPartners()
			},
			wantErr: "",
		},
		{
			// Setup-in-progress: merchant flipped the master toggle on but
			// hasn't configured a fulfillment route yet. Public-page gate
			// renders nothing until partners or zones land. Validator accepts.
			name: "Valid/SetupInProgress",
			mutate: func(s *database.DeliverySettings) {
				s.DeliveryEnabled = true
			},
			wantErr: "",
		},
		{
			name: "Normalize/DeliveryOff",
			mutate: func(s *database.DeliverySettings) {
				s.DeliveryEnabled = false
				s.InHouseDeliveryEnabled, s.ThirdPartyEnabled = true, true
				s.DeliveryZones = validZones()
				s.ExternalPartnerLinks = validPartners()
			},
			wantErr: "",
		},
		{
			name: "Reject/PartnerEnabledNoLinks",
			mutate: func(s *database.DeliverySettings) {
				s.DeliveryEnabled, s.ThirdPartyEnabled = true, true
				s.ExternalPartnerLinks = database.JSONRawMessage(`[]`)
			},
			wantErr: "third_party_enabled requires",
		},
		{
			name: "Reject/InHouseEnabledNoZones",
			mutate: func(s *database.DeliverySettings) {
				s.DeliveryEnabled, s.InHouseDeliveryEnabled = true, true
				s.DeliveryZones = database.JSONRawMessage(`[]`)
			},
			wantErr: "in_house_delivery_enabled requires",
		},
		{
			name: "Reject/ZoneMissingBoundaries",
			mutate: func(s *database.DeliverySettings) {
				s.DeliveryEnabled, s.InHouseDeliveryEnabled = true, true
				zones := []map[string]interface{}{
					{"name": "Bad", "boundaries": map[string]interface{}{"postal_codes": []string{}, "cities": []string{}}, "priority": 1, "estimated_time": 30, "is_active": true},
				}
				raw, _ := json.Marshal(zones)
				s.DeliveryZones = raw
			},
			wantErr: "zone requires at least one postal code or city",
		},
		{
			name: "Reject/ZoneDuplicatePriority",
			mutate: func(s *database.DeliverySettings) {
				s.DeliveryEnabled, s.InHouseDeliveryEnabled = true, true
				zones := []map[string]interface{}{
					{"name": "A", "boundaries": map[string]interface{}{"postal_codes": []string{"1"}}, "priority": 1, "estimated_time": 30, "is_active": true},
					{"name": "B", "boundaries": map[string]interface{}{"postal_codes": []string{"2"}}, "priority": 1, "estimated_time": 30, "is_active": true},
				}
				raw, _ := json.Marshal(zones)
				s.DeliveryZones = raw
			},
			wantErr: "duplicate active zone priority",
		},
		{
			// Regression: an inactive zone must NOT consume a priority slot.
			// Pre-fix the validator wrote priorities[z.Priority]=true unconditionally,
			// which caused an active zone sharing priority with an inactive one to
			// be misreported as a duplicate.
			name: "Valid/InactiveZoneSharingPriorityAllowed",
			mutate: func(s *database.DeliverySettings) {
				s.DeliveryEnabled, s.InHouseDeliveryEnabled = true, true
				zones := []map[string]interface{}{
					{"name": "Old", "boundaries": map[string]interface{}{"postal_codes": []string{"00000"}}, "priority": 1, "estimated_time": 30, "is_active": false},
					{"name": "New", "boundaries": map[string]interface{}{"postal_codes": []string{"10001"}}, "priority": 1, "estimated_time": 30, "is_active": true},
				}
				raw, _ := json.Marshal(zones)
				s.DeliveryZones = raw
			},
			wantErr: "",
		},
		{
			name: "Reject/NegativeFee",
			mutate: func(s *database.DeliverySettings) {
				s.DeliveryEnabled, s.InHouseDeliveryEnabled = true, true
				s.DeliveryZones = validZones()
				s.FlatDeliveryFee = -100
			},
			wantErr: "cannot be negative",
		},
		{
			name: "Reject/InvalidPaymentMode",
			mutate: func(s *database.DeliverySettings) {
				s.DeliveryEnabled, s.InHouseDeliveryEnabled = true, true
				s.DeliveryZones = validZones()
				s.PaymentMode = "bogus"
			},
			wantErr: "invalid payment_mode",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Matches the real default (column is `gorm:"default:true"`); these
			// cases cover zone/partner/payment rules, not custom delivery hours.
			settings := &database.DeliverySettings{
				DeliveryHoursSameAsBusiness: true,
				DeliveryZones:               database.JSONRawMessage(`[]`),
				ExternalPartnerLinks:        database.JSONRawMessage(`[]`),
			}
			tc.mutate(settings)
			err := svc.validateDeliverySettings(settings)
			if tc.wantErr == "" {
				assert.NoError(t, err)
				if tc.name == "Normalize/DeliveryOff" {
					assert.False(t, settings.InHouseDeliveryEnabled, "in-house should be normalized to false")
					assert.False(t, settings.ThirdPartyEnabled, "third-party should be normalized to false")
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}
