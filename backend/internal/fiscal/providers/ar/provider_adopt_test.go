package ar

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

func qrFecha(t *testing.T, qr string) string {
	t.Helper()
	parsed, err := url.Parse(qr)
	require.NoError(t, err)
	raw, err := base64.StdEncoding.DecodeString(parsed.Query().Get("p"))
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return decoded["fecha"].(string)
}

// The first attempt went out 2026-03-31 23:50 ART (CbteFch 20260331) and the
// response was lost. The retry runs 2026-04-01 00:10 ART. Adopting voucher 42
// must keep CbteFch for the QR fecha and the stored issue date; AFIP's QR
// check requires fecha == CbteFch.
func TestProviderIssueReceipt_AdoptUsesVoucherCbteFchAcrossMidnight(t *testing.T) {
	client := &fakeWSFEClient{compResp: &CompConsultarResponse{
		Resultado:     "A",
		CAE:           "70123456789012",
		ReceiptNumber: 42,
		ImpTotal:      "50.00",
		IssueDate:     time.Date(2026, 3, 31, 0, 0, 0, 0, argentinaLocation),
		CAEExpiresAt:  time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
	}}
	provider := NewProvider(client, 20123456789)
	input := validProviderIssueInput()
	input.AttemptedProviderReceiptID = "11-1-42"
	input.IssuedAt = time.Date(2026, 4, 1, 3, 10, 0, 0, time.UTC) // 00:10 ART

	result, err := provider.IssueReceipt(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "42", result.ReceiptNumber)
	require.Equal(t, "2026-03-31", qrFecha(t, result.QRPayload))
	require.NotNil(t, result.IssuedAt)
	require.Equal(t, "2026-03-31", result.IssuedAt.In(argentinaLocation).Format("2006-01-02"))
	require.Empty(t, client.requestNumbers)
}

// Same civil day: the retry's clock is kept (it carries the time of day).
func TestProviderIssueReceipt_AdoptSameDayKeepsRetryClock(t *testing.T) {
	client := &fakeWSFEClient{compResp: &CompConsultarResponse{
		Resultado:     "A",
		CAE:           "70123456789012",
		ReceiptNumber: 42,
		ImpTotal:      "50.00",
		IssueDate:     time.Date(2026, 5, 20, 0, 0, 0, 0, argentinaLocation),
	}}
	provider := NewProvider(client, 20123456789)
	input := validProviderIssueInput()
	input.AttemptedProviderReceiptID = "11-1-42"
	input.IssuedAt = time.Date(2026, 5, 20, 18, 0, 0, 0, time.UTC)

	result, err := provider.IssueReceipt(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result.IssuedAt)
	require.True(t, result.IssuedAt.Equal(input.IssuedAt))
	require.Equal(t, "2026-05-20", qrFecha(t, result.QRPayload))
}

// A fresh authorization reports no adopted date.
func TestProviderIssueReceipt_FreshAuthorizationHasNoAdoptedDate(t *testing.T) {
	provider := NewProvider(&fakeWSFEClient{}, 20123456789)
	result, err := provider.IssueReceipt(context.Background(), validProviderIssueInput())
	require.NoError(t, err)
	require.Nil(t, result.IssuedAt)
}

// Voucher 42 was this job's submission for $60.00; the bill now totals $50.00.
// Allocating 43 would leave two authorized facturas for one bill, so the job
// must stop for manual reconciliation without asking for a new number.
func TestProviderIssueReceipt_AttemptedVoucherAmountDriftStopsWithoutNewNumber(t *testing.T) {
	client := &fakeWSFEClient{compResp: &CompConsultarResponse{
		Resultado:     "A",
		CAE:           "70123456789012",
		ReceiptNumber: 42,
		ImpTotal:      "60.00",
	}}
	provider := NewProvider(client, 20123456789)
	input := validProviderIssueInput()
	input.AttemptedProviderReceiptID = "11-1-42"
	input.OnAttempt = func(string) error {
		t.Fatal("no new voucher number may be recorded")
		return nil
	}

	result, err := provider.IssueReceipt(context.Background(), input)
	require.Nil(t, result)
	require.ErrorIs(t, err, fiscal.ErrPermanent)
	require.ErrorContains(t, err, "reconcile manually")
	require.Equal(t, 0, client.lastAuthorizedCalls)
	require.Empty(t, client.requestNumbers)
}

// Job B authorized voucher 42 after A released the series lock. Totals match,
// so without the ownership check A would adopt B's CAE. A must allocate the
// next number instead.
func TestProviderIssueReceipt_AttemptedVoucherClaimedByOtherJobAllocatesNext(t *testing.T) {
	const consultCAE = "71111111111111"
	client := &fakeWSFEClient{
		lastAuthorized: 42, // B holds 42; the next allocation is 43
		cae:            "72222222222222",
		compResp: &CompConsultarResponse{
			Resultado:     "A",
			CAE:           consultCAE,
			ReceiptNumber: 42,
			ImpTotal:      "50.00",
		},
	}
	provider := NewProvider(client, 20123456789)
	input := validProviderIssueInput()
	input.AttemptedProviderReceiptID = "11-1-42"
	input.VoucherClaimed = func(id string) (bool, error) {
		require.Equal(t, "11-1-42", id)
		return true, nil
	}

	result, err := provider.IssueReceipt(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, client.lastAuthorizedCalls)
	require.NotEmpty(t, client.requestNumbers)
	require.NotEqual(t, consultCAE, result.AuthCode)
}

// The same stolen voucher with a different ImpTotal used to stop permanently.
// Another job owns it, so A allocates a new number instead.
func TestProviderIssueReceipt_AttemptedVoucherClaimedByOtherJobDifferentTotalNotPermanent(t *testing.T) {
	client := &fakeWSFEClient{
		lastAuthorized: 42,
		cae:            "72222222222222",
		compResp: &CompConsultarResponse{
			Resultado:     "A",
			CAE:           "71111111111111",
			ReceiptNumber: 42,
			ImpTotal:      "60.00",
		},
	}
	provider := NewProvider(client, 20123456789)
	input := validProviderIssueInput()
	input.AttemptedProviderReceiptID = "11-1-42"
	input.VoucherClaimed = func(string) (bool, error) { return true, nil }

	result, err := provider.IssueReceipt(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotErrorIs(t, err, fiscal.ErrPermanent)
	require.NotEmpty(t, client.requestNumbers)
}

// An ownership-check failure stays retryable and must not request a new CAE,
// even when the callback wraps fiscal.ErrPermanent.
func TestProviderIssueReceipt_AttemptedVoucherClaimCheckErrorIsRetryable(t *testing.T) {
	client := &fakeWSFEClient{compResp: &CompConsultarResponse{
		Resultado:     "A",
		CAE:           "71111111111111",
		ReceiptNumber: 42,
		ImpTotal:      "50.00",
	}}
	provider := NewProvider(client, 20123456789)
	input := validProviderIssueInput()
	input.AttemptedProviderReceiptID = "11-1-42"
	input.VoucherClaimed = func(string) (bool, error) {
		return false, fmt.Errorf("db down: %w", fiscal.ErrPermanent)
	}

	result, err := provider.IssueReceipt(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, result)
	require.NotErrorIs(t, err, fiscal.ErrPermanent)
	require.Empty(t, client.requestNumbers)
}

// The attempted voucher is still this job's. Matching totals adopt it and
// do not request a new number.
func TestProviderIssueReceipt_AttemptedVoucherOwnedByThisJobIsAdopted(t *testing.T) {
	const consultCAE = "70123456789012"
	client := &fakeWSFEClient{compResp: &CompConsultarResponse{
		Resultado:     "A",
		CAE:           consultCAE,
		ReceiptNumber: 42,
		ImpTotal:      "50.00",
	}}
	provider := NewProvider(client, 20123456789)
	input := validProviderIssueInput()
	input.AttemptedProviderReceiptID = "11-1-42"
	input.VoucherClaimed = func(string) (bool, error) { return false, nil }

	result, err := provider.IssueReceipt(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, consultCAE, result.AuthCode)
	require.Empty(t, client.requestNumbers)
}
