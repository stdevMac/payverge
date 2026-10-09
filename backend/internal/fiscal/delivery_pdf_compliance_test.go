package fiscal

import (
	"bytes"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func complianceTestReceipt(receiptType string) (database.FiscalReceipt, database.Business, database.BusinessFiscalSettings) {
	now := time.Now()
	num := "00000042"
	cae := "71234567890123"
	qr := "https://www.arca.gob.ar/fe/qr/?p=eyJ0ZXN0IjoxfQ=="
	pos := 3
	return database.FiscalReceipt{
			ReceiptType:      receiptType,
			ReceiptNumber:    &num,
			AuthCode:         &cae,
			QRPayload:        &qr,
			TotalAmountCents: 121000, // ARS 1.210,00 → IVA contenido 210,00 at 21%
			IssuedAt:         &now,
		},
		database.Business{Name: "Parrilla Test"},
		database.BusinessFiscalSettings{TaxID: "30123456789", TaxCondition: "responsable_inscripto", PointOfSale: &pos}
}

// RG 5614/2024 (Ley 27.743): B-type receipts must carry the consumer tax
// transparency legend with the IVA contained in the price.
func TestFacturaBPDFCarriesTransparencyLegend(t *testing.T) {
	receipt, business, settings := complianceTestReceipt("factura_b")
	pdf, err := renderReceiptPDFWithCompression(receipt, business, settings, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Transparencia Fiscal al Consumidor", // ASCII-safe fragment of the legend title
		"Ley 27.743",
		"IVA Contenido",
		"ARS 210,00", // 21% contained in 1.210,00
		"Otros Impuestos Nacionales Indirectos",
	} {
		if !bytes.Contains(pdf, []byte(want)) {
			t.Errorf("factura_b PDF missing RG 5614 fragment %q", want)
		}
	}
}

// The legend is specific to B-type documents: factura_c has no discriminated
// IVA and factura_a already discriminates it for the RI recipient.
func TestFacturaCPDFOmitsTransparencyLegend(t *testing.T) {
	receipt, business, settings := complianceTestReceipt("factura_c")
	settings.TaxCondition = "monotributo"
	pdf, err := renderReceiptPDFWithCompression(receipt, business, settings, false)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pdf, []byte("Transparencia Fiscal al Consumidor")) {
		t.Fatal("factura_c PDF must NOT carry the RG 5614 legend")
	}
}

// The emitter identity block must print the humanized condition, not the slug.
func TestPDFEmitterTaxConditionIsHumanized(t *testing.T) {
	receipt, business, settings := complianceTestReceipt("factura_b")
	pdf, err := renderReceiptPDFWithCompression(receipt, business, settings, false)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pdf, []byte("responsable_inscripto")) {
		t.Fatal("emitter block prints the raw tax-condition slug")
	}
	if !bytes.Contains(pdf, []byte("IVA Responsable Inscripto")) {
		t.Fatal("emitter block missing humanized tax condition")
	}
}
