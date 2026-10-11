//go:build integration_postgres

package accounting

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf/genesisdb"
)

// A close that is mid-transaction holds the books lock. A concurrent
// mark-paid must wait for it and then see the new lock, instead of passing a
// stale check and committing paid_at into the closed period.
func TestMarkPayrollRunPaid_WaitsForConcurrentCloseAndSeesLock(t *testing.T) {
	ctx := context.Background()
	pg, err := genesisdb.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	previous := database.GetDB()
	database.SetTestDB(pg.DB)
	t.Cleanup(func() { database.SetTestDB(previous) })

	business := &database.Business{
		BusinessId: "payroll-lock-race", OwnerAddress: "0x1111111111111111111111111111111111111111",
		Name: "Lock Race", SettlementAddr: "0x2222222222222222222222222222222222222222",
		TippingAddr: "0x3333333333333333333333333333333333333333", Timezone: "UTC", IsActive: true,
	}
	require.NoError(t, pg.DB.Create(business).Error)
	run := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusDraft,
		Currency:    "USD",
	}
	require.NoError(t, pg.DB.Create(run).Error)

	service := NewService(database.GetDBWrapper())
	closeTx := pg.DB.Begin()
	require.NoError(t, closeTx.Error)
	require.NoError(t, LockBooksTx(closeTx, business.ID))
	through := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	require.NoError(t, closeTx.Create(&database.AccountingPeriodLock{
		BusinessID: business.ID, LockedThrough: &through, Note: "close March",
	}).Error)

	done := make(chan error, 1)
	go func() {
		_, err := service.MarkPayrollRunPaid(business.ID, run.ID, time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC), nil, nil)
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("mark-paid finished while the close held the books lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, closeTx.Commit().Error)

	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrPeriodLocked)
	case <-time.After(10 * time.Second):
		t.Fatal("mark-paid did not finish after the close committed")
	}
	var got database.PayrollRun
	require.NoError(t, pg.DB.Session(&gorm.Session{}).First(&got, run.ID).Error)
	require.Equal(t, database.PayrollRunStatusDraft, got.Status)
}
