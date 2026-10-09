package fiscal

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

// knownReceptorConditions are the tax-condition slugs accepted on issue overrides.
var knownReceptorConditions = map[string]struct{}{
	"responsable_inscripto": {},
	"monotributo":           {},
	"exento":                {},
	"consumidor_final":      {},
}

// ValidateReceiverOverride checks doc type/number, tax condition, and name shape.
// Empty override fields are allowed (caller may send a partial override).
func ValidateReceiverOverride(o *ReceiverOverride) error {
	if o == nil {
		return nil
	}
	docType := strings.ToUpper(strings.TrimSpace(o.CustomerDocType))
	docNum := strings.TrimSpace(o.CustomerDocNumber)
	cond := strings.ToLower(strings.TrimSpace(o.CustomerTaxCondition))

	if cond != "" {
		if _, ok := knownReceptorConditions[cond]; !ok {
			return fmt.Errorf("%w: unknown tax condition %q", ErrInvalidReceiver, o.CustomerTaxCondition)
		}
	}

	switch docType {
	case "":
		// Unidentified consumidor final — number must be empty or ignored.
	case "DNI":
		digits := digitsOnly(docNum)
		if len(digits) < 7 || len(digits) > 8 {
			return fmt.Errorf("%w: DNI must be 7-8 digits", ErrInvalidReceiver)
		}
	case "CUIT", "CUIL":
		if !validateCUITMod11(docNum) {
			return fmt.Errorf("%w: invalid CUIT/CUIL checksum", ErrInvalidReceiver)
		}
	case "CF", "CONSUMIDOR_FINAL":
		// Treat as unidentified CF.
	default:
		return fmt.Errorf("%w: unsupported doc type %q", ErrInvalidReceiver, o.CustomerDocType)
	}
	return nil
}

// applyReceiverOverride validates and writes non-empty override fields onto the
// bill so the worker's bill-based snapshot path picks them up.
func applyReceiverOverride(db *gorm.DB, bill *database.Bill, o *ReceiverOverride) error {
	if err := ValidateReceiverOverride(o); err != nil {
		return err
	}
	updates := map[string]interface{}{}

	docType := strings.ToUpper(strings.TrimSpace(o.CustomerDocType))
	switch docType {
	case "CF", "CONSUMIDOR_FINAL":
		docType = ""
	}
	if o.CustomerDocType != "" || o.CustomerDocNumber != "" {
		if docType == "" {
			updates["fiscal_customer_doc_type"] = nil
			updates["fiscal_customer_doc_number"] = nil
			bill.FiscalCustomerDocType = nil
			bill.FiscalCustomerDocNumber = nil
		} else {
			dt := docType
			dn := digitsOnly(strings.TrimSpace(o.CustomerDocNumber))
			updates["fiscal_customer_doc_type"] = dt
			updates["fiscal_customer_doc_number"] = dn
			bill.FiscalCustomerDocType = &dt
			bill.FiscalCustomerDocNumber = &dn
		}
	}
	if strings.TrimSpace(o.CustomerTaxCondition) != "" {
		cond := strings.ToLower(strings.TrimSpace(o.CustomerTaxCondition))
		updates["fiscal_customer_tax_condition"] = cond
		bill.FiscalCustomerTaxCondition = &cond
	}
	if strings.TrimSpace(o.CustomerName) != "" {
		name := strings.TrimSpace(o.CustomerName)
		updates["fiscal_customer_name"] = name
		bill.FiscalCustomerName = &name
	}
	if len(updates) == 0 {
		return nil
	}
	// An operator override takes ownership of the receptor: clear the guest
	// session that set it so issuance keeps the operator's
	// identity and guest tokens can no longer replace it.
	updates["fiscal_customer_guest_session"] = nil
	bill.FiscalCustomerGuestSession = nil
	return db.Model(bill).Updates(updates).Error
}

func digitsOnly(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ValidateCUITMod11 is the shared CUIT/CUIL checksum used by checkout handlers
// and the issue-override path. It mirrors the AR provider check so a bad number
// is rejected before enqueueing a permanently-failing job.
func ValidateCUITMod11(raw string) bool {
	return validateCUITMod11(raw)
}

// validateCUITMod11 mirrors the AR provider check so handlers can reject bad
// overrides before enqueueing a permanently-failing job.
func validateCUITMod11(raw string) bool {
	digits := digitsOnly(raw)
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
		// Intentionally invalid — AFIP never issues this residue class (the
		// 20/27→23 prefix swap keeps every real CUIT/CUIL on the standard
		// formula). See ar.ValidateCUITMod11 for the full rationale.
		return false
	default:
		check = 11 - mod
	}
	return int(digits[10]-'0') == check
}
