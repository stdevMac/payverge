package fiscal

import "strings"

// ResolveIssuableReceiptType is the SINGLE canonical mapping from emitter +
// customer tax identity to the AFIP receipt type the pipeline will actually
// issue for Argentina. The worker (resolveReceiptType) delegates here so
// issuance stays on this mapping (D-2). Non-AR countries MUST go through
// ResolveIssuableReceiptTypeForCountry.
//
// Rules:
//   - A monotributo / exento emitter always issues factura_c.
//   - Otherwise (responsable inscripto emitter): a responsable_inscripto customer
//     identified by a CUIT/CUIL with a non-empty number → factura_a. This gate
//     mirrors the WSFE mapper, which rejects factura_a without a CUIT/CUIL (D-1):
//     a responsable_inscripto identified only by a DNI (or with no document) must
//     NOT become factura_a — it falls through to factura_b, a valid issuable
//     invoice.
//   - Everything else → factura_b.
func ResolveIssuableReceiptType(emitterCondition, customerCondition, customerDocType, customerDocNumber string) string {
	emitter := strings.ToLower(strings.TrimSpace(emitterCondition))
	if emitter == "monotributo" || emitter == "exento" {
		return "factura_c"
	}
	custCondition := strings.ToLower(strings.TrimSpace(customerCondition))
	custDocType := strings.ToUpper(strings.TrimSpace(customerDocType))
	if custCondition == "responsable_inscripto" &&
		(custDocType == "CUIT" || custDocType == "CUIL") &&
		strings.TrimSpace(customerDocNumber) != "" {
		return "factura_a"
	}
	return "factura_b"
}

// ResolveIssuableReceiptTypeForCountry picks the receipt type that matches the
// venue's fiscal country. Argentine businesses keep the AFIP A/B/C letter scheme;
// UAE readiness uses standard_invoice; US and other non-AFIP locales issue
// invoice (identified buyer) or receipt (consumer / no document).
func ResolveIssuableReceiptTypeForCountry(country, emitterCondition, customerCondition, customerDocType, customerDocNumber string) string {
	switch strings.ToUpper(strings.TrimSpace(country)) {
	case "AR":
		return ResolveIssuableReceiptType(emitterCondition, customerCondition, customerDocType, customerDocNumber)
	case "AE":
		return "standard_invoice"
	default:
		if strings.TrimSpace(customerDocNumber) != "" {
			return "invoice"
		}
		return "receipt"
	}
}

// ArgentinaLetterFromReceiptType returns the AFIP letter chip (A/B/C) for a
// factura_* / nota_* type, or "" when the type is not an Argentine letter doc.
func ArgentinaLetterFromReceiptType(receiptType string) string {
	switch strings.ToLower(strings.TrimSpace(receiptType)) {
	case "factura_a", "nota_de_credito_a", "nota_credito_a":
		return "A"
	case "factura_b", "nota_de_credito_b", "nota_credito_b":
		return "B"
	case "factura_c", "nota_de_credito_c", "nota_credito_c":
		return "C"
	default:
		return ""
	}
}

// IsAFIPLetterType reports whether storedType is an Argentine Factura/NC letter.
func IsAFIPLetterType(receiptType string) bool {
	return ArgentinaLetterFromReceiptType(receiptType) != ""
}

// NormalizeStoredReceiptTypeForCountry remaps leftover AFIP letter types onto
// the catalogue for the receipt's fiscal country. US venues that still store
// factura_a/b/c (pre-#255 demo rows) must surface as invoice/receipt — never
// as Factura A/B/C. Argentine rows are left unchanged.
func NormalizeStoredReceiptTypeForCountry(country, receiptType string) string {
	rt := strings.TrimSpace(receiptType)
	if rt == "" || !IsAFIPLetterType(rt) {
		return rt
	}
	switch strings.ToUpper(strings.TrimSpace(country)) {
	case "AR":
		return rt
	case "AE":
		return "standard_invoice"
	default:
		if ArgentinaLetterFromReceiptType(rt) == "A" {
			return "invoice"
		}
		return "receipt"
	}
}

// ReceiptTypeFilterValues expands a UI catalogue key so leftover AFIP letters
// on US/AE venues still match Invoice/Receipt/Standard invoice filters until
// (and after) the data migration rewrites stored rows.
func ReceiptTypeFilterValues(requested string) []string {
	rt := strings.ToLower(strings.TrimSpace(requested))
	if rt == "" {
		return nil
	}
	switch rt {
	case "invoice":
		return []string{"invoice", "factura_a", "nota_de_credito_a", "nota_credito_a"}
	case "receipt":
		return []string{
			"receipt",
			"factura_b", "factura_c",
			"nota_de_credito_b", "nota_de_credito_c",
			"nota_credito_b", "nota_credito_c",
		}
	case "standard_invoice":
		return []string{"standard_invoice", "factura_a", "factura_b", "factura_c"}
	default:
		return []string{requested}
	}
}
