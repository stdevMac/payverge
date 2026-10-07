package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getWithLang issues a GET with an optional Accept-Language header, which the
// plain performAccountingRequest helper cannot set.
func getWithLang(t *testing.T, router *gin.Engine, path, acceptLanguage string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func csvHeaderLine(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	require.GreaterOrEqual(t, len(lines), 1)
	return strings.TrimSpace(lines[0])
}

// payrollExportRouter wires the real payroll CSV route with one seeded run.
func payrollExportRouter(t *testing.T, owner string) (*gin.Engine, *database.Business) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	setupAccountingExportDB(t)
	business := createAccountingHandlerBusiness(t, owner)

	require.NoError(t, database.GetDB().Create(&database.PayrollRun{
		BusinessID:     business.ID,
		PeriodStart:    time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:      time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC),
		Status:         database.PayrollRunStatusDraft,
		Currency:       "ARS",
		GrossTotal:     150000,
		BonusTotal:     10000,
		DeductionTotal: 5000,
		NetTotal:       155000,
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", owner)
		c.Set("business_owner_address", owner)
		c.Next()
	})
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/payroll-runs/export.csv",
		server.RoleBasedAccessMiddleware("financial:read"),
		handler.ExportPayrollRunsCSV,
	)
	router.GET("/inside/businesses/:id/accounting/profit-loss/export.csv",
		server.RoleBasedAccessMiddleware("financial:read"),
		handler.ExportProfitLossCSV,
	)
	return router, business
}

// TestExportPayrollRunsCSVLocalizesHeaders is the #930 gate: the entries export
// has honored ?lang= since L6-12, but payroll shipped hardcoded English
// snake_case headers, so an es-AR operator downloading the payroll book got
// `period_start,period_end,status,...` no matter what the dashboard locale said.
// Values stay machine tokens (status enums, ISO dates) — only headers localize,
// matching entriesCSVHeaders.
func TestExportPayrollRunsCSVLocalizesHeaders(t *testing.T) {
	router, business := payrollExportRouter(t, "0xPayrollLang")
	base := fmt.Sprintf(
		"/inside/businesses/%d/accounting/payroll-runs/export.csv?start=2026-06-01&end=2026-06-30",
		business.ID,
	)

	assert.Equal(t,
		"inicio_período,fin_período,estado,beneficiarios,bruto,bono,deducción,neto,moneda",
		csvHeaderLine(t, getWithLang(t, router, base+"&lang=es", "")),
		"?lang=es must localize the payroll headers",
	)
	assert.Equal(t,
		"inicio_período,fin_período,estado,beneficiarios,bruto,bono,deducción,neto,moneda",
		csvHeaderLine(t, getWithLang(t, router, base+"&lang=es-AR", "")),
		"es-AR resolves to the es operator tier",
	)
	assert.Equal(t,
		"inicio_período,fin_período,estado,beneficiarios,bruto,bono,deducción,neto,moneda",
		csvHeaderLine(t, getWithLang(t, router, base, "es-AR,es;q=0.9")),
		"Accept-Language is the fallback when no ?lang= is supplied",
	)
	assert.Equal(t,
		"period_start,period_end,status,payees,gross,bonus,deduction,net,currency",
		csvHeaderLine(t, getWithLang(t, router, base, "")),
		"default stays the historical English header row",
	)
	assert.Equal(t,
		"period_start,period_end,status,payees,gross,bonus,deduction,net,currency",
		csvHeaderLine(t, getWithLang(t, router, base+"&lang=en", "es-AR")),
		"an explicit ?lang=en beats a Spanish browser",
	)
}

// TestExportPayrollRunsCSVKeepsMachineValues pins that localization stops at the
// header row — status enums and the currency code must not be translated, or a
// re-import / spreadsheet formula breaks.
func TestExportPayrollRunsCSVKeepsMachineValues(t *testing.T) {
	router, business := payrollExportRouter(t, "0xPayrollValues")
	w := getWithLang(t, router, fmt.Sprintf(
		"/inside/businesses/%d/accounting/payroll-runs/export.csv?start=2026-06-01&end=2026-06-30&lang=es",
		business.ID), "")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	assert.Contains(t, lines[1], "draft")
	assert.Contains(t, lines[1], "ARS")
	assert.Contains(t, lines[1], "2026-06-01")
}

// TestExportProfitLossCSVLocalizesHeaders — the P&L export never even read the
// locale, so `section,category,amount,currency` shipped to every operator.
func TestExportProfitLossCSVLocalizesHeaders(t *testing.T) {
	router, business := payrollExportRouter(t, "0xPnLLang")
	base := fmt.Sprintf(
		"/inside/businesses/%d/accounting/profit-loss/export.csv?start=2026-06-01&end=2026-06-30",
		business.ID,
	)

	assert.Equal(t, "sección,categoría,monto,moneda",
		csvHeaderLine(t, getWithLang(t, router, base+"&lang=es", "")))
	assert.Equal(t, "sección,categoría,monto,moneda",
		csvHeaderLine(t, getWithLang(t, router, base, "es-AR,es;q=0.9")))
	assert.Equal(t, "section,category,amount,currency",
		csvHeaderLine(t, getWithLang(t, router, base, "")))
	assert.Equal(t, "section,category,amount,currency",
		csvHeaderLine(t, getWithLang(t, router, base+"&lang=en", "es-AR")))
}

// TestExportProfitLossCSVKeepsMachineSectionTokens — section/category values are
// the report's stable keys ("revenue", "net_profit_loss"); localizing them would
// break every downstream consumer.
func TestExportProfitLossCSVKeepsMachineSectionTokens(t *testing.T) {
	router, business := payrollExportRouter(t, "0xPnLTokens")
	w := getWithLang(t, router, fmt.Sprintf(
		"/inside/businesses/%d/accounting/profit-loss/export.csv?start=2026-06-01&end=2026-06-30&lang=es",
		business.ID), "")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	assert.Contains(t, body, "revenue,recognized_payments")
	assert.Contains(t, body, "net,net_profit_loss")
}

// TestExportSalesDataHonorsAcceptLanguage — the reports export read
// `c.DefaultQuery("lang", "en")` directly, so a browser that sent
// `Accept-Language: es-AR` and no ?lang= downloaded an English-headed CSV while
// the sibling accounting exports (resolveExportLang) localized correctly.
func TestExportSalesDataHonorsAcceptLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xSalesLang")
	router := newSalesRangeRouter(t, "0xSalesLang")

	base := fmt.Sprintf("/inside/businesses/%d/reports/export?period=week&format=csv", business.ID)

	header := csvHeaderLine(t, getWithLang(t, router, base, "es-AR,es;q=0.9,en;q=0.8"))
	assert.True(t, strings.HasPrefix(header, "Reconocido el,"),
		"Accept-Language must localize the sales CSV; got %q", header)

	header = csvHeaderLine(t, getWithLang(t, router, base, ""))
	assert.True(t, strings.HasPrefix(header, "Recognized At,"),
		"no locale hint still falls back to English; got %q", header)

	header = csvHeaderLine(t, getWithLang(t, router, base+"&lang=en", "es-AR"))
	assert.True(t, strings.HasPrefix(header, "Recognized At,"),
		"an explicit ?lang=en beats a Spanish browser; got %q", header)
}

// errorEnvelopeCode pulls the machine code out of either error envelope shape
// (server.ErrorResponse or the analytics {"success":false,...} gin.H).
func errorEnvelopeCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	code, _ := body["code"].(string)
	return code
}

// TestAccountingDateRangeErrorsCarryDateRangeCode is the other half of #930:
// the date-shaped 400s on the money screens carried VALIDATION_INVALID_INPUT (or
// no code at all), which apiErrors.json renders as a generic message — so an
// es-AR operator who inverted a range saw the raw English `error` string.
// A dedicated code lets the FE localize "revisá el rango de fechas".
func TestAccountingDateRangeErrorsCarryDateRangeCode(t *testing.T) {
	router, business := payrollExportRouter(t, "0xDateRangeCode")
	base := fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs/export.csv", business.ID)

	cases := []struct{ name, query string }{
		{"bare request", ""},
		{"half range", "?start=2026-06-01"},
		{"unknown preset", "?period=fortnight"},
		{"unparseable start", "?start=nonsense&end=2026-06-30"},
		{"unparseable end", "?start=2026-06-01&end=nonsense"},
		{"inverted range", "?start=2026-06-30&end=2026-06-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := getWithLang(t, router, base+tc.query, "")
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Equal(t, server.ErrCodeInvalidDateRange, errorEnvelopeCode(t, w),
				"date-shaped 400s must be localizable by code")
		})
	}
}

// TestAnalyticsRangeErrorsCarryDateRangeCode covers the analytics envelope,
// which is a bespoke {"success": false, "error": "..."} gin.H rather than
// server.ErrorResponse — the code must be added to it additively.
func TestAnalyticsRangeErrorsCarryDateRangeCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xAnalyticsCode")
	router := newSalesRangeRouter(t, "0xAnalyticsCode")

	cases := []struct{ name, query string }{
		{"half range", "?start_date=2026-08-01"},
		{"unparseable start", "?start_date=nonsense&end_date=2026-08-02"},
		{"unparseable end", "?start_date=2026-08-01&end_date=nonsense"},
		{"inverted range", "?start_date=2026-08-23&end_date=2026-08-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, route := range []string{"reports/export", "analytics/sales"} {
				w := getWithLang(t, router, fmt.Sprintf(
					"/inside/businesses/%d/%s%s", business.ID, route, tc.query), "")
				require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
				assert.Equal(t, server.ErrCodeInvalidDateRange, errorEnvelopeCode(t, w), route)
			}
		})
	}
}

// TestTimeseriesRangeErrorsCarryDateRangeCode — the dashboard chart's own range
// guards (from/to/period) 400 in English with no code either.
func TestTimeseriesRangeErrorsCarryDateRangeCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xTimeseriesCode")

	router := newOwnerAnalyticsTestRouter("0xTimeseriesCode")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/timeseries",
		server.RoleBasedAccessMiddleware("analytics:sales"), handler.GetTimeseries)

	base := fmt.Sprintf("/inside/businesses/%d/analytics/timeseries", business.ID)
	cases := []struct{ name, query string }{
		{"unsupported period", "?period=fortnight"},
		{"unparseable from", "?from=nonsense"},
		{"unparseable to", "?to=nonsense"},
		{"inverted range", "?from=2026-08-23&to=2026-08-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := getWithLang(t, router, base+tc.query, "")
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Equal(t, server.ErrCodeInvalidDateRange, errorEnvelopeCode(t, w))
		})
	}
}
