package database

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOrderMarshalJSONNeverEmitsInternalState is the X-1 regression guard: the
// Business relation carries Stripe IDs, owner identity, and billing
// financials, and Order JSON is returned to UNAUTHENTICATED guests (order
// create response + public guest-orders poll). The persisted settlement
// snapshot is also server-internal. The marshaller must make both leaks
// structurally impossible regardless of how the relation was loaded.
func TestOrderMarshalJSONNeverEmitsInternalState(t *testing.T) {
	order := Order{
		ID:            42,
		BillID:        7,
		BusinessID:    7,
		Status:        OrderStatusPending,
		QuoteSnapshot: JSONRawMessage(`{"version":1,"quote":{"sentinel":"INTERNAL_QUOTE_LEAK"}}`),
		Business: Business{
			ID:              7,
			OwnerAddress:    "0xOWNERLEAK",
			OwnerName:       "Leaky Owner",
			Email:           "owner@leak.test",
			DisplayCurrency: "AED",
			DefaultCurrency: "USD",
		},
	}

	raw, err := json.Marshal(order)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))

	assert.NotContains(t, payload, "business", "Order JSON must never carry the Business relation")
	assert.NotContains(t, string(raw), "stripe_customer_id")
	assert.NotContains(t, string(raw), "owner_address")
	assert.NotContains(t, string(raw), "0xOWNERLEAK")
	assert.NotContains(t, payload, "quote_snapshot", "Order JSON must never carry the internal settlement snapshot")
	assert.NotContains(t, string(raw), "INTERNAL_QUOTE_LEAK")

	// Currency must still resolve from the in-memory Business relation.
	assert.Equal(t, "AED", payload["currency"])

	// FIND-051: unloaded Bill (guest create often has bill_id only) must not
	// invent a zero-money nested bill object.
	assert.NotContains(t, payload, "bill", "unloaded Bill must be omitted from Order JSON")
}

// FIND-051: guest create returns order without Bill preload; nested bill must
// not appear as a zero object. Loaded Bill still marshals with real id.
func TestOrderMarshalJSON_OmitsUnloadedBill(t *testing.T) {
	unloaded := Order{
		ID:         750,
		BillID:     752,
		BusinessID: 50,
		Status:     OrderStatusPending,
	}
	raw, err := json.Marshal(unloaded)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	assert.NotContains(t, payload, "bill")
	assert.Equal(t, float64(752), payload["bill_id"])

	loaded := Order{
		ID:         751,
		BillID:     753,
		BusinessID: 50,
		Status:     OrderStatusPending,
		Bill: Bill{
			ID:          753,
			BusinessID:  50,
			BillNumber:  "B-753",
			PublicToken: "tok_order_bill",
			TotalAmount: 480,
			Status:      BillStatusOpen,
		},
	}
	raw, err = json.Marshal(loaded)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &payload))
	bill, ok := payload["bill"].(map[string]any)
	require.True(t, ok, "loaded Bill must be present")
	assert.Equal(t, float64(753), bill["id"])
	assert.Equal(t, "B-753", bill["bill_number"])
	assert.Equal(t, float64(4.8), bill["total_amount"])
}

// TestGetOrderByRequestIdentityUsesNarrowBusinessProjection guards the
// duplicate-replay path: it must preload only id/default_currency/
// display_currency (the operator-list projection), never the full record.
func TestGetOrderByRequestIdentityUsesNarrowBusinessProjection(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	require.NoError(t, db.Model(&Business{}).Where("id = ?", biz.ID).
		Updates(map[string]interface{}{
			"owner_address":    "0xOWNERLEAK",
			"display_currency": "AED",
		}).Error)
	bill := helperBill(t, biz, nil, 0)

	reqID := "req-narrow-1"
	itemsJSON, err := json.Marshal([]OrderItem{
		{ID: "n-1", MenuItemName: "Soup", Price: 7, Quantity: 1, Subtotal: 7},
	})
	require.NoError(t, err)
	order := &Order{
		BillID:          bill.ID,
		BusinessID:      biz.ID,
		OrderNumber:     "O-narrow",
		Status:          OrderStatusPending,
		CreatedBy:       "guest",
		ClientRequestID: &reqID,
		Items:           string(itemsJSON),
	}
	require.NoError(t, db.Create(order).Error)

	got, _, err := GetOrderByRequestIdentity(bill.ID, "guest", reqID)
	require.NoError(t, err)
	assert.NotZero(t, got.Business.ID, "currency projection must stay preloaded")
	assert.Equal(t, "AED", got.Business.DisplayCurrency)
	assert.Empty(t, got.Business.OwnerAddress, "duplicate-replay path must not hydrate sensitive Business columns")
}
