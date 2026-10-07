package accounting

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPayrollDeletePostgres(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping PostgreSQL payroll deletion test in short mode")
	}

	ctx := context.Background()
	pg, err := testperf.StartIsolatedPostgres(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	require.NoError(t, pg.DB.Exec(`
		CREATE TABLE payroll_runs (
			id BIGSERIAL PRIMARY KEY,
			business_id BIGINT NOT NULL,
			period_start TIMESTAMPTZ NOT NULL,
			period_end TIMESTAMPTZ NOT NULL,
			status TEXT NOT NULL DEFAULT 'draft',
			currency VARCHAR(8),
			paid_at TIMESTAMPTZ,
			paid_by_user_id BIGINT,
			paid_by_staff_id BIGINT,
			voided_at TIMESTAMPTZ,
			voided_by_user_id BIGINT,
			voided_by_staff_id BIGINT,
			notes TEXT,
			gross_total BIGINT DEFAULT 0,
			bonus_total BIGINT DEFAULT 0,
			deduction_total BIGINT DEFAULT 0,
			net_total BIGINT DEFAULT 0,
			created_by_user_id BIGINT,
			created_by_staff_id BIGINT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE payroll_line_items (
			id BIGSERIAL PRIMARY KEY,
			payroll_run_id BIGINT NOT NULL REFERENCES payroll_runs(id),
			business_id BIGINT NOT NULL,
			payee_type TEXT NOT NULL,
			staff_id BIGINT,
			payee_name TEXT NOT NULL,
			gross_amount BIGINT DEFAULT 0,
			bonus_amount BIGINT DEFAULT 0,
			deduction_amount BIGINT DEFAULT 0,
			net_amount BIGINT DEFAULT 0,
			notes TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE accounting_period_locks (
			id BIGSERIAL PRIMARY KEY,
			business_id BIGINT NOT NULL,
			locked_through DATE,
			locked_by_user_id BIGINT,
			locked_by_staff_id BIGINT,
			note TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`).Error)

	previousDB := database.GetDB()
	database.SetTestDB(pg.DB)
	t.Cleanup(func() { database.SetTestDB(previousDB) })
	return NewService(database.GetDBWrapper()), pg.DB
}

func seedPayrollDeleteRun(t *testing.T, db *gorm.DB, businessID uint, status database.PayrollRunStatus) uint {
	t.Helper()
	var runID uint
	require.NoError(t, db.Raw(`
		INSERT INTO payroll_runs (business_id, period_start, period_end, status, currency)
		VALUES (?, '2026-07-01T00:00:00Z', '2026-07-15T00:00:00Z', ?, 'USD')
		RETURNING id
	`, businessID, status).Scan(&runID).Error)
	require.NotZero(t, runID)
	require.NoError(t, db.Exec(`
		INSERT INTO payroll_line_items
			(payroll_run_id, business_id, payee_type, payee_name, gross_amount, net_amount)
		VALUES (?, ?, 'contractor', 'Test Payee', 10000, 10000)
	`, runID, businessID).Error)
	return runID
}

func payrollDeleteCounts(t *testing.T, db *gorm.DB, runID uint) (int64, int64) {
	t.Helper()
	var runCount, lineCount int64
	require.NoError(t, db.Model(&database.PayrollRun{}).Where("id = ?", runID).Count(&runCount).Error)
	require.NoError(t, db.Model(&database.PayrollLineItem{}).Where("payroll_run_id = ?", runID).Count(&lineCount).Error)
	return runCount, lineCount
}

func TestDeletePayrollRun_PostgresSafety(t *testing.T) {
	service, db := setupPayrollDeletePostgres(t)

	t.Run("draft with line items deletes under foreign key enforcement", func(t *testing.T) {
		runID := seedPayrollDeleteRun(t, db, 101, database.PayrollRunStatusDraft)

		require.NoError(t, service.DeletePayrollRun(101, runID))
		runCount, lineCount := payrollDeleteCounts(t, db, runID)
		require.Zero(t, runCount)
		require.Zero(t, lineCount)
	})

	t.Run("non-draft runs cannot delete", func(t *testing.T) {
		for _, status := range []database.PayrollRunStatus{
			database.PayrollRunStatusPaid,
			database.PayrollRunStatusVoid,
		} {
			t.Run(string(status), func(t *testing.T) {
				runID := seedPayrollDeleteRun(t, db, 102, status)
				err := service.DeletePayrollRun(102, runID)
				require.ErrorContains(t, err, "draft")
				runCount, lineCount := payrollDeleteCounts(t, db, runID)
				require.Equal(t, int64(1), runCount)
				require.Equal(t, int64(1), lineCount)
			})
		}
	})

	t.Run("wrong tenant cannot delete", func(t *testing.T) {
		runID := seedPayrollDeleteRun(t, db, 103, database.PayrollRunStatusDraft)

		err := service.DeletePayrollRun(999, runID)
		require.ErrorContains(t, err, "not found")
		runCount, lineCount := payrollDeleteCounts(t, db, runID)
		require.Equal(t, int64(1), runCount)
		require.Equal(t, int64(1), lineCount)
	})

	t.Run("concurrent mark-paid and delete produce one valid outcome", func(t *testing.T) {
		for iteration := 0; iteration < 10; iteration++ {
			runID := seedPayrollDeleteRun(t, db, uint(200+iteration), database.PayrollRunStatusDraft)
			businessID := uint(200 + iteration)
			start := make(chan struct{})
			var wg sync.WaitGroup
			var deleteErr, paidErr error

			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				deleteErr = service.DeletePayrollRun(businessID, runID)
			}()
			go func() {
				defer wg.Done()
				<-start
				_, paidErr = service.MarkPayrollRunPaid(businessID, runID, time.Now().UTC(), nil, nil)
			}()
			close(start)
			wg.Wait()

			runCount, lineCount := payrollDeleteCounts(t, db, runID)
			switch {
			case deleteErr == nil:
				require.Error(t, paidErr, "mark-paid must not succeed after deletion")
				require.Zero(t, runCount)
				require.Zero(t, lineCount)
			case paidErr == nil:
				require.Error(t, deleteErr, "delete must not succeed after mark-paid")
				require.Equal(t, int64(1), runCount)
				require.Equal(t, int64(1), lineCount)
				var status database.PayrollRunStatus
				require.NoError(t, db.Raw("SELECT status FROM payroll_runs WHERE id = ?", runID).Scan(&status).Error)
				require.Equal(t, database.PayrollRunStatusPaid, status)
			default:
				t.Fatalf("iteration %d had no successful transition: delete=%v mark-paid=%v", iteration, deleteErr, paidErr)
			}
		}
	})

	t.Run("parent delete failure rolls back child delete", func(t *testing.T) {
		runID := seedPayrollDeleteRun(t, db, 301, database.PayrollRunStatusDraft)
		require.NoError(t, db.Exec(fmt.Sprintf(`
			CREATE FUNCTION reject_payroll_run_%d() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
				RAISE EXCEPTION 'injected payroll parent delete failure';
			END;
			$$;
			CREATE TRIGGER reject_payroll_run_%d
			BEFORE DELETE ON payroll_runs
			FOR EACH ROW WHEN (OLD.id = %d)
			EXECUTE FUNCTION reject_payroll_run_%d();
		`, runID, runID, runID, runID)).Error)

		err := service.DeletePayrollRun(301, runID)
		require.ErrorContains(t, err, "failed to delete payroll run")
		runCount, lineCount := payrollDeleteCounts(t, db, runID)
		require.Equal(t, int64(1), runCount, "parent must remain after rollback")
		require.Equal(t, int64(1), lineCount, "child deletion must roll back")
	})
}
