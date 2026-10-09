package fiscal

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guestsession"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// capturingIssueProvider records the IssueInput and returns no result, so
// processIssueJob stops at the retryable "empty_result" outcome without
// persisting a receipt.
type capturingIssueProvider struct {
	fakeProvider
	inputs []IssueInput
}

func (p *capturingIssueProvider) IssueReceipt(_ context.Context, in IssueInput) (*ReceiptResult, error) {
	p.inputs = append(p.inputs, in)
	return nil, nil
}

func seedGuestIdentityBill(t *testing.T, db *gorm.DB, setterSession string) database.Bill {
	t.Helper()
	bill := database.Bill{
		BusinessID:                 1,
		BillNumber:                 "GUARD-" + setterSession,
		TotalAmount:                12100,
		PaidAmount:                 12100,
		Status:                     database.BillStatusPaid,
		FiscalCustomerDocType:      strPtr("CUIT"),
		FiscalCustomerDocNumber:    strPtr("20111111112"),
		FiscalCustomerTaxCondition: strPtr("responsable_inscripto"),
		FiscalCustomerName:         strPtr("Squatter SA"),
		FiscalCustomerEmail:        strPtr("squat@example.com"),
		FiscalCustomerGuestSession: guestsession.FingerprintPtr(setterSession),
	}
	require.NoError(t, db.Create(&bill).Error)
	return bill
}

func seedGuestPaidPayment(t *testing.T, db *gorm.DB, billID uint, session string) {
	t.Helper()
	require.NoError(t, db.Create(&database.Payment{
		BillID:            billID,
		PayerAddr:         "crypto_guest",
		Amount:            12100,
		TxHash:            "0xguard-" + session,
		Status:            database.PaymentStatusConfirmed,
		PayerGuestSession: guestsession.FingerprintPtr(session),
	}).Error)
}

func runIssueJobForBill(t *testing.T, db *gorm.DB, billID uint) (*capturingIssueProvider, *JobOutcome) {
	t.Helper()
	svc := NewService(db, NewProviderRegistry())
	var loaded database.Bill
	require.NoError(t, db.First(&loaded, billID).Error)
	jobCtx := &JobContext{
		Bill:     loaded,
		Settings: database.BusinessFiscalSettings{Country: "AR", Provider: "arca"},
	}
	job := database.FiscalJob{BusinessID: 1, SettingsID: 1, BillID: billID}
	job.ID = 11
	provider := &capturingIssueProvider{}
	out, err := svc.processIssueJob(context.Background(), job, jobCtx, provider)
	require.NoError(t, err)
	return provider, out
}

// M-545: a guest session that set the receptor but did not pay, on a bill other
// guest sessions paid, is a squat — the factura goes to consumidor final.
func TestProcessIssueJob_DropsSquattedGuestIdentity(t *testing.T) {
	db := newFiscalTestDB(t)
	bill := seedGuestIdentityBill(t, db, "squatter")
	seedGuestPaidPayment(t, db, bill.ID, "payer")

	provider, out := runIssueJobForBill(t, db, bill.ID)
	require.Equal(t, "empty_result", out.ErrorCode)
	require.Len(t, provider.inputs, 1)
	in := provider.inputs[0]
	require.Empty(t, in.CustomerDocNumber, "squatter CUIT must not reach the provider")
	require.Empty(t, in.CustomerName)
	require.Empty(t, in.CustomerTaxCondition)
	require.Nil(t, in.Bill.FiscalCustomerEmail, "squatter email must not receive the factura")

	var reloaded database.Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.Nil(t, reloaded.FiscalCustomerDocNumber, "the squat is cleared on the bill too (delivery reads it)")
	require.Nil(t, reloaded.FiscalCustomerEmail)
	require.Nil(t, reloaded.FiscalCustomerGuestSession)
}

func TestProcessIssueJob_KeepsPayingGuestIdentity(t *testing.T) {
	db := newFiscalTestDB(t)
	bill := seedGuestIdentityBill(t, db, "payer")
	seedGuestPaidPayment(t, db, bill.ID, "payer")
	seedGuestPaidPayment(t, db, bill.ID, "friend")

	provider, _ := runIssueJobForBill(t, db, bill.ID)
	require.Len(t, provider.inputs, 1)
	require.Equal(t, "20111111112", provider.inputs[0].CustomerDocNumber, "a paying setter keeps their factura receptor")
	require.Equal(t, "Squatter SA", provider.inputs[0].CustomerName)
}

// A staff-settled bill carries no evidence the guest setter paid, so the
// setter's identity is a squat: a bill token holder must not be able to
// redirect the factura for a counter-paid bill to their own CUIT/email.
func TestProcessIssueJob_DropsGuestIdentityOnStaffSettledBill(t *testing.T) {
	db := newFiscalTestDB(t)
	bill := seedGuestIdentityBill(t, db, "squatter")
	require.NoError(t, db.Create(&database.Payment{
		BillID: bill.ID, PayerAddr: "cash", Amount: 12100, TxHash: "cash-guard",
		Status: database.PaymentStatusConfirmed,
	}).Error)

	provider, _ := runIssueJobForBill(t, db, bill.ID)
	require.Len(t, provider.inputs, 1)
	require.Empty(t, provider.inputs[0].CustomerDocNumber, "squatter CUIT must not reach the provider")
	require.Nil(t, provider.inputs[0].Bill.FiscalCustomerEmail)

	var reloaded database.Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.Nil(t, reloaded.FiscalCustomerDocNumber)
	require.Nil(t, reloaded.FiscalCustomerGuestSession)
}

func TestProcessIssueJob_GuardErrorIsRetryable(t *testing.T) {
	db := newFiscalTestDB(t)
	bill := seedGuestIdentityBill(t, db, "squatter")
	seedGuestPaidPayment(t, db, bill.ID, "payer")
	var loaded database.Bill
	require.NoError(t, db.First(&loaded, bill.ID).Error)
	require.NoError(t, db.Migrator().DropTable(&database.AlternativePayment{}))

	svc := NewService(db, NewProviderRegistry())
	jobCtx := &JobContext{Bill: loaded, Settings: database.BusinessFiscalSettings{Country: "AR", Provider: "arca"}}
	job := database.FiscalJob{BusinessID: 1, SettingsID: 1, BillID: bill.ID}
	provider := &capturingIssueProvider{}
	out, err := svc.processIssueJob(context.Background(), job, jobCtx, provider)
	require.NoError(t, err)
	require.True(t, out.Retryable)
	require.Equal(t, "fiscal_identity_check_failed", out.ErrorCode)
	require.Empty(t, provider.inputs, "never issue with an unverified receptor")
}

// An operator receiver override takes ownership: the guest setter is cleared,
// so the issuance guard never drops an operator-entered receptor.
func TestApplyReceiverOverride_ClearsGuestSetter(t *testing.T) {
	db := newFiscalTestDB(t)
	bill := seedGuestIdentityBill(t, db, "squatter")
	seedGuestPaidPayment(t, db, bill.ID, "payer")

	require.NoError(t, applyReceiverOverride(db, &bill, &ReceiverOverride{
		CustomerDocType:      "CUIT",
		CustomerDocNumber:    "20-11111111-2",
		CustomerTaxCondition: "responsable_inscripto",
		CustomerName:         "Operator Checked SA",
	}))
	require.Nil(t, bill.FiscalCustomerGuestSession)

	provider, _ := runIssueJobForBill(t, db, bill.ID)
	require.Len(t, provider.inputs, 1)
	require.Equal(t, "Operator Checked SA", provider.inputs[0].CustomerName, "operator receptor survives the guard")
}
