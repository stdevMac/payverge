package ar

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

var _ fiscal.Provider = (*Provider)(nil)

type WSFEClient interface {
	// Dummy runs the unauthenticated FEDummy health check and returns an error
	// when any AFIP server is down.
	Dummy(ctx context.Context) error
	// Authenticate forces a WSAA login round-trip (no invoice issued) so settings
	// validation can confirm the cert/key are accepted by AFIP.
	Authenticate(ctx context.Context) error
	LastAuthorized(ctx context.Context, payload WSFEPayload) (int64, error)
	RequestCAE(ctx context.Context, payload WSFEPayload, nextNumber int64) (*CAEResponse, error)
	// CompConsultar queries the authorization state of an already-submitted
	// voucher identified by (CbteTipo, PtoVta, CbteNro). Used by GetStatus.
	CompConsultar(ctx context.Context, cuit, ptoVta, cbteTipo, cbteNro int64) (*CompConsultarResponse, error)
}

type CAEResponse struct {
	ReceiptNumber int64
	CAE           string
	CAEExpiresAt  time.Time
}

type Provider struct {
	client WSFEClient
	// cuit is the issuer's CUIT (11-digit tax ID as int64). It is required to
	// build the WSFE <Auth> header for GetStatus, which has no WSFE payload to
	// derive it from (unlike IssueReceipt, where the CUIT comes off the input).
	cuit int64
}

func NewProvider(client WSFEClient, cuit int64) *Provider {
	return &Provider{client: client, cuit: cuit}
}

func (p *Provider) Country() string { return "AR" }

func (p *Provider) Name() string { return "arca" }

// ValidateSettings performs a live AFIP round-trip to confirm the business can
// issue invoices: the CUIT/point-of-sale are well-formed, the WSFE service is up
// (FEDummy), and the operator's cert/key are accepted by WSAA. It issues no
// invoice. Returned errors carry only AFIP/transport detail — never credential
// bytes.
func (p *Provider) ValidateSettings(ctx context.Context, settings fiscal.Settings) error {
	if p.client == nil {
		return errors.New("wsfe client is required")
	}
	if _, err := parseFixedDigits(settings.TaxID, 11); err != nil {
		return fmt.Errorf("CUIT must be exactly 11 digits: %w", err)
	}
	if settings.PointOfSale == nil || *settings.PointOfSale <= 0 {
		return errors.New("point of sale must be a positive number")
	}
	if err := p.client.Dummy(ctx); err != nil {
		return err
	}
	if err := p.client.Authenticate(ctx); err != nil {
		return err
	}
	return nil
}

func (p *Provider) IssueReceipt(ctx context.Context, input fiscal.IssueInput) (*fiscal.ReceiptResult, error) {
	if p.client == nil {
		return nil, errors.New("wsfe client is required")
	}
	payload, err := MapIssueInputToWSFE(input)
	if err != nil {
		return nil, err
	}
	caeRaw, number, expiresAt, adoptedDate, err := p.requestCAEWithReconcile(ctx, payload, input.AttemptedProviderReceiptID, input.OnAttempt, input.LockSeries, input.VoucherClaimed)
	if err != nil {
		return nil, err
	}
	return p.buildAuthorizedResult(payload, input.ReceiptType, input.IssuedAt, adoptedDate, caeRaw, number, expiresAt)
}

func (p *Provider) IssueCreditNote(ctx context.Context, input fiscal.CreditNoteInput) (*fiscal.ReceiptResult, error) {
	if p.client == nil {
		return nil, errors.New("wsfe client is required")
	}
	payload, err := MapCreditNoteToWSFE(input)
	if err != nil {
		return nil, err
	}
	caeRaw, number, expiresAt, adoptedDate, err := p.requestCAEWithReconcile(ctx, payload, input.AttemptedProviderReceiptID, input.OnAttempt, input.LockSeries, input.VoucherClaimed)
	if err != nil {
		return nil, err
	}
	// Derive the credit note receipt type string from the original.
	cnReceiptType := creditNoteReceiptType(input.OriginalReceipt.ReceiptType)
	return p.buildAuthorizedResult(payload, cnReceiptType, input.IssuedAt, adoptedDate, caeRaw, number, expiresAt)
}

// requestCAEWithReconcile requests a CAE and guards the lost-response
// duplicate-invoice window.
//
// Persisted-attempt contract: attempted is the "<type>-<pos>-<number>" tuple
// a previous attempt of this same job already sent (empty if none). When it
// decodes to this payload's receipt type and point of sale, FECompConsultar
// is called for that exact number N and LastAuthorized/RequestCAE are not
// used until the consult settles:
//   - a consult error or a nil response is retryable ("reconcile of previously
//     attempted voucher N failed") — AFIP may have committed N, so a new
//     number must not be allocated;
//   - Resultado "A" with a CAE: if voucherClaimed reports that an authorized
//     receipt stored for another job already holds the attempted id, N was
//     taken after this job released the series lock. It is not adopted and
//     it is not a permanent failure; allocation continues at
//     LastAuthorized+1. A voucherClaimed error is retryable ("check attempted
//     voucher N ownership") and does not wrap fiscal.ErrPermanent. A nil
//     voucherClaimed skips that check. Otherwise the CAE is adopted when
//     ImpTotal is missing or its cents match payload.TotalCents. A parsed
//     total that differs returns fiscal.ErrPermanent ("reconcile manually"):
//     N is this job's own submission, and allocating another number would
//     leave two authorized facturas for one bill;
//   - any other result (not authorized / not found) falls through.
//
// A decode failure is a retryable error (fail closed); a type/point-of-sale
// mismatch consults the attempted tuple itself.
//
// Normal allocation reads LastAuthorized, calls onAttempt with the encoded
// next number, and only then calls RequestCAE. If onAttempt returns an error
// the request is not sent and the error is retryable (it does not wrap
// fiscal.ErrPermanent). On RequestCAE failure:
//   - a deterministic AFIP rejection (errors.Is(err, fiscal.ErrPermanent)) is
//     propagated unchanged;
//   - a transient error triggers FECompConsultar(N). Resultado "A" with a CAE
//     is adopted. Otherwise the original transient error is returned so the
//     worker retries the same number.
//
// When lockSeries is non-nil it is taken first, before the attempted-reconcile
// CompConsultar and before LastAuthorized, on the key
// "arca:<CUIT>:<PtoVta>:<CbteTipo>". Release runs when this function returns,
// so one series stays locked across the authority round-trips and concurrent
// workers/replicas cannot race LastAuthorized against RequestCAE. A lock
// error is retryable ("lock fiscal series: ...") and does not wrap
// fiscal.ErrPermanent. A nil lockSeries keeps the single-worker assumption:
// the lost-response window is closed for one worker, and concurrent
// same-series issuance is not serialized.
func (p *Provider) requestCAEWithReconcile(ctx context.Context, payload *WSFEPayload, attempted string, onAttempt func(providerReceiptID string) error, lockSeries func(ctx context.Context, seriesKey string) (release func(), err error), voucherClaimed func(providerReceiptID string) (bool, error)) (caeRaw string, number int64, expiresAt, adoptedIssueDate time.Time, err error) {
	if lockSeries != nil {
		release, lerr := lockSeries(ctx, fmt.Sprintf("arca:%d:%d:%d", payload.CUIT, payload.PointOfSale, payload.ReceiptTypeCode))
		if lerr != nil {
			// %v, not %w: a lock failure must stay retryable even if the
			// callback wrapped fiscal.ErrPermanent.
			return "", 0, time.Time{}, time.Time{}, fmt.Errorf("lock fiscal series: %v", lerr)
		}
		defer release()
	}
	// attemptedSameSeries is true when a recorded attempt belongs to this
	// payload's series, so LastAuthorized is comparable to its number.
	attemptedSameSeries := false
	var attemptedNumber int64
	claimedByOther := false
	if attempted != "" {
		// Consult the DECODED (type, pos, N) of the recorded attempt, never the
		// current payload's: the bill or settings may have changed since.
		typeCode, pos, n, derr := decodeProviderReceiptID(attempted)
		if derr != nil {
			// Fail closed. Ignoring an unreadable attempt would allocate a new
			// number while the old one may be authorized. %v, not %w, keeps it
			// retryable.
			return "", 0, time.Time{}, time.Time{}, fmt.Errorf("recorded attempted voucher %q is unreadable: %v", attempted, derr)
		}
		sameSeries := typeCode == int64(payload.ReceiptTypeCode) && pos == int64(payload.PointOfSale)
		recon, cerr := p.client.CompConsultar(ctx, payload.CUIT, pos, typeCode, n)
		if cerr != nil || recon == nil {
			if cerr == nil {
				cerr = errors.New("empty reconcile response")
			}
			return "", 0, time.Time{}, time.Time{}, fmt.Errorf("reconcile of previously attempted voucher %d failed: %w", n, cerr)
		}
		// AFIP code 602 means "no data for these parameters": the voucher was
		// never authorized. Any other reported error leaves N undetermined.
		for _, e := range recon.Errors {
			if e.Code != afipVoucherNotFoundCode {
				return "", 0, time.Time{}, time.Time{}, fmt.Errorf(
					"reconcile of previously attempted voucher %d reported AFIP error %d: %s", n, e.Code, e.Msg)
			}
		}
		if strings.ToUpper(strings.TrimSpace(recon.Resultado)) == "A" && strings.TrimSpace(recon.CAE) != "" {
			// Another job can take N after this one records the attempt,
			// loses the response, sees "not authorized", and releases the
			// series lock. Adopting that CAE (or stopping forever) would
			// attach B's factura to A's bill. Fall through and allocate
			// LastAuthorized+1 instead.
			if voucherClaimed != nil {
				claimed, cerr := voucherClaimed(attempted)
				if cerr != nil {
					// %v, not %w: an ownership-check failure must stay
					// retryable even if the callback wrapped fiscal.ErrPermanent.
					return "", 0, time.Time{}, time.Time{}, fmt.Errorf("check attempted voucher %d ownership: %v", n, cerr)
				}
				claimedByOther = claimed
			}
			if !claimedByOther {
				// The attempted number was recorded under the series lock, so
				// this voucher is this job's own submission. If the bill
				// changed since then (total or series), allocating a new number
				// would leave the bill with two authorized facturas and voucher
				// n unstored. Stop for manual reconciliation instead.
				if !sameSeries {
					return "", 0, time.Time{}, time.Time{}, fmt.Errorf(
						"previously attempted voucher %s is authorized in a different series than this job now targets; reconcile manually: %w",
						attempted, fiscal.ErrPermanent)
				}
				if v, perr := strconv.ParseFloat(strings.TrimSpace(recon.ImpTotal), 64); perr == nil {
					if sent := int64(math.Round(v * 100)); sent != payload.TotalCents {
						return "", 0, time.Time{}, time.Time{}, fmt.Errorf(
							"previously attempted voucher %d is authorized for %d cents but the job now totals %d cents; reconcile manually: %w",
							n, sent, payload.TotalCents, fiscal.ErrPermanent)
					}
				}
				adoptedNumber := n
				if recon.ReceiptNumber > 0 {
					adoptedNumber = recon.ReceiptNumber
				}
				return strings.TrimSpace(recon.CAE), adoptedNumber, recon.CAEExpiresAt, recon.IssueDate, nil
			}
		}
		attemptedSameSeries = sameSeries
		attemptedNumber = n
	}

	last, lerr := p.client.LastAuthorized(ctx, *payload)
	if lerr != nil {
		return "", 0, time.Time{}, time.Time{}, lerr
	}
	// The consult did not show N authorized, yet AFIP's last number has reached
	// it: the consult and LastAuthorized disagree (AFIP lag, or N was taken
	// between the calls). Allocating last+1 could abandon a voucher that is
	// this job's own, so retry until the two agree. A voucher positively
	// claimed by another job's stored receipt is the one expected exception.
	if attemptedSameSeries && !claimedByOther && last >= attemptedNumber {
		return "", 0, time.Time{}, time.Time{}, fmt.Errorf(
			"attempted voucher %d is not confirmed authorized by AFIP but the last authorized number is %d; retrying to reconcile", attemptedNumber, last)
	}
	next := last + 1
	if onAttempt != nil {
		encoded := encodeProviderReceiptID(int64(payload.ReceiptTypeCode), int64(payload.PointOfSale), next)
		if aerr := onAttempt(encoded); aerr != nil {
			// %v, not %w: a callback failure must stay retryable even if the
			// callback wrapped fiscal.ErrPermanent.
			return "", 0, time.Time{}, time.Time{}, fmt.Errorf("record attempted voucher %d: %v", next, aerr)
		}
	}

	response, rerr := p.client.RequestCAE(ctx, *payload, next)
	if rerr != nil {
		if errors.Is(rerr, fiscal.ErrPermanent) {
			return "", 0, time.Time{}, time.Time{}, rerr
		}
		recon, cerr := p.client.CompConsultar(ctx, payload.CUIT, int64(payload.PointOfSale), int64(payload.ReceiptTypeCode), next)
		if cerr != nil || recon == nil {
			return "", 0, time.Time{}, time.Time{}, rerr
		}
		if strings.ToUpper(strings.TrimSpace(recon.Resultado)) != "A" || strings.TrimSpace(recon.CAE) == "" {
			return "", 0, time.Time{}, time.Time{}, rerr
		}
		adoptedNumber := recon.ReceiptNumber
		if adoptedNumber == 0 {
			adoptedNumber = next
		}
		return strings.TrimSpace(recon.CAE), adoptedNumber, recon.CAEExpiresAt, recon.IssueDate, nil
	}
	if response == nil {
		return "", 0, time.Time{}, time.Time{}, errors.New("wsfe CAE response is required")
	}
	return response.CAE, response.ReceiptNumber, response.CAEExpiresAt, time.Time{}, nil
}

// buildAuthorizedResult assembles the fiscal.ReceiptResult for an authorized
// voucher from the local WSFE payload plus the (possibly reconciled) CAE and
// receipt number. The QR is rebuilt from the local payload (the authoritative
// record of what was sent and authorized), so a reconciled adoption is not
// degraded relative to the happy path. AuthExpiresAt is nil when the CAE expiry
// is unknown (a reconcile that couldn't parse FchVto) — never fabricated.
//
// adoptedDate is the CbteFch AFIP holds for an adopted voucher (zero for a
// fresh authorization). A retry can run on a later civil day than the
// original submission; the QR "fecha" and the stored issue date must be the
// voucher's CbteFch, so it wins over issuedAt when the days differ.
func (p *Provider) buildAuthorizedResult(payload *WSFEPayload, receiptType string, issuedAt, adoptedDate time.Time, caeRaw string, number int64, expiresAt time.Time) (*fiscal.ReceiptResult, error) {
	authCode, err := authCodeFromCAE(caeRaw)
	if err != nil {
		return nil, err
	}
	var resultIssuedAt *time.Time
	if !adoptedDate.IsZero() {
		if issuedAt.In(argentinaLocation).Format("20060102") != adoptedDate.In(argentinaLocation).Format("20060102") {
			issuedAt = adoptedDate
		}
		d := issuedAt
		resultIssuedAt = &d
	}
	qr, err := BuildQRURL(QRInput{
		Date:          issuedAt,
		CUIT:          payload.CUIT,
		PointOfSale:   payload.PointOfSale,
		ReceiptType:   payload.ReceiptTypeCode,
		ReceiptNumber: number,
		Amount:        float64(payload.TotalCents) / 100,
		Currency:      payload.CurrencyCode,
		ExchangeRate:  payload.CurrencyRate,
		DocType:       payload.DocTypeCode,
		DocNumber:     strconv.FormatInt(payload.DocNumber, 10),
		AuthType:      "E",
		AuthCode:      authCode,
	})
	if err != nil {
		return nil, err
	}
	var expPtr *time.Time
	if !expiresAt.IsZero() {
		e := expiresAt
		expPtr = &e
	}
	return &fiscal.ReceiptResult{
		ProviderReceiptID: encodeProviderReceiptID(int64(payload.ReceiptTypeCode), int64(payload.PointOfSale), number),
		ReceiptType:       receiptType,
		ReceiptNumber:     strconv.FormatInt(number, 10),
		AuthCode:          caeRaw,
		AuthExpiresAt:     expPtr,
		QRPayload:         qr,
		Status:            fiscal.StatusAuthorized,
		IssuedAt:          resultIssuedAt,
	}, nil
}

// creditNoteReceiptType returns the human-readable receipt type string for the
// nota de crédito corresponding to the given original receipt type.
func creditNoteReceiptType(originalReceiptType string) string {
	switch strings.ToLower(strings.TrimSpace(originalReceiptType)) {
	case "factura_a":
		return "nota_de_credito_a"
	case "factura_b":
		return "nota_de_credito_b"
	case "factura_c":
		return "nota_de_credito_c"
	default:
		return "nota_de_credito"
	}
}

// GetStatus reconciles the authorization state of a previously submitted voucher
// by querying AFIP's FECompConsultar. The providerReceiptID is the composite key
// "<receiptTypeCode>-<pointOfSale>-<receiptNumber>" encoded at issue time (a CAE
// alone cannot reconstruct the (CbteTipo, PtoVta, CbteNro) tuple AFIP needs).
//
// Mapping:
//   - Resultado "A" → authorized (terminal).
//   - Resultado "R" → rejected (terminal), carrying any AFIP error detail.
//   - voucher not found (AFIP error, commonly code 602, or empty Resultado) →
//     failed_retryable, so the worker backs off and an operator can re-issue.
//   - a transport/SOAP error → returned as-is (retryable by default).
//
// A malformed providerReceiptID is a deterministic, non-retryable failure: the
// decode error wraps fiscal.ErrPermanent so the worker fails the job permanently
// rather than churning the retry budget on input that can never decode.
func (p *Provider) GetStatus(ctx context.Context, providerReceiptID string) (*fiscal.ReceiptStatus, error) {
	if p.client == nil {
		return nil, errors.New("wsfe client is required")
	}
	typeCode, pos, number, err := decodeProviderReceiptID(providerReceiptID)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.CompConsultar(ctx, p.cuit, pos, typeCode, number)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("wsfe CompConsultar returned no result")
	}

	now := p.now()
	switch strings.ToUpper(strings.TrimSpace(resp.Resultado)) {
	case "A":
		status := &fiscal.ReceiptStatus{Status: fiscal.StatusAuthorized, ProviderState: "A", CheckedAt: now}
		status.AuthCode = strings.TrimSpace(resp.CAE)
		if resp.ReceiptNumber > 0 {
			status.ReceiptNumber = strconv.FormatInt(resp.ReceiptNumber, 10)
		}
		if !resp.CAEExpiresAt.IsZero() {
			exp := resp.CAEExpiresAt
			status.AuthExpiresAt = &exp
		}
		// Rebuild the AFIP QR from the reconciled tuple. A QR-build failure (missing
		// or zero data) is non-fatal: the voucher is still authorized — we just
		// leave QRPayload empty rather than failing the whole status check.
		status.QRPayload = p.buildReconciledQR(typeCode, pos, resp, now)
		return status, nil
	case "R":
		return &fiscal.ReceiptStatus{
			Status:         fiscal.StatusRejected,
			ProviderState:  "R",
			ProviderErrors: compConsultarErrors(resp),
			CheckedAt:      now,
		}, nil
	default:
		// Empty/unknown Resultado or an explicit AFIP "not found" error (e.g. 602):
		// the voucher was never authorized. Treat as retryable so a re-issue/sweep
		// can recover rather than burying it as a permanent failure.
		return &fiscal.ReceiptStatus{
			Status:         fiscal.StatusFailedRetryable,
			ProviderState:  resp.Resultado,
			ProviderErrors: compConsultarErrors(resp),
			CheckedAt:      now,
		}, nil
	}
}

// now returns the provider's clock. There is no injectable clock on the AR
// provider today, so it delegates to time.Now (UTC).
func (p *Provider) now() time.Time {
	return time.Now().UTC()
}

// buildReconciledQR rebuilds the AFIP QR URL from a reconciled FECompConsultar
// result. typeCode/pos come from the decoded providerReceiptID; the receipt
// number, amount, currency, exchange rate, doc tuple, and CAE come from AFIP's
// ResultGet. It NEVER fabricates amounts: if ImpTotal is empty/unparseable the
// QR encodes 0 (the importe is still well-formed, just zero). A missing CAE or a
// QR-build error yields "" — the caller treats the status as authorized either
// way. The CAE is not secret, so it is fine to encode into the QR.
func (p *Provider) buildReconciledQR(typeCode, pos int64, resp *CompConsultarResponse, now time.Time) string {
	authCode, err := authCodeFromCAE(resp.CAE)
	if err != nil {
		// No usable CAE → cannot build a meaningful QR. Authorized status stands.
		return ""
	}

	number := resp.ReceiptNumber

	// Amount: never fabricated. Empty/unparseable ImpTotal encodes 0.
	amount := 0.0
	if v, perr := strconv.ParseFloat(strings.TrimSpace(resp.ImpTotal), 64); perr == nil {
		amount = v
	}

	// Exchange rate defaults to 1 when empty/unparseable.
	rate := 1.0
	if s := strings.TrimSpace(resp.MonCotiz); s != "" {
		if v, perr := strconv.ParseFloat(s, 64); perr == nil {
			rate = v
		}
	}

	// Doc number: "" when AFIP returned 0 (mirrors issue-time behavior).
	docNumber := ""
	if resp.DocNro != 0 {
		docNumber = strconv.FormatInt(resp.DocNro, 10)
	}

	// Issue date: AFIP's CbteFch when parseable, else the provider's clock.
	date := resp.IssueDate
	if date.IsZero() {
		date = now
	}

	qr, err := BuildQRURL(QRInput{
		Date:          date,
		CUIT:          p.cuit,
		PointOfSale:   int(pos),
		ReceiptType:   int(typeCode),
		ReceiptNumber: number,
		Amount:        amount,
		Currency:      strings.TrimSpace(resp.MonId),
		ExchangeRate:  rate,
		DocType:       resp.DocTipo,
		DocNumber:     docNumber,
		AuthType:      "E",
		AuthCode:      authCode,
	})
	if err != nil {
		return ""
	}
	return qr
}

// compConsultarErrors maps AFIP's FECompConsultar error entries into the generic
// fiscal.ProviderError shape. Only AFIP-supplied code/message are carried — never
// credential material.
func compConsultarErrors(resp *CompConsultarResponse) []fiscal.ProviderError {
	if resp == nil || len(resp.Errors) == 0 {
		return nil
	}
	out := make([]fiscal.ProviderError, 0, len(resp.Errors))
	for _, e := range resp.Errors {
		out = append(out, fiscal.ProviderError{
			Code:    strconv.Itoa(e.Code),
			Message: strings.TrimSpace(e.Msg),
		})
	}
	return out
}

// encodeProviderReceiptID encodes the WSFE lookup tuple AFIP requires for
// FECompConsultar into the opaque ProviderReceiptID persisted on the receipt:
// "<receiptTypeCode>-<pointOfSale>-<receiptNumber>" (e.g. "11-3-43"). The CAE is
// kept separately in AuthCode; it cannot reconstruct this tuple.
func encodeProviderReceiptID(typeCode, pos, number int64) string {
	return fmt.Sprintf("%d-%d-%d", typeCode, pos, number)
}

// decodeProviderReceiptID parses a ProviderReceiptID produced by
// encodeProviderReceiptID back into its (typeCode, pos, number) tuple. A
// malformed value yields an error wrapping fiscal.ErrPermanent, because the
// failure is deterministic — retrying can never make a bad key decode.
// afipVoucherNotFoundCode is the FECompConsultar error for a voucher AFIP has
// no record of ("no existen datos").
const afipVoucherNotFoundCode = 602

func decodeProviderReceiptID(s string) (typeCode, pos, number int64, err error) {
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("malformed provider receipt id %q: want \"type-pos-number\": %w", s, fiscal.ErrPermanent)
	}
	vals := make([]int64, 3)
	for i, p := range parts {
		v, parseErr := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if parseErr != nil {
			return 0, 0, 0, fmt.Errorf("malformed provider receipt id %q: %w", s, fiscal.ErrPermanent)
		}
		vals[i] = v
	}
	return vals[0], vals[1], vals[2], nil
}

func authCodeFromCAE(raw string) (int64, error) {
	cae := strings.TrimSpace(raw)
	if cae == "" {
		return 0, fmt.Errorf("invalid CAE %q: no digits: %w", raw, fiscal.ErrPermanent)
	}
	for _, r := range cae {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid CAE %q: must contain only digits: %w", raw, fiscal.ErrPermanent)
		}
	}
	authCode, err := strconv.ParseInt(cae, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid CAE %q: %v: %w", raw, err, fiscal.ErrPermanent)
	}
	return authCode, nil
}
