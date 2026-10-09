package fiscal

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func strptr(s string) *string { return &s }

// authorizedFacturaCFixture returns a typical authorized factura_c receipt with
// a CAE, receipt number, amounts (in cents), and an AFIP QR payload set, plus a
// matching emitter business and fiscal settings.
func authorizedFacturaCFixture() (database.FiscalReceipt, database.Business, database.BusinessFiscalSettings) {
	issued := time.Date(2026, 6, 19, 13, 30, 0, 0, time.UTC)
	caeExp := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	pos := 3
	receipt := database.FiscalReceipt{
		ID:               42,
		BusinessID:       7,
		Country:          "AR",
		Provider:         "afip",
		Action:           "issue",
		ReceiptType:      "factura_c",
		ReceiptNumber:    strptr("00000043"),
		AuthCode:         strptr("75123456789012"),
		AuthExpiresAt:    &caeExp,
		QRPayload:        strptr("https://www.afip.gob.ar/fe/qr/?p=eyJ2ZXIiOjEsImZlY2hhIjoiMjAyNi0wNi0xOSJ9"),
		TotalAmountCents: 121000, // $1,210.00 (incl. 21% IVA when applicable)
		Currency:         "ARS",
		IssuedAt:         &issued,
		Status:           database.FiscalStatusAuthorized,
	}
	business := database.Business{
		ID:   7,
		Name: "La Parrilla del Centro",
	}
	settings := database.BusinessFiscalSettings{
		BusinessID:   7,
		Country:      "AR",
		Provider:     "afip",
		TaxID:        "30712345678",
		TaxCondition: "Responsable Inscripto",
		PointOfSale:  &pos,
	}
	return receipt, business, settings
}

func TestRenderAFIPQRPNG(t *testing.T) {
	payload := "https://www.afip.gob.ar/fe/qr/?p=eyJ2ZXIiOjEsImZlY2hhIjoiMjAyNi0wNi0xOSJ9"
	out, err := RenderAFIPQRPNG(payload)
	if err != nil {
		t.Fatalf("RenderAFIPQRPNG returned error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("RenderAFIPQRPNG returned empty bytes")
	}
	// PNG magic header.
	pngMagic := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if len(out) < len(pngMagic) || !bytes.Equal(out[:len(pngMagic)], pngMagic) {
		t.Fatalf("output is not a PNG: first bytes %v", out[:min(len(out), len(pngMagic))])
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("png.Decode failed: %v", err)
	}
	b := img.Bounds()
	if b.Dx() < 32 || b.Dy() < 32 {
		t.Fatalf("decoded QR image is too small: %dx%d", b.Dx(), b.Dy())
	}
}

func TestRenderAFIPQRPNGEmptyPayload(t *testing.T) {
	if _, err := RenderAFIPQRPNG(""); err == nil {
		t.Fatal("expected error for empty QR payload, got nil")
	}
}

func TestRenderReceiptPDFFacturaC(t *testing.T) {
	receipt, business, settings := authorizedFacturaCFixture()
	out, err := RenderReceiptPDF(receipt, business, settings)
	if err != nil {
		t.Fatalf("RenderReceiptPDF returned error: %v", err)
	}
	if len(out) <= 1000 {
		t.Fatalf("PDF is suspiciously small: %d bytes", len(out))
	}
	if len(out) < 4 || string(out[:4]) != "%PDF" {
		t.Fatalf("output does not start with %%PDF header: %q", out[:min(len(out), 8)])
	}
}

// TestRenderReceiptPDFContainsKeyFields asserts the CAE and receipt number are
// present in the rendered PDF. fpdf normally deflate-compresses its content
// streams, which would hide raw substrings; renderReceiptPDFWithCompression with
// compress=false produces an uncompressed PDF so the text is searchable. The
// public RenderReceiptPDF stays compressed; this test exercises the same render
// path with compression disabled purely for assertion.
func TestRenderReceiptPDFContainsKeyFields(t *testing.T) {
	receipt, business, settings := authorizedFacturaCFixture()
	out, err := renderReceiptPDFWithCompression(receipt, business, settings, false)
	if err != nil {
		t.Fatalf("renderReceiptPDFWithCompression returned error: %v", err)
	}
	body := string(out)
	for _, want := range []string{
		"75123456789012", // CAE
		"00000043",       // receipt number
		"0003",           // zero-padded point of sale
		"30712345678",    // emitter CUIT
		"FACTURA C",      // receipt type + letter
		"CAE",            // CAE label
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered PDF missing expected text %q", want)
		}
	}
}

func TestRenderReceiptPDFConsumidorFinal(t *testing.T) {
	receipt, business, settings := authorizedFacturaCFixture()
	receipt.CustomerDocType = nil
	receipt.CustomerDocNumber = nil
	out, err := renderReceiptPDFWithCompression(receipt, business, settings, false)
	if err != nil {
		t.Fatalf("RenderReceiptPDF returned error: %v", err)
	}
	if !strings.Contains(string(out), "Consumidor Final") {
		t.Error("expected 'Consumidor Final' when no customer document present")
	}
}

func TestRenderReceiptPDFCreditNote(t *testing.T) {
	receipt, business, settings := authorizedFacturaCFixture()
	receipt.ReceiptType = "nota_de_credito_b"
	out, err := renderReceiptPDFWithCompression(receipt, business, settings, false)
	if err != nil {
		t.Fatalf("RenderReceiptPDF returned error: %v", err)
	}
	body := string(out)
	// cp1252-encoded "NOTA DE CRÉDITO B": É is 0xC9 in cp1252.
	if !strings.Contains(body, "NOTA DE CR") || !strings.Contains(body, " B") {
		t.Errorf("expected credit-note B title in PDF body")
	}
}
