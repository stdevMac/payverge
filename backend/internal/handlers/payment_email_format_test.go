package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReceiptEmailDate_ByLanguageFamily(t *testing.T) {
	ts := time.Date(2026, 7, 8, 15, 4, 0, 0, time.UTC)
	require.Equal(t, "July 8, 2026 at 3:04 PM", receiptEmailDate(ts, "en"))
	require.Equal(t, "July 8, 2026 at 3:04 PM", receiptEmailDate(ts, ""))
	require.Equal(t, "July 8, 2026 at 3:04 PM", receiptEmailDate(ts, "fr"))
	require.Equal(t, "8 de julio de 2026, 15:04", receiptEmailDate(ts, "es"))
	require.Equal(t, "8 de julio de 2026, 15:04", receiptEmailDate(ts, "es-AR"))
	require.Equal(t, "8 de julio de 2026, 15:04", receiptEmailDate(ts, "es_ar"))

	jan := time.Date(2026, 1, 2, 9, 30, 0, 0, time.UTC)
	require.Equal(t, "2 de enero de 2026, 09:30", receiptEmailDate(jan, "es"))
}

func TestReceiptEmailMoney_UsesBusinessCurrency(t *testing.T) {
	require.Equal(t, "USD 12.34", receiptEmailMoney(1234, "USD"))
	require.Equal(t, "ARS 1500.00", receiptEmailMoney(150000, "ARS"))
	require.Equal(t, "AED 99.90", receiptEmailMoney(9990, "AED"))
	// Zero-decimal currency: no fake cents (money.MajorUnitString contract).
	require.Equal(t, "JPY 1000", receiptEmailMoney(100000, "JPY"))
	// Empty currency defaults to USD.
	require.Equal(t, "USD 12.34", receiptEmailMoney(1234, ""))
}
