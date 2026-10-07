package ar

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

func failClosedInput(attempted string) fiscal.IssueInput {
	in := validProviderIssueInput()
	in.AttemptedProviderReceiptID = attempted
	in.OnAttempt = func(string) error {
		panic("onAttempt must not run")
	}
	return in
}

// A consult that reports an AFIP error leaves the attempted voucher
// undetermined; with LastAuthorized already at that number the job must stop
// and retry, not allocate.
func TestRequestCAE_ConsultErrorWithLastAuthorizedAtAttemptedIsRetryable(t *testing.T) {
	client := &fakeWSFEClient{
		lastAuthorized: 10,
		compResp: &CompConsultarResponse{Errors: []struct {
			Code int
			Msg  string
		}{{Code: 501, Msg: "boom"}}},
	}
	_, err := NewProvider(client, 20123456789).IssueReceipt(context.Background(), failClosedInput("11-1-10"))
	require.Error(t, err)
	require.NotErrorIs(t, err, fiscal.ErrPermanent)
	require.Empty(t, client.requestNumbers)
}

// Not found (602) with LastAuthorized still below N is a clean "never
// authorized": the same number is re-requested.
func TestRequestCAE_ConsultNotFoundReallocatesSameNumber(t *testing.T) {
	client := &fakeWSFEClient{
		lastAuthorized: 9,
		compResp: &CompConsultarResponse{Errors: []struct {
			Code int
			Msg  string
		}{{Code: 602, Msg: "no existen datos"}}},
	}
	in := validProviderIssueInput()
	in.AttemptedProviderReceiptID = "11-1-10"
	_, err := NewProvider(client, 20123456789).IssueReceipt(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, []int64{10}, client.requestNumbers)
}

// Consult says "not authorized" but AFIP's last number already reached N.
func TestRequestCAE_NotAuthorizedButLastAtAttemptedIsRetryable(t *testing.T) {
	client := &fakeWSFEClient{lastAuthorized: 10, compResp: &CompConsultarResponse{Resultado: ""}}
	_, err := NewProvider(client, 20123456789).IssueReceipt(context.Background(), failClosedInput("11-1-10"))
	require.Error(t, err)
	require.NotErrorIs(t, err, fiscal.ErrPermanent)
	require.Empty(t, client.requestNumbers)
}

func TestRequestCAE_UndecodableAttemptFailsClosed(t *testing.T) {
	client := &fakeWSFEClient{}
	_, err := NewProvider(client, 20123456789).IssueReceipt(context.Background(), failClosedInput("garbage"))
	require.Error(t, err)
	require.NotErrorIs(t, err, fiscal.ErrPermanent)
	require.Empty(t, client.requestNumbers)
	require.Equal(t, 0, client.lastAuthorizedCalls)
}

// The recorded attempt is in another series (bill changed type). The consult
// must use the recorded tuple, and an authorized voucher there stops the job.
func TestRequestCAE_AttemptInOtherSeriesIsConsultedByItsOwnTuple(t *testing.T) {
	client := &fakeWSFEClient{compResp: &CompConsultarResponse{Resultado: "A", CAE: "70123456789012", ReceiptNumber: 5, ImpTotal: "50.00"}}
	_, err := NewProvider(client, 20123456789).IssueReceipt(context.Background(), failClosedInput("6-1-5"))
	require.ErrorIs(t, err, fiscal.ErrPermanent)
	require.Equal(t, []int64{5}, client.compNumbers)
	require.Empty(t, client.requestNumbers)
}
