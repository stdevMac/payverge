package services

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestResolveOrderabilityDecisionTable(t *testing.T) {
	tests := []struct {
		name  string
		facts OrderabilityFacts
		want  Orderability
	}{
		{name: "available", facts: OrderabilityFacts{ManualAvailable: true, BusinessOpen: true, GuestOrderingEnabled: true}, want: Orderability{Orderable: true, State: OrderabilityAvailable}},
		{name: "manual disabled", facts: OrderabilityFacts{ManualAvailable: false, BusinessOpen: true, GuestOrderingEnabled: true}, want: Orderability{State: OrderabilityManualDisabled}},
		{name: "inventory hard block", facts: OrderabilityFacts{ManualAvailable: true, InventoryEnabled: true, InventoryMode: "hard_block", InventoryOut: true, BusinessOpen: true, GuestOrderingEnabled: true}, want: Orderability{State: OrderabilityInventoryOut}},
		{name: "inventory warn empty blocks sale like hard block", facts: OrderabilityFacts{ManualAvailable: true, InventoryEnabled: true, InventoryMode: "warn", InventoryOut: true, BusinessOpen: true, GuestOrderingEnabled: true}, want: Orderability{State: OrderabilityInventoryOut}},
		{name: "low inventory warns", facts: OrderabilityFacts{ManualAvailable: true, InventoryEnabled: true, InventoryMode: "warn", InventoryWarning: true, BusinessOpen: true, GuestOrderingEnabled: true}, want: Orderability{Orderable: true, State: OrderabilityInventoryWarn}},
		{name: "business closed for guest", facts: OrderabilityFacts{Context: OrderabilityContextGuest, ManualAvailable: true, BusinessOpen: false, GuestOrderingEnabled: true}, want: Orderability{State: OrderabilityBusinessClosed}},
		{name: "guest closed wins over inventory out", facts: OrderabilityFacts{Context: OrderabilityContextGuest, ManualAvailable: true, InventoryEnabled: true, InventoryMode: "warn", InventoryOut: true, BusinessOpen: false, GuestOrderingEnabled: true}, want: Orderability{State: OrderabilityBusinessClosed}},
		{name: "guest closed wins over inventory low-stock warn", facts: OrderabilityFacts{Context: OrderabilityContextGuest, ManualAvailable: true, InventoryEnabled: true, InventoryMode: "warn", InventoryWarning: true, BusinessOpen: false, GuestOrderingEnabled: true}, want: Orderability{State: OrderabilityBusinessClosed}},
		{name: "guest closed wins over inventory hard block", facts: OrderabilityFacts{Context: OrderabilityContextGuest, ManualAvailable: true, InventoryEnabled: true, InventoryMode: "hard_block", InventoryOut: true, BusinessOpen: false, GuestOrderingEnabled: true}, want: Orderability{State: OrderabilityBusinessClosed}},
		{name: "ordering disabled for guest", facts: OrderabilityFacts{Context: OrderabilityContextGuest, ManualAvailable: true, BusinessOpen: true, GuestOrderingEnabled: false}, want: Orderability{State: OrderabilityOrderingOff}},
		{name: "operator ignores guest gates", facts: OrderabilityFacts{Context: OrderabilityContextOperator, ManualAvailable: true, BusinessOpen: false, GuestOrderingEnabled: false}, want: Orderability{Orderable: true, State: OrderabilityAvailable}},
		{name: "operator still sees inventory out when closed", facts: OrderabilityFacts{Context: OrderabilityContextOperator, ManualAvailable: true, InventoryEnabled: true, InventoryMode: "warn", InventoryOut: true, BusinessOpen: false, GuestOrderingEnabled: false}, want: Orderability{State: OrderabilityInventoryOut}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveOrderability(tt.facts))
		})
	}
}

func TestProjectOrderability_LapsedSubscriptionIsOrderingOff(t *testing.T) {
	categories := []database.MenuCategory{{
		ID: "c1",
		Items: []database.MenuItem{
			{ID: "burger", Name: "Burger", IsAvailable: true},
		},
	}}
	business := &database.Business{
		KitchenEnabled: true,
		OrdersEnabled:  true,
	}

	projection := ProjectOrderability(business, categories, OrderabilityContextGuest, true)
	got := projection["burger"]
	assert.False(t, got.Orderable)
	assert.Equal(t, OrderabilityOrderingOff, got.State)
}
