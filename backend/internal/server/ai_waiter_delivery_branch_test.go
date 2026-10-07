package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestBuildDeliveryContext(t *testing.T) {
	cases := []struct {
		name       string
		settings   *database.DeliverySettings
		wantSubstr string
	}{
		{
			name: "InHouse",
			settings: &database.DeliverySettings{
				DeliveryEnabled:        true,
				InHouseDeliveryEnabled: true,
				FlatDeliveryFee:        500,  // $5.00 cents
				FreeDeliveryMinimum:    3000, // $30.00 cents
			},
			wantSubstr: "Order Delivery",
		},
		{
			name: "PartnerOnly",
			settings: &database.DeliverySettings{
				DeliveryEnabled:      true,
				ThirdPartyEnabled:    true,
				ExternalPartnerLinks: database.JSONRawMessage(`[{"name":"Talabat","url":"https://x"}]`),
			},
			wantSubstr: "delivery partners",
		},
		{
			name: "Off",
			settings: &database.DeliverySettings{
				DeliveryEnabled: false,
			},
			wantSubstr: "",
		},
		{
			name:       "Nil",
			settings:   nil,
			wantSubstr: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildDeliveryContext(tc.settings)
			if tc.wantSubstr == "" {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tc.wantSubstr)
			}
		})
	}
}
