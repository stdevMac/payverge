package ar

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// creditNoteTypeFor
// ---------------------------------------------------------------------------

func TestCreditNoteTypeFor(t *testing.T) {
	cases := []struct {
		receiptType string
		wantCode    int
		wantErr     bool
	}{
		{"factura_a", 3, false},
		{"factura_b", 8, false},
		{"factura_c", 13, false},
		{"FACTURA_B", 8, false}, // case-insensitive
		{"Factura_A", 3, false}, // mixed case
		{"nota_de_credito", 0, true},
		{"unknown", 0, true},
		{"", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.receiptType, func(t *testing.T) {
			got, err := creditNoteTypeFor(tc.receiptType)
			if tc.wantErr {
				require.Error(t, err)
				require.Equal(t, 0, got)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantCode, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MapCreditNoteToWSFE
// ---------------------------------------------------------------------------

func TestMapCreditNoteToWSFE_FacturaB(t *testing.T) {
	origNum := "7"
	input := validCreditNoteInput("factura_b", &origNum)

	p, err := MapCreditNoteToWSFE(input)

	require.NoError(t, err)
	// Nota de crédito B → type code 8
	require.Equal(t, 8, p.ReceiptTypeCode, "nota de crédito B is type code 8")
	// associated-voucher fields reference the original factura_b (type 6)
	require.Equal(t, 6, p.AssocTypeCode, "CbtesAsoc Tipo must be 6 (factura_b)")
	require.Equal(t, 3, p.AssocPointOfSale, "CbtesAsoc PtoVta from settings")
	require.Equal(t, int64(7), p.AssocNumber, "CbtesAsoc Nro from original receipt number")
	// CUIT and point of sale from settings
	require.Equal(t, int64(20123456789), p.CUIT)
	require.Equal(t, 3, p.PointOfSale)
	// VAT: factura_b credit note carries 21% inclusive VAT
	require.Greater(t, p.VATCents, int64(0), "factura_b credit note must carry VAT")
	require.Equal(t, p.NetCents+p.VATCents, p.TotalCents, "net+vat must equal total")
}

func TestMapCreditNoteToWSFE_FacturaA(t *testing.T) {
	origNum := "15"
	input := validCreditNoteInput("factura_a", &origNum)
	input.OriginalReceipt.ReceiptType = "factura_a"
	// factura_a credit notes require the original recipient CUIT — supply it here.
	input.OriginalReceipt.CustomerDocType = stringPtr("CUIT")
	input.OriginalReceipt.CustomerDocNumber = stringPtr("20111111112")
	input.OriginalReceipt.CustomerTaxCondition = stringPtr("responsable_inscripto")
	input.Settings.TaxID = "20111111112"
	input.Settings.PointOfSale = intPtr(1)

	p, err := MapCreditNoteToWSFE(input)

	require.NoError(t, err)
	require.Equal(t, 3, p.ReceiptTypeCode, "nota de crédito A is type code 3")
	require.Equal(t, 1, p.AssocTypeCode, "CbtesAsoc Tipo must be 1 (factura_a)")
	require.Equal(t, 1, p.AssocPointOfSale)
	require.Equal(t, int64(15), p.AssocNumber)
	require.Greater(t, p.VATCents, int64(0), "factura_a credit note must carry VAT")
	// Verify CUIT recipient is carried through.
	require.Equal(t, 80, p.DocTypeCode, "NC-A must carry recipient DocTipo 80 (CUIT)")
	require.Equal(t, int64(20111111112), p.DocNumber)
	require.Equal(t, CondicionIVAReceptorRI, p.CondicionIVAReceptorId)
}

func TestMapCreditNoteToWSFE_FacturaC(t *testing.T) {
	origNum := "3"
	input := validCreditNoteInput("factura_c", &origNum)
	input.OriginalReceipt.ReceiptType = "factura_c"

	p, err := MapCreditNoteToWSFE(input)

	require.NoError(t, err)
	require.Equal(t, 13, p.ReceiptTypeCode, "nota de crédito C is type code 13")
	require.Equal(t, 11, p.AssocTypeCode, "CbtesAsoc Tipo must be 11 (factura_c)")
	// factura_c credit note has zero VAT
	require.Equal(t, int64(0), p.VATCents, "factura_c credit note must have zero VAT")
	require.Equal(t, p.TotalCents, p.NetCents, "factura_c: net == total")
}

func TestMapCreditNoteToWSFE_MissingOriginalReceiptNumber(t *testing.T) {
	input := validCreditNoteInput("factura_b", nil) // nil ReceiptNumber

	_, err := MapCreditNoteToWSFE(input)

	require.Error(t, err)
	require.Contains(t, err.Error(), "receipt number")
}

func TestMapCreditNoteToWSFE_UnknownOriginalReceiptType(t *testing.T) {
	origNum := "1"
	input := validCreditNoteInput("factura_b", &origNum)
	input.OriginalReceipt.ReceiptType = "nota_x" // unsupported

	_, err := MapCreditNoteToWSFE(input)

	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// RequestCAE CbtesAsoc (wsfe_client integration)
// ---------------------------------------------------------------------------

// TestWSFERequestCAE_CbtesAsocPresent verifies that when AssocNumber > 0,
// the outgoing SOAP body contains <ar:CbtesAsoc> with the correct Tipo/PtoVta/Nro.
func TestWSFERequestCAE_CbtesAsocPresent(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capturedBody = string(raw)
		io.WriteString(w, feCAESolicitarFixtureApproved("71000000000008", "20260630", 1))
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	p := WSFEPayload{
		CUIT:            20111111112,
		PointOfSale:     1,
		ReceiptTypeCode: 8, // nota de crédito B
		ConceptCode:     1,
		DocTypeCode:     99,
		DocNumber:       0,
		IssueDate:       time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
		TotalCents:      12100,
		NetCents:        10000,
		VATCents:        2100,
		CurrencyCode:    "PES",
		CurrencyRate:    1,
		// associated original factura_b
		AssocTypeCode:    6,
		AssocPointOfSale: 1,
		AssocNumber:      7,
	}
	_, err := c.RequestCAE(context.Background(), p, 1)
	require.NoError(t, err)

	// Assert CbtesAsoc block is present with correct values
	require.Contains(t, capturedBody, "<ar:CbtesAsoc>", "CbtesAsoc block must be present")
	require.Contains(t, capturedBody, "<ar:CbteAsoc>", "CbteAsoc element must be present")
	require.Contains(t, capturedBody, "<ar:Tipo>6</ar:Tipo>", "Tipo must be 6 (factura_b)")
	require.Contains(t, capturedBody, "<ar:PtoVta>1</ar:PtoVta>", "PtoVta must be 1")
	require.Contains(t, capturedBody, "<ar:Nro>7</ar:Nro>", "Nro must be 7")
	// Receipt type in FeCabReq must be 8 (nota de crédito B)
	require.Contains(t, capturedBody, "<ar:CbteTipo>8</ar:CbteTipo>", "FeCabReq CbteTipo must be 8")
}

// TestWSFERequestCAE_NoCbtesAsocWhenAbsent verifies that existing IssueReceipt
// calls (no associated voucher) do NOT emit a CbtesAsoc block.
func TestWSFERequestCAE_NoCbtesAsocWhenAbsent(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capturedBody = string(raw)
		io.WriteString(w, feCAESolicitarFixtureApproved("71000000000006", "20260630", 43))
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	p := WSFEPayload{
		CUIT:            20111111112,
		PointOfSale:     1,
		ReceiptTypeCode: 6, // factura_b — no associated voucher
		ConceptCode:     1,
		DocTypeCode:     99,
		DocNumber:       0,
		IssueDate:       time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
		TotalCents:      12100,
		NetCents:        10000,
		VATCents:        2100,
		CurrencyCode:    "PES",
		CurrencyRate:    1,
		// AssocNumber == 0 → no CbtesAsoc
	}
	_, err := c.RequestCAE(context.Background(), p, 43)
	require.NoError(t, err)

	require.False(t, strings.Contains(capturedBody, "<ar:CbtesAsoc>"),
		"CbtesAsoc must NOT be present for a regular factura (no AssocNumber), body:\n%s", capturedBody)
}

// ---------------------------------------------------------------------------
// Provider.IssueCreditNote (end-to-end with httptest WSFE server)
// ---------------------------------------------------------------------------

func TestProviderIssueCreditNote_FacturaB(t *testing.T) {
	// Two-round-trip server: first call = LastAuthorized, second = RequestCAE.
	callCount := 0
	var capturedCAEBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		callCount++
		if strings.Contains(body, "FECompUltimoAutorizado") {
			io.WriteString(w, feCompUltimoAutorizadoFixture(0))
		} else {
			capturedCAEBody = body
			io.WriteString(w, feCAESolicitarFixtureApproved("71000000000008", "20260630", 1))
		}
	}))
	defer srv.Close()

	wsfe := newTestWSFEClient(srv.URL)
	provider := NewProvider(wsfe, 20111111112)

	result, err := provider.IssueCreditNote(context.Background(), validCreditNoteInputWithSettings())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, fiscal.StatusAuthorized, result.Status)
	require.Equal(t, "71000000000008", result.AuthCode)
	require.Equal(t, "1", result.ReceiptNumber)
	require.NotEmpty(t, result.QRPayload)

	// The credit note receipt type must be "nota_de_credito_b" (or similar)
	require.NotEmpty(t, result.ReceiptType)

	// Verify CbtesAsoc is in the SOAP body
	require.Contains(t, capturedCAEBody, "<ar:CbtesAsoc>")
	require.Contains(t, capturedCAEBody, "<ar:Tipo>6</ar:Tipo>")         // original = factura_b
	require.Contains(t, capturedCAEBody, "<ar:CbteTipo>8</ar:CbteTipo>") // credit note B
}

func TestProviderIssueCreditNote_NilClient(t *testing.T) {
	provider := NewProvider(nil, 20111111112)
	_, err := provider.IssueCreditNote(context.Background(), validCreditNoteInputWithSettings())
	require.Error(t, err)
	require.Contains(t, err.Error(), "wsfe client")
}

func TestProviderIssueCreditNote_LastAuthorizedError(t *testing.T) {
	client := &fakeWSFEClient{lastErr: io.EOF}
	provider := NewProvider(client, 20111111112)
	_, err := provider.IssueCreditNote(context.Background(), validCreditNoteInputWithSettings())
	require.ErrorIs(t, err, io.EOF)
}

func TestProviderIssueCreditNote_RequestCAEError(t *testing.T) {
	client := &fakeWSFEClient{requestErr: io.EOF}
	provider := NewProvider(client, 20111111112)
	_, err := provider.IssueCreditNote(context.Background(), validCreditNoteInputWithSettings())
	require.ErrorIs(t, err, io.EOF)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func validCreditNoteInput(origReceiptType string, origReceiptNumber *string) fiscal.CreditNoteInput {
	pos := 3
	return fiscal.CreditNoteInput{
		Settings: fiscal.Settings{
			TaxID:       "20123456789",
			PointOfSale: &pos,
		},
		OriginalReceipt: database.FiscalReceipt{
			ReceiptType:      origReceiptType,
			ReceiptNumber:    origReceiptNumber,
			TotalAmountCents: 12100, // 121.00 ARS inclusive of 21% IVA
			Currency:         "ARS",
		},
		Reason:   "order cancelled",
		IssuedAt: time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
	}
}

// An NC against a factura_a must identify the recipient by CUIT (DocTipo 80 +
// non-zero DocNumber), re-derived from the original receipt — NOT consumidor
// final (99/0), which AFIP rejects permanently for a nota de crédito A.
func TestMapCreditNoteToWSFE_FacturaACarriesRecipientCUIT(t *testing.T) {
	origNum := "9"
	input := validCreditNoteInput("factura_a", &origNum)
	input.OriginalReceipt.CustomerDocType = stringPtr("CUIT")
	input.OriginalReceipt.CustomerDocNumber = stringPtr("20111111112") // valid mod-11
	input.OriginalReceipt.CustomerTaxCondition = stringPtr("responsable_inscripto")

	p, err := MapCreditNoteToWSFE(input)
	require.NoError(t, err)
	require.Equal(t, 3, p.ReceiptTypeCode, "nota de crédito A is type code 3")
	require.Equal(t, 80, p.DocTypeCode, "NC-A must carry recipient DocTipo 80 (CUIT)")
	require.Equal(t, int64(20111111112), p.DocNumber, "NC-A must carry the original recipient CUIT")
	require.Equal(t, CondicionIVAReceptorRI, p.CondicionIVAReceptorId)
}

// factura_b / factura_c credit notes still use consumidor final (99/0).
func TestMapCreditNoteToWSFE_FacturaBStaysConsumerFinal(t *testing.T) {
	origNum := "7"
	input := validCreditNoteInput("factura_b", &origNum)
	p, err := MapCreditNoteToWSFE(input)
	require.NoError(t, err)
	require.Equal(t, 99, p.DocTypeCode)
	require.Equal(t, int64(0), p.DocNumber)
}

func stringPtr(s string) *string { return &s }

func validCreditNoteInputWithSettings() fiscal.CreditNoteInput {
	pos := 1
	origNum := "7"
	return fiscal.CreditNoteInput{
		Settings: fiscal.Settings{
			TaxID:       "20123456789",
			PointOfSale: &pos,
		},
		OriginalReceipt: database.FiscalReceipt{
			ReceiptType:      "factura_b",
			ReceiptNumber:    &origNum,
			TotalAmountCents: 12100,
			Currency:         "ARS",
		},
		Reason:   "factura anulada",
		IssuedAt: time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
	}
}
