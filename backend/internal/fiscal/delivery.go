package fiscal

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
	"github.com/skip2/go-qrcode"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal/display"
)

// RenderAFIPQRPNG encodes the AFIP QR payload URL (receipt.QRPayload) to a PNG.
// The payload is the canonical AFIP "comprobante" verification URL
// (https://www.afip.gob.ar/fe/qr/?p=...). Returns an error for an empty payload.
func RenderAFIPQRPNG(qrPayload string) ([]byte, error) {
	if strings.TrimSpace(qrPayload) == "" {
		return nil, fmt.Errorf("fiscal: empty QR payload")
	}
	// qrcode.Encode returns PNG bytes directly. Medium recovery + 512px matches
	// the existing repo convention (tools_handlers.go) and stays scannable when
	// embedded into the receipt PDF.
	png, err := qrcode.Encode(qrPayload, qrcode.Medium, 512)
	if err != nil {
		return nil, fmt.Errorf("fiscal: encode AFIP QR: %w", err)
	}
	return png, nil
}

// RenderReceiptPDF renders an AFIP fiscal receipt (factura A/B/C or nota de
// crédito) to a self-contained PDF byte slice. business supplies the emitter
// display name; settings supplies the emitter identity (CUIT/tax condition/point
// of sale); receipt supplies all fiscal fields (CAE, number, type, amounts in
// cents, QR payload).
//
// Money on the receipt is int64 CENTS; it is divided by 100 only for display and
// the stored values are never mutated. Neto/IVA are derived from the
// VAT-inclusive total using the same SplitInclusiveVAT logic the WSFE mapper
// uses (factura_c / nota de crédito C are IVA-exento → full amount as Neto).
func RenderReceiptPDF(receipt database.FiscalReceipt, business database.Business, settings database.BusinessFiscalSettings) ([]byte, error) {
	return renderReceiptPDFWithCompression(receipt, business, settings, true)
}

// renderReceiptPDFWithCompression is the shared render path. compress=true (the
// public default) yields a smaller deflate-compressed PDF; tests pass
// compress=false so the content stream stays as plain text and field assertions
// (CAE, receipt number, CUIT, title) can search the raw bytes.
func renderReceiptPDFWithCompression(receipt database.FiscalReceipt, business database.Business, settings database.BusinessFiscalSettings, compress bool) ([]byte, error) {
	qrPNG, err := RenderAFIPQRPNG(deref(receipt.QRPayload))
	if err != nil {
		return nil, err
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(compress)
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()
	// cp1252 translator so Spanish accents (É, í, ó, ñ, á) render in the core
	// Helvetica font instead of mojibake. Empty descriptor defaults to cp1252.
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	const contentWidth = 180.0 // A4 width (210mm) minus 15mm margins each side.

	// --- Header: emitter name + receipt title ---
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(contentWidth, 9, tr(emitterName(business)), "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "B", 14)
	pdf.CellFormat(contentWidth, 8, tr(receiptTitle(receipt.ReceiptType)), "", 1, "L", false, 0, "")
	pdf.Ln(2)

	// --- Emitter identity block ---
	pdf.SetFont("Helvetica", "", 10)
	writeLabeled(pdf, tr, contentWidth, "CUIT", emitterCUIT(settings))
	if tc := strings.TrimSpace(settings.TaxCondition); tc != "" {
		writeLabeled(pdf, tr, contentWidth, "Condición frente al IVA", humanTaxCondition(tc))
	}
	writeLabeled(pdf, tr, contentWidth, "Comprobante N°", receiptNumberDisplay(receipt, settings))
	writeLabeled(pdf, tr, contentWidth, "Fecha de emisión", issueDateDisplay(receipt))
	pdf.Ln(2)

	// --- Customer block ---
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(contentWidth, 7, tr("Receptor"), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	writeCustomerBlock(pdf, tr, contentWidth, receipt)
	pdf.Ln(2)

	// --- Amounts block ---
	net, vat, total := receiptAmounts(receipt)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(contentWidth, 7, tr("Importes"), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	writeMoneyRow(pdf, tr, contentWidth, "Neto", net)
	writeMoneyRow(pdf, tr, contentWidth, "IVA", vat)
	pdf.SetFont("Helvetica", "B", 11)
	writeMoneyRow(pdf, tr, contentWidth, "Total", total)
	if display.IsTransparencyLegendType(receipt.ReceiptType) {
		pdf.Ln(2)
		writeTransparencyLegend(pdf, tr, contentWidth, vat)
	}
	pdf.Ln(3)

	// --- CAE block ---
	pdf.SetFont("Helvetica", "", 10)
	writeLabeled(pdf, tr, contentWidth, "CAE", deref(receipt.AuthCode))
	writeLabeled(pdf, tr, contentWidth, "Vto. CAE", caeExpiryDisplay(receipt))
	pdf.Ln(4)

	// --- Embedded AFIP QR ---
	const qrName = "afip_qr"
	pdf.RegisterImageReader(qrName, "PNG", bytes.NewReader(qrPNG))
	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("fiscal: register QR image: %w", err)
	}
	const qrSize = 40.0
	pdf.Image(qrName, 15, pdf.GetY(), qrSize, qrSize, false, "PNG", 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("fiscal: render receipt PDF: %w", err)
	}
	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("fiscal: render receipt PDF: %w", err)
	}
	return buf.Bytes(), nil
}

// emitterName returns the business display name, falling back to a placeholder.
func emitterName(business database.Business) string {
	if name := strings.TrimSpace(business.Name); name != "" {
		return name
	}
	return "Emisor"
}

// emitterCUIT returns the emitter's CUIT from the fiscal settings TaxID.
func emitterCUIT(settings database.BusinessFiscalSettings) string {
	return strings.TrimSpace(settings.TaxID)
}

// ReceiptTitle is the exported form of receiptTitle: it maps a stored
// receipt_type to the human-facing AFIP document title (e.g. "FACTURA C"). Used
// by the delivery adapter when labeling the receipt email.
func ReceiptTitle(receiptType string) string { return receiptTitle(receiptType) }

// receiptTitle delegates to the shared display package (also used by the
// thermal-ticket formatter pipeline).
func receiptTitle(receiptType string) string { return display.ReceiptTitle(receiptType) }

// receiptNumberDisplay renders the AFIP "0003-00000043" form when both the point
// of sale and receipt number are available, otherwise the receipt number alone.
func receiptNumberDisplay(receipt database.FiscalReceipt, settings database.BusinessFiscalSettings) string {
	number := strings.TrimSpace(deref(receipt.ReceiptNumber))
	if number == "" {
		return "—"
	}
	if settings.PointOfSale != nil {
		return fmt.Sprintf("%04d-%s", *settings.PointOfSale, number)
	}
	return number
}

// issueDateDisplay formats the receipt issue date (DD/MM/YYYY). Falls back to the
// created timestamp when IssuedAt is nil.
func issueDateDisplay(receipt database.FiscalReceipt) string {
	if receipt.IssuedAt != nil && !receipt.IssuedAt.IsZero() {
		return receipt.IssuedAt.Format("02/01/2006")
	}
	if !receipt.CreatedAt.IsZero() {
		return receipt.CreatedAt.Format("02/01/2006")
	}
	return "—"
}

// caeExpiryDisplay formats the CAE expiry date (DD/MM/YYYY) or a placeholder.
func caeExpiryDisplay(receipt database.FiscalReceipt) string {
	if receipt.AuthExpiresAt != nil && !receipt.AuthExpiresAt.IsZero() {
		return receipt.AuthExpiresAt.Format("02/01/2006")
	}
	return "—"
}

// receiptAmounts derives Neto, IVA and Total (in cents) from the stored
// VAT-inclusive total. factura_c and nota de crédito C are IVA-exento (0% rate →
// full amount is Neto). The stored cents value is never mutated.
func receiptAmounts(receipt database.FiscalReceipt) (netCents, vatCents, totalCents int64) {
	totalCents = receipt.TotalAmountCents
	rate := vatRateForReceiptType(receipt.ReceiptType)
	netCents, vatCents = SplitInclusiveVAT(totalCents, rate)
	return netCents, vatCents, totalCents
}

// vatRateForReceiptType delegates to the shared display package.
func vatRateForReceiptType(receiptType string) float64 {
	return display.VATRateForReceiptType(receiptType)
}

// writeCustomerBlock renders the receptor document/name/condición, or
// "Consumidor Final" when no customer document is present on the receipt.
// Factura A requires name + condición; B prints them when present; C uses the
// same layout. Labels go through tr() (cp1252) so tildes encode correctly.
func writeCustomerBlock(pdf *fpdf.Fpdf, tr func(string) string, width float64, receipt database.FiscalReceipt) {
	docType := strings.TrimSpace(deref(receipt.CustomerDocType))
	docNumber := strings.TrimSpace(deref(receipt.CustomerDocNumber))
	name := strings.TrimSpace(deref(receipt.CustomerName))
	cond := strings.TrimSpace(deref(receipt.CustomerTaxCondition))

	if name != "" {
		writeLabeled(pdf, tr, width, "Razon social", name)
	}

	if docType == "" && docNumber == "" {
		if name == "" {
			pdf.CellFormat(width, 6, tr("Consumidor Final"), "", 1, "L", false, 0, "")
		}
	} else {
		label := strings.ToUpper(docType)
		if label == "" {
			label = "Documento"
		}
		value := docNumber
		if value == "" {
			value = "—"
		}
		writeLabeled(pdf, tr, width, label, value)
	}

	if cond != "" {
		writeLabeled(pdf, tr, width, "Condicion frente al IVA", humanTaxCondition(cond))
	} else if strings.EqualFold(strings.TrimSpace(receipt.ReceiptType), "factura_a") {
		// Factura A always shows a condition line when missing (should be rare).
		writeLabeled(pdf, tr, width, "Condicion frente al IVA", "Responsable Inscripto")
	}
}

// humanTaxCondition delegates to the shared display package.
func humanTaxCondition(slug string) string { return display.HumanTaxCondition(slug) }

// writeLabeled renders a "Label: value" line.
func writeLabeled(pdf *fpdf.Fpdf, tr func(string) string, width float64, label, value string) {
	if strings.TrimSpace(value) == "" {
		value = "—"
	}
	pdf.CellFormat(width, 6, tr(fmt.Sprintf("%s: %s", label, value)), "", 1, "L", false, 0, "")
}

// writeMoneyRow renders a right-aligned "Label" / "ARS amount" two-column row.
func writeMoneyRow(pdf *fpdf.Fpdf, tr func(string) string, width float64, label string, cents int64) {
	const labelWidth = 40.0
	pdf.CellFormat(labelWidth, 6, tr(label), "", 0, "L", false, 0, "")
	pdf.CellFormat(width-labelWidth, 6, tr(formatARS(cents)), "", 1, "R", false, 0, "")
}

// writeTransparencyLegend renders the RG 5614/2024 consumer tax-transparency
// block ("Régimen de Transparencia Fiscal al Consumidor", Ley 27.743) required
// on B-type documents issued to final consumers — mandatory for all
// Responsables Inscriptos since 2025-04-01. vatCents is the IVA contained in
// the (VAT-inclusive) total. Payverge does not itemize other national indirect
// taxes on restaurant bills, so that line legally discloses ARS 0,00.
func writeTransparencyLegend(pdf *fpdf.Fpdf, tr func(string) string, width float64, vatCents int64) {
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(width, 5, tr("Régimen de Transparencia Fiscal al Consumidor (Ley 27.743)"), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	writeMoneyRow(pdf, tr, width, "IVA Contenido", vatCents)
	writeMoneyRow(pdf, tr, width, "Otros Impuestos Nacionales Indirectos", 0)
	pdf.SetFont("Helvetica", "", 10)
}

// formatARS delegates to the shared display package.
func formatARS(cents int64) string { return display.FormatARS(cents) }

// deref returns the dereferenced string or "" for a nil pointer.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
