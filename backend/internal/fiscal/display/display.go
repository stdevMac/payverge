// Package display holds pure, dependency-free helpers for rendering Argentine
// fiscal documents (AFIP/ARCA): receipt titles, es-AR money formatting, the
// inclusive-IVA split, and RG 5614 legend applicability. It exists as a leaf
// package so internal/services/print can share this logic with internal/fiscal
// without an import cycle (fiscal → services/operational_alerts → services/print).
package display

import (
	"fmt"
	"math"
	"strings"
)

// DefaultArgentinaVATRate is the standard IVA rate used in Argentina (21%).
const DefaultArgentinaVATRate = 21.0

// SplitInclusiveVAT splits a VAT-inclusive total (in cents) into net and VAT
// cents using the given rate percentage (e.g. 21.0 for 21%).
//
// The invariant net+vat == totalCents always holds because vat is computed as
// the remainder: vat = totalCents - net. This matches AFIP's requirement that
// the declared amounts reconcile exactly to the receipt total.
//
// For rate <= 0 (factura C / exento) the full amount is returned as net with
// zero VAT.
func SplitInclusiveVAT(totalCents int64, ratePct float64) (netCents, vatCents int64) {
	if ratePct <= 0 {
		return totalCents, 0
	}
	denom := 100.0 + ratePct
	netF := float64(totalCents) * 100.0 / denom
	netCents = int64(math.Round(netF))
	vatCents = totalCents - netCents
	return netCents, vatCents
}

// ReceiptTitle maps the stored receipt_type string to the Spanish AFIP document
// title including its letter (A/B/C). Handles both the "nota_de_credito_*" and
// "nota_credito_*" stored variants. Unknown types are surfaced uppercased so
// they are never silently dropped from a legal document.
func ReceiptTitle(receiptType string) string {
	rt := strings.ToLower(strings.TrimSpace(receiptType))
	switch rt {
	case "factura_a":
		return "FACTURA A"
	case "factura_b":
		return "FACTURA B"
	case "factura_c":
		return "FACTURA C"
	case "nota_de_credito_a", "nota_credito_a":
		return "NOTA DE CRÉDITO A"
	case "nota_de_credito_b", "nota_credito_b":
		return "NOTA DE CRÉDITO B"
	case "nota_de_credito_c", "nota_credito_c":
		return "NOTA DE CRÉDITO C"
	case "nota_de_credito", "nota_credito":
		return "NOTA DE CRÉDITO"
	// Non-AFIP / US-style document types — never surface as Factura A/B/C.
	case "invoice":
		return "INVOICE"
	case "receipt":
		return "RECEIPT"
	case "standard_invoice":
		return "STANDARD INVOICE"
	default:
		return strings.ToUpper(strings.TrimSpace(receiptType))
	}
}

// HumanTaxCondition maps stored tax-condition slugs to printable Spanish labels.
func HumanTaxCondition(slug string) string {
	switch strings.ToLower(strings.TrimSpace(slug)) {
	case "responsable_inscripto":
		return "IVA Responsable Inscripto"
	case "monotributo":
		return "Responsable Monotributo"
	case "exento":
		return "IVA Sujeto Exento"
	case "consumidor_final":
		return "Consumidor Final"
	default:
		return slug
	}
}

// VATRateForReceiptType returns the IVA rate that applies to a receipt type.
// factura_c / nota de crédito C are exento (0); all others use the standard 21%.
func VATRateForReceiptType(receiptType string) float64 {
	rt := strings.ToLower(strings.TrimSpace(receiptType))
	if rt == "factura_c" || rt == "nota_de_credito_c" || rt == "nota_credito_c" {
		return 0
	}
	return DefaultArgentinaVATRate
}

// IsTransparencyLegendType reports whether the RG 5614/2024 (Ley 27.743)
// consumer tax-transparency legend applies to this receipt type. The regime
// covers B-type documents issued to final consumers by Responsables Inscriptos
// (factura_c carries no discriminated IVA; factura_a discriminates it already).
func IsTransparencyLegendType(receiptType string) bool {
	rt := strings.ToLower(strings.TrimSpace(receiptType))
	return rt == "factura_b" || rt == "nota_de_credito_b" || rt == "nota_credito_b"
}

// FormatARS formats int64 cents as Argentine pesos for DISPLAY only:
// "ARS 1.210,00" (period thousands separator, comma decimals — es-AR
// convention). It never mutates the stored cents value.
func FormatARS(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	pesos := cents / 100
	frac := cents % 100
	intPart := groupThousands(pesos)
	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("ARS %s%s,%02d", sign, intPart, frac)
}

// groupThousands inserts "." every three digits from the right (es-AR grouping).
func groupThousands(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
		if len(s) > rem {
			b.WriteByte('.')
		}
	}
	for i := rem; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte('.')
		}
	}
	return b.String()
}
