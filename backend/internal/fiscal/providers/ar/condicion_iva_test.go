package ar

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"

	"github.com/stretchr/testify/require"
)

func TestCondicionIVAReceptorID(t *testing.T) {
	tests := []struct {
		cond string
		want int
	}{
		{"responsable_inscripto", CondicionIVAReceptorRI},
		{"Responsable_Inscripto", CondicionIVAReceptorRI},
		{"exento", CondicionIVAReceptorExento},
		{"monotributo", CondicionIVAReceptorMonotributo},
		{"consumidor_final", CondicionIVAReceptorConsumidorFinal},
		{"", CondicionIVAReceptorConsumidorFinal},
		{"  ", CondicionIVAReceptorConsumidorFinal},
		{"unknown_slug", CondicionIVAReceptorConsumidorFinal},
	}
	for _, tc := range tests {
		t.Run(tc.cond, func(t *testing.T) {
			require.Equal(t, tc.want, CondicionIVAReceptorID(tc.cond))
		})
	}
	// Explicit ID assertions for the four conditions we emit.
	require.Equal(t, 1, CondicionIVAReceptorID("responsable_inscripto"))
	require.Equal(t, 4, CondicionIVAReceptorID("exento"))
	require.Equal(t, 6, CondicionIVAReceptorID("monotributo"))
	require.Equal(t, 5, CondicionIVAReceptorID("consumidor_final"))
	require.Equal(t, 5, CondicionIVAReceptorID(""))
}

func TestValidateCUITMod11(t *testing.T) {
	// 20-11111111-2 is a commonly used AFIP-style CUIT with a valid check digit.
	require.True(t, ValidateCUITMod11("20111111112"))
	require.True(t, ValidateCUITMod11("20-11111111-2"))
	require.False(t, ValidateCUITMod11("20123456789")) // wrong check digit
	require.False(t, ValidateCUITMod11("2011111111"))  // short
	require.False(t, ValidateCUITMod11(""))
	require.False(t, ValidateCUITMod11("abcdefghijk"))
}

func TestMapIssueInputToWSFE_SetsCondicionIVAReceptorId(t *testing.T) {
	tests := []struct {
		name    string
		rt      string
		cond    string
		docType string
		docNum  string
		wantID  int
		wantErr bool
	}{
		{
			name: "factura_b CF",
			rt:   "factura_b", cond: "consumidor_final", docType: "DNI", docNum: "12345678",
			wantID: CondicionIVAReceptorConsumidorFinal,
		},
		{
			name: "factura_b absent condition",
			rt:   "factura_b", cond: "", docType: "DNI", docNum: "12345678",
			wantID: CondicionIVAReceptorConsumidorFinal,
		},
		{
			name: "factura_a RI",
			rt:   "factura_a", cond: "responsable_inscripto", docType: "CUIT", docNum: "20111111112",
			wantID: CondicionIVAReceptorRI,
		},
		{
			name: "factura_c exento receptor",
			rt:   "factura_c", cond: "exento", docType: "", docNum: "",
			wantID: CondicionIVAReceptorExento,
		},
		{
			name: "factura_a with CF is permanent",
			rt:   "factura_a", cond: "consumidor_final", docType: "CUIT", docNum: "20111111112",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := validIssueInput()
			in.ReceiptType = tc.rt
			in.CustomerTaxCondition = tc.cond
			in.CustomerDocType = tc.docType
			in.CustomerDocNumber = tc.docNum
			out, err := MapIssueInputToWSFE(in)
			if tc.wantErr {
				require.Error(t, err)
				require.True(t, errors.Is(err, fiscal.ErrPermanent))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantID, out.CondicionIVAReceptorId)
		})
	}
}

func TestMapIssueInputToWSFE_FacturaBCFThreshold(t *testing.T) {
	t.Setenv("FISCAL_AR_CF_ID_THRESHOLD_CENTS", "5000")
	in := validIssueInput()
	in.ReceiptType = "factura_b"
	in.CustomerDocType = "" // DocTipo 99
	in.CustomerDocNumber = ""
	in.CustomerTaxCondition = "consumidor_final"
	in.TotalAmountCents = 5000 // == threshold → reject
	out, err := MapIssueInputToWSFE(in)
	require.Error(t, err)
	require.Nil(t, out)
	require.True(t, errors.Is(err, fiscal.ErrPermanent))
	require.Contains(t, err.Error(), "threshold")

	in.TotalAmountCents = 4999
	out, err = MapIssueInputToWSFE(in)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, 99, out.DocTypeCode)

	// Identified CF (DNI) is allowed above threshold.
	in.TotalAmountCents = 50_000
	in.CustomerDocType = "DNI"
	in.CustomerDocNumber = "12345678"
	out, err = MapIssueInputToWSFE(in)
	require.NoError(t, err)
	require.Equal(t, 96, out.DocTypeCode)
}

func TestMapCreditNoteToWSFE_SetsCondicionIVAReceptorId(t *testing.T) {
	num := "43"
	cond := "responsable_inscripto"
	docT := "CUIT"
	docN := "20111111112"
	in := fiscal.CreditNoteInput{
		Settings: fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
		OriginalReceipt: database.FiscalReceipt{
			ReceiptType:          "factura_a",
			ReceiptNumber:        &num,
			TotalAmountCents:     12100,
			Currency:             "ARS",
			CustomerDocType:      &docT,
			CustomerDocNumber:    &docN,
			CustomerTaxCondition: &cond,
		},
		IssuedAt: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
	}
	p, err := MapCreditNoteToWSFE(in)
	require.NoError(t, err)
	require.Equal(t, CondicionIVAReceptorRI, p.CondicionIVAReceptorId)
}

func TestWSFERequestCAE_IncludesCondicionIVAReceptorId(t *testing.T) {
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capturedBody = string(raw)
		_, _ = io.WriteString(w, feCAESolicitarFixtureApproved("71000000000001", "20260630", 43))
	}))
	defer srv.Close()

	c := newTestWSFEClient(srv.URL)
	_, err := c.RequestCAE(context.Background(), WSFEPayload{
		CUIT: 20111111112, PointOfSale: 1, ReceiptTypeCode: 6, ConceptCode: 1,
		DocTypeCode: 99, DocNumber: 0,
		IssueDate:              time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
		TotalCents:             12100,
		NetCents:               10000,
		VATCents:               2100,
		CurrencyCode:           "PES",
		CurrencyRate:           1,
		CondicionIVAReceptorId: CondicionIVAReceptorConsumidorFinal,
	}, 43)
	require.NoError(t, err)
	require.Contains(t, capturedBody, "<ar:CondicionIVAReceptorId>5</ar:CondicionIVAReceptorId>")
}
