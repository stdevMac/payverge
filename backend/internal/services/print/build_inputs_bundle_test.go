package print

import "testing"

func TestDecodeBillItems_GroupsBundleComponentsWithoutDuplicatePrices(t *testing.T) {
	raw := `[
      {"name":"Steak Plate","quantity":1,"subtotal":0,"item_type":"bundle_item","parent_bundle_id":9},
      {"name":"Date Night for Two","quantity":1,"subtotal":62.27,"item_type":"bundle","bundle_id":9},
      {"name":"Chocolate Cake","quantity":1,"subtotal":0,"item_type":"bundle_item","parent_bundle_id":9},
      {"name":"Bundle discount","quantity":1,"subtotal":-5,"item_type":"discount"}
    ]`

	items, err := decodeBillItems(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("want 4 visible lines, got %#v", items)
	}
	if items[0].Name != "Date Night for Two" || items[0].Kind != "item" || items[0].Subtotal != 62.27 {
		t.Fatalf("bundle charge should print once first, got %#v", items[0])
	}
	for _, detail := range items[1:3] {
		if detail.Kind != "detail" || detail.Subtotal != 0 {
			t.Fatalf("included bundle components must be no-price detail rows: %#v", detail)
		}
	}
	if items[3].Kind != "discount" || items[3].Subtotal != -5 {
		t.Fatalf("discount must remain an explicit adjustment: %#v", items[3])
	}
}

func TestDecodeBillItems_KeepsRepeatedBundleOrdersSeparate(t *testing.T) {
	raw := `[
      {"name":"Combo","quantity":1,"subtotal":20,"item_type":"bundle","bundle_id":7,"order_id":101},
      {"name":"Burger","quantity":1,"subtotal":0,"item_type":"bundle_item","parent_bundle_id":7,"order_id":101},
      {"name":"Combo","quantity":1,"subtotal":20,"item_type":"bundle","bundle_id":7,"order_id":102},
      {"name":"Salad","quantity":1,"subtotal":0,"item_type":"bundle_item","parent_bundle_id":7,"order_id":102}
    ]`

	items, err := decodeBillItems(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("want two parents with one detail each, got %#v", items)
	}
	if items[0].Name != "Combo" || items[1].Name != "Burger" || items[1].Kind != "detail" {
		t.Fatalf("first order grouping is wrong: %#v", items[:2])
	}
	if items[2].Name != "Combo" || items[3].Name != "Salad" || items[3].Kind != "detail" {
		t.Fatalf("second order grouping is wrong: %#v", items[2:])
	}
}

func TestDecodeBillItems_KeepsRepeatedBundleOccurrencesInOneOrderSeparate(t *testing.T) {
	raw := `[
      {"name":"Combo","quantity":1,"subtotal":20,"item_type":"bundle","bundle_id":7,"order_id":101,"bundle_occurrence_id":"first"},
      {"name":"Burger","quantity":1,"subtotal":0,"item_type":"bundle_item","parent_bundle_id":7,"order_id":101,"bundle_occurrence_id":"first"},
      {"name":"Combo","quantity":1,"subtotal":20,"item_type":"bundle","bundle_id":7,"order_id":101,"bundle_occurrence_id":"second"},
      {"name":"Salad","quantity":1,"subtotal":0,"item_type":"bundle_item","parent_bundle_id":7,"order_id":101,"bundle_occurrence_id":"second"}
    ]`

	items, err := decodeBillItems(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || items[1].Name != "Burger" || items[3].Name != "Salad" {
		t.Fatalf("each repeated parent must render only its own children: %#v", items)
	}
}
