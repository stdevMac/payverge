package handlers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func payrollLockRouter(business *database.Business) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", business.OwnerAddress)
		c.Set("business_owner_address", business.OwnerAddress)
		c.Next()
	})
	handler := NewAccountingHandler(database.GetDBWrapper())
	perm := server.RoleBasedAccessMiddleware(string(server.PermPayrollWrite))
	router.POST("/inside/businesses/:id/accounting/payroll-runs", perm, handler.CreatePayrollRun)
	router.POST("/inside/businesses/:id/accounting/payroll-runs/:runId/mark-paid", perm, handler.MarkPayrollRunPaid)
	router.POST("/inside/businesses/:id/accounting/payroll-runs/:runId/void", perm, handler.VoidPayrollRun)
	return router
}

func lockAccountingThrough(t *testing.T, businessID uint, through time.Time) {
	t.Helper()
	day := time.Date(through.Year(), through.Month(), through.Day(), 0, 0, 0, 0, time.UTC)
	require.NoError(t, database.GetDB().Create(&database.AccountingPeriodLock{
		BusinessID:    businessID,
		LockedThrough: &day,
	}).Error)
}

// PAY-LOCK-PAIDAT: mark-paid writes paid_at = now; when today is inside the
// locked period the write would land in closed books, so it is refused even
// though the run's period end is open.
func TestMarkPayrollRunPaid_PaidAtInLockedPeriodReturns409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xPaidAtLockOwner")

	today := time.Now().UTC()
	run := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: today.AddDate(0, 0, 5),
		PeriodEnd:   today.AddDate(0, 0, 20),
		Status:      database.PayrollRunStatusDraft,
		GrossTotal:  100,
		NetTotal:    100,
	}
	require.NoError(t, database.GetDB().Create(run).Error)
	lockAccountingThrough(t, business.ID, today)

	w := performAccountingRequest(t, payrollLockRouter(business), http.MethodPost,
		fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs/%d/mark-paid", business.ID, run.ID), map[string]any{})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "period_locked")

	var persisted database.PayrollRun
	require.NoError(t, database.GetDB().First(&persisted, run.ID).Error)
	require.Equal(t, database.PayrollRunStatusDraft, persisted.Status)
	require.Nil(t, persisted.PaidAt)
}

// PAY-LOCK-PAIDAT: voiding a paid run removes its expense from the paid_at
// period, so a stored paid_at inside the locked period refuses the void.
func TestVoidPayrollRun_StoredPaidAtInLockedPeriodReturns409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xVoidLockOwner")

	paidAt := time.Date(2026, time.April, 20, 12, 0, 0, 0, time.UTC)
	run := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.May, 15, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusPaid,
		PaidAt:      &paidAt,
		GrossTotal:  100,
		NetTotal:    100,
	}
	require.NoError(t, database.GetDB().Create(run).Error)
	lockAccountingThrough(t, business.ID, time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC))

	w := performAccountingRequest(t, payrollLockRouter(business), http.MethodPost,
		fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs/%d/void", business.ID, run.ID), map[string]any{})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "period_locked")

	var persisted database.PayrollRun
	require.NoError(t, database.GetDB().First(&persisted, run.ID).Error)
	require.Equal(t, database.PayrollRunStatusPaid, persisted.Status)
}

// PAY-OVERLAP: a run overlapping a non-void run's days returns 409.
func TestCreatePayrollRun_OverlappingPeriodReturns409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOverlapOwner")
	router := payrollLockRouter(business)
	path := fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs", business.ID)
	body := func(start, end string) map[string]any {
		return map[string]any{
			"period_start": start,
			"period_end":   end,
			"line_items": []map[string]any{
				{"payee_type": "contractor", "payee_name": "Freelancer", "gross_amount": 100},
			},
		}
	}

	w := performAccountingRequest(t, router, http.MethodPost, path, body("2026-03-01T00:00:00Z", "2026-03-15T00:00:00Z"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w = performAccountingRequest(t, router, http.MethodPost, path, body("2026-03-10T00:00:00Z", "2026-03-31T00:00:00Z"))
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	w = performAccountingRequest(t, router, http.MethodPost, path, body("2026-03-16T00:00:00Z", "2026-03-31T00:00:00Z"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

// ACCT-LOCK-TZ follow-up: period_end is a calendar date stored at UTC
// midnight. A Buenos Aires business locked through 2026-03-31 must accept a
// run ending 2026-04-01; shifting that midnight into UTC-3 would read
// 2026-03-31 and wrongly refuse it. A run ending on the locked day is still 409.
func TestCreatePayrollRun_PeriodEndIsDateNotShiftedByTimezone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xDateLockOwner")
	require.NoError(t, database.GetDB().Model(business).Update("timezone", "America/Argentina/Buenos_Aires").Error)
	lockAccountingThrough(t, business.ID, time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC))

	router := payrollLockRouter(business)
	path := fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs", business.ID)
	body := func(start, end string) map[string]any {
		return map[string]any{
			"period_start": start,
			"period_end":   end,
			"line_items": []map[string]any{
				{"payee_type": "contractor", "payee_name": "Freelancer", "gross_amount": 100},
			},
		}
	}

	w := performAccountingRequest(t, router, http.MethodPost, path, body("2026-03-20T00:00:00Z", "2026-03-31T00:00:00Z"))
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	w = performAccountingRequest(t, router, http.MethodPost, path, body("2026-04-01T00:00:00Z", "2026-04-01T00:00:00Z"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}
