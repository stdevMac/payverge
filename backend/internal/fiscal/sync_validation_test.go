package fiscal

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

func strPtrT(s string) *string { return &s }

// TestValidateSynchronousIssue_FacturaBOverThreshold: the CF identification
// threshold must reject at ISSUE time (400 to the operator), not only in the
// async WSFE mapper (silent failed_permanent). Regression for the dead
// ErrCFIdentificationRequired handler branch.
func TestValidateSynchronousIssue_FacturaBOverThreshold(t *testing.T) {
	settings := &database.BusinessFiscalSettings{
		Provider:     "arca",
		TaxCondition: "responsable_inscripto",
	}
	over := CFIDThresholdCents()

	// Unidentified consumidor final at/over threshold → ErrCFIdentificationRequired.
	bill := &database.Bill{TotalAmount: over}
	err := validateSynchronousIssue(settings, bill)
	require.ErrorIs(t, err, ErrCFIdentificationRequired)

	// Under threshold → allowed.
	bill = &database.Bill{TotalAmount: over - 1}
	require.NoError(t, validateSynchronousIssue(settings, bill))

	// Identified by DNI → allowed regardless of amount.
	bill = &database.Bill{
		TotalAmount:             over * 2,
		FiscalCustomerDocType:   strPtrT("DNI"),
		FiscalCustomerDocNumber: strPtrT("12345678"),
	}
	require.NoError(t, validateSynchronousIssue(settings, bill))

	// Demo provider never talks to WSFE → no synchronous threshold.
	demo := &database.BusinessFiscalSettings{Provider: "demo", TaxCondition: "responsable_inscripto"}
	bill = &database.Bill{TotalAmount: over * 2}
	require.NoError(t, validateSynchronousIssue(demo, bill))

	// factura_c issuer (monotributo) → threshold does not apply.
	mono := &database.BusinessFiscalSettings{Provider: "arca", TaxCondition: "monotributo"}
	require.NoError(t, validateSynchronousIssue(mono, bill))
}

// TestBaseReceipt_CreditNoteSnapshotsOriginalReceptor: the stored credit-note
// row (and thus its PDF) must carry the receptor that was on the ORIGINAL
// authorized invoice, not the bill's mutable fiscal fields, which later issue
// overrides keep rewriting.
func TestBaseReceipt_CreditNoteSnapshotsOriginalReceptor(t *testing.T) {
	svc := &Service{}
	job := database.FiscalJob{BusinessID: 1, SettingsID: 2, BillID: 3}
	jobCtx := &JobContext{
		Bill: database.Bill{
			TotalAmount:                100_00,
			FiscalCustomerDocType:      strPtrT("DNI"),
			FiscalCustomerDocNumber:    strPtrT("99999999"),
			FiscalCustomerTaxCondition: strPtrT("consumidor_final"),
			FiscalCustomerName:         strPtrT("Drifted Name"),
		},
		Settings: database.BusinessFiscalSettings{Country: "AR", Provider: "arca"},
	}
	original := &database.FiscalReceipt{
		CustomerDocType:      strPtrT("CUIT"),
		CustomerDocNumber:    strPtrT("27111111117"),
		CustomerTaxCondition: strPtrT("responsable_inscripto"),
		CustomerName:         strPtrT("Original SA"),
	}

	nc := svc.baseReceipt(job, jobCtx, ActionCreditNote, "nota_credito_a", original)
	require.Equal(t, "CUIT", *nc.CustomerDocType)
	require.Equal(t, "27111111117", *nc.CustomerDocNumber)
	require.Equal(t, "responsable_inscripto", *nc.CustomerTaxCondition)
	require.Equal(t, "Original SA", *nc.CustomerName)

	// Issue jobs (original == nil) still snapshot from the bill.
	issue := svc.baseReceipt(job, jobCtx, ActionIssueReceipt, "factura_b", nil)
	require.Equal(t, "DNI", *issue.CustomerDocType)
	require.Equal(t, "Drifted Name", *issue.CustomerName)
}

// TestValidateCUITMod11_Residue1ClassInvalid documents WHY the residue-1 class
// ("check digit would be 10") is rejected: AFIP never issues such CUIT/CUILs —
// generation swaps prefix 20/27 → 23, putting the number back on the standard
// formula. Lenient validators that re-map 10→1 or 10→9 accept impossible CUITs.
func TestValidateCUITMod11_Residue1ClassInvalid(t *testing.T) {
	// First 10 digits "2000000001": weighted sum 12, mod 1 → computed check 10.
	for d := byte('0'); d <= '9'; d++ {
		require.False(t, validateCUITMod11("2000000001"+string(d)),
			"residue-1 CUIT must be invalid for every check digit (got valid at %c)", d)
	}
	// Control: standard-formula CUITs stay valid.
	require.True(t, validateCUITMod11("27111111117"))
	require.True(t, validateCUITMod11("20000000001"))
}
