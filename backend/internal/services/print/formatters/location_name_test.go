package formatters

import (
	"strings"
	"testing"
)

// #825: table/counter display names are stored WITH the type word embedded
// ("Table 1", "Counter 3") for demo and default-provisioned venues, and every
// printed document prepended the localized type word again — "Table Table 1"
// on the kitchen ticket, "Table | Table 1" on the bill and receipt rows.
// Mirrors frontend/src/lib/tableLabel.ts formatEntityName.
func TestEntityValueStripsTheRedundantTypeWord(t *testing.T) {
	cases := []struct {
		label string
		name  string
		want  string
	}{
		// The live demo venue: names are seeded as "Table N".
		{"Table", "Table 1", "1"},
		{"Table", "Table 12", "12"},
		{"Counter", "Counter 3", "3"},
		// Localized label over an English seed name.
		{"Mesa", "Table 9", "9"},
		{"Mesa", "Mesa 9", "9"},
		{"Barra", "Counter 3", "3"},
		// A bare type word carries no identity of its own.
		{"Table", "Table", ""},
		{"Mesa", "Table", ""},
		{"Table", "", ""},
		{"Table", "   ", ""},
		// Custom operator names keep their identity; the label column
		// already says "Table", so the value is the name as typed.
		{"Table", "Patio A", "Patio A"},
		{"Mesa", "Patio A", "Patio A"},
		// A custom name that merely STARTS with the type word as a longer
		// word must not be truncated.
		{"Mesa", "Tablecloth Corner", "Tablecloth Corner"},
		{"Table", "Tablecloth Corner", "Tablecloth Corner"},
		// Casing and padding as stored.
		{"Table", "table 7", "7"},
		{"Table", "  Table 7  ", "7"},
	}
	for _, tc := range cases {
		if got := EntityValue(tc.label, tc.name); got != tc.want {
			t.Errorf("EntityValue(%q, %q) = %q, want %q", tc.label, tc.name, got, tc.want)
		}
	}
}

func TestEntityNameNeverDoublesTheTypeWord(t *testing.T) {
	cases := []struct {
		label string
		name  string
		want  string
	}{
		{"Table", "Table 1", "Table 1"},
		{"Mesa", "Table 9", "Mesa 9"},
		{"Counter", "Counter 3", "Counter 3"},
		{"Table", "T-4", "Table T-4"},
		{"Table", "Patio A", "Table Patio A"},
		{"Mesa", "Table", "Mesa"},
		{"Table", "", "Table"},
	}
	for _, tc := range cases {
		if got := EntityName(tc.label, tc.name); got != tc.want {
			t.Errorf("EntityName(%q, %q) = %q, want %q", tc.label, tc.name, got, tc.want)
		}
	}
}

// The three printed documents must never emit the doubled word for a seeded
// name. Live print jobs 1110-1115 on the demo venue all carry "Table N".
func TestPrintedDocumentsDoNotDoubleTheTableWord(t *testing.T) {
	kitchen, err := KitchenTicket(KitchenTicketInput{
		BusinessName: "Test Bistro",
		TableName:    "Table 1",
		BillNumber:   "B-0001",
		OrderNumber:  "ORD-99",
		CreatedAt:    "2026-08-22 19:32",
		Items:        []KitchenLineItem{{Name: "Margherita", Quantity: 1}},
		Language:     "en",
	}, 80)
	if err != nil {
		t.Fatalf("KitchenTicket: %v", err)
	}
	if strings.Contains(kitchen, "Table Table") {
		t.Errorf("kitchen ticket doubles the table word:\n%s", kitchen)
	}
	if !strings.Contains(kitchen, "Table 1") {
		t.Errorf("kitchen ticket lost the table identity:\n%s", kitchen)
	}

	bill, err := Bill(BillInput{
		BusinessName: "Test Bistro",
		TableName:    "Table 4",
		BillNumber:   "B-0001",
		CreatedAt:    "2026-08-22 19:32",
		Items:        []BillLineItem{{Name: "Margherita", Quantity: 1, Subtotal: 10, Kind: BillLineKindItem}},
		Subtotal:     10,
		Total:        10,
		Language:     "en",
	}, 80)
	if err != nil {
		t.Fatalf("Bill: %v", err)
	}
	if strings.Contains(bill, "Table</span><span class=\"value\">Table 4") {
		t.Errorf("bill doubles the table word:\n%s", bill)
	}
	if !strings.Contains(bill, ">Table</span>") {
		t.Errorf("bill lost the table label row:\n%s", bill)
	}
	if !strings.Contains(bill, ">4</span>") {
		t.Errorf("bill lost the table identity:\n%s", bill)
	}
}
