package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakePinger struct {
	errs  []error
	calls int
}

func (f *fakePinger) PingContext(context.Context) error {
	f.calls++
	if len(f.errs) == 0 {
		return nil
	}
	err := f.errs[0]
	f.errs = f.errs[1:]
	return err
}

func TestWaitForDatabaseRetriesHostReadinessRace(t *testing.T) {
	pinger := &fakePinger{
		errs: []error{
			errors.New("unexpected EOF"),
			errors.New("pq: the database system is starting up"),
			nil,
		},
	}

	err := waitForDatabase(context.Background(), pinger, 50*time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("waitForDatabase: %v", err)
	}
	if pinger.calls != 3 {
		t.Fatalf("calls = %d, want 3", pinger.calls)
	}
}

func TestRetryTransientDatabaseStartupRetriesUnexpectedEOF(t *testing.T) {
	calls := 0
	err := retryTransientDatabaseStartup(context.Background(), 50*time.Millisecond, time.Millisecond, func() error {
		calls++
		if calls < 3 {
			return errors.New("reconcile: failed to inspect existing schema before migrations: unexpected EOF")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryTransientDatabaseStartup: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestRetryTransientDatabaseStartupDoesNotRetryDirtyMigration(t *testing.T) {
	calls := 0
	err := retryTransientDatabaseStartup(context.Background(), 50*time.Millisecond, time.Millisecond, func() error {
		calls++
		return errors.New("schema_migrations is dirty (version=135); refusing to emit baseline")
	})
	if err == nil {
		t.Fatal("expected dirty migration error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestIsTransientDatabaseStartupError(t *testing.T) {
	transient := []string{
		"open gorm: unexpected EOF",
		"reconcile: pq: the database system is starting up",
		"read schema_migrations: driver: bad connection",
	}
	for _, msg := range transient {
		if !isTransientDatabaseStartupError(errors.New(msg)) {
			t.Fatalf("expected transient: %s", msg)
		}
	}
	if isTransientDatabaseStartupError(errors.New("schema_migrations is dirty")) {
		t.Fatal("dirty migration state must not be treated as transient")
	}
}
