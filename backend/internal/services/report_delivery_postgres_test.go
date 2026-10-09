package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf/genesisdb"
)

func TestReportDeliveryTwoWorkersSendOneWindow_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short")
	}
	ctx := context.Background()
	pg, err := genesisdb.Start(ctx)
	if err != nil {
		t.Fatalf("genesis postgres: %v (is Docker running, or can TEST_DATABASE_URL create child databases?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	due := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	require.NoError(t, pg.DB.Exec(`INSERT INTO businesses (business_id, owner_address, name, settlement_addr, tipping_addr, email, owner_name, created_at, updated_at) VALUES ('reports', '0x1111111111111111111111111111111111111111', 'Reports', '', '', 'owner@example.test', 'Owner', NOW(), NOW())`).Error)
	require.NoError(t, pg.DB.Exec(`
		INSERT INTO report_schedules (business_id, frequency, hour, minute, timezone, is_active, next_send_at)
		VALUES (1, 'daily', 8, 0, 'UTC', TRUE, ?)
	`, due).Error)

	previous := database.GetDB()
	database.SetTestDB(pg.DB)
	t.Cleanup(func() { database.SetTestDB(previous) })
	dbw := database.GetDBWrapper()
	sender := &fakeReportSender{}
	workerA := configuredReportWorker(dbw, &fakeReportRenderer{}, sender)
	workerB := configuredReportWorker(dbw, &fakeReportRenderer{}, sender)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, worker := range []*ReportScheduler{workerA, workerB} {
		wg.Add(1)
		go func(rs *ReportScheduler) {
			defer wg.Done()
			<-start
			rs.runOnce(due)
		}(worker)
	}
	close(start)
	wg.Wait()

	calls, unique, _ := sender.snapshot()
	require.Equal(t, 1, calls, "only the lease winner reaches transport")
	require.Equal(t, 1, unique)

	var rows []database.ReportDelivery
	require.NoError(t, pg.DB.Find(&rows).Error)
	require.Len(t, rows, 1, "unique schedule/window creates one outbox row")
	require.Equal(t, database.ReportDeliveryStateSent, rows[0].State)
	require.Equal(t, 1, rows[0].AttemptCount)

	var schedule database.ReportSchedule
	require.NoError(t, pg.DB.First(&schedule, 1).Error)
	require.NotNil(t, schedule.LastSentAt)
	require.True(t, schedule.NextSendAt.After(due))
}
