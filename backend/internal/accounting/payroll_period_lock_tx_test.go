package accounting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func lockBooksThrough(t *testing.T, businessID uint, through time.Time) {
	t.Helper()
	d := time.Date(through.Year(), through.Month(), through.Day(), 0, 0, 0, 0, time.UTC)
	require.NoError(t, database.GetDB().Create(&database.AccountingPeriodLock{
		BusinessID: businessID, LockedThrough: &d, Note: "close",
	}).Error)
}

// The service checks the period inside the write transaction, so a lock
// that lands after any caller-side pre-check still stops the mutation.
func TestPayrollMutations_CheckPeriodLockInWriteTransaction(t *testing.T) {
	setupAccountingTestDB(t)

	t.Run("mark paid into a locked day", func(t *testing.T) {
		business := createAccountingBusiness(t, "0xOwnerLockPaid", "lockpaid")
		service := NewService(database.GetDBWrapper())
		run := createDraftRunWithLine(t, service, business.ID, nil) // period_end 03-15
		lockBooksThrough(t, business.ID, time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC))

		_, err := service.MarkPayrollRunPaid(business.ID, run.ID, time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC), nil, nil)
		require.ErrorIs(t, err, ErrPeriodLocked, "period_end 03-15 is in the closed period")
		got, gerr := service.GetPayrollRun(business.ID, run.ID)
		require.NoError(t, gerr)
		require.Equal(t, database.PayrollRunStatusDraft, got.Status)
	})

	t.Run("void a run paid in a locked period", func(t *testing.T) {
		business := createAccountingBusiness(t, "0xOwnerLockVoid", "lockvoid")
		service := NewService(database.GetDBWrapper())
		run := createDraftRunWithLine(t, service, business.ID, nil)
		_, err := service.MarkPayrollRunPaid(business.ID, run.ID, time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC), nil, nil)
		require.NoError(t, err)
		lockBooksThrough(t, business.ID, time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC))

		_, err = service.VoidPayrollRun(business.ID, run.ID, time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC), nil, nil)
		require.ErrorIs(t, err, ErrPeriodLocked)
		got, gerr := service.GetPayrollRun(business.ID, run.ID)
		require.NoError(t, gerr)
		require.Equal(t, database.PayrollRunStatusPaid, got.Status)
	})

	t.Run("delete a draft whose period is locked", func(t *testing.T) {
		business := createAccountingBusiness(t, "0xOwnerLockDel", "lockdel")
		service := NewService(database.GetDBWrapper())
		run := createDraftRunWithLine(t, service, business.ID, nil)
		lockBooksThrough(t, business.ID, time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC))

		require.ErrorIs(t, service.DeletePayrollRun(business.ID, run.ID), ErrPeriodLocked)
		_, gerr := service.GetPayrollRun(business.ID, run.ID)
		require.NoError(t, gerr, "the draft must survive")
	})

	t.Run("open period still works", func(t *testing.T) {
		business := createAccountingBusiness(t, "0xOwnerLockOpen", "lockopen")
		service := NewService(database.GetDBWrapper())
		run := createDraftRunWithLine(t, service, business.ID, nil)
		lockBooksThrough(t, business.ID, time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC))

		paid, err := service.MarkPayrollRunPaid(business.ID, run.ID, time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC), nil, nil)
		require.NoError(t, err)
		require.Equal(t, database.PayrollRunStatusPaid, paid.Status)
	})
}
