package formatters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReceiptFormatter_80mm_GoldenHTML(t *testing.T) {
	in := ReceiptInput{
		BillInput: BillInput{
			BusinessName: "Test Bistro",
			TableName:    "Table 4",
			BillNumber:   "B-0001",
			CreatedAt:    "2026-05-19T19:38:00Z",
			Items:        []BillLineItem{{Name: "Burger", Quantity: 2, Subtotal: 24.00}},
			Subtotal:     24.00,
			TaxAmount:    2.40,
			Total:        29.40,
			Currency:     "USD",
			Language:     "en",
		},
		TipAmount:     3.00,
		PaymentMethod: "card_visa",
		TransactionID: "txn_abc123",
		TotalPaid:     29.40,
	}
	got, err := Receipt(in, 80)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want, err := os.ReadFile(filepath.Join("..", "testdata", "receipt_80mm.golden.html"))
	if err != nil {
		t.Fatalf("golden file missing: %v", err)
	}
	if strings.TrimSpace(got) != strings.TrimSpace(string(want)) {
		t.Fatalf("HTML diverged.\nGOT:\n%s\nWANT:\n%s", got, string(want))
	}
}

func TestReceiptFormatter_RejectsUnsupportedWidth(t *testing.T) {
	_, err := Receipt(ReceiptInput{}, 110)
	if err == nil {
		t.Fatal("expected error for unsupported paper width")
	}
}

func TestReceiptFormatter_PrintsAddressAndWrapsTransactionReference(t *testing.T) {
	html, err := Receipt(ReceiptInput{
		BillInput: BillInput{
			BusinessName:    "Test Bistro",
			BusinessAddress: "123 Main Street, Buenos Aires",
			Language:        "en",
		},
		TransactionID: "0xverylongtransactionreferencethatmustneveroverflowthepaper",
	}, 58)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"123 Main Street, Buenos Aires",
		"class=\"transaction\"",
		"overflow-wrap: anywhere",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("receipt missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "mm auto") {
		t.Fatal("receipt uses invalid auto page height")
	}
}

func TestReceiptFormatter_LabelsDocumentOnce(t *testing.T) {
	html, err := Receipt(ReceiptInput{BillInput: BillInput{Language: "en"}}, 80)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, ">RECEIPT</div>") {
		t.Fatalf("receipt does not identify the document: %s", html)
	}
	if got := strings.Count(html, "*** PAID ***"); got != 1 {
		t.Fatalf("paid stamp rendered %d times, want 1", got)
	}
}
