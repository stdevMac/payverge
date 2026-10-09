package testperf

import (
	"context"
	"errors"
	"strings"
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

func TestWaitForPostgresPingRetriesStartupRace(t *testing.T) {
	pinger := &fakePinger{
		errs: []error{
			errors.New(`pq: the database system is starting up`),
			errors.New(`pq: the database system is starting up`),
			nil,
		},
	}

	err := waitForPostgresPing(context.Background(), pinger, 50*time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("waitForPostgresPing: %v", err)
	}
	if pinger.calls != 3 {
		t.Fatalf("calls = %d, want 3", pinger.calls)
	}
}

func TestIsolatedDatabaseDSNReplacesOnlyDatabaseName(t *testing.T) {
	got, err := isolatedDatabaseDSN(
		"postgres://test-user:test-pass@127.0.0.1:5432/shared?sslmode=disable&connect_timeout=5",
		"pv_test_123",
	)
	if err != nil {
		t.Fatalf("isolatedDatabaseDSN: %v", err)
	}
	if !strings.Contains(got, "/pv_test_123?") {
		t.Fatalf("child DSN %q does not contain isolated database name", got)
	}
	for _, option := range []string{"sslmode=disable", "connect_timeout=5"} {
		if !strings.Contains(got, option) {
			t.Errorf("child DSN %q lost %s", got, option)
		}
	}
}

func TestIsolatedDatabaseDSNRejectsUnsafeName(t *testing.T) {
	_, err := isolatedDatabaseDSN("postgres://test:test@localhost/shared", `bad"; DROP DATABASE shared;`)
	if err == nil {
		t.Fatal("unsafe database name must be rejected")
	}
}
