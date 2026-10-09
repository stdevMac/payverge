package database

import "testing"

func TestPrintJob_TableName(t *testing.T) {
	if (PrintJob{}).TableName() != "print_jobs" {
		t.Fatalf("expected print_jobs, got %q", (PrintJob{}).TableName())
	}
}

func TestPrintJobStatus_IsValid(t *testing.T) {
	valid := []PrintJobStatus{
		PrintJobStatusPending, PrintJobStatusRouted, PrintJobStatusPrinting,
		PrintJobStatusPrinted, PrintJobStatusFailed, PrintJobStatusCancelled,
	}
	for _, s := range valid {
		if !s.IsValid() {
			t.Fatalf("expected %q to be valid", s)
		}
	}
	if PrintJobStatus("garbage").IsValid() {
		t.Fatalf("garbage should not be valid")
	}
}

func TestPrintJobKind_IsValid(t *testing.T) {
	for _, k := range []PrintJobKind{PrintJobKindBill, PrintJobKindReceipt} {
		if !k.IsValid() {
			t.Fatalf("expected %q to be valid", k)
		}
	}
	// Sprint 1 only accepts bill + receipt; kitchen/bar/void/modify
	// are still declared as constants for forward-compat but IsValid
	// returns true for them so Sprint 2 can use them without churn.
	for _, k := range []PrintJobKind{PrintJobKindKitchen, PrintJobKindBar, PrintJobKindVoid, PrintJobKindModify} {
		if !k.IsValid() {
			t.Fatalf("expected %q to be valid (forward-compat)", k)
		}
	}
}
