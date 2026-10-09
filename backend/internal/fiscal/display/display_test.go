package display

import "testing"

func TestReceiptTitle(t *testing.T) {
	cases := map[string]string{
		"factura_a":         "FACTURA A",
		"factura_b":         "FACTURA B",
		"factura_c":         "FACTURA C",
		"nota_de_credito_b": "NOTA DE CRÉDITO B",
		"nota_credito_c":    "NOTA DE CRÉDITO C",
		// US / non-AFIP types must never render as Factura A/B/C (#255).
		"invoice":          "INVOICE",
		"receipt":          "RECEIPT",
		"standard_invoice": "STANDARD INVOICE",
		"weird_type":       "WEIRD_TYPE",
	}
	for in, want := range cases {
		if got := ReceiptTitle(in); got != want {
			t.Errorf("ReceiptTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanTaxCondition(t *testing.T) {
	if got := HumanTaxCondition("responsable_inscripto"); got != "IVA Responsable Inscripto" {
		t.Fatalf("got %q", got)
	}
	if got := HumanTaxCondition("monotributo"); got != "Responsable Monotributo" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatARS(t *testing.T) {
	if got := FormatARS(121000); got != "ARS 1.210,00" {
		t.Fatalf("FormatARS(121000) = %q", got)
	}
	if got := FormatARS(-5); got != "ARS -0,05" {
		t.Fatalf("FormatARS(-5) = %q", got)
	}
}

func TestSplitInclusiveVATReconciles(t *testing.T) {
	net, vat := SplitInclusiveVAT(121000, DefaultArgentinaVATRate)
	if net+vat != 121000 {
		t.Fatalf("net %d + vat %d != total", net, vat)
	}
	if net != 100000 || vat != 21000 {
		t.Fatalf("21%% split of 1210.00 = net %d / vat %d, want 100000/21000", net, vat)
	}
	net, vat = SplitInclusiveVAT(5000, 0)
	if net != 5000 || vat != 0 {
		t.Fatalf("exento split: net %d vat %d", net, vat)
	}
}

func TestIsTransparencyLegendType(t *testing.T) {
	for _, rt := range []string{"factura_b", "nota_de_credito_b", "nota_credito_b", "FACTURA_B"} {
		if !IsTransparencyLegendType(rt) {
			t.Errorf("IsTransparencyLegendType(%q) = false, want true", rt)
		}
	}
	for _, rt := range []string{"factura_a", "factura_c", "nota_de_credito_c", ""} {
		if IsTransparencyLegendType(rt) {
			t.Errorf("IsTransparencyLegendType(%q) = true, want false", rt)
		}
	}
}
