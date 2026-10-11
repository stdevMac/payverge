package crm

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// exportCustomersCSV drives the real ExportCustomers handler and parses the
// streamed CSV so assertions read columns by name instead of by offset.
func exportCustomersCSV(t *testing.T, handler *Handler, businessID uint, query string) ([]string, [][]string) {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", businessID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)

	handler.ExportCustomers(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	records, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	require.NoError(t, err)
	require.NotEmpty(t, records)
	return records[0], records[1:]
}

func csvColumnIndex(t *testing.T, headers []string, name string) int {
	t.Helper()
	for i, header := range headers {
		if header == name {
			return i
		}
	}
	t.Fatalf("CSV header %q not found in %v", name, headers)
	return -1
}

// TestExportCustomersCSVNamesTheVenueCurrency is the CRM half of the #926 gate.
// The "Total Spent" column is a bare number, so an ARS carta exported
// `89000.00` with nothing saying those are pesos — while the payments and
// accounting CSVs both ship a currency column. The export must name the money.
func TestExportCustomersCSVNamesTheVenueCurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	business := createCRMTestBusiness(t, db, "biz-export-ars", "0xOwnerArs", "Parrilla")
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", business.ID).
		Updates(map[string]any{"display_currency": "ARS", "default_currency": "ARS"}).Error)

	customer := createCRMTestCustomer(t, db, "ars-export@example.com")
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:   customer.ID,
		BusinessID:   business.ID,
		FirstVisitAt: time.Now(),
		TotalSpent:   89_000_00,
		IsActive:     true,
	}).Error)

	headers, rows := exportCustomersCSV(t, handler, business.ID, "")
	require.Len(t, rows, 1)

	idx := csvColumnIndex(t, headers, "Currency")
	require.Equal(t, "ARS", rows[0][idx],
		"CRM export must name the venue currency next to Total Spent")
	require.Equal(t, csvColumnIndex(t, headers, "Total Spent")+1, idx,
		"Currency belongs immediately after the money column, matching the accounting CSV")
}

// TestExportCustomersCSVCurrencyHeaderIsLocalized keeps the new column inside
// the existing ?lang= contract instead of leaving one English header in an
// otherwise Spanish file.
func TestExportCustomersCSVCurrencyHeaderIsLocalized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	business := createCRMTestBusiness(t, db, "biz-export-es", "0xOwnerEs", "Bodegón")
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", business.ID).
		Updates(map[string]any{"display_currency": "ARS"}).Error)

	customer := createCRMTestCustomer(t, db, "es-export@example.com")
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:   customer.ID,
		BusinessID:   business.ID,
		FirstVisitAt: time.Now(),
		TotalSpent:   1_500_00,
		IsActive:     true,
	}).Error)

	headers, rows := exportCustomersCSV(t, handler, business.ID, "lang=es-AR")
	require.Len(t, rows, 1)
	require.Equal(t, "ARS", rows[0][csvColumnIndex(t, headers, "Moneda")])
}

// TestExportCustomersCSVFallsBackToUSD pins the no-currency-configured venue on
// the same resolver the rest of the platform uses (display → default → USD),
// so the column is never blank.
func TestExportCustomersCSVFallsBackToUSD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	business := createCRMTestBusiness(t, db, "biz-export-usd", "0xOwnerUsd", "Diner")
	customer := createCRMTestCustomer(t, db, "usd-export@example.com")
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID:   customer.ID,
		BusinessID:   business.ID,
		FirstVisitAt: time.Now(),
		TotalSpent:   42_00,
		IsActive:     true,
	}).Error)

	headers, rows := exportCustomersCSV(t, handler, business.ID, "")
	require.Len(t, rows, 1)
	require.Equal(t, "USD", rows[0][csvColumnIndex(t, headers, "Currency")])
}

// TestExportCustomersCSVFailsLoudlyWhenTheQueryFails pins that a DB failure
// before any CSV byte is committed is a 500 JSON error, not a 200 download of
// a header-only (silently empty) customer file.
func TestExportCustomersCSVFailsLoudlyWhenTheQueryFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	business := createCRMTestBusiness(t, db, "biz-export-fail", "0xOwnerFail", "Broken")
	require.NoError(t, db.Migrator().DropTable(&database.CustomerBusiness{}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler.ExportCustomers(c)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")
	require.Empty(t, w.Header().Get("Content-Disposition"))
	require.Contains(t, w.Body.String(), "Could not export customers")
	require.NotContains(t, w.Body.String(), "Email")
}
