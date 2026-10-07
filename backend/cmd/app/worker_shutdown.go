package main

import (
	"context"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/logger"
)

// dbWorkerShutdownTimeout caps how long shutdown waits for DB-backed
// background workers to finish their current pass before the pool closes.
const dbWorkerShutdownTimeout = 10 * time.Second

// dbWorkerGroup owns background loops that read or write the database. Each
// loop gets its own cancellable context. Shutdown cancels them all and waits
// (up to a cap) for every loop to return. main calls it before sqlDB.Close, so
// a worker never runs a query against a closed pool and never stops halfway
// through a pass because the pool vanished under it.
type dbWorkerGroup struct {
	mu      sync.Mutex
	workers []dbWorker
}

type dbWorker struct {
	name   string
	cancel context.CancelFunc
	done   <-chan struct{}
}

// Go starts run on its own goroutine with a context that Shutdown cancels.
func (g *dbWorkerGroup) Go(name string, run func(ctx context.Context)) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	g.add(name, cancel, done)
	go func() {
		defer close(done)
		run(ctx)
	}()
}

// Track registers a loop that was started elsewhere and signals exit by
// closing done.
func (g *dbWorkerGroup) Track(name string, cancel context.CancelFunc, done <-chan struct{}) {
	g.add(name, cancel, done)
}

func (g *dbWorkerGroup) add(name string, cancel context.CancelFunc, done <-chan struct{}) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.workers = append(g.workers, dbWorker{name: name, cancel: cancel, done: done})
}

// Shutdown cancels every worker, then waits for them with one shared
// deadline. It returns the names of the workers that were still running when
// the deadline passed.
func (g *dbWorkerGroup) Shutdown(timeout time.Duration) []string {
	g.mu.Lock()
	workers := append([]dbWorker(nil), g.workers...)
	g.mu.Unlock()

	for _, w := range workers {
		w.cancel()
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var stuck []string
	for i, w := range workers {
		select {
		case <-w.done:
		case <-deadline.C:
			for _, rest := range workers[i:] {
				select {
				case <-rest.done:
				default:
					stuck = append(stuck, rest.name)
				}
			}
			if logger.Logger != nil {
				logger.Logger.Warnf("Background DB workers still running after %s: %v", timeout, stuck)
			}
			return stuck
		}
	}
	return nil
}
