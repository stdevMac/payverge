package ar

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"

	"github.com/stretchr/testify/require"
)

func TestProviderIssueReceiptUsesClient(t *testing.T) {
	client := &fakeWSFEClient{}
	provider := NewProvider(client, 20123456789)
	input := validProviderIssueInput()
	result, err := provider.IssueReceipt(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, fiscal.StatusAuthorized, result.Status)
	// AuthCode carries the CAE; ProviderReceiptID is now the composite WSFE lookup
	// key "<typeCode>-<pos>-<number>" (factura_c → code 11, PoS 1, number 42).
	require.Equal(t, "70123456789012", result.AuthCode)
	require.Equal(t, "11-1-42", result.ProviderReceiptID)
	require.NotEmpty(t, result.QRPayload)
	require.Equal(t, int64(42), client.nextNumber)
	require.Equal(t, int64(20123456789), client.lastPayload.CUIT)
	require.Equal(t, 1, client.lastPayload.PointOfSale)
	require.Equal(t, 11, client.lastPayload.ReceiptTypeCode)
	require.Equal(t, int64(5000), client.lastPayload.TotalCents)
	require.Equal(t, "PES", client.lastPayload.CurrencyCode)
}

func TestProviderIssueReceiptNilClientErrors(t *testing.T) {
	provider := NewProvider(nil, 20123456789)
	result, err := provider.IssueReceipt(context.Background(), validProviderIssueInput())
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "wsfe client")
}

func TestProviderIssueReceiptLastAuthorizedErrorPropagates(t *testing.T) {
	expected := errors.New("last authorized failed")
	provider := NewProvider(&fakeWSFEClient{lastErr: expected}, 20123456789)
	result, err := provider.IssueReceipt(context.Background(), validProviderIssueInput())
	require.ErrorIs(t, err, expected)
	require.Nil(t, result)
}

func TestProviderIssueReceiptRequestCAEErrorPropagates(t *testing.T) {
	expected := errors.New("cae request failed")
	client := &fakeWSFEClient{requestErr: expected}
	provider := NewProvider(client, 20123456789)
	result, err := provider.IssueReceipt(context.Background(), validProviderIssueInput())
	require.ErrorIs(t, err, expected)
	require.Nil(t, result)
	require.Equal(t, int64(42), client.nextNumber)
}

func TestProviderIssueReceiptMalformedCAEErrors(t *testing.T) {
	provider := NewProvider(&fakeWSFEClient{cae: "CAE-123"}, 20123456789)
	result, err := provider.IssueReceipt(context.Background(), validProviderIssueInput())
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "CAE")
}

func TestProviderIssueReceiptOverflowCAEErrors(t *testing.T) {
	provider := NewProvider(&fakeWSFEClient{cae: "999999999999999999999999"}, 20123456789)
	result, err := provider.IssueReceipt(context.Background(), validProviderIssueInput())
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "CAE")
}

func TestProviderIssueReceiptNilCAEResponseErrors(t *testing.T) {
	provider := NewProvider(&fakeWSFEClient{nilResponse: true}, 20123456789)
	result, err := provider.IssueReceipt(context.Background(), validProviderIssueInput())
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "CAE response")
}

func validProviderIssueInput() fiscal.IssueInput {
	point := 1
	return fiscal.IssueInput{
		Settings: fiscal.Settings{
			TaxID:       "20123456789",
			PointOfSale: &point,
		},
		Bill:             database.Bill{TotalAmount: 5000, PaidAmount: 5000, Status: database.BillStatusPaid},
		ReceiptType:      "factura_c",
		Currency:         "ARS",
		TotalAmountCents: 5000,
		IssuedAt:         time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC),
	}
}

type fakeWSFEClient struct {
	lastPayload WSFEPayload
	nextNumber  int64
	lastErr     error
	requestErr  error
	cae         string
	nilResponse bool

	dummyErr error
	authErr  error
	dummyHit int
	authHit  int

	// CompConsultar capture/injection.
	compResp *CompConsultarResponse
	compErr  error
	compHit  int

	// lastAuthorized overrides the default LastAuthorized result of 41.
	// 0 keeps the default. lastAuthorizedCalls counts every LastAuthorized call.
	lastAuthorized      int64
	lastAuthorizedCalls int
	// requestNumbers is every voucher number passed to RequestCAE, in order.
	requestNumbers []int64
	// compNumbers is every voucher number passed to CompConsultar, in order.
	compNumbers []int64
}

func (f *fakeWSFEClient) Dummy(ctx context.Context) error {
	f.dummyHit++
	return f.dummyErr
}

func (f *fakeWSFEClient) Authenticate(ctx context.Context) error {
	f.authHit++
	return f.authErr
}

func (f *fakeWSFEClient) LastAuthorized(ctx context.Context, payload WSFEPayload) (int64, error) {
	f.lastAuthorizedCalls++
	if f.lastErr != nil {
		return 0, f.lastErr
	}
	f.lastPayload = payload
	if f.lastAuthorized != 0 {
		return f.lastAuthorized, nil
	}
	return 41, nil
}

func (f *fakeWSFEClient) RequestCAE(ctx context.Context, payload WSFEPayload, nextNumber int64) (*CAEResponse, error) {
	f.lastPayload = payload
	f.nextNumber = nextNumber
	f.requestNumbers = append(f.requestNumbers, nextNumber)
	if f.requestErr != nil {
		return nil, f.requestErr
	}
	if f.nilResponse {
		return nil, nil
	}
	cae := f.cae
	if cae == "" {
		cae = "70123456789012"
	}
	return &CAEResponse{
		ReceiptNumber: 42,
		CAE:           cae,
		CAEExpiresAt:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}, nil
}

func (f *fakeWSFEClient) CompConsultar(ctx context.Context, cuit, ptoVta, cbteTipo, cbteNro int64) (*CompConsultarResponse, error) {
	f.compHit++
	f.compNumbers = append(f.compNumbers, cbteNro)
	if f.compErr != nil {
		return nil, f.compErr
	}
	if f.compResp != nil {
		return f.compResp, nil
	}
	// Default: the queried number was never authorized. The T4 reconcile must
	// treat a fresh-number lookup as "not committed" unless a test explicitly
	// injects an authorized compResp (the lost-response adopt path).
	return &CompConsultarResponse{Resultado: ""}, nil
}

// --- T4: idempotent issuance — reconcile a lost-response authorization --------

// When RequestCAE fails with a TRANSIENT/transport error (not ErrPermanent),
// AFIP may have already committed the CAE before the response was lost. The
// provider must reconcile via FECompConsultar(N) and, if it reports Resultado=A,
// adopt that authorization instead of returning an error (which would later
// cause a duplicate-number re-issue).
func TestProviderIssueReceipt_AdoptsLostResponseAuthorization(t *testing.T) {
	client := &fakeWSFEClient{
		requestErr: errors.New("connection reset by peer"), // transient
		compResp: &CompConsultarResponse{
			Resultado:     "A",
			CAE:           "70123456789012",
			ReceiptNumber: 42,
			CAEExpiresAt:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	provider := NewProvider(client, 20123456789)
	result, err := provider.IssueReceipt(context.Background(), validProviderIssueInput())
	require.NoError(t, err)
	require.Equal(t, fiscal.StatusAuthorized, result.Status)
	require.Equal(t, "70123456789012", result.AuthCode)
	require.Equal(t, "42", result.ReceiptNumber)
	require.Equal(t, "11-1-42", result.ProviderReceiptID)
	require.NotEmpty(t, result.QRPayload)
	require.Equal(t, 1, client.compHit, "must reconcile exactly once")
}

// When the reconcile reports the number was never authorized, the ORIGINAL
// transient error is returned so the worker retries; because LastAuthorized
// still returns N-1 the next attempt re-requests the SAME number (no duplicate).
func TestProviderIssueReceipt_ReconcileNotFoundReturnsOriginalError(t *testing.T) {
	expected := errors.New("connection reset by peer")
	client := &fakeWSFEClient{requestErr: expected, compResp: &CompConsultarResponse{Resultado: ""}}
	provider := NewProvider(client, 20123456789)
	result, err := provider.IssueReceipt(context.Background(), validProviderIssueInput())
	require.ErrorIs(t, err, expected)
	require.Nil(t, result)
	require.Equal(t, 1, client.compHit, "reconcile is attempted once")
	require.Equal(t, int64(42), client.nextNumber, "RequestCAE used LastAuthorized+1")
}

// A deterministic AFIP rejection (ErrPermanent) must NOT trigger a reconcile —
// the invoice will never be authorized, so the permanent error propagates.
func TestProviderIssueReceipt_PermanentErrorSkipsReconcile(t *testing.T) {
	client := &fakeWSFEClient{requestErr: fmt.Errorf("AFIP rejected: %w", fiscal.ErrPermanent)}
	provider := NewProvider(client, 20123456789)
	result, err := provider.IssueReceipt(context.Background(), validProviderIssueInput())
	require.ErrorIs(t, err, fiscal.ErrPermanent)
	require.Nil(t, result)
	require.Equal(t, 0, client.compHit, "no reconcile for a permanent rejection")
}

// The same reconcile guard protects credit-note issuance.
func TestProviderIssueCreditNote_AdoptsLostResponseAuthorization(t *testing.T) {
	client := &fakeWSFEClient{
		requestErr: errors.New("i/o timeout"),
		compResp: &CompConsultarResponse{
			Resultado:     "A",
			CAE:           "70123456789099",
			ReceiptNumber: 42,
			CAEExpiresAt:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	provider := NewProvider(client, 20123456789)
	result, err := provider.IssueCreditNote(context.Background(), validCreditNoteInputWithSettings())
	require.NoError(t, err)
	require.Equal(t, fiscal.StatusAuthorized, result.Status)
	require.Equal(t, "70123456789099", result.AuthCode)
	require.Equal(t, "42", result.ReceiptNumber)
	require.Equal(t, 1, client.compHit)
}

// TestRequestCAE_AmbiguousTimeout_RetryQueriesAttemptedNumberNeverNPlus1 locks
// the persisted-attempt contract: when RequestCAE(N) times out and the
// FECompConsultar(N) reconcile also errors, a later retry must keep querying N
// and must not allocate N+1 even if LastAuthorized has moved to N.
func TestRequestCAE_AmbiguousTimeout_RetryQueriesAttemptedNumberNeverNPlus1(t *testing.T) {
	client := &fakeWSFEClient{
		requestErr: errors.New("i/o timeout"),
		compErr:    errors.New("consultar down"),
	}
	provider := NewProvider(client, 20123456789)

	var recorded string
	input := validProviderIssueInput()
	input.OnAttempt = func(id string) error {
		recorded = id
		return nil
	}

	result, err := provider.IssueReceipt(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, result)
	require.NotErrorIs(t, err, fiscal.ErrPermanent)
	require.Equal(t, "11-1-42", recorded)
	require.Equal(t, []int64{42}, client.requestNumbers)
	require.Equal(t, 1, client.lastAuthorizedCalls)

	// AFIP committed 42, but the consult is still down. The retry must not
	// ask LastAuthorized or request 43.
	client.lastAuthorized = 42
	client.requestErr = nil
	beforeLA := client.lastAuthorizedCalls
	beforeReq := len(client.requestNumbers)
	input.AttemptedProviderReceiptID = recorded
	input.OnAttempt = func(string) error {
		t.Fatal("OnAttempt must not run while a persisted attempt is still ambiguous")
		return nil
	}
	result, err = provider.IssueReceipt(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, result)
	require.NotErrorIs(t, err, fiscal.ErrPermanent)
	require.ErrorContains(t, err, "reconcile of previously attempted voucher 42 failed")
	require.Equal(t, beforeLA, client.lastAuthorizedCalls, "LastAuthorized must not run on the ambiguous retry")
	require.Len(t, client.requestNumbers, beforeReq)
	require.NotContains(t, client.requestNumbers, int64(43))

	client.compErr = nil
	client.compResp = &CompConsultarResponse{
		Resultado:     "A",
		CAE:           "70123456789012",
		ReceiptNumber: 42,
		ImpTotal:      "50.00",
		CAEExpiresAt:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	result, err = provider.IssueReceipt(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, fiscal.StatusAuthorized, result.Status)
	require.Equal(t, "42", result.ReceiptNumber)
	require.Equal(t, "70123456789012", result.AuthCode)
	require.Equal(t, "11-1-42", result.ProviderReceiptID)
	require.Len(t, client.requestNumbers, beforeReq, "adopting the attempted number must not call RequestCAE")
	require.NotContains(t, client.requestNumbers, int64(43))
}

// OnAttempt is the last chance to persist the number before it is sent. A
// failure there must skip RequestCAE and stay retryable.
func TestRequestCAE_OnAttemptErrorSkipsRequest(t *testing.T) {
	client := &fakeWSFEClient{}
	provider := NewProvider(client, 20123456789)
	input := validProviderIssueInput()
	input.OnAttempt = func(id string) error {
		require.Equal(t, "11-1-42", id)
		return fmt.Errorf("db down: %w", fiscal.ErrPermanent)
	}
	result, err := provider.IssueReceipt(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, result)
	require.NotErrorIs(t, err, fiscal.ErrPermanent)
	require.Empty(t, client.requestNumbers)
	require.Equal(t, 1, client.lastAuthorizedCalls)
	require.Equal(t, 0, client.compHit)
}

// recordingWSFE records LastAuthorized and RequestCAE around the fake client
// so a test can assert the series lock brackets both calls.
type recordingWSFE struct {
	*fakeWSFEClient
	order *[]string
}

func (r *recordingWSFE) LastAuthorized(ctx context.Context, payload WSFEPayload) (int64, error) {
	*r.order = append(*r.order, "LastAuthorized")
	return r.fakeWSFEClient.LastAuthorized(ctx, payload)
}

func (r *recordingWSFE) RequestCAE(ctx context.Context, payload WSFEPayload, nextNumber int64) (*CAEResponse, error) {
	*r.order = append(*r.order, "RequestCAE")
	return r.fakeWSFEClient.RequestCAE(ctx, payload, nextNumber)
}

// TestIssueReceipt_LockSeriesOrdersAndRetryableFailure locks the series around
// number allocation: the callback runs once, before LastAuthorized, and its
// release runs after RequestCAE. A lock error stays retryable and never
// reaches AFIP.
func TestIssueReceipt_LockSeriesOrdersAndRetryableFailure(t *testing.T) {
	t.Run("before LastAuthorized and released after RequestCAE", func(t *testing.T) {
		var order []string
		client := &fakeWSFEClient{}
		provider := NewProvider(&recordingWSFE{fakeWSFEClient: client, order: &order}, 20123456789)
		input := validProviderIssueInput()
		input.LockSeries = func(ctx context.Context, key string) (func(), error) {
			order = append(order, "lock:"+key)
			return func() { order = append(order, "release") }, nil
		}

		result, err := provider.IssueReceipt(context.Background(), input)
		require.NoError(t, err)
		require.Equal(t, fiscal.StatusAuthorized, result.Status)
		require.Equal(t, []string{
			"lock:arca:20123456789:1:11",
			"LastAuthorized",
			"RequestCAE",
			"release",
		}, order)
	})

	t.Run("lock error is retryable and skips AFIP", func(t *testing.T) {
		client := &fakeWSFEClient{}
		provider := NewProvider(client, 20123456789)
		input := validProviderIssueInput()
		input.LockSeries = func(ctx context.Context, key string) (func(), error) {
			return nil, fmt.Errorf("advisory: %w", fiscal.ErrPermanent)
		}

		result, err := provider.IssueReceipt(context.Background(), input)
		require.Error(t, err)
		require.Nil(t, result)
		require.NotErrorIs(t, err, fiscal.ErrPermanent)
		require.ErrorContains(t, err, "lock fiscal series:")
		require.Equal(t, 0, client.lastAuthorizedCalls)
		require.Empty(t, client.requestNumbers)
	})
}

// A persisted number that AFIP reports as not found was never committed, so
// allocation falls back to LastAuthorized+1.
func TestRequestCAE_AttemptedNotFoundFallsBackToLastAuthorizedPlusOne(t *testing.T) {
	client := &fakeWSFEClient{compResp: &CompConsultarResponse{Resultado: ""}}
	provider := NewProvider(client, 20123456789)
	input := validProviderIssueInput()
	input.AttemptedProviderReceiptID = "11-1-99"
	result, err := provider.IssueReceipt(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, fiscal.StatusAuthorized, result.Status)
	require.Equal(t, []int64{99}, client.compNumbers)
	require.Equal(t, []int64{42}, client.requestNumbers)
	require.Equal(t, 1, client.lastAuthorizedCalls)
}
