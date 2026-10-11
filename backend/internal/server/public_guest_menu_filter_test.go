package server

import (
	"encoding/json"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestBuildPublicGuestMenuResponse_KeepsUnavailableItemsWithOrderability(t *testing.T) {
	cats := []database.MenuCategory{{
		ID: "c1", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "i1", Name: "Available", Price: 10, IsAvailable: true},
			{ID: "i2", Name: "86d", Price: 12, IsAvailable: false},
		},
	}, {
		ID: "c2", Name: "AllGone",
		Items: []database.MenuItem{
			{ID: "i3", Name: "AlsoGone", Price: 9, IsAvailable: false},
		},
	}}
	raw, _ := json.Marshal(cats)
	menu := &database.Menu{Categories: string(raw)}

	business := &database.Business{IsActive: true, KitchenEnabled: true, OrdersEnabled: true}
	resp := buildPublicGuestMenuResponse(menu, business)
	out, _ := json.Marshal(resp["categories"])

	var got []database.MenuCategory
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("categories must be valid JSON array: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("unavailable items must remain visible: got %d categories", len(got))
	}
	if len(got[0].Items) != 2 {
		t.Fatalf("both available and unavailable items must remain visible: %+v", got[0].Items)
	}
	projection, ok := resp["item_orderability"].(map[string]services.Orderability)
	if !ok {
		t.Fatalf("item_orderability must be a typed projection: %#v", resp["item_orderability"])
	}
	if projection["i1"].State != services.OrderabilityAvailable || !projection["i1"].Orderable {
		t.Fatalf("available item projection is wrong: %+v", projection["i1"])
	}
	if projection["i2"].State != services.OrderabilityManualDisabled || projection["i2"].Orderable {
		t.Fatalf("manual unavailable projection is wrong: %+v", projection["i2"])
	}
}

func TestBuildPublicGuestMenuResponse_LapsedSubscriptionDisablesOrderability(t *testing.T) {
	cats := []database.MenuCategory{{
		ID: "c1", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "i1", Name: "Burger", Price: 10, IsAvailable: true},
		},
	}}
	raw, _ := json.Marshal(cats)
	menu := &database.Menu{Categories: string(raw)}
	business := &database.Business{
		KitchenEnabled: true,
		OrdersEnabled:  true,
	}

	resp := buildPublicGuestMenuResponse(menu, business)
	projection, ok := resp["item_orderability"].(map[string]services.Orderability)
	if !ok {
		t.Fatalf("item_orderability must be a typed projection: %#v", resp["item_orderability"])
	}
	if projection["i1"].Orderable {
		t.Fatalf("lapsed venue must not advertise orderable items: %+v", projection["i1"])
	}
	if projection["i1"].State != services.OrderabilityOrderingOff {
		t.Fatalf("lapsed venue orderability state=%q want ordering_disabled", projection["i1"].State)
	}
}

// Malformed/empty stored JSON must not panic and must serve an empty menu.
func TestBuildPublicGuestMenuResponse_MalformedJSONIsEmpty(t *testing.T) {
	resp := buildPublicGuestMenuResponse(&database.Menu{Categories: "not-json"})
	if resp["categories"] == nil {
		t.Fatal("malformed menu must serve an empty (non-nil) categories array, not nil")
	}
}

func BenchmarkBuildPublicGuestMenuResponse(b *testing.B) {
	// Realistic 8-category × 12-item menu, half unavailable.
	cats := make([]database.MenuCategory, 8)
	for c := range cats {
		items := make([]database.MenuItem, 12)
		for i := range items {
			items[i] = database.MenuItem{
				ID:          "item",
				Name:        "Item",
				Price:       1000,
				IsAvailable: i%2 == 0, // half available
			}
		}
		cats[c] = database.MenuCategory{
			ID:    "cat",
			Name:  "Category",
			Items: items,
		}
	}
	raw, _ := json.Marshal(cats)
	menu := &database.Menu{Categories: string(raw)}

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		_ = buildPublicGuestMenuResponse(menu)
	}
}
