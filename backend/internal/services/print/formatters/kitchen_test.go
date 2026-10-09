package formatters

import (
	"strings"
	"testing"
)

func TestKitchenTicket_RendersModifiersAndAllergens(t *testing.T) {
	input := KitchenTicketInput{
		BusinessName: "Test Bistro",
		TableName:    "T-4",
		BillNumber:   "B-0001",
		OrderNumber:  "ORD-99",
		CreatedAt:    "2026-05-23 19:32",
		OrderNotes:   "VIP — rush",
		Items: []KitchenLineItem{
			{
				Name:      "Margherita",
				Quantity:  1,
				Modifiers: []string{"No olives", "Extra basil"},
				Notes:     "Light cheese please",
				Allergens: []string{"gluten", "dairy"},
			},
			{
				Name:     "House Salad",
				Quantity: 2,
			},
		},
		Language: "en",
	}

	got, err := KitchenTicket(input, 80)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	mustContain := []string{
		"KITCHEN",
		"Order #ORD-99",
		"Table T-4",
		"** VIP — rush **",
		"1x Margherita",
		"+ No olives",
		"+ Extra basil",
		"Light cheese please",
		"gluten",
		"dairy",
		"ALLERGENS",
		"2x House Salad",
	}
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, got)
		}
	}
}

func TestKitchenTicket_RejectsUnsupportedWidth(t *testing.T) {
	_, err := KitchenTicket(KitchenTicketInput{BusinessName: "x"}, 110)
	if err == nil {
		t.Fatal("expected error for unsupported paper width")
	}
}

func TestKitchenTicket_HandlesEmptyOptionalFields(t *testing.T) {
	input := KitchenTicketInput{
		BusinessName: "Test Bistro",
		TableName:    "T-1",
		BillNumber:   "B-1",
		OrderNumber:  "ORD-1",
		CreatedAt:    "2026-05-23 19:32",
		Items: []KitchenLineItem{
			{Name: "Coffee", Quantity: 1},
		},
		Language: "en",
	}

	got, err := KitchenTicket(input, 58)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// No allergen banner when none set.
	if strings.Contains(got, "ALLERGENS") {
		t.Errorf("did not expect ALLERGENS banner when no allergens set:\n%s", got)
	}
	// No order-notes banner when none set.
	if strings.Contains(got, "**  **") {
		t.Errorf("did not expect empty order-notes banner:\n%s", got)
	}
}
