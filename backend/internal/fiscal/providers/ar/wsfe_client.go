package ar

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

// compile-time assertion: wsfeClient must satisfy WSFEClient.
var _ WSFEClient = (*wsfeClient)(nil)

const (
	wsfeProdURL = "https://servicios1.afip.gov.ar/wsfev1/service.asmx"
	wsfeHomoURL = "https://wswhomo.afip.gov.ar/wsfev1/service.asmx"
	wsaaProdURL = "https://wsaa.afip.gov.ar/ws/services/LoginCms"
	wsaaHomoURL = "https://wsaahomo.afip.gov.ar/ws/services/LoginCms"
)

// wsfeEndpoint returns the WSFE service URL for the given environment.
// Production always uses the official AFIP host. FISCAL_WSFE_URL overrides
// the URL only for non-production environments (tests and homologación).
func wsfeEndpoint(env string) string {
	if env == "production" {
		return wsfeProdURL
	}
	if v := os.Getenv("FISCAL_WSFE_URL"); v != "" {
		return v
	}
	return wsfeHomoURL
}

// wsaaEndpoint returns the WSAA service URL for the given environment.
// Production always uses the official AFIP host. FISCAL_WSAA_URL overrides
// the URL only for non-production environments (tests and homologación).
func wsaaEndpoint(env string) string {
	if env == "production" {
		return wsaaProdURL
	}
	if v := os.Getenv("FISCAL_WSAA_URL"); v != "" {
		return v
	}
	return wsaaHomoURL
}

// authProvider is the auth dependency consumed by the WSFE client.
// WSAAClient already satisfies this interface.
type authProvider interface {
	Authenticate(ctx context.Context) (*WSAACredentials, error)
}

// wsfeClient is the concrete implementation that satisfies the WSFEClient interface
// declared in provider.go. It communicates with AFIP's WSFEv1 SOAP service.
type wsfeClient struct {
	endpoint string
	auth     authProvider
	http     *http.Client
}

// NewWSFEClient constructs a WSFEClient that will call the given WSFE endpoint URL.
// auth must supply a valid token/sign pair (typically a *WSAAClient).
func NewWSFEClient(endpoint string, auth authProvider) WSFEClient {
	return &wsfeClient{
		endpoint: endpoint,
		auth:     auth,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

// feAuthHeader is the common <Auth> block required by every WSFE operation.
const feAuthHeader = `<ar:Auth><ar:Token>%s</ar:Token><ar:Sign>%s</ar:Sign><ar:Cuit>%d</ar:Cuit></ar:Auth>`

// call sends a SOAP request to the WSFE endpoint and returns the raw response body.
// soapAction is the unqualified operation name (e.g. "FECompUltimoAutorizado").
func (c *wsfeClient) call(ctx context.Context, soapAction, body string) ([]byte, error) {
	envelope := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ar="http://ar.gov.afip.dif.FEV1/">` +
		`<soapenv:Body>` + body + `</soapenv:Body></soapenv:Envelope>`

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBufferString(envelope))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", "http://ar.gov.afip.dif.FEV1/"+soapAction)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wsfe %s: %w", soapAction, err)
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("wsfe %s read body: %w", soapAction, readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wsfe %s HTTP %d: %s", soapAction, resp.StatusCode, truncateForError(raw, 200))
	}
	return raw, nil
}

// Dummy calls FEDummy, the unauthenticated WSFE health check. AFIP returns the
// status of its three internal servers; all three must be "OK" for the service
// to be usable. FEDummy carries no <Auth> block (it requires no token/sign).
func (c *wsfeClient) Dummy(ctx context.Context) error {
	raw, err := c.call(ctx, "FEDummy", `<ar:FEDummy></ar:FEDummy>`)
	if err != nil {
		return err
	}

	var out struct {
		AppServer  string `xml:"Body>FEDummyResponse>FEDummyResult>AppServer"`
		DbServer   string `xml:"Body>FEDummyResponse>FEDummyResult>DbServer"`
		AuthServer string `xml:"Body>FEDummyResponse>FEDummyResult>AuthServer"`
	}
	if err := xml.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("unmarshal dummy: %w", err)
	}

	for _, s := range []struct {
		name, value string
	}{
		{"AppServer", out.AppServer},
		{"DbServer", out.DbServer},
		{"AuthServer", out.AuthServer},
	} {
		if !strings.EqualFold(strings.TrimSpace(s.value), "OK") {
			return fmt.Errorf("wsfe FEDummy: %s is not OK (got %q)", s.name, strings.TrimSpace(s.value))
		}
	}
	return nil
}

// Authenticate forces a WSAA login (via the cached auth provider) without issuing
// an invoice, so settings validation can confirm the cert/key are accepted by
// AFIP. The returned credentials are discarded; only success/failure matters.
// The WSAA client caches its token, so this is cheap to call.
func (c *wsfeClient) Authenticate(ctx context.Context) error {
	_, err := c.auth.Authenticate(ctx)
	return err
}

// LastAuthorized calls FECompUltimoAutorizado and returns the last authorized
// receipt number for the given point-of-sale and receipt type.
func (c *wsfeClient) LastAuthorized(ctx context.Context, payload WSFEPayload) (int64, error) {
	creds, err := c.auth.Authenticate(ctx)
	if err != nil {
		return 0, err
	}

	body := fmt.Sprintf(
		`<ar:FECompUltimoAutorizado>`+feAuthHeader+
			`<ar:PtoVta>%d</ar:PtoVta><ar:CbteTipo>%d</ar:CbteTipo></ar:FECompUltimoAutorizado>`,
		creds.Token, creds.Sign, payload.CUIT,
		payload.PointOfSale, payload.ReceiptTypeCode,
	)

	raw, err := c.call(ctx, "FECompUltimoAutorizado", body)
	if err != nil {
		return 0, err
	}

	var out struct {
		CbteNro int64  `xml:"Body>FECompUltimoAutorizadoResponse>FECompUltimoAutorizadoResult>CbteNro"`
		ErrMsg  string `xml:"Body>FECompUltimoAutorizadoResponse>FECompUltimoAutorizadoResult>Errors>Err>Msg"`
	}
	if err := xml.Unmarshal(raw, &out); err != nil {
		return 0, fmt.Errorf("unmarshal last-authorized: %w", err)
	}
	if out.ErrMsg != "" {
		return 0, fmt.Errorf("wsfe error: %s", out.ErrMsg)
	}
	return out.CbteNro, nil
}

// RequestCAE calls FECAESolicitar to request a CAE (Código de Autorización Electrónico)
// for the given invoice. Money amounts are sent as AFIP decimals (cents / 100, formatted
// as "%.2f"). ImpTotal is ImpNeto + ImpIVA + ImpOpEx. An IVA array is included when
// VATCents > 0; its alícuota id comes from the payload. CbteFch is the Argentina
// civil date of IssueDate.
func (c *wsfeClient) RequestCAE(ctx context.Context, p WSFEPayload, nextNumber int64) (*CAEResponse, error) {
	creds, err := c.auth.Authenticate(ctx)
	if err != nil {
		return nil, err
	}

	ivaXML := ""
	if p.VATCents > 0 {
		// Alícuota id is derived from the VAT rate by the mapper (F-ALICUOTA); fall
		// back to 5 (21%) if an older/empty payload left it unset.
		alicuotaID := p.IVAAlicuotaID
		if alicuotaID == 0 {
			alicuotaID = 5
		}
		ivaXML = fmt.Sprintf(
			`<ar:Iva><ar:AlicIva><ar:Id>%d</ar:Id><ar:BaseImp>%.2f</ar:BaseImp><ar:Importe>%.2f</ar:Importe></ar:AlicIva></ar:Iva>`,
			alicuotaID, float64(p.NetCents)/100, float64(p.VATCents)/100,
		)
	}

	// CbtesAsoc is required for credit notes (notas de crédito) to reference
	// the original authorized receipt. It is omitted for regular receipts
	// (factura A/B/C) where AssocNumber is zero.
	cbtesAsocXML := ""
	if p.AssocNumber > 0 {
		cbtesAsocXML = fmt.Sprintf(
			`<ar:CbtesAsoc><ar:CbteAsoc><ar:Tipo>%d</ar:Tipo><ar:PtoVta>%d</ar:PtoVta><ar:Nro>%d</ar:Nro></ar:CbteAsoc></ar:CbtesAsoc>`,
			p.AssocTypeCode, p.AssocPointOfSale, p.AssocNumber,
		)
	}

	// CondicionIVAReceptorId (RG 5616) is mandatory on FECAESolicitar since 2025.
	// Default to consumidor final (5) if the mapper left it zero so we never
	// omit the element on a real provider=arca call.
	condicionID := p.CondicionIVAReceptorId
	if condicionID == 0 {
		condicionID = CondicionIVAReceptorConsumidorFinal
	}

	det := fmt.Sprintf(
		`<ar:FECAEDetRequest>`+
			`<ar:Concepto>%d</ar:Concepto><ar:DocTipo>%d</ar:DocTipo><ar:DocNro>%d</ar:DocNro>`+
			`<ar:CbteDesde>%d</ar:CbteDesde><ar:CbteHasta>%d</ar:CbteHasta><ar:CbteFch>%s</ar:CbteFch>`+
			`<ar:ImpTotal>%.2f</ar:ImpTotal><ar:ImpTotConc>0.00</ar:ImpTotConc><ar:ImpNeto>%.2f</ar:ImpNeto>`+
			`<ar:ImpOpEx>%.2f</ar:ImpOpEx><ar:ImpTrib>0.00</ar:ImpTrib><ar:ImpIVA>%.2f</ar:ImpIVA>`+
			`<ar:MonId>%s</ar:MonId><ar:MonCotiz>%.6f</ar:MonCotiz>`+
			`<ar:CondicionIVAReceptorId>%d</ar:CondicionIVAReceptorId>%s%s</ar:FECAEDetRequest>`,
		p.ConceptCode, p.DocTypeCode, p.DocNumber,
		nextNumber, nextNumber,
		p.IssueDate.In(argentinaLocation).Format("20060102"),
		float64(p.TotalCents)/100, float64(p.NetCents)/100, float64(p.OpExCents)/100, float64(p.VATCents)/100,
		p.CurrencyCode, p.CurrencyRate, condicionID, ivaXML, cbtesAsocXML,
	)

	body := fmt.Sprintf(
		`<ar:FECAESolicitar>`+feAuthHeader+
			`<ar:FeCAEReq><ar:FeCabReq>`+
			`<ar:CantReg>1</ar:CantReg><ar:PtoVta>%d</ar:PtoVta><ar:CbteTipo>%d</ar:CbteTipo>`+
			`</ar:FeCabReq><ar:FeDetReq>%s</ar:FeDetReq></ar:FeCAEReq></ar:FECAESolicitar>`,
		creds.Token, creds.Sign, p.CUIT,
		p.PointOfSale, p.ReceiptTypeCode, det,
	)

	raw, err := c.call(ctx, "FECAESolicitar", body)
	if err != nil {
		return nil, err
	}

	var out struct {
		Resultado string        `xml:"Body>FECAESolicitarResponse>FECAESolicitarResult>FeDetResp>FECAEDetResponse>Resultado"`
		CAE       string        `xml:"Body>FECAESolicitarResponse>FECAESolicitarResult>FeDetResp>FECAEDetResponse>CAE"`
		CAEFchVto string        `xml:"Body>FECAESolicitarResponse>FECAESolicitarResult>FeDetResp>FECAEDetResponse>CAEFchVto"`
		CbteDesde int64         `xml:"Body>FECAESolicitarResponse>FECAESolicitarResult>FeDetResp>FECAEDetResponse>CbteDesde"`
		Obs       []afipMessage `xml:"Body>FECAESolicitarResponse>FECAESolicitarResult>FeDetResp>FECAEDetResponse>Observaciones>Obs"`
		Errs      []afipMessage `xml:"Body>FECAESolicitarResponse>FECAESolicitarResult>Errors>Err"`
	}
	if err := xml.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unmarshal cae: %w", err)
	}
	if out.Resultado != "A" || out.CAE == "" {
		// Keep AFIP code + message in the error text for operator visibility.
		// AFIP codes/messages are diagnostic, never credential material.
		detail := formatAFIPMessages(out.Errs, out.Obs)
		baseErr := fmt.Errorf("CAE not approved (%s): %s", out.Resultado, detail)
		// A definitive AFIP rejection (Resultado=R, e.g. an invalid field) will
		// never succeed on retry — classify it permanent so the worker fails
		// fast instead of churning the retry budget re-submitting the same
		// invoice. Empty/ambiguous Resultado (request-level or transport-ish
		// failures surfaced via <Errors>) and AFIP messages that signal a
		// temporary outage stay retryable.
		if afipRejectionIsPermanent(out.Resultado, out.Errs, out.Obs) {
			return nil, fmt.Errorf("%w: %v", fiscal.ErrPermanent, baseErr)
		}
		return nil, baseErr
	}
	exp, err := time.Parse("20060102", out.CAEFchVto)
	if err != nil {
		return nil, fmt.Errorf("parse CAEFchVto %q: %w", out.CAEFchVto, err)
	}
	return &CAEResponse{ReceiptNumber: out.CbteDesde, CAE: out.CAE, CAEExpiresAt: exp}, nil
}

// afipMessage is a single AFIP <Err>/<Obs> entry: a numeric code plus a
// human-readable message. Both are diagnostic, never credential material.
type afipMessage struct {
	Code int    `xml:"Code"`
	Msg  string `xml:"Msg"`
}

// formatAFIPMessages renders AFIP errors then observations as "[code] message"
// (or just the message when no code is present) joined by "; ", so the operator
// sees the AFIP diagnostic code alongside the text.
func formatAFIPMessages(errs, obs []afipMessage) string {
	parts := make([]string, 0, len(errs)+len(obs))
	for _, m := range append(append([]afipMessage{}, errs...), obs...) {
		switch {
		case m.Code != 0 && m.Msg != "":
			parts = append(parts, fmt.Sprintf("[%d] %s", m.Code, m.Msg))
		case m.Code != 0:
			parts = append(parts, fmt.Sprintf("[%d]", m.Code))
		case m.Msg != "":
			parts = append(parts, m.Msg)
		}
	}
	return strings.Join(parts, "; ")
}

// afipTransientMessages lists case-insensitive substrings that mark an AFIP
// condition as temporary/infrastructural — the same invoice may succeed on a
// later attempt, so it must stay retryable even when AFIP returned Resultado=R.
var afipTransientMessages = []string{
	"intente nuevamente",
	"no se encuentra disponible",
	"no disponible",
	"temporal",
	"timeout",
	"time out",
	"timed out",
	"reintente",
}

// afipRecoverableRejectionMessages lists case-insensitive substrings for AFIP
// Resultado=R rejections a RETRY can fix by recomputing the next sequence number.
// The chief case is the non-correlative rejection two same-series issuances can
// race into (worker/attempt A takes number N; B's request for N is rejected "no
// es correlativo"; on retry B recomputes LastAuthorized+1 and succeeds). These are
// NOT infrastructural like afipTransientMessages, but permanently failing them
// abandons an invoice a retry would authorize, so they must stay retryable.
var afipRecoverableRejectionMessages = []string{
	"correlativo", // "El campo CbteDesde ... no es correlativo al último comprobante autorizado"
}

// afipRetryableCodes are AFIP error/observation codes that stay retryable
// regardless of message text. 10016 means CbteDesde is not the next number to
// authorize — a same-series race a retry re-sequences past, even when the
// message omits the word "correlativo".
var afipRetryableCodes = map[int]bool{10016: true}

// afipRejectionIsPermanent decides whether a non-approved FECAESolicitar result
// should be classified as a permanent failure (no point retrying):
//
//   - Resultado == "R": a definitive AFIP rejection (e.g. invalid field, doc
//     mismatch). Permanent, UNLESS any error/observation code is in
//     afipRetryableCodes (10016 is always retryable, regardless of message
//     text), OR any message matches the transient allowlist (AFIP occasionally
//     rejects with an "intente nuevamente"-style infra message) OR the
//     recoverable-rejection allowlist (a non-correlative sequence-number
//     rejection a retry re-sequences past).
//   - Any other Resultado (notably empty, surfaced via a request-level <Errors>
//     block, or an ambiguous/partial response): treated as retryable. A
//     false-permanent that abandons a recoverable invoice is worse than a few
//     wasted retries.
func afipRejectionIsPermanent(resultado string, errs, obs []afipMessage) bool {
	if resultado != "R" {
		return false
	}
	for _, m := range append(append([]afipMessage{}, errs...), obs...) {
		if afipRetryableCodes[m.Code] {
			return false
		}
		lower := strings.ToLower(m.Msg)
		for _, t := range afipTransientMessages {
			if strings.Contains(lower, t) {
				return false
			}
		}
		for _, t := range afipRecoverableRejectionMessages {
			if strings.Contains(lower, t) {
				return false
			}
		}
	}
	return true
}

// CompConsultarResponse is the parsed result of a FECompConsultar query: AFIP's
// authorization state ("A"/"R") of an already-submitted voucher, its CAE
// (CodAutorizacion), the full authorization tuple from ResultGet, and any error
// entries (e.g. code 602 "comprobante inexistente" when the voucher was never
// authorized).
//
// The detail fields (ReceiptNumber, IssueDate, CAEExpiresAt, ImpTotal, MonId,
// MonCotiz, DocTipo, DocNro) let a reconcile rebuild a receipt row + QR whose
// original issue response was lost. The CAE is not secret, so all of this is
// safe to carry. Date fields are parsed defensively: an unparseable CbteFch /
// FchVto leaves the corresponding time zero and does NOT fail the call.
type CompConsultarResponse struct {
	Resultado     string
	CAE           string
	ReceiptNumber int64     // CbteDesde — the authorized receipt number
	IssueDate     time.Time // parsed from CbteFch ("YYYYMMDD"); zero if unparseable
	CAEExpiresAt  time.Time // parsed from FchVto ("YYYYMMDD"); zero if unparseable
	ImpTotal      string    // raw decimal string, e.g. "121.00"
	MonId         string    // currency code, e.g. "PES"
	MonCotiz      string    // raw exchange-rate decimal string, e.g. "1.000000"
	DocTipo       int       // recipient document type code
	DocNro        int64     // recipient document number
	Errors        []struct {
		Code int
		Msg  string
	}
}

// CompConsultar calls FECompConsultar to query the authorization state of a
// voucher already submitted to AFIP, identified by (CbteTipo, PtoVta, CbteNro).
// It is used by status reconciliation (GetStatus) to confirm whether a CAE was
// granted. Errors carry only AFIP/transport detail, never credential material.
func (c *wsfeClient) CompConsultar(ctx context.Context, cuit, ptoVta, cbteTipo, cbteNro int64) (*CompConsultarResponse, error) {
	creds, err := c.auth.Authenticate(ctx)
	if err != nil {
		return nil, err
	}

	body := fmt.Sprintf(
		`<ar:FECompConsultar>`+feAuthHeader+
			`<ar:FeCompConsReq><ar:CbteTipo>%d</ar:CbteTipo><ar:CbteNro>%d</ar:CbteNro><ar:PtoVta>%d</ar:PtoVta></ar:FeCompConsReq></ar:FECompConsultar>`,
		creds.Token, creds.Sign, cuit,
		cbteTipo, cbteNro, ptoVta,
	)

	raw, err := c.call(ctx, "FECompConsultar", body)
	if err != nil {
		return nil, err
	}

	var out struct {
		Resultado string `xml:"Body>FECompConsultarResponse>FECompConsultarResult>ResultGet>Resultado"`
		CAE       string `xml:"Body>FECompConsultarResponse>FECompConsultarResult>ResultGet>CodAutorizacion"`
		CbteDesde int64  `xml:"Body>FECompConsultarResponse>FECompConsultarResult>ResultGet>CbteDesde"`
		CbteFch   string `xml:"Body>FECompConsultarResponse>FECompConsultarResult>ResultGet>CbteFch"`
		FchVto    string `xml:"Body>FECompConsultarResponse>FECompConsultarResult>ResultGet>FchVto"`
		ImpTotal  string `xml:"Body>FECompConsultarResponse>FECompConsultarResult>ResultGet>ImpTotal"`
		MonId     string `xml:"Body>FECompConsultarResponse>FECompConsultarResult>ResultGet>MonId"`
		MonCotiz  string `xml:"Body>FECompConsultarResponse>FECompConsultarResult>ResultGet>MonCotiz"`
		DocTipo   int    `xml:"Body>FECompConsultarResponse>FECompConsultarResult>ResultGet>DocTipo"`
		DocNro    int64  `xml:"Body>FECompConsultarResponse>FECompConsultarResult>ResultGet>DocNro"`
		Errs      []struct {
			Code int    `xml:"Code"`
			Msg  string `xml:"Msg"`
		} `xml:"Body>FECompConsultarResponse>FECompConsultarResult>Errors>Err"`
	}
	if err := xml.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unmarshal comp-consultar: %w", err)
	}

	resp := &CompConsultarResponse{
		Resultado:     out.Resultado,
		CAE:           out.CAE,
		ReceiptNumber: out.CbteDesde,
		ImpTotal:      strings.TrimSpace(out.ImpTotal),
		MonId:         strings.TrimSpace(out.MonId),
		MonCotiz:      strings.TrimSpace(out.MonCotiz),
		DocTipo:       out.DocTipo,
		DocNro:        out.DocNro,
	}
	// Parse dates defensively: a malformed/empty value must NOT fail the whole
	// reconcile — leave the time zero and continue. CbteFch is an Argentina
	// civil date, not a UTC instant; parsing it in ART keeps a later QR
	// "fecha" (formatted with argentinaLocation) on the same calendar day.
	if t, perr := time.ParseInLocation("20060102", strings.TrimSpace(out.CbteFch), argentinaLocation); perr == nil {
		resp.IssueDate = t
	}
	if t, perr := time.Parse("20060102", strings.TrimSpace(out.FchVto)); perr == nil {
		resp.CAEExpiresAt = t
	}
	for _, e := range out.Errs {
		resp.Errors = append(resp.Errors, struct {
			Code int
			Msg  string
		}{Code: e.Code, Msg: e.Msg})
	}
	return resp, nil
}
