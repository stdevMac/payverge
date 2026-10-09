package fiscal

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

func TestResolveReceiptType(t *testing.T) {
	sp := func(s string) *string { return &s }

	cases := []struct {
		name             string
		emitterCondition string
		custCondition    string
		custDocType      string
		custDocNumber    string
		want             string
	}{
		{"monotributo emitter → factura_c", "monotributo", "responsable_inscripto", "CUIT", "20111111112", "factura_c"},
		{"exento emitter → factura_c", "exento", "", "", "", "factura_c"},
		{"RI customer with CUIT → factura_a", "responsable_inscripto", "responsable_inscripto", "CUIT", "20111111112", "factura_a"},
		{"RI customer with CUIL → factura_a", "responsable_inscripto", "responsable_inscripto", "CUIL", "20111111112", "factura_a"},
		// D-1 regression: a responsable_inscripto identified only by a DNI must NOT
		// become factura_a (the WSFE mapper rejects factura_a without a CUIT/CUIL);
		// it falls through to factura_b, a valid issuable invoice.
		{"RI customer with DNI → factura_b (D-1)", "responsable_inscripto", "responsable_inscripto", "DNI", "12345678", "factura_b"},
		{"RI customer no doc → factura_b", "responsable_inscripto", "responsable_inscripto", "", "", "factura_b"},
		{"consumidor final → factura_b", "responsable_inscripto", "consumidor_final", "DNI", "12345678", "factura_b"},
		{"no customer identity → factura_b", "responsable_inscripto", "", "", "", "factura_b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bill := database.Bill{}
			if tc.custCondition != "" {
				bill.FiscalCustomerTaxCondition = sp(tc.custCondition)
			}
			if tc.custDocType != "" {
				bill.FiscalCustomerDocType = sp(tc.custDocType)
			}
			if tc.custDocNumber != "" {
				bill.FiscalCustomerDocNumber = sp(tc.custDocNumber)
			}
			got := resolveReceiptType(Settings{Country: "AR", TaxCondition: tc.emitterCondition}, bill)
			require.Equal(t, tc.want, got)

			// D-2: the worker's resolveReceiptType must be a thin delegate of the
			// single canonical resolver — they must agree on every case.
			canonical := ResolveIssuableReceiptType(tc.emitterCondition, tc.custCondition, tc.custDocType, tc.custDocNumber)
			require.Equal(t, tc.want, canonical, "ResolveIssuableReceiptType must match resolveReceiptType")
		})
	}
}

func TestResolveIssuableReceiptTypeForCountry(t *testing.T) {
	sp := func(s string) *string { return &s }

	t.Run("AR keeps AFIP letters", func(t *testing.T) {
		bill := database.Bill{FiscalCustomerTaxCondition: sp("consumidor_final")}
		got := resolveReceiptType(Settings{Country: "AR", TaxCondition: "responsable_inscripto"}, bill)
		require.Equal(t, "factura_b", got)
	})

	t.Run("US with no customer doc → receipt", func(t *testing.T) {
		got := ResolveIssuableReceiptTypeForCountry("US", "general", "consumidor_final", "", "")
		require.Equal(t, "receipt", got)
		bill := database.Bill{}
		require.Equal(t, "receipt", resolveReceiptType(Settings{Country: "US", TaxCondition: "general"}, bill))
	})

	t.Run("US with customer doc → invoice", func(t *testing.T) {
		got := ResolveIssuableReceiptTypeForCountry("US", "general", "", "EIN", "12-3456789")
		require.Equal(t, "invoice", got)
		doc := "12-3456789"
		bill := database.Bill{FiscalCustomerDocNumber: &doc}
		require.Equal(t, "invoice", resolveReceiptType(Settings{Country: " us ", TaxCondition: "general"}, bill))
	})

	t.Run("AE → standard_invoice", func(t *testing.T) {
		require.Equal(t, "standard_invoice",
			ResolveIssuableReceiptTypeForCountry("AE", "", "", "", ""))
	})
}

func TestArgentinaLetterFromReceiptType(t *testing.T) {
	require.Equal(t, "A", ArgentinaLetterFromReceiptType("factura_a"))
	require.Equal(t, "B", ArgentinaLetterFromReceiptType("factura_b"))
	require.Equal(t, "C", ArgentinaLetterFromReceiptType("nota_de_credito_c"))
	require.Equal(t, "", ArgentinaLetterFromReceiptType("invoice"))
	require.Equal(t, "", ArgentinaLetterFromReceiptType("receipt"))
}

func TestNormalizeStoredReceiptTypeForCountry(t *testing.T) {
	require.Equal(t, "factura_b", NormalizeStoredReceiptTypeForCountry("AR", "factura_b"))
	require.Equal(t, "invoice", NormalizeStoredReceiptTypeForCountry("US", "factura_a"))
	require.Equal(t, "receipt", NormalizeStoredReceiptTypeForCountry("US", "factura_b"))
	require.Equal(t, "receipt", NormalizeStoredReceiptTypeForCountry("US", "factura_c"))
	require.Equal(t, "invoice", NormalizeStoredReceiptTypeForCountry(" us ", "nota_de_credito_a"))
	require.Equal(t, "standard_invoice", NormalizeStoredReceiptTypeForCountry("AE", "factura_c"))
	require.Equal(t, "invoice", NormalizeStoredReceiptTypeForCountry("US", "invoice"))
	require.Equal(t, "receipt", NormalizeStoredReceiptTypeForCountry("", "factura_b"))
}

func TestReceiptTypeFilterValues(t *testing.T) {
	require.Contains(t, ReceiptTypeFilterValues("invoice"), "factura_a")
	require.Contains(t, ReceiptTypeFilterValues("receipt"), "factura_b")
	require.Equal(t, []string{"factura_a"}, ReceiptTypeFilterValues("factura_a"))
	require.Nil(t, ReceiptTypeFilterValues(""))
}
