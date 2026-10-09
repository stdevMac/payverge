package formatters

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestBillFormatter_80mm_GoldenHTML(t *testing.T) {
	input := BillInput{
		BusinessName: "Test Bistro",
		TableName:    "Table 4",
		BillNumber:   "B-0001",
		CreatedAt:    "2026-05-19T19:32:00Z",
		Items: []BillLineItem{
			{Name: "Burger", Quantity: 2, Subtotal: 24.00},
			{Name: "Fries", Quantity: 1, Subtotal: 5.50},
		},
		Subtotal:         29.50,
		TaxAmount:        2.95,
		ServiceFeeAmount: 1.48,
		Total:            33.93,
		Currency:         "USD",
		Language:         "en",
	}

	got, err := Bill(input, 80)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	goldenPath := filepath.Join("..", "testdata", "bill_80mm.golden.html")
	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file missing: %v — write it by hand the first time", err)
	}
	if strings.TrimSpace(got) != strings.TrimSpace(string(wantBytes)) {
		t.Fatalf("HTML diverged from golden.\nGOT:\n%s\n\nWANT:\n%s", got, string(wantBytes))
	}
}

func TestBillFormatter_UsesExplicitThermalPageSizeAtBothWidths(t *testing.T) {
	input := BillInput{
		BusinessName:    "Long Restaurant Name",
		BusinessAddress: "123 Long Avenue, Buenos Aires",
		Items:           []BillLineItem{{Name: "A long item name that must wrap safely", Quantity: 1, Subtotal: 12}},
		Language:        "en",
	}
	for _, width := range []int{58, 80} {
		html, err := Bill(input, width)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(html, "mm auto") {
			t.Fatalf("%dmm CSS contains invalid auto page height", width)
		}
		wantWidth := regexp.MustCompile(`@page \{ size: ` + regexp.QuoteMeta(fmt.Sprintf("%dmm", width)) + ` [0-9]+mm;`)
		if !wantWidth.MatchString(html) {
			t.Fatalf("%dmm output lacks explicit thermal page dimensions: %s", width, html)
		}
		if !strings.Contains(html, fmt.Sprintf("width: %dmm", width)) {
			t.Fatalf("%dmm output does not constrain body width", width)
		}
		if !strings.Contains(html, "overflow-wrap: anywhere") {
			t.Fatalf("%dmm output does not protect long item names", width)
		}
	}
}

func TestBillFormatter_RejectsUnsupportedWidth(t *testing.T) {
	_, err := Bill(BillInput{BusinessName: "x"}, 110)
	if err == nil {
		t.Fatal("expected error for unsupported paper width")
	}
}

func TestBillFormatter_DiscountDoesNotPrintAsAQuantity(t *testing.T) {
	html, err := Bill(BillInput{
		Items: []BillLineItem{{Name: "Bundle discount", Quantity: 1, Subtotal: -4, Kind: BillLineKindDiscount}},
	}, 80)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "1× Bundle discount") {
		t.Fatalf("discount was rendered like a purchased item: %s", html)
	}
	if !strings.Contains(html, ">Bundle discount</span><span class=\"item-amount\">-4.00") {
		t.Fatalf("discount line is missing: %s", html)
	}
}
