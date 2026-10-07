package ar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

// stubAuthProvider returns fixed token/sign without any network call.
type stubAuthProvider struct {
	token string
	sign  string
}

func (s *stubAuthProvider) Authenticate(_ context.Context) (*WSAACredentials, error) {
	return &WSAACredentials{
		Token:     s.token,
		Sign:      s.sign,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}, nil
}

// newTestWSFEClient creates a WSFEClient pointed at the given URL with a stub auth provider.
func newTestWSFEClient(url string) WSFEClient {
	return NewWSFEClient(url, &stubAuthProvider{token: "test-token", sign: "test-sign"})
}

// feCompUltimoAutorizadoFixture returns a minimal well-formed SOAP envelope
// simulating a FECompUltimoAutorizadoResponse with the given receipt number.
func feCompUltimoAutorizadoFixture(n int64) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <FECompUltimoAutorizadoResponse>
      <FECompUltimoAutorizadoResult>
        <CbteNro>%d</CbteNro>
      </FECompUltimoAutorizadoResult>
    </FECompUltimoAutorizadoResponse>
  </soapenv:Body>
</soapenv:Envelope>`, n)
}

func TestWSFELastAuthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, feCompUltimoAutorizadoFixture(42))
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	got, err := c.LastAuthorized(context.Background(), WSFEPayload{
		CUIT:            20111111112,
		PointOfSale:     1,
		ReceiptTypeCode: 6,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != 42 {
		t.Errorf("want 42 got %d", got)
	}
}

func TestWSFELastAuthorized_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	_, err := c.LastAuthorized(context.Background(), WSFEPayload{CUIT: 20111111112, PointOfSale: 1, ReceiptTypeCode: 6})
	if err == nil {
		t.Fatal("expected error for non-200 HTTP response, got nil")
	}
}

func TestWSFELastAuthorized_WSFEError(t *testing.T) {
	errFixture := `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <FECompUltimoAutorizadoResponse>
      <FECompUltimoAutorizadoResult>
        <CbteNro>0</CbteNro>
        <Errors><Err><Msg>punto de venta invalido</Msg></Err></Errors>
      </FECompUltimoAutorizadoResult>
    </FECompUltimoAutorizadoResponse>
  </soapenv:Body>
</soapenv:Envelope>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, errFixture)
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	_, err := c.LastAuthorized(context.Background(), WSFEPayload{CUIT: 20111111112, PointOfSale: 999, ReceiptTypeCode: 6})
	if err == nil {
		t.Fatal("expected error for WSFE error response, got nil")
	}
}

func TestWSFELastAuthorized_AuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should never be reached.
		io.WriteString(w, feCompUltimoAutorizadoFixture(1))
	}))
	defer srv.Close()

	failAuth := &failingAuthProvider{}
	c := NewWSFEClient(srv.URL, failAuth)
	_, err := c.LastAuthorized(context.Background(), WSFEPayload{CUIT: 20111111112, PointOfSale: 1, ReceiptTypeCode: 6})
	if err == nil {
		t.Fatal("expected error when auth fails, got nil")
	}
}

type failingAuthProvider struct{}

func (f *failingAuthProvider) Authenticate(_ context.Context) (*WSAACredentials, error) {
	return nil, fmt.Errorf("certificate expired")
}

// feCAESolicitarFixtureApproved returns a canned SOAP FECAESolicitarResponse
// with Resultado=A and the provided CAE, expiry date, and receipt number.
func feCAESolicitarFixtureApproved(cae, fchVto string, desde int64) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <FECAESolicitarResponse>
      <FECAESolicitarResult>
        <FeDetResp>
          <FECAEDetResponse>
            <Resultado>A</Resultado>
            <CAE>%s</CAE>
            <CAEFchVto>%s</CAEFchVto>
            <CbteDesde>%d</CbteDesde>
          </FECAEDetResponse>
        </FeDetResp>
      </FECAESolicitarResult>
    </FECAESolicitarResponse>
  </soapenv:Body>
</soapenv:Envelope>`, cae, fchVto, desde)
}

// feCAESolicitarFixtureRejected returns a canned SOAP FECAESolicitarResponse
// with Resultado=R and the provided error code and message.
func feCAESolicitarFixtureRejected(code, msg string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <FECAESolicitarResponse>
      <FECAESolicitarResult>
        <FeDetResp>
          <FECAEDetResponse>
            <Resultado>R</Resultado>
            <CAE></CAE>
            <CAEFchVto></CAEFchVto>
            <CbteDesde>1</CbteDesde>
            <Observaciones>
              <Obs>
                <Msg>%s</Msg>
              </Obs>
            </Observaciones>
          </FECAEDetResponse>
        </FeDetResp>
        <Errors>
          <Err>
            <Code>%s</Code>
            <Msg>%s</Msg>
          </Err>
        </Errors>
      </FECAESolicitarResult>
    </FECAESolicitarResponse>
  </soapenv:Body>
</soapenv:Envelope>`, msg, code, msg)
}

func TestWSFERequestCAE_Approved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, feCAESolicitarFixtureApproved("71000000000001", "20260630", 43))
	}))
	defer srv.Close()
	c := newTestWSFEClient(srv.URL)
	res, err := c.RequestCAE(context.Background(), WSFEPayload{CUIT: 20111111112, PointOfSale: 1,
		ReceiptTypeCode: 6, ConceptCode: 1, DocTypeCode: 99, DocNumber: 0,
		IssueDate:  time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
		TotalCents: 12100, NetCents: 10000, VATCents: 2100, CurrencyCode: "PES", CurrencyRate: 1}, 43)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.CAE != "71000000000001" || res.ReceiptNumber != 43 {
		t.Errorf("got %+v", res)
	}
	if res.CAEExpiresAt.Format("20060102") != "20260630" {
		t.Errorf("expiry: %v", res.CAEExpiresAt)
	}
}

func TestWSFERequestCAE_FacturaCNoIVABlock(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capturedBody = string(raw)
		io.WriteString(w, feCAESolicitarFixtureApproved("71000000000099", "20260630", 1))
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	_, err := c.RequestCAE(context.Background(), WSFEPayload{
		CUIT:            20111111112,
		PointOfSale:     1,
		ReceiptTypeCode: 11, // factura C / consumidor final
		ConceptCode:     1,
		DocTypeCode:     99,
		DocNumber:       0,
		IssueDate:       time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
		TotalCents:      12100,
		NetCents:        12100,
		VATCents:        0,
		CurrencyCode:    "PES",
		CurrencyRate:    1,
	}, 1)
	if err != nil {
		t.Fatalf("RequestCAE returned unexpected error: %v", err)
	}

	// IVA block must be absent — AFIP rejects factura C with an IVA array.
	if strings.Contains(capturedBody, "<ar:Iva>") {
		t.Errorf("<ar:Iva> block must be omitted for factura C (VATCents==0), but it was present in:\n%s", capturedBody)
	}

	// Positive control: <ar:ImpIVA> must still appear as 0.00.
	if !strings.Contains(capturedBody, "<ar:ImpIVA>0.00</ar:ImpIVA>") {
		t.Errorf("<ar:ImpIVA>0.00</ar:ImpIVA> must be present in the SOAP body, but was not found in:\n%s", capturedBody)
	}
}

func TestWSFERequestCAE_EmitsImpOpEx(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capturedBody = string(raw)
		io.WriteString(w, feCAESolicitarFixtureApproved("71000000000001", "20260630", 43))
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	_, err := c.RequestCAE(context.Background(), WSFEPayload{
		CUIT:            20111111112,
		PointOfSale:     1,
		ReceiptTypeCode: 6,
		ConceptCode:     1,
		DocTypeCode:     99,
		IssueDate:       time.Date(2026, 6, 19, 15, 0, 0, 0, time.UTC),
		TotalCents:      12550,
		NetCents:        11000,
		VATCents:        1050,
		OpExCents:       500,
		CurrencyCode:    "PES",
		CurrencyRate:    1,
		IVAAlicuotaID:   4,
	}, 43)
	if err != nil {
		t.Fatalf("RequestCAE: %v", err)
	}
	// 110.00 + 10.50 + 5.00 = 125.50. BaseImp/Importe follow net/VAT.
	for _, want := range []string{
		"<ar:ImpTotal>125.50</ar:ImpTotal>",
		"<ar:ImpNeto>110.00</ar:ImpNeto>",
		"<ar:ImpOpEx>5.00</ar:ImpOpEx>",
		"<ar:ImpIVA>10.50</ar:ImpIVA>",
		"<ar:Id>4</ar:Id>",
		"<ar:BaseImp>110.00</ar:BaseImp>",
		"<ar:Importe>10.50</ar:Importe>",
	} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("SOAP body missing %s:\n%s", want, capturedBody)
		}
	}
}

func TestWSFERequestCAE_CbteFchUsesArgentinaCivilDate(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capturedBody = string(raw)
		io.WriteString(w, feCAESolicitarFixtureApproved("71000000000001", "20260630", 1))
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	// 02:30 UTC on 10 Mar is 23:30 on 9 Mar in Argentina.
	_, err := c.RequestCAE(context.Background(), WSFEPayload{
		CUIT:            20111111112,
		PointOfSale:     1,
		ReceiptTypeCode: 6,
		ConceptCode:     1,
		DocTypeCode:     99,
		IssueDate:       time.Date(2026, 3, 10, 2, 30, 0, 0, time.UTC),
		TotalCents:      12100,
		NetCents:        10000,
		VATCents:        2100,
		CurrencyCode:    "PES",
		CurrencyRate:    1,
	}, 1)
	if err != nil {
		t.Fatalf("RequestCAE: %v", err)
	}
	if !strings.Contains(capturedBody, "<ar:CbteFch>20260309</ar:CbteFch>") {
		t.Errorf("CbteFch must be the Argentina civil date 20260309, body:\n%s", capturedBody)
	}
}

func TestWSFERequestCAE_Rejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, feCAESolicitarFixtureRejected("10015", "Factura ya autorizada"))
	}))
	defer srv.Close()
	c := newTestWSFEClient(srv.URL)
	_, err := c.RequestCAE(context.Background(), WSFEPayload{CUIT: 20111111112, PointOfSale: 1, ReceiptTypeCode: 6, ConceptCode: 1, DocTypeCode: 99, CurrencyCode: "PES", CurrencyRate: 1}, 1)
	if err == nil || !strings.Contains(err.Error(), "Factura ya autorizada") {
		t.Fatalf("want rejection error, got %v", err)
	}
}

func requestCAEForTest(t *testing.T, fixture string) error {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, fixture)
	}))
	defer srv.Close()
	c := newTestWSFEClient(srv.URL)
	_, err := c.RequestCAE(context.Background(), WSFEPayload{
		CUIT: 20111111112, PointOfSale: 1, ReceiptTypeCode: 6, ConceptCode: 1,
		DocTypeCode: 99, CurrencyCode: "PES", CurrencyRate: 1,
	}, 1)
	return err
}

// A definitive AFIP rejection (Resultado=R, plain validation error) must be
// classified PERMANENT so the worker fails fast instead of churning the retry
// budget re-submitting the same invoice.
func TestWSFERequestCAE_DefinitiveRejection_IsPermanent(t *testing.T) {
	err := requestCAEForTest(t, feCAESolicitarFixtureRejected("10048", "El campo DocNro es invalido"))
	if err == nil {
		t.Fatal("want rejection error, got nil")
	}
	if !errors.Is(err, fiscal.ErrPermanent) {
		t.Fatalf("definitive AFIP rejection must wrap fiscal.ErrPermanent, got %v", err)
	}
	// Operator visibility: AFIP code + message must survive in the error text.
	// 10016 is the sequence-number race and is always retryable, so this case
	// uses a different validation code.
	if !strings.Contains(err.Error(), "10048") || !strings.Contains(err.Error(), "El campo DocNro es invalido") {
		t.Errorf("error must carry AFIP code+message for operator visibility, got %q", err.Error())
	}
}

// EXCEPTION: even when Resultado=R, a transient/infra message (e.g. AFIP asking
// the caller to retry) must stay RETRYABLE (NOT wrapped) so the invoice is not
// abandoned over a recoverable outage.
func TestWSFERequestCAE_TransientRejection_IsRetryable(t *testing.T) {
	err := requestCAEForTest(t, feCAESolicitarFixtureRejected("10001", "El sistema no se encuentra disponible, intente nuevamente"))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if errors.Is(err, fiscal.ErrPermanent) {
		t.Fatalf("transient AFIP message must NOT be permanent, got %v", err)
	}
	if !strings.Contains(err.Error(), "intente nuevamente") {
		t.Errorf("error must carry the AFIP message, got %q", err.Error())
	}
}

// An ambiguous response (empty Resultado with an <Errors> block, e.g. a
// request-level/outage condition) must stay RETRYABLE — a false-permanent that
// abandons a recoverable invoice is worse than a few wasted retries.
func TestWSFERequestCAE_EmptyResultadoWithErrors_IsRetryable(t *testing.T) {
	fixture := `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <FECAESolicitarResponse>
      <FECAESolicitarResult>
        <Errors>
          <Err>
            <Code>600</Code>
            <Msg>ValidacionDeToken: Computador no autorizado</Msg>
          </Err>
        </Errors>
      </FECAESolicitarResult>
    </FECAESolicitarResponse>
  </soapenv:Body>
</soapenv:Envelope>`
	err := requestCAEForTest(t, fixture)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if errors.Is(err, fiscal.ErrPermanent) {
		t.Fatalf("empty-Resultado/ambiguous error must NOT be permanent, got %v", err)
	}
	if !strings.Contains(err.Error(), "600") || !strings.Contains(err.Error(), "Computador no autorizado") {
		t.Errorf("error must carry AFIP code+message, got %q", err.Error())
	}
}

func TestWSFECompConsultar_Approved(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capturedBody = string(raw)
		io.WriteString(w, feCompConsultarFixture("A", "71000000000001"))
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	resp, err := c.CompConsultar(context.Background(), 20111111112, 3, 11, 43)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Resultado != "A" || resp.CAE != "71000000000001" {
		t.Errorf("got %+v", resp)
	}

	// The request must carry the (CbteTipo, PtoVta, CbteNro) tuple AFIP needs.
	for _, want := range []string{
		"<ar:CbteTipo>11</ar:CbteTipo>",
		"<ar:CbteNro>43</ar:CbteNro>",
		"<ar:PtoVta>3</ar:PtoVta>",
		"<ar:Cuit>20111111112</ar:Cuit>",
	} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("CompConsultar SOAP body missing %q in:\n%s", want, capturedBody)
		}
	}
}

// feCompConsultarRichFixture returns a FECompConsultarResponse ResultGet block
// populated with the full authorization tuple AFIP returns for an authorized
// voucher (used to assert the enriched CompConsultar parse).
func feCompConsultarRichFixture() string {
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

func TestWSFECompConsultar_ParsesFullAuthorizationDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, feCompConsultarRichFixture())
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	resp, err := c.CompConsultar(context.Background(), 20111111112, 3, 11, 43)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Resultado != "A" {
		t.Errorf("Resultado: want A got %q", resp.Resultado)
	}
	if resp.CAE != "71000000000001" {
		t.Errorf("CAE: want 71000000000001 got %q", resp.CAE)
	}
	if resp.ReceiptNumber != 43 {
		t.Errorf("ReceiptNumber: want 43 got %d", resp.ReceiptNumber)
	}
	if resp.IssueDate.Format("20060102") != "20260619" {
		t.Errorf("IssueDate: want 20260619 got %v", resp.IssueDate)
	}
	if resp.CAEExpiresAt.Format("20060102") != "20260630" {
		t.Errorf("CAEExpiresAt: want 20260630 got %v", resp.CAEExpiresAt)
	}
	if resp.ImpTotal != "121.00" {
		t.Errorf("ImpTotal: want 121.00 got %q", resp.ImpTotal)
	}
	if resp.MonId != "PES" {
		t.Errorf("MonId: want PES got %q", resp.MonId)
	}
	if resp.MonCotiz != "1.000000" {
		t.Errorf("MonCotiz: want 1.000000 got %q", resp.MonCotiz)
	}
	if resp.DocTipo != 80 {
		t.Errorf("DocTipo: want 80 got %d", resp.DocTipo)
	}
	if resp.DocNro != 20111111112 {
		t.Errorf("DocNro: want 20111111112 got %d", resp.DocNro)
	}
}

func TestWSFECompConsultar_BadDatesDoNotFailCall(t *testing.T) {
	badDates := `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <FECompConsultarResponse>
      <FECompConsultarResult>
        <ResultGet>
          <Resultado>A</Resultado>
          <CodAutorizacion>71000000000001</CodAutorizacion>
          <CbteDesde>43</CbteDesde>
          <CbteFch>not-a-date</CbteFch>
          <FchVto></FchVto>
          <ImpTotal>121.00</ImpTotal>
          <MonId>PES</MonId>
          <MonCotiz>1.000000</MonCotiz>
        </ResultGet>
      </FECompConsultarResult>
    </FECompConsultarResponse>
  </soapenv:Body>
</soapenv:Envelope>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, badDates)
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	resp, err := c.CompConsultar(context.Background(), 20111111112, 3, 11, 43)
	if err != nil {
		t.Fatalf("a bad date must not fail the whole call, got err: %v", err)
	}
	if resp.Resultado != "A" || resp.CAE != "71000000000001" {
		t.Errorf("got %+v", resp)
	}
	if !resp.IssueDate.IsZero() {
		t.Errorf("unparseable CbteFch must leave IssueDate zero, got %v", resp.IssueDate)
	}
	if !resp.CAEExpiresAt.IsZero() {
		t.Errorf("empty FchVto must leave CAEExpiresAt zero, got %v", resp.CAEExpiresAt)
	}
}

func TestWSFECompConsultar_NotFoundCarriesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, feCompConsultarNotFoundFixture())
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	resp, err := c.CompConsultar(context.Background(), 20111111112, 3, 11, 999)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Resultado != "" {
		t.Errorf("want empty Resultado for not-found, got %q", resp.Resultado)
	}
	if len(resp.Errors) == 0 || resp.Errors[0].Code != 602 {
		t.Errorf("want AFIP error 602, got %+v", resp.Errors)
	}
}

func TestWSFECompConsultar_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	_, err := c.CompConsultar(context.Background(), 20111111112, 3, 11, 43)
	if err == nil {
		t.Fatal("expected error for non-200 HTTP response, got nil")
	}
}

func TestWSFEEndpointFor(t *testing.T) {
	if got := wsfeEndpoint("production"); got != "https://servicios1.afip.gov.ar/wsfev1/service.asmx" {
		t.Errorf("prod: %s", got)
	}
	if got := wsfeEndpoint("sandbox"); got != "https://wswhomo.afip.gov.ar/wsfev1/service.asmx" {
		t.Errorf("homo: %s", got)
	}
}

func TestWSAAEndpointFor(t *testing.T) {
	if got := wsaaEndpoint("production"); got != "https://wsaa.afip.gov.ar/ws/services/LoginCms" {
		t.Errorf("prod: %s", got)
	}
	if got := wsaaEndpoint("sandbox"); got != "https://wsaahomo.afip.gov.ar/ws/services/LoginCms" {
		t.Errorf("homo: %s", got)
	}
}

func TestWSFEEndpointEnvOverride(t *testing.T) {
	t.Setenv("FISCAL_WSFE_URL", "http://x")
	if got := wsfeEndpoint("production"); got != wsfeProdURL {
		t.Errorf("production must ignore override, got %s", got)
	}
	if got := wsfeEndpoint("sandbox"); got != "http://x" {
		t.Errorf("sandbox override: %s", got)
	}
}

func TestWSAAEndpointEnvOverride(t *testing.T) {
	t.Setenv("FISCAL_WSAA_URL", "http://x")
	if got := wsaaEndpoint("production"); got != wsaaProdURL {
		t.Errorf("production must ignore override, got %s", got)
	}
	if got := wsaaEndpoint("sandbox"); got != "http://x" {
		t.Errorf("sandbox override: %s", got)
	}
}
