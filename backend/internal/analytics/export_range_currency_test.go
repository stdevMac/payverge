package analytics

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// seedExportPayment writes one confirmed payment (and its bill) so
// GetRecognizedPaymentEvents has a row to recognize at `at`.
func seedExportPayment(t *testing.T, db *database.DB, businessID uint, number string, at time.Time, amount int64) {
	t.Helper()

	bill := createAnalyticsBill(t, db, businessID, number, amount, amount, 0, database.BillStatusPaid, at.Add(-time.Hour), at)
	confirmedAt := at
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0x" + number,
		Amount:        amount,
		TxHash:        "export_" + number,
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)
}

func parseExportCSV(t *testing.T, data []byte) ([]string, [][]string) {
	t.Helper()
	records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	require.NoError(t, err)
	require.NotEmpty(t, records)
	return records[0], records[1:]
}

func exportColumn(t *testing.T, headers []string, name string) int {
	t.Helper()
	for i, header := range headers {
		if header == name {
			return i
		}
	}
	t.Fatalf("CSV header %q not found in %v", name, headers)
	return -1
}

func setExportVenueCurrency(t *testing.T, db *database.DB, businessID uint, display, fallback string) {
	t.Helper()
	require.NoError(t, db.GetGorm().Model(&database.Business{}).
		Where("id = ?", businessID).
		Updates(map[string]any{"display_currency": display, "default_currency": fallback}).Error)
}

// TestExportSalesDataCSVNamesTheVenueCurrency is the sales half of the #926
// gate. Every money column in the sales export was a bare number, so an ARS
// carta downloaded `121500.00` with nothing saying those were pesos — while
// the payments and accounting CSVs both carry a currency column.
func TestExportSalesDataCSVNamesTheVenueCurrency(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 8, 23, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Parrilla Quebracho Azul")
	setExportVenueCurrency(t, db, business.ID, "ARS", "ARS")

	seedExportPayment(t, db, business.ID, "ARSSALE", fixedNow.Add(-2*time.Hour), 121_500_00)

	data, err := service.ExportSalesData(business.ID, "today", "csv", time.UTC, "en")
	require.NoError(t, err)

	headers, rows := parseExportCSV(t, data)
	require.Len(t, rows, 1)
	assert.Equal(t, "ARS", rows[0][exportColumn(t, headers, "Currency")],
		"sales export must name the venue currency its amounts are denominated in")
}

// TestExportSalesDataCSVCurrencyHeaderIsLocalized keeps the new column inside
// the export's existing lang contract.
func TestExportSalesDataCSVCurrencyHeaderIsLocalized(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 8, 23, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Bodegón Mesa Larga")
	setExportVenueCurrency(t, db, business.ID, "", "ARS")

	seedExportPayment(t, db, business.ID, "ESSALE", fixedNow.Add(-2*time.Hour), 1_000_00)

	data, err := service.ExportSalesData(business.ID, "today", "csv", time.UTC, "es-AR")
	require.NoError(t, err)

	headers, rows := parseExportCSV(t, data)
	require.Len(t, rows, 1)
	assert.Equal(t, "ARS", rows[0][exportColumn(t, headers, "Moneda")])
}

// TestExportSalesDataCSVFallsBackToUSD pins the resolver order used everywhere
// else (display → default → USD) so the column is never blank.
func TestExportSalesDataCSVFallsBackToUSD(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 8, 23, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Unset Currency Diner")

	seedExportPayment(t, db, business.ID, "USDSALE", fixedNow.Add(-2*time.Hour), 10_00)

	data, err := service.ExportSalesData(business.ID, "today", "csv", time.UTC, "en")
	require.NoError(t, err)

	headers, rows := parseExportCSV(t, data)
	require.Len(t, rows, 1)
	assert.Equal(t, "USD", rows[0][exportColumn(t, headers, "Currency")])
}

// TestExportSalesDataRangeHonorsCustomWindow is the service half of the #925
// gate: an explicit [start, end) must scope the export instead of silently
// resolving the default preset window.
func TestExportSalesDataRangeHonorsCustomWindow(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 8, 23, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Range Restaurant")

	seedExportPayment(t, db, business.ID, "LASTNIGHT", time.Date(2026, 8, 22, 21, 0, 0, 0, time.UTC), 500_00)
	seedExportPayment(t, db, business.ID, "TONIGHT", fixedNow.Add(-2*time.Hour), 900_00)

	start := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	data, err := service.ExportSalesDataRange(business.ID, "week", &start, &end, "csv", time.UTC, "en")
	require.NoError(t, err)

	csvText := string(data)
	assert.Contains(t, csvText, "LASTNIGHT")
	assert.NotContains(t, csvText, "TONIGHT",
		"an explicit custom range must win over the period preset")
}

// TestExportSalesDataRangeRejectsInvertedWindow proves the service refuses an
// end-before-start range rather than quietly widening it.
func TestExportSalesDataRangeRejectsInvertedWindow(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 8, 23, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Inverted Restaurant")

	start := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	_, err := service.ExportSalesDataRange(business.ID, "week", &start, &end, "csv", time.UTC, "en")
	require.Error(t, err)
}

// TestGetPeriodReportRangeHonorsCustomWindow mirrors the export gate for the
// analytics/sales JSON surface, which silently rewrote an inverted range to
// the default period window.
func TestGetPeriodReportRangeHonorsCustomWindow(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 8, 23, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Report Range Restaurant")

	seedExportPayment(t, db, business.ID, "RRLAST", time.Date(2026, 8, 22, 21, 0, 0, 0, time.UTC), 500_00)
	seedExportPayment(t, db, business.ID, "RRTONIGHT", fixedNow.Add(-2*time.Hour), 900_00)

	start := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	report, err := service.GetPeriodReportRange(business.ID, "week", &start, &end, time.UTC)
	require.NoError(t, err)

	assert.Equal(t, start, report.StartDate.UTC())
	assert.Equal(t, end, report.EndDate.UTC())
	assert.InDelta(t, 500.00, report.TotalRevenue, 0.01,
		"only the requested day's recognized revenue belongs in the report")
}
