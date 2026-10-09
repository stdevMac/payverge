package formatters

import (
	"strings"
	"testing"
	"time"
)

func sampleBillInput(lang string) BillInput {
	return BillInput{
		BusinessName:     "Test Bistro",
		TableName:        "Table 4",
		BillNumber:       "B-0001",
		CreatedAt:        "2026-05-19 19:32",
		Items:            []BillLineItem{{Name: "Burger", Quantity: 2, Subtotal: 24.00}},
		Subtotal:         24.00,
		TaxAmount:        2.40,
		ServiceFeeAmount: 1.00,
		Total:            27.40,
		Currency:         "USD",
		Language:         lang,
	}
}

func sampleReceiptInput(lang string) ReceiptInput {
	return ReceiptInput{
		BillInput:     sampleBillInput(lang),
		TipAmount:     3.00,
		PaymentMethod: "card_visa",
		TransactionID: "txn_abc123",
		TotalPaid:     30.40,
	}
}

func sampleKitchenInput(lang string) KitchenTicketInput {
	return KitchenTicketInput{
		BusinessName: "Test Bistro",
		TableName:    "T-4",
		BillNumber:   "B-0001",
		OrderNumber:  "ORD-99",
		CreatedAt:    "2026-05-23 19:32",
		Items: []KitchenLineItem{
			{Name: "Margherita", Quantity: 1, Allergens: []string{"gluten", "dairy"}},
		},
		Language: lang,
	}
}

func TestBill_SpanishLabels(t *testing.T) {
	for _, lang := range []string{"es", "es-AR", "es_ar"} {
		got, err := Bill(sampleBillInput(lang), 80)
		if err != nil {
			t.Fatalf("[%s] unexpected err: %v", lang, err)
		}
		for _, want := range []string{
			">Cuenta</span>", ">Mesa</span>", ">Fecha</span>",
			">Subtotal</span>", ">Impuesto</span>", ">Servicio</span>",
			"PRECUENTA — No es un recibo",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("[%s] expected output to contain %q, got:\n%s", lang, want, got)
			}
		}
		for _, notWant := range []string{
			">Bill</span>", ">Table</span>", ">Date</span>",
			">Tax</span>", ">Service</span>", "PRE-BILL", "Not a receipt",
		} {
			if strings.Contains(got, notWant) {
				t.Errorf("[%s] did not expect English label %q in Spanish output:\n%s", lang, notWant, got)
			}
		}
	}
}

func TestBill_EnglishUnchanged(t *testing.T) {
	for _, lang := range []string{"en", "", "fr"} {
		got, err := Bill(sampleBillInput(lang), 80)
		if err != nil {
			t.Fatalf("[%q] unexpected err: %v", lang, err)
		}
		for _, want := range []string{
			">Bill</span>", ">Table</span>", ">Date</span>",
			">Tax</span>", ">Service</span>", "PRE-BILL — Not a receipt",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("[%q] expected English label %q, got:\n%s", lang, want, got)
			}
		}
		if strings.Contains(got, "Cuenta") || strings.Contains(got, "PRECUENTA") {
			t.Errorf("[%q] did not expect Spanish labels in English output:\n%s", lang, got)
		}
	}
}

func TestReceipt_SpanishLabels(t *testing.T) {
	for _, lang := range []string{"es", "es-AR"} {
		got, err := Receipt(sampleReceiptInput(lang), 80)
		if err != nil {
			t.Fatalf("[%s] unexpected err: %v", lang, err)
		}
		for _, want := range []string{
			">RECIBO</div>", ">Cuenta</span>", ">Mesa</span>", ">Fecha</span>",
			">Propina</span>", ">Método</span>", ">Transacción</span>",
			"Total pagado", "*** PAGADO ***", "¡Gracias!",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("[%s] expected output to contain %q, got:\n%s", lang, want, got)
			}
		}
		for _, notWant := range []string{
			">Bill</span>", ">Tip</span>", ">Method</span>",
			">Txn</span>", "Total Paid", "*** PAID ***", "Thank you!",
		} {
			if strings.Contains(got, notWant) {
				t.Errorf("[%s] did not expect English label %q in Spanish output:\n%s", lang, notWant, got)
			}
		}
	}
}

func TestReceipt_EnglishUnchanged(t *testing.T) {
	got, err := Receipt(sampleReceiptInput("en"), 80)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, want := range []string{
		">RECEIPT</div>", ">Tip</span>", ">Method</span>", ">Txn</span>",
		"Total Paid", "*** PAID ***", "Thank you!",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected English label %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "PAGADO") || strings.Contains(got, "Gracias") {
		t.Errorf("did not expect Spanish labels in English output:\n%s", got)
	}
}

func TestKitchen_SpanishLabels(t *testing.T) {
	for _, lang := range []string{"es", "es-AR"} {
		got, err := KitchenTicket(sampleKitchenInput(lang), 80)
		if err != nil {
			t.Fatalf("[%s] unexpected err: %v", lang, err)
		}
		for _, want := range []string{
			// The localized order-number label must carry its own separator: it
			// is glued directly to the number in the template ({{.L.OrderNum}}{{.OrderNumber}}),
			// and "Pedido N.º" has no tight prefix like the English "#", so it
			// needs a trailing space to read "Pedido N.º ORD-99" not "...N.ºORD-99".
			"COCINA", "Pedido N.º ORD-99", "Mesa T-4", "! ALÉRGENOS:",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("[%s] expected output to contain %q, got:\n%s", lang, want, got)
			}
		}
		// Safety: the English allergen header must NOT appear on a Spanish ticket.
		if strings.Contains(got, "ALLERGENS") {
			t.Errorf("[%s] English ALLERGENS header leaked onto Spanish kitchen ticket:\n%s", lang, got)
		}
		for _, notWant := range []string{"KITCHEN", "Order #", "Table T-4"} {
			if strings.Contains(got, notWant) {
				t.Errorf("[%s] did not expect English label %q in Spanish output:\n%s", lang, notWant, got)
			}
		}
	}
}

func TestKitchen_EnglishUnchanged(t *testing.T) {
	got, err := KitchenTicket(sampleKitchenInput("en"), 80)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, want := range []string{"KITCHEN", "Order #ORD-99", "Table T-4", "! ALLERGENS:"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected English label %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "COCINA") || strings.Contains(got, "ALÉRGENOS") {
		t.Errorf("did not expect Spanish labels in English output:\n%s", got)
	}
}

func TestPaymentMethodLabel_Localizes(t *testing.T) {
	cases := []struct{ lang, method, want string }{
		{"en", "cash", "Cash"},
		{"en", "card", "Card"},
		{"en", "crypto", "Crypto"},
		{"en", "cross-chain", "Crypto (cross-chain)"},
		{"en", "plugin", "Online payment"},
		{"en", "USDC", "USDC"},
		{"es", "cash", "Efectivo"},
		{"es", "card", "Tarjeta"},
		{"es", "crypto", "Cripto"},
		{"es", "cross-chain", "Cripto (multi-cadena)"},
		{"es", "plugin", "Pago en línea"},
		{"es", "mercadopago", "Mercado Pago"},
		{"es-AR", "cash", "Efectivo"},
		{"es-AR", "stripe", "Tarjeta (Stripe)"},
		// Unknown methods pass through untouched — better a raw enum than a
		// blank line on a guest receipt.
		{"en", "pm_custom_thing", "pm_custom_thing"},
		{"es", "", ""},
		// Unsupported print locale falls back to the English label set.
		{"fr", "cash", "Cash"},
	}
	for _, c := range cases {
		if got := PaymentMethodLabel(c.lang, c.method); got != c.want {
			t.Errorf("PaymentMethodLabel(%q, %q) = %q, want %q", c.lang, c.method, got, c.want)
		}
	}
}

func TestFormatTicketTime_Localizes(t *testing.T) {
	ts := time.Date(2026, 7, 8, 19, 32, 0, 0, time.UTC)
	cases := map[string]string{
		"en":    "2026-07-08 19:32",
		"":      "2026-07-08 19:32",
		"fr":    "2026-07-08 19:32",
		"es":    "08/07/2026 19:32",
		"es-AR": "08/07/2026 19:32",
		"es_ar": "08/07/2026 19:32",
	}
	for lang, want := range cases {
		if got := FormatTicketTime(lang, ts); got != want {
			t.Errorf("FormatTicketTime(%q) = %q, want %q", lang, got, want)
		}
	}
}
