package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSalesRangeRouter wires the two real routes #925 exercised
// (analytics/sales and reports/export) behind the owner middleware the
// production stack uses.
func newSalesRangeRouter(t *testing.T, ownerAddress string) *gin.Engine {
	t.Helper()

	router := newOwnerAnalyticsTestRouter(ownerAddress)
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/sales", server.RoleBasedAccessMiddleware("analytics:sales"), handler.GetSalesAnalytics)
	router.GET("/inside/businesses/:id/reports/export", server.RoleBasedAccessMiddleware("reports:export"), handler.ExportSalesData)
	return router
}

// seedSalesRangePayment writes a confirmed payment so the recognized-payment
// ledger has something to export on `at`.
func seedSalesRangePayment(t *testing.T, businessID uint, number string, at time.Time, amountCents int64) {
	t.Helper()

	bill := &database.Bill{
		BusinessID:     businessID,
		BillNumber:     fmt.Sprintf("%s-%d", number, time.Now().UnixNano()),
		Items:          "[]",
		Subtotal:       amountCents,
		TotalAmount:    amountCents,
		PaidAmount:     amountCents,
		Status:         database.BillStatusPaid,
		SettlementAddr: "settlement-" + number,
		TippingAddr:    "tipping-" + number,
		CreatedAt:      at.Add(-time.Hour),
		UpdatedAt:      at,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	confirmedAt := at
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0x" + number,
		Amount:        amountCents,
		TxHash:        "range_" + number + fmt.Sprintf("_%d", time.Now().UnixNano()),
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)
}

// TestExportSalesDataRejectsInvertedDateRange is the #925 gate. QA asked for
// 2026-08-23 → 2026-08-01 and got 200 plus `sales_data_week.csv` full of last
// week's rows: the handler read neither `start_date` nor `end_date`, so the
// inverted range vanished and the default preset answered instead. An owner
// exporting "last night" must not silently receive last week.
func TestExportSalesDataRejectsInvertedDateRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	router := newSalesRangeRouter(t, "0xOwner")
	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf(
		"/inside/businesses/%d/reports/export?start_date=2026-08-23&end_date=2026-08-01&format=csv", business.ID), nil)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.NotContains(t, w.Header().Get("Content-Disposition"), "sales_data_week.csv")
}

// TestGetSalesAnalyticsRejectsInvertedDateRange covers the JSON sibling QA saw
// "rewritten to today" for the same inverted range.
func TestGetSalesAnalyticsRejectsInvertedDateRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	router := newSalesRangeRouter(t, "0xOwner")
	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf(
		"/inside/businesses/%d/analytics/sales?start_date=2026-08-23&end_date=2026-08-01", business.ID), nil)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// TestExportSalesDataRejectsOneSidedDateRange: half a range is a typo, not a
// preset. Answering the default week for it is the same silent substitution.
func TestExportSalesDataRejectsOneSidedDateRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	router := newSalesRangeRouter(t, "0xOwner")
	for _, query := range []string{"start_date=2026-08-01", "end=2026-08-01"} {
		w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf(
			"/inside/businesses/%d/reports/export?%s&format=csv", business.ID, query), nil)
		require.Equal(t, http.StatusBadRequest, w.Code, "%s: %s", query, w.Body.String())
	}
}

// TestExportSalesDataRejectsUnparseableDateRange keeps a malformed date from
// degrading to the default window.
func TestExportSalesDataRejectsUnparseableDateRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	router := newSalesRangeRouter(t, "0xOwner")
	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf(
		"/inside/businesses/%d/reports/export?start_date=23-08-2026&end_date=2026-08-24&format=csv", business.ID), nil)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// TestExportSalesDataHonorsCustomDateRange proves the accepted range actually
// scopes the file (and names itself), rather than being parsed and dropped.
func TestExportSalesDataHonorsCustomDateRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	lastNight := time.Now().UTC().AddDate(0, 0, -1)
	dayBefore := lastNight.AddDate(0, 0, -1)
	seedSalesRangePayment(t, business.ID, "LASTNIGHT", lastNight, 500_00)
	seedSalesRangePayment(t, business.ID, "DAYBEFORE", dayBefore, 900_00)

	day := lastNight.Format("2006-01-02")
	router := newSalesRangeRouter(t, "0xOwner")
	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf(
		"/inside/businesses/%d/reports/export?start_date=%s&end_date=%s&format=csv", business.ID, day, day), nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "LASTNIGHT")
	assert.NotContains(t, w.Body.String(), "DAYBEFORE")
	assert.Contains(t, w.Header().Get("Content-Disposition"), "sales_data_"+day+"_"+day+".csv",
		"the filename must name the exported range, not the preset it replaced")
}

// TestGetSalesAnalyticsHonorsCustomDateRange is the JSON sibling of the above:
// the report window must be the requested day, not today.
func TestGetSalesAnalyticsHonorsCustomDateRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	// Exactly 24h apart so the two rows always land on different calendar days
	// regardless of the wall clock the suite happens to run at.
	lastNight := time.Now().UTC().AddDate(0, 0, -1)
	seedSalesRangePayment(t, business.ID, "JSONLAST", lastNight, 500_00)
	seedSalesRangePayment(t, business.ID, "JSONTODAY", lastNight.AddDate(0, 0, 1), 900_00)

	day := lastNight.Format("2006-01-02")
	router := newSalesRangeRouter(t, "0xOwner")
	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf(
		"/inside/businesses/%d/analytics/sales?start=%s&end=%s", business.ID, day, day), nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var payload struct {
		Data struct {
			StartDate    time.Time `json:"start_date"`
			EndDate      time.Time `json:"end_date"`
			TotalRevenue float64   `json:"total_revenue"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, day, payload.Data.StartDate.UTC().Format("2006-01-02"))
	assert.InDelta(t, 500.00, payload.Data.TotalRevenue, 0.01)
}

// TestExportSalesDataWithoutRangeStillUsesPeriod guards the untouched default:
// no start/end means the preset period keeps answering exactly as before.
func TestExportSalesDataWithoutRangeStillUsesPeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	router := newSalesRangeRouter(t, "0xOwner")
	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf(
		"/inside/businesses/%d/reports/export?period=week&format=csv", business.ID), nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Disposition"), "sales_data_week.csv")
}
