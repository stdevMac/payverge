package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAccountingExportDB(t *testing.T) {
	t.Helper()
	gormDB := setupAccountingHandlerDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.BusinessFiscalSettings{},
		&database.FiscalReceipt{},
	))
}

func TestExportEntriesCSV_StreamsActiveEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingExportDB(t)
	business := createAccountingHandlerBusiness(t, "0xExportOwner")

	occurred := time.Date(2026, time.June, 25, 15, 30, 0, 0, time.UTC)
	active := database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeExpense,
		Category:    "rent",
		Amount:      420050, // $4200.50
		Currency:    "USD",
		OccurredAt:  occurred,
		Description: "June rent",
		Reference:   "INV-RENT-1",
	}
	require.NoError(t, database.GetDB().Create(&active).Error)

	voidedAt := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	voided := database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeIncome,
		Category:    "catering",
		Amount:      10000,
		Currency:    "USD",
		OccurredAt:  occurred,
		Description: "Voided deposit",
		Reference:   "VOID-1",
		VoidedAt:    &voidedAt,
	}
	require.NoError(t, database.GetDB().Create(&voided).Error)

	// Outside window — must not appear.
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeExpense,
		Category:    "utilities",
		Amount:      5000,
		Currency:    "USD",
		OccurredAt:  time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
		Description: "Out of range",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xExportOwner")
		c.Set("business_owner_address", "0xExportOwner")
		c.Next()
	})
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/entries/export.csv",
		server.RoleBasedAccessMiddleware("financial:read"),
		handler.ExportEntriesCSV,
	)

	path := fmt.Sprintf(
		"/inside/businesses/%d/accounting/entries/export.csv?start=2026-06-21&end=2026-07-20&status=active",
		business.ID,
	)
	w := performAccountingRequest(t, router, http.MethodGet, path, nil)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/csv", w.Header().Get("Content-Type"))
	assert.Equal(t,
		`attachment; filename="entries-2026-06-21-2026-07-20.csv"`,
		w.Header().Get("Content-Disposition"),
	)

	body := w.Body.String()
	lines := strings.Split(strings.TrimSpace(body), "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	// L6-12 added the notes column; the header row must carry it.
	assert.Equal(t, "occurred_at,type,category,description,reference,status,amount,currency,notes", lines[0])

	// One active entry only (voided excluded by status=active).
	dataLines := lines[1:]
	require.Len(t, dataLines, 1)
	assert.Contains(t, dataLines[0], "expense")
	assert.Contains(t, dataLines[0], "rent")
	assert.Contains(t, dataLines[0], "June rent")
	assert.Contains(t, dataLines[0], "INV-RENT-1")
	assert.Contains(t, dataLines[0], "active")
	assert.Contains(t, dataLines[0], "4200.50")
	assert.Contains(t, dataLines[0], "USD")
	assert.NotContains(t, body, "Voided deposit")
	assert.NotContains(t, body, "Out of range")
}

func TestExportPayrollRunsCSV_IncludesPayeeCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingExportDB(t)
	business := createAccountingHandlerBusiness(t, "0xPayrollExport")

	periodStart := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	run := database.PayrollRun{
		BusinessID:     business.ID,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		Status:         database.PayrollRunStatusDraft,
		Currency:       "USD",
		GrossTotal:     150000,
		BonusTotal:     10000,
		DeductionTotal: 5000,
		NetTotal:       155000,
	}
	require.NoError(t, database.GetDB().Create(&run).Error)
	require.NoError(t, database.GetDB().Create(&database.PayrollLineItem{
		PayrollRunID: run.ID,
		BusinessID:   business.ID,
		PayeeType:    database.PayrollPayeeTypeStaff,
		PayeeName:    "Alice",
		GrossAmount:  100000,
		NetAmount:    100000,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.PayrollLineItem{
		PayrollRunID: run.ID,
		BusinessID:   business.ID,
		PayeeType:    database.PayrollPayeeTypeContractor,
		PayeeName:    "Bob Co",
		GrossAmount:  50000,
		NetAmount:    55000,
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xPayrollExport")
		c.Set("business_owner_address", "0xPayrollExport")
		c.Next()
	})
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/payroll-runs/export.csv",
		server.RoleBasedAccessMiddleware("financial:read"),
		handler.ExportPayrollRunsCSV,
	)

	path := fmt.Sprintf(
		"/inside/businesses/%d/accounting/payroll-runs/export.csv?start=2026-06-01&end=2026-06-30&status=draft",
		business.ID,
	)
	w := performAccountingRequest(t, router, http.MethodGet, path, nil)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/csv", w.Header().Get("Content-Type"))
	assert.Equal(t,
		`attachment; filename="payroll-runs-2026-06-01-2026-06-30.csv"`,
		w.Header().Get("Content-Disposition"),
	)

	body := w.Body.String()
	lines := strings.Split(strings.TrimSpace(body), "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	assert.Equal(t, "period_start,period_end,status,payees,gross,bonus,deduction,net,currency", lines[0])
	assert.Contains(t, lines[1], "draft")
	assert.Contains(t, lines[1], ",2,") // payees aggregate
	assert.Contains(t, lines[1], "1500.00")
	assert.Contains(t, lines[1], "100.00")
	assert.Contains(t, lines[1], "50.00")
	assert.Contains(t, lines[1], "1550.00")
	assert.Contains(t, lines[1], "USD")
}
