package ar

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/fiscal"

	"github.com/stretchr/testify/require"
)

// feCompConsultarFixture returns a canned FECompConsultarResponse with the given
// Resultado ("A"/"R") and CodAutorizacion (CAE).
func feCompConsultarFixture(resultado, cae string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <FECompConsultarResponse>
      <FECompConsultarResult>
        <ResultGet>
          <Resultado>` + resultado + `</Resultado>
          <CodAutorizacion>` + cae + `</CodAutorizacion>
        </ResultGet>
      </FECompConsultarResult>
    </FECompConsultarResponse>
  </soapenv:Body>
</soapenv:Envelope>`
}

// feCompConsultarNotFoundFixture mimics AFIP returning an Errors block (e.g. code
// 602 "comprobante inexistente") with no ResultGet payload.
func feCompConsultarNotFoundFixture() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <FECompConsultarResponse>
      <FECompConsultarResult>
        <Errors>
          <Err>
            <Code>602</Code>
            <Msg>No existe el comprobante solicitado</Msg>
          </Err>
        </Errors>
      </FECompConsultarResult>
    </FECompConsultarResponse>
  </soapenv:Body>
</soapenv:Envelope>`
}

func TestEncodeDecodeProviderReceiptID(t *testing.T) {
	id := encodeProviderReceiptID(11, 3, 43)
	require.Equal(t, "11-3-43", id)

	typeCode, pos, number, err := decodeProviderReceiptID(id)
	require.NoError(t, err)
	require.Equal(t, int64(11), typeCode)
	require.Equal(t, int64(3), pos)
	require.Equal(t, int64(43), number)
}

func TestDecodeProviderReceiptID_MalformedWrapsErrPermanent(t *testing.T) {
	for _, bad := range []string{"", "11-3", "11-3-43-9", "a-b-c", "11--3"} {
		_, _, _, err := decodeProviderReceiptID(bad)
		require.Error(t, err, "input %q must fail", bad)
		require.ErrorIs(t, err, fiscal.ErrPermanent, "malformed key %q must wrap ErrPermanent", bad)
	}
}

func TestGetStatus_AuthorizedMapsToAuthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, feCompConsultarFixture("A", "71000000000001"))
	}))
	defer srv.Close()

	p := NewProvider(newTestWSFEClient(srv.URL), 20111111112)
	status, err := p.GetStatus(context.Background(), encodeProviderReceiptID(11, 3, 43))
	require.NoError(t, err)
	require.Equal(t, fiscal.StatusAuthorized, status.Status)
	require.Equal(t, "A", status.ProviderState)
	require.False(t, status.CheckedAt.IsZero())
}

// feCompConsultarRichAuthorizedFixture mirrors AFIP's full ResultGet block for an
// authorized voucher so GetStatus can rebuild the CAE/number/expiry/QR.
func feCompConsultarRichAuthorizedFixture() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <FECompConsultarResponse>
      <FECompConsultarResult>
        <ResultGet>
          <Resultado>A</Resultado>
          <CodAutorizacion>71000000000001</CodAutorizacion>
          <CbteDesde>43</CbteDesde>
          <CbteFch>20260619</CbteFch>
          <FchVto>20260630</FchVto>
          <ImpTotal>121.00</ImpTotal>
          <MonId>PES</MonId>
          <MonCotiz>1.000000</MonCotiz>
          <DocTipo>80</DocTipo>
          <DocNro>20111111112</DocNro>
        </ResultGet>
      </FECompConsultarResult>
    </FECompConsultarResponse>
  </soapenv:Body>
</soapenv:Envelope>`
}

func TestGetStatus_AuthorizedEnrichesAuthDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, feCompConsultarRichAuthorizedFixture())
	}))
	defer srv.Close()

	p := NewProvider(newTestWSFEClient(srv.URL), 20111111112)
	status, err := p.GetStatus(context.Background(), encodeProviderReceiptID(11, 3, 43))
	require.NoError(t, err)
	require.Equal(t, fiscal.StatusAuthorized, status.Status)
	require.Equal(t, "A", status.ProviderState)

	// The full authorization tuple must be plumbed through, not thrown away.
	require.Equal(t, "71000000000001", status.AuthCode)
	require.Equal(t, "43", status.ReceiptNumber)
	require.NotNil(t, status.AuthExpiresAt)
	require.Equal(t, "20260630", status.AuthExpiresAt.Format("20060102"))
	require.NotEmpty(t, status.QRPayload)
	require.Contains(t, status.QRPayload, "https://www.arca.gob.ar/fe/qr/")
}

func TestGetStatus_RejectedMapsToRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, feCompConsultarFixture("R", ""))
	}))
	defer srv.Close()

	p := NewProvider(newTestWSFEClient(srv.URL), 20111111112)
	status, err := p.GetStatus(context.Background(), encodeProviderReceiptID(11, 3, 43))
	require.NoError(t, err)
	require.Equal(t, fiscal.StatusRejected, status.Status)
	require.Equal(t, "R", status.ProviderState)
}

func TestGetStatus_NotFoundMapsToFailedRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, feCompConsultarNotFoundFixture())
	}))
	defer srv.Close()

	p := NewProvider(newTestWSFEClient(srv.URL), 20111111112)
	status, err := p.GetStatus(context.Background(), encodeProviderReceiptID(11, 3, 43))
	require.NoError(t, err)
	require.Equal(t, fiscal.StatusFailedRetryable, status.Status)
	require.NotEmpty(t, status.ProviderErrors)
	require.Equal(t, "602", status.ProviderErrors[0].Code)
}

func TestGetStatus_MalformedReceiptIDReturnsPermanentError(t *testing.T) {
	// No server call should happen for a malformed key.
	p := NewProvider(&fakeWSFEClient{}, 20111111112)
	status, err := p.GetStatus(context.Background(), "not-a-valid-key")
	require.Error(t, err)
	require.Nil(t, status)
	require.ErrorIs(t, err, fiscal.ErrPermanent)
}

func TestGetStatus_TransportErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p := NewProvider(newTestWSFEClient(srv.URL), 20111111112)
	status, err := p.GetStatus(context.Background(), encodeProviderReceiptID(11, 3, 43))
	require.Error(t, err)
	require.Nil(t, status)
	// A transport/SOAP error must NOT be wrapped as permanent (retryable by default).
	require.False(t, errors.Is(err, fiscal.ErrPermanent))
}

func TestGetStatus_NilClientErrors(t *testing.T) {
	p := NewProvider(nil, 20111111112)
	status, err := p.GetStatus(context.Background(), encodeProviderReceiptID(11, 3, 43))
	require.Error(t, err)
	require.Nil(t, status)
}
