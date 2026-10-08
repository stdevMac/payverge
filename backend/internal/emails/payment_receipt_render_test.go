package emails

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func realTemplatesRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller for templates root")
	}
	return filepath.Join(filepath.Dir(currentFile), "..", "..", "email", "templates")
}

func TestPaymentReceiptEscapesItemNames(t *testing.T) {
	tm, err := NewTemplateManager(realTemplatesRoot(t))
	if err != nil {
		t.Fatalf("new template manager: %v", err)
	}

	data := map[string]interface{}{
		"business_name":  "Cafe Test",
		"payment_date":   "January 2, 2026 at 3:04 PM",
		"payment_method": "Card",
		"transaction_id": "tx_123",
		"items": []map[string]interface{}{
			{"name": "Fish & <Chips>", "quantity": 2, "line_total": "$24.00"},
		},
		"total_amount": "$24.00",
	}

	// All three locale templates were edited in lockstep; every one must escape
	// item names natively. "en" normalizes to the eng family.
	for _, lang := range []string{"en", "es", "es_ar"} {
		t.Run(lang, func(t *testing.T) {
			body, err := tm.Render(lang, "payment_receipt", data)
			if err != nil {
				t.Fatalf("render: %v", err)
			}

			if !strings.Contains(body, "Fish &amp; &lt;Chips&gt;") {
				t.Fatalf("expected escaped item name in body, got:\n%s", body)
			}
			if strings.Contains(body, "Fish & <Chips>") {
				t.Fatalf("raw unescaped item name leaked into body")
			}
			if !strings.Contains(body, "$24.00") {
				t.Fatalf("expected line total in body")
			}
		})
	}
}

func TestPaymentReceiptEmptyItemsRenders(t *testing.T) {
	tm, err := NewTemplateManager(realTemplatesRoot(t))
	if err != nil {
		t.Fatalf("new template manager: %v", err)
	}
	data := map[string]interface{}{
		"business_name":  "Cafe Test",
		"payment_date":   "January 2, 2026",
		"payment_method": "Cash",
		"transaction_id": "tx_0",
		"items":          []map[string]interface{}{},
		"total_amount":   "$0.00",
	}
	if _, err := tm.Render("en", "payment_receipt", data); err != nil {
		t.Fatalf("render with empty items: %v", err)
	}
}
