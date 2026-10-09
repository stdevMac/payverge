package formatters

import (
	"strings"
	"testing"
)

func fiscalReceiptInput() ReceiptInput {
	return ReceiptInput{
		BillInput: BillInput{
			BusinessName: "Parrilla Test",
			BillNumber:   "B-001",
			Language:     "es",
			Total:        1210.00,
		},
		TotalPaid:     1210.00,
		PaymentMethod: "Efectivo",
		Fiscal: &FiscalTicketInfo{
			ReceiptTitle:  "FACTURA B",
			ReceiptNumber: "0003-00000042",
			EmitterCUIT:   "30123456789",
			EmitterIVA:    "IVA Responsable Inscripto",
			CAE:           "71234567890123",
			CAEExpiry:     "20/08/2026",
			IVAContained:  "ARS 210,00",
			OtherTaxes:    "ARS 0,00",
			QRDataURI:     "data:image/png;base64,iVBORw0KGgo=",
		},
	}
}

// A ticket carrying fiscal data must print every legally required field:
// document title, número, CUIT, CAE + expiry, RG 5614 legend, and the ARCA QR.
func TestReceiptRendersFiscalBlock(t *testing.T) {
	html, err := Receipt(fiscalReceiptInput(), 80)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"FACTURA B",
		"0003-00000042",
		"30123456789",
		"IVA Responsable Inscripto",
		"71234567890123",
		"20/08/2026",
		"Transparencia Fiscal al Consumidor",
		"Ley 27.743",
		"IVA Contenido",
		"ARS 210,00",
		`src="data:image/png;base64,iVBORw0KGgo="`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("fiscal ticket missing %q", want)
		}
	}
	// html/template rewrites unsafe URLs to "#ZgotmplZ" — the QR data URI must
	// survive (that's why QRDataURI is template.URL, not string).
	if strings.Contains(html, "ZgotmplZ") {
		t.Fatal("QR data URI was escaped away by html/template")
	}
}

// Without fiscal info the ticket is the unchanged courtesy receipt.
func TestReceiptWithoutFiscalBlockUnchanged(t *testing.T) {
	in := fiscalReceiptInput()
	in.Fiscal = nil
	html, err := Receipt(in, 80)
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"CAE", "FACTURA", "Transparencia"} {
		if strings.Contains(html, absent) {
			t.Errorf("courtesy receipt must not contain %q", absent)
		}
	}
}

// The RG 5614 sub-block only renders when IVAContained is set (B-type letters);
// a factura C fiscal block prints CAE/número but no legend.
func TestReceiptFiscalBlockLegendOnlyWhenProvided(t *testing.T) {
	in := fiscalReceiptInput()
	in.Fiscal.ReceiptTitle = "FACTURA C"
	in.Fiscal.IVAContained = ""
	in.Fiscal.OtherTaxes = ""
	html, err := Receipt(in, 80)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "Transparencia Fiscal") {
		t.Fatal("legend must be omitted when IVAContained is empty")
	}
	if !strings.Contains(html, "71234567890123") {
		t.Fatal("CAE must still print")
	}
}
