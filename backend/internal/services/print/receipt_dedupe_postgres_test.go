package print

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf"
)

func TestReceiptEnqueue_PostgresConcurrentConfirmationsCreateOneJob(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PostgreSQL receipt concurrency test in short mode")
	}
	ctx := context.Background()
	pg, err := testperf.StartIsolatedPostgres(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	require.NoError(t, pg.DB.Exec(`
		CREATE TABLE businesses (
			id BIGSERIAL PRIMARY KEY,
			default_language TEXT NOT NULL DEFAULT 'en'
		);
		CREATE TABLE bills (
			id BIGSERIAL PRIMARY KEY,
			business_id BIGINT NOT NULL REFERENCES businesses(id),
			status TEXT NOT NULL DEFAULT 'open',
			closed_at TIMESTAMPTZ NULL,
			settled_at TIMESTAMPTZ NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE printers (
			id BIGSERIAL PRIMARY KEY,
			business_id BIGINT NOT NULL REFERENCES businesses(id),
			location_id BIGINT NULL,
			name TEXT NOT NULL,
			role TEXT NOT NULL,
			transport TEXT NOT NULL,
			paper_width_mm INT NOT NULL DEFAULT 80,
			code_page TEXT NOT NULL DEFAULT 'CP858',
			cloudprnt_token TEXT NULL,
			cloudprnt_last_seen_at TIMESTAMPTZ NULL,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			fallback_printer_id BIGINT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE TABLE print_jobs (
			id BIGSERIAL PRIMARY KEY,
			business_id BIGINT NOT NULL REFERENCES businesses(id),
			location_id BIGINT NULL,
			printer_id BIGINT NULL,
			order_id BIGINT NULL,
			kind TEXT NOT NULL,
			source_type TEXT NOT NULL,
			source_id BIGINT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			payload_html TEXT NULL,
			payload_escpos BYTEA NULL,
			retries INT NOT NULL DEFAULT 0,
			attempt_count INT NOT NULL DEFAULT 0,
			max_attempts INT NOT NULL DEFAULT 6,
			next_attempt_at TIMESTAMPTZ NULL,
			last_error TEXT NULL,
			language TEXT NOT NULL DEFAULT 'en',
			created_by TEXT NOT NULL DEFAULT 'system',
			claimed_by VARCHAR(64) NULL,
			lease_expires_at TIMESTAMPTZ NULL,
			presented_at TIMESTAMPTZ NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			printed_at TIMESTAMPTZ NULL,
			kitchen_acked_at TIMESTAMPTZ NULL
		);
		INSERT INTO businesses (id, default_language) VALUES (1, 'en');
		INSERT INTO bills (id, business_id, status, closed_at)
		VALUES (1, 1, 'paid', NOW());
	`).Error)

	svc := NewService(pg.DB)
	const confirmations = 20
	enqueueWave := func() {
		t.Helper()
		start := make(chan struct{})
		errs := make(chan error, confirmations)
		var wg sync.WaitGroup
		for i := 0; i < confirmations; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, enqueueErr := svc.Enqueue(ctx, EnqueueParams{
					BusinessID: 1,
					Kind:       database.PrintJobKindReceipt,
					SourceType: "bill",
					SourceID:   1,
					Language:   "en",
					CreatedBy:  "system",
				})
				errs <- enqueueErr
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for enqueueErr := range errs {
			require.NoError(t, enqueueErr)
		}
	}
	enqueueWave()

	var count int64
	require.NoError(t, pg.DB.Model(&database.PrintJob{}).
		Where("business_id = 1 AND kind = ? AND source_type = 'bill' AND source_id = 1", database.PrintJobKindReceipt).
		Count(&count).Error)
	require.Equal(t, int64(1), count)

	// A refund/reversal clears closed_at; a later repayment writes a new paid
	// cycle boundary. Twenty callbacks for that later cycle still create exactly
	// one new receipt rather than reusing the first cycle or multiplying jobs.
	require.NoError(t, pg.DB.Exec(`
		UPDATE bills SET status = 'open', closed_at = NULL, updated_at = clock_timestamp() WHERE id = 1;
		SELECT pg_sleep(0.01);
		UPDATE bills SET status = 'paid', closed_at = clock_timestamp(), updated_at = clock_timestamp() WHERE id = 1;
	`).Error)
	enqueueWave()
	require.NoError(t, pg.DB.Model(&database.PrintJob{}).
		Where("business_id = 1 AND kind = ? AND source_type = 'bill' AND source_id = 1", database.PrintJobKindReceipt).
		Count(&count).Error)
	require.Equal(t, int64(2), count)
}
