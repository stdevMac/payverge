package main

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A DB-backed worker must have exited before the pool is closed: shutdown
// cancels it, waits for the loop to return, and only then closes the DB.
func TestDBWorkerGroupShutdownStopsWorkersBeforeDBClose(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)

	var closed atomic.Bool
	var useAfterClose atomic.Int32
	var queries atomic.Int32
	sweep := func() {
		if closed.Load() {
			useAfterClose.Add(1)
		}
		var one int
		if err := db.Raw("SELECT 1").Scan(&one).Error; err == nil {
			queries.Add(1)
		}
	}

	var group dbWorkerGroup
	for _, name := range []string{"crypto_refunds", "reorg_watch"} {
		group.Go(name, func(ctx context.Context) {
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					// Finish the in-flight pass like a real sweep would.
					sweep()
					return
				case <-ticker.C:
					sweep()
				}
			}
		})
	}
	require.Eventually(t, func() bool { return queries.Load() > 4 }, 5*time.Second, time.Millisecond)

	require.Empty(t, group.Shutdown(5*time.Second))
	closed.Store(true)
	require.NoError(t, sqlDB.Close())
	time.Sleep(20 * time.Millisecond)
	require.Zero(t, useAfterClose.Load(), "no worker may touch the DB after it is closed")
}

func TestDBWorkerGroupShutdownIsCapped(t *testing.T) {
	var group dbWorkerGroup
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	group.Go("ignores-cancel", func(context.Context) { <-release })
	group.Go("well-behaved", func(ctx context.Context) { <-ctx.Done() })

	start := time.Now()
	stuck := group.Shutdown(50 * time.Millisecond)
	require.Less(t, time.Since(start), 2*time.Second)
	require.Equal(t, []string{"ignores-cancel"}, stuck)
}

func TestTrackedWorkerIsCancelledOnShutdown(t *testing.T) {
	var group dbWorkerGroup
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { <-ctx.Done(); close(done) }()
	group.Track("account_erasure", cancel, done)
	require.Empty(t, group.Shutdown(time.Second))
}

// main.go must hand the crypto refund worker, the reorg sweep and the account
// erasure janitor to the group and drain it before sqlDB.Close.
func TestMainDrainsDBWorkersBeforeClosingDB(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	require.NoError(t, err)
	src := string(raw)
	require.NotContains(t, src, "_ = cryptoRefundCancel")
	require.NotContains(t, src, "_ = reorgCancel")
	for _, name := range []string{`dbWorkers.Go("crypto_refunds"`, `dbWorkers.Go("reorg_watch"`, `dbWorkers.Track("account_erasure"`} {
		require.Contains(t, src, name)
	}
	drain := strings.Index(src, "dbWorkers.Shutdown(dbWorkerShutdownTimeout)")
	closeDB := strings.Index(src, "sqlDB.Close()")
	require.Positive(t, drain, "shutdown must drain DB workers")
	require.Positive(t, closeDB)
	require.Less(t, drain, closeDB, "DB workers must be drained before the pool closes")
}
