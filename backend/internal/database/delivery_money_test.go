package database

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeliveryOrder_MarshalJSON_EmitsDollars(t *testing.T) {
	order := DeliveryOrder{
		DeliveryFee: 599, // $5.99
		DriverTip:   250, // $2.50
		PlatformFee: 75,  // $0.75
	}

	raw, err := json.Marshal(order)
	assert.NoError(t, err)

	var decoded map[string]interface{}
	assert.NoError(t, json.Unmarshal(raw, &decoded))

	assert.InDelta(t, 5.99, decoded["delivery_fee"], 0.001)
	assert.InDelta(t, 2.50, decoded["driver_tip"], 0.001)
	assert.InDelta(t, 0.75, decoded["platform_fee"], 0.001)
}

func TestDeliveryOrder_MarshalJSON_ZeroValues(t *testing.T) {
	order := DeliveryOrder{}
	raw, err := json.Marshal(order)
	assert.NoError(t, err)

	var decoded map[string]interface{}
	assert.NoError(t, json.Unmarshal(raw, &decoded))

	assert.Equal(t, 0.0, decoded["delivery_fee"])
	assert.Equal(t, 0.0, decoded["driver_tip"])
	assert.Equal(t, 0.0, decoded["platform_fee"])
}

func TestDeliveryOrder_MarshalJSON_EmitsHydratedTotalAndCurrency(t *testing.T) {
	cents := int64(18650)
	order := DeliveryOrder{
		ID:                 625,
		DeliveryFee:        2900,
		DispatchTotalCents: &cents,
		DispatchCurrency:   "ARS",
	}
	raw, err := json.Marshal(order)
	assert.NoError(t, err)
	var decoded map[string]interface{}
	assert.NoError(t, json.Unmarshal(raw, &decoded))
	assert.InDelta(t, 186.50, decoded["total"], 0.001)
	assert.Equal(t, "ARS", decoded["currency"])
	assert.InDelta(t, 29.00, decoded["delivery_fee"], 0.001)
}

// The hydrated dispatch total must run through the same centsToDollars site as
// every other money field, so an odd-cent total cannot pick up float drift from
// a hand-rolled /100 somewhere in the service layer (#896).
func TestDeliveryOrder_MarshalJSON_HydratedTotalUsesCanonicalConverter(t *testing.T) {
	for _, cents := range []int64{1, 7, 99, 12345, 999999999} {
		c := cents
		order := DeliveryOrder{ID: 626, DispatchTotalCents: &c}
		raw, err := json.Marshal(order)
		assert.NoError(t, err)
		var decoded map[string]interface{}
		assert.NoError(t, json.Unmarshal(raw, &decoded))
		assert.Equal(t, centsToDollars(cents), decoded["total"],
			"hydrated total for %d cents must match centsToDollars exactly", cents)
	}
}

// A delivery whose bill row could not be joined must not claim the order is
// worth $0.00 — total stays null and the UI keeps its own empty state (#896).
func TestDeliveryOrder_MarshalJSON_NoHydratedTotalStaysNull(t *testing.T) {
	order := DeliveryOrder{ID: 627, BusinessID: 5, DispatchCurrency: "USD"}
	raw, err := json.Marshal(order)
	assert.NoError(t, err)
	var decoded map[string]interface{}
	assert.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Nil(t, decoded["total"], "missing bill must leave total null, not 0")
}

// 896-B: the dispatch projection must resolve currency exactly like the rest of
// the app — display_currency wins, default_currency is the fallback, and USD is
// the floor for the (very common) rows where neither column was ever set.
func TestResolveDispatchCurrency_PrefersDisplayThenDefaultThenUSD(t *testing.T) {
	assert.Equal(t, "ARS", ResolveDispatchCurrency("ARS", "USD"),
		"display currency must win over default")
	assert.Equal(t, "EUR", ResolveDispatchCurrency("", "EUR"),
		"default currency is the fallback when no display currency is set")
	assert.Equal(t, "USD", ResolveDispatchCurrency("", ""),
		"default_currency has no DB default; USD is the floor, never a blank chip")
	assert.Equal(t, "USD", ResolveDispatchCurrency("   ", "  "),
		"whitespace-only columns must not pass for a currency")
	assert.Equal(t,
		resolveBusinessCurrency("", Business{DisplayCurrency: "BRL", DefaultCurrency: "USD"}),
		ResolveDispatchCurrency("BRL", "USD"),
		"dispatch resolution must not drift from the shared business resolver")
}

func TestDeliveryOrder_MarshalJSON_OmitsUnloadedBill(t *testing.T) {
	// FIND-044: non-pointer Bill association must not emit empty bill{} on wire.
	order := DeliveryOrder{ID: 147, BusinessID: 50, DeliveryNumber: "D-1"}
	raw, err := json.Marshal(order)
	assert.NoError(t, err)
	var decoded map[string]interface{}
	assert.NoError(t, json.Unmarshal(raw, &decoded))
	_, hasBill := decoded["bill"]
	assert.False(t, hasBill, "unloaded bill must be omitted, got %v", decoded["bill"])
}

func TestDeliveryDriver_MarshalJSON_PartialProjectionIsSlim(t *testing.T) {
	// Mirrors preloadDeliveryDispatchDriverSummary.
	raw, err := json.Marshal(DeliveryDriver{
		ID: 9, BusinessID: 50, Name: "Demo Driver 1", Phone: "+12125550300",
		Email: "demo@payverge.local",
	})
	assert.NoError(t, err)
	var m map[string]interface{}
	assert.NoError(t, json.Unmarshal(raw, &m))
	assert.Equal(t, "Demo Driver 1", m["name"])
	assert.Equal(t, "+12125550300", m["phone"])
	assert.Equal(t, "demo@payverge.local", m["email"])
	for _, banned := range []string{
		"is_active", "total_deliveries", "total_earnings", "license_number",
		"vehicle_plate", "status", "is_available", "created_at", "updated_at",
	} {
		if _, ok := m[banned]; ok {
			t.Errorf("partial driver must not emit %q", banned)
		}
	}
}

func TestDeliveryZone_MarshalJSON_EmitsDollars(t *testing.T) {
	zone := DeliveryZone{
		DeliveryFee:        450,  // $4.50
		MinimumOrderAmount: 1500, // $15.00
	}

	raw, err := json.Marshal(zone)
	assert.NoError(t, err)

	var decoded map[string]interface{}
	assert.NoError(t, json.Unmarshal(raw, &decoded))

	assert.InDelta(t, 4.50, decoded["delivery_fee"], 0.001)
	assert.InDelta(t, 15.00, decoded["minimum_order_amount"], 0.001)
}

func TestDeliverySettings_MarshalJSON_EmitsDollars(t *testing.T) {
	settings := DeliverySettings{
		FlatDeliveryFee:     500,  // $5.00
		FreeDeliveryMinimum: 3000, // $30.00
		MinimumOrderAmount:  1500, // $15.00
	}

	raw, err := json.Marshal(settings)
	assert.NoError(t, err)

	var decoded map[string]interface{}
	assert.NoError(t, json.Unmarshal(raw, &decoded))

	assert.InDelta(t, 5.00, decoded["flat_delivery_fee"], 0.001)
	assert.InDelta(t, 30.00, decoded["free_delivery_minimum"], 0.001)
	assert.InDelta(t, 15.00, decoded["minimum_order_amount"], 0.001)
}

func TestDeliverySettings_MarshalJSON_ZeroValues(t *testing.T) {
	settings := DeliverySettings{}
	raw, err := json.Marshal(settings)
	assert.NoError(t, err)

	var decoded map[string]interface{}
	assert.NoError(t, json.Unmarshal(raw, &decoded))

	assert.Equal(t, 0.0, decoded["flat_delivery_fee"])
	assert.Equal(t, 0.0, decoded["free_delivery_minimum"])
	assert.Equal(t, 0.0, decoded["minimum_order_amount"])
}
