package ar

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

// CondicionIVAReceptorId values required by AFIP RG 5616 on FECAESolicitar
// (mandatory since 2025). Catalog also includes 7/8/9/10/13/15/16; we only
// emit the four conditions our issuer/receiver model can produce. IDs match
// WSFE FEParamGetCondicionIvaReceptor.
const (
	CondicionIVAReceptorRI              = 1 // IVA Responsable Inscripto
	CondicionIVAReceptorExento          = 4 // IVA Sujeto Exento
	CondicionIVAReceptorConsumidorFinal = 5 // Consumidor Final
	CondicionIVAReceptorMonotributo     = 6 // Responsable Monotributo
)

type WSFEPayload struct {
	CUIT            int64
	PointOfSale     int
	ReceiptTypeCode int
	ConceptCode     int
	DocTypeCode     int
	DocNumber       int64
	IssueDate       time.Time
	TotalCents      int64
	NetCents        int64
	VATCents        int64
	// OpExCents is the exempt amount (ImpOpEx): delivery fee and driver tip that
	// sit in the bill total outside net + IVA. Zero for the inclusive-split path
	// and for factura C. ImpTotal = ImpNeto + ImpIVA + ImpOpEx.
	OpExCents        int64
	TipCentsIncluded int64
	CurrencyCode     string
	CurrencyRate     float64
	// IVAAlicuotaID is the AFIP alícuota id matching the VAT rate (e.g. 5 = 21%).
	// Derived from the rate by vatAlicuotaID so the emitted <Id> always matches the
	// declared net/VAT split (F-ALICUOTA). Zero means no IVA line (factura_c / 0%).
	IVAAlicuotaID int
	// CondicionIVAReceptorId is the RG 5616 receptor IVA condition id emitted as
	// <ar:CondicionIVAReceptorId> on every FECAEDetRequest (issue + credit note).
	CondicionIVAReceptorId int
	// Associated-voucher fields for credit notes (CbtesAsoc in FECAEDetRequest).
	// When AssocNumber > 0 the WSFE client emits the CbtesAsoc block referencing
	// the original authorized receipt. Zero value means no associated voucher.
	AssocTypeCode    int   // Tipo — the WSFE receipt-type code of the original receipt
	AssocPointOfSale int   // PtoVta — point of sale of the original receipt
	AssocNumber      int64 // Nro — receipt number of the original receipt
}

// CondicionIVAReceptorID maps a stored tax-condition slug to the AFIP
// CondicionIVAReceptorId integer. Empty / unknown / consumidor_final → 5.
// Pure function — table-tested; safe to call from mappers and handlers.
func CondicionIVAReceptorID(taxCondition string) int {
	switch strings.ToLower(strings.TrimSpace(taxCondition)) {
	case "responsable_inscripto":
		return CondicionIVAReceptorRI
	case "exento":
		return CondicionIVAReceptorExento
	case "monotributo":
		return CondicionIVAReceptorMonotributo
	case "consumidor_final", "":
		return CondicionIVAReceptorConsumidorFinal
	default:
		return CondicionIVAReceptorConsumidorFinal
	}
}

// CFIDThresholdCents returns the env-tunable Factura B CF identification
// threshold (cents). Delegates to the canonical fiscal-package helper.
func CFIDThresholdCents() int64 {
	return fiscal.CFIDThresholdCents()
}

// ValidateCUITMod11 checks an 11-digit CUIT/CUIL with AFIP mod-11 weights
// 5,4,3,2,7,6,5,4,3,2. Non-digit characters are stripped first.
func ValidateCUITMod11(raw string) bool {
	digits := digitString(raw)
	if len(digits) != 11 {
		return false
	}
	weights := []int{5, 4, 3, 2, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 10; i++ {
		sum += int(digits[i]-'0') * weights[i]
	}
	mod := sum % 11
	var check int
	switch 11 - mod {
	case 11:
		check = 0
	case 10:
		// Residue class "check digit 10" is intentionally INVALID: AFIP never
		// issues such a CUIT/CUIL — generation swaps the 20/27 prefix to 23,
		// which changes the weighted sum so the standard formula holds again
		// (dv 9 for ex-20, 4 for ex-27). Do NOT re-map 10→1/9 here; lenient
		// validators that do accept numbers AFIP cannot have issued.
		return false
	default:
		check = 11 - mod
	}
	return int(digits[10]-'0') == check
}

// KnownCustomerTaxConditions is the set accepted on issue overrides / resolve-type.
var KnownCustomerTaxConditions = map[string]struct{}{
	"responsable_inscripto": {},
	"monotributo":           {},
	"exento":                {},
	"consumidor_final":      {},
}

// validateLetterReceptorConsistency rejects letter × receptor combinations that
// AFIP would permanently reject (e.g. factura_a with consumidor final).
func validateLetterReceptorConsistency(receiptType string, condicionID int) error {
	rt := strings.ToLower(strings.TrimSpace(receiptType))
	switch rt {
	case "factura_a":
		if condicionID != CondicionIVAReceptorRI {
			return fmt.Errorf("factura_a requires receptor condición responsable_inscripto (id 1), got %d: %w", condicionID, fiscal.ErrPermanent)
		}
	}
	return nil
}

// validateFacturaBCFIdentification enforces the Factura B unidentified-CF
// amount threshold: DocTipo 99 / DocNro 0 above the threshold requires DNI/CUIT.
func validateFacturaBCFIdentification(receiptType string, docTypeCode int, docNumber, totalCents int64) error {
	rt := strings.ToLower(strings.TrimSpace(receiptType))
	if rt != "factura_b" {
		return nil
	}
	if docTypeCode != 99 || docNumber != 0 {
		return nil
	}
	threshold := CFIDThresholdCents()
	if totalCents >= threshold {
		return fmt.Errorf(
			"factura_b over threshold (%d cents) requires DNI or CUIT for consumidor final (unidentified DocTipo 99): %w",
			threshold, fiscal.ErrPermanent,
		)
	}
	return nil
}

func MapIssueInputToWSFE(input fiscal.IssueInput) (*WSFEPayload, error) {
	cuit, err := parseFixedDigits(input.Settings.TaxID, 11)
	if err != nil {
		return nil, fmt.Errorf("invalid CUIT: %w", fiscal.ErrPermanent)
	}
	if input.Settings.PointOfSale == nil || *input.Settings.PointOfSale <= 0 {
		return nil, fmt.Errorf("point of sale is required: %w", fiscal.ErrPermanent)
	}
	rtCode, err := receiptTypeCode(input.ReceiptType)
	if err != nil {
		return nil, err
	}
	docType, docNumber, err := recipientDocument(input.CustomerDocType, input.CustomerDocNumber)
	if err != nil {
		return nil, err
	}
	// factura_a legally requires an IVA-registered recipient (CUIT or CUIL,
	// both resolve to DocTypeCode 80). Reject any other doc type up-front so
	// the caller gets a clear error before making any network call.
	if strings.EqualFold(strings.TrimSpace(input.ReceiptType), "factura_a") && docType != 80 {
		return nil, fmt.Errorf("factura_a requires a CUIT/CUIL recipient (got doc type %d): %w", docType, fiscal.ErrPermanent)
	}
	currency, err := currencyCode(input.Currency)
	if err != nil {
		return nil, err
	}
	total := input.TotalAmountCents
	if total == 0 {
		total = input.Bill.TotalAmount
	}

	// factura_c stays IVA-exento (rate 0, ImpOpEx 0). factura_a/b take the
	// business alícuota when it is one AFIP publishes, and IVA from the bill
	// when tax was itemized. Otherwise the inclusive total is split at 21%.
	rt := strings.ToLower(strings.TrimSpace(input.ReceiptType))
	vatRate := issueVATRate(rt, input.Business.TaxRate)
	var netCents, vatCents, opExCents int64
	if rt == "factura_c" {
		netCents, vatCents = fiscal.SplitInclusiveVAT(total, 0)
	} else {
		netCents, vatCents, opExCents = splitIssueIVA(input.Bill, total, vatRate)
	}

	condicionID := CondicionIVAReceptorID(input.CustomerTaxCondition)
	if err := validateLetterReceptorConsistency(input.ReceiptType, condicionID); err != nil {
		return nil, err
	}
	if err := validateFacturaBCFIdentification(input.ReceiptType, docType, docNumber, total); err != nil {
		return nil, err
	}

	return &WSFEPayload{
		CUIT:                   cuit,
		PointOfSale:            *input.Settings.PointOfSale,
		ReceiptTypeCode:        rtCode,
		ConceptCode:            1,
		DocTypeCode:            docType,
		DocNumber:              docNumber,
		IssueDate:              input.IssuedAt,
		TotalCents:             total,
		NetCents:               netCents,
		VATCents:               vatCents,
		OpExCents:              opExCents,
		TipCentsIncluded:       0,
		CurrencyCode:           currency,
		CurrencyRate:           1,
		IVAAlicuotaID:          vatAlicuotaID(vatRate),
		CondicionIVAReceptorId: condicionID,
	}, nil
}

// issueVATRate is the alícuota used for factura A/B. A business tax rate is
// used only when it is an AFIP alícuota other than 0 (27, 21, 10.5, 5, 2.5);
// anything else, including an unset 0, falls back to the 21% standard rate.
// factura_c is always 0.
func issueVATRate(receiptType string, businessTaxRate float64) float64 {
	if strings.EqualFold(strings.TrimSpace(receiptType), "factura_c") {
		return 0
	}
	switch businessTaxRate {
	case 27, 21, 10.5, 5, 2.5:
		return businessTaxRate
	default:
		return fiscal.DefaultArgentinaVATRate
	}
}

// splitIssueIVA splits a factura A/B total so that ImpIVA = ImpNeto × alícuota,
// which AFIP enforces per AlicIva. Bill tax is charged on the subtotal only
// (service fee, delivery fee and driver tip carry no IVA), so:
//   - ImpNeto is the subtotal and ImpIVA is TaxAmount.
//   - A loyalty discount is taken off that taxed gross (subtotal + tax). The
//     reduced gross is split inclusively at the rate, so net and IVA shrink
//     together. Any discount left over reduces the untaxed remainder. When
//     the discount covers the whole taxed gross, net and IVA are zero and
//     the entire total (service fee, delivery, tip) is ImpOpEx.
//   - Whatever is left in the total (service fee, delivery, tip) is ImpOpEx.
//
// If TaxAmount does not match subtotal × rate within one cent (for example,
// the business rate is not an AFIP alícuota and the 21% fallback is declared),
// or the row is otherwise inconsistent, the taxed gross is re-split
// inclusively at the declared rate. Tax not itemized uses the inclusive split
// of the whole total.
func splitIssueIVA(bill database.Bill, total int64, rate float64) (netCents, vatCents, opExCents int64) {
	if bill.TaxAmount <= 0 || bill.Subtotal <= 0 {
		netCents, vatCents = fiscal.SplitInclusiveVAT(total, rate)
		return netCents, vatCents, 0
	}
	taxedGross := bill.Subtotal + bill.TaxAmount - bill.LoyaltyDiscountCents
	if taxedGross > total {
		taxedGross = total
	}
	if taxedGross <= 0 {
		// Loyalty covered the taxed food and its IVA. The remainder is
		// service fee, delivery, or tip, which carry no IVA.
		return 0, 0, total
	}
	expectedVAT := int64(math.Round(float64(bill.Subtotal) * rate / 100))
	diff := expectedVAT - bill.TaxAmount
	if bill.LoyaltyDiscountCents <= 0 && taxedGross == bill.Subtotal+bill.TaxAmount && diff >= -1 && diff <= 1 {
		netCents, vatCents = bill.Subtotal, bill.TaxAmount
	} else {
		netCents, vatCents = fiscal.SplitInclusiveVAT(taxedGross, rate)
	}
	return netCents, vatCents, total - netCents - vatCents
}

// vatAlicuotaID maps a VAT rate percentage to its AFIP alícuota id so the emitted
// <Iva><AlicIva><Id> always matches the declared net/VAT split. Unknown rates fall
// back to 5 (21%), the pipeline's standard bucket. Keeping the rate→id mapping in
// one place stops the id and the rate from drifting apart (F-ALICUOTA).
func vatAlicuotaID(ratePct float64) int {
	switch ratePct {
	case 27.0:
		return 6
	case 21.0:
		return 5
	case 10.5:
		return 4
	case 5.0:
		return 8
	case 2.5:
		return 9
	case 0.0:
		return 3
	default:
		return 5
	}
}

// MapCreditNoteToWSFE maps a CreditNoteInput to a WSFEPayload for the credit
// note type corresponding to the original receipt (factura_a→3, factura_b→8,
// factura_c→13). It sets AssocTypeCode/AssocPointOfSale/AssocNumber so that
// RequestCAE emits the required CbtesAsoc block referencing the original receipt.
func MapCreditNoteToWSFE(input fiscal.CreditNoteInput) (*WSFEPayload, error) {
	cuit, err := parseFixedDigits(input.Settings.TaxID, 11)
	if err != nil {
		return nil, fmt.Errorf("invalid CUIT: %w", fiscal.ErrPermanent)
	}
	if input.Settings.PointOfSale == nil || *input.Settings.PointOfSale <= 0 {
		return nil, fmt.Errorf("point of sale is required: %w", fiscal.ErrPermanent)
	}

	// Credit note receipt type is derived from the original receipt type.
	cnTypeCode, err := creditNoteTypeFor(input.OriginalReceipt.ReceiptType)
	if err != nil {
		return nil, err
	}

	// Original receipt type code (for CbtesAsoc Tipo field).
	origTypeCode, err := receiptTypeCode(input.OriginalReceipt.ReceiptType)
	if err != nil {
		return nil, err
	}

	// Original receipt number is required to build the CbtesAsoc reference.
	if input.OriginalReceipt.ReceiptNumber == nil || strings.TrimSpace(*input.OriginalReceipt.ReceiptNumber) == "" {
		return nil, fmt.Errorf("original receipt number is required for credit note: %w", fiscal.ErrPermanent)
	}
	origNumber, err := strconv.ParseInt(strings.TrimSpace(*input.OriginalReceipt.ReceiptNumber), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid original receipt number %q: %v: %w", *input.OriginalReceipt.ReceiptNumber, err, fiscal.ErrPermanent)
	}
	if origNumber <= 0 {
		return nil, fmt.Errorf("original receipt number must be positive, got %d: %w", origNumber, fiscal.ErrPermanent)
	}

	// The alícuota mirrors the original factura: the business rate for A/B
	// (21% fallback), 0 for factura_c.
	origRT := strings.ToLower(strings.TrimSpace(input.OriginalReceipt.ReceiptType))
	vatRate := issueVATRate(origRT, input.BusinessTaxRate)

	// Recipient doc: a nota de crédito A (factura_a original) MUST identify the
	// recipient by CUIT (DocTipo 80), mirroring the original factura A — AFIP
	// rejects an NC-A with consumidor final (99/0) permanently. factura_b/_c
	// credit notes use consumidor final. Re-derive from the recipient doc that
	// the issue path persisted on the original receipt.
	docTypeCode := 99
	var docNumber int64 = 0
	if origRT == "factura_a" {
		dt := strings.TrimSpace(derefString(input.OriginalReceipt.CustomerDocType))
		dn := strings.TrimSpace(derefString(input.OriginalReceipt.CustomerDocNumber))
		code, num, derr := recipientDocument(dt, dn)
		if derr != nil {
			return nil, fmt.Errorf("nota de crédito A requires the original CUIT recipient: %w", derr)
		}
		if code != 80 {
			return nil, fmt.Errorf("nota de crédito A requires a CUIT/CUIL recipient (got doc type %d): %w", code, fiscal.ErrPermanent)
		}
		docTypeCode, docNumber = code, num
	}

	// Credit amount: a positive AmountCents credits exactly that (a partial
	// refund); zero falls back to the full original receipt total. The VAT is then
	// split out of the credited amount (factura A/B), so a partial nota de crédito
	// carries a proportional net/VAT breakdown.
	total := input.AmountCents
	if total <= 0 {
		total = input.OriginalReceipt.TotalAmountCents
	}
	netCents, vatCents, opExCents := splitCreditNoteIVA(origRT, input.Bill, input.OriginalReceipt.TotalAmountCents, total, vatRate)

	currency, err := currencyCode(input.OriginalReceipt.Currency)
	if err != nil {
		// Default to ARS when original receipt has no/empty currency.
		currency = "PES"
	}

	// RG 5616: credit notes must also send CondicionIVAReceptorId — reuse the
	// condition snapshotted on the original receipt (or CF when absent).
	condicionID := CondicionIVAReceptorID(derefString(input.OriginalReceipt.CustomerTaxCondition))
	// NC-A inherits factura_a consistency (must be RI).
	if origRT == "factura_a" {
		if err := validateLetterReceptorConsistency("factura_a", condicionID); err != nil {
			return nil, err
		}
	}

	return &WSFEPayload{
		CUIT:                   cuit,
		PointOfSale:            *input.Settings.PointOfSale,
		ReceiptTypeCode:        cnTypeCode,
		ConceptCode:            1,
		DocTypeCode:            docTypeCode,
		DocNumber:              docNumber,
		IssueDate:              input.IssuedAt,
		TotalCents:             total,
		NetCents:               netCents,
		VATCents:               vatCents,
		OpExCents:              opExCents,
		TipCentsIncluded:       0,
		CurrencyCode:           currency,
		CurrencyRate:           1,
		IVAAlicuotaID:          vatAlicuotaID(vatRate),
		CondicionIVAReceptorId: condicionID,
		AssocTypeCode:          origTypeCode,
		AssocPointOfSale:       *input.Settings.PointOfSale,
		AssocNumber:            origNumber,
	}, nil
}

// splitCreditNoteIVA mirrors the original factura's net / IVA / ImpOpEx split
// (splitIssueIVA on the same bill, rate, and original total) onto the credited
// amount. A full credit reuses that split exactly. A partial credit scales
// ImpOpEx by credited/original (rounded half up) and splits the rest
// inclusively at the alícuota, so ImpIVA stays ImpNeto × rate and
// ImpNeto + ImpIVA + ImpOpEx always equals the credited total.
// factura_c credits are IVA-exento: the whole amount is the net.
func splitCreditNoteIVA(origReceiptType string, bill database.Bill, originalTotal, credited int64, rate float64) (netCents, vatCents, opExCents int64) {
	if strings.EqualFold(strings.TrimSpace(origReceiptType), "factura_c") {
		netCents, vatCents = fiscal.SplitInclusiveVAT(credited, 0)
		return netCents, vatCents, 0
	}
	if originalTotal <= 0 {
		netCents, vatCents = fiscal.SplitInclusiveVAT(credited, rate)
		return netCents, vatCents, 0
	}
	oNet, oVAT, oOpEx := splitIssueIVA(bill, originalTotal, rate)
	if credited == originalTotal {
		return oNet, oVAT, oOpEx
	}
	// Scale the untaxed part, then split the rest inclusively so the credited
	// IVA stays net × alícuota, as AFIP checks.
	opExCents = (oOpEx*credited + originalTotal/2) / originalTotal
	if opExCents > credited {
		opExCents = credited
	}
	netCents, vatCents = fiscal.SplitInclusiveVAT(credited-opExCents, rate)
	return netCents, vatCents, opExCents
}

// creditNoteTypeFor returns the AFIP receipt type code for the nota de crédito
// corresponding to the given original receipt type:
//
//	factura_a → nota de crédito A (code 3)
//	factura_b → nota de crédito B (code 8)
//	factura_c → nota de crédito C (code 13)
func creditNoteTypeFor(originalReceiptType string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(originalReceiptType)) {
	case "factura_a":
		return 3, nil
	case "factura_b":
		return 8, nil
	case "factura_c":
		return 13, nil
	default:
		return 0, fmt.Errorf("cannot derive credit note type for receipt type %q: %w", originalReceiptType, fiscal.ErrPermanent)
	}
}

func receiptTypeCode(receiptType string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(receiptType)) {
	case "factura_a":
		return 1, nil
	case "factura_b":
		return 6, nil
	case "factura_c":
		return 11, nil
	default:
		return 0, fmt.Errorf("unsupported receipt type %q: %w", receiptType, fiscal.ErrPermanent)
	}
}

func recipientDocument(docType, docNumber string) (int, int64, error) {
	switch strings.ToUpper(strings.TrimSpace(docType)) {
	case "DNI":
		n, err := parseDigitsInRange(docNumber, 7, 8)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid DNI: %w", fiscal.ErrPermanent)
		}
		return 96, n, nil
	case "CUIT", "CUIL":
		if !ValidateCUITMod11(docNumber) {
			return 0, 0, fmt.Errorf("invalid CUIT/CUIL checksum: %w", fiscal.ErrPermanent)
		}
		n, err := parseFixedDigits(docNumber, 11)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid CUIT/CUIL: %w", fiscal.ErrPermanent)
		}
		return 80, n, nil
	default:
		return 99, 0, nil
	}
}

func currencyCode(currency string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "", "ARS", "PES":
		return "PES", nil
	default:
		return "", fmt.Errorf("unsupported currency %q: %w", currency, fiscal.ErrPermanent)
	}
}

func parseFixedDigits(raw string, length int) (int64, error) {
	digits := digitString(raw)
	if len(digits) != length {
		return 0, fmt.Errorf("expected %d digits", length)
	}
	return strconv.ParseInt(digits, 10, 64)
}

func parseDigitsInRange(raw string, minLength, maxLength int) (int64, error) {
	digits := digitString(raw)
	if len(digits) < minLength || len(digits) > maxLength {
		return 0, fmt.Errorf("expected %d-%d digits", minLength, maxLength)
	}
	return strconv.ParseInt(digits, 10, 64)
}

func digitString(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
