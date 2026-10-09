package ar

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"

	"github.com/stretchr/testify/require"
)

func TestMapIssueInputToWSFE_ExcludesTips(t *testing.T) {
	out, err := MapIssueInputToWSFE(validIssueInput())
	require.NoError(t, err)
	require.Equal(t, int64(20123456789), out.CUIT)
	require.Equal(t, 3, out.PointOfSale)
	require.Equal(t, 6, out.ReceiptTypeCode)
	require.Equal(t, int64(10000), out.TotalCents)
	require.Equal(t, int64(0), out.TipCentsIncluded)
	require.Equal(t, 96, out.DocTypeCode)
}

func TestMapIssueInputToWSFE_FacturaCMapsReceiptType(t *testing.T) {
	input := validIssueInput()
	input.ReceiptType = "factura_c"

	out, err := MapIssueInputToWSFE(input)

	require.NoError(t, err)
	require.Equal(t, 11, out.ReceiptTypeCode)
}

func TestMapIssueInputToWSFE_InvalidCUITErrors(t *testing.T) {
	tests := []struct {
		name string
		cuit string
	}{
		{name: "missing", cuit: ""},
		{name: "short", cuit: "20-1234567-8"},
		{name: "long", cuit: "20-123456789-1"},
		{name: "overflow", cuit: "999999999999999999999999999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validIssueInput()
			input.Settings.TaxID = tt.cuit

			out, err := MapIssueInputToWSFE(input)

			require.Error(t, err)
			require.Nil(t, out)
		})
	}
}

func TestMapIssueInputToWSFE_MissingOrInvalidPOSErrors(t *testing.T) {
	tests := []struct {
		name string
		pos  *int
	}{
		{name: "missing", pos: nil},
		{name: "zero", pos: intPtr(0)},
		{name: "negative", pos: intPtr(-1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validIssueInput()
			input.Settings.PointOfSale = tt.pos

			out, err := MapIssueInputToWSFE(input)

			require.Error(t, err)
			require.Nil(t, out)
		})
	}
}

func TestMapIssueInputToWSFE_UnsupportedReceiptTypeErrors(t *testing.T) {
	input := validIssueInput()
	input.ReceiptType = "nota_x"

	out, err := MapIssueInputToWSFE(input)

	require.Error(t, err)
	require.Nil(t, out)
}

func TestMapIssueInputToWSFE_MalformedRecognizedRecipientDocErrors(t *testing.T) {
	tests := []struct {
		name      string
		docType   string
		docNumber string
	}{
		{name: "empty DNI", docType: "DNI", docNumber: ""},
		{name: "short DNI", docType: "DNI", docNumber: "123456"},
		{name: "long DNI", docType: "DNI", docNumber: "123456789"},
		{name: "overflow DNI", docType: "DNI", docNumber: "999999999999999999999999999"},
		{name: "short CUIT", docType: "CUIT", docNumber: "20-1234567-8"},
		{name: "long CUIL", docType: "CUIL", docNumber: "20-123456789-1"},
		{name: "overflow CUIT", docType: "CUIT", docNumber: "999999999999999999999999999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validIssueInput()
			input.CustomerDocType = tt.docType
			input.CustomerDocNumber = tt.docNumber

			out, err := MapIssueInputToWSFE(input)

			require.Error(t, err)
			require.Nil(t, out)
		})
	}
}

func TestMapIssueInputToWSFE_UnknownDocTypeMapsConsumerFinal(t *testing.T) {
	input := validIssueInput()
	input.CustomerDocType = "passport"
	input.CustomerDocNumber = "ABC123"

	out, err := MapIssueInputToWSFE(input)

	require.NoError(t, err)
	require.Equal(t, 99, out.DocTypeCode)
	require.Equal(t, int64(0), out.DocNumber)
}

func TestMapIssueInputToWSFE_EmptyCurrencyMapsToPES(t *testing.T) {
	input := validIssueInput()
	input.Currency = ""

	out, err := MapIssueInputToWSFE(input)

	require.NoError(t, err)
	require.Equal(t, "PES", out.CurrencyCode)
	require.Equal(t, float64(1), out.CurrencyRate)
}

func TestMapIssueInputToWSFE_NonARSPESCurrencyErrors(t *testing.T) {
	input := validIssueInput()
	input.Currency = "USD"

	out, err := MapIssueInputToWSFE(input)

	require.Error(t, err)
	require.Nil(t, out)
}

func TestMapIssueInputToWSFE_ZeroTotalFallsBackToBillTotal(t *testing.T) {
	input := validIssueInput()
	input.TotalAmountCents = 0
	input.Bill.TotalAmount = 8800

	out, err := MapIssueInputToWSFE(input)

	require.NoError(t, err)
	require.Equal(t, int64(8800), out.TotalCents)
	// factura_b applies 21% inclusive VAT split; net+vat must equal total.
	require.Equal(t, out.NetCents+out.VATCents, out.TotalCents)
	require.Greater(t, out.VATCents, int64(0))
}

// ---------------------------------------------------------------------------
// VAT breakdown tests (Task 10)
// ---------------------------------------------------------------------------

func TestMapIssueInput_FacturaB_VAT(t *testing.T) {
	// 12100 cents at 21% inclusive: net = round(12100*100/121) = 10000, vat = 2100.
	in := fiscal.IssueInput{
		Settings:          fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
		ReceiptType:       "factura_b",
		Currency:          "ARS",
		TotalAmountCents:  12100,
		CustomerDocType:   "DNI",
		CustomerDocNumber: "12345678",
	}
	p, err := MapIssueInputToWSFE(in)
	require.NoError(t, err)
	require.Equal(t, int64(12100), p.TotalCents)
	require.Equal(t, p.NetCents+p.VATCents, p.TotalCents, "net+vat must equal total")
	require.Greater(t, p.VATCents, int64(0), "factura_b must carry VAT")
	require.Equal(t, 1, p.ConceptCode)
}

func TestMapIssueInput_FacturaB_ItemizedTaxKeepsDeliveryExempt(t *testing.T) {
	// Tax is charged on the subtotal only, so the net is the subtotal (10000)
	// and IVA is the itemized 10.5% tax (1050) = net × alícuota, as AFIP checks.
	// The untaxed service fee (1000) and delivery (500) leave as ImpOpEx.
	pos := 1
	in := fiscal.IssueInput{
		Business:          database.Business{TaxRate: 10.5},
		Settings:          fiscal.Settings{TaxID: "20111111112", PointOfSale: &pos},
		ReceiptType:       "factura_b",
		Currency:          "ARS",
		TotalAmountCents:  12550,
		CustomerDocType:   "DNI",
		CustomerDocNumber: "12345678",
		Bill: database.Bill{
			Subtotal:         10000,
			TaxAmount:        1050,
			ServiceFeeAmount: 1000,
			TotalAmount:      12550,
		},
	}
	p, err := MapIssueInputToWSFE(in)
	require.NoError(t, err)
	require.Equal(t, int64(1050), p.VATCents)
	require.Equal(t, int64(10000), p.NetCents, "net is the taxed subtotal; the untaxed service fee is not IVA base")
	require.Equal(t, int64(1500), p.OpExCents, "service fee + delivery are ImpOpEx")
	require.Equal(t, 4, p.IVAAlicuotaID)
	require.Equal(t, p.TotalCents, p.NetCents+p.VATCents+p.OpExCents)
}

func TestMapIssueInput_FacturaB_Itemized21NoServiceFee(t *testing.T) {
	pos := 1
	in := fiscal.IssueInput{
		Business:         database.Business{TaxRate: 21},
		Settings:         fiscal.Settings{TaxID: "20111111112", PointOfSale: &pos},
		ReceiptType:      "factura_b",
		Currency:         "ARS",
		TotalAmountCents: 12100,
		Bill: database.Bill{
			Subtotal:    10000,
			TaxAmount:   2100,
			TotalAmount: 12100,
		},
	}
	p, err := MapIssueInputToWSFE(in)
	require.NoError(t, err)
	require.Equal(t, in.Bill.TaxAmount, p.VATCents)
	require.Equal(t, int64(10000), p.NetCents)
	require.Equal(t, int64(0), p.OpExCents)
	require.Equal(t, 5, p.IVAAlicuotaID)
	require.Equal(t, p.TotalCents, p.NetCents+p.VATCents+p.OpExCents)
}

func TestMapIssueInput_FacturaC_NoVAT(t *testing.T) {
	in := fiscal.IssueInput{
		Settings:         fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
		ReceiptType:      "factura_c",
		Currency:         "ARS",
		TotalAmountCents: 5000,
	}
	p, err := MapIssueInputToWSFE(in)
	require.NoError(t, err)
	require.Equal(t, int64(0), p.VATCents, "factura_c must have zero VAT")
	require.Equal(t, p.TotalCents, p.NetCents, "factura_c: net == total")
	require.Equal(t, int64(0), p.OpExCents)
}

func TestMapIssueInput_FacturaC_IgnoresItemizedTax(t *testing.T) {
	pos := 1
	in := fiscal.IssueInput{
		Business:         database.Business{TaxRate: 21},
		Settings:         fiscal.Settings{TaxID: "20111111112", PointOfSale: &pos},
		ReceiptType:      "factura_c",
		Currency:         "ARS",
		TotalAmountCents: 12100,
		Bill: database.Bill{
			Subtotal:    10000,
			TaxAmount:   2100,
			TotalAmount: 12100,
		},
	}
	p, err := MapIssueInputToWSFE(in)
	require.NoError(t, err)
	require.Equal(t, int64(0), p.VATCents, "factura_c stays exempt even when the bill itemizes tax")
	require.Equal(t, p.TotalCents, p.NetCents)
	require.Equal(t, int64(0), p.OpExCents)
}

func TestMapIssueInput_FacturaA_CUIT_OK(t *testing.T) {
	in := fiscal.IssueInput{
		Settings:             fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
		ReceiptType:          "factura_a",
		Currency:             "ARS",
		TotalAmountCents:     12100,
		CustomerDocType:      "CUIT",
		CustomerDocNumber:    "20111111112", // valid mod-11
		CustomerTaxCondition: "responsable_inscripto",
	}
	p, err := MapIssueInputToWSFE(in)
	require.NoError(t, err)
	require.Equal(t, 1, p.ReceiptTypeCode, "factura_a → receipt type code 1")
	require.Equal(t, 80, p.DocTypeCode, "factura_a requires CUIT (doc type 80)")
	require.Equal(t, CondicionIVAReceptorRI, p.CondicionIVAReceptorId)
	require.Greater(t, p.VATCents, int64(0), "factura_a must carry VAT")
	require.Equal(t, p.NetCents+p.VATCents, p.TotalCents, "net+vat must equal total")
}

func TestMapIssueInput_FacturaA_DNI_Errors(t *testing.T) {
	// factura_a legally requires an IVA-registered recipient (CUIT/CUIL).
	// A DNI or unknown doc type must be rejected.
	tests := []struct {
		name    string
		docType string
		docNum  string
	}{
		{name: "DNI", docType: "DNI", docNum: "12345678"},
		{name: "unknown", docType: "", docNum: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := fiscal.IssueInput{
				Settings:          fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
				ReceiptType:       "factura_a",
				Currency:          "ARS",
				TotalAmountCents:  12100,
				CustomerDocType:   tt.docType,
				CustomerDocNumber: tt.docNum,
			}
			p, err := MapIssueInputToWSFE(in)
			require.Error(t, err, "factura_a with non-CUIT doc must error")
			require.Nil(t, p)
		})
	}
}

// ---------------------------------------------------------------------------
// Deterministic-error classification (T5 / I-1)
//
// Every local validation/mapping error is deterministic: it will fail
// identically on every retry, so it must be classified PERMANENT (wrap
// fiscal.ErrPermanent) and not churn the worker's retry budget.
// ---------------------------------------------------------------------------

func TestMapIssueInputToWSFE_DeterministicErrorsArePermanent(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(in *fiscal.IssueInput)
	}{
		{"invalid CUIT", func(in *fiscal.IssueInput) { in.Settings.TaxID = "123" }},
		{"missing point of sale", func(in *fiscal.IssueInput) { in.Settings.PointOfSale = nil }},
		{"unsupported receipt type", func(in *fiscal.IssueInput) { in.ReceiptType = "nota_x" }},
		{"invalid DNI recipient", func(in *fiscal.IssueInput) { in.CustomerDocType = "DNI"; in.CustomerDocNumber = "12" }},
		{"invalid CUIT recipient", func(in *fiscal.IssueInput) { in.CustomerDocType = "CUIT"; in.CustomerDocNumber = "12" }},
		{"factura_a without CUIT", func(in *fiscal.IssueInput) {
			in.ReceiptType = "factura_a"
			in.CustomerDocType = "DNI"
			in.CustomerDocNumber = "12345678"
		}},
		{"unsupported currency", func(in *fiscal.IssueInput) { in.Currency = "USD" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validIssueInput()
			tc.mutate(&in)
			_, err := MapIssueInputToWSFE(in)
			require.Error(t, err)
			require.True(t, errors.Is(err, fiscal.ErrPermanent), "want ErrPermanent, got %v", err)
		})
	}
}

func TestMapCreditNoteToWSFE_DeterministicErrorsArePermanent(t *testing.T) {
	num := func(s string) *string { return &s }
	base := func() fiscal.CreditNoteInput {
		return fiscal.CreditNoteInput{
			Settings: fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
			OriginalReceipt: database.FiscalReceipt{
				ReceiptType:      "factura_b",
				ReceiptNumber:    num("43"),
				TotalAmountCents: 10000,
				Currency:         "ARS",
			},
			IssuedAt: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
		}
	}
	cases := []struct {
		name   string
		mutate func(in *fiscal.CreditNoteInput)
	}{
		{"invalid CUIT", func(in *fiscal.CreditNoteInput) { in.Settings.TaxID = "123" }},
		{"missing point of sale", func(in *fiscal.CreditNoteInput) { in.Settings.PointOfSale = nil }},
		{"unsupported original receipt type", func(in *fiscal.CreditNoteInput) { in.OriginalReceipt.ReceiptType = "nota_x" }},
		{"missing original receipt number", func(in *fiscal.CreditNoteInput) { in.OriginalReceipt.ReceiptNumber = nil }},
		{"non-numeric original receipt number", func(in *fiscal.CreditNoteInput) { in.OriginalReceipt.ReceiptNumber = num("abc") }},
		{"non-positive original receipt number", func(in *fiscal.CreditNoteInput) { in.OriginalReceipt.ReceiptNumber = num("0") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base()
			tc.mutate(&in)
			_, err := MapCreditNoteToWSFE(in)
			require.Error(t, err)
			require.True(t, errors.Is(err, fiscal.ErrPermanent), "want ErrPermanent, got %v", err)
		})
	}
}

func TestAuthCodeFromCAE_InvalidIsPermanent(t *testing.T) {
	for _, raw := range []string{"", "  ", "12ab34", "not-a-number"} {
		_, err := authCodeFromCAE(raw)
		require.Error(t, err)
		require.True(t, errors.Is(err, fiscal.ErrPermanent), "want ErrPermanent for %q, got %v", raw, err)
	}
}

// ---------------------------------------------------------------------------
// Partial-amount notas de crédito (T7)
// ---------------------------------------------------------------------------

func TestMapCreditNoteToWSFE_PartialAmount(t *testing.T) {
	num := func(s string) *string { return &s }
	in := fiscal.CreditNoteInput{
		Settings: fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
		OriginalReceipt: database.FiscalReceipt{
			ReceiptType:      "factura_b",
			ReceiptNumber:    num("43"),
			TotalAmountCents: 10000,
			Currency:         "ARS",
		},
		AmountCents: 2500,
		IssuedAt:    time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
	}
	p, err := MapCreditNoteToWSFE(in)
	require.NoError(t, err)
	require.Equal(t, int64(2500), p.TotalCents, "a partial credit note credits the partial amount, not the original total")
	require.Equal(t, p.NetCents+p.VATCents, p.TotalCents, "net+vat must equal the credited amount")
	require.Greater(t, p.VATCents, int64(0), "factura_b NC carries VAT")
}

func TestMapCreditNoteToWSFE_ZeroAmountFallsBackToOriginalTotal(t *testing.T) {
	num := func(s string) *string { return &s }
	in := fiscal.CreditNoteInput{
		Settings: fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
		OriginalReceipt: database.FiscalReceipt{
			ReceiptType:      "factura_b",
			ReceiptNumber:    num("43"),
			TotalAmountCents: 10000,
			Currency:         "ARS",
		},
		AmountCents: 0, // unset → full original total
		IssuedAt:    time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
	}
	p, err := MapCreditNoteToWSFE(in)
	require.NoError(t, err)
	require.Equal(t, int64(10000), p.TotalCents)
}

func validIssueInput() fiscal.IssueInput {
	point := 3
	return fiscal.IssueInput{
		Settings: fiscal.Settings{
			TaxID:        "20123456789",
			TaxCondition: "responsable_inscripto",
			PointOfSale:  &point,
		},
		Bill: database.Bill{
			ID:          1,
			Status:      database.BillStatusPaid,
			TotalAmount: 10000,
			PaidAmount:  10000,
			TipAmount:   1500,
		},
		TotalAmountCents:  10000,
		TipAmountCents:    1500,
		Currency:          "ARS",
		ReceiptType:       "factura_b",
		IssuedAt:          time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
		CustomerDocType:   "DNI",
		CustomerDocNumber: "12345678",
	}
}

func intPtr(value int) *int {
	return &value
}

// A full nota de crédito must credit exactly the IVA, net, and ImpOpEx the
// factura declared. The factura below (10.5% itemized tax, 500 delivery fee
// as ImpOpEx) used to be credited as a flat 21% inclusive split of 12550,
// which over-credited IVA and dropped the exempt part.
func TestMapCreditNoteToWSFE_FullCreditMirrorsFacturaSplit(t *testing.T) {
	num := func(s string) *string { return &s }
	bill := database.Bill{Subtotal: 10000, TaxAmount: 1050, ServiceFeeAmount: 1000, TotalAmount: 12550}
	issue, err := MapIssueInputToWSFE(fiscal.IssueInput{
		Business:          database.Business{TaxRate: 10.5},
		Settings:          fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
		ReceiptType:       "factura_b",
		Currency:          "ARS",
		TotalAmountCents:  12550,
		CustomerDocType:   "DNI",
		CustomerDocNumber: "12345678",
		Bill:              bill,
	})
	require.NoError(t, err)

	cn, err := MapCreditNoteToWSFE(fiscal.CreditNoteInput{
		Settings: fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
		OriginalReceipt: database.FiscalReceipt{
			ReceiptType:      "factura_b",
			ReceiptNumber:    num("43"),
			TotalAmountCents: 12550,
			Currency:         "ARS",
		},
		Bill:            bill,
		BusinessTaxRate: 10.5,
		IssuedAt:        time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, issue.TotalCents, cn.TotalCents)
	require.Equal(t, issue.NetCents, cn.NetCents)
	require.Equal(t, issue.VATCents, cn.VATCents)
	require.Equal(t, issue.OpExCents, cn.OpExCents)
	require.Equal(t, issue.IVAAlicuotaID, cn.IVAAlicuotaID)
}

// A partial credit scales the factura's IVA and ImpOpEx by credited/original
// and keeps ImpNeto + ImpIVA + ImpOpEx equal to the credited amount.
func TestMapCreditNoteToWSFE_PartialCreditScalesFacturaSplit(t *testing.T) {
	num := func(s string) *string { return &s }
	bill := database.Bill{Subtotal: 10000, TaxAmount: 1050, ServiceFeeAmount: 1000, TotalAmount: 12550}
	cn, err := MapCreditNoteToWSFE(fiscal.CreditNoteInput{
		Settings: fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
		OriginalReceipt: database.FiscalReceipt{
			ReceiptType:      "factura_b",
			ReceiptNumber:    num("43"),
			TotalAmountCents: 12550,
			Currency:         "ARS",
		},
		Bill:            bill,
		BusinessTaxRate: 10.5,
		AmountCents:     6275, // half
		IssuedAt:        time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, int64(6275), cn.TotalCents)
	require.Equal(t, int64(525), cn.VATCents)
	require.Equal(t, int64(750), cn.OpExCents)
	require.Equal(t, int64(5000), cn.NetCents)
	require.Equal(t, 4, cn.IVAAlicuotaID, "10.5% alícuota id")
	require.Equal(t, cn.TotalCents, cn.NetCents+cn.VATCents+cn.OpExCents)
}

// factura_c credits stay IVA-exento regardless of itemized bill tax.
func TestMapCreditNoteToWSFE_FacturaCStaysExempt(t *testing.T) {
	num := func(s string) *string { return &s }
	cn, err := MapCreditNoteToWSFE(fiscal.CreditNoteInput{
		Settings: fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
		OriginalReceipt: database.FiscalReceipt{
			ReceiptType:      "factura_c",
			ReceiptNumber:    num("7"),
			TotalAmountCents: 12100,
			Currency:         "ARS",
		},
		Bill:            database.Bill{Subtotal: 10000, TaxAmount: 2100, TotalAmount: 12100},
		BusinessTaxRate: 21,
		IssuedAt:        time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), cn.VATCents)
	require.Equal(t, int64(0), cn.OpExCents)
	require.Equal(t, cn.TotalCents, cn.NetCents)
}

// requireAlicuotaConsistent asserts AFIP's AlicIva check: ImpIVA must equal
// ImpNeto × alícuota (one cent of rounding allowed).
func requireAlicuotaConsistent(t *testing.T, p *WSFEPayload, rate float64) {
	t.Helper()
	expected := int64(math.Round(float64(p.NetCents) * rate / 100))
	diff := expected - p.VATCents
	require.Truef(t, diff >= -1 && diff <= 1, "IVA %d is not net %d × %.1f%% (%d)", p.VATCents, p.NetCents, rate, expected)
	require.Equal(t, p.TotalCents, p.NetCents+p.VATCents+p.OpExCents)
}

// A 21% bill with a service fee: the fee is untaxed, so it must not be IVA base.
func TestMapIssueInput_FacturaB_ServiceFeeIsNotIVABase(t *testing.T) {
	pos := 1
	p, err := MapIssueInputToWSFE(fiscal.IssueInput{
		Business:         database.Business{TaxRate: 21},
		Settings:         fiscal.Settings{TaxID: "20111111112", PointOfSale: &pos},
		ReceiptType:      "factura_b",
		Currency:         "ARS",
		TotalAmountCents: 13100,
		Bill:             database.Bill{Subtotal: 10000, TaxAmount: 2100, ServiceFeeAmount: 1000, TotalAmount: 13100},
	})
	require.NoError(t, err)
	require.Equal(t, int64(10000), p.NetCents)
	require.Equal(t, int64(2100), p.VATCents)
	require.Equal(t, int64(1000), p.OpExCents)
	requireAlicuotaConsistent(t, p, 21)
}

// A loyalty discount lowers the taxed gross; net and IVA shrink together so
// IVA stays net × alícuota instead of keeping the full tax on a smaller net.
func TestMapIssueInput_FacturaB_LoyaltyDiscountKeepsAlicuotaConsistent(t *testing.T) {
	pos := 1
	p, err := MapIssueInputToWSFE(fiscal.IssueInput{
		Business:         database.Business{TaxRate: 21},
		Settings:         fiscal.Settings{TaxID: "20111111112", PointOfSale: &pos},
		ReceiptType:      "factura_b",
		Currency:         "ARS",
		TotalAmountCents: 11100,
		Bill:             database.Bill{Subtotal: 10000, TaxAmount: 2100, LoyaltyDiscountCents: 1000, TotalAmount: 11100},
	})
	require.NoError(t, err)
	require.Equal(t, int64(9174), p.NetCents)
	require.Equal(t, int64(1926), p.VATCents)
	require.Equal(t, int64(0), p.OpExCents)
	requireAlicuotaConsistent(t, p, 21)
}

// A business rate that is not an AFIP alícuota declares 21%. Tax charged at
// another rate cannot be declared as-is; the taxed gross is re-split at 21%.
func TestMapIssueInput_FacturaB_NonAlicuotaRateResplitsTaxedGross(t *testing.T) {
	pos := 1
	p, err := MapIssueInputToWSFE(fiscal.IssueInput{
		Business:         database.Business{TaxRate: 15},
		Settings:         fiscal.Settings{TaxID: "20111111112", PointOfSale: &pos},
		ReceiptType:      "factura_b",
		Currency:         "ARS",
		TotalAmountCents: 12000,
		Bill:             database.Bill{Subtotal: 10000, TaxAmount: 1500, ServiceFeeAmount: 500, TotalAmount: 12000},
	})
	require.NoError(t, err)
	require.Equal(t, 5, p.IVAAlicuotaID)
	require.Equal(t, int64(500), p.OpExCents)
	requireAlicuotaConsistent(t, p, 21)
}

// Loyalty covered the taxed food and its IVA. The leftover service fee must
// be ImpOpEx, not an inclusive IVA split of the untaxed remainder.
func TestSplitIssueIVA_LoyaltyCoversTaxedGrossLeavesRemainderUntaxed(t *testing.T) {
	bill := database.Bill{Subtotal: 10000, TaxAmount: 2100, LoyaltyDiscountCents: 12100}
	net, vat, opEx := splitIssueIVA(bill, 1000, 21)
	require.Equal(t, int64(0), net)
	require.Equal(t, int64(0), vat)
	require.Equal(t, int64(1000), opEx)
}

// A partial credit of that bill credits only the untaxed remainder, scaled.
func TestSplitCreditNoteIVA_LoyaltyCoversTaxedGrossPartialCreditIsUntaxed(t *testing.T) {
	bill := database.Bill{Subtotal: 10000, TaxAmount: 2100, LoyaltyDiscountCents: 12100}
	net, vat, opEx := splitCreditNoteIVA("factura_b", bill, 1000, 500, 21)
	require.Equal(t, int64(0), net)
	require.Equal(t, int64(0), vat)
	require.Equal(t, int64(500), opEx)
	require.Equal(t, int64(500), net+vat+opEx)
}

// A partial credit on a bill with an untaxed service fee keeps the credited
// IVA consistent with its net.
func TestMapCreditNoteToWSFE_PartialCreditKeepsAlicuotaConsistent(t *testing.T) {
	num := "43"
	cn, err := MapCreditNoteToWSFE(fiscal.CreditNoteInput{
		Settings: fiscal.Settings{TaxID: "20111111112", PointOfSale: intPtr(1)},
		OriginalReceipt: database.FiscalReceipt{
			ReceiptType: "factura_b", ReceiptNumber: &num, TotalAmountCents: 13100, Currency: "ARS",
		},
		Bill:            database.Bill{Subtotal: 10000, TaxAmount: 2100, ServiceFeeAmount: 1000, TotalAmount: 13100},
		BusinessTaxRate: 21,
		AmountCents:     4000,
		IssuedAt:        time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, int64(4000), cn.TotalCents)
	require.Equal(t, int64(305), cn.OpExCents)
	requireAlicuotaConsistent(t, cn, 21)
}
