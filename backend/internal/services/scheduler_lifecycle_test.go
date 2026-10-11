package services

import (
	"testing"

	"gorm.io/gorm"
)

// TestReportSchedulerStartStopStart asserts a second Start after Stop does not
// panic on a closed stopChan and runs cleanly.
func TestReportSchedulerStartStopStart(t *testing.T) {
	rs := NewReportScheduler(nil, nil, nil)
	if err := rs.Start(); err != nil {
		t.Fatal(err)
	}
	if err := rs.Stop(); err != nil {
		t.Fatal(err)
	}
	// Must not panic on close of an already-closed channel.
	if err := rs.Start(); err != nil {
		t.Fatalf("second Start failed: %v", err)
	}
	_ = rs.Stop()
}

// TestGuestFeedbackSchedulerStartStopStart asserts a Stop→Start cycle is safe.
func TestGuestFeedbackSchedulerStartStopStart(t *testing.T) {
	gfs := NewGuestFeedbackScheduler(nil)
	if err := gfs.Start(); err != nil {
		t.Fatal(err)
	}
	if err := gfs.Stop(); err != nil {
		t.Fatal(err)
	}
	// Must not panic on close of an already-closed channel.
	if err := gfs.Start(); err != nil {
		t.Fatalf("second Start failed: %v", err)
	}
	_ = gfs.Stop()
}

// TestLifecycleSchedulerStartStopStart asserts a Stop→Start cycle is safe.
func TestLifecycleSchedulerStartStopStart(t *testing.T) {
	ls := NewLifecycleScheduler(nil)
	if err := ls.Start(); err != nil {
		t.Fatal(err)
	}
	if err := ls.Stop(); err != nil {
		t.Fatal(err)
	}
	// Must not panic on close of an already-closed channel.
	if err := ls.Start(); err != nil {
		t.Fatalf("second Start failed: %v", err)
	}
	_ = ls.Stop()
}

// TestDirectorDigestSchedulerStartStopStart asserts a Stop→Start cycle is safe.
func TestDirectorDigestSchedulerStartStopStart(t *testing.T) {
	s := NewDirectorDigestScheduler((*gorm.DB)(nil), nil, nil)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	s.Stop()
	// Must not panic on close of an already-closed channel.
	if err := s.Start(); err != nil {
		t.Fatalf("second Start failed: %v", err)
	}
	s.Stop()
}

// TestInventoryAlertSchedulerStartStopStart asserts a Stop→Start cycle is safe.
func TestInventoryAlertSchedulerStartStopStart(t *testing.T) {
	s := NewInventoryAlertScheduler((*gorm.DB)(nil), nil)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	s.Stop()
	// Must not panic on close of an already-closed channel.
	if err := s.Start(); err != nil {
		t.Fatalf("second Start failed: %v", err)
	}
	s.Stop()
}

// TestMilestoneSchedulerStartStopStart asserts a Stop→Start cycle is safe.
func TestMilestoneSchedulerStartStopStart(t *testing.T) {
	ms := NewMilestoneScheduler(nil)
	if err := ms.Start(); err != nil {
		t.Fatal(err)
	}
	if err := ms.Stop(); err != nil {
		t.Fatal(err)
	}
	// Must not panic on close of an already-closed channel.
	if err := ms.Start(); err != nil {
		t.Fatalf("second Start failed: %v", err)
	}
	if err := ms.Stop(); err != nil {
		t.Fatal(err)
	}
}
